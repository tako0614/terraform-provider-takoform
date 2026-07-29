package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/portableconformance"
)

const (
	portableAPI = "forms.takoform.com/v1alpha1"
	providerFQN = "registry.terraform.io/tako0614/takoform"
)

type hostContract struct {
	APIGroup          string                                  `json:"apiGroup"`
	DiscoveryPath     string                                  `json:"discoveryPath"`
	OptionalFeatures  []string                                `json:"optionalFeatures"`
	OptionalEndpoints []string                                `json:"optionalEndpoints"`
	PlanBinding       portableconformance.PlanBindingContract `json:"planBinding"`
	Idempotency       portableconformance.IdempotencyContract `json:"idempotency"`
	WireJSON          portableconformance.WireJSONContract    `json:"wireJSON"`
	Operations        []struct {
		Name           string `json:"name"`
		Method         string `json:"method"`
		Path           string `json:"path"`
		Precondition   string `json:"precondition"`
		IdempotencyKey bool   `json:"idempotencyKey"`
		Mutates        bool   `json:"mutates"`
		Optional       bool   `json:"optional"`
	} `json:"operations"`
	ErrorEnvelope struct {
		Codes                  []string `json:"codes"`
		AutomaticallyRetryable []string `json:"automaticallyRetryable"`
	} `json:"errorEnvelope"`
}

type releaseLock struct {
	Version           string `json:"version"`
	Tag               string `json:"tag"`
	ProviderAddress   string `json:"providerAddress"`
	PublicationStatus string `json:"publicationStatus"`
	CLIMatrix         []struct {
		Product         string `json:"product"`
		ProviderAddress string `json:"providerAddress"`
	} `json:"cliMatrix"`
	Versioning struct {
		PortableAPI string `json:"portableApiVersion"`
	} `json:"versioning"`
}

type packageInventory struct {
	Packages []struct {
		Kind          string              `json:"kind"`
		Path          string              `json:"path"`
		FormRef       formpackage.FormRef `json:"formRef"`
		PackageDigest string              `json:"packageDigest"`
	} `json:"packages"`
}

// This is the cross-document gate. Individual schema, package, provider, and
// runner implementations keep their deeper checks in their owning packages.
func TestNormativeIdentitiesAgree(t *testing.T) {
	t.Parallel()
	host := readFile[hostContract](t, "host-api", "operations.json")
	release := readFile[releaseLock](t, "..", "release", "version.json")
	trust := readFile[struct {
		Provider struct {
			Distribution struct {
				Registry string `json:"registry"`
			} `json:"distribution"`
		} `json:"provider"`
	}](t, "trust", "profile.json")
	conformance := readConformance(t)
	inventory := readFile[packageInventory](t, "..", "forms", "standard-package-set.json")

	identities := map[string]string{
		"host API":             host.APIGroup,
		"host conformance":     conformance.APIVersion,
		"host runner FormRef":  conformance.RunnerInput.Identity.FormRef.APIVersion,
		"release API lock":     release.Versioning.PortableAPI,
		"FormRef schema":       formRefSchemaAPI(t),
		"release provider FQN": release.ProviderAddress,
		"trust provider FQN":   trust.Provider.Distribution.Registry,
	}
	for source, got := range identities {
		want := portableAPI
		if strings.Contains(source, "provider FQN") {
			want = providerFQN
		}
		if got != want {
			t.Errorf("%s = %q, want %q", source, got, want)
		}
	}
	if host.DiscoveryPath != conformance.DiscoveryPath {
		t.Errorf("discovery path disagrees: host=%q conformance=%q", host.DiscoveryPath, conformance.DiscoveryPath)
	}
	if release.Version != "1.0.0" || release.Tag != "v"+release.Version || release.PublicationStatus != "candidate-only" {
		t.Errorf("provider candidate lock disagrees: version=%q tag=%q status=%q", release.Version, release.Tag, release.PublicationStatus)
	}
	products := map[string]bool{}
	for _, cli := range release.CLIMatrix {
		products[cli.Product] = true
		if cli.ProviderAddress != providerFQN {
			t.Errorf("%s provider FQN = %q, want %q", cli.Product, cli.ProviderAddress, providerFQN)
		}
	}
	if len(release.CLIMatrix) != 2 || !products["OpenTofu"] || !products["Terraform"] {
		t.Errorf("provider CLI matrix must contain exactly OpenTofu and Terraform: %#v", products)
	}

	var runnerMatch bool
	for _, entry := range inventory.Packages {
		if entry.FormRef.APIVersion != portableAPI {
			t.Errorf("%s FormRef API = %q", entry.Kind, entry.FormRef.APIVersion)
		}
		if entry.Kind != conformance.RunnerInput.Identity.FormRef.Kind {
			continue
		}
		ref := conformance.RunnerInput.Identity.FormRef
		runnerMatch = entry.FormRef.APIVersion == ref.APIVersion &&
			entry.FormRef.Kind == ref.Kind &&
			entry.FormRef.DefinitionVersion == ref.DefinitionVersion &&
			entry.FormRef.SchemaDigest == ref.SchemaDigest &&
			entry.PackageDigest == conformance.RunnerInput.Identity.PackageDigest
	}
	if !runnerMatch {
		t.Error("portable-host runner does not pin one exact current Form/package identity")
	}
}

func TestHostAndConformanceContractsAgree(t *testing.T) {
	t.Parallel()
	host := readFile[hostContract](t, "host-api", "operations.json")
	conformance := readConformance(t)

	if !reflect.DeepEqual(host.ErrorEnvelope.Codes, conformance.StableErrorCodes) {
		t.Errorf("stable error taxonomy disagrees:\nhost=%v\nconformance=%v", host.ErrorEnvelope.Codes, conformance.StableErrorCodes)
	}
	if !reflect.DeepEqual(host.ErrorEnvelope.AutomaticallyRetryable, conformance.RetryableCodes) {
		t.Errorf("retry taxonomy disagrees: host=%v conformance=%v", host.ErrorEnvelope.AutomaticallyRetryable, conformance.RetryableCodes)
	}
	if !reflect.DeepEqual(host.PlanBinding, conformance.PlanBinding) {
		t.Errorf("plan binding contract disagrees: host=%#v conformance=%#v", host.PlanBinding, conformance.PlanBinding)
	}
	if !reflect.DeepEqual(host.Idempotency, conformance.Idempotency) {
		t.Errorf("idempotency authorization contract disagrees: host=%#v conformance=%#v", host.Idempotency, conformance.Idempotency)
	}
	if host.WireJSON != conformance.WireJSON {
		t.Errorf("raw JSON contract disagrees: host=%#v conformance=%#v", host.WireJSON, conformance.WireJSON)
	}

	var idempotent []string
	preconditions := map[string]string{}
	interfacePaths := map[string]bool{}
	for _, operation := range host.Operations {
		preconditions[operation.Name] = operation.Precondition
		if operation.IdempotencyKey {
			idempotent = append(idempotent, operation.Name)
		}
		if strings.Contains(strings.ToLower(operation.Name), "drift") ||
			strings.Contains(strings.ToLower(operation.Path), "/drift") {
			t.Errorf("drift is observed evidence, not a host operation: %#v", operation)
		}
		interfaceSurface := strings.Contains(strings.ToLower(operation.Name), "interface") ||
			strings.Contains(strings.ToLower(operation.Path), "/interface")
		if !interfaceSurface {
			continue
		}
		if !strings.HasPrefix(operation.Path, "/interfaces") {
			t.Errorf("host contract exposes an unreviewed Interface operation: %#v", operation)
			continue
		}
		if operation.Method != "GET" || operation.Mutates || operation.IdempotencyKey ||
			!operation.Optional || operation.Precondition != "none" {
			t.Errorf("Interface operation is not a pure optional read: %#v", operation)
		}
		if interfacePaths[operation.Path] {
			t.Errorf("host contract duplicates Interface path %q", operation.Path)
		}
		interfacePaths[operation.Path] = true
	}
	if !reflect.DeepEqual(idempotent, conformance.IdempotentOperations) {
		t.Errorf("idempotent operations disagree: host=%v conformance=%v", idempotent, conformance.IdempotentOperations)
	}
	wantPreconditions := map[string]string{
		"apply": "if-none-match-on-create-otherwise-if-match", "import": "if-none-match-on-create-otherwise-if-match",
		"observe": "if-match", "refresh": "if-match", "delete": "if-match",
	}
	for operation, want := range wantPreconditions {
		if preconditions[operation] != want {
			t.Errorf("%s precondition = %q, want %q", operation, preconditions[operation], want)
		}
	}
	declaration := conformance.InterfaceDeclarations
	if !declaration.Optional || !declaration.ReadOnly ||
		declaration.MaterializationSource != "form-declared-descriptors" ||
		declaration.EndpointOriginRule != "same-origin-with-api" ||
		!contains(host.OptionalFeatures, declaration.FeatureFlag) ||
		!contains(host.OptionalEndpoints, declaration.EndpointKey) {
		t.Errorf("Interface boundary disagrees between host and conformance: %#v", declaration)
	}
	wantInterfacePaths := map[string]bool{
		strings.TrimPrefix(declaration.ListPath, "{api}"): true,
		strings.TrimPrefix(declaration.GetPath, "{api}"):  true,
	}
	if !reflect.DeepEqual(interfacePaths, wantInterfacePaths) {
		t.Errorf("Interface paths disagree: host=%v conformance=%v", interfacePaths, wantInterfacePaths)
	}
}

func TestCurrentFormCapabilitySemanticsAreExact(t *testing.T) {
	t.Parallel()
	inventory := readFile[packageInventory](t, "..", "forms", "standard-package-set.json")
	var indexed *formpackage.FormDefinition
	for _, entry := range inventory.Packages {
		definition := readFile[formpackage.FormDefinition](
			t,
			"..",
			filepath.FromSlash(entry.Path),
			"definition.json",
		)
		if contains(definition.LifecycleCapabilities, "drift") &&
			!contains(definition.LifecycleCapabilities, "observe") {
			t.Errorf("%s declares drift outcome support without observation capability", entry.Kind)
		}
		if entry.Kind == "IndexedStore" {
			indexed = &definition
		}
	}
	if indexed == nil {
		t.Fatal("current Form inventory omits IndexedStore")
	}
	if len(indexed.Interfaces) != 1 {
		t.Fatalf("IndexedStore Interfaces = %d, want only data.indexed@1", len(indexed.Interfaces))
	}

	var descriptor *formpackage.InterfaceDescriptor
	for index := range indexed.Interfaces {
		candidate := &indexed.Interfaces[index]
		if candidate.Name == "data.indexed" && candidate.Version == "1" {
			descriptor = candidate
			break
		}
	}
	if descriptor == nil {
		t.Fatal("IndexedStore omits data.indexed@1")
	}
	wantDocument := map[string]any{
		"operations": []any{"delete", "get", "put", "query"},
	}
	wantInputs := []formpackage.InterfaceInputDeclaration{
		{Name: "resource", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/id"},
		{Name: "name", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/name"},
	}
	if !descriptor.Required ||
		descriptor.ResourceURIInput != "" ||
		!reflect.DeepEqual(descriptor.Document, wantDocument) ||
		!reflect.DeepEqual(descriptor.Inputs, wantInputs) {
		t.Fatalf("data.indexed@1 exceeds its current operation/input descriptor: %#v", descriptor)
	}
	properties, ok := descriptor.DocumentSchema["properties"].(map[string]any)
	if !ok || len(properties) != 1 || properties["operations"] == nil {
		t.Fatalf("data.indexed@1 document schema defines more than operations: %#v", descriptor.DocumentSchema)
	}
	raw, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	for _, unimplemented := range []string{"endpoint", "schemaDigest", "resourceUriInput"} {
		if strings.Contains(string(raw), unimplemented) {
			t.Errorf("data.indexed@1 retains unimplemented %s binding", unimplemented)
		}
	}
	for _, retiredPrototype := range []string{
		filepath.Join("..", "internal", "indexedsql", "contract.go"),
		filepath.Join("..", "conformance", "data-indexed-v1", "manifest.json"),
		filepath.Join("data-indexed", "request.schema.json"),
		filepath.Join("data-indexed", "response.schema.json"),
	} {
		if _, err := os.Stat(retiredPrototype); !os.IsNotExist(err) {
			t.Errorf("retired table-shaped data.indexed prototype remains at %s", retiredPrototype)
		}
	}

	schema := readFile[struct {
		Properties struct {
			Positive struct {
				MaxItems int `json:"maxItems"`
			} `json:"conformanceFixtures"`
			Negative struct {
				MaxItems int `json:"maxItems"`
			} `json:"negativeConformanceFixtures"`
		} `json:"properties"`
	}](t, "schemas", "form-definition.schema.json")
	if schema.Properties.Positive.MaxItems != 32 || schema.Properties.Negative.MaxItems != 32 {
		t.Errorf(
			"fixture class maxima must independently remain positive=32 and negative=32: positive=%d negative=%d",
			schema.Properties.Positive.MaxItems,
			schema.Properties.Negative.MaxItems,
		)
	}
}

func TestPortableNormativeSurfacesStayBackendNeutral(t *testing.T) {
	t.Parallel()
	conformance := readConformance(t)
	for _, field := range []string{"credential", "secret", "price", "billing", "backend", "selected_implementation", "target"} {
		if !contains(conformance.ForbiddenProviderState, field) {
			t.Errorf("provider state no longer forbids %q", field)
		}
	}
	if err := formpackage.ValidatePortableData(conformance.RunnerInput.Desired); err != nil {
		t.Fatalf("portable-host runner desired state leaks host authority: %v", err)
	}

	var paths []string
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && filepath.Ext(path) == ".json" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	inventory := readFile[packageInventory](t, "..", "forms", "standard-package-set.json")
	paths = append(paths,
		filepath.Join("..", "conformance", "portable-host-v1", "contract.json"),
		filepath.Join("..", "forms", "standard-package-set.json"),
	)
	for _, entry := range inventory.Packages {
		root := filepath.Join("..", filepath.FromSlash(entry.Path))
		for _, name := range []string{
			"definition.json", "package-index.json", "fixtures/desired.json",
			"fixtures/observed.json", "fixtures/output.json",
		} {
			paths = append(paths, filepath.Join(root, filepath.FromSlash(name)))
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(raw))
		for _, token := range []string{
			"cloudflare", "workers.dev", "compatibility_date", "compatibilitydate",
			"compatibility_flags", "compatibilityflags", "wrangler",
		} {
			if strings.Contains(lower, token) {
				t.Errorf("%s leaks backend-specific token %q", path, token)
			}
		}
	}
}

func formRefSchemaAPI(t *testing.T) string {
	t.Helper()
	schema := readFile[struct {
		Properties struct {
			APIVersion struct {
				Const string `json:"const"`
			} `json:"apiVersion"`
		} `json:"properties"`
	}](t, "schemas", "form-ref.schema.json")
	return schema.Properties.APIVersion.Const
}

func readConformance(t *testing.T) portableconformance.Contract {
	t.Helper()
	contract, err := portableconformance.Verify(filepath.Join("..", "conformance", "portable-host-v1"))
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func readFile[T any](t *testing.T, parts ...string) T {
	t.Helper()
	path := filepath.Join(parts...)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
