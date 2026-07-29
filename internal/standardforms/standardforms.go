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
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/formregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/portableconformance"
	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

type Spec struct {
	Kind        string
	Slug        string
	Title       string
	Description string
	Version     string
	Immutable   []string
}

// Specs is the active portable Form set, derived from the one declaration in
// internal/formcatalog. Nothing here restates a Form: adding a kind to the
// catalogue adds it to the packages, the inventory, and the provider at once.
var Specs = activeSpecs()

func activeSpecs() []Spec {
	specs := make([]Spec, 0, len(formcatalog.Kinds))
	for _, kind := range formcatalog.Kinds {
		specs = append(specs, Spec{
			Kind: kind.Kind, Slug: kind.Slug, Title: kind.Title, Description: kind.Description,
			Version: kind.Version(), Immutable: kind.ImmutableFields(),
		})
	}
	return specs
}

// retiredPackageVersion is the definition and package version of the Forms
// this project published before the portable Form set was rebuilt.
const retiredPackageVersion = "1.0.1"

// portableGeneration names the rebuilt, intent-shaped Form set. It is not a
// package version: each Form carries its own SemVer so a retained kind token
// can start a new major line without renumbering every other Form.
const portableGeneration = "portable-v1"

// RetiredKinds is the exact published set whose immutable bytes and admission
// evidence remain verifiable. Those releases are never rewritten, re-signed,
// or reshaped; they are simply no longer the set this provider implements.
var RetiredKinds = []Spec{
	{Kind: "EdgeWorker", Slug: "edge-worker", Version: retiredPackageVersion},
	{Kind: "ObjectBucket", Slug: "object-bucket", Version: retiredPackageVersion},
	{Kind: "KVStore", Slug: "kv-store", Version: retiredPackageVersion},
	{Kind: "SQLDatabase", Slug: "sql-database", Version: retiredPackageVersion},
	{Kind: "Queue", Slug: "queue", Version: retiredPackageVersion},
	{Kind: "VectorIndex", Slug: "vector-index", Version: retiredPackageVersion},
	{Kind: "DurableWorkflow", Slug: "durable-workflow", Version: retiredPackageVersion},
	{Kind: "ContainerService", Slug: "container-service", Version: retiredPackageVersion},
	{Kind: "StatefulActorNamespace", Slug: "stateful-actor-namespace", Version: retiredPackageVersion},
	{Kind: "Schedule", Slug: "schedule", Version: retiredPackageVersion},
}

var externalRequirements = []string{
	"immutable-release-tag",
	"registry-install-readback",
	"sigstore-signature-and-provenance",
	"conforming-host-lifecycle-proof",
	"terraform-provider-protocol-lifecycle-proof",
	"portable-invalid-argument-negative-lifecycle-proof",
	"signed-standard-admission-evidence",
}

type Inventory struct {
	Format              string           `json:"format"`
	Classification      string           `json:"classification"`
	Generation          string           `json:"generation"`
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
	// A retired Form leaves nothing behind: its package directory goes with it,
	// so the corpus can never advertise a kind the provider no longer implements.
	if err := pruneRetiredPackages(root); err != nil {
		return err
	}
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
	inventory := Inventory{
		Format: "takoform.standard-package-set@v1", Classification: "structural-candidate",
		Generation: portableGeneration, LocalConformance: "structural-only",
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
	if err := updateConformanceManifest(root, entries); err != nil {
		return err
	}
	if err := updatePortableHostContract(root, entries); err != nil {
		return err
	}
	if err := generateRetiredInventory(root); err != nil {
		return err
	}
	if err := generateReleasePlan(root, entries); err != nil {
		return err
	}
	return generatePublishedSurfaces(root)
}

func syncCandidateReleaseSource(root string, entry InventoryEntry) error {
	source := filepath.Join(root, filepath.FromSlash(entry.Path))
	destination := filepath.Join(root, "forms", "releases", releaseIDForKind(entry.Kind), entry.FormRef.DefinitionVersion)
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		return fmt.Errorf("sync %s candidate release source: %w", entry.Kind, err)
	}
	return nil
}

func generatePackage(root string, spec Spec) (InventoryEntry, error) {
	kind, ok := formcatalog.ByKind(spec.Kind)
	if !ok {
		return InventoryEntry{}, fmt.Errorf("no declared Form for %s", spec.Kind)
	}
	desired := kind.CanonicalDesired()
	negatives, err := kind.NegativeCases()
	if err != nil {
		return InventoryEntry{}, err
	}
	negativeFixtures := make([]formpackage.NegativeFixture, 0, len(negatives))
	negativeFiles := make(map[string]any, len(negatives))
	for _, negative := range negatives {
		path := "fixtures/negative-" + negative.Field.HCL + ".json"
		negativeFiles[path] = negative.Desired
		negativeFixtures = append(negativeFixtures, formpackage.NegativeFixture{
			Name: "reject-" + negative.Field.HCL, Stage: "desired",
			InputPath: path, ExpectedFailure: "schema_validation_failed",
		})
	}
	definition := formpackage.FormDefinition{
		APIVersion: formpackage.FormAPIVersion, Kind: spec.Kind, DefinitionVersion: spec.Version,
		Title: spec.Title, Description: spec.Description, Status: "standard",
		DesiredSchema: kind.DesiredSchema(), ObservedSchema: formcatalog.ObservedSchema(), OutputSchema: kind.OutputSchema(),
		ImmutableFields:       append([]string(nil), spec.Immutable...),
		LifecycleCapabilities: []string{"create", "read", "update", "delete", "import", "observe", "refresh", "drift"},
		Interfaces:            kind.InterfaceDescriptors(),
		ConformanceFixtures: []formpackage.ConformanceFixture{{
			Name: "canonical", DesiredPath: "fixtures/desired.json", ObservedPath: "fixtures/observed.json", OutputPath: "fixtures/output.json",
		}},
		NegativeFixtures: negativeFixtures,
	}
	packageRoot := filepath.Join(root, "conformance", "form-package-v1", "positive", "standard", spec.Slug)
	if err := os.RemoveAll(packageRoot); err != nil {
		return InventoryEntry{}, err
	}
	files := map[string]any{
		"definition.json": definition, "fixtures/desired.json": desired,
		"fixtures/observed.json": kind.CanonicalObserved(), "fixtures/output.json": kind.CanonicalOutput(),
	}
	for path, value := range negativeFiles {
		files[path] = value
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
	ref := formpackage.FormRef{APIVersion: formpackage.FormAPIVersion, Kind: spec.Kind, DefinitionVersion: spec.Version, SchemaDigest: schemaDigest}
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
	index := formpackage.PackageIndex{APIVersion: formpackage.PackageAPIVersion, Kind: formpackage.PackageKind, PackageVersion: spec.Version, FormRef: ref, DefinitionPath: "definition.json", Files: packageFiles}
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
	if inventory.Format != "takoform.standard-package-set@v1" || inventory.Classification != "structural-candidate" || inventory.Generation != portableGeneration || inventory.PublicationReady || inventory.LocalConformance != "structural-only" || inventory.AdmissionStatus != "external-required" || !reflect.DeepEqual(inventory.ExternalRequired, externalRequirements) || len(inventory.Packages) != len(Specs) {
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
		releaseRoot := filepath.Join(root, "forms", "releases", releaseID, entry.FormRef.DefinitionVersion)
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
	if err := VerifyReleasePlan(root); err != nil {
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

// VerifyAdmissionClosure is the fail-closed Phase 2 candidate-closure gate.
// Structural candidates are verified first, then the exact retained
// standard-admission reports and distribution readbacks must close over the
// compiled set and pass offline authentication. Passing this check does not
// activate admission: VerifyReleaseReady separately requires the matching
// controller-authorized immutable GitHub Release and its live readback.
func VerifyAdmissionClosure(root string) error {
	if err := Verify(root); err != nil {
		return err
	}
	candidates, err := AdmissionCandidateSet(root)
	if err != nil {
		return err
	}
	return admissionrelease.VerifyAdmissionSet(root, candidates)
}

// VerifyCurrentAdmissionClosure verifies only the current Takoform FormRef
// lane. It does not describe or gate unrelated Cloud services that have no
// Takoform definition.
func VerifyCurrentAdmissionClosure(root string) error {
	if err := Verify(root); err != nil {
		return err
	}
	candidates, err := CurrentAdmissionCandidateSet(root)
	if err != nil {
		return err
	}
	return admissionrelease.VerifyAdmissionSetAt(root, "admission/v3", candidates)
}

// VerifyPublishedPackageSet verifies the retained, immutable distribution
// readback for the complete structural candidate set. Passing this gate proves
// package publication and its package-index publisher identity only. It does
// not upgrade any Form to portable-standard.
func VerifyPublishedPackageSet(root string) error {
	candidates, err := publishedPackageCandidateSet(root)
	if err != nil {
		return err
	}
	return admissionrelease.VerifyPublishedPackageSet(root, candidates)
}

// VerifyCurrentPublishedPackageSet verifies the immutable per-Form releases
// selected by the current mixed-version admission generation.
func VerifyCurrentPublishedPackageSet(root string) error {
	candidates, err := CurrentAdmissionCandidateSet(root)
	if err != nil {
		return err
	}
	return admissionrelease.VerifyPublishedPackageSetAt(root, "admission/v3", candidates)
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
	if len(published.Entries) != len(RetiredKinds) {
		return admissionrelease.CandidateSet{}, fmt.Errorf("published package set has %d entries, want %d", len(published.Entries), len(RetiredKinds))
	}
	byKind := make(map[string]admissionrelease.PublishedPackageEntry, len(published.Entries))
	for _, entry := range published.Entries {
		if _, duplicate := byKind[entry.Kind]; duplicate {
			return admissionrelease.CandidateSet{}, fmt.Errorf("published package set duplicates %s", entry.Kind)
		}
		byKind[entry.Kind] = entry
	}
	candidates := make([]admissionrelease.Candidate, 0, len(RetiredKinds))
	for _, spec := range RetiredKinds {
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

// AdmissionCandidateSet returns the exact retired published set whose
// admission evidence this repository retains.
//
// It is deliberately not the active candidate set: only a subset of the
// rebuilt portable Forms has published packages and none has complete
// admission evidence, so binding this retained lane to them would manufacture
// a claim that does not exist.
func AdmissionCandidateSet(root string) (admissionrelease.CandidateSet, error) {
	candidates := make([]admissionrelease.Candidate, 0, len(RetiredKinds))
	for _, spec := range RetiredKinds {
		packagePath := filepath.ToSlash(filepath.Join("forms", "releases", releaseIDForKind(spec.Kind), spec.Version))
		report, err := formpackage.VerifyDirectory(filepath.Join(root, filepath.FromSlash(packagePath)))
		if err != nil {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s retired release source: %w", spec.Kind, err)
		}
		candidates = append(candidates, admissionrelease.Candidate{
			Kind: spec.Kind, Slug: spec.Slug, PackagePath: packagePath,
			FormRef: report.FormRef, PackageDigest: report.PackageDigest,
		})
	}
	return admissionrelease.CandidateSet{
		DefinitionVersion: retiredPackageVersion,
		PackageVersion:    retiredPackageVersion,
		Entries:           candidates,
	}, nil
}

type currentAdmissionCandidateInventory struct {
	Format          string                             `json:"format"`
	Generation      string                             `json:"generation"`
	AdmissionStatus string                             `json:"admissionStatus"`
	Packages        []currentAdmissionCandidatePackage `json:"packages"`
}

type currentAdmissionCandidatePackage struct {
	Kind          string              `json:"kind"`
	Slug          string              `json:"slug"`
	ReleaseID     string              `json:"releaseId"`
	Version       string              `json:"version"`
	Tag           string              `json:"tag"`
	SourcePath    string              `json:"sourcePath"`
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}

// CurrentPortableCandidateSet returns the complete current portable-v1
// catalog as exact local release-source identities. Provider conformance must
// close over this full set before a smaller admission generation selects any
// subset from it.
func CurrentPortableCandidateSet(root string) (admissionrelease.CandidateSet, error) {
	if err := Verify(root); err != nil {
		return admissionrelease.CandidateSet{}, fmt.Errorf("verify current portable catalog: %w", err)
	}
	var inventory Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &inventory); err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	if inventory.Format != "takoform.standard-package-set@v1" ||
		inventory.Generation != "portable-v1" ||
		len(inventory.Packages) != len(Specs) || len(inventory.Packages) != 34 {
		return admissionrelease.CandidateSet{}, fmt.Errorf("current portable catalog identity is invalid")
	}
	specByKind := make(map[string]Spec, len(Specs))
	for _, spec := range Specs {
		specByKind[spec.Kind] = spec
	}
	seen := make(map[string]struct{}, len(inventory.Packages))
	candidates := make([]admissionrelease.Candidate, 0, len(inventory.Packages))
	for index, entry := range inventory.Packages {
		spec, ok := specByKind[entry.Kind]
		if !ok || filepath.Base(entry.Path) != spec.Slug ||
			entry.AdmissionStatus != "external-required" ||
			entry.FormRef.Kind != entry.Kind ||
			!formpackage.ValidDigest(entry.FormRef.SchemaDigest) ||
			!formpackage.ValidDigest(entry.PackageDigest) {
			return admissionrelease.CandidateSet{}, fmt.Errorf("current portable packages[%d] has invalid exact identity", index)
		}
		if _, duplicate := seen[entry.Kind]; duplicate {
			return admissionrelease.CandidateSet{}, fmt.Errorf("current portable packages[%d] duplicates kind %s", index, entry.Kind)
		}
		seen[entry.Kind] = struct{}{}
		packagePath := filepath.ToSlash(filepath.Join("forms", "releases", releaseIDForKind(entry.Kind), entry.FormRef.DefinitionVersion))
		report, err := formpackage.VerifyDirectory(filepath.Join(root, filepath.FromSlash(packagePath)))
		if err != nil {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s current portable release source: %w", entry.Kind, err)
		}
		if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s current portable release-source identity drift", entry.Kind)
		}
		candidates = append(candidates, admissionrelease.Candidate{
			Kind: entry.Kind, Slug: spec.Slug, PackagePath: packagePath,
			FormRef: entry.FormRef, PackageDigest: entry.PackageDigest,
		})
	}
	return admissionrelease.CandidateSet{Generation: inventory.Generation, Entries: candidates}, nil
}

// CurrentAdmissionCandidateSet returns the exact mixed-version package
// identities selected for the generation-aware v3 admission lane. Publication
// and admission evidence remain separate: this function verifies only the
// reviewed inventory and its local data-only package bytes.
func CurrentAdmissionCandidateSet(root string) (admissionrelease.CandidateSet, error) {
	var inventory currentAdmissionCandidateInventory
	if err := readJSON(filepath.Join(root, "forms", "admission-candidate-set.json"), &inventory); err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	var standard Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &standard); err != nil {
		return admissionrelease.CandidateSet{}, fmt.Errorf("read current standard package set: %w", err)
	}
	if inventory.Format != "takoform.admission-candidate-set@v1" ||
		inventory.Generation != "ga-core-v1" ||
		inventory.AdmissionStatus != "external-required" ||
		len(inventory.Packages) != 10 {
		return admissionrelease.CandidateSet{}, fmt.Errorf("current admission inventory identity is invalid")
	}
	if standard.Format != "takoform.standard-package-set@v1" ||
		standard.Generation != "portable-v1" ||
		len(standard.Packages) != 34 {
		return admissionrelease.CandidateSet{}, fmt.Errorf("current standard package set identity is invalid")
	}
	standardByKind := make(map[string]InventoryEntry, len(standard.Packages))
	for index, item := range standard.Packages {
		if _, duplicate := standardByKind[item.Kind]; duplicate {
			return admissionrelease.CandidateSet{}, fmt.Errorf("current standard packages[%d] duplicates kind %s", index, item.Kind)
		}
		standardByKind[item.Kind] = item
	}
	seenKinds := make(map[string]struct{}, len(inventory.Packages))
	seenSlugs := make(map[string]struct{}, len(inventory.Packages))
	candidates := make([]admissionrelease.Candidate, 0, len(inventory.Packages))
	for index, item := range inventory.Packages {
		if _, duplicate := seenKinds[item.Kind]; duplicate {
			return admissionrelease.CandidateSet{}, fmt.Errorf("current admission packages[%d] duplicates kind %s", index, item.Kind)
		}
		seenKinds[item.Kind] = struct{}{}
		if _, duplicate := seenSlugs[item.Slug]; duplicate {
			return admissionrelease.CandidateSet{}, fmt.Errorf("current admission packages[%d] duplicates slug %s", index, item.Slug)
		}
		seenSlugs[item.Slug] = struct{}{}
		releaseID := releaseIDForKind(item.Kind)
		wantTag := "forms/" + releaseID + "/v" + item.Version
		wantPath := filepath.ToSlash(filepath.Join("forms", "releases", releaseID, item.Version))
		if item.Kind == "" || item.Slug == "" || item.ReleaseID != releaseID ||
			item.Version == "" || item.Version != item.FormRef.DefinitionVersion ||
			item.Tag != wantTag || item.SourcePath != wantPath ||
			item.FormRef.APIVersion != formpackage.FormAPIVersion ||
			item.FormRef.Kind != item.Kind || !formpackage.ValidDigest(item.FormRef.SchemaDigest) ||
			!formpackage.ValidDigest(item.PackageDigest) {
			return admissionrelease.CandidateSet{}, fmt.Errorf("current admission packages[%d] has invalid exact identity", index)
		}
		standardItem, ok := standardByKind[item.Kind]
		if !ok || filepath.Base(standardItem.Path) != item.Slug ||
			standardItem.FormRef != item.FormRef ||
			standardItem.PackageDigest != item.PackageDigest {
			return admissionrelease.CandidateSet{}, fmt.Errorf("current admission packages[%d] is not an exact member of portable-v1", index)
		}
		report, err := formpackage.VerifyDirectory(filepath.Join(root, filepath.FromSlash(item.SourcePath)))
		if err != nil {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s current admission package: %w", item.Kind, err)
		}
		if report.FormRef != item.FormRef || report.PackageDigest != item.PackageDigest {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s current admission package bytes drift from inventory", item.Kind)
		}
		candidates = append(candidates, admissionrelease.Candidate{
			Kind: item.Kind, Slug: item.Slug, PackagePath: item.SourcePath,
			FormRef: item.FormRef, PackageDigest: item.PackageDigest,
		})
	}
	return admissionrelease.CandidateSet{Generation: inventory.Generation, Entries: candidates}, nil
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

func pruneRetiredPackages(root string) error {
	standardRoot := filepath.Join(root, "conformance", "form-package-v1", "positive", "standard")
	entries, err := os.ReadDir(standardRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	declared := make(map[string]struct{}, len(Specs))
	for _, spec := range Specs {
		declared[spec.Slug] = struct{}{}
	}
	for _, entry := range entries {
		if _, keep := declared[entry.Name()]; keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(standardRoot, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
