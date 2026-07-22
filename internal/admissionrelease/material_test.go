package admissionrelease

import (
	"bytes"
	"encoding/json"
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

func TestBuildCanonicalSetReturnsCanonicalBytes(t *testing.T) {
	t.Parallel()
	want := testSet()
	// Admission activation has its own immutable release stream. It may advance
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
	if decoded.Format != setFormat || decoded.DefinitionVersion != "1.0.0" || decoded.PackageVersion != "1.0.0" || decoded.AdmissionReleaseTag != "forms/admissions/v1.0.2" || decoded.Entries[0].Kind != set.Entries[0].Kind {
		t.Fatalf("unexpected set: %#v", decoded)
	}
}
