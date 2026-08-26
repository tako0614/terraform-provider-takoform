package formpackage

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestValidateBindingDefinitionUsesEmbeddedNormativeSchema(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../bindings/candidates/v1alpha2/module-worker.actor/definition.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateBindingDefinition(raw); err != nil {
		t.Fatalf("valid current Binding Definition was rejected: %v", err)
	}

	var fixture map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "unknown field",
			mutate: func(value map[string]any) {
				value["unknown"] = true
			},
		},
		{
			name: "wrong exact target ref",
			mutate: func(value map[string]any) {
				target := value["targetInterface"].(map[string]any)
				target["apiVersion"] = "interfaces.takoform.com/v1alpha2"
			},
		},
		{
			name: "binding name grammar is not compilable",
			mutate: func(value map[string]any) {
				value["bindingNameGrammar"] = "^[$"
			},
		},
		{
			name: "binding name grammar is not anchored",
			mutate: func(value map[string]any) {
				value["bindingNameGrammar"] = "[A-Za-z_$][A-Za-z0-9_$]*"
			},
		},
		{
			name: "binding name grammar has no end anchor",
			mutate: func(value map[string]any) {
				value["bindingNameGrammar"] = "^[A-Za-z_$][A-Za-z0-9_$]*"
			},
		},
		{
			name: "binding name grammar escapes end anchor",
			mutate: func(value map[string]any) {
				value["bindingNameGrammar"] = `^[A-Za-z_$][A-Za-z0-9_$]*\$`
			},
		},
		{
			name: "one top-level alternative has no begin-text anchor",
			mutate: func(value map[string]any) {
				value["bindingNameGrammar"] = `^$|foo$`
			},
		},
		{
			name: "multiline mode weakens anchors to line boundaries",
			mutate: func(value map[string]any) {
				value["bindingNameGrammar"] = `^(?m)[A-Za-z_$][A-Za-z0-9_$]*$`
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := make(map[string]any, len(fixture)+1)
			for key, value := range fixture {
				candidate[key] = value
			}
			test.mutate(candidate)
			encoded, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateBindingDefinition(encoded); err == nil || !strings.Contains(err.Error(), "Binding Definition") {
				t.Fatalf("invalid Binding Definition error = %v, want normative-schema rejection", err)
			}
		})
	}
}
