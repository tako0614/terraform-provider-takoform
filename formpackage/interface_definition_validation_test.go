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

func TestValidateInterfaceDefinitionRejectsSemanticReferenceGaps(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "duplicate operation names with different objects",
			mutate: func(definition map[string]any) {
				operations := definition["operations"].([]any)
				operations = append(operations, map[string]any{
					"name":         "invoke",
					"description":  "a different operation object",
					"inputSchema":  map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object"},
					"outputSchema": map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object"},
					"errors":       []any{},
				})
				definition["operations"] = operations
			},
			want: "declared more than once",
		},
		{
			name: "fixture references undeclared operation",
			mutate: func(definition map[string]any) {
				definition["fixtures"] = []any{map[string]any{
					"name": "unknown-operation",
					"steps": []any{map[string]any{
						"operation": "missing",
						"input":     map[string]any{},
					}},
				}}
			},
			want: "undeclared operation",
		},
		{
			name: "fixture expects an undeclared operation error",
			mutate: func(definition map[string]any) {
				operation := definition["operations"].([]any)[0].(map[string]any)
				operation["errors"] = []any{"known_error"}
				definition["fixtures"] = []any{map[string]any{
					"name": "unknown-error",
					"steps": []any{map[string]any{
						"operation":     "invoke",
						"input":         map[string]any{},
						"expectedError": "missing_error",
					}},
				}}
			},
			want: "does not declare",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := interfaceDefinitionFixture()
			test.mutate(candidate)
			raw, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateInterfaceDefinition(raw); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateInterfaceDefinition error = %v, want %q", err, test.want)
			}
		})
	}
}
