package admissionrelease

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestStandardAdmissionWorkflowClosesCandidateMaterials(t *testing.T) {
	t.Parallel()
	workflow := readStandardAdmissionWorkflow(t)

	required := []string{
		`release-candidate-contract.json`,
		`go run ./cmd/standard-form-conformance admission-closure-check`,
		`policyDigest: sha(fs.readFileSync("admission/v1/trust/offline-sigstore-pins.json"))`,
		`candidate.policyDigest !== digestFile("admission/v1/trust/offline-sigstore-pins.json")`,
		`set.entries.length !== contract.standardAdmissionSet.entryCount`,
		`subjects.length !== contract.retainedSubjects.count`,
		`packages: [rootPackage, ...packages], files, relationships`,
		`retainedSubjectCount: contract.provenance.retainedSubjectCount`,
		`...packageDependencies, ...subjectDependencies`,
		`dependencies.length !== contract.provenance.resolvedDependencyCount`,
		`candidate.configDigest !== byName.get("standard-admission-set.json")`,
		`JSON.stringify(candidate.sbomDigests) !== JSON.stringify([byName.get(sbomName)])`,
		`JSON.stringify(candidate.provenanceDigests) !== JSON.stringify([byName.get(provenanceName)])`,
		`JSON.stringify(candidate.artifactDigests) !== JSON.stringify(candidate.releaseAssets.map(({ digest }) => digest))`,
		`cmp admission/v1/registry/provider-lifecycle-matrix.json "${fresh}"`,
		`cmp "${output}/fresh-provider-lifecycle-matrix.json" "${RUNNER_TEMP}/archived-provider-lifecycle-matrix.json"`,
		`cmp candidate/fresh-provider-lifecycle-matrix.json "${RUNNER_TEMP}/promoted-archived-provider-lifecycle-matrix.json"`,
		`admissionReleaseVersion: version, definitionVersion: set.definitionVersion, packageVersion: set.packageVersion`,
		`versionInfo: set.packageVersion`,
		`rootPackages[0].versionInfo !== process.env.VERSION`,
		`pkg.versionInfo !== set.packageVersion`,
	}
	for _, fragment := range required {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("standard-admission workflow lacks material-closure contract %q", fragment)
		}
	}

	forbidden := []string{
		`go run ./cmd/standard-form-conformance release-check`,
		`policyDigest: sha(fs.readFileSync("admission/v1/trust/registry-readback-policy.json"))`,
		`resolvedDependencies: []`,
		`packages: [{ name: "Takoform standard Form admission", SPDXID: "SPDXRef-Package"`,
		`set.definitionVersion !== version`,
		`set.packageVersion !== version`,
		`set.definitionVersion !== process.env.VERSION`,
		`set.packageVersion !== process.env.VERSION`,
		`pkg.versionInfo !== process.env.VERSION`,
	}
	for _, fragment := range forbidden {
		if strings.Contains(workflow, fragment) {
			t.Errorf("standard-admission workflow retains incomplete candidate metadata %q", fragment)
		}
	}
}

func TestStandardAdmissionCandidateManifestShapeRemainsControllerCompatible(t *testing.T) {
	t.Parallel()
	workflow := readStandardAdmissionWorkflow(t)

	// The shared machine-readable contract remains
	// takos.release-candidate-manifest@v1 with the same exact keys and seven
	// release assets. Both the workflow and fixed adapter consume byte-identical
	// copies of this fixture.
	required := []string{
		`kind: "takos.release-candidate-manifest@v1", surfaceId: "takoform-standard-form-admission"`,
		`const candidateKeys = [...contract.candidateManifest.keys].sort();`,
		`const expectedNames = contract.candidateManifest.releaseAssets.map(expandVersion).sort();`,
		`JSON.stringify(Object.keys(candidate).sort()) !== JSON.stringify(candidateKeys)`,
	}
	for _, fragment := range required {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("standard-admission candidate/controller compatibility lacks %q", fragment)
		}
	}
}

func TestStandardAdmissionPromotionAuthenticatesAttestationReadback(t *testing.T) {
	t.Parallel()
	workflow := readStandardAdmissionWorkflow(t)

	promote := strings.Index(workflow, "\n  promote:")
	start := strings.Index(workflow, "- name: Verify envelope bindings and every candidate byte")
	end := strings.Index(workflow, "- name: Preflight signed tag and deterministic target")
	if promote < 0 || start <= promote || end <= start {
		t.Fatal("locate promotion candidate-verification step")
	}
	if !strings.Contains(workflow[promote:start], "attestations: read") {
		t.Fatal("promotion job must grant read-only GitHub attestation access")
	}
	step := workflow[start:end]
	if !strings.Contains(step, `GH_TOKEN: ${{ github.token }}`) {
		t.Fatal("promotion candidate-verification step must authenticate GitHub attestation readback")
	}
	if !strings.Contains(step, `gh attestation verify candidate/standard-admission-set.json`) ||
		!strings.Contains(step, `gh attestation verify candidate/takoform-standard-admission-v1.tar.gz`) {
		t.Fatal("promotion candidate-verification step must verify both GitHub attestations")
	}
}

func TestStandardAdmissionPromotionUsesOnlyOneUseControllerAuthorization(t *testing.T) {
	t.Parallel()
	workflow := readStandardAdmissionWorkflow(t)
	promote := strings.Index(workflow, "\n  promote:")
	if promote < 0 {
		t.Fatal("locate admission promotion job")
	}
	promotion := workflow[promote:]
	for _, required := range []string{
		`inputs.authorization_secret_name`,
		`AUTHORIZATION_SECRET_JSON: ${{ secrets[inputs.authorization_secret_name] }}`,
		`takos.release-safety-one-use-authorization@v1`,
		`release/standard-admission-promotion-input-contract.json`,
		`.promotionInputDigest == $promotion_input_digest`,
		`.authorizationDigest == $digest and .releaseId == $release_id and .envelopeDigest == $envelope`,
		`age_seconds <= 300`,
		`remaining_seconds > 0 && remaining_seconds <= 300`,
		`- name: Preflight signed tag and deterministic target`,
	} {
		if !strings.Contains(promotion, required) {
			t.Errorf("standard-admission promotion lacks one-use controller authorization marker %q", required)
		}
	}
	for _, binding := range []string{
		`phase: process.env.PHASE`,
		`release_id: process.env.RELEASE_ID`,
		`version: process.env.VERSION`,
		`tag: process.env.TAG`,
		`source_commit: process.env.SOURCE_COMMIT`,
		`candidate_run_id: process.env.CANDIDATE_RUN_ID`,
		`candidate_manifest_digest: process.env.CANDIDATE_MANIFEST_DIGEST`,
		`envelope_digest: process.env.ENVELOPE_DIGEST`,
		`controller_commit: process.env.CONTROLLER_COMMIT`,
		`controller_digest: process.env.CONTROLLER_DIGEST`,
		`adapter_digest: process.env.ADAPTER_DIGEST`,
		`authorization_digest: process.env.AUTHORIZATION_DIGEST`,
		`authorization_secret_name: process.env.AUTHORIZATION_SECRET_NAME`,
		`artifact_digests_b64: process.env.ARTIFACT_DIGESTS_B64`,
		`health_checks_b64: process.env.HEALTH_CHECKS_B64`,
		`target_fingerprint: process.env.TARGET_FINGERPRINT`,
	} {
		if !strings.Contains(promotion, binding) {
			t.Errorf("standard-admission promotion digest omits exact binding %q", binding)
		}
	}
	for _, forbidden := range []string{
		`secrets.RELEASE_SAFETY_AUTHORIZATION_DIGEST`,
		`RELEASE_SAFETY_RULESET_AUDIT_TOKEN`,
		`RULESET_AUDIT_TOKEN`,
		`/immutable-releases`,
		`/rulesets`,
	} {
		if strings.Contains(promotion, forbidden) {
			t.Errorf("standard-admission promotion retains controller-owned policy audit %q", forbidden)
		}
	}
}

func readStandardAdmissionWorkflow(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve workflow contract test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "standard-admission-release.yml"))
	if err != nil {
		t.Fatalf("read standard-admission workflow: %v", err)
	}
	return string(raw)
}
