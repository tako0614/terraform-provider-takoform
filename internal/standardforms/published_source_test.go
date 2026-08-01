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

// hasRetainedPredecessor reports whether the retained history already carries
// an earlier version of the same release identity, which is what makes an
// unpublished plan entry a successor rather than an invention.
func hasRetainedPredecessor(
	published map[string]publishedReleaseSource,
	release PlannedFormRelease,
) bool {
	planned, err := parseStableFormVersion(release.Version)
	if err != nil {
		return false
	}
	for _, source := range published {
		if source.ReleaseID != release.ReleaseID {
			continue
		}
		existing, err := parseStableFormVersion(source.Version)
		if err != nil {
			continue
		}
		if stableFormVersionLess(existing, planned) {
			return true
		}
	}
	return false
}

func TestCommittedPublishedReleaseSourcesCoverCurrentPlanAndHistory(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	published, err := discoverPublishedReleaseSources(root)
	if err != nil {
		t.Fatal(err)
	}

	var plan ReleasePlan
	if err := readJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), &plan); err != nil {
		t.Fatal(err)
	}
	// A planned release is either already retained, in which case its bytes
	// must match exactly, or it is a version authored ahead of publication.
	// The second case is what authoring a Form looks like before its release
	// train runs; demanding retained history for it would require evidence
	// that only publishing can produce, from a gate publishing runs behind.
	// An unpublished plan entry must still be a successor of something real,
	// so a fabricated Kind cannot hide in the gap.
	retained := 0
	pending := 0
	for _, release := range plan.Releases {
		source, ok := published[publishedReleaseKey(release.ReleaseID, release.Version)]
		if !ok {
			if !hasRetainedPredecessor(published, release) {
				t.Fatalf(
					"planned %s@%s is neither retained nor a successor of a retained release",
					release.Kind, release.Version,
				)
			}
			pending++
			continue
		}
		if source.AdmissionGeneration == "" {
			// Tagged and byte-verified against the plan by
			// verifyLocalPublishedFormTags, but not yet snapshotted into an
			// admission generation. That is the state between publishing a
			// release and retaining it, and it is not drift.
			pending++
			continue
		}
		if source.AdmissionGeneration != "v4" ||
			source.Tag != release.Tag ||
			source.FormRef != release.FormRef ||
			source.PackageDigest != release.PackageDigest {
			t.Fatalf("retained current publication drift for %s@%s: %#v", release.Kind, release.Version, source)
		}
		retained++
	}
	if retained+pending != len(plan.Releases) || retained+pending != 34 {
		t.Fatalf("plan coverage = %d retained + %d pending, want exact plan count %d",
			retained, pending, len(plan.Releases))
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
	runTestGit(t, root, "-c", "user.name=Takoform Test", "-c", "user.email=test@takoform.invalid", "tag", "-a", "-m", "published source", tag)
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

func TestPlannedUnretainedFormTagAcceptsExactSourceIdentityAndBytes(t *testing.T) {
	t.Parallel()

	root, _, _ := createPlannedUnretainedTagFixture(t)
	if err := verifyLocalPublishedFormTags(root, map[string]publishedReleaseSource{}); err != nil {
		t.Fatalf("exact planned unretained Form tag rejected: %v", err)
	}
}

func TestUnretainedFormTagMustBeAnnotated(t *testing.T) {
	t.Parallel()

	root, tag, _ := createPlannedUnretainedTagFixture(t)
	runTestGit(t, root, "tag", "-d", tag)
	runTestGit(t, root, "tag", tag)

	err := verifyLocalPublishedFormTags(root, map[string]publishedReleaseSource{})
	if err == nil || !strings.Contains(err.Error(), "must be an annotated tag") {
		t.Fatalf("lightweight planned Form tag error = %v", err)
	}
}

func TestPlannedUnretainedFormTagJoinsNoOverwriteMap(t *testing.T) {
	t.Parallel()

	root, tag, sourcePath := createPlannedUnretainedTagFixture(t)
	published, err := discoverPublishedReleaseSources(root)
	if err != nil {
		t.Fatal(err)
	}
	var plan ReleasePlan
	if err := readJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), &plan); err != nil {
		t.Fatal(err)
	}
	planned := plan.Releases[0]
	source, ok := published[publishedReleaseKey(planned.ReleaseID, planned.Version)]
	if !ok || source.Tag != tag || source.AdmissionGeneration != "" {
		t.Fatalf("planned transitional source missing from no-overwrite map: %#v", source)
	}

	stagingRoot := t.TempDir()
	stagedPath := filepath.Join(stagingRoot, "candidate")
	if err := os.CopyFS(stagedPath, os.DirFS(filepath.Join(root, filepath.FromSlash(sourcePath)))); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(stagedPath, formpackage.PackageIndexFilename)
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, append(indexRaw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := InventoryEntry{
		Kind: planned.Kind, Path: "candidate",
		FormRef: planned.FormRef, PackageDigest: planned.PackageDigest,
	}
	err = verifyNoPublishedReleaseOverwrite(root, stagingRoot, []InventoryEntry{entry}, published)
	if err == nil || !strings.Contains(err.Error(), "package-index.json bytes differ") {
		t.Fatalf("planned transitional no-overwrite error = %v", err)
	}
}

func TestUnretainedFormTagMustBelongToCurrentReleasePlan(t *testing.T) {
	t.Parallel()

	root, _, _ := createPlannedUnretainedTagFixture(t)
	unknownTag := "forms/k-aaaaaaaa/v1.0.0"
	runTestGit(t, root, "-c", "user.name=Takoform Test", "-c", "user.email=test@takoform.invalid", "tag", "-a", "-m", "unknown source", unknownTag)

	err := verifyLocalPublishedFormTags(root, map[string]publishedReleaseSource{})
	if err == nil || !strings.Contains(err.Error(), "not in the current release plan") {
		t.Fatalf("unplanned Form tag error = %v", err)
	}
}

func TestUnretainedFormTagRejectsPlannedIdentityDrift(t *testing.T) {
	t.Parallel()

	root, tag, _ := createPlannedUnretainedTagFixture(t)
	var plan ReleasePlan
	if err := readJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), &plan); err != nil {
		t.Fatal(err)
	}
	plan.Releases[0].PackageDigest = "sha256:" + strings.Repeat("0", 64)
	if err := writeJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), plan); err != nil {
		t.Fatal(err)
	}

	err := verifyLocalPublishedFormTags(root, map[string]publishedReleaseSource{})
	if err == nil || !strings.Contains(err.Error(), "current release plan") {
		t.Fatalf("planned identity drift for %s error = %v", tag, err)
	}
}

func TestUnretainedFormTagRejectsSourceByteDrift(t *testing.T) {
	t.Parallel()

	root, tag, sourcePath := createPlannedUnretainedTagFixture(t)
	indexPath := filepath.Join(root, filepath.FromSlash(sourcePath), formpackage.PackageIndexFilename)
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, append(indexRaw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := formpackage.VerifyDirectory(filepath.Dir(indexPath)); err != nil {
		t.Fatalf("whitespace-only source drift should retain its canonical identity: %v", err)
	}

	err = verifyLocalPublishedFormTags(root, map[string]publishedReleaseSource{})
	if err == nil || !strings.Contains(err.Error(), "differs byte-for-byte from immutable tag") {
		t.Fatalf("planned source byte drift for %s error = %v", tag, err)
	}
}

func TestUnretainedFormTagRejectsSourceFileClosureDrift(t *testing.T) {
	t.Parallel()

	root, tag, sourcePath := createPlannedUnretainedTagFixture(t)
	extraPath := filepath.Join(root, filepath.FromSlash(sourcePath), "unexpected.txt")
	if err := os.WriteFile(extraPath, []byte("not present in the immutable tag\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifyLocalPublishedFormTags(root, map[string]publishedReleaseSource{})
	if err == nil ||
		!strings.Contains(err.Error(), "current release plan") ||
		!strings.Contains(err.Error(), "file closure mismatch") {
		t.Fatalf("planned source file-closure drift for %s error = %v", tag, err)
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

func createPlannedUnretainedTagFixture(t *testing.T) (root, tag, sourcePath string) {
	t.Helper()

	repositoryRoot := filepath.Join("..", "..")
	root = t.TempDir()
	kind := "EdgeWorker"
	version := "3.0.0"
	if err := os.MkdirAll(filepath.Join(root, "admission"), 0o755); err != nil {
		t.Fatal(err)
	}
	releaseID := releaseIDForKind(kind)
	sourcePath = filepath.ToSlash(filepath.Join("forms", "releases", releaseID, version))
	sourceRoot := filepath.Join(root, filepath.FromSlash(sourcePath))
	if err := os.CopyFS(
		sourceRoot,
		os.DirFS(filepath.Join(repositoryRoot, filepath.FromSlash(sourcePath))),
	); err != nil {
		t.Fatal(err)
	}
	report, err := formpackage.VerifyDirectory(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	entry := InventoryEntry{
		Kind: kind, Path: "fixtures/edge-worker",
		FormRef: report.FormRef, PackageDigest: report.PackageDigest,
	}
	inventory := Inventory{Packages: []InventoryEntry{entry}}
	if err := writeJSON(filepath.Join(root, "forms", "standard-package-set.json"), inventory); err != nil {
		t.Fatal(err)
	}
	plan, err := buildReleasePlan(root, inventory.Packages)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, filepath.FromSlash(ReleasePlanPath)), plan); err != nil {
		t.Fatal(err)
	}

	runTestGit(t, root, "init", "-q")
	runTestGit(t, root, "add", "forms")
	runTestGit(t, root, "-c", "user.name=Takoform Test", "-c", "user.email=test@takoform.invalid", "commit", "-qm", "planned source")
	tag = plan.Releases[0].Tag
	runTestGit(t, root, "-c", "user.name=Takoform Test", "-c", "user.email=test@takoform.invalid", "tag", "-a", "-m", "planned source", tag)
	return root, tag, sourcePath
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
