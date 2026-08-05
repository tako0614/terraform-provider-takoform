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

	"github.com/tako0614/terraform-provider-takoform/internal/portableconformance"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "portable-host-conformance:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: portable-host-conformance self-test|run [options]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	contractPath := flags.String("contract", "conformance/portable-host-v2", "portable host contract directory")
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
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	contract, err := portableconformance.Verify(*contractPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var report portableconformance.HostRunnerReport
	switch args[0] {
	case "self-test":
		if *endpoint != "" || *tokenEnv != "" || *alternateTokenEnv != "" || *alternateTenantTokenEnv != "" {
			return errors.New(
				"self-test does not accept --endpoint, --token-env, --alternate-token-env, or --alternate-tenant-token-env",
			)
		}
		report, err = portableconformance.SelfTest(ctx, contract)
	case "run":
		if strings.TrimSpace(*endpoint) == "" {
			return errors.New("run requires --endpoint")
		}
		token := ""
		alternateToken := ""
		alternateTenantToken := ""
		if *tokenEnv != "" {
			token, err = requiredEnvironment(*tokenEnv)
			if err != nil {
				return err
			}
		}
		if *alternateTokenEnv != "" {
			alternateToken, err = requiredEnvironment(*alternateTokenEnv)
			if err != nil {
				return err
			}
		}
		if *alternateTenantTokenEnv != "" {
			alternateTenantToken, err = requiredEnvironment(*alternateTenantTokenEnv)
			if err != nil {
				return err
			}
		}
		report, err = portableconformance.RunEndpoint(ctx, contract, portableconformance.EndpointOptions{
			Endpoint: *endpoint, Token: token, AlternateToken: alternateToken,
			AlternateTenantToken: alternateTenantToken,
			HTTPClient:           &http.Client{Timeout: 30 * time.Second},
			Classification:       portableconformance.EndpointConformanceRun,
		})
	default:
		return fmt.Errorf("unknown command %q; use self-test or run", args[0])
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
