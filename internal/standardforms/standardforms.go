package standardforms

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/formregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/portableconformance"
	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

const (
	definitionVersion = "1.0.1"
	packageVersion    = "1.0.1"
)

type Spec struct {
	Kind        string
	Slug        string
	Title       string
	Description string
	Immutable   []string
}

var Specs = []Spec{
	{Kind: "EdgeWorker", Slug: "edge-worker", Title: "Edge Worker", Description: "Provider-neutral edge application backed by a prebuilt immutable artifact.", Immutable: []string{"/name"}},
	{Kind: "ObjectBucket", Slug: "object-bucket", Title: "Object Bucket", Description: "Provider-neutral object storage with a portable default storage class.", Immutable: []string{"/name"}},
	{Kind: "KVStore", Slug: "kv-store", Title: "Key Value Store", Description: "Provider-neutral key/value state with an optional consistency preference.", Immutable: []string{"/name"}},
	{Kind: "SQLDatabase", Slug: "sql-database", Title: "SQL Database", Description: "Provider-neutral SQL storage with an open engine capability token.", Immutable: []string{"/name", "/engine"}},
	{Kind: "Queue", Slug: "queue", Title: "Queue", Description: "Provider-neutral asynchronous delivery and event fan-out.", Immutable: []string{"/name"}},
	{Kind: "VectorIndex", Slug: "vector-index", Title: "Vector Index", Description: "Provider-neutral vector index with dimensions fixed for the materialized index lifecycle.", Immutable: []string{"/name", "/dimensions"}},
	{Kind: "DurableWorkflow", Slug: "durable-workflow", Title: "Durable Workflow", Description: "Provider-neutral versioned durable workflow definition and instance-state lifecycle.", Immutable: []string{"/name"}},
	{Kind: "ContainerService", Slug: "container-service", Title: "Container Service", Description: "Provider-neutral OCI container service, separate from an edge worker.", Immutable: []string{"/name"}},
	{Kind: "StatefulActorNamespace", Slug: "stateful-actor-namespace", Title: "Stateful Actor Namespace", Description: "Provider-neutral namespace, class, and storage contract for stateful actors.", Immutable: []string{"/name"}},
	{Kind: "Schedule", Slug: "schedule", Title: "Schedule", Description: "Provider-neutral five-field cron lifecycle with one explicit invokable connection.", Immutable: []string{"/name"}},
}

var externalRequirements = []string{
	"immutable-release-tag",
	"registry-install-readback",
	"sigstore-signature-and-provenance",
	"takosumi-portable-host-lifecycle-proof",
	"terraform-provider-protocol-lifecycle-proof",
	"portable-invalid-argument-negative-lifecycle-proof",
	"signed-standard-admission-evidence",
}

type Inventory struct {
	Format              string           `json:"format"`
	Classification      string           `json:"classification"`
	DefinitionVersion   string           `json:"definitionVersion"`
	PackageVersion      string           `json:"packageVersion"`
	LocalConformance    string           `json:"localConformance"`
	PublicationReady    bool             `json:"publicationReady"`
	AdmissionStatus     string           `json:"admissionStatus"`
	ExternalRequired    []string         `json:"externalRequired"`
	ConformanceManifest string           `json:"conformanceManifest"`
	Packages            []InventoryEntry `json:"packages"`
}

type InventoryEntry struct {
	Kind            string              `json:"kind"`
	Path            string              `json:"path"`
	AdmissionStatus string              `json:"admissionStatus"`
	ConformanceCase string              `json:"conformanceCase"`
	FormRef         formpackage.FormRef `json:"formRef"`
	PackageDigest   string              `json:"packageDigest"`
}

func Generate(root string) error {
	entries := make([]InventoryEntry, 0, len(Specs))
	for _, spec := range Specs {
		entry, err := generatePackage(root, spec)
		if err != nil {
			return err
		}
		if err := syncCandidateReleaseSource(root, entry); err != nil {
			return err
		}
		entries = append(entries, entry)
	}
	sqlDatabaseV2, err := generateSQLDatabaseV2(root)
	if err != nil {
		return err
	}
	inventory := Inventory{
		Format: "takoform.standard-package-set@v1", Classification: "structural-candidate",
		DefinitionVersion: definitionVersion, PackageVersion: packageVersion, LocalConformance: "structural-only",
		PublicationReady: false, AdmissionStatus: "external-required",
		ExternalRequired:    append([]string(nil), externalRequirements...),
		ConformanceManifest: "conformance/form-package-v1/manifest.json", Packages: entries,
	}
	if err := writeJSON(filepath.Join(root, "forms", "standard-package-set.json"), inventory); err != nil {
		return err
	}
	refs := make(map[string]formregistry.Ref, len(entries))
	for _, entry := range entries {
		refs[entry.Kind] = formregistry.Ref{
			APIVersion: entry.FormRef.APIVersion, Kind: entry.FormRef.Kind,
			DefinitionVersion: entry.FormRef.DefinitionVersion,
			SchemaDigest:      entry.FormRef.SchemaDigest, PackageDigest: entry.PackageDigest,
		}
	}
	if err := writeJSON(filepath.Join(root, "internal", "formregistry", "candidate-refs.json"), refs); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(root, "conformance", "standard-form-admission-v1")); err != nil {
		return err
	}
	if err := updateConformanceManifest(root, entries, sqlDatabaseV2); err != nil {
		return err
	}
	return updatePortableHostContract(root, entries)
}

func syncCandidateReleaseSource(root string, entry InventoryEntry) error {
	source := filepath.Join(root, filepath.FromSlash(entry.Path))
	destination := filepath.Join(root, "forms", "releases", releaseIDForKind(entry.Kind), packageVersion)
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		return fmt.Errorf("sync %s candidate release source: %w", entry.Kind, err)
	}
	return nil
}

func generatePackage(root string, spec Spec) (InventoryEntry, error) {
	desiredSchema, err := desiredSchema(spec.Kind)
	if err != nil {
		return InventoryEntry{}, err
	}
	desired, err := canonicalDesired(spec.Kind)
	if err != nil {
		return InventoryEntry{}, err
	}
	name, _ := desired["name"].(string)
	definition := formpackage.FormDefinition{
		APIVersion: formpackage.FormAPIVersion, Kind: spec.Kind, DefinitionVersion: definitionVersion,
		Title: spec.Title, Description: spec.Description, Status: "standard",
		DesiredSchema: desiredSchema, ObservedSchema: observedSchema(), OutputSchema: outputSchema(spec.Kind),
		ImmutableFields:       append([]string(nil), spec.Immutable...),
		LifecycleCapabilities: []string{"create", "read", "update", "delete", "import", "observe", "refresh", "drift"},
		Interfaces:            standardInterfaceDescriptors(spec.Kind),
		ConformanceFixtures: []formpackage.ConformanceFixture{{
			Name: "canonical", DesiredPath: "fixtures/desired.json", ObservedPath: "fixtures/observed.json", OutputPath: "fixtures/output.json",
		}},
		NegativeFixtures: []formpackage.NegativeFixture{{
			Name: "reject-invalid-semantics", Stage: "desired", InputPath: "fixtures/negative.json", ExpectedFailure: "schema_validation_failed",
		}},
	}
	observed := map[string]any{
		"driftedFields": []any{}, "generation": 1, "id": spec.Kind + "/" + name,
		"imported": true, "portability": "portable", "ready": true,
	}
	output := map[string]any{
		"generation": 1, "id": spec.Kind + "/" + name, "kind": spec.Kind, "name": name,
		"portability": "portable",
	}
	if spec.Kind == "SQLDatabase" {
		output["engine"] = desired["engine"]
	}
	negative, err := negativeDesired(spec.Kind, desired)
	if err != nil {
		return InventoryEntry{}, err
	}
	packageRoot := filepath.Join(root, "conformance", "form-package-v1", "positive", "standard", spec.Slug)
	if err := os.RemoveAll(packageRoot); err != nil {
		return InventoryEntry{}, err
	}
	files := map[string]any{
		"definition.json": definition, "fixtures/desired.json": desired, "fixtures/negative.json": negative,
		"fixtures/observed.json": observed, "fixtures/output.json": output,
	}
	for relative, value := range files {
		if err := writeJSON(filepath.Join(packageRoot, filepath.FromSlash(relative)), value); err != nil {
			return InventoryEntry{}, err
		}
	}
	definitionRaw, err := os.ReadFile(filepath.Join(packageRoot, "definition.json"))
	if err != nil {
		return InventoryEntry{}, err
	}
	schemaDigest, err := formpackage.DigestCanonicalJSON(definitionRaw)
	if err != nil {
		return InventoryEntry{}, err
	}
	ref := formpackage.FormRef{APIVersion: formpackage.FormAPIVersion, Kind: spec.Kind, DefinitionVersion: definitionVersion, SchemaDigest: schemaDigest}
	paths := make([]string, 0, len(files))
	for relative := range files {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	packageFiles := make([]formpackage.PackageFile, 0, len(paths))
	for _, relative := range paths {
		raw, err := os.ReadFile(filepath.Join(packageRoot, filepath.FromSlash(relative)))
		if err != nil {
			return InventoryEntry{}, err
		}
		mediaType := "application/json"
		if relative == "definition.json" {
			mediaType = formpackage.DefinitionMediaType
		}
		packageFiles = append(packageFiles, formpackage.PackageFile{Path: relative, MediaType: mediaType, Size: int64(len(raw)), Digest: formpackage.DigestBytes(raw)})
	}
	index := formpackage.PackageIndex{APIVersion: formpackage.PackageAPIVersion, Kind: formpackage.PackageKind, PackageVersion: packageVersion, FormRef: ref, DefinitionPath: "definition.json", Files: packageFiles}
	if err := writeJSON(filepath.Join(packageRoot, formpackage.PackageIndexFilename), index); err != nil {
		return InventoryEntry{}, err
	}
	report, err := formpackage.VerifyDirectory(packageRoot)
	if err != nil {
		return InventoryEntry{}, fmt.Errorf("verify generated %s: %w", spec.Kind, err)
	}
	if err := provider.VerifyStandardFormStructure(spec.Kind, desired); err != nil {
		return InventoryEntry{}, err
	}
	return InventoryEntry{
		Kind: spec.Kind, Path: filepath.ToSlash(filepath.Join("conformance", "form-package-v1", "positive", "standard", spec.Slug)),
		AdmissionStatus: "external-required", ConformanceCase: "standard-" + spec.Slug + "-package", FormRef: report.FormRef, PackageDigest: report.PackageDigest,
	}, nil
}

func Verify(root string) error {
	var inventory Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &inventory); err != nil {
		return err
	}
	if inventory.Format != "takoform.standard-package-set@v1" || inventory.Classification != "structural-candidate" || inventory.DefinitionVersion != definitionVersion || inventory.PackageVersion != packageVersion || inventory.PublicationReady || inventory.LocalConformance != "structural-only" || inventory.AdmissionStatus != "external-required" || !reflect.DeepEqual(inventory.ExternalRequired, externalRequirements) || len(inventory.Packages) != len(Specs) {
		return fmt.Errorf("standard package inventory identity or release truth is invalid")
	}
	if _, err := os.Stat(filepath.Join(root, "conformance", "standard-form-admission-v1")); err == nil {
		return fmt.Errorf("structural-only verification must not emit passed standard-admission evidence")
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, entry := range inventory.Packages {
		if entry.AdmissionStatus != "external-required" {
			return fmt.Errorf("%s admission evidence status is not external-required", entry.Kind)
		}
		packageRoot := filepath.Join(root, filepath.FromSlash(entry.Path))
		report, err := formpackage.VerifyDirectory(packageRoot)
		if err != nil {
			return fmt.Errorf("%s package: %w", entry.Kind, err)
		}
		if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
			return fmt.Errorf("%s inventory digest drift", entry.Kind)
		}
		releaseID := releaseIDForKind(entry.Kind)
		releaseRoot := filepath.Join(root, "forms", "releases", releaseID, packageVersion)
		if err := verifyReleaseSource(packageRoot, releaseRoot, entry); err != nil {
			return fmt.Errorf("%s release source: %w", entry.Kind, err)
		}
		compiled, err := formregistry.ForKind(entry.Kind)
		if err != nil {
			return err
		}
		if compiled.APIVersion != entry.FormRef.APIVersion || compiled.Kind != entry.FormRef.Kind ||
			compiled.DefinitionVersion != entry.FormRef.DefinitionVersion || compiled.SchemaDigest != entry.FormRef.SchemaDigest ||
			compiled.PackageDigest != entry.PackageDigest {
			return fmt.Errorf("%s provider candidate ref drift", entry.Kind)
		}
		var desired map[string]any
		if err := readJSON(filepath.Join(packageRoot, "fixtures", "desired.json"), &desired); err != nil {
			return err
		}
		if err := provider.VerifyStandardFormStructure(entry.Kind, desired); err != nil {
			return err
		}
	}
	if _, err := verifySQLDatabaseV2(root); err != nil {
		return err
	}
	return VerifyMaterializableCandidate(root)
}

func releaseIDForKind(kind string) string {
	return "k-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(kind)))
}

func verifyReleaseSource(fixtureRoot, releaseRoot string, entry InventoryEntry) error {
	report, err := formpackage.VerifyDirectory(releaseRoot)
	if err != nil {
		return err
	}
	if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
		return fmt.Errorf("identity differs from the exact structural candidate")
	}
	fixtureIndexRaw, err := os.ReadFile(filepath.Join(fixtureRoot, formpackage.PackageIndexFilename))
	if err != nil {
		return err
	}
	releaseIndexRaw, err := os.ReadFile(filepath.Join(releaseRoot, formpackage.PackageIndexFilename))
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(fixtureIndexRaw, releaseIndexRaw) {
		return fmt.Errorf("package-index.json bytes differ from the reviewed fixture source")
	}
	index, err := formpackage.ValidatePackageIndex(releaseIndexRaw)
	if err != nil {
		return err
	}
	for _, file := range index.Files {
		fixtureRaw, err := os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(file.Path)))
		if err != nil {
			return err
		}
		releaseRaw, err := os.ReadFile(filepath.Join(releaseRoot, filepath.FromSlash(file.Path)))
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(fixtureRaw, releaseRaw) {
			return fmt.Errorf("%s bytes differ from the reviewed fixture source", file.Path)
		}
	}
	return nil
}

// VerifyCandidatePublication is the Phase 1 provider publication gate. It
// proves that the provider still embeds only the reviewed structural candidate
// set and that the release descriptor explicitly remains candidate-only. It
// does not read, create, or upgrade standard-admission evidence.
func VerifyCandidatePublication(root string) error {
	if err := Verify(root); err != nil {
		return err
	}
	var descriptor struct {
		SchemaVersion     int    `json:"schemaVersion"`
		Version           string `json:"version"`
		Tag               string `json:"tag"`
		ProviderAddress   string `json:"providerAddress"`
		PublicationStatus string `json:"publicationStatus"`
	}
	if err := readJSON(filepath.Join(root, "release", "version.json"), &descriptor); err != nil {
		return err
	}
	if descriptor.SchemaVersion != 1 || descriptor.Version == "" || descriptor.Tag != "v"+descriptor.Version ||
		descriptor.ProviderAddress != "registry.terraform.io/tako0614/takoform" || descriptor.PublicationStatus != "candidate-only" {
		return fmt.Errorf("Phase 1 provider publication requires the exact candidate-only release descriptor")
	}
	return nil
}

// VerifyReleaseReady is the fail-closed Phase 2 Form-admission activation
// gate. Structural candidates are verified first, then the exact retained
// standard-admission reports and distribution readbacks must close over the
// compiled set and pass offline authentication. It is deliberately not part
// of Phase 1 candidate-only provider publication.
func VerifyReleaseReady(root string) error {
	if err := Verify(root); err != nil {
		return err
	}
	candidates, err := AdmissionCandidateSet()
	if err != nil {
		return err
	}
	return admissionrelease.VerifyAdmissionSet(root, candidates)
}

// VerifyPublishedPackageSet verifies the retained, immutable distribution
// readback for the complete structural candidate set. Passing this gate proves
// package publication and its package-index publisher identity only. It does
// not upgrade any Form to portable-standard or replace release-check.
func VerifyPublishedPackageSet(root string) error {
	candidates, err := publishedPackageCandidateSet(root)
	if err != nil {
		return err
	}
	return admissionrelease.VerifyPublishedPackageSet(root, candidates)
}

// publishedPackageCandidateSet reconstructs the historical immutable set from
// its retained release sources. The active provider candidate may advance to a
// later all-or-nothing set before that new set is published, but the previous
// publication proof must remain independently verifiable throughout the
// candidate window.
func publishedPackageCandidateSet(root string) (admissionrelease.CandidateSet, error) {
	var published admissionrelease.PublishedPackageSet
	if err := readJSON(filepath.Join(root, "admission", "v1", "published-package-set.json"), &published); err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	if len(published.Entries) != len(Specs) {
		return admissionrelease.CandidateSet{}, fmt.Errorf("published package set has %d entries, want %d", len(published.Entries), len(Specs))
	}
	byKind := make(map[string]admissionrelease.PublishedPackageEntry, len(published.Entries))
	for _, entry := range published.Entries {
		if _, duplicate := byKind[entry.Kind]; duplicate {
			return admissionrelease.CandidateSet{}, fmt.Errorf("published package set duplicates %s", entry.Kind)
		}
		byKind[entry.Kind] = entry
	}
	candidates := make([]admissionrelease.Candidate, 0, len(Specs))
	for _, spec := range Specs {
		entry, ok := byKind[spec.Kind]
		if !ok || entry.Slug != spec.Slug {
			return admissionrelease.CandidateSet{}, fmt.Errorf("published package set omits exact %s/%s identity", spec.Kind, spec.Slug)
		}
		packagePath := filepath.ToSlash(filepath.Join("forms", "releases", releaseIDForKind(spec.Kind), published.PackageVersion))
		report, err := formpackage.VerifyDirectory(filepath.Join(root, filepath.FromSlash(packagePath)))
		if err != nil {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s historical published package source: %w", spec.Kind, err)
		}
		if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s historical published package source identity drift", spec.Kind)
		}
		candidates = append(candidates, admissionrelease.Candidate{
			Kind: spec.Kind, Slug: spec.Slug, PackagePath: packagePath,
			FormRef: entry.FormRef, PackageDigest: entry.PackageDigest,
		})
	}
	return admissionrelease.CandidateSet{
		DefinitionVersion: published.DefinitionVersion,
		PackageVersion:    published.PackageVersion,
		Entries:           candidates,
	}, nil
}

// AdmissionCandidateSet returns the exact all-or-nothing structural candidate
// compiled into the provider. Callers may use it to build non-publishable
// admission material, but the returned identities grant no admission status.
func AdmissionCandidateSet() (admissionrelease.CandidateSet, error) {
	candidates := make([]admissionrelease.Candidate, 0, len(Specs))
	for _, spec := range Specs {
		ref, err := formregistry.ForKind(spec.Kind)
		if err != nil {
			return admissionrelease.CandidateSet{}, err
		}
		candidates = append(candidates, admissionrelease.Candidate{
			Kind: spec.Kind, Slug: spec.Slug,
			PackagePath: filepath.ToSlash(filepath.Join("conformance", "form-package-v1", "positive", "standard", spec.Slug)),
			FormRef: formpackage.FormRef{
				APIVersion: ref.APIVersion, Kind: ref.Kind, DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
			},
			PackageDigest: ref.PackageDigest,
		})
	}
	return admissionrelease.CandidateSet{
		DefinitionVersion: definitionVersion,
		PackageVersion:    packageVersion,
		Entries:           candidates,
	}, nil
}

func desiredSchema(kind string) (map[string]any, error) {
	name := map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": ".*\\S.*"}
	properties := map[string]any{"name": name}
	required := []string{"name"}
	defs := map[string]any{}
	addConnections := func(requiredConnection bool) {
		for key, value := range connectionDefinitions() {
			defs[key] = value
		}
		properties["connections"] = map[string]any{"$ref": "#/$defs/connections"}
		if requiredConnection {
			required = append(required, "connections")
		}
	}
	addArtifact := func() {
		for key, value := range artifactDefinitions() {
			defs[key] = value
		}
		properties["source"] = map[string]any{"$ref": "#/$defs/artifactSource"}
		required = append(required, "source")
	}
	switch kind {
	case "EdgeWorker":
		addArtifact()
		addConnections(false)
		properties["compatibilityDate"] = map[string]any{"type": "string", "minLength": 1}
		properties["compatibilityFlags"] = tokenArraySchema()
		properties["profiles"] = tokenArraySchema()
	case "ObjectBucket":
		properties["storageClass"] = map[string]any{"type": "string", "enum": []string{"standard", "infrequent_access"}, "default": "standard"}
		properties["interfaces"] = tokenArraySchema()
	case "KVStore":
		properties["consistency"] = map[string]any{"type": "string", "enum": []string{"eventual", "strong"}}
	case "SQLDatabase":
		properties["engine"] = tokenSchema("sqlite")
		properties["migrationsPath"] = map[string]any{"type": "string", "minLength": 1}
	case "Queue":
		properties["delivery"] = map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"maxRetries":   map[string]any{"type": "integer", "minimum": 0},
				"maxBatchSize": map[string]any{"type": "integer", "minimum": 0},
			},
		}
	case "VectorIndex":
		addConnections(false)
		properties["dimensions"] = map[string]any{"type": "integer", "minimum": 1}
		properties["metric"] = tokenSchema("cosine")
		required = append(required, "dimensions")
	case "DurableWorkflow":
		addArtifact()
		addConnections(false)
		properties["entrypoint"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 256, "pattern": ".*\\S.*"}
		properties["retry"] = map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"maxAttempts":           map[string]any{"type": "integer", "minimum": 1},
				"initialBackoffSeconds": map[string]any{"type": "integer", "minimum": 0},
			},
		}
		required = append(required, "entrypoint")
	case "ContainerService":
		addConnections(false)
		properties["image"] = map[string]any{"type": "string", "pattern": "^[^@\\s]+@sha256:[A-Fa-f0-9]{64}$"}
		properties["ports"] = map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 1, "maximum": 65535}}
		properties["publicHttp"] = map[string]any{"type": "boolean"}
		required = append(required, "image")
	case "StatefulActorNamespace":
		addConnections(false)
		properties["className"] = map[string]any{"type": "string", "pattern": "^[A-Za-z_$][A-Za-z0-9_$]*$"}
		properties["storageProfile"] = tokenSchema("durable_sqlite")
		properties["migrationTag"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": ".*\\S.*"}
		required = append(required, "className")
	case "Schedule":
		for key, value := range scheduleConnectionDefinitions() {
			defs[key] = value
		}
		properties["cron"] = map[string]any{"type": "string", "pattern": "^[0-9*,-]+(?:/[1-9][0-9]*)? [0-9*,-]+(?:/[1-9][0-9]*)? [0-9*,-]+(?:/[1-9][0-9]*)? [0-9*,-]+(?:/[1-9][0-9]*)? [0-9*,-]+(?:/[1-9][0-9]*)?$"}
		properties["timezone"] = map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:/+-]{0,127}$", "default": "UTC"}
		properties["connections"] = map[string]any{"$ref": "#/$defs/connections"}
		required = append(required, "cron", "connections")
	default:
		return nil, fmt.Errorf("no explicit standard desired schema for %s", kind)
	}
	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
		"required": required, "properties": properties,
	}
	if len(defs) > 0 {
		schema["$defs"] = defs
	}
	return schema, nil
}

func observedSchema() map[string]any {
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
		"required": []string{"id", "ready", "generation", "imported", "portability", "driftedFields"},
		"properties": map[string]any{
			"id":          map[string]any{"type": "string", "minLength": 1},
			"ready":       map[string]any{"type": "boolean"},
			"generation":  map[string]any{"type": "integer", "minimum": 1},
			"imported":    map[string]any{"type": "boolean"},
			"portability": map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"},
			"driftedFields": map[string]any{
				"type": "array", "uniqueItems": true,
				"items": map[string]any{"type": "string", "pattern": "^(?:/(?:[^~/]|~0|~1)*)+$"},
			},
		},
	}
}

func outputSchema(kind string) map[string]any {
	required := []string{"id", "kind", "name", "generation", "portability"}
	properties := map[string]any{
		"id":          map[string]any{"type": "string", "minLength": 1},
		"kind":        map[string]any{"type": "string", "const": kind},
		"name":        map[string]any{"type": "string", "minLength": 1},
		"generation":  map[string]any{"type": "integer", "minimum": 1},
		"portability": map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"},
	}
	if kind == "SQLDatabase" {
		required = append(required, "engine")
		properties["engine"] = tokenSchema("sqlite")
	}
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
		"required": required, "properties": properties,
	}
}

// standardInterfaceDescriptors owns only portable, data-only declarations.
// Hosts still own runtime records, routing, authorization, and lifecycle. A
// Schedule consumes DurableWorkflow rather than exposing a runtime surface, so
// it intentionally has no descriptor.
func standardInterfaceDescriptors(kind string) []formpackage.InterfaceDescriptor {
	type interfaceSpec struct {
		name        string
		description string
		operations  []string
	}
	interfaces := map[string]interfaceSpec{
		"EdgeWorker":             {name: "http.request", description: "Portable HTTP request surface exposed by an edge application.", operations: []string{"request"}},
		"ObjectBucket":           {name: "object.storage", description: "Portable object storage operations.", operations: []string{"delete", "get", "list", "put"}},
		"KVStore":                {name: "keyvalue.store", description: "Portable key/value operations.", operations: []string{"delete", "get", "list", "put"}},
		"SQLDatabase":            {name: "sql.query", description: "Portable SQL query and transaction operations.", operations: []string{"execute", "query", "transaction"}},
		"Queue":                  {name: "queue.messages", description: "Portable queue delivery operations.", operations: []string{"acknowledge", "receive", "send"}},
		"VectorIndex":            {name: "vector.query", description: "Portable vector index operations.", operations: []string{"delete", "query", "upsert"}},
		"DurableWorkflow":        {name: "workflow.invoke", description: "Portable durable workflow invocation operations.", operations: []string{"cancel", "invoke", "status"}},
		"ContainerService":       {name: "http.request", description: "Portable HTTP request surface exposed by a container service.", operations: []string{"request"}},
		"StatefulActorNamespace": {name: "actor.invoke", description: "Portable stateful actor invocation operations.", operations: []string{"invoke"}},
	}
	spec, ok := interfaces[kind]
	if !ok {
		return nil
	}
	operationValues := make([]any, 0, len(spec.operations))
	for _, operation := range spec.operations {
		operationValues = append(operationValues, operation)
	}
	descriptor := formpackage.InterfaceDescriptor{
		Name: spec.name, Version: "1", Description: spec.description, Required: true,
		Document: map[string]any{"operations": operationValues},
		DocumentSchema: map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
			"required": []string{"operations"},
			"properties": map[string]any{
				"operations": map[string]any{
					"type": "array", "minItems": len(operationValues), "maxItems": len(operationValues), "uniqueItems": true,
					"items": map[string]any{"type": "string", "enum": operationValues},
				},
			},
		},
		Inputs: []formpackage.InterfaceInputDeclaration{
			{Name: "resource", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/id"},
			{Name: "name", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/name"},
		},
	}
	if kind == "SQLDatabase" {
		descriptor.Inputs = append(descriptor.Inputs, formpackage.InterfaceInputDeclaration{
			Name: "engine", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/engine",
		})
	}
	return []formpackage.InterfaceDescriptor{descriptor}
}

func canonicalDesired(kind string) (map[string]any, error) {
	connections := func(name, resource string, permissions []any, projection string) map[string]any {
		return map[string]any{name: map[string]any{"resource": resource, "permissions": permissions, "projection": projection}}
	}
	switch kind {
	case "EdgeWorker":
		return map[string]any{
			"name": "edge", "source": map[string]any{
				"artifactUrl":    "https://github.com/tako0614/takosumi/releases/download/standard-form-runtime-v1.0.3/edge-worker.mjs",
				"artifactSha256": "281b77f65f6258e56d0468a580b1f67baf9f4d71891c2f7259ce24c47bf7d67e",
			},
			"compatibilityDate": "2026-07-20", "compatibilityFlags": []any{"nodejs_compat"}, "profiles": []any{"workers"},
			"connections": connections("ASSETS", "ObjectBucket/edge-assets", []any{"read", "write"}, "object.binding.v1"),
		}, nil
	case "ObjectBucket":
		return map[string]any{"name": "assets", "storageClass": "standard", "interfaces": []any{"s3_api"}}, nil
	case "KVStore":
		return map[string]any{"name": "cache", "consistency": "eventual"}, nil
	case "SQLDatabase":
		return map[string]any{"name": "main", "engine": "sqlite"}, nil
	case "Queue":
		return map[string]any{"name": "jobs"}, nil
	case "VectorIndex":
		return map[string]any{"name": "embeddings", "dimensions": 1536, "metric": "cosine"}, nil
	case "DurableWorkflow":
		return map[string]any{
			"name": "ingest", "source": map[string]any{
				"artifactRef":    "standard-form-runtime/v1.0.3/durable-workflow.mjs",
				"artifactSha256": "8712e09089276b497669472eddc0aa425c6fa2bf766037f7351690a3517d5ac5",
			},
			"entrypoint": "IngestWorkflow", "retry": map[string]any{"maxAttempts": 3, "initialBackoffSeconds": 5},
		}, nil
	case "ContainerService":
		return map[string]any{
			"name": "agent", "image": "docker.io/library/nginx@sha256:845b5424415de5f77dd5753cbb7c1be8bd8e44cc81f20f9705783a02f8848317", "ports": []any{80}, "publicHttp": true,
		}, nil
	case "StatefulActorNamespace":
		return map[string]any{
			"name": "rooms", "className": "RoomActor", "storageProfile": "durable_sqlite", "migrationTag": "v1",
		}, nil
	case "Schedule":
		return map[string]any{
			"name": "nightly", "cron": "0 0 * * *", "timezone": "UTC",
			"connections": connections("workflow", "DurableWorkflow/ingest", []any{"invoke"}, "schedule_trigger"),
		}, nil
	default:
		return nil, fmt.Errorf("no canonical standard desired fixture for %s", kind)
	}
}

func negativeDesired(kind string, canonical map[string]any) (map[string]any, error) {
	negative := cloneJSONMap(canonical)
	switch kind {
	case "EdgeWorker":
		negative["source"] = map[string]any{"artifactUrl": "https://artifacts.example.test/edge.js"}
	case "ObjectBucket":
		negative["storageClass"] = "cold"
	case "KVStore":
		negative["consistency"] = "linearizable"
	case "SQLDatabase":
		negative["engine"] = "not a token"
	case "Queue":
		negative["delivery"] = map[string]any{"maxRetries": -1}
	case "VectorIndex":
		negative["dimensions"] = 0
	case "DurableWorkflow":
		negative["retry"] = map[string]any{"maxAttempts": 0}
	case "ContainerService":
		negative["ports"] = []any{0}
	case "StatefulActorNamespace":
		negative["className"] = "not a class"
	case "Schedule":
		negative["cron"] = "0 0 * *"
	default:
		return nil, fmt.Errorf("no negative standard fixture for %s", kind)
	}
	return negative, nil
}

func connectionDefinitions() map[string]any {
	return map[string]any{
		"connections": map[string]any{
			"type": "object", "propertyNames": portableMapKeys(),
			"additionalProperties": map[string]any{"$ref": "#/$defs/connection"},
		},
		"connection": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"resource", "permissions", "projection"},
			"properties": map[string]any{
				"resource":    map[string]any{"type": "string", "pattern": "^\\S+$"},
				"permissions": map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"}},
				"projection":  map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"},
			},
		},
	}
}

func scheduleConnectionDefinitions() map[string]any {
	return map[string]any{
		"connections": map[string]any{
			"type": "object", "minProperties": 1, "maxProperties": 1, "propertyNames": portableMapKeys(),
			"additionalProperties": map[string]any{"$ref": "#/$defs/scheduleConnection"},
		},
		"scheduleConnection": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"resource", "permissions", "projection"},
			"properties": map[string]any{
				"resource": map[string]any{"type": "string", "pattern": "^\\S+$"},
				"permissions": map[string]any{
					"type": "array", "minItems": 1, "uniqueItems": true,
					"contains": map[string]any{"const": "invoke", "type": "string"},
					"items":    map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"},
				},
				"projection": map[string]any{"const": "schedule_trigger", "type": "string"},
			},
		},
	}
}

func artifactDefinitions() map[string]any {
	return map[string]any{
		"artifactSource": map[string]any{"oneOf": []any{
			map[string]any{"type": "object", "additionalProperties": false, "required": []string{"artifactPath"}, "properties": map[string]any{"artifactPath": map[string]any{"type": "string", "minLength": 1}}},
			map[string]any{"type": "object", "additionalProperties": false, "required": []string{"artifactUrl", "artifactSha256"}, "properties": map[string]any{"artifactUrl": map[string]any{"type": "string", "format": "uri", "pattern": "^https://"}, "artifactSha256": map[string]any{"$ref": "#/$defs/sha256"}}},
			map[string]any{"type": "object", "additionalProperties": false, "required": []string{"artifactRef", "artifactSha256"}, "properties": map[string]any{"artifactRef": map[string]any{"type": "string", "minLength": 1}, "artifactSha256": map[string]any{"$ref": "#/$defs/sha256"}}},
		}},
		"sha256": map[string]any{"type": "string", "pattern": "^(sha256:)?[A-Fa-f0-9]{64}$"},
	}
}

func tokenArraySchema() map[string]any {
	return map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": "^\\S+$"}}
}

func tokenSchema(defaultValue string) map[string]any {
	return map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$", "default": defaultValue}
}

func portableMapKeys() map[string]any {
	return map[string]any{
		"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._-]{0,63}$",
		"x-takoform-fieldPolicy": "portable-data-only-v1",
	}
}

func cloneJSONMap(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	return result
}

func updateConformanceManifest(root string, entries []InventoryEntry, successors ...InventoryEntry) error {
	path := filepath.Join(root, "conformance", "form-package-v1", "manifest.json")
	var manifest struct {
		SchemaVersion int `json:"schemaVersion"`
		Positive      []struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"positive"`
		Negative []map[string]any `json:"negative"`
	}
	if err := readJSON(path, &manifest); err != nil {
		return err
	}
	kept := manifest.Positive[:0]
	for _, item := range manifest.Positive {
		if !strings.HasPrefix(item.Name, "standard-") {
			kept = append(kept, item)
		}
	}
	manifest.Positive = kept
	for _, entry := range entries {
		manifest.Positive = append(manifest.Positive, struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Kind string `json:"kind"`
		}{Name: entry.ConformanceCase, Path: strings.TrimPrefix(entry.Path, "conformance/form-package-v1/"), Kind: entry.Kind})
	}
	for _, entry := range successors {
		manifest.Positive = append(manifest.Positive, struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Kind string `json:"kind"`
		}{Name: entry.ConformanceCase, Path: strings.TrimPrefix(entry.Path, "conformance/form-package-v1/"), Kind: entry.Kind})
	}
	return writeJSON(path, manifest)
}

func updatePortableHostContract(root string, entries []InventoryEntry) error {
	var bucket *InventoryEntry
	for index := range entries {
		if entries[index].Kind == "ObjectBucket" {
			bucket = &entries[index]
			break
		}
	}
	if bucket == nil {
		return fmt.Errorf("standard ObjectBucket missing")
	}
	contractPath := filepath.Join(root, "conformance", "portable-host-v1", "contract.json")
	var contract portableconformance.Contract
	if err := readJSON(contractPath, &contract); err != nil {
		return err
	}
	contract.RunnerInput.Identity = portableconformance.InstalledFormReference{
		FormRef: portableconformance.FormRef{
			APIVersion: bucket.FormRef.APIVersion, Kind: bucket.FormRef.Kind,
			DefinitionVersion: bucket.FormRef.DefinitionVersion, SchemaDigest: bucket.FormRef.SchemaDigest,
		},
		PackageDigest: bucket.PackageDigest,
	}
	runnerDigest, err := portableconformance.RunnerEvidenceDigest(contract)
	if err != nil {
		return err
	}
	contract.RunnerEvidence.SHA256 = runnerDigest
	if err := writeJSON(contractPath, contract); err != nil {
		return err
	}
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	manifest := struct {
		Format   string `json:"format"`
		Contract string `json:"contract"`
		SHA256   string `json:"sha256"`
	}{Format: "takoform.portable-host-conformance-manifest@v1", Contract: "contract.json", SHA256: hex.EncodeToString(digest[:])}
	return writeJSON(filepath.Join(root, "conformance", "portable-host-v1", "manifest.json"), manifest)
}

func readJSON(path string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
