package standardforms

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/formpublication"
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

// Specs is the retained provider-v1 Legacy compatibility set derived from
// internal/formcatalog. It is not the project's current Form lifecycle
// inventory; forms/lifecycle.json currently declares nine Proposals and no
// Experimental-or-later current Forms.
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

const (
	legacyGaCoreV2Root   = "admission/v4"
	retainedGaCoreV1Root = "admission/v3"
)

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
	return fmt.Errorf(
		"legacy catalog generation is retired; forms/lifecycle.json declares Proposal-derived current candidates separately, so verify retained compatibility surfaces instead",
	)
}

func Verify(root string) error {
	authority, err := readProjectLifecycleAuthority(root)
	if err != nil {
		return err
	}
	published, err := discoverPublishedReleaseSources(root)
	if err != nil {
		return fmt.Errorf("published release sources: %w", err)
	}
	if err := verifyLegacyReleaseInventory(root, authority, published); err != nil {
		return err
	}
	if err := VerifyFormSemVerHistory(root); err != nil {
		return fmt.Errorf("Form SemVer history: %w", err)
	}
	if err := VerifyPublishedSurfaces(root); err != nil {
		return fmt.Errorf("catalog-derived public surfaces: %w", err)
	}
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
		kind, ok := formcatalog.ByKind(entry.Kind)
		if !ok {
			return fmt.Errorf("%s inventory kind is not declared", entry.Kind)
		}
		if err := verifyGeneratedFixtureContract(packageRoot, kind); err != nil {
			return fmt.Errorf("%s generated fixture contract: %w", entry.Kind, err)
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
	if err := verifyPortableHostContract(root, inventory.Packages); err != nil {
		return err
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
		return fmt.Errorf("identity differs from the exact Legacy compatibility identity")
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

// VerifyCandidatePublication is the provider compatibility publication gate.
// It proves that the provider still embeds only the reviewed Legacy set and
// that the immutable release descriptor retains its candidate-only metadata.
// It cannot change Form lifecycle, Host Support, or activation.
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
		return fmt.Errorf("provider publication requires the exact retained candidate-only release descriptor")
	}
	return nil
}

// VerifyLegacyAdmissionEvidence authenticates retained Git identities and
// exact set bytes without interpreting any historical admission field as
// current Form maturity. Historical semantic claims are not rerun under a
// different current provider/catalog policy.
func VerifyLegacyAdmissionEvidence(root string) error {
	if err := Verify(root); err != nil {
		return err
	}
	if err := verifyHistoricalAdmissionAssignments(root); err != nil {
		return fmt.Errorf("legacy admission identity history: %w", err)
	}
	if err := VerifyRetainedGaCoreV1PublishedPackageSet(root); err != nil {
		return fmt.Errorf("legacy admission/v3 publication checkpoint: %w", err)
	}
	return nil
}

// VerifyPublishedPackageSet verifies the retained, immutable distribution
// readback for the complete Legacy compatibility set. Passing this gate proves
// package publication and package-index publisher identity only. It changes no
// lifecycle or host authority.
func VerifyPublishedPackageSet(root string) error {
	candidates, err := publishedPackageCandidateSet(root)
	if err != nil {
		return err
	}
	return admissionrelease.VerifyPublishedPackageSet(root, candidates)
}

// VerifyRetainedGaCoreV1PublishedPackageSet verifies the immutable per-Form
// releases selected by the retained ga-core-v1 generation. It reconstructs
// that historical candidate set from the retained publication snapshot so an
// unpublished successor cannot make old publication proof unverifiable.
func VerifyRetainedGaCoreV1PublishedPackageSet(root string) error {
	candidates, err := retainedMixedVersionCandidateSet(root, retainedGaCoreV1Root, "ga-core-v1")
	if err != nil {
		return err
	}
	return admissionrelease.VerifyPublishedPackageSetAt(root, retainedGaCoreV1Root, candidates)
}

// VerifyLegacyPublishedPackageSet verifies the immutable per-Form releases in
// the pre-reset portable-v1 catalog. Publication remains a historical fact;
// it does not grant current Form maturity.
func VerifyLegacyPublishedPackageSet(root string) error {
	_, err := LegacyPublishedPackageSet(root)
	return err
}

// LegacyPublishedPackageSet authenticates and returns the exact retained
// all-Form publication manifest from the pre-reset portable-v1 line.
func LegacyPublishedPackageSet(root string) (formpublication.Set, error) {
	candidates, err := LegacyPortableCandidateSet(root)
	if err != nil {
		return formpublication.Set{}, err
	}
	// Publication evidence describes what is published; source may legitimately
	// have moved ahead of it. Authenticate the retained bytes against the
	// identities they actually claim, then check separately that source is
	// either level with publication or an unpublished successor of it. Passing
	// source identities straight in would make the first version of any new
	// Form unpublishable: this gate would demand evidence that only publishing
	// could produce, and publishing runs behind this gate.
	published, err := publishedExpectation(root, candidates)
	if err != nil {
		return formpublication.Set{}, err
	}
	set, err := formpublication.VerifyAt(root, legacyGaCoreV2Root, published)
	if err != nil {
		return formpublication.Set{}, err
	}
	if err := assertSourceIsPublishedOrSuccessor(candidates, published); err != nil {
		return formpublication.Set{}, err
	}
	return set, nil
}

// publishedExpectation rewrites the source candidate set to the exact Form
// identities the retained publication manifest carries. Every other field is
// left untouched, so publication verification keeps checking real published
// bytes against their own claims rather than against a version nobody has
// released yet.
func publishedExpectation(
	root string,
	candidates admissionrelease.CandidateSet,
) (admissionrelease.CandidateSet, error) {
	var manifest formpublication.Set
	path := filepath.Join(
		root,
		filepath.FromSlash(legacyGaCoreV2Root),
		formpublication.SetFilename,
	)
	if err := readJSON(path, &manifest); err != nil {
		return admissionrelease.CandidateSet{}, fmt.Errorf(
			"read retained publication manifest: %w", err)
	}
	byKind := make(map[string]formpublication.Entry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if _, duplicate := byKind[entry.Kind]; duplicate {
			return admissionrelease.CandidateSet{}, fmt.Errorf(
				"retained publication manifest repeats %s", entry.Kind)
		}
		byKind[entry.Kind] = entry
	}
	expected := candidates
	expected.Entries = make([]admissionrelease.Candidate, 0, len(candidates.Entries))
	for _, candidate := range candidates.Entries {
		entry, ok := byKind[candidate.Kind]
		if !ok {
			return admissionrelease.CandidateSet{}, fmt.Errorf(
				"retained publication manifest omits %s", candidate.Kind)
		}
		candidate.FormRef = entry.FormRef
		candidate.PackageDigest = entry.PackageDigest
		candidate.PackagePath = entry.SourcePath
		expected.Entries = append(expected.Entries, candidate)
	}
	return expected, nil
}

// assertSourceIsPublishedOrSuccessor allows source to be ahead of publication
// but never to disagree with it. A Form at the published version must match
// its published bytes exactly; a Form beyond it must be a strictly greater
// stable SemVer, which is the state of any release that has been authored and
// not yet published.
func assertSourceIsPublishedOrSuccessor(
	source, published admissionrelease.CandidateSet,
) error {
	if len(source.Entries) != len(published.Entries) {
		return fmt.Errorf("publication expectation cardinality changed")
	}
	for index, candidate := range source.Entries {
		release := published.Entries[index]
		if candidate.Kind != release.Kind {
			return fmt.Errorf("publication expectation reordered at %d", index)
		}
		if candidate.FormRef == release.FormRef &&
			candidate.PackageDigest == release.PackageDigest {
			continue
		}
		sourceVersion, err := parseStableFormVersion(candidate.FormRef.DefinitionVersion)
		if err != nil {
			return fmt.Errorf("%s source version: %w", candidate.Kind, err)
		}
		publishedVersion, err := parseStableFormVersion(release.FormRef.DefinitionVersion)
		if err != nil {
			return fmt.Errorf("%s published version: %w", candidate.Kind, err)
		}
		if !stableFormVersionLess(publishedVersion, sourceVersion) {
			return fmt.Errorf(
				"%s source %s does not match published %s and is not a later version; "+
					"a published release is immutable",
				candidate.Kind,
				candidate.FormRef.DefinitionVersion,
				release.FormRef.DefinitionVersion,
			)
		}
	}
	return nil
}

func retainedMixedVersionCandidateSet(root, retainedRoot, generation string) (admissionrelease.CandidateSet, error) {
	var published admissionrelease.PublishedPackageSet
	if err := readJSON(filepath.Join(root, filepath.FromSlash(retainedRoot), "published-package-set.json"), &published); err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	if published.Format != "takoform.published-package-set@v2" ||
		published.Generation != generation ||
		published.DefinitionVersion != "" ||
		published.PackageVersion != "" ||
		len(published.Entries) != 10 {
		return admissionrelease.CandidateSet{}, fmt.Errorf("%s published package set identity is invalid", generation)
	}
	seenKinds := make(map[string]struct{}, len(published.Entries))
	seenSlugs := make(map[string]struct{}, len(published.Entries))
	candidates := make([]admissionrelease.Candidate, 0, len(published.Entries))
	for index, entry := range published.Entries {
		if entry.Kind == "" || entry.Slug == "" ||
			entry.FormRef.APIVersion != formpackage.FormAPIVersion ||
			entry.FormRef.Kind != entry.Kind ||
			!formpackage.ValidDigest(entry.FormRef.SchemaDigest) ||
			!formpackage.ValidDigest(entry.PackageDigest) {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s published packages[%d] has invalid exact identity", generation, index)
		}
		if _, duplicate := seenKinds[entry.Kind]; duplicate {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s published packages[%d] duplicates kind %s", generation, index, entry.Kind)
		}
		seenKinds[entry.Kind] = struct{}{}
		if _, duplicate := seenSlugs[entry.Slug]; duplicate {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s published packages[%d] duplicates slug %s", generation, index, entry.Slug)
		}
		seenSlugs[entry.Slug] = struct{}{}
		packagePath := filepath.ToSlash(filepath.Join("forms", "releases", releaseIDForKind(entry.Kind), entry.FormRef.DefinitionVersion))
		report, err := formpackage.VerifyDirectory(filepath.Join(root, filepath.FromSlash(packagePath)))
		if err != nil {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s historical published package source: %w", entry.Kind, err)
		}
		if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s historical published package source identity drift", entry.Kind)
		}
		candidates = append(candidates, admissionrelease.Candidate{
			Kind: entry.Kind, Slug: entry.Slug, PackagePath: packagePath,
			FormRef: entry.FormRef, PackageDigest: entry.PackageDigest,
		})
	}
	return admissionrelease.CandidateSet{Generation: generation, Entries: candidates}, nil
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

// LegacyPortableCandidateSet returns the complete pre-reset portable-v1
// catalog as exact local release-source identities. It is retained for
// provider compatibility and historical evidence, not current maturity.
func LegacyPortableCandidateSet(root string) (admissionrelease.CandidateSet, error) {
	if err := Verify(root); err != nil {
		return admissionrelease.CandidateSet{}, fmt.Errorf("verify Legacy portable-v1 catalog: %w", err)
	}
	var inventory Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &inventory); err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	if inventory.Format != "takoform.standard-package-set@v1" ||
		inventory.Generation != "portable-v1" ||
		len(inventory.Packages) != len(Specs) || len(inventory.Packages) != 34 {
		return admissionrelease.CandidateSet{}, fmt.Errorf("Legacy portable-v1 catalog identity is invalid")
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
			return admissionrelease.CandidateSet{}, fmt.Errorf("Legacy portable-v1 packages[%d] has invalid exact identity", index)
		}
		if _, duplicate := seen[entry.Kind]; duplicate {
			return admissionrelease.CandidateSet{}, fmt.Errorf("Legacy portable-v1 packages[%d] duplicates kind %s", index, entry.Kind)
		}
		seen[entry.Kind] = struct{}{}
		packagePath := filepath.ToSlash(filepath.Join("forms", "releases", releaseIDForKind(entry.Kind), entry.FormRef.DefinitionVersion))
		report, err := formpackage.VerifyDirectory(filepath.Join(root, filepath.FromSlash(packagePath)))
		if err != nil {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s Legacy portable-v1 release source: %w", entry.Kind, err)
		}
		if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
			return admissionrelease.CandidateSet{}, fmt.Errorf("%s Legacy portable-v1 release-source identity drift", entry.Kind)
		}
		candidates = append(candidates, admissionrelease.Candidate{
			Kind: entry.Kind, Slug: spec.Slug, PackagePath: packagePath,
			FormRef: entry.FormRef, PackageDigest: entry.PackageDigest,
		})
	}
	return admissionrelease.CandidateSet{Generation: inventory.Generation, Entries: candidates}, nil
}

// CurrentAdmissionCandidateSet is retained only as a fail-closed compatibility
// seam for the removed internal v4 material builder. New central admission
// generations are forbidden by the lifecycle authority.
//
// Deprecated: there is no current central admission candidate set.
func CurrentAdmissionCandidateSet(root string) (admissionrelease.CandidateSet, error) {
	return admissionrelease.CandidateSet{}, fmt.Errorf("central admission generation was retired; use host-owned activation policy and retained Legacy evidence")
}

func verifyPortableHostContract(root string, entries []InventoryEntry) error {
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
	contract, err := portableconformance.Verify(filepath.Join(root, "conformance", "portable-host-v1"))
	if err != nil {
		return fmt.Errorf("portable host contract: %w", err)
	}
	wantIdentity := portableconformance.InstalledFormReference{
		FormRef: portableconformance.FormRef{
			APIVersion: bucket.FormRef.APIVersion, Kind: bucket.FormRef.Kind,
			DefinitionVersion: bucket.FormRef.DefinitionVersion, SchemaDigest: bucket.FormRef.SchemaDigest,
		},
		PackageDigest: bucket.PackageDigest,
	}
	if contract.RunnerInput.Identity != wantIdentity {
		return fmt.Errorf("portable host contract ObjectBucket identity differs from the Legacy compatibility identity")
	}
	wantDesired, err := portableHostDesired(root, *bucket)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(contract.RunnerInput.Desired, wantDesired) {
		return fmt.Errorf("portable host contract desired input differs from the exact ObjectBucket canonical fixture")
	}
	wantNegativeFixtures, err := portableHostDesiredNegativeFixtures(root, *bucket)
	if err != nil {
		return err
	}
	if len(contract.RunnerInput.NegativeFixtures) != len(wantNegativeFixtures) {
		return fmt.Errorf("portable host contract desired negative inventory has %d entries, want %d",
			len(contract.RunnerInput.NegativeFixtures), len(wantNegativeFixtures))
	}
	for index, want := range wantNegativeFixtures {
		got := contract.RunnerInput.NegativeFixtures[index]
		if got.Name != want.Name || got.Stage != want.Stage || got.Path != want.Path || got.SHA256 != want.SHA256 {
			return fmt.Errorf("portable host contract desired negative fixture %d differs from exact ObjectBucket package bytes", index)
		}
	}
	return nil
}

func portableHostDesired(root string, bucket InventoryEntry) (map[string]any, error) {
	var desired map[string]any
	path := filepath.Join(root, filepath.FromSlash(bucket.Path), "fixtures", "desired.json")
	if err := readJSON(path, &desired); err != nil {
		return nil, fmt.Errorf("read standard ObjectBucket canonical desired fixture: %w", err)
	}
	return desired, nil
}

func portableHostDesiredNegativeFixtures(root string, bucket InventoryEntry) ([]portableconformance.RunnerNegativeFixture, error) {
	packageRoot := filepath.Join(root, filepath.FromSlash(bucket.Path))
	definitionRaw, err := os.ReadFile(filepath.Join(packageRoot, "definition.json"))
	if err != nil {
		return nil, err
	}
	definition, err := formpackage.ValidateDefinition(definitionRaw)
	if err != nil {
		return nil, err
	}
	contractRoot := filepath.Join(root, "conformance", "portable-host-v1")
	fixtures := make([]portableconformance.RunnerNegativeFixture, 0, len(definition.NegativeFixtures))
	for _, fixture := range definition.NegativeFixtures {
		if fixture.Stage != "desired" {
			continue
		}
		source := filepath.Join(packageRoot, filepath.FromSlash(fixture.InputPath))
		raw, err := os.ReadFile(source)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(contractRoot, source)
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, portableconformance.RunnerNegativeFixture{
			Name: fixture.Name, Stage: fixture.Stage,
			Path: filepath.ToSlash(relative), SHA256: formpackage.DigestBytes(raw),
		})
	}
	if len(fixtures) == 0 {
		return nil, fmt.Errorf("standard ObjectBucket has no desired-stage negative fixtures for host conformance")
	}
	return fixtures, nil
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
