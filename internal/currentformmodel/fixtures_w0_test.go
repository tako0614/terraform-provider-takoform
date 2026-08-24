package currentformmodel

import "testing"

func TestNegativeCasesCoverEveryNewFieldKindAndNestedMember(t *testing.T) {
	t.Parallel()
	args := Field{
		HCL: "args", Wire: "args", Kind: KindStringList, Required: true, Doc: "Ordered arguments.",
		ItemPattern: `^[a-z-]+$`, MaxLength: 16, MaxItems: 8, Example: []any{"same", "same"},
	}
	attributes := Field{
		HCL: "attributes", Wire: "attributes", Kind: KindStringMap, Required: true, Doc: "Typed attributes.",
		ItemPattern: `^[a-z]+$`, MaxLength: 16, MaxProperties: 8, Example: map[string]any{"Team": "core"},
	}
	filterPolicy := Field{
		HCL: "filter_policy", Wire: "filterPolicy", Kind: KindStringSetMap, Required: true, Doc: "Typed filter sets.",
		ItemPattern: `^[a-z]+$`, MaxLength: 16, MaxItems: 8, MaxProperties: 8,
		Example: map[string]any{"color": []any{"blue", "red"}},
	}
	options := Field{
		HCL: "options", Wire: "options", Kind: KindObject, Required: true, Doc: "Recursive options.",
		Example: map[string]any{"nested": map[string]any{"args": []any{"same", "same"}}},
		Fields: []Field{{
			HCL: "nested", Wire: "nested", Kind: KindObject, Required: true, Doc: "Nested object.",
			Fields: []Field{{
				HCL: "args", Wire: "args", Kind: KindStringList, Required: true, Doc: "Nested ordered args.",
				ItemPattern: `^[a-z-]+$`, MaxLength: 16, MaxItems: 8,
			}},
		}},
	}
	items := Field{
		HCL: "items", Wire: "items", Kind: KindObjectList, Required: true, Doc: "Recursive object list.", MaxItems: 8,
		Example: []any{map[string]any{"attributes": map[string]any{"Team": "core"}}},
		Fields: []Field{{
			HCL: "attributes", Wire: "attributes", Kind: KindStringMap, Required: true, Doc: "Nested typed attributes.",
			ItemPattern: `^[a-z]+$`, MaxLength: 16, MaxProperties: 8,
		}},
	}
	target := Field{
		HCL: "target", Wire: "target", Kind: KindTaggedObject, Required: true, Doc: "Closed target.",
		Discriminator: "type", Example: map[string]any{"type": "queue", "queue": map[string]any{
			"apiVersion": "queue.forms.example", "kind": "Queue", "name": "jobs",
		}},
		Variants: []TaggedObjectVariant{
			{Tag: "queue", Fields: []Field{exactRefField("queue", "queue", "queue.forms.example", "Queue")}},
			{Tag: "topic", Fields: []Field{exactRefField("topic", "topic", "topic.forms.example", "Topic")}},
		},
	}
	form := semanticForm(args, attributes, filterPolicy, options, items, target)
	form.Family = Family{Group: "fixture.forms.example"}
	form.RequiresHostAPI = "forms.takoform.com/v1"
	if err := form.Validate(); err != nil {
		t.Fatal(err)
	}
	cases, err := form.NegativeCases()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]bool{}
	for _, item := range cases {
		byName[item.Name] = true
	}
	for _, want := range []string{
		"args", "attributes", "filter-policy", "options", "items", "target",
		"options-nested-args", "items-attributes", "target-queue-queue",
	} {
		if !byName[want] {
			t.Errorf("missing W0 negative case %q (have %v)", want, names(cases))
		}
	}

	compiled := compileDesiredSchema(t, mustDesiredSchema(t, form))
	for _, item := range cases {
		if err := compiled.Validate(item.Desired); err == nil {
			t.Errorf("negative case %s unexpectedly satisfies desiredSchema: %#v", item.Name, item.Desired)
		}
	}
}
