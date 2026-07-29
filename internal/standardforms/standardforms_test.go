package standardforms

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

func TestCommittedCandidateSetVerifies(t *testing.T) {
	t.Parallel()
	if err := Verify(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseSourceRequiresExactReviewedFixtureBytes(t *testing.T) {
	t.Parallel()
	fixtureRoot := filepath.Join("..", "..", "conformance", "form-package-v1", "positive", "standard", "object-bucket")
	releaseRoot := filepath.Join(t.TempDir(), "release")
	if err := os.CopyFS(releaseRoot, os.DirFS(fixtureRoot)); err != nil {
		t.Fatal(err)
	}
	report, err := formpackage.VerifyDirectory(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	entry := InventoryEntry{Kind: "ObjectBucket", FormRef: report.FormRef, PackageDigest: report.PackageDigest}
	if err := verifyReleaseSource(fixtureRoot, releaseRoot, entry); err != nil {
		t.Fatalf("exact release source rejected: %v", err)
	}
	indexPath := filepath.Join(releaseRoot, formpackage.PackageIndexFilename)
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, append(indexRaw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseSource(fixtureRoot, releaseRoot, entry); err == nil || !strings.Contains(err.Error(), "package-index.json bytes differ") {
		t.Fatalf("non-exact release source error = %v", err)
	}
}

func TestPublishedPackageSetVerifiesIndependentlyOfAdmission(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	if err := VerifyPublishedPackageSet(root); err != nil {
		t.Fatal(err)
	}
	var retired RetiredInventory
	if err := readJSON(filepath.Join(root, filepath.FromSlash(RetiredInventoryPath)), &retired); err != nil {
		t.Fatal(err)
	}
	var published struct {
		DefinitionVersion string `json:"definitionVersion"`
		PackageVersion    string `json:"packageVersion"`
	}
	if err := readJSON(filepath.Join(root, "admission", "v1", "published-package-set.json"), &published); err != nil {
		t.Fatal(err)
	}
	if retired.DefinitionVersion != published.DefinitionVersion || retired.PackageVersion != published.PackageVersion {
		t.Fatalf("retired inventory drifted from published evidence: retired=%s/%s published=%s/%s",
			retired.DefinitionVersion, retired.PackageVersion, published.DefinitionVersion, published.PackageVersion)
	}
	if len(retired.Packages) != len(RetiredKinds) {
		t.Fatalf("retired inventory holds %d packages, want %d", len(retired.Packages), len(RetiredKinds))
	}
	// The rebuilt Forms must not silently reuse a published identity.
	published_ids := map[string]struct{}{}
	for _, entry := range retired.Packages {
		published_ids[entry.FormRef.Kind+"@"+entry.FormRef.DefinitionVersion] = struct{}{}
	}
	var active Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &active); err != nil {
		t.Fatal(err)
	}
	for _, entry := range active.Packages {
		if _, clash := published_ids[entry.FormRef.Kind+"@"+entry.FormRef.DefinitionVersion]; clash {
			t.Fatalf("%s reuses a published definition version", entry.FormRef.Kind)
		}
	}
}

func TestCurrentAdmissionCandidateSetIsExactMixedVersionGeneration(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	set, err := CurrentAdmissionCandidateSet(root)
	if err != nil {
		t.Fatal(err)
	}
	if set.Generation != "ga-core-v1" || set.DefinitionVersion != "" || set.PackageVersion != "" || len(set.Entries) != 10 {
		t.Fatalf("unexpected current admission set: %#v", set)
	}
	versions := map[string]bool{}
	for _, entry := range set.Entries {
		versions[entry.FormRef.DefinitionVersion] = true
	}
	if !versions["1.0.0"] || !versions["2.0.0"] || len(versions) != 2 {
		t.Fatalf("current admission set lost mixed versions: %#v", versions)
	}
}

func TestCurrentPortableCandidateSetRetainsAllThirtyFourIdentities(t *testing.T) {
	t.Parallel()
	set, err := CurrentPortableCandidateSet(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if set.Generation != "portable-v1" || len(set.Entries) != 34 {
		t.Fatalf("unexpected current portable set: generation=%q entries=%d", set.Generation, len(set.Entries))
	}
}

func TestCurrentPublishedPackageSetAuthenticatesExactLiveReadback(t *testing.T) {
	t.Parallel()
	if err := VerifyCurrentPublishedPackageSet(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}

// TestEveryDeclaredFormHasMaterializableFixtures proves the generated
// fixtures are real input rather than placeholders, for every declared Form.
func TestEveryDeclaredFormHasMaterializableFixtures(t *testing.T) {
	t.Parallel()
	if err := VerifyMaterializableCandidate(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	for _, kind := range formcatalog.Kinds {
		desired := kind.CanonicalDesired()
		if desired["name"] == "" {
			t.Fatalf("%s canonical fixture has no name", kind.Kind)
		}
		if _, err := kind.NegativeDesired(); err != nil {
			t.Fatalf("%s has no rejectable counter-example: %v", kind.Kind, err)
		}
		if kind.Artifact {
			source, ok := desired["source"].(map[string]any)
			if !ok || len(source) == 0 {
				t.Fatalf("%s declares an artifact source with no fixture", kind.Kind)
			}
		}
		if kind.Connections == formcatalog.ConnectionsRequired {
			if _, ok := desired["connections"].(map[string]any); !ok {
				t.Fatalf("%s requires a connection but its fixture declares none", kind.Kind)
			}
		}
	}
}

// TestDeclaredFormsOwnPortableRuntimeInterfaceDescriptors proves interface
// declarations stay portable: open names, non-secret documents, and inputs
// resolved only through the Form's own outputs.
func TestDeclaredFormsOwnPortableRuntimeInterfaceDescriptors(t *testing.T) {
	t.Parallel()
	declaredNames := map[string]struct{}{}
	for _, kind := range formcatalog.Kinds {
		descriptors := kind.InterfaceDescriptors()
		if len(descriptors) != len(kind.Interfaces) {
			t.Fatalf("%s descriptor count = %d, want %d", kind.Kind, len(descriptors), len(kind.Interfaces))
		}
		for _, descriptor := range descriptors {
			declaredNames[descriptor.Name] = struct{}{}
			if descriptor.Version != "1" || !descriptor.Required || descriptor.Document == nil {
				t.Fatalf("%s portable descriptor = %#v", kind.Kind, descriptor)
			}
			if strings.Contains(strings.ToLower(descriptor.Name), "takosumi") {
				t.Fatalf("%s descriptor leaks a host identity: %s", kind.Kind, descriptor.Name)
			}
			for _, input := range descriptor.Inputs {
				if !formpackage.PortableInterfaceInputSource(input.Source) {
					t.Fatalf("%s descriptor input is not portable: %#v", kind.Kind, input)
				}
			}
		}
	}
	if len(declaredNames) < 10 {
		t.Fatalf("the portable Form set declares only %d distinct runtime interfaces", len(declaredNames))
	}
}

// TestInventoryCoversEveryDeclaredFormExactlyOnce proves the generated
// inventory is the catalogue, with no dropped or duplicated Form.
func TestInventoryCoversEveryDeclaredFormExactlyOnce(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	var inventory Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Packages) != len(formcatalog.Kinds) {
		t.Fatalf("inventory holds %d packages, want %d declared Forms", len(inventory.Packages), len(formcatalog.Kinds))
	}
	seen := map[string]struct{}{}
	for _, entry := range inventory.Packages {
		declared, ok := formcatalog.ByKind(entry.Kind)
		if !ok {
			t.Fatalf("inventory holds undeclared kind %s", entry.Kind)
		}
		if _, duplicate := seen[entry.Kind]; duplicate {
			t.Fatalf("inventory duplicates %s", entry.Kind)
		}
		seen[entry.Kind] = struct{}{}
		if entry.FormRef.DefinitionVersion != declared.Version() {
			t.Fatalf("%s definition version = %s, want %s", entry.Kind, entry.FormRef.DefinitionVersion, declared.Version())
		}
		if entry.AdmissionStatus != "external-required" {
			t.Fatalf("%s claims admission status %q", entry.Kind, entry.AdmissionStatus)
		}
		report, err := formpackage.VerifyDirectory(filepath.Join(root, filepath.FromSlash(entry.Path)))
		if err != nil {
			t.Fatalf("%s package: %v", entry.Kind, err)
		}
		if report.PackageDigest != entry.PackageDigest {
			t.Fatalf("%s package digest drift", entry.Kind)
		}
	}
}

func TestCandidatePublicationDoesNotActivateStandardForms(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	inventoryPath := filepath.Join(root, "forms", "standard-package-set.json")
	before, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCandidatePublication(root); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("candidate publication gate mutated the standard package inventory")
	}
	var inventory Inventory
	if err := readJSON(inventoryPath, &inventory); err != nil {
		t.Fatal(err)
	}
	if inventory.AdmissionStatus != "external-required" || inventory.PublicationReady {
		t.Fatalf("candidate publication changed admission truth: status=%q ready=%v", inventory.AdmissionStatus, inventory.PublicationReady)
	}
	for _, entry := range inventory.Packages {
		if entry.AdmissionStatus != "external-required" {
			t.Fatalf("candidate publication admitted %s", entry.Kind)
		}
	}
}
