package admissionrelease

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireTagCommitRejectsMissingAndMismatchedRefs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runTestGit(t, root, "init", "--quiet")
	runTestGit(t, root, "config", "user.name", "Takoform Test")
	runTestGit(t, root, "config", "user.email", "test@takoform.invalid")
	filename := filepath.Join(root, "fixture.txt")
	if err := os.WriteFile(filename, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "add", "fixture.txt")
	runTestGit(t, root, "commit", "--quiet", "-m", "first")
	first := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))
	tag := "forms/k-iv4gc3lqnrsvg5dpojsq/v1.0.0"
	runTestGit(t, root, "tag", tag)
	if err := requireTagCommit(root, "fixture", tag, first); err != nil {
		t.Fatalf("exact tag ref: %v", err)
	}
	if err := requireTagCommit(root, "fixture", "forms/missing/v1.0.0", first); err == nil || !strings.Contains(err.Error(), "resolve") {
		t.Fatalf("missing tag error = %v", err)
	}
	if err := os.WriteFile(filename, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "add", "fixture.txt")
	runTestGit(t, root, "commit", "--quiet", "-m", "second")
	second := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))
	if err := requireCommitAncestor(root, "fixture tooling", first, second); err != nil {
		t.Fatalf("retained ancestor rejected: %v", err)
	}
	if err := requireCommitAncestor(root, "fixture tooling", second, first); err == nil || !strings.Contains(err.Error(), "not an ancestor") {
		t.Fatalf("non-ancestor tooling commit error = %v", err)
	}
	if err := requireCommitAncestor(root, "fixture tooling", strings.Repeat("f", 40), second); err == nil || !strings.Contains(err.Error(), "not retained") {
		t.Fatalf("missing tooling commit error = %v", err)
	}
	if err := requireTagCommit(root, "fixture", tag, second); err == nil || !strings.Contains(err.Error(), "want retained commit") {
		t.Fatalf("mismatched tag error = %v", err)
	}
}

func TestPinnedProviderSignatureRequiresExactFingerprint(t *testing.T) {
	t.Parallel()
	valid := "[GNUPG:] VALIDSIG " + providerSigningFingerprint + " 2026-01-01 0 4 0 1 10 00 " + providerSigningFingerprint
	if !hasPinnedProviderSignature(valid) {
		t.Fatal("exact pinned provider fingerprint was rejected")
	}
	if hasPinnedProviderSignature(strings.Replace(valid, providerSigningFingerprint, strings.Repeat("0", 40), 1)) {
		t.Fatal("unreviewed provider signing fingerprint was accepted")
	}
}

func runTestGit(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}
