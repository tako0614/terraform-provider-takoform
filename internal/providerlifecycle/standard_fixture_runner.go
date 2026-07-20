package providerlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

// StandardFixtureCase carries exact desired fixtures from one verified Form
// Package or reviewed release source. Callers are responsible for verifying
// the exact package closure before invoking the provider protocol runner.
type StandardFixtureCase struct {
	Kind         string
	Identity     standardform.InstalledFormReference
	PositiveName string
	Positive     map[string]any
	NegativeName string
	Negative     map[string]any
}

// StandardFixtureEvidence records only provider-protocol observations. The
// portable invalid_argument code is the normalized form of a provider
// configuration diagnostic that occurred before the mock host was mutated.
type StandardFixtureEvidence struct {
	Kind              string
	Identity          standardform.InstalledFormReference
	PositiveName      string
	PositivePassed    bool
	NegativeName      string
	NegativeErrorCode string
	NegativePassed    bool
}

// StandardFixtureRun identifies the exact provider binary and CLI/FQN used to
// execute the retained fixture set through Terraform provider protocol v6.
type StandardFixtureRun struct {
	CLI            CLIIdentity
	ProviderBinary ProviderBinaryIdentity
	Evidence       []StandardFixtureEvidence
}

// RunStandardFixtures builds the real provider binary and applies each exact
// verified positive and negative desired fixture through a Terraform-compatible
// CLI. Negative fixtures must fail with provider diagnostics before the test
// Form host receives a mutation.
func RunStandardFixtures(ctx context.Context, repoRoot, cliPath string, cases []StandardFixtureCase) (StandardFixtureRun, error) {
	ordered, err := validateAndOrderStandardFixtureCases(cases)
	if err != nil {
		return StandardFixtureRun{}, err
	}
	cli, identity, err := identifyCLI(ctx, cliPath)
	if err != nil {
		return StandardFixtureRun{}, err
	}
	providerVersion, err := loadProviderVersion(repoRoot)
	if err != nil {
		return StandardFixtureRun{}, err
	}
	temp, err := os.MkdirTemp("", "takoform-standard-provider-fixtures-")
	if err != nil {
		return StandardFixtureRun{}, err
	}
	defer os.RemoveAll(temp)

	forms := make(map[string]client.InstalledFormReference, len(ordered))
	for _, fixture := range ordered {
		forms[fixture.Kind] = client.InstalledFormReference{
			FormRef: client.FormRef{
				APIVersion: fixture.Identity.FormRef.APIVersion, Kind: fixture.Identity.FormRef.Kind,
				DefinitionVersion: fixture.Identity.FormRef.DefinitionVersion, SchemaDigest: fixture.Identity.FormRef.SchemaDigest,
			},
			PackageDigest: fixture.Identity.PackageDigest,
		}
	}
	host := newFormHostWithForms(forms)
	server := httptest.NewServer(host)
	defer server.Close()

	binDir := filepath.Join(temp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return StandardFixtureRun{}, err
	}
	providerBinary := filepath.Join(binDir, "terraform-provider-takoform")
	if output, err := runCommand(ctx, repoRoot, nil, "go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-buildid= -X main.version="+providerVersion, "-o", providerBinary, "."); err != nil {
		return StandardFixtureRun{}, fmt.Errorf("build provider binary for standard fixtures: %w\n%s", err, output)
	}
	providerBinarySHA256, err := fileSHA256(providerBinary)
	if err != nil {
		return StandardFixtureRun{}, err
	}
	reportedVersion, err := runCommand(ctx, repoRoot, nil, providerBinary, "-version")
	if err != nil || strings.TrimSpace(reportedVersion) != providerVersion {
		return StandardFixtureRun{}, fmt.Errorf("standard fixture provider binary reported version %q, want %q: %w", strings.TrimSpace(reportedVersion), providerVersion, err)
	}

	cliConfig := filepath.Join(temp, "terraformrc")
	cliConfigBody := fmt.Sprintf(`provider_installation {
  dev_overrides {
    %q = %q
  }
  direct {}
}
`, identity.ProviderAddress, binDir)
	if err := os.WriteFile(cliConfig, []byte(cliConfigBody), 0o600); err != nil {
		return StandardFixtureRun{}, err
	}
	env := terraformRunnerEnvironment(cliConfig)

	evidence := make([]StandardFixtureEvidence, 0, len(ordered))
	for _, fixture := range ordered {
		item, _ := resourceCaseForKind(fixture.Kind)
		positiveDir := filepath.Join(temp, "positive", item.ResourceType)
		if err := os.MkdirAll(positiveDir, 0o755); err != nil {
			return StandardFixtureRun{}, err
		}
		positiveConfig, err := standardFixtureConfig(server.URL, identity.ProviderAddress, providerVersion, item.ResourceType, fixture.Positive)
		if err != nil {
			return StandardFixtureRun{}, fmt.Errorf("render %s positive fixture: %w", fixture.Kind, err)
		}
		if err := os.WriteFile(filepath.Join(positiveDir, "main.tf"), []byte(positiveConfig), 0o600); err != nil {
			return StandardFixtureRun{}, err
		}
		beforePositive := host.mutationCount(fixture.Kind)
		if output, err := runCommand(ctx, repoRoot, env, cli, "-chdir="+positiveDir, "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
			return StandardFixtureRun{}, fmt.Errorf("%s retained positive fixture did not pass provider protocol: %w\n%s", fixture.Kind, err, output)
		}
		if host.mutationCount(fixture.Kind) <= beforePositive {
			return StandardFixtureRun{}, fmt.Errorf("%s retained positive fixture did not reach the Form host mutation path", fixture.Kind)
		}
		if output, err := runCommand(ctx, repoRoot, env, cli, "-chdir="+positiveDir, "destroy", "-auto-approve", "-input=false", "-no-color"); err != nil {
			return StandardFixtureRun{}, fmt.Errorf("%s retained positive fixture cleanup: %w\n%s", fixture.Kind, err, output)
		}

		negativeDir := filepath.Join(temp, "negative", item.ResourceType)
		if err := os.MkdirAll(negativeDir, 0o755); err != nil {
			return StandardFixtureRun{}, err
		}
		negativeConfig, err := standardFixtureConfig(server.URL, identity.ProviderAddress, providerVersion, item.ResourceType, fixture.Negative)
		if err != nil {
			return StandardFixtureRun{}, fmt.Errorf("render %s negative fixture: %w", fixture.Kind, err)
		}
		if err := os.WriteFile(filepath.Join(negativeDir, "main.tf"), []byte(negativeConfig), 0o600); err != nil {
			return StandardFixtureRun{}, err
		}
		before := host.mutationCount(fixture.Kind)
		output, negativeErr := runCommand(ctx, repoRoot, env, cli, "-chdir="+negativeDir, "apply", "-auto-approve", "-input=false", "-no-color")
		after := host.mutationCount(fixture.Kind)
		if negativeErr == nil {
			return StandardFixtureRun{}, fmt.Errorf("%s retained negative fixture unexpectedly passed provider protocol", fixture.Kind)
		}
		if after != before {
			return StandardFixtureRun{}, fmt.Errorf("%s retained negative fixture reached the Form host mutation path", fixture.Kind)
		}
		diagnosticField, diagnosticDetail, ok := standardNegativeDiagnostic(fixture.Kind)
		if !ok || !strings.Contains(output, "Error:") || !strings.Contains(output, diagnosticField) || !strings.Contains(output, diagnosticDetail) || strings.Contains(output, "Unsupported argument") || strings.Contains(output, "Invalid expression") {
			return StandardFixtureRun{}, fmt.Errorf("%s retained negative fixture did not produce a provider configuration diagnostic\n%s", fixture.Kind, output)
		}
		evidence = append(evidence, StandardFixtureEvidence{
			Kind: fixture.Kind, Identity: fixture.Identity, PositiveName: fixture.PositiveName, PositivePassed: true,
			NegativeName: fixture.NegativeName, NegativeErrorCode: "invalid_argument", NegativePassed: true,
		})
	}

	return StandardFixtureRun{
		CLI: identity, ProviderBinary: ProviderBinaryIdentity{Version: providerVersion, SHA256: providerBinarySHA256}, Evidence: evidence,
	}, nil
}

func standardNegativeDiagnostic(kind string) (string, string, bool) {
	diagnostics := map[string][2]string{
		"EdgeWorker":             {"artifact_sha256", "artifact_sha256 is required when artifact_url"},
		"ObjectBucket":           {"storage_class", `"cold" is not a valid value`},
		"KVStore":                {"consistency", `"linearizable" is not a valid value`},
		"SQLDatabase":            {"engine", "portable capability-token grammar"},
		"Queue":                  {"max_retries", "value must be at least 0"},
		"VectorIndex":            {"dimensions", "value must be at least 1"},
		"DurableWorkflow":        {"max_attempts", "value must be at least 1"},
		"ContainerService":       {"ports", "0 must be between 1 and 65535"},
		"StatefulActorNamespace": {"class_name", "portable runtime class grammar"},
		"Schedule":               {"cron", "portable five-field expression"},
	}
	diagnostic, ok := diagnostics[kind]
	return diagnostic[0], diagnostic[1], ok
}

func terraformRunnerEnvironment(cliConfig string) []string {
	return append(sanitizedTerraformBaseEnvironment(),
		"TF_CLI_CONFIG_FILE="+cliConfig,
		"TF_IN_AUTOMATION=1",
		"CHECKPOINT_DISABLE=1",
		"TF_PLUGIN_CACHE_DIR=",
	)
}

func sanitizedTerraformBaseEnvironment() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "TF_") || strings.HasPrefix(key, "TOFU_") || strings.HasPrefix(key, "TAKOFORM_") || key == "CHECKPOINT_DISABLE" {
			continue
		}
		environment = append(environment, entry)
	}
	return environment
}

func validateAndOrderStandardFixtureCases(cases []StandardFixtureCase) ([]StandardFixtureCase, error) {
	if len(cases) != len(resourceCases) {
		return nil, fmt.Errorf("standard fixture set has %d kinds, want exactly %d", len(cases), len(resourceCases))
	}
	byKind := make(map[string]StandardFixtureCase, len(cases))
	for _, fixture := range cases {
		if _, ok := byKind[fixture.Kind]; ok {
			return nil, fmt.Errorf("standard fixture set duplicates %s", fixture.Kind)
		}
		if _, ok := resourceCaseForKind(fixture.Kind); !ok || strings.TrimSpace(fixture.PositiveName) == "" || strings.TrimSpace(fixture.NegativeName) == "" || fixture.Positive == nil || fixture.Negative == nil ||
			fixture.Identity.FormRef.APIVersion != client.APIVersion || fixture.Identity.FormRef.Kind != fixture.Kind || strings.TrimSpace(fixture.Identity.FormRef.DefinitionVersion) == "" ||
			!validDigest(fixture.Identity.FormRef.SchemaDigest) || !validDigest(fixture.Identity.PackageDigest) {
			return nil, fmt.Errorf("standard fixture set contains an incomplete or unknown %q case", fixture.Kind)
		}
		byKind[fixture.Kind] = fixture
	}
	ordered := make([]StandardFixtureCase, 0, len(resourceCases))
	for _, item := range resourceCases {
		fixture, ok := byKind[item.Kind]
		if !ok {
			return nil, fmt.Errorf("standard fixture set omits %s", item.Kind)
		}
		ordered = append(ordered, fixture)
	}
	return ordered, nil
}

func resourceCaseForKind(kind string) (resourceCase, bool) {
	for _, item := range resourceCases {
		if item.Kind == kind {
			return item, true
		}
	}
	return resourceCase{}, false
}

func (h *formHost) mutationCount(kind string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	counts := h.counts[kind]
	if counts == nil {
		return 0
	}
	return counts.create + counts.update + counts.nativeImport + counts.cliImport + counts.delete
}

func standardFixtureConfig(endpoint, providerAddress, providerVersion, resourceType string, desired map[string]any) (string, error) {
	body, err := renderStandardDesired(desired)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`terraform {
  required_providers {
    takoform = { source = %q, version = %q }
  }
}
provider "takoform" {
  endpoint = %q
  space = "prod"
}
resource %q "fixture" {
%s}
`, providerAddress, providerVersion, endpoint, resourceType, body), nil
}

func renderStandardDesired(desired map[string]any) (string, error) {
	projected := make(map[string]any, len(desired)+4)
	for name, value := range desired {
		switch name {
		case "source":
			source, ok := value.(map[string]any)
			if !ok {
				return "", fmt.Errorf("source is not an object")
			}
			for sourceName, sourceValue := range source {
				projected[camelToSnakeFixture(sourceName)] = sourceValue
			}
		case "delivery":
			delivery, ok := value.(map[string]any)
			if !ok {
				return "", fmt.Errorf("delivery is not an object")
			}
			for deliveryName, deliveryValue := range delivery {
				projected[camelToSnakeFixture(deliveryName)] = deliveryValue
			}
		case "retry":
			retry, ok := value.(map[string]any)
			if !ok {
				return "", fmt.Errorf("retry is not an object")
			}
			for retryName, retryValue := range retry {
				projected[camelToSnakeFixture(retryName)] = retryValue
			}
		case "connections":
			connections, ok := value.(map[string]any)
			if !ok {
				return "", fmt.Errorf("connections is not an object")
			}
			names := make([]string, 0, len(connections))
			for connectionName := range connections {
				names = append(names, connectionName)
			}
			sort.Strings(names)
			items := make([]any, 0, len(names))
			for _, connectionName := range names {
				connection, ok := connections[connectionName].(map[string]any)
				if !ok {
					return "", fmt.Errorf("connection %q is not an object", connectionName)
				}
				item := make(map[string]any, len(connection)+1)
				item["name"] = connectionName
				for field, fieldValue := range connection {
					item[field] = fieldValue
				}
				items = append(items, item)
			}
			projected["connections"] = items
		default:
			projected[camelToSnakeFixture(name)] = value
		}
	}
	keys := make([]string, 0, len(projected))
	for key := range projected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var body strings.Builder
	for _, key := range keys {
		value, err := renderHCLValue(projected[key])
		if err != nil {
			return "", fmt.Errorf("%s: %w", key, err)
		}
		fmt.Fprintf(&body, "  %s = %s\n", key, value)
	}
	return body.String(), nil
}

func renderHCLValue(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		raw, _ := json.Marshal(typed)
		return string(raw), nil
	case bool:
		return strconv.FormatBool(typed), nil
	case json.Number:
		if _, err := typed.Int64(); err != nil {
			return "", fmt.Errorf("number %q is not an integer", typed)
		}
		return typed.String(), nil
	case float64:
		if math.Trunc(typed) != typed {
			return "", fmt.Errorf("number %v is not an integer", typed)
		}
		return strconv.FormatInt(int64(typed), 10), nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			rendered, err := renderHCLValue(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, rendered)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			rendered, err := renderHCLValue(typed[key])
			if err != nil {
				return "", err
			}
			parts = append(parts, key+" = "+rendered)
		}
		return "{ " + strings.Join(parts, ", ") + " }", nil
	default:
		return "", fmt.Errorf("unsupported JSON value type %T", value)
	}
}

func camelToSnakeFixture(value string) string {
	var result strings.Builder
	for index, character := range value {
		if index > 0 && character >= 'A' && character <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(character)
	}
	return strings.ToLower(result.String())
}
