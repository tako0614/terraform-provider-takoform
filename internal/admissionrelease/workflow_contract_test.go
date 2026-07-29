package admissionrelease

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const standardAdmissionReleaseCandidateContractSHA256 = "249991f2c54b901c664597a03920258dc97eb09dc589b04ebc64b02c3df3425b"

func TestStandardAdmissionReleaseCandidateContractBytesArePinned(t *testing.T) {
	t.Parallel()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve workflow contract test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, "admission", "v1", "trust", "release-candidate-contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	if actual := fmt.Sprintf("%x", sha256.Sum256(raw)); actual != standardAdmissionReleaseCandidateContractSHA256 {
		t.Fatalf("release-candidate contract digest = %s, want %s", actual, standardAdmissionReleaseCandidateContractSHA256)
	}
}

func TestRepositoryDoesNotReintroduceSetWideReleaseWorkflows(t *testing.T) {
	t.Parallel()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve workflow contract test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	for _, retired := range []string{
		".github/workflows/standard-admission-release.yml",
		".github/workflows/standard-form-package-set-release.yml",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(retired))); err == nil {
			t.Errorf("%s reintroduces one mutable set-wide release lane", retired)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", retired, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "forms", "release-plan.json")); err != nil {
		t.Fatalf("the independent per-Form release plan is missing: %v", err)
	}
}
