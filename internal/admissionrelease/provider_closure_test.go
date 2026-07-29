package admissionrelease

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

func TestFullProviderReportClosureRejectsTamperedUnselectedReport(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sourceForms := filepath.Join("..", "..", "forms")
	if err := os.MkdirAll(filepath.Join(root, "forms"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(
		filepath.Join(root, "forms", "releases"),
		os.DirFS(filepath.Join(sourceForms, "releases")),
	); err != nil {
		t.Fatal(err)
	}
	inventoryRaw, err := os.ReadFile(filepath.Join(sourceForms, "standard-package-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeRetainedTestFile(t, root, "forms/standard-package-set.json", inventoryRaw)
	var inventory providerClosureInventory
	if err := decodeStrictJSON(inventoryRaw, &inventory); err != nil {
		t.Fatal(err)
	}

	const retainedRoot = "admission/v3"
	const sourceCommit = "0123456789abcdef0123456789abcdef01234567"
	const runnerVersion = "fixture-1.0.1"
	bundleRaw := []byte(`{}`)
	manifest := providerClosureManifest{
		Format: currentProviderManifestFormat, Status: "candidate-only", ProofType: "provider",
		Subject: currentProviderSubject, Generation: "portable-v1", RunnerVersion: runnerVersion,
		Source:  providerClosureSource{Repository: currentProviderRepository, Commit: sourceCommit},
		Reports: make([]providerClosureManifestEntry, 0, len(inventory.Packages)),
	}
	signed := providerClosureSignedManifest{
		Format: currentProviderSignedFormat, Status: "candidate-only", ProofType: "provider",
		Generation: "portable-v1", Subject: currentProviderSubject,
		CertificateIdentity: currentProviderIdentity, Workflow: currentProviderWorkflow,
		WorkflowRunID: "123", WorkflowRunAttempt: 1,
		Source:  providerClosureSource{Repository: currentProviderRepository, Commit: sourceCommit},
		Entries: make([]providerClosureSignedEntry, 0, len(inventory.Packages)),
	}
	closure := ProviderReportClosure{
		Generation:         "portable-v1",
		ManifestPath:       "provider-closure/provider-report-manifest.json",
		SignedManifestPath: "provider-closure/signed-provider-report-candidate.json",
		ChecksumsPath:      "provider-closure/SHA256SUMS",
		Reports:            make([]ProviderReportClosureEntry, 0, len(inventory.Packages)),
	}
	checksums := make(map[string]string, len(inventory.Packages)*2+2)
	for _, item := range inventory.Packages {
		slug := filepath.Base(item.Path)
		identity := standardform.InstalledFormReference{FormRef: item.FormRef, PackageDigest: item.PackageDigest}
		definition, err := readDefinition(filepath.Join(root, "forms", "releases", releaseIDForKind(item.Kind), item.FormRef.DefinitionVersion))
		if err != nil {
			t.Fatal(err)
		}
		positives := make([]string, 0, len(definition.ConformanceFixtures))
		for _, fixture := range definition.ConformanceFixtures {
			positives = append(positives, fixture.Name)
		}
		negatives := make([]string, 0, len(definition.NegativeFixtures))
		for _, fixture := range definition.NegativeFixtures {
			negatives = append(negatives, fixture.Name)
		}
		report := completeRunnerReport(roleProviderReport, currentProviderSubject, identity, positives, negatives)
		report.RunnerVersion = runnerVersion
		reportRaw := canonicalFixture(t, report)
		reportRelative := path.Join("packages", slug, "provider-report.json")
		bundleRelative := path.Join("packages", slug, "provider-report.sigstore.json")
		retainedReport := path.Join("provider-closure", reportRelative)
		retainedBundle := path.Join("provider-closure", bundleRelative)
		writeRetainedTestFile(t, root, path.Join(retainedRoot, retainedReport), reportRaw)
		writeRetainedTestFile(t, root, path.Join(retainedRoot, retainedBundle), bundleRaw)
		reportDigest := formpackage.DigestBytes(reportRaw)
		bundleDigest := formpackage.DigestBytes(bundleRaw)
		manifest.Reports = append(manifest.Reports, providerClosureManifestEntry{
			Kind: item.Kind, Slug: slug, Path: reportRelative, BundlePath: bundleRelative,
			Digest: reportDigest, Identity: identity,
		})
		signed.Entries = append(signed.Entries, providerClosureSignedEntry{
			Kind: item.Kind, Slug: slug, ReportPath: reportRelative, ReportDigest: reportDigest,
			BundlePath: bundleRelative, BundleDigest: bundleDigest,
		})
		closure.Reports = append(closure.Reports, ProviderReportClosureEntry{
			Kind: item.Kind, Slug: slug, Identity: identity,
			ReportPath: retainedReport, ReportDigest: reportDigest, SigstoreBundle: retainedBundle,
		})
		checksums[reportRelative], checksums[bundleRelative] = reportDigest, bundleDigest
	}
	manifestRaw := canonicalFixture(t, manifest)
	signed.Manifest = providerClosureRetainedRef{
		Path: "provider-report-manifest.json", Digest: formpackage.DigestBytes(manifestRaw),
	}
	signedRaw := canonicalFixture(t, signed)
	closure.ManifestDigest = formpackage.DigestBytes(manifestRaw)
	closure.SignedManifestDigest = formpackage.DigestBytes(signedRaw)
	checksums["provider-report-manifest.json"] = closure.ManifestDigest
	checksums["signed-provider-report-candidate.json"] = closure.SignedManifestDigest
	names := make([]string, 0, len(checksums))
	for name := range checksums {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s  %s", strings.TrimPrefix(checksums[name], "sha256:"), name))
	}
	checksumsRaw := []byte(strings.Join(lines, "\n") + "\n")
	closure.ChecksumsDigest = formpackage.DigestBytes(checksumsRaw)
	writeRetainedTestFile(t, root, path.Join(retainedRoot, closure.ManifestPath), manifestRaw)
	writeRetainedTestFile(t, root, path.Join(retainedRoot, closure.SignedManifestPath), signedRaw)
	writeRetainedTestFile(t, root, path.Join(retainedRoot, closure.ChecksumsPath), checksumsRaw)

	selected := make(map[string]struct{}, 10)
	for _, report := range closure.Reports[:10] {
		selected[report.Kind] = struct{}{}
	}
	extra, identity, err := verifyFullProviderReportClosure(root, retainedRoot, Set{ProviderReportClosure: &closure}, selected)
	if err != nil {
		t.Fatal(err)
	}
	if len(extra) != 24 || identity.sourceCommit != sourceCommit || identity.runnerVersion != runnerVersion {
		t.Fatalf("unexpected full provider closure result: extra=%d identity=%#v", len(extra), identity)
	}

	tampered := closure.Reports[10]
	writeRetainedTestFile(t, root, path.Join(retainedRoot, tampered.ReportPath), []byte(`{"tampered":true}`))
	if _, _, err := verifyFullProviderReportClosure(root, retainedRoot, Set{ProviderReportClosure: &closure}, selected); err == nil ||
		!strings.Contains(err.Error(), "closure bytes differ") {
		t.Fatalf("tampered unselected provider report error = %v", err)
	}
}
