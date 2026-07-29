package standardforms

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestCommittedPublishedReleaseSourcesCoverEveryRetainedIdentity(t *testing.T) {
	t.Parallel()

	published, err := discoverPublishedReleaseSources(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 30 {
		t.Fatalf("published release source count = %d, want 30", len(published))
	}
	for _, identity := range []struct {
		kind, version string
	}{
		{kind: "EdgeWorker", version: "1.0.0"},
		{kind: "EdgeWorker", version: "1.0.1"},
		{kind: "ContainerService", version: "2.0.0"},
	} {
		releaseID := releaseIDForKind(identity.kind)
		if _, ok := published[publishedReleaseKey(releaseID, identity.version)]; !ok {
			t.Fatalf("retained publication history omits %s@%s", identity.kind, identity.version)
		}
	}
}

func TestPublishedReleasePreflightRejectsCanonicalDigestEquivalentByteDrift(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Join("..", "..")
	root := t.TempDir()
	releaseID := releaseIDForKind("EdgeWorker")
	version := "1.0.1"
	copyPublishedReleaseFixture(t, repositoryRoot, root, "v1", releaseID, version)

	published, err := discoverPublishedReleaseSources(root)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "forms", "releases", releaseID, version)
	stagingRoot := t.TempDir()
	staged := filepath.Join(stagingRoot, "candidate")
	if err := os.CopyFS(staged, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	report, err := formpackage.VerifyDirectory(staged)
	if err != nil {
		t.Fatal(err)
	}
	entry := InventoryEntry{
		Kind: "EdgeWorker", Path: "candidate",
		FormRef: report.FormRef, PackageDigest: report.PackageDigest,
	}
	if err := verifyNoPublishedReleaseOverwrite(root, stagingRoot, []InventoryEntry{entry}, published); err != nil {
		t.Fatalf("byte-exact published candidate rejected: %v", err)
	}

	indexPath := filepath.Join(staged, formpackage.PackageIndexFilename)
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, append(indexRaw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	// The package digest is RFC 8785 based, so insignificant JSON whitespace
	// alone does not change it. Published source protection must still reject
	// the byte-level overwrite.
	afterWhitespace, err := formpackage.VerifyDirectory(staged)
	if err != nil {
		t.Fatalf("whitespace-only package index change should remain structurally valid: %v", err)
	}
	if afterWhitespace.PackageDigest != report.PackageDigest {
		t.Fatalf("whitespace-only change altered canonical package digest: %s != %s", afterWhitespace.PackageDigest, report.PackageDigest)
	}
	err = verifyNoPublishedReleaseOverwrite(root, stagingRoot, []InventoryEntry{entry}, published)
	if err == nil || !strings.Contains(err.Error(), "package-index.json bytes differ") {
		t.Fatalf("published byte-drift error = %v", err)
	}
}

func TestPublishedReleaseSourceMustMatchImmutableTagBytes(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Join("..", "..")
	root := t.TempDir()
	releaseID := releaseIDForKind("EdgeWorker")
	version := "1.0.1"
	copyPublishedReleaseFixture(t, repositoryRoot, root, "v1", releaseID, version)

	runTestGit(t, root, "init", "-q")
	runTestGit(t, root, "add", "admission", "forms")
	runTestGit(t, root, "-c", "user.name=Takoform Test", "-c", "user.email=test@takoform.invalid", "commit", "-qm", "published source")
	tag := "forms/" + releaseID + "/v" + version
	runTestGit(t, root, "tag", tag)
	if _, err := discoverPublishedReleaseSources(root); err != nil {
		t.Fatalf("byte-exact tagged source rejected: %v", err)
	}

	indexPath := filepath.Join(root, "forms", "releases", releaseID, version, formpackage.PackageIndexFilename)
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, append(indexRaw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := formpackage.VerifyDirectory(filepath.Dir(indexPath)); err != nil {
		t.Fatalf("whitespace-only tagged source change should retain its canonical digest: %v", err)
	}
	_, err = discoverPublishedReleaseSources(root)
	if err == nil || !strings.Contains(err.Error(), "differs byte-for-byte from immutable tag") {
		t.Fatalf("tagged source byte-drift error = %v", err)
	}
}

func TestGenerateRejectsTamperedPublishedSourceBeforeRepositoryMutation(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Join("..", "..")
	root := t.TempDir()
	releaseID := releaseIDForKind("EdgeWorker")
	version := "1.0.1"
	copyPublishedReleaseFixture(t, repositoryRoot, root, "v1", releaseID, version)

	desiredPath := filepath.Join(root, "forms", "releases", releaseID, version, "fixtures", "desired.json")
	desiredRaw, err := os.ReadFile(desiredPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desiredPath, append(desiredRaw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	sentinelPath := filepath.Join(root, "conformance", "form-package-v1", "positive", "standard", "obsolete", "sentinel.txt")
	if err := os.MkdirAll(filepath.Dir(sentinelPath), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := []byte("must survive rejected generation\n")
	if err := os.WriteFile(sentinelPath, sentinel, 0o644); err != nil {
		t.Fatal(err)
	}

	err = Generate(root)
	if err == nil || !strings.Contains(err.Error(), "published release source") {
		t.Fatalf("tampered published source error = %v", err)
	}
	after, readErr := os.ReadFile(sentinelPath)
	if readErr != nil {
		t.Fatalf("generation mutated the repository before immutable-source preflight: %v", readErr)
	}
	if !bytes.Equal(after, sentinel) {
		t.Fatalf("generation rewrote sentinel before immutable-source preflight: %q", after)
	}
}

func copyPublishedReleaseFixture(t *testing.T, repositoryRoot, destinationRoot, generation, releaseID, version string) {
	t.Helper()

	sourceRoot := filepath.Join(repositoryRoot, "forms", "releases", releaseID, version)
	destinationSource := filepath.Join(destinationRoot, "forms", "releases", releaseID, version)
	if err := os.CopyFS(destinationSource, os.DirFS(sourceRoot)); err != nil {
		t.Fatal(err)
	}
	retainedRoot := filepath.Join(repositoryRoot, "admission", generation, "releases", releaseID, version)
	destinationRetained := filepath.Join(destinationRoot, "admission", generation, "releases", releaseID, version)
	if err := os.CopyFS(destinationRetained, os.DirFS(retainedRoot)); err != nil {
		t.Fatal(err)
	}
}

func runTestGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	commandArguments := append([]string{"-C", root}, arguments...)
	output, err := exec.Command("git", commandArguments...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
}
