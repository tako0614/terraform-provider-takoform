package currentformmodel

import (
	"reflect"
	"strings"
	"testing"
)

// TestOrderedStringListPreservesOrderAndDuplicates distinguishes command/args
// style values from a set. Repeating an argument is meaningful and no schema,
// default, or fixture layer may silently deduplicate or reorder it.
func TestOrderedStringListPreservesOrderAndDuplicates(t *testing.T) {
	t.Parallel()
	form := semanticForm(Field{
		HCL: "args", Wire: "args", Kind: KindStringList,
		Doc:               "Ordered process arguments. When omitted, the image arguments apply.",
		AbsenceIsSemantic: true,
		ItemPattern:       `^[a-z-]+$`, MaxLength: 32, MaxItems: 8,
		Example: []any{"retry", "retry", "once"},
	})
	if err := form.Validate(); err != nil {
		t.Fatal(err)
	}
	schema := mustDesiredSchema(t, form)
	args := schema["properties"].(map[string]any)["args"].(map[string]any)
	if _, present := args["uniqueItems"]; present {
		t.Fatalf("ordered string-list emitted uniqueItems: %#v", args)
	}
	want := []any{"retry", "retry", "once"}
	if got := form.CanonicalDesired()["args"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical args = %#v, want %#v", got, want)
	}
}

// TestBoundedStringMapsRenderClosedSchemasAndCanonicalSetValues covers the two
// map shapes W0 needs without introducing an arbitrary JSON value surface.
// RFC 8785 orders object keys at encoding time; set-valued map arrays are
// additionally sorted here so defaults and fixtures have one spelling.
func TestBoundedStringMapsRenderClosedSchemasAndCanonicalSetValues(t *testing.T) {
	t.Parallel()
	form := semanticForm(
		Field{
			HCL: "attributes", Wire: "attributes", Kind: KindStringMap,
			Doc: "Message attributes.", Default: map[string]any{"z": "last", "a": "first"},
			ItemPattern: `^[a-z]+$`, MaxLength: 16, MaxProperties: 10,
		},
		Field{
			HCL: "filter_policy", Wire: "filterPolicy", Kind: KindStringSetMap,
			Doc:         "Attribute inclusion filter.",
			Default:     map[string]any{"status": []any{"pending", "done"}},
			Example:     map[string]any{"status": []any{"pending", "done"}},
			ItemPattern: `^[a-z]+$`, MaxLength: 16,
			MinItems: 1, MaxItems: 16, MaxProperties: 10,
		},
	)
	if err := form.Validate(); err != nil {
		t.Fatal(err)
	}
	schema := mustDesiredSchema(t, form)
	properties := schema["properties"].(map[string]any)
	attributes := properties["attributes"].(map[string]any)
	if got := attributes["maxProperties"]; got != 10 {
		t.Fatalf("attributes maxProperties = %v, want 10", got)
	}
	attributeValue := attributes["additionalProperties"].(map[string]any)
	if attributeValue["type"] != "string" || attributeValue["maxLength"] != 16 {
		t.Fatalf("attributes value schema = %#v", attributeValue)
	}

	filter := properties["filterPolicy"].(map[string]any)
	set := filter["additionalProperties"].(map[string]any)
	if set["uniqueItems"] != true || set["minItems"] != 1 || set["maxItems"] != 16 {
		t.Fatalf("filterPolicy set schema = %#v", set)
	}
	wantSet := []any{"done", "pending"}
	defaultSet := filter["default"].(map[string]any)["status"]
	if !reflect.DeepEqual(defaultSet, wantSet) {
		t.Fatalf("filter default = %#v, want lexical %#v", defaultSet, wantSet)
	}
	exampleSet := form.CanonicalDesired()["filterPolicy"].(map[string]any)["status"]
	if !reflect.DeepEqual(exampleSet, wantSet) {
		t.Fatalf("filter fixture = %#v, want lexical %#v", exampleSet, wantSet)
	}
}

func TestStringMapsRefuseUnboundedOrDuplicateDefaults(t *testing.T) {
	t.Parallel()
	for name, field := range map[string]Field{
		"string map has no property ceiling": {
			HCL: "attributes", Wire: "attributes", Kind: KindStringMap,
			Doc: "Attributes.", Default: map[string]any{}, ItemPattern: `^[a-z]+$`, MaxLength: 16,
		},
		"set map has no value ceiling": {
			HCL: "filter_policy", Wire: "filterPolicy", Kind: KindStringSetMap,
			Doc: "Filter.", Default: map[string]any{}, ItemPattern: `^[a-z]+$`, MaxLength: 16,
			MaxProperties: 10,
		},
		"set map default repeats a value": {
			HCL: "filter_policy", Wire: "filterPolicy", Kind: KindStringSetMap,
			Doc: "Filter.", Default: map[string]any{"status": []any{"done", "done"}},
			ItemPattern: `^[a-z]+$`, MaxLength: 16, MaxItems: 16, MaxProperties: 10,
		},
		"string map has contradictory property bounds": {
			HCL: "attributes", Wire: "attributes", Kind: KindStringMap,
			Doc: "Attributes.", Required: true, Example: map[string]any{"one": "value"},
			ItemPattern: `^[a-z]+$`, MaxLength: 16, MinProperties: 2, MaxProperties: 1,
		},
		"ordered list has contradictory item bounds": {
			HCL: "args", Wire: "args", Kind: KindStringList,
			Doc: "Arguments.", Required: true, Example: []any{"one"},
			ItemPattern: `^[a-z]+$`, MaxLength: 16, MinItems: 2, MaxItems: 1,
		},
	} {
		name, field := name, field
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := semanticForm(field).Validate()
			if err == nil || (!strings.Contains(err.Error(), "bound") && !strings.Contains(err.Error(), "duplicate")) {
				t.Fatalf("Validate error = %v, want bounded/duplicate refusal", err)
			}
		})
	}
}
