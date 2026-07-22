package providerreport

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
