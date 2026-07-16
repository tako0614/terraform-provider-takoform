// Package characterization loads and verifies the frozen Phase 0/1
// compatibility-candidate evidence used by the Takoform provider and client
// tests. It deliberately does not define FormRef, Form Package, or standard
// Service Form authority.
package characterization

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	Classification = "compatibility-candidate"
	ManifestFormat = "takoform.compatibility-characterization@v1"
	APIVersion     = "forms.takoform.com/v1alpha1"
)

type KindIdentity struct {
	Kind         string
	ResourceType string
}

var ExpectedKinds = []KindIdentity{
	{Kind: "EdgeWorker", ResourceType: "takoform_edge_worker"},
	{Kind: "ObjectBucket", ResourceType: "takoform_object_bucket"},
	{Kind: "KVStore", ResourceType: "takoform_kv_store"},
	{Kind: "Queue", ResourceType: "takoform_queue"},
	{Kind: "SQLDatabase", ResourceType: "takoform_sql_database"},
	{Kind: "ContainerService", ResourceType: "takoform_container_service"},
	{Kind: "VectorIndex", ResourceType: "takoform_vector_index"},
	{Kind: "DurableWorkflow", ResourceType: "takoform_durable_workflow"},
	{Kind: "StatefulActorNamespace", ResourceType: "takoform_stateful_actor_namespace"},
	{Kind: "Schedule", ResourceType: "takoform_schedule"},
}

var CaseFiles = map[string]string{
	"providerSchema": "fixtures/provider-schema.json",
	"desired":        "fixtures/desired.json",
	"observed":       "fixtures/observed.json",
	"output":         "fixtures/output.json",
	"import":         "fixtures/import.json",
	"error":          "fixtures/error.json",
}

var SchemaFiles = map[string]string{
	"providerSchema": "schemas/provider-schema.schema.json",
	"desired":        "schemas/desired.schema.json",
	"observed":       "schemas/observed.schema.json",
	"output":         "schemas/output.schema.json",
	"import":         "schemas/import.schema.json",
	"error":          "schemas/error.schema.json",
	"discovery":      "schemas/discovery.schema.json",
}

const DiscoveryFile = "fixtures/discovery.json"

type Manifest struct {
	Format              string            `json:"format"`
	Classification      string            `json:"classification"`
	PublicationReady    bool              `json:"publicationReady"`
	PortableStandard    bool              `json:"portableStandard"`
	APIVersion          string            `json:"apiVersion"`
	DigestMethod        string            `json:"digestMethod"`
	Files               []FileDigest      `json:"files"`
	DiscoveryCaseDigest string            `json:"discoveryCaseDigest"`
	Kinds               []KindCaseDigests `json:"kinds"`
}

type FileDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type KindCaseDigests struct {
	Kind         string            `json:"kind"`
	ResourceType string            `json:"resourceType"`
	CaseDigests  map[string]string `json:"caseDigests"`
}

type CaseDocument[T any] struct {
	Format         string `json:"format"`
	Classification string `json:"classification"`
	Cases          []T    `json:"cases"`
}

type ResourceCase struct {
	Kind         string          `json:"kind"`
	ResourceType string          `json:"resourceType"`
	Resource     json.RawMessage `json:"resource"`
}

type ProviderSchemaCase struct {
	Kind         string          `json:"kind"`
	ResourceType string          `json:"resourceType"`
	Attributes   []AttributeCase `json:"attributes"`
}

type AttributeCase struct {
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Required      bool            `json:"required"`
	Optional      bool            `json:"optional"`
	Computed      bool            `json:"computed"`
	Default       *string         `json:"default,omitempty"`
	Validators    int             `json:"validators"`
	PlanModifiers int             `json:"planModifiers"`
	Attributes    []AttributeCase `json:"attributes,omitempty"`
}

type OutputCase struct {
	Kind         string      `json:"kind"`
	ResourceType string      `json:"resourceType"`
	State        OutputState `json:"state"`
}

type OutputState struct {
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	Space                  string            `json:"space"`
	SelectedImplementation string            `json:"selectedImplementation"`
	Target                 string            `json:"target"`
	Locked                 bool              `json:"locked"`
	Portability            string            `json:"portability"`
	Outputs                map[string]string `json:"outputs"`
}

type ImportCase struct {
	Kind         string         `json:"kind"`
	ResourceType string         `json:"resourceType"`
	Input        string         `json:"input"`
	Expected     ImportExpected `json:"expected"`
}

type ImportExpected struct {
	Space string `json:"space"`
	Name  string `json:"name"`
}

type ErrorCase struct {
	Kind         string                   `json:"kind"`
	ResourceType string                   `json:"resourceType"`
	Scenario     string                   `json:"scenario"`
	Expected     ProviderErrorExpected    `json:"expected"`
	API          APIErrorCharacterization `json:"api"`
}

type ProviderErrorExpected struct {
	Summary string `json:"summary"`
	Path    string `json:"path"`
}

type APIErrorCharacterization struct {
	Status    int             `json:"status"`
	Body      json.RawMessage `json:"body"`
	Code      string          `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"requestId"`
}

type DiscoveryFixture struct {
	Format         string          `json:"format"`
	Classification string          `json:"classification"`
	Host           json.RawMessage `json:"host"`
	Capabilities   json.RawMessage `json:"capabilities"`
}

type VerificationReport struct {
	KindCount int
	FileCount int
}

func LoadManifest(root string) (Manifest, error) {
	var value Manifest
	err := decodeStrict(filepath.Join(root, "manifest.json"), &value)
	return value, err
}

func LoadCases[T any](root, category string) (CaseDocument[T], error) {
	path, ok := CaseFiles[category]
	if !ok {
		return CaseDocument[T]{}, fmt.Errorf("unknown characterization category %q", category)
	}
	var value CaseDocument[T]
	err := decodeStrict(filepath.Join(root, path), &value)
	return value, err
}

func LoadDiscovery(root string) (DiscoveryFixture, error) {
	var value DiscoveryFixture
	err := decodeStrict(filepath.Join(root, DiscoveryFile), &value)
	return value, err
}

func Verify(root string) (VerificationReport, error) {
	manifest, err := LoadManifest(root)
	if err != nil {
		return VerificationReport{}, err
	}
	if manifest.Format != ManifestFormat || manifest.Classification != Classification ||
		manifest.PublicationReady || manifest.PortableStandard || manifest.APIVersion != APIVersion {
		return VerificationReport{}, errors.New("manifest must remain a non-publishable compatibility candidate")
	}
	if manifest.DigestMethod != "sha256 over repository bytes; case digest over encoding/json normalized value (characterization only)" {
		return VerificationReport{}, errors.New("unexpected characterization digest method")
	}

	expectedFiles := expectedEvidenceFiles()
	if len(manifest.Files) != len(expectedFiles) {
		return VerificationReport{}, fmt.Errorf("manifest has %d files, want %d", len(manifest.Files), len(expectedFiles))
	}
	for index, path := range expectedFiles {
		entry := manifest.Files[index]
		if entry.Path != path {
			return VerificationReport{}, fmt.Errorf("manifest file %d is %q, want %q", index, entry.Path, path)
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return VerificationReport{}, fmt.Errorf("read %s: %w", path, err)
		}
		if containsAuthorityClaim(raw) {
			return VerificationReport{}, fmt.Errorf("%s contains a forbidden authority claim", path)
		}
		if got := DigestBytes(raw); got != entry.SHA256 {
			return VerificationReport{}, fmt.Errorf("%s digest is %s, want %s", path, got, entry.SHA256)
		}
	}

	for _, path := range SchemaFiles {
		if err := verifySchema(filepath.Join(root, path)); err != nil {
			return VerificationReport{}, err
		}
	}

	schemaCases, err := LoadCases[ProviderSchemaCase](root, "providerSchema")
	if err != nil {
		return VerificationReport{}, err
	}
	desiredCases, err := LoadCases[ResourceCase](root, "desired")
	if err != nil {
		return VerificationReport{}, err
	}
	observedCases, err := LoadCases[ResourceCase](root, "observed")
	if err != nil {
		return VerificationReport{}, err
	}
	outputCases, err := LoadCases[OutputCase](root, "output")
	if err != nil {
		return VerificationReport{}, err
	}
	importCases, err := LoadCases[ImportCase](root, "import")
	if err != nil {
		return VerificationReport{}, err
	}
	errorCases, err := LoadCases[ErrorCase](root, "error")
	if err != nil {
		return VerificationReport{}, err
	}
	discovery, err := LoadDiscovery(root)
	if err != nil {
		return VerificationReport{}, err
	}

	documents := []struct {
		name           string
		format         string
		classification string
		cases          any
	}{
		{"providerSchema", schemaCases.Format, schemaCases.Classification, schemaCases.Cases},
		{"desired", desiredCases.Format, desiredCases.Classification, desiredCases.Cases},
		{"observed", observedCases.Format, observedCases.Classification, observedCases.Cases},
		{"output", outputCases.Format, outputCases.Classification, outputCases.Cases},
		{"import", importCases.Format, importCases.Classification, importCases.Cases},
		{"error", errorCases.Format, errorCases.Classification, errorCases.Cases},
	}
	for _, document := range documents {
		if document.format != "takoform.compatibility-candidate."+document.name+"@v1" || document.classification != Classification {
			return VerificationReport{}, fmt.Errorf("invalid %s fixture identity", document.name)
		}
	}
	if discovery.Format != "takoform.compatibility-candidate.discovery@v1" || discovery.Classification != Classification {
		return VerificationReport{}, errors.New("invalid discovery fixture identity")
	}

	caseDigests := map[string]map[string]string{}
	if err := collectCaseDigests(caseDigests, "providerSchema", schemaCases.Cases); err != nil {
		return VerificationReport{}, err
	}
	if err := collectCaseDigests(caseDigests, "desired", desiredCases.Cases); err != nil {
		return VerificationReport{}, err
	}
	if err := collectCaseDigests(caseDigests, "observed", observedCases.Cases); err != nil {
		return VerificationReport{}, err
	}
	if err := collectCaseDigests(caseDigests, "output", outputCases.Cases); err != nil {
		return VerificationReport{}, err
	}
	if err := collectCaseDigests(caseDigests, "import", importCases.Cases); err != nil {
		return VerificationReport{}, err
	}
	if err := collectCaseDigests(caseDigests, "error", errorCases.Cases); err != nil {
		return VerificationReport{}, err
	}

	if len(manifest.Kinds) != len(ExpectedKinds) {
		return VerificationReport{}, fmt.Errorf("manifest has %d kinds, want %d", len(manifest.Kinds), len(ExpectedKinds))
	}
	for index, identity := range ExpectedKinds {
		entry := manifest.Kinds[index]
		if entry.Kind != identity.Kind || entry.ResourceType != identity.ResourceType {
			return VerificationReport{}, fmt.Errorf("manifest kind %d is %s/%s, want %s/%s", index, entry.Kind, entry.ResourceType, identity.Kind, identity.ResourceType)
		}
		for _, category := range []string{"providerSchema", "desired", "observed", "output", "import", "error"} {
			want := caseDigests[identity.Kind][category]
			if entry.CaseDigests[category] != want {
				return VerificationReport{}, fmt.Errorf("%s %s digest is %q, want %q", identity.Kind, category, entry.CaseDigests[category], want)
			}
		}
		if len(entry.CaseDigests) != 6 {
			return VerificationReport{}, fmt.Errorf("%s has unexpected case digest categories", identity.Kind)
		}
	}

	if err := validateResourceCases(desiredCases.Cases, false); err != nil {
		return VerificationReport{}, err
	}
	if err := validateResourceCases(observedCases.Cases, true); err != nil {
		return VerificationReport{}, err
	}
	if err := validateSchemaCases(schemaCases.Cases); err != nil {
		return VerificationReport{}, err
	}
	if err := validateOutputCases(outputCases.Cases); err != nil {
		return VerificationReport{}, err
	}
	if err := validateImportCases(importCases.Cases); err != nil {
		return VerificationReport{}, err
	}
	if err := validateErrorCases(errorCases.Cases); err != nil {
		return VerificationReport{}, err
	}
	if err := validateDiscovery(discovery); err != nil {
		return VerificationReport{}, err
	}
	if got, err := DigestJSONValue(discovery); err != nil || got != manifest.DiscoveryCaseDigest {
		if err != nil {
			return VerificationReport{}, err
		}
		return VerificationReport{}, fmt.Errorf("discovery case digest is %s, want %s", got, manifest.DiscoveryCaseDigest)
	}

	return VerificationReport{KindCount: len(ExpectedKinds), FileCount: len(expectedFiles)}, nil
}

func RenderManifest(root string) (Manifest, error) {
	manifest, err := LoadManifest(root)
	if err != nil {
		return Manifest{}, err
	}
	manifest.Format = ManifestFormat
	manifest.Classification = Classification
	manifest.PublicationReady = false
	manifest.PortableStandard = false
	manifest.APIVersion = APIVersion
	manifest.DigestMethod = "sha256 over repository bytes; case digest over encoding/json normalized value (characterization only)"
	manifest.Files = nil
	for _, path := range expectedEvidenceFiles() {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return Manifest{}, err
		}
		manifest.Files = append(manifest.Files, FileDigest{Path: path, SHA256: DigestBytes(raw)})
	}

	caseDigests := map[string]map[string]string{}
	schemaCases, err := LoadCases[ProviderSchemaCase](root, "providerSchema")
	if err != nil {
		return Manifest{}, err
	}
	desiredCases, err := LoadCases[ResourceCase](root, "desired")
	if err != nil {
		return Manifest{}, err
	}
	observedCases, err := LoadCases[ResourceCase](root, "observed")
	if err != nil {
		return Manifest{}, err
	}
	outputCases, err := LoadCases[OutputCase](root, "output")
	if err != nil {
		return Manifest{}, err
	}
	importCases, err := LoadCases[ImportCase](root, "import")
	if err != nil {
		return Manifest{}, err
	}
	errorCases, err := LoadCases[ErrorCase](root, "error")
	if err != nil {
		return Manifest{}, err
	}
	if err := collectCaseDigests(caseDigests, "providerSchema", schemaCases.Cases); err != nil {
		return Manifest{}, err
	}
	if err := collectCaseDigests(caseDigests, "desired", desiredCases.Cases); err != nil {
		return Manifest{}, err
	}
	if err := collectCaseDigests(caseDigests, "observed", observedCases.Cases); err != nil {
		return Manifest{}, err
	}
	if err := collectCaseDigests(caseDigests, "output", outputCases.Cases); err != nil {
		return Manifest{}, err
	}
	if err := collectCaseDigests(caseDigests, "import", importCases.Cases); err != nil {
		return Manifest{}, err
	}
	if err := collectCaseDigests(caseDigests, "error", errorCases.Cases); err != nil {
		return Manifest{}, err
	}
	manifest.Kinds = nil
	for _, identity := range ExpectedKinds {
		manifest.Kinds = append(manifest.Kinds, KindCaseDigests{
			Kind: identity.Kind, ResourceType: identity.ResourceType, CaseDigests: caseDigests[identity.Kind],
		})
	}
	discovery, err := LoadDiscovery(root)
	if err != nil {
		return Manifest{}, err
	}
	manifest.DiscoveryCaseDigest, err = DigestJSONValue(discovery)
	return manifest, err
}

func DigestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func DigestJSONValue(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var normalized any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&normalized); err != nil {
		return "", err
	}
	raw, err = json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return DigestBytes(raw), nil
}

func EvidenceRoot(repoRoot string) string {
	return filepath.Join(repoRoot, "conformance", "compatibility-candidate-v1")
}

func FindRepoRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			if _, err := os.Stat(EvidenceRoot(current)); err == nil {
				return current, nil
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("could not find Takoform repository root")
		}
		current = parent
	}
}

func decodeStrict(path string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("decode %s: trailing JSON value", path)
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func expectedEvidenceFiles() []string {
	files := []string{DiscoveryFile}
	for _, path := range CaseFiles {
		files = append(files, path)
	}
	for _, path := range SchemaFiles {
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

func verifySchema(path string) error {
	var schema map[string]any
	if err := decodeStrict(path, &schema); err != nil {
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return fmt.Errorf("%s is not JSON Schema draft 2020-12", path)
	}
	if !strings.Contains(strings.ToLower(fmt.Sprint(schema["title"])), "compatibility candidate") {
		return fmt.Errorf("%s title must identify compatibility candidate scope", path)
	}
	return nil
}

func containsAuthorityClaim(raw []byte) bool {
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"formref", "form package", "formpackage", "schemadigest", "standard v1"} {
		if strings.Contains(lower, forbidden) {
			return true
		}
	}
	return false
}

func collectCaseDigests[T any](out map[string]map[string]string, category string, cases []T) error {
	if len(cases) != len(ExpectedKinds) {
		return fmt.Errorf("%s has %d cases, want %d", category, len(cases), len(ExpectedKinds))
	}
	for index, value := range cases {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		var identity struct {
			Kind         string `json:"kind"`
			ResourceType string `json:"resourceType"`
		}
		if err := json.Unmarshal(raw, &identity); err != nil {
			return err
		}
		want := ExpectedKinds[index]
		if identity.Kind != want.Kind || identity.ResourceType != want.ResourceType {
			return fmt.Errorf("%s case %d is %s/%s, want %s/%s", category, index, identity.Kind, identity.ResourceType, want.Kind, want.ResourceType)
		}
		digest, err := DigestJSONValue(value)
		if err != nil {
			return err
		}
		if out[identity.Kind] == nil {
			out[identity.Kind] = map[string]string{}
		}
		out[identity.Kind][category] = digest
	}
	return nil
}

func validateResourceCases(cases []ResourceCase, observed bool) error {
	for _, item := range cases {
		var resource struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Metadata   struct {
				Name      string `json:"name"`
				Space     string `json:"space"`
				ManagedBy string `json:"managedBy"`
			} `json:"metadata"`
			Spec   map[string]any `json:"spec"`
			Status *struct {
				Resolution map[string]any `json:"resolution"`
				Outputs    map[string]any `json:"outputs"`
			} `json:"status"`
		}
		if err := json.Unmarshal(item.Resource, &resource); err != nil {
			return fmt.Errorf("%s resource: %w", item.Kind, err)
		}
		if resource.APIVersion != APIVersion || resource.Kind != item.Kind || resource.Metadata.Name == "" || resource.Metadata.Space == "" || resource.Spec == nil {
			return fmt.Errorf("%s has invalid resource envelope", item.Kind)
		}
		if !observed && resource.Metadata.ManagedBy != "opentofu" {
			return fmt.Errorf("%s desired resource is not managed by opentofu", item.Kind)
		}
		if observed && (resource.Status == nil || resource.Status.Resolution == nil || resource.Status.Outputs == nil) {
			return fmt.Errorf("%s observed resource lacks sanitized status evidence", item.Kind)
		}
	}
	return nil
}

func validateSchemaCases(cases []ProviderSchemaCase) error {
	for _, item := range cases {
		last := ""
		for _, attribute := range item.Attributes {
			if attribute.Name <= last || attribute.Type == "" || (!attribute.Required && !attribute.Optional && !attribute.Computed) {
				return fmt.Errorf("%s has invalid or unsorted provider attribute %q", item.Kind, attribute.Name)
			}
			last = attribute.Name
		}
	}
	return nil
}

func validateOutputCases(cases []OutputCase) error {
	for _, item := range cases {
		state := item.State
		if state.ID == "" || state.Name == "" || state.Space == "" || state.SelectedImplementation == "" || state.Target == "" || state.Outputs == nil {
			return fmt.Errorf("%s output characterization is incomplete", item.Kind)
		}
	}
	return nil
}

func validateImportCases(cases []ImportCase) error {
	for _, item := range cases {
		space, name, ok := strings.Cut(item.Input, "/")
		if !ok || space == "" || name == "" || item.Expected.Space != space || item.Expected.Name != name {
			return fmt.Errorf("%s import characterization is invalid", item.Kind)
		}
	}
	return nil
}

func validateErrorCases(cases []ErrorCase) error {
	for _, item := range cases {
		if item.Scenario == "" || item.Expected.Summary == "" || item.Expected.Path == "" || item.API.Status < 400 || item.API.Code == "" || item.API.Message == "" || item.API.RequestID == "" || len(item.API.Body) == 0 {
			return fmt.Errorf("%s error characterization is incomplete", item.Kind)
		}
	}
	return nil
}

func validateDiscovery(value DiscoveryFixture) error {
	var host struct {
		APIVersions []string        `json:"api_versions"`
		Features    map[string]bool `json:"features"`
	}
	if err := json.Unmarshal(value.Host, &host); err != nil {
		return err
	}
	if len(host.APIVersions) != 1 || host.APIVersions[0] != APIVersion || !host.Features["service_forms"] {
		return errors.New("discovery fixture does not advertise the frozen candidate API")
	}
	var caps struct {
		APIVersion string          `json:"apiVersion"`
		Resources  map[string]bool `json:"resources"`
	}
	if err := json.Unmarshal(value.Capabilities, &caps); err != nil {
		return err
	}
	if caps.APIVersion != APIVersion || len(caps.Resources) != len(ExpectedKinds) {
		return errors.New("capabilities fixture has unexpected identity or kind count")
	}
	for _, identity := range ExpectedKinds {
		if !caps.Resources[identity.Kind] {
			return fmt.Errorf("capabilities fixture does not enable %s", identity.Kind)
		}
	}
	return nil
}
