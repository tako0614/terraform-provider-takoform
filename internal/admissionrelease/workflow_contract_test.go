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
