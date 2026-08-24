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
	"path/filepath"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/portableconformancev3"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "portable-host-conformance:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: portable-host-conformance self-test|run|suite [options]")
	}
	if args[0] == "suite" {
		return runStableSuite(args[1:], stdout)
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	// The current lane. The v1beta1 corpus is retained but not runnable by this
	// build: it drives the first protocol lane against the SECOND family
	// generation, a pairing decision 0047 makes invalid, and restoring the
	// valid one needs frozen interface and binding trees this repository has no
	// writer for (conformance/README.md).
	// No default: a run states which corpus it drives. Defaulting to one of
	// several runnable lanes is how the newest lane went unmeasured — the gate
	// invoked the command without --contract and never saw it.
	contractPath := flags.String("contract", "", "portable host contract directory (required)")
	endpoint := flags.String("endpoint", "", "disposable conformance endpoint origin")
	tokenEnv := flags.String("token-env", "", "environment variable containing the bearer token")
	alternateTokenEnv := flags.String(
		"alternate-token-env",
		"",
		"environment variable containing the same-tenant alternate-principal bearer token",
	)
	alternateTenantTokenEnv := flags.String(
		"alternate-tenant-token-env",
		"",
		"environment variable containing the same-principal alternate-tenant bearer token",
	)
	survey := flags.Bool(
		"survey",
		false,
		"continue past failing checks and report the whole gap surface as one failure; a survey never produces a passed report",
	)
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	// One lane. The retained v1alpha1/v1alpha2 corpora and their runner were
	// withdrawn with their epochs (decision 0042); this command verifies the
	// manifest identity and refuses anything that is not the current corpus.
	if strings.TrimSpace(*contractPath) == "" {
		return errors.New("--contract is required: a run states which corpus it drives")
	}
	format, err := manifestFormat(*contractPath)
	if err != nil {
		return err
	}
	if !portableconformancev3.DrivesManifestFormat(format) {
		return fmt.Errorf(
			"contract %s declares manifest format %q; this runner drives %s",
			*contractPath, format,
			strings.Join(portableconformancev3.DrivenManifestFormats(), " and "),
		)
	}
	return runV3(
		args[0], *contractPath, *endpoint,
		*tokenEnv, *alternateTokenEnv, *alternateTenantTokenEnv,
		*survey,
		stdout,
	)
}

func runStableSuite(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("suite", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "stable v1 conformance suite manifest (required)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if strings.TrimSpace(*manifestPath) == "" {
		return errors.New("suite requires --manifest")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	report, err := portableconformancev3.RunStableSuite(ctx, *manifestPath)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func manifestFormat(contractPath string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(contractPath, "manifest.json"))
	if err != nil {
		return "", err
	}
	var index struct {
		Format string `json:"format"`
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return "", fmt.Errorf("decode %s manifest: %w", contractPath, err)
	}
	return index.Format, nil
}

func runV3(
	command, contractPath, endpoint string,
	tokenEnv, alternateTokenEnv, alternateTenantTokenEnv string,
	survey bool,
	stdout io.Writer,
) error {
	contract, err := portableconformancev3.Verify(contractPath)
	if err != nil {
		return err
	}
	// The self-test drives an in-process host and finishes in seconds; a run
	// against a real endpoint pays real network and real convergence waits, and
	// a survey deliberately keeps going past failures. Two minutes measured the
	// reference host, not the task.
	deadline := 2 * time.Minute
	if command == "run" {
		deadline = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	var report portableconformancev3.HostRunnerReport
	switch command {
	case "self-test":
		if survey {
			return errors.New("self-test does not accept --survey; the reference host passes or the corpus is broken")
		}
		if endpoint != "" || tokenEnv != "" || alternateTokenEnv != "" || alternateTenantTokenEnv != "" {
			return errors.New(
				"self-test does not accept --endpoint, --token-env, --alternate-token-env, or --alternate-tenant-token-env",
			)
		}
		report, err = portableconformancev3.SelfTest(ctx, contract)
	case "run":
		if strings.TrimSpace(endpoint) == "" {
			return errors.New("run requires --endpoint")
		}
		token := ""
		alternateToken := ""
		alternateTenantToken := ""
		if tokenEnv != "" {
			token, err = requiredEnvironment(tokenEnv)
			if err != nil {
				return err
			}
		}
		if alternateTokenEnv != "" {
			alternateToken, err = requiredEnvironment(alternateTokenEnv)
			if err != nil {
				return err
			}
		}
		if alternateTenantTokenEnv != "" {
			alternateTenantToken, err = requiredEnvironment(alternateTenantTokenEnv)
			if err != nil {
				return err
			}
		}
		report, err = portableconformancev3.RunEndpoint(ctx, contract, portableconformancev3.EndpointOptions{
			Endpoint: endpoint, Token: token, AlternateToken: alternateToken,
			AlternateTenantToken: alternateTenantToken,
			HTTPClient:           &http.Client{Timeout: 30 * time.Second},
			Classification:       portableconformancev3.EndpointConformanceRun,
			Survey:               survey,
		})
	default:
		return fmt.Errorf("unknown command %q; use self-test or run", command)
	}
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func requiredEnvironment(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("--token-env must name one environment variable")
	}
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("environment variable %s is missing or empty", name)
	}
	return value, nil
}
