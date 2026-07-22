package standardform

import (
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
