package admissionrelease

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const historicalAdmissionReleaseCandidateContractSHA256 = "249991f2c54b901c664597a03920258dc97eb09dc589b04ebc64b02c3df3425b"

func TestHistoricalAdmissionReleaseCandidateContractBytesRemainPinned(t *testing.T) {
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
	if actual := fmt.Sprintf("%x", sha256.Sum256(raw)); actual != historicalAdmissionReleaseCandidateContractSHA256 {
		t.Fatalf("historical release-candidate contract digest = %s, want %s", actual, historicalAdmissionReleaseCandidateContractSHA256)
	}
}

func TestRepositoryDoesNotExposeSetWideAdmissionReleaseSurface(t *testing.T) {
	t.Parallel()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve workflow contract test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	for _, retired := range []string{
		".github/workflows/standard-admission-release.yml",
		".github/workflows/standard-form-package-set-release.yml",
		"cmd/form-package-release/standard_set.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(retired))); err == nil {
			t.Errorf("%s reintroduces one mutable set-wide release lane", retired)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", retired, err)
		}
	}
	for _, required := range []string{
		"forms/release-plan.json",
		".github/workflows/form-package-release.yml",
		".github/workflows/standard-admission-evidence.yml",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(required))); err != nil {
			t.Fatalf("the independent publication/evidence surface %s is missing: %v", required, err)
		}
	}

	commandSource, err := os.ReadFile(filepath.Join(root, "cmd", "form-package-release", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{
		"build-standard-set",
		"finalize-standard-set",
		"verify-standard-set-candidate",
		"build-standard-set-readback",
		"coordinated-standard-set",
		"standard-form-package-set-release.yml",
	} {
		if strings.Contains(string(commandSource), retired) {
			t.Errorf("cmd/form-package-release still exposes retired token %q", retired)
		}
	}

	evidenceWorkflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "standard-admission-evidence.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(evidenceWorkflow)
	if !strings.Contains(workflow, "Upload the closed signed candidate without publishing") ||
		!strings.Contains(workflow, "release, tag, registry, contents write, or production mutation: none") ||
		!strings.Contains(workflow, "run-name: ${{ inputs.request_id }}") ||
		!strings.Contains(workflow, `test "$(find "${HOST_CANDIDATE_PATH}" -type f | wc -l)" -eq 26`) ||
		!strings.Contains(workflow, `cosign verify-blob "${HOST_CANDIDATE_PATH}/portable-host-runner-report.json"`) ||
		!strings.Contains(workflow, `cosign verify-blob "${HOST_CANDIDATE_PATH}/SHA256SUMS"`) ||
		!strings.Contains(workflow, `--bundle "${HOST_CANDIDATE_PATH}/SHA256SUMS.sigstore.json"`) {
		t.Fatal("standard-admission-evidence workflow does not declare its evidence-only boundary")
	}
	for _, forbidden := range []string{"contents: write", "gh release create", "git push", "GITHUB_RUN_DISPLAY_TITLE"} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("standard-admission-evidence workflow contains publication authority %q", forbidden)
		}
	}
}
