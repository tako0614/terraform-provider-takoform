package standardforms

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestCommittedStableSetVerifies(t *testing.T) {
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

func TestAdmissionActivationGateFailsClosedWithoutExternalAdmission(t *testing.T) {
	t.Parallel()
	err := VerifyReleaseReady(filepath.Join("..", ".."))
	if err == nil || !strings.Contains(err.Error(), "missing admission/v1/standard-admission-set.json") {
		t.Fatalf("release gate error = %v", err)
	}
}

func TestPublishedPackageSetVerifiesWithoutAdmittingForms(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	if err := VerifyPublishedPackageSet(root); err != nil {
		t.Fatal(err)
	}
	err := VerifyReleaseReady(root)
	if err == nil || !strings.Contains(err.Error(), "missing admission/v1/standard-admission-set.json") {
		t.Fatalf("published package readback opened admission: %v", err)
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
