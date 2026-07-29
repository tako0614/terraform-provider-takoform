package standardform

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestEvidenceForbiddenAuthorityFields(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value any
		valid bool
	}{
		{name: "manager", value: map[string]any{"audit": map[string]any{"manager": "x"}}},
		{name: "secret", value: map[string]any{"fixtures": []any{map[string]any{"secret": "x"}}}},
		{name: "structural-provider-proof", value: map[string]any{"conformance": map[string]any{"provider": map[string]any{"subject": "terraform-provider-takoform"}}}, valid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := rejectForbiddenEvidenceKeys(test.value, "$")
			if test.valid && err != nil {
				t.Fatal(err)
			}
			if !test.valid && err == nil {
				t.Fatal("forbidden authority field unexpectedly accepted")
			}
		})
	}
}

func TestPortableNegativeErrorCode(t *testing.T) {
	t.Parallel()
	if !validPortableNegativeErrorCode("invalid_argument") {
		t.Fatal("portable invalid_argument rejected")
	}
	for _, internalOrCompatibilityCode := range []string{"schema_validation_failed", "invalid_spec"} {
		if validPortableNegativeErrorCode(internalOrCompatibilityCode) {
			t.Fatalf("non-portable wire code %q accepted", internalOrCompatibilityCode)
		}
	}
}

func TestValidateEvidenceBytesRejectsNonCanonicalInputBeforeClaims(t *testing.T) {
	t.Parallel()
	_, err := ValidateEvidenceBytes(
		[]byte(`{ "apiVersion": "forms.takoform.com/standard-admission/v1alpha1" }`),
		formpackage.VerificationReport{}, formpackage.FormDefinition{},
	)
	if err == nil || !strings.Contains(err.Error(), "not RFC 8785 canonical") {
		t.Fatalf("err = %v, want canonical-byte rejection", err)
	}
}

func TestNegativeFixtureNamesForProof(t *testing.T) {
	t.Parallel()
	fixtures := []NegativeFixture{
		{Name: "reject-desired", Stage: "desired"},
		{Name: "reject-observed", Stage: "observed"},
	}

	host, err := HostNegativeFixtureNames(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"reject-desired"}; !reflect.DeepEqual(host, want) {
		t.Fatalf("host fixture names = %v, want %v", host, want)
	}

	provider, err := ProviderNegativeFixtureNames(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"reject-desired", "reject-observed"}; !reflect.DeepEqual(provider, want) {
		t.Fatalf("provider fixture names = %v, want %v", provider, want)
	}
}

func TestNegativeFixtureNamesForProofRejectInvalidFixtureClosure(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		fixtures []NegativeFixture
	}{
		{name: "empty name", fixtures: []NegativeFixture{{Stage: "desired"}}},
		{name: "blank name", fixtures: []NegativeFixture{{Name: " \t", Stage: "desired"}}},
		{name: "duplicate name", fixtures: []NegativeFixture{
			{Name: "reject-value", Stage: "desired"},
			{Name: "reject-value", Stage: "observed"},
		}},
		{name: "output stage", fixtures: []NegativeFixture{{Name: "reject-output", Stage: "output"}}},
		{name: "unknown stage", fixtures: []NegativeFixture{{Name: "reject-value", Stage: "host-specific"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := HostNegativeFixtureNames(test.fixtures); err == nil {
				t.Fatal("host fixture selection unexpectedly succeeded")
			}
			if _, err := ProviderNegativeFixtureNames(test.fixtures); err == nil {
				t.Fatal("provider fixture selection unexpectedly succeeded")
			}
		})
	}
}

func TestValidateEvidenceBytesUsesStageAwareProofCoverage(t *testing.T) {
	t.Parallel()
	negatives := []NegativeFixture{
		{Name: "reject-desired", Stage: "desired", Input: map[string]any{"name": 1}, ExpectedErrorCode: InvalidArgumentErrorCode},
		{Name: "reject-observed", Stage: "observed", Input: map[string]any{"id": 1}, ExpectedErrorCode: InvalidArgumentErrorCode},
	}

	raw, report, definition := validEvidenceBytes(t, negatives, []string{"reject-desired"}, []string{"reject-desired", "reject-observed"})
	if _, err := ValidateEvidenceBytes(raw, report, definition); err != nil {
		t.Fatalf("stage-aware evidence rejected: %v", err)
	}

	raw, report, definition = validEvidenceBytes(t, negatives, []string{"reject-desired", "reject-observed"}, []string{"reject-desired", "reject-observed"})
	if _, err := ValidateEvidenceBytes(raw, report, definition); err == nil || !strings.Contains(err.Error(), "host conformance proof") {
		t.Fatalf("err = %v, want host proof coverage rejection", err)
	}

	raw, report, definition = validEvidenceBytes(t, negatives, []string{"reject-desired"}, []string{"reject-desired"})
	if _, err := ValidateEvidenceBytes(raw, report, definition); err == nil || !strings.Contains(err.Error(), "provider conformance proof") {
		t.Fatalf("err = %v, want provider proof coverage rejection", err)
	}
}

func TestValidateEvidenceBytesPreservesAllDesiredEvidence(t *testing.T) {
	t.Parallel()
	negatives := []NegativeFixture{
		{Name: "reject-one", Stage: "desired", Input: map[string]any{"name": 1}, ExpectedErrorCode: InvalidArgumentErrorCode},
		{Name: "reject-two", Stage: "desired", Input: map[string]any{"name": 2}, ExpectedErrorCode: InvalidArgumentErrorCode},
	}
	names := []string{"reject-one", "reject-two"}
	raw, report, definition := validEvidenceBytes(t, negatives, names, names)
	if _, err := ValidateEvidenceBytes(raw, report, definition); err != nil {
		t.Fatalf("all-desired evidence rejected: %v", err)
	}
}

func TestValidateEvidenceBytesRejectsOutputStageWithoutProofSemantics(t *testing.T) {
	t.Parallel()
	negatives := []NegativeFixture{{
		Name: "reject-output", Stage: "output", Input: map[string]any{"url": 1}, ExpectedErrorCode: InvalidArgumentErrorCode,
	}}
	raw, report, definition := validEvidenceBytes(t, negatives, nil, nil)
	if _, err := ValidateEvidenceBytes(raw, report, definition); err == nil || !strings.Contains(err.Error(), `unsupported admission-proof stage "output"`) {
		t.Fatalf("err = %v, want output-stage rejection", err)
	}
}

func TestValidateEvidenceBytesRejectsObservedOnlyNegativeClosure(t *testing.T) {
	t.Parallel()
	negatives := []NegativeFixture{{
		Name: "reject-observed", Stage: "observed", Input: map[string]any{"id": 1}, ExpectedErrorCode: InvalidArgumentErrorCode,
	}}
	raw, report, definition := validEvidenceBytes(t, negatives, nil, []string{"reject-observed"})
	if _, err := ValidateEvidenceBytes(raw, report, definition); err == nil || !strings.Contains(err.Error(), "non-empty host and provider negative-fixture coverage") {
		t.Fatalf("err = %v, want empty host coverage rejection", err)
	}
}

func validEvidenceBytes(
	t *testing.T,
	negatives []NegativeFixture,
	hostNegativeNames []string,
	providerNegativeNames []string,
) ([]byte, formpackage.VerificationReport, formpackage.FormDefinition) {
	t.Helper()
	schemaDigest := "sha256:" + strings.Repeat("a", 64)
	packageDigest := "sha256:" + strings.Repeat("b", 64)
	proofDigest := "sha256:" + strings.Repeat("c", 64)
	formRef := formpackage.FormRef{
		APIVersion:        formpackage.FormAPIVersion,
		Kind:              "Queue",
		DefinitionVersion: "1.0.0",
		SchemaDigest:      schemaDigest,
	}
	report := formpackage.VerificationReport{FormRef: formRef, PackageDigest: packageDigest}
	identity := InstalledFormReference{FormRef: formRef, PackageDigest: packageDigest}
	proof := func(subject string, negativeNames []string) ConformanceProof {
		return ConformanceProof{
			Subject:          subject,
			RunnerVersion:    "1.0.0",
			Identity:         identity,
			Status:           "passed",
			PositiveFixtures: []string{"canonical"},
			NegativeFixtures: append([]string(nil), negativeNames...),
			EvidenceDigest:   proofDigest,
		}
	}
	evidence := AdmissionEvidence{
		APIVersion:           APIVersion,
		Identity:             identity,
		Classification:       "portable-standard",
		ApprovedSchemaDigest: schemaDigest,
		Audit: Audit{
			Lifecycle: LifecycleAudit{
				Create: true, Read: true, Update: true, Delete: true,
				Import: true, Observe: true, Refresh: true, Drift: true,
			},
			Immutability: ImmutabilityAudit{Reviewed: true, Fields: []string{"/name"}},
			Security: SecurityAudit{
				SecretFreeDesiredState: true, CredentialBoundaryExternal: true, DataOnlyPackage: true,
			},
			Interfaces: InterfaceAudit{
				Reviewed: true, BindingAuthorityExternal: true, SecretFreeDocuments: true,
			},
		},
		Fixtures: Fixtures{
			Positive: []PositiveFixture{{Name: "canonical", Desired: map[string]any{"name": "main"}, Observed: map[string]any{}, Output: map[string]any{}}},
			Negative: negatives,
		},
		Conformance: Conformance{
			Host:     proof("host:test", hostNegativeNames),
			Provider: proof("provider:test", providerNegativeNames),
		},
	}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(encoded)
	if err != nil {
		t.Fatal(err)
	}
	definition := formpackage.FormDefinition{
		APIVersion:        formpackage.FormAPIVersion,
		Kind:              formRef.Kind,
		DefinitionVersion: formRef.DefinitionVersion,
		Status:            "standard",
		ImmutableFields:   []string{"/name"},
		LifecycleCapabilities: []string{
			"create", "read", "update", "delete", "import", "observe", "refresh", "drift",
		},
	}
	return canonical, report, definition
}
