package formpackage

import (
	"encoding/json"
	"strings"
	"testing"
)

func interfaceDefinitionFixture() map[string]any {
	objectSchema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
	}
	return map[string]any{
		"apiVersion": "interfaces.takoform.com/v1alpha1",
		"kind":       "InterfaceDefinition",
		"name":       "example.invoke",
		"version":    "1.0.0",
		"operations": []any{map[string]any{
			"name": "invoke", "inputSchema": objectSchema,
			"outputSchema": objectSchema, "errors": []any{},
		}},
		"semantics": map[string]any{"consistency": "eventual"},
	}
}

func TestValidateInterfaceDefinitionUsesEmbeddedNormativeSchema(t *testing.T) {
	t.Parallel()
	valid, err := json.Marshal(interfaceDefinitionFixture())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateInterfaceDefinition(valid); err != nil {
		t.Fatalf("valid Interface Definition was rejected: %v", err)
	}

	invalid := interfaceDefinitionFixture()
	delete(invalid, "semantics")
	raw, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateInterfaceDefinition(raw)
	if err == nil || !strings.Contains(err.Error(), "semantics") {
		t.Fatalf("missing semantics error = %v, want normative-schema rejection", err)
	}
}
