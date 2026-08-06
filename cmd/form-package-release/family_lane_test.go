package main

// family_lane_test.go covers the publication blocker of the Form Family lane:
// build-package compared the tag's decoded release id against the bare
// index.FormRef.Kind, but a family release id encodes "<group>/<Kind>" so the
// comparison could never match and nothing in the Edge Family could be tagged.

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const testFamilyGroup = "edge.forms.takoform.com/v1alpha1"

// stageFamilyPackage copies the real published Edge Family ModuleWorker
// package into a throwaway repository and returns its verified digest.
func stageFamilyPackage(t *testing.T) (repo, packageDir string, report formpackage.VerificationReport) {
	t.Helper()
	repo = makeTestRepo(t)
	packageDir = filepath.Join(repo, "package")
	copyTree(t, filepath.Join(repositoryRoot(t), "forms", "candidates", "edge", "v1alpha1", "module-worker"), packageDir)
	verified, err := formpackage.VerifyDirectory(packageDir)
	if err != nil {
		t.Fatalf("staged family package does not verify: %v", err)
	}
	if verified.FormRef.APIVersion != testFamilyGroup || verified.FormRef.Kind != "ModuleWorker" {
		t.Fatalf("staged package is not the family ModuleWorker: %+v", verified.FormRef)
	}
	gitCommitAll(t, repo, "family package")
	return repo, packageDir, verified
}

func TestBuildFamilyPackageAcceptsTheGroupQualifiedReleaseID(t *testing.T) {
	repo, packageDir, report := stageFamilyPackage(t)

	releaseID := formpackage.ReleaseIDForGroupKind(testFamilyGroup, "ModuleWorker")
	artifactID := strings.Replace(report.PackageDigest, ":", "-", 1)
	tag := "forms/" + releaseID + "/" + artifactID

	// The locator the verified package derives is the tag being requested, and
	// it is the family lane rather than the retained central one.
	locator, err := formpackage.ParsePublicationTag(tag)
	if err != nil {
		t.Fatal(err)
	}
	if locator.APIVersion != formpackage.FamilyPackageAPIVersion {
		t.Fatalf("family tag parsed as %q", locator.APIVersion)
	}

	output := filepath.Join(t.TempDir(), "release")
	if err := run([]string{
		"build-package", "--repo", repo, "--tag", tag,
		"--package-dir", packageDir, "--output", output,
		"--tooling-commit", testToolingCommit, "--allow-untagged-candidate",
	}, io.Discard); err != nil {
		t.Fatalf("family package could not be built for publication: %v", err)
	}

	var manifest releaseManifest
	readJSON(t, filepath.Join(output, "release-manifest.json"), &manifest)
	if manifest.ReleaseID != releaseID || manifest.ArtifactID != artifactID || manifest.PackageVersion != "" {
		t.Fatalf("family release manifest identity = %+v", manifest)
	}
	if manifest.FormRef.APIVersion != testFamilyGroup || manifest.FormRef.Kind != "ModuleWorker" {
		t.Fatalf("family release manifest FormRef = %+v", manifest.FormRef)
	}
	base := "takoform-form-" + releaseID + "_" + artifactID
	for _, name := range []string{base + ".tar.gz", base + "_package-index.json", base + "_sbom.spdx.json"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("family release asset %q is missing: %v", name, err)
		}
	}
}

// TestBuildFamilyPackageRejectsTheBareKindReleaseID keeps the lanes apart: the
// central release line of the same Kind name must not be able to carry a family
// package, or two different Forms would share one publication identity.
func TestBuildFamilyPackageRejectsTheBareKindReleaseID(t *testing.T) {
	repo, packageDir, report := stageFamilyPackage(t)

	bareTag := "forms/" + mustReleaseID(t, "ModuleWorker") + "/" +
		strings.Replace(report.PackageDigest, ":", "-", 1)
	output := filepath.Join(t.TempDir(), "release")
	err := run([]string{
		"build-package", "--repo", repo, "--tag", bareTag,
		"--package-dir", packageDir, "--output", output,
		"--tooling-commit", testToolingCommit, "--allow-untagged-candidate",
	}, io.Discard)
	if err == nil {
		t.Fatal("a family package was published under the central bare-Kind release line")
	}
	if !strings.Contains(err.Error(), "does not match verified package publication locator") {
		t.Fatalf("unexpected rejection: %v", err)
	}
}

// TestBuildFamilyPackageRejectsAnotherFamilysReleaseID proves the group half of
// the release identity is enforced, not merely encoded.
func TestBuildFamilyPackageRejectsAnotherFamilysReleaseID(t *testing.T) {
	repo, packageDir, report := stageFamilyPackage(t)

	foreignTag := "forms/" +
		formpackage.ReleaseIDForGroupKind("forms.example.com/v1alpha1", "ModuleWorker") + "/" +
		strings.Replace(report.PackageDigest, ":", "-", 1)
	output := filepath.Join(t.TempDir(), "release")
	err := run([]string{
		"build-package", "--repo", repo, "--tag", foreignTag,
		"--package-dir", packageDir, "--output", output,
		"--tooling-commit", testToolingCommit, "--allow-untagged-candidate",
	}, io.Discard)
	if err == nil {
		t.Fatal("a family package was published under another family's release line")
	}
}
