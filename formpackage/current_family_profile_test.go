package formpackage

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func addContainerLikeFields(definition map[string]any) {
	desired := definition["desiredSchema"].(map[string]any)
	properties := desired["properties"].(map[string]any)
	stringList := func() map[string]any {
		return map[string]any{
			"type": "array", "maxItems": 64,
			"items": map[string]any{
				"type": "string", "pattern": `^[^\x00\r\n]{1,256}$`, "maxLength": 256,
			},
		}
	}
	properties["command"] = stringList()
	properties["args"] = stringList()
	properties["concurrencyTarget"] = map[string]any{
		"type": "integer", "minimum": 1, "maximum": 1000,
	}
	required, _ := desired["required"].([]any)
	desired["required"] = append(required, "concurrencyTarget")
}

func TestValidateDefinitionAcceptsBoundedContainerProcessConfiguration(t *testing.T) {
	t.Parallel()
	definition := currentFamilyDefinitionFixture(t)
	definition["requiresHostApi"] = "forms.takoform.com/v1"
	addContainerLikeFields(definition)
	if _, err := ValidateDefinition(canonicalMarshal(t, definition)); err != nil {
		t.Fatalf("bounded Container-like process configuration was rejected: %v", err)
	}
}

func reviewedInterfaceTargetSchema(group, kind string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"apiVersion", "kind", "name"},
		"properties": map[string]any{
			"apiVersion": map[string]any{"type": "string", "const": group},
			"kind":       map[string]any{"type": "string", "const": kind},
			"name": map[string]any{
				"type": "string", "minLength": 1, "maxLength": 63,
				"pattern": `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`,
			},
		},
		"x-takoform-required-interface": map[string]any{
			"apiVersion": "interfaces.takoform.com/v1alpha1",
			"name":       "queue.pull", "version": "1.0.0",
			"schemaDigest": "sha256:" + strings.Repeat("a", 64),
		},
	}
}

func reviewedTaggedTargetSchema() map[string]any {
	branch := func(tag, member, group, kind string) map[string]any {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"type", member},
			"properties": map[string]any{
				"type": map[string]any{"type": "string", "const": tag},
				member: reviewedInterfaceTargetSchema(group, kind),
			},
		}
	}
	return map[string]any{
		"x-takoform-discriminator": "type",
		"oneOf": []any{
			branch("queueMessage", "queue", "queue.forms.takoform.com", "PullQueue"),
			branch("topicPublish", "topic", "topic.forms.takoform.com", "Topic"),
		},
	}
}

func TestValidateDefinitionAcceptsOnlySchemaProvenPortableTargetFields(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		target map[string]any
	}{
		{name: "direct canonical ResourceTarget", target: reviewedInterfaceTargetSchema("queue.forms.takoform.com", "PullQueue")},
		{name: "closed tagged target union", target: reviewedTaggedTargetSchema()},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			definition := currentFamilyDefinitionFixture(t)
			definition["requiresHostApi"] = "forms.takoform.com/v1"
			desired := definition["desiredSchema"].(map[string]any)
			desired["properties"].(map[string]any)["target"] = test.target
			required, _ := desired["required"].([]any)
			desired["required"] = append(required, "target")
			if _, err := ValidateDefinition(canonicalMarshal(t, definition)); err != nil {
				t.Fatalf("schema-proven portable target was rejected: %v", err)
			}
		})
	}
}

func TestValidateDefinitionRejectsExecutableOrOpenCommandAndTargetFields(t *testing.T) {
	t.Parallel()
	openTarget := reviewedInterfaceTargetSchema("queue.forms.takoform.com", "PullQueue")
	openTarget["additionalProperties"] = true
	unannotatedTarget := reviewedInterfaceTargetSchema("queue.forms.takoform.com", "PullQueue")
	delete(unannotatedTarget, "x-takoform-required-interface")
	ambiguousTarget := reviewedInterfaceTargetSchema("queue.forms.takoform.com", "PullQueue")
	ambiguousTarget["x-takoform-target-formrefs"] = []any{map[string]any{
		"apiVersion": "queue.forms.takoform.com", "kind": "PullQueue", "definitionVersion": "0.1.0",
		"schemaDigest": "sha256:" + strings.Repeat("b", 64),
	}}
	partlyAnnotatedUnion := reviewedTaggedTargetSchema()
	secondBranch := partlyAnnotatedUnion["oneOf"].([]any)[1].(map[string]any)
	delete(secondBranch["properties"].(map[string]any)["topic"].(map[string]any), "x-takoform-required-interface")
	for _, test := range []struct {
		name  string
		field string
		shape map[string]any
	}{
		{name: "command string", field: "command", shape: map[string]any{"type": "string", "maxLength": 4096}},
		{name: "unbounded command list", field: "command", shape: map[string]any{
			"type": "array", "items": map[string]any{"type": "string", "pattern": ".+", "maxLength": 256},
		}},
		{name: "embedded script", field: "script", shape: map[string]any{"type": "string", "maxLength": 4096}},
		{name: "bare target", field: "target", shape: map[string]any{"type": "string", "maxLength": 128}},
		{name: "open object target", field: "target", shape: openTarget},
		{name: "unannotated ResourceTarget", field: "target", shape: unannotatedTarget},
		{name: "ambiguous target contract", field: "target", shape: ambiguousTarget},
		{name: "partly annotated tagged target", field: "target", shape: partlyAnnotatedUnion},
		{name: "backend target", field: "backendTarget", shape: reviewedInterfaceTargetSchema("queue.forms.takoform.com", "PullQueue")},
		{name: "unbounded concurrency target", field: "concurrencyTarget", shape: map[string]any{"type": "integer", "minimum": 1}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			definition := currentFamilyDefinitionFixture(t)
			definition["requiresHostApi"] = "forms.takoform.com/v1"
			properties := definition["desiredSchema"].(map[string]any)["properties"].(map[string]any)
			properties[test.field] = test.shape
			_, err := ValidateDefinition(canonicalMarshal(t, definition))
			if err == nil || !strings.Contains(err.Error(), "forbidden field") {
				t.Fatalf("ValidateDefinition error = %v, want forbidden field", err)
			}
		})
	}
}

func currentFamilyDefinitionFixture(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../forms/candidates/edge.forms.takoform.com/worker-deployment/definition.json")
	if err != nil {
		t.Fatal(err)
	}
	var definition map[string]any
	if err := DecodeStrictIJSON(raw, &definition); err != nil {
		t.Fatal(err)
	}
	return definition
}

// TestValidateDefinitionSelectsTheEmbeddedV1Profile proves the runtime
// parser, not only a direct normative-schema test, owns the new profile. The
// versionless group with an exact stable Host requirement accepts the v1-only
// constraint, while the retained
// versioned group is checked against its own immutable predecessor and cannot
// float onto the current schema.
func TestValidateDefinitionSelectsTheEmbeddedV1Profile(t *testing.T) {
	t.Parallel()
	current := currentFamilyDefinitionFixture(t)
	current["requiresHostApi"] = "forms.takoform.com/v1"
	current["constraints"] = []any{map[string]any{"kind": "acyclic", "reference": "/worker"}}
	raw := canonicalMarshal(t, current)
	decoded, err := ValidateDefinition(raw)
	if err != nil {
		t.Fatalf("versionless v1 Definition was refused by the runtime parser: %v", err)
	}
	if len(decoded.Constraints) != 1 || decoded.Constraints[0].Kind != "acyclic" || decoded.Constraints[0].Reference != "/worker" {
		t.Fatalf("decoded v1 constraint = %#v", decoded.Constraints)
	}

	retained := make(map[string]any, len(current))
	for key, value := range current {
		retained[key] = value
	}
	retained["apiVersion"] = "edge.forms.takoform.com/v1beta1"
	if _, err := ValidateDefinition(canonicalMarshal(t, retained)); err == nil {
		t.Fatal("retained versioned family floated onto the v1 Definition profile")
	}

	occupiedBeta4 := currentFamilyDefinitionFixture(t)
	occupiedBeta4["requiresHostApi"] = "forms.takoform.com/v1beta4"
	occupiedBeta4["constraints"] = []any{map[string]any{"kind": "acyclic", "reference": "/worker"}}
	if _, err := ValidateDefinition(canonicalMarshal(t, occupiedBeta4)); err == nil {
		t.Fatal("occupied beta4 Host lane silently selected the stable v1 Definition profile")
	}
}

func TestEmbeddedV1SchemaPinsIdentityAndConstraintVocabulary(t *testing.T) {
	t.Parallel()
	raw, err := schemaFiles.ReadFile("schemas/form-definition-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := DecodeStrictIJSON(raw, &document); err != nil {
		t.Fatal(err)
	}
	if got := document["$id"]; got != stableFamilyFormDefinitionSchemaID {
		t.Fatalf("embedded v1 $id = %v, want %s", got, stableFamilyFormDefinitionSchemaID)
	}
	properties := document["properties"].(map[string]any)
	constraints := properties["constraints"].(map[string]any)
	items := constraints["items"].(map[string]any)
	branches := items["oneOf"].([]any)
	got := map[string]bool{}
	for _, rawBranch := range branches {
		branch := rawBranch.(map[string]any)
		kindSchema := branch["properties"].(map[string]any)["kind"].(map[string]any)
		kind, ok := kindSchema["const"].(string)
		if !ok || kind == "" || got[kind] {
			t.Fatalf("embedded constraint branch has invalid/duplicate kind: %#v", kindSchema)
		}
		got[kind] = true
	}
	want := []string{
		"exclusive", "sum", "claim", "hostAssigned", "orderedPair", "uniqueBy",
		"acyclic", "distinctPair", "uniquePair", "sameResolvedTarget",
	}
	if len(got) != len(want) {
		t.Fatalf("embedded constraint kinds = %v, want %v", got, want)
	}
	for _, kind := range want {
		if !got[kind] {
			t.Fatalf("embedded v1 schema omits constraint kind %s", kind)
		}
	}
}

func TestZeroSumConstraintPreservesRequiredTotalMember(t *testing.T) {
	t.Parallel()
	base := currentFamilyDefinitionFixture(t)
	raw := canonicalMarshal(t, base)
	var definition FormDefinition
	if err := DecodeStrictIJSON(raw, &definition); err != nil {
		t.Fatal(err)
	}
	definition.Constraints = []FormConstraint{{Kind: "sum", List: "/versions", Member: "weight", Total: 0}}
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"total":0`)) {
		t.Fatalf("zero sum lost its required total member: %s", encoded)
	}
	decoded, err := ValidateDefinition(encoded)
	if err != nil {
		t.Fatalf("valid zero sum was refused after typed encoding: %v", err)
	}
	if len(decoded.Constraints) != 1 || decoded.Constraints[0].Total != 0 {
		t.Fatalf("decoded zero sum = %#v", decoded.Constraints)
	}
}
