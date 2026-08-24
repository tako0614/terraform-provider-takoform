package currentformmodel

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func taggedTargetForm() Form {
	form := semanticForm(Field{
		HCL: "target", Wire: "target", Kind: KindTaggedObject,
		Doc: "Exactly one delivery target.", Required: true,
		Discriminator: "type",
		Variants: []TaggedObjectVariant{
			{
				Tag: "queueMessage",
				Fields: []Field{{
					HCL: "queue", Wire: "queue", Kind: KindResourceRef, Required: true,
					Doc:     "Queue receiving the message.",
					Example: map[string]any{"apiVersion": "queue.forms.takoform.com", "kind": "PullQueue", "name": "jobs"},
					ResourceTarget: &ResourceTarget{
						Group: "queue.forms.takoform.com", Kind: "PullQueue",
						Contract: testInterfaceContract(),
					},
				}},
			},
			{
				Tag: "topicPublish",
				Fields: []Field{{
					HCL: "topic", Wire: "topic", Kind: KindResourceRef, Required: true,
					Doc:     "Topic receiving the message.",
					Example: map[string]any{"apiVersion": "topic.forms.takoform.com", "kind": "Topic", "name": "events"},
					ResourceTarget: &ResourceTarget{
						Group: "topic.forms.takoform.com", Kind: "Topic",
						Contract: testInterfaceContract(),
					},
				}},
			},
		},
		Example: map[string]any{
			"type":  "queueMessage",
			"queue": map[string]any{"apiVersion": "queue.forms.takoform.com", "kind": "PullQueue", "name": "jobs"},
		},
	})
	form.RequiresHostAPI = "forms.takoform.com/v1"
	return form
}

func compileDesiredSchema(t *testing.T, schema map[string]any) *jsonschema.Schema {
	t.Helper()
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const id = "https://example.test/desired.schema.json"
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource(id, document); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(id)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func TestTaggedObjectIsAClosedExactlyOneVariant(t *testing.T) {
	t.Parallel()
	form := taggedTargetForm()
	if err := form.Validate(); err != nil {
		t.Fatal(err)
	}
	schema := mustDesiredSchema(t, form)
	compiled := compileDesiredSchema(t, schema)
	queue := map[string]any{"target": map[string]any{
		"type":  "queueMessage",
		"queue": map[string]any{"apiVersion": "queue.forms.takoform.com", "kind": "PullQueue", "name": "jobs"},
	}}
	if err := compiled.Validate(queue); err != nil {
		t.Fatalf("selected queue variant was rejected: %v", err)
	}
	for name, invalid := range map[string]any{
		"unknown discriminator": map[string]any{"target": map[string]any{"type": "unknown"}},
		"branch member disagrees": map[string]any{"target": map[string]any{
			"type":  "queueMessage",
			"topic": map[string]any{"apiVersion": "topic.forms.takoform.com", "kind": "Topic", "name": "events"},
		}},
		"members from two branches": map[string]any{"target": map[string]any{
			"type":  "queueMessage",
			"queue": map[string]any{"apiVersion": "queue.forms.takoform.com", "kind": "PullQueue", "name": "jobs"},
			"topic": map[string]any{"apiVersion": "topic.forms.takoform.com", "kind": "Topic", "name": "events"},
		}},
	} {
		if err := compiled.Validate(invalid); err == nil {
			t.Errorf("%s was accepted: %#v", name, invalid)
		}
	}
}

func TestRelationsTraverseOnlyTheSelectedTaggedVariant(t *testing.T) {
	t.Parallel()
	form := taggedTargetForm()
	relations, err := DeriveRelations(mustDesiredSchema(t, form))
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 2 {
		t.Fatalf("derived %d relations, want one per closed variant", len(relations))
	}
	spec := map[string]any{"target": map[string]any{
		"type":  "queueMessage",
		"queue": map[string]any{"apiVersion": "queue.forms.takoform.com", "kind": "PullQueue", "name": "jobs"},
		// This is schema-invalid on purpose. Relation traversal still must never
		// read a non-selected branch if a caller violates the validate-first order.
		"topic": map[string]any{"apiVersion": "topic.forms.takoform.com", "kind": "Topic", "name": "events"},
	}}
	instances := RelationInstances(relations, spec)
	if len(instances) != 1 || instances[0].TargetName != "jobs" || instances[0].Pointer != "/target/queue" {
		t.Fatalf("selected relation instances = %#v", instances)
	}
}
