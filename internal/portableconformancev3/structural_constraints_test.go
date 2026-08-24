package portableconformancev3

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func structuralConstraintHost(t *testing.T) (*ReferenceHost, Contract, FormRef) {
	t.Helper()
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	catalog := newCatalog(contract.APIVersion)
	catalog.family = constraintTestGroup
	ref := constraintTestRef("StructuralConstraintHolder", "0.1.0", "9")
	installConstraintTestForm(t, catalog, &InstalledForm{
		Ref: ref, Role: "policy", Title: "Structural constraint holder",
		DesiredSchema: closedConstraintSchema(map[string]any{
			"lower": map[string]any{"type": "integer"},
			"upper": map[string]any{"type": "integer"},
			"rows": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 8,
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"properties": map[string]any{
						"key":   map[string]any{"type": "string"},
						"value": map[string]any{"type": "integer"},
					},
					"required": []any{"key", "value"},
				},
			},
		}, "lower", "upper", "rows"),
		Lifecycle:       lifecycleCapabilitiesWithUpdate(),
		RequiresHostAPI: contract.APIVersion,
		Constraints: []formpackage.FormConstraint{
			{Kind: "orderedPair", References: []string{"/lower", "/upper"}},
			{Kind: "uniqueBy", List: "/rows", Member: "key"},
		},
	})
	return NewReferenceHost(contract, catalog), contract, ref
}

func TestOrderedPairAndUniqueByAreEnforcedAtValidatePrepareAndApply(t *testing.T) {
	if key, ok := canonicalScalarKey("a"); !ok || key != "string\x00a" {
		t.Fatalf("canonical scalar key = %q, %v", key, ok)
	}
	numericOne, ok := canonicalScalarKey(json.Number("1"))
	if !ok {
		t.Fatal("integer JSON number was not a scalar key")
	}
	numericOneDecimal, ok := canonicalScalarKey(json.Number("1.0"))
	if !ok || numericOne != numericOneDecimal {
		t.Fatalf("equivalent JSON numbers have keys %q and %q", numericOne, numericOneDecimal)
	}
	numericThousand, ok := canonicalScalarKey(json.Number("1e3"))
	decimalThousand, decimalOK := canonicalScalarKey(json.Number("1000"))
	if !ok || !decimalOK || numericThousand != decimalThousand {
		t.Fatalf("equivalent exponent JSON numbers have keys %q and %q", numericThousand, decimalThousand)
	}
	stringOne, _ := canonicalScalarKey("1")
	if stringOne == numericOne {
		t.Fatal("string and numeric JSON scalar domains collapsed")
	}
	host, contract, ref := structuralConstraintHost(t)
	server := httptest.NewServer(host)
	defer server.Close()
	valid := map[string]any{
		"lower": 1, "upper": 2,
		"rows": []any{
			map[string]any{"key": "a", "value": 1},
			map[string]any{"key": "b", "value": 2},
		},
	}
	status, raw := hostRequest(
		t, server, http.MethodPost, contract.APIPath+"/resources/validate", nil,
		constraintResourceBody(ref, "structural-valid", "conformance", valid),
	)
	if status != http.StatusOK || !strings.Contains(string(raw), `"valid":true`) {
		t.Fatalf("valid structural document = %d %s", status, strings.TrimSpace(string(raw)))
	}
	createConstraintResource(t, server, contract, ref, "structural-valid", "conformance", valid, "key-structural-valid")

	cases := []struct {
		name string
		spec map[string]any
		kind string
	}{
		{
			name: "descending", kind: "orderedPair",
			spec: map[string]any{
				"lower": 3, "upper": 2,
				"rows": []any{map[string]any{"key": "a", "value": 1}},
			},
		},
		{
			name: "duplicate", kind: "uniqueBy",
			spec: map[string]any{
				"lower": 1, "upper": 2,
				"rows": []any{
					map[string]any{"key": "a", "value": 1},
					map[string]any{"key": "a", "value": 2},
				},
			},
		},
	}
	for _, testCase := range cases {
		body := constraintResourceBody(ref, "structural-"+testCase.name, "conformance", testCase.spec)
		status, raw := hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/validate", nil, body)
		if status != http.StatusOK || !strings.Contains(string(raw), `"valid":false`) || !strings.Contains(string(raw), testCase.kind) {
			t.Fatalf("%s validate = %d %s", testCase.name, status, strings.TrimSpace(string(raw)))
		}
		status, raw = hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/prepare", nil, body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s prepare = %d %s, want 400", testCase.name, status, strings.TrimSpace(string(raw)))
		}
	}
	for _, testCase := range []struct {
		name string
		spec map[string]any
	}{
		{
			name: "missing-ordered-operand",
			spec: map[string]any{
				"lower": 1,
				"rows":  []any{map[string]any{"key": "a", "value": 1}},
			},
		},
		{
			name: "missing-unique-member",
			spec: map[string]any{
				"lower": 1, "upper": 2,
				"rows": []any{map[string]any{"value": 1}},
			},
		},
	} {
		body := constraintResourceBody(ref, "structural-"+testCase.name, "conformance", testCase.spec)
		status, raw := hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/validate", nil, body)
		if status != http.StatusOK || !strings.Contains(string(raw), `"valid":false`) {
			t.Fatalf("%s validate = %d %s, want valid=false", testCase.name, status, strings.TrimSpace(string(raw)))
		}
		status, raw = hostRequest(t, server, http.MethodPost, contract.APIPath+"/resources/prepare", nil, body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s prepare = %d %s, want 400", testCase.name, status, strings.TrimSpace(string(raw)))
		}
	}
}
