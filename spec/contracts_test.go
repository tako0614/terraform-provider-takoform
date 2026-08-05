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
	hostAPI        = "forms.takoform.com/v1alpha2"
	currentFormAPI = "forms.takoform.com/v1alpha2"
	providerFQN    = "registry.terraform.io/tako0614/takoform"
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

type legacyPackageInventory struct {
	Packages []struct {
		Kind          string              `json:"kind"`
		Path          string              `json:"path"`
		FormRef       formpackage.FormRef `json:"formRef"`
		PackageDigest string              `json:"packageDigest"`
	} `json:"packages"`
}

type currentCandidateInventory struct {
	Format             string `json:"format"`
	FormAPIVersion     string `json:"formApiVersion"`
	PackageAPIVersion  string `json:"packageApiVersion"`
	PublicationStatus  string `json:"publicationStatus"`
	LifecycleAuthority string `json:"lifecycleAuthority"`
	AuthoringSource    string `json:"authoringSource"`
	AuthoringPolicy    string `json:"authoringPolicy"`
	Forms              []struct {
		Kind          string              `json:"kind"`
		ProposalID    string              `json:"proposalId"`
		Path          string              `json:"path"`
		FormRef       formpackage.FormRef `json:"formRef"`
		PackageDigest string              `json:"packageDigest"`
	} `json:"forms"`
}

type proposalLifecycleBoundary struct {
	CurrentForms []any `json:"currentForms"`
	Proposals    []struct {
		ID string `json:"id"`
	} `json:"proposals"`
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
	legacyInventory := readFile[legacyPackageInventory](t, "..", "forms", "standard-package-set.json")
	currentInventory := readFile[currentCandidateInventory](
		t,
		"..",
		"forms",
		"candidates",
		"v1alpha2",
		"candidate-set.json",
	)
	lifecycle := readFile[proposalLifecycleBoundary](t, "..", "forms", "lifecycle.json")

	hostIdentities := map[string]string{
		"host API":            host.APIGroup,
		"host conformance":    conformance.APIVersion,
		"host runner FormRef": conformance.RunnerInput.Identity.FormRef.APIVersion,
	}
	for source, got := range hostIdentities {
		if got != hostAPI {
			t.Errorf("%s = %q, want %q", source, got, hostAPI)
		}
	}
	currentIdentities := map[string]string{
		"release API lock": release.Versioning.PortableAPI,
		"FormRef schema":   formRefSchemaAPI(t),
	}
	for source, got := range currentIdentities {
		if got != currentFormAPI {
			t.Errorf("%s = %q, want %q", source, got, currentFormAPI)
		}
	}
	if currentInventory.FormAPIVersion != formpackage.CurrentFormAPIVersion {
		t.Errorf("current candidate Form API = %q, want %q", currentInventory.FormAPIVersion, formpackage.CurrentFormAPIVersion)
	}
	if currentInventory.PackageAPIVersion != formpackage.CurrentPackageAPIVersion {
		t.Errorf("current candidate package API = %q, want %q", currentInventory.PackageAPIVersion, formpackage.CurrentPackageAPIVersion)
	}
	if currentInventory.AuthoringSource != "internal/currentformcatalog" ||
		currentInventory.AuthoringPolicy != "independent-semantic-contract" {
		t.Errorf("current candidates are not bound to the independent current catalog: source=%q policy=%q", currentInventory.AuthoringSource, currentInventory.AuthoringPolicy)
	}
	if currentInventory.Format != "takoform.current-form-candidates@v2" ||
		currentInventory.PublicationStatus != "unpublished" ||
		currentInventory.LifecycleAuthority != "forms/lifecycle.json" {
		t.Errorf("current candidate inventory claims lifecycle authority or publication: format=%q publication=%q authority=%q", currentInventory.Format, currentInventory.PublicationStatus, currentInventory.LifecycleAuthority)
	}
	if len(currentInventory.Forms) != 9 {
		t.Errorf("current candidate inventory has %d Forms, want 9", len(currentInventory.Forms))
	}
	proposalIDs := make(map[string]struct{}, len(lifecycle.Proposals))
	for _, proposal := range lifecycle.Proposals {
		proposalIDs[proposal.ID] = struct{}{}
	}
	if len(lifecycle.CurrentForms) != 0 {
		t.Errorf("source candidates must not be called Experimental while lifecycle currentForms is non-empty without an admission transition")
	}
	for _, entry := range currentInventory.Forms {
		_, hasProposal := proposalIDs[entry.ProposalID]
		if entry.Kind == "" || entry.Path == "" || !hasProposal || entry.FormRef.Kind != entry.Kind ||
			entry.FormRef.APIVersion != currentFormAPI || entry.FormRef.DefinitionVersion != "0.1.0" {
			t.Errorf("current candidate identity is not an exact v1alpha2 0.1.0 Form: %#v", entry)
		}
	}
	distributionIdentities := map[string]string{
		"release provider FQN": release.ProviderAddress,
		"trust provider FQN":   trust.Provider.Distribution.Registry,
	}
	for source, got := range distributionIdentities {
		if got != providerFQN {
			t.Errorf("%s = %q, want %q", source, got, providerFQN)
		}
	}
	if host.DiscoveryPath != conformance.DiscoveryPath {
		t.Errorf("discovery path disagrees: host=%q conformance=%q", host.DiscoveryPath, conformance.DiscoveryPath)
	}
	if release.Version != "2.0.0" || release.Tag != "v"+release.Version || release.PublicationStatus != "candidate-only" {
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
	for _, entry := range legacyInventory.Packages {
		if entry.FormRef.APIVersion != formpackage.LegacyFormAPIVersion {
			t.Errorf("%s FormRef API = %q", entry.Kind, entry.FormRef.APIVersion)
		}
	}
	for _, entry := range currentInventory.Forms {
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
		t.Error("portable-host v2 runner does not pin one exact current Form/package identity")
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

func TestLegacyIndexedStoreCapabilitySemanticsAreExact(t *testing.T) {
	t.Parallel()
	inventory := readFile[legacyPackageInventory](t, "..", "forms", "standard-package-set.json")
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
		t.Fatal("Legacy Form inventory omits IndexedStore")
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
		t.Fatalf("data.indexed@1 exceeds its retained operation/input descriptor: %#v", descriptor)
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

	for _, schemaName := range []string{"form-definition.schema.json", "form-definition-v1alpha2.schema.json"} {
		schema := readFile[struct {
			Properties struct {
				Positive struct {
					MaxItems int `json:"maxItems"`
				} `json:"conformanceFixtures"`
				Negative struct {
					MaxItems int `json:"maxItems"`
				} `json:"negativeConformanceFixtures"`
			} `json:"properties"`
		}](t, "schemas", schemaName)
		if schema.Properties.Positive.MaxItems != 32 || schema.Properties.Negative.MaxItems != 32 {
			t.Errorf(
				"%s fixture class maxima must independently remain positive=32 and negative=32: positive=%d negative=%d",
				schemaName,
				schema.Properties.Positive.MaxItems,
				schema.Properties.Negative.MaxItems,
			)
		}
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
	inventory := readFile[legacyPackageInventory](t, "..", "forms", "standard-package-set.json")
	paths = append(paths,
		filepath.Join("..", "conformance", "portable-host-v2", "contract.json"),
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
	currentRoot := filepath.Join("..", "forms", "candidates", "v1alpha2")
	if err := filepath.WalkDir(currentRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && filepath.Ext(path) == ".json" {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
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
	}](t, "schemas", "form-ref-v1alpha2.schema.json")
	return schema.Properties.APIVersion.Const
}

func readConformance(t *testing.T) portableconformance.Contract {
	t.Helper()
	contract, err := portableconformance.Verify(filepath.Join("..", "conformance", "portable-host-v2"))
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
