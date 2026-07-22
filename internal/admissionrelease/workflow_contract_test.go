package admissionrelease

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStandardAdmissionWorkflowClosesCandidateMaterials(t *testing.T) {
	t.Parallel()
	workflow := readStandardAdmissionWorkflow(t)

	required := []string{
		`policyDigest: sha(fs.readFileSync("admission/v1/trust/offline-sigstore-pins.json"))`,
		`candidate.policyDigest !== digestFile("admission/v1/trust/offline-sigstore-pins.json")`,
		`set.entries.length !== 10`,
		`subjects.length !== 41`,
		`packages: [rootPackage, ...packages], files, relationships`,
		`retainedSubjectCount: 41`,
		`...packageDependencies, ...subjectDependencies`,
		`dependencies.length !== 54`,
		`candidate.configDigest !== byName.get("standard-admission-set.json")`,
		`JSON.stringify(candidate.sbomDigests) !== JSON.stringify([byName.get(sbomName)])`,
		`JSON.stringify(candidate.provenanceDigests) !== JSON.stringify([byName.get(provenanceName)])`,
		`JSON.stringify(candidate.artifactDigests) !== JSON.stringify(candidate.releaseAssets.map(({ digest }) => digest))`,
	}
	for _, fragment := range required {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("standard-admission workflow lacks material-closure contract %q", fragment)
		}
	}

	forbidden := []string{
		`policyDigest: sha(fs.readFileSync("admission/v1/trust/registry-readback-policy.json"))`,
		`resolvedDependencies: []`,
		`packages: [{ name: "Takoform standard Form admission", SPDXID: "SPDXRef-Package"`,
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

	// The controller contract remains takos.release-candidate-manifest@v1 with
	// the same exact keys and seven release assets. Only the meaning of the
	// opaque policy digest and the contents of the existing SBOM/provenance
	// assets become stronger.
	required := []string{
		`kind: "takos.release-candidate-manifest@v1", surfaceId: "takoform-standard-form-admission"`,
		`const candidateKeys = ["artifactDigests", "builtAt", "configDigest", "kind", "ociImages", "policyDigest", "provenanceDigests", "releaseAssets", "repository", "sbomDigests", "sourceCommit", "surfaceId", "tag", "toolchainDigest", "version", "workflowRunId"].sort();`,
		`const expectedNames = ["SHA256SUMS", "fresh-provider-lifecycle-matrix.json", "standard-admission-set.json", "standard-admission-set.sigstore.json", "takoform-standard-admission-v1.tar.gz",`,
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
