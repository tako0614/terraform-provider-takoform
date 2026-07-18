package admissionrelease

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

const (
	testSchemaDigest   = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	testPackageDigest  = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	testEvidenceDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
)

func TestVerifyAdmissionSetRejectsMissingManifest(t *testing.T) {
	t.Parallel()
	err := VerifyAdmissionSet(t.TempDir(), testCandidates())
	if err == nil || !strings.Contains(err.Error(), "missing admission/v1/standard-admission-set.json") {
		t.Fatalf("missing manifest error = %v", err)
	}
}

func TestVerifyAdmissionSetRejectsUnknownManifestField(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	set := testSet()
	raw, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw[:len(raw)-1], []byte(`,"unreviewed":true}`)...)
	writeRetainedTestFile(t, root, setManifestPath, raw)

	err = VerifyAdmissionSet(root, testCandidates())
	if err == nil || !strings.Contains(err.Error(), `unknown field "unreviewed"`) {
		t.Fatalf("unknown-field error = %v", err)
	}
}

func TestVerifyAdmissionSetRejectsCandidateIdentityMismatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	set := testSet()
	set.Entries[0].PackageDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	writeRetainedTestJSON(t, root, setManifestPath, set)

	err := VerifyAdmissionSet(root, testCandidates())
	if err == nil || !strings.Contains(err.Error(), "retained set identity does not match the compiled candidate") {
		t.Fatalf("candidate mismatch error = %v", err)
	}
}

func TestVerifyAdmissionSetRejectsRetainedEvidenceDigestMismatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	set := testSet()
	writeRetainedTestJSON(t, root, setManifestPath, set)
	writeRetainedTestFile(t, root, admissionRootPath+"/"+set.Entries[0].EvidencePath, []byte(`{}`))

	err := VerifyAdmissionSet(root, testCandidates())
	if err == nil || !strings.Contains(err.Error(), "retained evidence digest mismatch") {
		t.Fatalf("evidence mismatch error = %v", err)
	}
}

func TestVerifyAdmissionSetRejectsStructurallyValidEvidenceWithoutReportClosure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	packagePath := "conformance/form-package-v1/positive/standard/object-bucket"
	source := filepath.Join("..", "..", filepath.FromSlash(packagePath))
	if err := os.CopyFS(filepath.Join(root, filepath.FromSlash(packagePath)), os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	report, err := formpackage.VerifyDirectory(filepath.Join(root, filepath.FromSlash(packagePath)))
	if err != nil {
		t.Fatal(err)
	}
	identity := standardform.InstalledFormReference{FormRef: report.FormRef, PackageDigest: report.PackageDigest}
	proof := standardform.ConformanceProof{
		Subject: "host:https://example.invalid", RunnerVersion: "1.0.0", Identity: identity, Status: "passed",
		PositiveFixtures: []string{"canonical"}, NegativeFixtures: []string{"reject-invalid-semantics"}, EvidenceDigest: testEvidenceDigest,
	}
	evidence := standardform.AdmissionEvidence{
		APIVersion: standardform.APIVersion, Identity: identity, Classification: "portable-standard", ApprovedSchemaDigest: report.FormRef.SchemaDigest,
		Audit: standardform.Audit{
			Lifecycle:    standardform.LifecycleAudit{Create: true, Read: true, Update: true, Delete: true, Import: true, Observe: true, Refresh: true, Drift: true},
			Immutability: standardform.ImmutabilityAudit{Reviewed: true, Fields: []string{"/name"}},
			Security:     standardform.SecurityAudit{SecretFreeDesiredState: true, CredentialBoundaryExternal: true, DataOnlyPackage: true},
			Interfaces:   standardform.InterfaceAudit{Reviewed: true, BindingAuthorityExternal: true, SecretFreeDocuments: true},
		},
		Fixtures: standardform.Fixtures{
			Positive: []standardform.PositiveFixture{{Name: "canonical", Desired: map[string]any{}, Observed: map[string]any{}, Output: map[string]any{}}},
			Negative: []standardform.NegativeFixture{{Name: "reject-invalid-semantics", Stage: "desired", Input: map[string]any{}, ExpectedErrorCode: standardform.InvalidArgumentErrorCode}},
		},
		Conformance: standardform.Conformance{Host: proof, Provider: proof},
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}

	candidates := CandidateSet{DefinitionVersion: "1.0.0", PackageVersion: "1.0.0", Entries: []Candidate{{
		Kind: "ObjectBucket", Slug: "object-bucket", PackagePath: packagePath, FormRef: report.FormRef, PackageDigest: report.PackageDigest,
	}}}
	set := testSet()
	set.Entries[0].FormRef = report.FormRef
	set.Entries[0].PackageDigest = report.PackageDigest
	set.Entries[0].EvidenceDigest = formpackage.DigestBytes(canonical)
	writeRetainedTestJSON(t, root, setManifestPath, set)
	writeRetainedTestFile(t, root, admissionRootPath+"/"+set.Entries[0].EvidencePath, canonical)

	err = VerifyAdmissionSet(root, candidates)
	if err == nil || !strings.Contains(err.Error(), "host-report.json") {
		t.Fatalf("missing report closure error = %v", err)
	}
}

func testCandidates() CandidateSet {
	return CandidateSet{
		DefinitionVersion: "1.0.0",
		PackageVersion:    "1.0.0",
		Entries: []Candidate{{
			Kind: "ObjectBucket", Slug: "object-bucket", PackagePath: "conformance/form-package-v1/positive/standard/object-bucket",
			FormRef: formpackage.FormRef{
				APIVersion: formpackage.FormAPIVersion, Kind: "ObjectBucket", DefinitionVersion: "1.0.0", SchemaDigest: testSchemaDigest,
			},
			PackageDigest: testPackageDigest,
		}},
	}
}

func testSet() Set {
	return Set{
		Format: setFormat, DefinitionVersion: "1.0.0", PackageVersion: "1.0.0", AdmissionReleaseTag: "forms/admissions/v1.0.0",
		ProviderRegistryReadback: RegistryReadbackRef{
			Path: "registry/provider-readback.json", Digest: testEvidenceDigest,
			SigstoreBundle: "registry/provider-readback.sigstore.json",
		},
		Entries: []SetEntry{{
			Kind: "ObjectBucket", Slug: "object-bucket",
			FormRef: formpackage.FormRef{
				APIVersion: formpackage.FormAPIVersion, Kind: "ObjectBucket", DefinitionVersion: "1.0.0", SchemaDigest: testSchemaDigest,
			},
			PackageDigest: testPackageDigest,
			ReleaseTag:    "forms/" + releaseIDForKind("ObjectBucket") + "/v1.0.0", ReleaseCommit: "0123456789abcdef0123456789abcdef01234567",
			PackageReleaseManifestPath:   "releases/k-j5rguzldorbhky3lmv2a/1.0.0/release-manifest.json",
			PackageReleaseManifestDigest: testEvidenceDigest,
			PackageIndexPath:             "releases/k-j5rguzldorbhky3lmv2a/1.0.0/takoform-form-k-j5rguzldorbhky3lmv2a_1.0.0_package-index.json",
			PackageIndexSigstoreBundle:   "releases/k-j5rguzldorbhky3lmv2a/1.0.0/takoform-form-k-j5rguzldorbhky3lmv2a_1.0.0_package-index.sigstore.json",
			EvidencePath:                 "packages/object-bucket/evidence.json", EvidenceDigest: testEvidenceDigest,
			HostReportPath: "packages/object-bucket/host-report.json", HostReportDigest: testEvidenceDigest,
			HostReportSigstoreBundle: "packages/object-bucket/host-report.sigstore.json",
			ProviderReportPath:       "packages/object-bucket/provider-report.json", ProviderReportDigest: testEvidenceDigest,
			ProviderReportSigstoreBundle: "packages/object-bucket/provider-report.sigstore.json",
			AdmissionStatus:              "portable-standard",
		}},
	}
}

func writeRetainedTestJSON(t *testing.T, root, relative string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeRetainedTestFile(t, root, relative, raw)
}

func writeRetainedTestFile(t *testing.T, root, relative string, raw []byte) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
