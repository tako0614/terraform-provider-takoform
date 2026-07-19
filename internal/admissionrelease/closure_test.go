package admissionrelease

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

func TestVerifyAdmissionSetAcceptsCompleteAuthenticatedLocalFixture(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	packagePath := "conformance/form-package-v1/positive/standard/object-bucket"
	sourcePackage := filepath.Join("..", "..", filepath.FromSlash(packagePath))
	if err := os.CopyFS(filepath.Join(root, filepath.FromSlash(packagePath)), os.DirFS(sourcePackage)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "release"), 0o755); err != nil {
		t.Fatal(err)
	}
	versionRaw, err := os.ReadFile(filepath.Join("..", "..", "release", "version.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "release", "version.json"), versionRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	packageRoot := filepath.Join(root, filepath.FromSlash(packagePath))
	packageReport, err := formpackage.VerifyDirectory(packageRoot)
	if err != nil {
		t.Fatal(err)
	}
	identity := standardform.InstalledFormReference{FormRef: packageReport.FormRef, PackageDigest: packageReport.PackageDigest}
	positiveNames := []string{"canonical"}
	negativeNames := []string{"reject-invalid-semantics"}

	host := completeRunnerReport(roleHostReport, "host:https://host.example.test", identity, positiveNames, negativeNames)
	provider := completeRunnerReport(roleProviderReport, "provider:registry.terraform.io/tako0614/takoform", identity, positiveNames, negativeNames)
	hostRaw := writeCanonicalTestJSON(t, root, "admission/v1/packages/object-bucket/host-report.json", host)
	providerRaw := writeCanonicalTestJSON(t, root, "admission/v1/packages/object-bucket/provider-report.json", provider)
	hostDigest := formpackage.DigestBytes(hostRaw)
	providerDigest := formpackage.DigestBytes(providerRaw)

	evidence := completeAdmissionEvidence(identity, host, hostDigest, provider, providerDigest)
	evidenceRaw := writeCanonicalTestJSON(t, root, "admission/v1/packages/object-bucket/evidence.json", evidence)
	writeTestBundlePlaceholders(t, root,
		"admission/v1/packages/object-bucket/evidence.sigstore.json",
		"admission/v1/packages/object-bucket/host-report.sigstore.json",
		"admission/v1/packages/object-bucket/provider-report.sigstore.json",
	)

	releaseID := releaseIDForKind("ObjectBucket")
	releaseDirectory := path.Join("releases", releaseID, "1.0.0")
	base := "takoform-form-" + releaseID + "_1.0.0"
	indexName := base + "_package-index.json"
	bundleName := base + "_package-index.sigstore.json"
	indexRaw, err := os.ReadFile(filepath.Join(packageRoot, formpackage.PackageIndexFilename))
	if err != nil {
		t.Fatal(err)
	}
	indexRaw, err = formpackage.Canonicalize(indexRaw)
	if err != nil {
		t.Fatal(err)
	}
	writeRetainedTestFile(t, filepath.Join(root, "admission", "v1"), path.Join(releaseDirectory, indexName), indexRaw)
	archiveRaw := buildPackageFixtureArchive(t, packageRoot, indexRaw)
	trailingArchive := append(append([]byte(nil), archiveRaw...), []byte("trailing")...)
	if err := verifyPackageArchive(trailingArchive, indexRaw); err == nil || !strings.Contains(err.Error(), "trailing bytes") {
		t.Fatalf("trailing gzip member bytes error = %v", err)
	}
	nondeterministicArchive := buildPackageFixtureArchiveWithMutation(t, packageRoot, indexRaw, func(header *tar.Header) {
		header.Uid = 1000
	})
	if err := verifyPackageArchive(nondeterministicArchive, indexRaw); err == nil || !strings.Contains(err.Error(), "deterministic regular file") {
		t.Fatalf("nondeterministic tar metadata error = %v", err)
	}
	releaseCommit := "0123456789abcdef0123456789abcdef01234567"
	releaseTag := "forms/" + releaseID + "/v1.0.0"
	metadataManifest := packageReleaseManifest{
		Tag: releaseTag, SourceRepository: sourceRepository, SourceCommit: releaseCommit, Workflow: packageReleaseWorkflow,
		PackageVersion: "1.0.0", PackageDigest: packageReport.PackageDigest, FormRef: packageReport.FormRef,
		Canonicalization: "RFC8785", SignedSubject: indexName,
	}
	sbomRaw := buildPackageSBOMFixture(t, packageRoot, indexRaw, metadataManifest)
	metadataAssets := map[string]releaseAsset{
		indexName:        {Name: indexName, MediaType: packageIndexMediaType, Size: int64(len(indexRaw)), Digest: formpackage.DigestBytes(indexRaw)},
		base + ".tar.gz": {Name: base + ".tar.gz", MediaType: "application/gzip", Size: int64(len(archiveRaw)), Digest: formpackage.DigestBytes(archiveRaw)},
	}
	provenanceRaw := buildPackageProvenanceFixture(t, metadataManifest, metadataAssets)
	assets := map[string]struct {
		media string
		raw   []byte
	}{
		bundleName:                       {media: sigstoreBundleMediaTypeV03, raw: []byte(`{"fixture":true}`)},
		base + ".tar.gz":                 {media: "application/gzip", raw: archiveRaw},
		base + "_sbom.spdx.json":         {media: "application/spdx+json", raw: sbomRaw},
		base + "_provenance.intoto.json": {media: "application/vnd.in-toto+json", raw: provenanceRaw},
	}
	releaseAssets := []releaseAsset{{Name: indexName, MediaType: packageIndexMediaType, Size: int64(len(indexRaw)), Digest: formpackage.DigestBytes(indexRaw)}}
	for _, name := range []string{base + ".tar.gz", bundleName, base + "_provenance.intoto.json", base + "_sbom.spdx.json"} {
		asset := assets[name]
		writeRetainedTestFile(t, filepath.Join(root, "admission", "v1"), path.Join(releaseDirectory, name), asset.raw)
		releaseAssets = append(releaseAssets, releaseAsset{Name: name, MediaType: asset.media, Size: int64(len(asset.raw)), Digest: formpackage.DigestBytes(asset.raw)})
	}
	manifest := packageReleaseManifest{
		SchemaVersion: packageReleaseSchema, ReleaseType: packageReleaseType, Tag: releaseTag,
		SourceRepository: sourceRepository, SourceCommit: releaseCommit, Workflow: packageReleaseWorkflow,
		PackageVersion: "1.0.0", ReleaseID: releaseID, PackageDigest: packageReport.PackageDigest, FormRef: packageReport.FormRef,
		Canonicalization: "RFC8785", SignedSubject: indexName, SignatureBundle: bundleName, SignatureMediaType: sigstoreBundleMediaTypeV03,
		PublisherPolicy: releasePublisherPolicy{
			OIDCIssuer: "https://token.actions.githubusercontent.com",
			Identity:   "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/heads/main",
			TagPattern: "refs/tags/forms/k-*/v*",
		},
		Assets: releaseAssets, PublicationReady: true, PublicationBlockers: []string{},
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeRetainedTestFile(t, filepath.Join(root, "admission", "v1"), path.Join(releaseDirectory, "release-manifest.json"), manifestRaw)

	registryRef, registryRaw := writeRegistryFixture(t, root, versionRaw, releaseCommit)
	set := Set{
		Format: setFormat, DefinitionVersion: "1.0.0", PackageVersion: "1.0.0", AdmissionReleaseTag: "forms/admissions/v1.0.0",
		ProviderRegistryReadback: registryRef,
		Entries: []SetEntry{{
			Kind: "ObjectBucket", Slug: "object-bucket", FormRef: packageReport.FormRef, PackageDigest: packageReport.PackageDigest,
			ReleaseTag: "forms/" + releaseID + "/v1.0.0", ReleaseCommit: releaseCommit,
			PackageReleaseManifestPath: path.Join(releaseDirectory, "release-manifest.json"), PackageReleaseManifestDigest: formpackage.DigestBytes(manifestRaw),
			PackageIndexPath: path.Join(releaseDirectory, indexName), PackageIndexSigstoreBundle: path.Join(releaseDirectory, bundleName),
			EvidencePath: "packages/object-bucket/evidence.json", EvidenceDigest: formpackage.DigestBytes(evidenceRaw),
			HostReportPath: "packages/object-bucket/host-report.json", HostReportDigest: hostDigest,
			HostReportSigstoreBundle: "packages/object-bucket/host-report.sigstore.json",
			ProviderReportPath:       "packages/object-bucket/provider-report.json", ProviderReportDigest: providerDigest,
			ProviderReportSigstoreBundle: "packages/object-bucket/provider-report.sigstore.json",
			AdmissionStatus:              "portable-standard",
		}},
	}
	writeRetainedTestJSON(t, root, setManifestPath, set)
	candidates := CandidateSet{DefinitionVersion: "1.0.0", PackageVersion: "1.0.0", Entries: []Candidate{{
		Kind: "ObjectBucket", Slug: "object-bucket", PackagePath: packagePath, FormRef: packageReport.FormRef, PackageDigest: packageReport.PackageDigest,
	}}}
	verifier := &recordingSubjectVerifier{}
	refVerifier := &recordingReleaseRefVerifier{}
	if err := verifyAdmissionSet(root, candidates, verifier, refVerifier); err != nil {
		t.Fatalf("complete authenticated local fixture: %v", err)
	}
	if verifier.subjectCount != 5 || !refVerifier.called || formpackage.DigestBytes(registryRaw) != registryRef.Digest {
		t.Fatalf("authenticated closure = %d subjects, registry digest %s", verifier.subjectCount, registryRef.Digest)
	}

	for name, candidate := range map[string][]byte{
		"sbom-noncanonical": append(append([]byte(nil), sbomRaw...), '\n'),
		"sbom-duplicate":    []byte(`{"spdxVersion":"SPDX-2.3","spdxVersion":"SPDX-2.3"}`),
		"sbom-unknown":      mutateCanonicalFixture(t, sbomRaw, func(value map[string]any) { value["unexpected"] = true }),
		"sbom-omission":     mutateCanonicalFixture(t, sbomRaw, func(value map[string]any) { delete(value, "files") }),
		"sbom-tamper":       mutateCanonicalFixture(t, sbomRaw, func(value map[string]any) { value["name"] = "different package" }),
	} {
		if err := verifyPackageSBOM(candidate, indexRaw, packageRoot, metadataManifest); err == nil {
			t.Errorf("%s unexpectedly passed", name)
		}
	}
	for name, candidate := range map[string][]byte{
		"provenance-noncanonical": append(append([]byte(nil), provenanceRaw...), '\n'),
		"provenance-duplicate":    []byte(`{"_type":"https://in-toto.io/Statement/v1","_type":"https://in-toto.io/Statement/v1"}`),
		"provenance-unknown":      mutateCanonicalFixture(t, provenanceRaw, func(value map[string]any) { value["unexpected"] = true }),
		"provenance-omission":     mutateCanonicalFixture(t, provenanceRaw, func(value map[string]any) { delete(value, "subject") }),
		"provenance-tamper": mutateCanonicalFixture(t, provenanceRaw, func(value map[string]any) {
			value["predicate"].(map[string]any)["buildDefinition"].(map[string]any)["resolvedDependencies"].([]any)[0].(map[string]any)["digest"].(map[string]any)["gitCommit"] = strings.Repeat("f", 40)
		}),
	} {
		if err := verifyPackageProvenance(candidate, metadataManifest, metadataAssets); err == nil {
			t.Errorf("%s unexpectedly passed", name)
		}
	}
}

func completeRunnerReport(role, subject string, identity standardform.InstalledFormReference, positives, negatives []string) RunnerReport {
	positiveResults := make([]PositiveFixtureResult, 0, len(positives))
	for _, name := range positives {
		positiveResults = append(positiveResults, PositiveFixtureResult{Name: name, Passed: true})
	}
	negativeResults := make([]NegativeFixtureResult, 0, len(negatives))
	for _, name := range negatives {
		negativeResults = append(negativeResults, NegativeFixtureResult{Name: name, ErrorCode: standardform.InvalidArgumentErrorCode, Passed: true})
	}
	return RunnerReport{
		Format: runnerReportFormat, Role: role, Subject: subject, RunnerVersion: "fixture-1.0.0", Identity: identity, Status: "passed",
		Lifecycle:        standardform.LifecycleAudit{Create: true, Read: true, Update: true, Delete: true, Import: true, Observe: true, Refresh: true, Drift: true},
		PositiveFixtures: positiveResults, NegativeFixtures: negativeResults,
	}
}

func completeAdmissionEvidence(identity standardform.InstalledFormReference, host RunnerReport, hostDigest string, provider RunnerReport, providerDigest string) standardform.AdmissionEvidence {
	proof := func(report RunnerReport, digest string) standardform.ConformanceProof {
		positive := make([]string, 0, len(report.PositiveFixtures))
		for _, fixture := range report.PositiveFixtures {
			positive = append(positive, fixture.Name)
		}
		negative := make([]string, 0, len(report.NegativeFixtures))
		for _, fixture := range report.NegativeFixtures {
			negative = append(negative, fixture.Name)
		}
		return standardform.ConformanceProof{
			Subject: report.Subject, RunnerVersion: report.RunnerVersion, Identity: report.Identity, Status: report.Status,
			PositiveFixtures: positive, NegativeFixtures: negative, EvidenceDigest: digest,
		}
	}
	return standardform.AdmissionEvidence{
		APIVersion: standardform.APIVersion, Identity: identity, Classification: "portable-standard", ApprovedSchemaDigest: identity.FormRef.SchemaDigest,
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
		Conformance: standardform.Conformance{Host: proof(host, hostDigest), Provider: proof(provider, providerDigest)},
	}
}

func writeRegistryFixture(t *testing.T, root string, versionRaw []byte, releaseCommit string) (RegistryReadbackRef, []byte) {
	t.Helper()
	var version struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(versionRaw, &version); err != nil {
		t.Fatal(err)
	}
	requirements, descriptorDigest, err := providerlifecycle.LoadCLIMatrix(root)
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + fmt.Sprintf("%064d", 1)
	checks := providerlifecycle.CheckEvidence{Create: true, Read: true, Update: true, Observe: true, Refresh: true, NativeImport: true, CLIImport: true, Delete: true, DriftState: true, NameReplace: true}
	resourceIdentities := []struct{ kind, resourceType string }{
		{"EdgeWorker", "takoform_edge_worker"}, {"ObjectBucket", "takoform_object_bucket"},
		{"KVStore", "takoform_kv_store"}, {"Queue", "takoform_queue"}, {"SQLDatabase", "takoform_sql_database"},
		{"ContainerService", "takoform_container_service"}, {"VectorIndex", "takoform_vector_index"},
		{"DurableWorkflow", "takoform_durable_workflow"}, {"StatefulActorNamespace", "takoform_stateful_actor_namespace"},
		{"Schedule", "takoform_schedule"},
	}
	resources := make([]providerlifecycle.ResourceEvidence, 0, len(resourceIdentities))
	immutable := make([]providerlifecycle.ImmutableReplaceEvidence, 0, len(resourceIdentities)+2)
	for _, identity := range resourceIdentities {
		resources = append(resources, providerlifecycle.ResourceEvidence{Kind: identity.kind, ResourceType: identity.resourceType, Checks: checks})
		immutable = append(immutable, providerlifecycle.ImmutableReplaceEvidence{Kind: identity.kind, Field: "/name", Passed: true})
	}
	immutable = append(immutable,
		providerlifecycle.ImmutableReplaceEvidence{Kind: "SQLDatabase", Field: "/engine", Passed: true},
		providerlifecycle.ImmutableReplaceEvidence{Kind: "VectorIndex", Field: "/dimensions", Passed: true},
	)
	providerBinary := providerlifecycle.ProviderBinaryIdentity{Version: version.Version, SHA256: digest}
	reports := make([]providerlifecycle.Report, 0, len(requirements))
	for _, requirement := range requirements {
		report := providerlifecycle.Report{
			Format: providerlifecycle.ReportFormat, Classification: "generic-lifecycle-candidate", PublicationReady: false,
			BindingStatus: "exact-structural-candidate-set", RunnerSubject: providerlifecycle.RunnerSubject,
			Protocol:           "Terraform provider protocol v6 + versioned Form host HTTP",
			InstallationSource: providerlifecycle.DirectRegistryInstall,
			CandidateSetSHA256: providerlifecycle.CandidateSetSHA256(), ProviderSchemaSHA256: digest,
			ProviderBinary: providerBinary,
			CLI:            providerlifecycle.CLIIdentity{Product: requirement.Product, Version: requirement.Version, ProviderAddress: requirement.ProviderAddress, ExecutableName: requirement.Product, ExecutableSHA256: digest},
			Resources:      resources,
			NegativeChecks: []providerlifecycle.NegativeEvidence{
				{Name: "response-name-substitution-rejected", Kind: "ObjectBucket", Fixture: "name substitution", Passed: true},
				{Name: "response-package-digest-substitution-rejected", Kind: "KVStore", Fixture: "package substitution", Passed: true},
			},
			ImmutableReplace: immutable,
		}
		reports = append(reports, report)
	}
	matrix := providerlifecycle.MatrixReport{
		Format: providerlifecycle.MatrixReportFormat, Classification: "supported-cli-fqn-candidate-matrix", PublicationReady: false,
		ReleaseDescriptorSHA256: descriptorDigest, CandidateSetSHA256: providerlifecycle.CandidateSetSHA256(), ProviderSchemaSHA256: digest,
		InstallationSource: providerlifecycle.DirectRegistryInstall, Reports: reports,
	}
	matrixRaw, err := json.Marshal(matrix)
	if err != nil {
		t.Fatal(err)
	}
	matrixPath := "registry/provider-lifecycle-matrix.json"
	admissionRoot := filepath.Join(root, "admission", "v1")
	writeRetainedTestFile(t, admissionRoot, matrixPath, matrixRaw)
	readback, readbackRaw, err := BuildRegistryReadback(root, filepath.Join(admissionRoot, filepath.FromSlash(matrixPath)), releaseCommit)
	if err != nil {
		t.Fatal(err)
	}
	if !readback.PublicationReady || len(readback.Installs) != 2 || readback.LifecycleMatrixDigest != formpackage.DigestBytes(matrixRaw) {
		t.Fatalf("deterministic Registry readback = %#v", readback)
	}
	writeRetainedTestFile(t, root, "admission/v1/registry/provider-readback.json", readbackRaw)
	writeTestBundlePlaceholders(t, root, "admission/v1/registry/provider-readback.sigstore.json")
	return RegistryReadbackRef{Path: "registry/provider-readback.json", Digest: formpackage.DigestBytes(readbackRaw), SigstoreBundle: "registry/provider-readback.sigstore.json"}, readbackRaw
}

type recordingSubjectVerifier struct {
	subjectCount int
}

type recordingReleaseRefVerifier struct {
	called bool
}

func (v *recordingReleaseRefVerifier) VerifyReleaseRefs(_ string, _ Set, _ ProviderRegistryReadback) error {
	v.called = true
	return nil
}

func (v *recordingSubjectVerifier) VerifyRetainedSubjects(admissionRoot string, _ Set, subjects []RetainedSubject) error {
	for _, subject := range subjects {
		if len(subject.Canonical) == 0 {
			return fmt.Errorf("empty subject")
		}
		if _, err := readRetainedRelativeFile(admissionRoot, subject.SigstorePath, maxSigstoreBundleBytes); err != nil {
			return err
		}
	}
	v.subjectCount = len(subjects)
	return nil
}

func writeCanonicalTestJSON(t *testing.T, root, relative string, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	writeRetainedTestFile(t, root, relative, canonical)
	return canonical
}

func canonicalFixture(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func mutateCanonicalFixture(t *testing.T, raw []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	mutate(value)
	return canonicalFixture(t, value)
}

func buildPackageSBOMFixture(t *testing.T, packageRoot string, indexRaw []byte, manifest packageReleaseManifest) []byte {
	t.Helper()
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		t.Fatal(err)
	}
	code, err := packageVerificationCode(packageRoot, indexRaw, index)
	if err != nil {
		t.Fatal(err)
	}
	files := make([]spdxFile, 0, len(index.Files)+1)
	appendFile := func(name, digest string) {
		digest = strings.TrimPrefix(digest, "sha256:")
		files = append(files, spdxFile{
			FileName: "./" + name, SPDXID: "SPDXRef-File-" + releaseSPDXID(name) + "-" + digest[:12],
			Checksums:        []spdxChecksum{{Algorithm: "SHA256", ChecksumValue: digest}},
			LicenseConcluded: "NOASSERTION", LicenseInfoInFiles: []string{"NOASSERTION"}, CopyrightText: "NOASSERTION",
		})
	}
	appendFile(formpackage.PackageIndexFilename, formpackage.DigestBytes(indexRaw))
	for _, file := range index.Files {
		appendFile(file.Path, file.Digest)
	}
	return canonicalFixture(t, packageSBOM{
		SPDXVersion: "SPDX-2.3", DataLicense: "CC0-1.0", SPDXID: "SPDXRef-DOCUMENT",
		Name:              "Takoform Form Package " + manifest.FormRef.Kind + " " + manifest.PackageVersion,
		DocumentNamespace: "https://forms.takoform.com/spdx/package/" + strings.TrimPrefix(manifest.PackageDigest, "sha256:"),
		CreationInfo:      spdxCreationInfo{Creators: []string{"Tool: takoform-form-package-release"}, Created: "2026-07-19T00:00:00Z"},
		Packages: []spdxPackage{{
			Name: manifest.FormRef.Kind, SPDXID: "SPDXRef-Package", VersionInfo: manifest.PackageVersion,
			DownloadLocation: "NOASSERTION", FilesAnalyzed: true,
			PackageVerificationCode: spdxPackageVerificationCode{Value: code},
			LicenseConcluded:        "NOASSERTION", LicenseDeclared: "NOASSERTION", CopyrightText: "NOASSERTION",
		}},
		Files: files,
		Relationships: []spdxRelationship{{
			SPDXElementID: "SPDXRef-DOCUMENT", RelationshipType: "DESCRIBES", RelatedSPDXElement: "SPDXRef-Package",
		}},
	})
}

func buildPackageProvenanceFixture(t *testing.T, manifest packageReleaseManifest, assets map[string]releaseAsset) []byte {
	t.Helper()
	archiveName := strings.TrimSuffix(manifest.SignedSubject, "_package-index.json") + ".tar.gz"
	subjects := make([]provenanceSubject, 0, 2)
	for _, name := range []string{manifest.SignedSubject, archiveName} {
		asset := assets[name]
		subjects = append(subjects, provenanceSubject{Name: name, Digest: map[string]string{"sha256": strings.TrimPrefix(asset.Digest, "sha256:")}})
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].Name < subjects[j].Name })
	return canonicalFixture(t, packageProvenance{
		Type: "https://in-toto.io/Statement/v1", Subject: subjects, PredicateType: "https://slsa.dev/provenance/v1",
		Predicate: provenancePredicate{
			BuildDefinition: provenanceBuildDefinition{
				BuildType:          "https://forms.takoform.com/buildtypes/data-release/v1",
				ExternalParameters: map[string]string{"tag": manifest.Tag},
				InternalParameters: map[string]string{"canonicalization": "RFC8785"},
				ResolvedDependencies: []provenanceDependency{{
					URI: "git+https://" + manifest.SourceRepository, Digest: map[string]string{"gitCommit": manifest.SourceCommit},
				}},
			},
			RunDetails: provenanceRunDetails{Builder: provenanceBuilder{ID: "https://" + manifest.SourceRepository + "/" + manifest.Workflow}},
		},
	})
}

func writeTestBundlePlaceholders(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, relative := range paths {
		writeRetainedTestFile(t, root, relative, []byte(`{"fixture":"authenticated-by-test-verifier"}`))
	}
}

func buildPackageFixtureArchive(t *testing.T, packageRoot string, indexRaw []byte) []byte {
	return buildPackageFixtureArchiveWithMutation(t, packageRoot, indexRaw, nil)
}

func buildPackageFixtureArchiveWithMutation(t *testing.T, packageRoot string, indexRaw []byte, mutate func(*tar.Header)) []byte {
	t.Helper()
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&output, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	write := func(name string, raw []byte) {
		t.Helper()
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(raw)), ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(), Format: tar.FormatPAX}
		if mutate != nil {
			mutate(header)
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	write(formpackage.PackageIndexFilename, indexRaw)
	files := append([]formpackage.PackageFile(nil), index.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		raw, err := os.ReadFile(filepath.Join(packageRoot, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatal(err)
		}
		write(file.Path, raw)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
