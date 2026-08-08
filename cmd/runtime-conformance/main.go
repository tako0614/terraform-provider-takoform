// Command runtime-conformance executes the ES Module Worker runtime ABI
// conformance corpus.
//
// It is a separate command from portable-host-conformance on purpose. That
// command measures a CONTROL PLANE: it takes a host endpoint and three
// tenant/principal credentials, drives the Host API, and reports what a host
// says and refuses. This one measures a RUNTIME: it takes the base URL of a
// worker deployed from the corpus bundle, an optional adapter over the
// runtime's module loader, and reports what an isolate did. The two answer
// different questions about different subjects and their reports are not
// interchangeable, so one command name for both would make a passing report
// ambiguous about what was actually measured
// (spec/decisions/0023-the-runtime-abi-is-measured-separately-from-the-control-plane.md).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "runtime-conformance:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: runtime-conformance verify|self-test|run [options]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	contractPath := flags.String(
		"contract", "conformance/runtime-abi-v1", "runtime ABI conformance corpus directory")
	endpoint := flags.String(
		"endpoint", "", "base URL of the worker deployed from the corpus bundle")
	loaderEndpoint := flags.String(
		"loader-endpoint", "",
		"disposable adapter over the runtime's module loader; without it the load lane is unmeasured")
	tokenEnv := flags.String(
		"token-env", "", "environment variable carrying the worker's bearer credential")
	loaderTokenEnv := flags.String(
		"loader-token-env", "", "environment variable carrying the loader adapter's bearer credential")
	deadline := flags.Duration(
		"deadline", 10*time.Minute, "overall deadline for the run")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	contract, err := runtimeconformance.Verify(*contractPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *deadline)
	defer cancel()

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")

	switch args[0] {
	case "verify":
		if *endpoint != "" || *loaderEndpoint != "" || *tokenEnv != "" || *loaderTokenEnv != "" {
			return errors.New("verify loads the corpus and takes no endpoint or credential")
		}
		return encoder.Encode(map[string]any{
			"format":         "takoform.runtime-abi-corpus-verification@v1",
			"contract":       *contractPath,
			"contractDigest": contract.Digest(),
			"interface":      contract.Interface,
			"checks":         contract.RequiredChecks,
		})
	case "self-test":
		if *endpoint != "" || *loaderEndpoint != "" || *tokenEnv != "" || *loaderTokenEnv != "" {
			return errors.New(
				"self-test runs against the in-process stand-in and takes no endpoint or credential")
		}
		report, runErr := runtimeconformance.SelfTest(ctx, contract)
		if err := encoder.Encode(report); err != nil {
			return err
		}
		return runErr
	case "run":
		if strings.TrimSpace(*endpoint) == "" {
			return errors.New("run requires --endpoint")
		}
		token, err := optionalEnvironment(*tokenEnv)
		if err != nil {
			return err
		}
		loaderToken, err := optionalEnvironment(*loaderTokenEnv)
		if err != nil {
			return err
		}
		report, runErr := runtimeconformance.RunEndpoint(ctx, contract, runtimeconformance.EndpointOptions{
			Endpoint:       *endpoint,
			LoaderEndpoint: *loaderEndpoint,
			Token:          token,
			LoaderToken:    loaderToken,
			HTTPClient:     &http.Client{Timeout: 2 * time.Minute},
			Classification: runtimeconformance.DeployedRuntimeRun,
		})
		if err := encoder.Encode(report); err != nil {
			return err
		}
		return runErr
	default:
		return fmt.Errorf("unknown command %q; use verify, self-test or run", args[0])
	}
}

// optionalEnvironment reads a credential the operator's deployment may or may
// not require. Naming a variable that is empty is an error; naming none is not.
func optionalEnvironment(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("environment variable %s is missing or empty", name)
	}
	return value, nil
}
