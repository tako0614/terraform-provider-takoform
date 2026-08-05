package providerreport

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
)

func TestLoadPublishedFixturesUsesExactRetainedReleaseArchives(t *testing.T) {
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := LoadPublishedFixtures(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != len(standardforms.RetiredKinds) {
		t.Fatalf("fixture count = %d, want %d", len(fixtures), len(standardforms.RetiredKinds))
	}
	seen := map[string]bool{}
	for _, fixture := range fixtures {
		if seen[fixture.Kind] || fixture.Identity.FormRef.Kind != fixture.Kind || !formpackage.ValidDigest(fixture.Identity.FormRef.SchemaDigest) || !formpackage.ValidDigest(fixture.Identity.PackageDigest) {
			t.Fatalf("invalid exact fixture identity: %#v", fixture)
		}
		seen[fixture.Kind] = true
		if fixture.PositiveName != "canonical" || fixture.Positive == nil || len(fixture.Negatives) == 0 {
			t.Fatalf("invalid retained fixture closure for %s", fixture.Kind)
		}
		for _, negative := range fixture.Negatives {
			if negative.Input == nil || negative.Stage == "" || bytes.Equal(mustJSON(t, fixture.Positive), mustJSON(t, negative.Input)) {
				t.Fatalf("%s %s does not differ from the canonical fixture", fixture.Kind, negative.Name)
			}
		}
	}
	for _, spec := range standardforms.RetiredKinds {
		if !seen[spec.Kind] {
			t.Fatalf("retained published fixture set omits %s", spec.Kind)
		}
	}
}

func TestLoadCandidateFixturesUsesExactCurrentReleaseSources(t *testing.T) {
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := LoadCandidateFixtures(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != len(currentformcatalog.Kinds) {
		t.Fatalf("fixture count = %d, want %d", len(fixtures), len(currentformcatalog.Kinds))
	}
	for _, fixture := range fixtures {
		declared, ok := currentformcatalog.ByKind(fixture.Kind)
		if !ok {
			t.Fatalf("undeclared candidate kind %s", fixture.Kind)
		}
		if fixture.Identity.FormRef.Kind != fixture.Kind || fixture.Identity.FormRef.DefinitionVersion != declared.Version() || !formpackage.ValidDigest(fixture.Identity.FormRef.SchemaDigest) || !formpackage.ValidDigest(fixture.Identity.PackageDigest) {
			t.Fatalf("invalid current candidate identity: %#v", fixture)
		}
	}
}

func TestGenerateRunsActualProviderProtocolAndWritesCanonicalPerKindReports(t *testing.T) {
	if testing.Short() {
		t.Skip("actual provider protocol integration")
	}
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Skip("OpenTofu is required for the provider protocol integration test")
	}
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	reports, err := Generate(context.Background(), root, "tofu")
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != len(currentformcatalog.Kinds) {
		t.Fatalf("report count = %d, want %d", len(reports), len(currentformcatalog.Kinds))
	}
	candidates, err := LoadCandidateFixtures(root)
	if err != nil {
		t.Fatal(err)
	}
	exactCandidates := make(map[string]PublishedFixture, len(candidates))
	for _, candidate := range candidates {
		exactCandidates[candidate.Kind] = candidate
	}
	output := filepath.Join(t.TempDir(), "reports")
	if err := Write(root, output, reports); err != nil {
		t.Fatal(err)
	}
	for _, generated := range reports {
		if generated.report.Format != reportFormat || generated.report.Role != providerRole || generated.report.Status != "passed" || generated.report.Subject != "provider:"+providerlifecycle.CanonicalProviderAddress {
			t.Fatalf("invalid provider-report identity for %s: %#v", generated.kind, generated.report)
		}
		if generated.digest != formpackage.DigestBytes(generated.canonical) {
			t.Fatalf("invalid canonical digest for %s", generated.kind)
		}
		declared, ok := currentformcatalog.ByKind(generated.kind)
		if !ok {
			t.Fatalf("report names undeclared kind %s", generated.kind)
		}
		candidate, ok := exactCandidates[generated.kind]
		if !ok {
			t.Fatalf("report names kind absent from the exact candidate package set: %s", generated.kind)
		}
		if generated.report.Identity != candidate.Identity || generated.report.Identity.FormRef.DefinitionVersion != declared.Version() || generated.report.RunnerVersion != providerReleaseVersion(t, root) {
			t.Fatalf("report %s relabeled executed candidate identity: %#v", generated.kind, generated.report)
		}
		if !formpackage.ValidDigest(generated.report.ProviderBinarySHA256) {
			t.Fatalf("report %s does not bind the exact executed provider binary: %#v", generated.kind, generated.report)
		}
		hasObserved := false
		for _, negative := range candidate.Negatives {
			if negative.Stage == "observed" {
				hasObserved = true
				break
			}
		}
		if !hasObserved {
			t.Fatalf("%s exact candidate package declares no observed rejection fixture", generated.kind)
		}
		if _, err := admissionrelease.ValidateCanonicalProviderRunnerReportWithStages(
			generated.canonical,
			candidate.Identity,
			[]string{candidate.PositiveName},
			negativeExpectations(candidate.Negatives),
		); err != nil {
			t.Fatalf("validate %s canonical report: %v", generated.kind, err)
		}
		written, err := os.ReadFile(filepath.Join(output, generated.slug, "provider-report.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(written, generated.canonical) {
			t.Fatalf("written %s provider-report bytes drifted", generated.kind)
		}
	}
	sourceCommit := strings.Repeat("a", 40)
	exportRoot := filepath.Join(t.TempDir(), "provider-report-candidate")
	inventory, err := Export(root, exportRoot, sourceCommit, reports)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Format != directoryInventoryFormat || inventory.Status != "candidate-only" || inventory.ProofType != "provider" || inventory.Source.Commit != sourceCommit || inventory.Source.Repository != "https://github.com/tako0614/terraform-provider-takoform.git" || inventory.Generation == "" || inventory.RunnerVersion != providerReleaseVersion(t, root) || len(inventory.Reports) != len(currentformcatalog.Kinds) {
		t.Fatalf("invalid exported provider-report manifest: %#v", inventory)
	}
	verified, err := VerifyDirectory(root, exportRoot, sourceCommit)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inventory, verified) {
		t.Fatal("verified provider-report manifest differs from export")
	}
	for _, descriptor := range inventory.Reports {
		if descriptor.Path != filepath.ToSlash(filepath.Join("packages", descriptor.Slug, "provider-report.json")) {
			t.Fatalf("non-canonical provider-report path: %#v", descriptor)
		}
		if descriptor.BundlePath != filepath.ToSlash(filepath.Join("packages", descriptor.Slug, "provider-report.sigstore.json")) {
			t.Fatalf("non-canonical provider-report bundle path: %#v", descriptor)
		}
	}
	if _, err := VerifyDirectory(root, exportRoot, strings.Repeat("b", 40)); err == nil || !strings.Contains(err.Error(), "rederived exact report closure") {
		t.Fatalf("source substitution error = %v", err)
	}
	extra := filepath.Join(exportRoot, "unexpected")
	if err := os.WriteFile(extra, []byte("unexpected"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyDirectory(root, exportRoot, sourceCommit); err == nil || !strings.Contains(err.Error(), "file closure differs") {
		t.Fatalf("extra-file closure error = %v", err)
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(exportRoot, filepath.FromSlash(inventory.Reports[0].Path))
	reportRaw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, append(reportRaw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyDirectory(root, exportRoot, sourceCommit); err == nil || !strings.Contains(err.Error(), "provider-report") {
		t.Fatalf("non-canonical report error = %v", err)
	}
	if err := Write(root, filepath.Join(root, "admission", "v1", "unsigned-provider-reports"), reports); err == nil || !strings.Contains(err.Error(), "admission tree") {
		t.Fatalf("admission-tree write error = %v", err)
	}
	symlink := filepath.Join(t.TempDir(), "admission-link")
	if err := os.Symlink(filepath.Join(root, "admission"), symlink); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, symlink, reports); err == nil || !strings.Contains(err.Error(), "admission tree") {
		t.Fatalf("symlinked admission-tree write error = %v", err)
	}
	traversal := append([]GeneratedReport(nil), reports...)
	traversal[0].slug = "../escape"
	if err := Write(root, filepath.Join(t.TempDir(), "traversal"), traversal); err == nil || !strings.Contains(err.Error(), "substituted") {
		t.Fatalf("traversal write error = %v", err)
	}
	forged := append([]GeneratedReport(nil), reports...)
	forged[0].report.Subject = "provider:forged.example.test/example/provider"
	if err := Write(root, filepath.Join(t.TempDir(), "forged"), forged); err == nil || !strings.Contains(err.Error(), "differs from canonical bytes") {
		t.Fatalf("forged report write error = %v", err)
	}
}

func TestStandardProviderReportWorkflowSeparatesExecutionAndSigningAuthority(t *testing.T) {
	t.Parallel()
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "standard-provider-report.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)
	for _, required := range []string{
		"request_id:",
		"required: true",
		"type: string",
		"run-name: ${{ inputs.request_id }}",
		"REQUEST_ID: ${{ inputs.request_id }}",
		`[[ ! "$REQUEST_ID" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]]`,
		"permissions: {}",
		"environment: standard-provider-report",
		"id-token: write",
		"artifact-ids: ${{ needs.generate.outputs.artifact_id }}",
		"digest-mismatch: error",
		"takoform.standard-provider-report-candidate@v1",
		"takoform.standard-provider-report-signed-candidate@v1",
		"status:\"candidate-only\"",
		"proofType:\"provider\"",
		"bundlePath,bundleDigest",
		"provider-report-manifest.json",
		"signed-provider-report-candidate.json",
		"SHA256SUMS",
		"providerBinarySha256",
		"requestId",
		"workflowRunId",
		"workflowRunAttempt:Number(process.env.GITHUB_RUN_ATTEMPT)",
		"headSha",
		"takoform-standard-provider-report-unsigned-current-${REQUEST_ID}-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}-${GITHUB_SHA:0:12}",
		"takoform-standard-provider-report-candidate-current-${{ inputs.request_id }}-${{ github.run_id }}-${{ github.run_attempt }}-${{ needs.generate.outputs.source_commit_short }}",
		`--source-commit "${GITHUB_SHA}"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("workflow omits %q", required)
		}
	}
	if strings.Count(workflow, "actions/checkout@") != 2 {
		t.Fatal("the signer must read the exact checked-out inventory used to derive its report closure")
	}
	signer := strings.Split(workflow, "\n  sign:\n")
	if len(signer) != 2 || !strings.Contains(signer[1], "actions/checkout@") ||
		!strings.Contains(signer[1], "ref: ${{ needs.generate.outputs.source_commit }}") ||
		!strings.Contains(signer[1], "contents: read") ||
		strings.Contains(signer[1], "contents: write") ||
		strings.Contains(signer[1], "go run") ||
		strings.Contains(signer[1], "gh release") {
		t.Fatal("signer permissions or mutation boundary drifted")
	}
	for _, forbidden := range []string{"unsignedArtifact:", "sigstoreBundlePath", "sigstoreBundleDigest"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("workflow reintroduced non-canonical signed handoff field %q", forbidden)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	marshaled, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := formpackage.Canonicalize(marshaled)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// providerReleaseVersion reads the reviewed release descriptor, so a version
// bump does not need this test edited to keep proving the binding.
func providerReleaseVersion(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "release", "version.json"))
	if err != nil {
		t.Fatal(err)
	}
	var descriptor struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		t.Fatal(err)
	}
	if descriptor.Version == "" {
		t.Fatal("release descriptor has no version")
	}
	return descriptor.Version
}
