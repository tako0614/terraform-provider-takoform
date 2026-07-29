package admissionrelease

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const standardAdmissionReleaseCandidateContractSHA256 = "249991f2c54b901c664597a03920258dc97eb09dc589b04ebc64b02c3df3425b"
const standardAdmissionPromotionInputContractSHA256 = "bf153d8699236c9d3979f1b0257968d41414cb104d73ea9d41a4e2b7081c7e69"

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

func TestStandardAdmissionPromotionInputContractIsClosedAndDigestSensitive(t *testing.T) {
	t.Parallel()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve workflow contract test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, "release", "standard-admission-promotion-input-contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	if actual := fmt.Sprintf("%x", sha256.Sum256(raw)); actual != standardAdmissionPromotionInputContractSHA256 {
		t.Fatalf("promotion-input contract digest = %s, want %s", actual, standardAdmissionPromotionInputContractSHA256)
	}
	var contract struct {
		Format string   `json:"format"`
		Keys   []string `json:"keys"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	expected := []string{
		"phase", "release_id", "version", "tag", "source_commit", "candidate_run_id",
		"candidate_manifest_digest", "envelope_digest", "controller_commit", "controller_digest",
		"adapter_digest", "authorization_digest", "authorization_secret_name", "artifact_digests_b64",
		"health_checks_b64", "target_fingerprint",
	}
	if contract.Format != "takoform.standard-admission-promotion-input-contract@v1" || fmt.Sprint(contract.Keys) != fmt.Sprint(expected) {
		t.Fatalf("unexpected promotion-input contract: %#v", contract)
	}
	inputs := make(map[string]string, len(expected))
	for index, key := range expected {
		inputs[key] = fmt.Sprintf("value-%d", index)
	}
	digest := func(value map[string]string) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	baseline := digest(inputs)
	for _, key := range expected {
		changed := make(map[string]string, len(inputs))
		for name, value := range inputs {
			changed[name] = value
		}
		changed[key] += "-changed"
		if digest(changed) == baseline {
			t.Fatalf("changing promotion input %s did not change canonical digest", key)
		}
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
