package admissionrelease

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

func TestValidateCanonicalHostRunnerReportBindsExactFixtureBytes(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	identity := standardform.InstalledFormReference{
		FormRef: formpackage.FormRef{
			APIVersion: formpackage.FormAPIVersion, Kind: "ObjectBucket",
			DefinitionVersion: "1.0.1", SchemaDigest: digest,
		},
		PackageDigest: digest,
	}
	positive := FixtureDigestBinding{PackageFixtureDigest: digest, EffectiveInputDigest: digest}
	negative := FixtureDigestBinding{PackageFixtureDigest: digest, EffectiveInputDigest: digest}
	report := completeRunnerReport(
		roleHostReport,
		"host:https://host.example.test",
		identity,
		[]string{"canonical"},
		[]string{"reject-invalid-semantics"},
		fixtureDigestBinding(positive),
		fixtureDigestBinding(negative),
	)
	raw := canonicalFixture(t, report)
	if _, err := ValidateCanonicalHostRunnerReport(
		raw,
		identity,
		map[string]FixtureDigestBinding{"canonical": positive},
		map[string]FixtureDigestBinding{"reject-invalid-semantics": negative},
	); err != nil {
		t.Fatalf("valid report: %v", err)
	}

	if _, err := ValidateCanonicalHostRunnerReport(
		append(append([]byte(nil), raw...), '\n'),
		identity,
		map[string]FixtureDigestBinding{"canonical": positive},
		map[string]FixtureDigestBinding{"reject-invalid-semantics": negative},
	); err == nil || !strings.Contains(err.Error(), "RFC 8785 canonical") {
		t.Fatalf("noncanonical report error = %v", err)
	}

	wrong := positive
	wrong.EffectiveInputDigest = "sha256:" + strings.Repeat("b", 64)
	if _, err := ValidateCanonicalHostRunnerReport(
		raw,
		identity,
		map[string]FixtureDigestBinding{"canonical": wrong},
		map[string]FixtureDigestBinding{"reject-invalid-semantics": negative},
	); err == nil || !strings.Contains(err.Error(), "exact package and effective input bytes") {
		t.Fatalf("fixture substitution error = %v", err)
	}
}

func TestValidateCanonicalHostRunnerReportUsesDesiredNegativeFixturesOnly(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	identity := standardform.InstalledFormReference{
		FormRef: formpackage.FormRef{
			APIVersion: formpackage.FormAPIVersion, Kind: "ObjectBucket",
			DefinitionVersion: "3.0.0", SchemaDigest: digest,
		},
		PackageDigest: digest,
	}
	positive := FixtureDigestBinding{PackageFixtureDigest: digest, EffectiveInputDigest: digest}
	desired := FixtureDigestBinding{
		PackageFixtureDigest: digest, EffectiveInputDigest: digest, Stage: "desired",
	}
	observed := FixtureDigestBinding{
		PackageFixtureDigest: digest, EffectiveInputDigest: digest, Stage: "observed",
	}
	report := completeRunnerReport(
		roleHostReport,
		"host:https://host.example.test",
		identity,
		[]string{"canonical"},
		[]string{"reject-desired"},
		fixtureDigestBinding(positive),
		fixtureDigestBinding(desired),
	)
	raw := canonicalFixture(t, report)
	if _, err := ValidateCanonicalHostRunnerReport(
		raw,
		identity,
		map[string]FixtureDigestBinding{"canonical": positive},
		map[string]FixtureDigestBinding{"reject-desired": desired, "reject-observed": observed},
	); err != nil {
		t.Fatalf("desired-only host report: %v", err)
	}

	overclaim := completeRunnerReport(
		roleHostReport,
		"host:https://host.example.test",
		identity,
		[]string{"canonical"},
		[]string{"reject-desired", "reject-observed"},
		fixtureDigestBinding(positive),
		fixtureDigestBinding(desired),
		fixtureDigestBinding(observed),
	)
	if _, err := ValidateCanonicalHostRunnerReport(
		canonicalFixture(t, overclaim),
		identity,
		map[string]FixtureDigestBinding{"canonical": positive},
		map[string]FixtureDigestBinding{"reject-desired": desired, "reject-observed": observed},
	); err == nil || !strings.Contains(err.Error(), "reject-observed") {
		t.Fatalf("host observed-stage overclaim error = %v", err)
	}

	for _, stage := range []string{"output", "host-private", " observed "} {
		t.Run(stage, func(t *testing.T) {
			invalid := observed
			invalid.Stage = stage
			if _, err := ValidateCanonicalHostRunnerReport(
				raw,
				identity,
				map[string]FixtureDigestBinding{"canonical": positive},
				map[string]FixtureDigestBinding{"reject-desired": desired, "reject-invalid": invalid},
			); err == nil || !strings.Contains(err.Error(), "not supported") {
				t.Fatalf("stage %q error = %v", stage, err)
			}
		})
	}
	emptyNegativeReport := completeRunnerReport(
		roleHostReport,
		"host:https://host.example.test",
		identity,
		[]string{"canonical"},
		nil,
		fixtureDigestBinding(positive),
	)
	if _, err := ValidateCanonicalHostRunnerReport(
		canonicalFixture(t, emptyNegativeReport),
		identity,
		map[string]FixtureDigestBinding{"canonical": positive},
		map[string]FixtureDigestBinding{"reject-observed": observed},
	); err == nil || !strings.Contains(err.Error(), "negative fixture expectations are required") {
		t.Fatalf("observed-only host binding error = %v", err)
	}
}

func TestValidateCanonicalProviderRunnerReportWithStagesCoversDesiredAndObserved(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	identity := standardform.InstalledFormReference{
		FormRef: formpackage.FormRef{
			APIVersion: formpackage.FormAPIVersion, Kind: "ObjectBucket",
			DefinitionVersion: "3.0.0", SchemaDigest: digest,
		},
		PackageDigest: digest,
	}
	report := completeRunnerReport(
		roleProviderReport,
		"provider:registry.terraform.io/tako0614/takoform",
		identity,
		[]string{"canonical"},
		[]string{"reject-desired", "reject-observed"},
	)
	raw := canonicalFixture(t, report)
	expectations := []NegativeFixtureExpectation{
		{Name: "reject-desired", Stage: "desired"},
		{Name: "reject-observed", Stage: "observed"},
	}
	if _, err := ValidateCanonicalProviderRunnerReportWithStages(raw, identity, []string{"canonical"}, expectations); err != nil {
		t.Fatalf("desired+observed provider report: %v", err)
	}

	desiredOnly := completeRunnerReport(
		roleProviderReport,
		"provider:registry.terraform.io/tako0614/takoform",
		identity,
		[]string{"canonical"},
		[]string{"reject-desired"},
	)
	if _, err := ValidateCanonicalProviderRunnerReportWithStages(
		canonicalFixture(t, desiredOnly),
		identity,
		[]string{"canonical"},
		expectations,
	); err == nil || !strings.Contains(err.Error(), "fixture closure") {
		t.Fatalf("provider observed-stage omission error = %v", err)
	}

	for _, stage := range []string{"output", "host-private", " observed "} {
		t.Run(stage, func(t *testing.T) {
			if _, err := ValidateCanonicalProviderRunnerReportWithStages(
				raw,
				identity,
				[]string{"canonical"},
				[]NegativeFixtureExpectation{{Name: "reject-desired", Stage: "desired"}, {Name: "reject-invalid", Stage: stage}},
			); err == nil || !strings.Contains(err.Error(), "not supported") {
				t.Fatalf("stage %q error = %v", stage, err)
			}
		})
	}
	if _, err := ValidateCanonicalProviderRunnerReportWithStages(raw, identity, []string{"canonical"}, nil); err == nil ||
		!strings.Contains(err.Error(), "negative fixture expectations are required") {
		t.Fatalf("empty provider expectation error = %v", err)
	}

	// The pre-stage API remains source- and behavior-compatible: every supplied
	// negative name is interpreted as a desired-stage expectation.
	if _, err := ValidateCanonicalProviderRunnerReport(
		raw,
		identity,
		[]string{"canonical"},
		[]string{"reject-desired", "reject-observed"},
	); err != nil {
		t.Fatalf("all-desired compatibility API: %v", err)
	}
}

func TestBuildCanonicalSetReturnsCanonicalBytes(t *testing.T) {
	t.Parallel()
	want := testSet()
	// Admission closure has its own immutable checkpoint tag. It may advance
	// without republishing the exact Form definition/package closure.
	want.AdmissionReleaseTag = "forms/admissions/v1.0.2"
	set, raw, err := BuildCanonicalSet(
		testCandidates(),
		want.AdmissionReleaseTag,
		want.ProviderRegistryReadback,
		want.Entries,
	)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, canonical) {
		t.Fatal("set bytes are not canonical")
	}
	var decoded Set
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Format != setFormatV2 || decoded.DefinitionVersion != "1.0.0" || decoded.PackageVersion != "1.0.0" || decoded.AdmissionReleaseTag != "forms/admissions/v1.0.2" || decoded.Entries[0].Kind != set.Entries[0].Kind {
		t.Fatalf("unexpected set: %#v", decoded)
	}
}

func TestBuildCanonicalSetV3PreservesPerEntryVersions(t *testing.T) {
	t.Parallel()
	candidates := CandidateSet{
		Generation: "ga-core-v1",
		Entries: []Candidate{
			testV3Candidate("ObjectBucket", "object-bucket", "2.0.0"),
			testV3Candidate("KeyValueStore", "key-value-store", "1.0.0"),
		},
	}
	entries := []SetEntry{
		testV3Entry("ObjectBucket", "object-bucket", "2.0.0"),
		testV3Entry("KeyValueStore", "key-value-store", "1.0.0"),
	}
	set, raw, err := BuildCanonicalSet(
		candidates,
		"forms/admissions/v2.0.0",
		RegistryReadbackRef{Path: "registry/provider-readback.json", Digest: testEvidenceDigest, SigstoreBundle: "registry/provider-readback.sigstore.json"},
		entries,
		testV3ProviderClosure(candidates),
	)
	if err != nil {
		t.Fatal(err)
	}
	if set.Format != setFormatV3 || set.Generation != "ga-core-v1" || set.DefinitionVersion != "" || set.PackageVersion != "" {
		t.Fatalf("unexpected v3 set identity: %#v", set)
	}
	if set.Entries[0].ReleaseTag != "forms/"+releaseIDForKind("ObjectBucket")+"/v2.0.0" ||
		set.Entries[1].ReleaseTag != "forms/"+releaseIDForKind("KeyValueStore")+"/v1.0.0" {
		t.Fatalf("v3 entry versions collapsed: %#v", set.Entries)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil || !bytes.Equal(raw, canonical) {
		t.Fatalf("v3 set is not canonical: %v", err)
	}
}

func testV3ProviderClosure(candidates CandidateSet) ProviderReportClosure {
	reports := make([]ProviderReportClosureEntry, 0, 34)
	for _, candidate := range candidates.Entries {
		reports = append(reports, ProviderReportClosureEntry{
			Kind: candidate.Kind, Slug: candidate.Slug,
			Identity:       standardform.InstalledFormReference{FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest},
			ReportPath:     path.Join("provider-closure", "packages", candidate.Slug, "provider-report.json"),
			ReportDigest:   testEvidenceDigest,
			SigstoreBundle: path.Join("provider-closure", "packages", candidate.Slug, "provider-report.sigstore.json"),
		})
	}
	for index := len(reports); index < 34; index++ {
		kind := fmt.Sprintf("ExtraKind%02d", index)
		slug := fmt.Sprintf("extra-kind-%02d", index)
		reports = append(reports, ProviderReportClosureEntry{
			Kind: kind, Slug: slug,
			Identity: standardform.InstalledFormReference{
				FormRef: formpackage.FormRef{
					APIVersion: formpackage.FormAPIVersion, Kind: kind,
					DefinitionVersion: "1.0.0", SchemaDigest: testSchemaDigest,
				},
				PackageDigest: testPackageDigest,
			},
			ReportPath:     path.Join("provider-closure", "packages", slug, "provider-report.json"),
			ReportDigest:   testEvidenceDigest,
			SigstoreBundle: path.Join("provider-closure", "packages", slug, "provider-report.sigstore.json"),
		})
	}
	return ProviderReportClosure{
		Generation:           "portable-v1",
		ManifestPath:         "provider-closure/provider-report-manifest.json",
		ManifestDigest:       testEvidenceDigest,
		SignedManifestPath:   "provider-closure/signed-provider-report-candidate.json",
		SignedManifestDigest: testEvidenceDigest,
		ChecksumsPath:        "provider-closure/SHA256SUMS",
		ChecksumsDigest:      testEvidenceDigest,
		Reports:              reports,
	}
}

func testV3Candidate(kind, slug, version string) Candidate {
	return Candidate{
		Kind: kind, Slug: slug, PackagePath: "forms/releases/" + releaseIDForKind(kind) + "/" + version,
		FormRef: formpackage.FormRef{
			APIVersion: formpackage.FormAPIVersion, Kind: kind, DefinitionVersion: version, SchemaDigest: testSchemaDigest,
		},
		PackageDigest: testPackageDigest,
	}
}

func testV3Entry(kind, slug, version string) SetEntry {
	releaseID := releaseIDForKind(kind)
	base := "releases/" + releaseID + "/" + version + "/takoform-form-" + releaseID + "_" + version
	return SetEntry{
		Kind: kind, Slug: slug,
		FormRef: formpackage.FormRef{
			APIVersion: formpackage.FormAPIVersion, Kind: kind, DefinitionVersion: version, SchemaDigest: testSchemaDigest,
		},
		PackageDigest: testPackageDigest,
		ReleaseTag:    "forms/" + releaseID + "/v" + version, ReleaseCommit: "0123456789abcdef0123456789abcdef01234567",
		ReleaseToolingCommit:         "89abcdef0123456789abcdef0123456789abcdef",
		PackageReleaseManifestPath:   "releases/" + releaseID + "/" + version + "/release-manifest.json",
		PackageReleaseManifestDigest: testEvidenceDigest,
		PackageIndexPath:             base + "_package-index.json",
		PackageIndexSigstoreBundle:   base + "_package-index.sigstore.json",
		EvidencePath:                 "packages/" + slug + "/evidence.json",
		EvidenceDigest:               testEvidenceDigest,
		HostReportPath:               "packages/" + slug + "/host-report.json",
		HostReportDigest:             testEvidenceDigest,
		HostReportSigstoreBundle:     "packages/" + slug + "/host-report.sigstore.json",
		ProviderReportPath:           "provider-closure/packages/" + slug + "/provider-report.json",
		ProviderReportDigest:         testEvidenceDigest,
		ProviderReportSigstoreBundle: "provider-closure/packages/" + slug + "/provider-report.sigstore.json",
		AdmissionStatus:              "portable-standard",
	}
}
