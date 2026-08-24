package portableconformancev3

import (
	"context"
	"net/http/httptest"
	"testing"
)

// This targeted black-box test keeps the constraint matrix runnable while the
// real family catalog may be moving concurrently. The complete SelfTest still
// owns integration with that catalog; this test owns the conformance-only
// Definitions and the exact required-check implementation itself.
func TestDeclaredConstraintSemanticsCorpusCheck(t *testing.T) {
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := FallbackCatalog(contract)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewReferenceHost(contract, catalog))
	defer server.Close()
	runner := &v3Runner{
		ctx: context.Background(), contract: contract,
		endpoint: server.URL, token: referencePrimaryToken,
		alternateToken: referenceAlternateToken, alternateTenantToken: referenceAlternateTenantToken,
		httpClient: server.Client(), apiBase: server.URL + contract.APIPath,
		completed: map[string]bool{},
	}
	runner.pinDesiredSchemas()
	canonical := func(label string, value any) string {
		t.Helper()
		encoded, err := canonicalJSON(value)
		if err != nil {
			t.Fatalf("canonicalize %s: %v", label, err)
		}
		return encoded
	}
	for _, entry := range constraintDefinitionInventory(&contract.RunnerInput) {
		served, err := runner.formDefinition(entry.probe.FormRef)
		if err != nil {
			t.Fatalf("served %s Definition: %v", entry.label, err)
		}
		servedSchema := canonical(entry.label+" served schema", served.DesiredSchema)
		pinnedSchema := canonical(entry.label+" pinned schema", entry.probe.Definition.DesiredSchema)
		servedConstraints := canonical(entry.label+" served constraints", map[string]any{"constraints": served.Constraints})
		pinnedConstraints := canonical(entry.label+" pinned constraints", map[string]any{"constraints": entry.probe.Definition.Constraints})
		if servedSchema != pinnedSchema || servedConstraints != pinnedConstraints {
			t.Fatalf("served %s Definition drifted from the byte-pinned constraint fixture", entry.label)
		}
	}
	if err := runner.checkDeclaredConstraintSemanticsEnforced(); err != nil {
		t.Fatal(err)
	}
	if !runner.completed["declared-constraint-semantics-enforced"] {
		t.Fatal("constraint semantics check did not record completion")
	}
}
