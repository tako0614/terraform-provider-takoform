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
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
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
	Negatives    []StandardNegativeFixture
}

// StandardNegativeFixture is one rejectable input a Form declares.
type StandardNegativeFixture struct {
	Name  string
	Stage string
	Input map[string]any
}

// StandardFixtureEvidence records only provider-protocol observations. The
// portable invalid_argument code is the normalized form of a provider
// configuration diagnostic that occurred before the mock host was mutated.
type StandardFixtureEvidence struct {
	Kind              string
	Identity          standardform.InstalledFormReference
	PositiveName      string
	PositivePassed    bool
	NegativeNames     []string
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
	if output, err := buildLocalProviderBinary(ctx, repoRoot, providerVersion, providerBinary); err != nil {
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

		negativeNames := make([]string, 0, len(fixture.Negatives))
		for index, negative := range fixture.Negatives {
			negativeDir := filepath.Join(temp, "negative", item.ResourceType, fmt.Sprintf("%d", index))
			if err := os.MkdirAll(negativeDir, 0o755); err != nil {
				return StandardFixtureRun{}, err
			}
			desired := negative.Input
			if negative.Stage == "observed" {
				desired = fixture.Positive
			}
			negativeConfig, err := standardFixtureConfig(server.URL, identity.ProviderAddress, providerVersion, item.ResourceType, desired)
			if err != nil {
				return StandardFixtureRun{}, fmt.Errorf("render %s %s fixture: %w", fixture.Kind, negative.Name, err)
			}
			if err := os.WriteFile(filepath.Join(negativeDir, "main.tf"), []byte(negativeConfig), 0o600); err != nil {
				return StandardFixtureRun{}, err
			}
			before := host.mutationCount(fixture.Kind)
			if negative.Stage == "observed" {
				host.setObservedOverride(fixture.Kind, negative.Input)
			}
			output, negativeErr := runCommand(ctx, repoRoot, env, cli, "-chdir="+negativeDir, "apply", "-auto-approve", "-input=false", "-no-color")
			if negative.Stage == "observed" {
				host.setObservedOverride("", nil)
				if name, ok := fixture.Positive["name"].(string); ok {
					host.removeResource(fixture.Kind, name)
				}
			}
			after := host.mutationCount(fixture.Kind)
			if negativeErr == nil {
				return StandardFixtureRun{}, fmt.Errorf("%s %s unexpectedly passed provider protocol", fixture.Kind, negative.Name)
			}
			switch negative.Stage {
			case "desired":
				if after != before {
					return StandardFixtureRun{}, fmt.Errorf("%s %s reached the Form host mutation path", fixture.Kind, negative.Name)
				}
				diagnosticField, diagnosticDetail, ok := standardNegativeDiagnostic(fixture.Kind, negative.Name, negative.Input)
				normalizedOutput := strings.Join(strings.Fields(output), " ")
				if !ok || !strings.Contains(normalizedOutput, "Error:") || !strings.Contains(normalizedOutput, diagnosticField) || !strings.Contains(normalizedOutput, diagnosticDetail) || strings.Contains(normalizedOutput, "Unsupported argument") || strings.Contains(normalizedOutput, "Invalid expression") {
					return StandardFixtureRun{}, fmt.Errorf("%s %s did not produce a provider configuration diagnostic\n%s", fixture.Kind, negative.Name, output)
				}
			case "observed":
				if after <= before {
					return StandardFixtureRun{}, fmt.Errorf("%s %s did not exercise the host response path", fixture.Kind, negative.Name)
				}
				normalizedOutput := strings.Join(strings.Fields(output), " ")
				if !strings.Contains(normalizedOutput, "Error: Host returned invalid portable Resource") ||
					!strings.Contains(normalizedOutput, "observed document does not satisfy the exact "+fixture.Kind+" Form contract") {
					return StandardFixtureRun{}, fmt.Errorf("%s %s did not reject the exact invalid observed fixture\n%s", fixture.Kind, negative.Name, output)
				}
			}
			negativeNames = append(negativeNames, negative.Name)
		}
		evidence = append(evidence, StandardFixtureEvidence{
			Kind: fixture.Kind, Identity: fixture.Identity, PositiveName: fixture.PositiveName, PositivePassed: true,
			NegativeNames: negativeNames, NegativeErrorCode: "invalid_argument", NegativePassed: true,
		})
	}

	return StandardFixtureRun{
		CLI: identity, ProviderBinary: ProviderBinaryIdentity{Version: providerVersion, SHA256: providerBinarySHA256}, Evidence: evidence,
	}, nil
}

// standardNegativeDiagnostic derives the attribute and message a rejected
// desired fixture must produce, from the required key or counter-example the
// Form itself declares.
//
// The negative case therefore always tests a constraint the Form states,
// rather than a separately maintained list that can drift away from it.
func standardNegativeDiagnostic(kind, fixtureName string, desired map[string]any) (string, string, bool) {
	declared, ok := formcatalog.ByKind(kind)
	if !ok {
		return "", "", false
	}
	attribute := strings.TrimPrefix(fixtureName, "reject-")
	if missing, ok := strings.CutPrefix(attribute, "missing-"); ok {
		switch missing {
		case "name":
			return "name", "required", true
		case "source":
			if declared.Artifact {
				return "artifact_url", "required", true
			}
		case "connections":
			if declared.Connections == formcatalog.ConnectionsRequired {
				return "connections", "required", true
			}
		default:
			for _, field := range declared.Fields {
				if field.Required && field.HCL == missing {
					return field.HCL, "required", true
				}
			}
		}
		return "", "", false
	}
	for _, field := range declared.Fields {
		if field.HCL != attribute {
			continue
		}
		return field.HCL, expectedDiagnosticDetail(field), true
	}
	switch attribute {
	case "artifact-source":
		if declared.Artifact {
			return "artifact_sha256", "64-character SHA-256", true
		}
	case "artifact-url-userinfo", "artifact-url-query", "artifact-url-fragment":
		if declared.Artifact {
			return "artifact_url", formcatalog.GrammarCredentialFreeHTTPSURL.Message("artifact_url"), true
		}
	case "connections-required", "connections-cardinality":
		if declared.Connections != formcatalog.ConnectionsAbsent {
			return "connections", "requires", true
		}
	}
	violations, err := declared.ConditionalViolations(desired)
	if err != nil || len(violations) != 1 {
		return "", "", false
	}
	field := camelToSnakeFixture(violations[0].WireField)
	for _, declaredField := range declared.Fields {
		if declaredField.Wire == violations[0].WireField {
			field = declaredField.HCL
			break
		}
	}
	return field, violations[0].Detail, true
}

func expectedDiagnosticDetail(field formcatalog.Field) string {
	// The expectation is derived from the same counter-example the fixture
	// carries, declared or derived, so the two can never describe different
	// violations.
	counter, ok := field.CounterExampleValue()
	if !ok {
		return "Invalid value"
	}
	if len(field.Enum) > 0 {
		if value, ok := counter.(string); ok {
			return fmt.Sprintf("%q is not a valid value", value)
		}
		if members, ok := counter.([]any); ok && len(members) > 0 {
			return fmt.Sprintf("%q is not a valid value", fmt.Sprint(members[0]))
		}
	}
	switch field.Type {
	case formcatalog.TypeInt:
		if number, ok := asInt64(counter); ok {
			if field.Min != nil && number < *field.Min {
				return fmt.Sprintf("value must be at least %d", *field.Min)
			}
			if field.Max != nil && number > *field.Max {
				return fmt.Sprintf("value must be at most %d", *field.Max)
			}
		}
	case formcatalog.TypeIntSet:
		if members, ok := counter.([]any); ok && len(members) > 0 {
			if number, ok := asInt64(members[0]); ok {
				return fmt.Sprintf("%d must be between %d and %d", number, boundOrDefault(field.Min, 0), boundOrDefault(field.Max, 0))
			}
		}
	}
	if _, hasGrammar := field.Grammar.Pattern(); hasGrammar {
		return field.Grammar.Message(field.HCL)
	}
	return "Invalid value"
}

func boundOrDefault(value *int64, fallback int64) int64 {
	if value == nil {
		return fallback
	}
	return *value
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
		if _, ok := resourceCaseForKind(fixture.Kind); !ok || strings.TrimSpace(fixture.PositiveName) == "" || len(fixture.Negatives) == 0 || fixture.Positive == nil ||
			fixture.Identity.FormRef.APIVersion != client.APIVersion || fixture.Identity.FormRef.Kind != fixture.Kind || strings.TrimSpace(fixture.Identity.FormRef.DefinitionVersion) == "" ||
			!validDigest(fixture.Identity.FormRef.SchemaDigest) || !validDigest(fixture.Identity.PackageDigest) {
			return nil, fmt.Errorf("standard fixture set contains an incomplete or unknown %q case", fixture.Kind)
		}
		for _, negative := range fixture.Negatives {
			if strings.TrimSpace(negative.Name) == "" || (negative.Stage != "desired" && negative.Stage != "observed") || negative.Input == nil {
				return nil, fmt.Errorf("standard fixture set contains an incomplete %s negative case", fixture.Kind)
			}
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
			// A configuration key is author-defined, so it may not be a bare
			// HCL identifier. Quoting keeps a rejected key a validation
			// diagnostic instead of a parse error.
			parts = append(parts, strconv.Quote(key)+" = "+rendered)
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
