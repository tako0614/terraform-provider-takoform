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
	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
	"github.com/tako0614/terraform-provider-takoform/standardform"
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
	if len(fixtures) != 10 {
		t.Fatalf("fixture count = %d, want 10", len(fixtures))
	}
	seen := map[string]bool{}
	for _, fixture := range fixtures {
		if seen[fixture.Kind] || fixture.Identity.FormRef.Kind != fixture.Kind || !formpackage.ValidDigest(fixture.Identity.FormRef.SchemaDigest) || !formpackage.ValidDigest(fixture.Identity.PackageDigest) {
			t.Fatalf("invalid exact fixture identity: %#v", fixture)
		}
		seen[fixture.Kind] = true
		if fixture.PositiveName != "canonical" || fixture.NegativeName != "reject-invalid-semantics" || fixture.Positive == nil || fixture.Negative == nil {
			t.Fatalf("invalid retained fixture closure for %s", fixture.Kind)
		}
		if bytes.Equal(mustJSON(t, fixture.Positive), mustJSON(t, fixture.Negative)) {
			t.Fatalf("%s positive and negative fixture unexpectedly match", fixture.Kind)
		}
	}
	for _, kind := range []string{"EdgeWorker", "ObjectBucket", "KVStore", "SQLDatabase", "Queue", "VectorIndex", "DurableWorkflow", "ContainerService", "StatefulActorNamespace", "Schedule"} {
		if !seen[kind] {
			t.Fatalf("published fixture set omits %s", kind)
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
	if len(fixtures) != 10 {
		t.Fatalf("fixture count = %d, want 10", len(fixtures))
	}
	for _, fixture := range fixtures {
		if fixture.Identity.FormRef.Kind != fixture.Kind || fixture.Identity.FormRef.DefinitionVersion != "1.0.1" || !formpackage.ValidDigest(fixture.Identity.FormRef.SchemaDigest) || !formpackage.ValidDigest(fixture.Identity.PackageDigest) {
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
	if len(reports) != 10 {
		t.Fatalf("report count = %d, want 10", len(reports))
	}
	candidates, err := LoadCandidateFixtures(root)
	if err != nil {
		t.Fatal(err)
	}
	exactIdentity := make(map[string]standardform.InstalledFormReference, len(candidates))
	for _, candidate := range candidates {
		exactIdentity[candidate.Kind] = candidate.Identity
	}
	output := filepath.Join(t.TempDir(), "reports")
	if err := Write(root, output, reports); err != nil {
		t.Fatal(err)
	}
	for _, generated := range reports {
		if generated.report.Format != reportFormat || generated.report.Role != providerRole || generated.report.Status != "passed" || !strings.HasPrefix(generated.report.Subject, "provider:registry.opentofu.org/") {
			t.Fatalf("invalid provider-report identity for %s: %#v", generated.kind, generated.report)
		}
		if generated.digest != formpackage.DigestBytes(generated.canonical) {
			t.Fatalf("invalid canonical digest for %s", generated.kind)
		}
		if generated.report.Identity != exactIdentity[generated.kind] || generated.report.Identity.FormRef.DefinitionVersion != "1.0.1" || generated.report.RunnerVersion != "0.1.3" {
			t.Fatalf("report %s relabeled executed candidate identity: %#v", generated.kind, generated.report)
		}
		if _, err := admissionrelease.ValidateCanonicalProviderRunnerReport(generated.canonical, generated.report.Identity, []string{"canonical"}, []string{"reject-invalid-semantics"}); err != nil {
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
	if inventory.Format != directoryInventoryFormat || inventory.Status != "candidate-only" || inventory.ProofType != "provider" || inventory.Source.Commit != sourceCommit || inventory.Source.Repository != "https://github.com/tako0614/terraform-provider-takoform.git" || inventory.DefinitionVersion != "1.0.1" || inventory.PackageVersion != "1.0.1" || inventory.RunnerVersion != "0.1.3" || len(inventory.Reports) != 10 {
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
		"takoform-standard-provider-report-candidate-1.0.1-${{ needs.generate.outputs.source_commit_short }}",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("workflow omits %q", required)
		}
	}
	if strings.Count(workflow, "actions/checkout@") != 1 {
		t.Fatal("the checkout-free signer must not execute repository source")
	}
	signer := strings.Split(workflow, "\n  sign:\n")
	if len(signer) != 2 || strings.Contains(signer[1], "actions/checkout@") || strings.Contains(signer[1], "contents: write") || strings.Contains(signer[1], "contents: read") || strings.Contains(signer[1], "gh release") {
		t.Fatal("signer permissions or mutation boundary drifted")
	}
	for _, forbidden := range []string{"unsignedArtifact:", "sigstoreBundlePath", "sigstoreBundleDigest"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("workflow reintroduced non-canonical signed handoff field %q", forbidden)
		}
	}
	qualityRaw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "quality.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(qualityRaw), `--source-commit "${GITHUB_SHA}"`) {
		t.Fatal("quality workflow does not bind provider reports to its exact source commit")
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
