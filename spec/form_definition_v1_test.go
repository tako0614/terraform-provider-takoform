package spec

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileFormDefinitionV1(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	for _, path := range []string{
		"schemas/interface-ref-v1alpha1.schema.json",
		"schemas/binding-ref-v1alpha2.schema.json",
		"schemas/form-definition-v1.schema.json",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		id := document.(map[string]any)["$id"].(string)
		if err := compiler.AddResource(id, document); err != nil {
			t.Fatalf("add %s: %v", path, err)
		}
	}
	compiled, err := compiler.Compile("https://forms.takoform.com/schemas/v1/form-definition.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func minimalV1Definition(constraint map[string]any) map[string]any {
	definition := map[string]any{
		"apiVersion":        "queue.forms.takoform.com",
		"kind":              "Example",
		"definitionVersion": "0.1.0",
		"title":             "Example",
		"role":              "identity",
		"requiresHostApi":   "forms.takoform.com/v1",
		"desiredSchema": map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "object",
		},
		"lifecycleCapabilities": []any{"create", "read", "delete", "import", "observe"},
	}
	if constraint != nil {
		definition["constraints"] = []any{constraint}
	}
	return definition
}

func TestFormDefinitionV1AcceptsResolvedUIDConstraintVariants(t *testing.T) {
	t.Parallel()
	compiled := compileFormDefinitionV1(t)
	for name, constraint := range map[string]map[string]any{
		"acyclic": {
			"kind": "acyclic", "reference": "/deadLetter/queue",
		},
		"distinctPair": {
			"kind": "distinctPair", "references": []any{"/target", "/deadLetter"},
		},
		"uniquePair": {
			"kind": "uniquePair", "references": []any{"/topic", "/target"},
		},
		"sameResolvedTarget": {
			"kind": "sameResolvedTarget", "anchor": "/function",
			"members": "/versions/*/functionVersion", "through": "/function",
		},
	} {
		name, constraint := name, constraint
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := compiled.Validate(minimalV1Definition(constraint)); err != nil {
				t.Fatalf("valid %s constraint rejected: %v", name, err)
			}
		})
	}
}

func TestFormDefinitionV1AcceptsStructuralConstraintVariants(t *testing.T) {
	t.Parallel()
	compiled := compileFormDefinitionV1(t)
	for name, constraint := range map[string]map[string]any{
		"orderedPair": {
			"kind": "orderedPair", "references": []any{"/minimum", "/maximum"},
		},
		"uniqueBy": {
			"kind": "uniqueBy", "list": "/secondaryIndexes", "member": "name",
		},
	} {
		name, constraint := name, constraint
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := compiled.Validate(minimalV1Definition(constraint)); err != nil {
				t.Fatalf("valid %s constraint rejected: %v", name, err)
			}
		})
	}
}

func TestFormDefinitionV1RejectsOpenOrMalformedConstraintStates(t *testing.T) {
	t.Parallel()
	compiled := compileFormDefinitionV1(t)
	for name, constraint := range map[string]map[string]any{
		"unknown kind": {
			"kind": "graphExpression", "reference": "/target",
		},
		"foreign member": {
			"kind": "uniquePair", "references": []any{"/topic", "/target"}, "through": "/function",
		},
		"duplicate pair": {
			"kind": "distinctPair", "references": []any{"/target", "/target"},
		},
		"ordered pair wildcard": {
			"kind": "orderedPair", "references": []any{"/bounds/*/minimum", "/maximum"},
		},
		"uniqueBy missing member": {
			"kind": "uniqueBy", "list": "/secondaryIndexes",
		},
		"uniqueBy pointer member": {
			"kind": "uniqueBy", "list": "/secondaryIndexes", "member": "nested/name",
		},
		"acyclic wildcard": {
			"kind": "acyclic", "reference": "/queues/*",
		},
		"same target without list member": {
			"kind": "sameResolvedTarget", "anchor": "/function",
			"members": "/versions/functionVersion", "through": "/function",
		},
		"same target with two list levels": {
			"kind": "sameResolvedTarget", "anchor": "/function",
			"members": "/versions/*/groups/*/functionVersion", "through": "/function",
		},
	} {
		name, constraint := name, constraint
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := compiled.Validate(minimalV1Definition(constraint)); err == nil {
				t.Fatalf("malformed %s constraint was accepted: %#v", name, constraint)
			}
		})
	}
}

func TestFormDefinitionV1RequiresAVersionlessFormGroupAndStableHostLane(t *testing.T) {
	t.Parallel()
	compiled := compileFormDefinitionV1(t)
	definition := minimalV1Definition(nil)
	definition["apiVersion"] = "queue.forms.takoform.com/v1beta1"
	if err := compiled.Validate(definition); err == nil {
		t.Fatal("v1 Form Definition accepted a family-versioned group")
	}
	definition = minimalV1Definition(nil)
	definition["requiresHostApi"] = "forms.takoform.com/v1beta4"
	if err := compiled.Validate(definition); err == nil {
		t.Fatal("v1 Form Definition accepted the occupied beta4 Host lane")
	}
}
