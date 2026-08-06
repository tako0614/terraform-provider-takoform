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

	"github.com/tako0614/terraform-provider-takoform/internal/portableconformance"
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
	format, err := manifestFormat(*contractPath)
	if err != nil {
		return err
	}
	if format == portableconformancev3.ManifestFormat {
		return runV3(
			args[0], *contractPath, *endpoint,
			*tokenEnv, *alternateTokenEnv, *alternateTenantTokenEnv,
			stdout,
		)
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

// manifestFormat reads only the corpus manifest identity so the command can
// dispatch to the retained v1/v2 lane or the Host API v1alpha3 lane.
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
	stdout io.Writer,
) error {
	contract, err := portableconformancev3.Verify(contractPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var report portableconformancev3.HostRunnerReport
	switch command {
	case "self-test":
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
