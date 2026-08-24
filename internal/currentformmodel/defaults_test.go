package currentformmodel

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func semanticForm(fields ...Field) Form {
	return Form{
		Family: Family{Group: "edge.forms.takoform.com", Version: "v1alpha1"},
		Kind:   "ExampleIdentity", Slug: "example-identity",
		RequiresHostAPI: "forms.takoform.com/v1beta1", Role: RoleIdentity, Title: "Example Identity", Description: "Identity.",
		DefinitionVersion: "0.1.0", Fields: fields,
	}
}

// TestOptionalFieldWithoutMeaningIsRejected proves the boundary rule: an
// optional field that declares neither a portable default nor an explicit
// absent-case semantics leaves two conforming hosts free to disagree, so it
// never reaches a Form Definition.
func TestOptionalFieldWithoutMeaningIsRejected(t *testing.T) {
	t.Parallel()
	form := semanticForm(Field{
		HCL: "retention_seconds", Wire: "retentionSeconds", Kind: KindInteger,
		Min: I64(60), Max: I64(600), Doc: "Retention bound.",
	})
	err := form.Validate()
	if err == nil {
		t.Fatal("an optional field with neither Default nor AbsenceIsSemantic must fail authoring validation")
	}
	if !strings.Contains(err.Error(), "retentionSeconds") ||
		!strings.Contains(err.Error(), "AbsenceIsSemantic") {
		t.Fatalf("validate error = %v, want it to name the field and the two ways out", err)
	}

	// Either way out satisfies the rule.
	defaulted := semanticForm(Field{
		HCL: "retention_seconds", Wire: "retentionSeconds", Kind: KindInteger,
		Min: I64(60), Max: I64(600), Default: 300, Doc: "Retention bound.",
	})
	if err := defaulted.Validate(); err != nil {
		t.Fatalf("a defaulted optional field was rejected: %v", err)
	}
	semantic := semanticForm(Field{
		HCL: "dead_letter_queue", Wire: "deadLetterQueue", Kind: KindResourceRef,
		TargetKind: "AtLeastOnceQueue", Target: testInterfaceContract(), AbsenceIsSemantic: true,
		Doc: "Queue receiving exhausted messages. Without it, exhausted messages are dropped.",
	})
	if err := semantic.Validate(); err != nil {
		t.Fatalf("an absence-is-semantics field was rejected: %v", err)
	}
}

// TestAbsenceIsSemanticRequiresStatedBehavior keeps the exemption auditable:
// the marker alone never suffices, the Doc must say what absence does.
func TestAbsenceIsSemanticRequiresStatedBehavior(t *testing.T) {
	t.Parallel()
	form := semanticForm(Field{
		HCL: "dead_letter_queue", Wire: "deadLetterQueue", Kind: KindResourceRef,
		TargetKind: "AtLeastOnceQueue", Target: testInterfaceContract(), AbsenceIsSemantic: true,
		Doc: "Queue receiving messages that exhausted their retries.",
	})
	err := form.Validate()
	if err == nil || !strings.Contains(err.Error(), "absent-case behavior") {
		t.Fatalf("validate error = %v, want the unstated-absent-case rule", err)
	}
}

// TestMalformedDefaultsAreRejected proves a declared default is held to the
// same schema its own field emits.
func TestMalformedDefaultsAreRejected(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name  string
		field Field
		want  string
	}{
		{
			name: "integer below the declared minimum",
			field: Field{
				HCL: "retention_seconds", Wire: "retentionSeconds", Kind: KindInteger,
				Min: I64(60), Max: I64(600), Default: 59, Doc: "Retention bound.",
			},
			want: "below the declared minimum",
		},
		{
			name: "integer above the declared maximum",
			field: Field{
				HCL: "retention_seconds", Wire: "retentionSeconds", Kind: KindInteger,
				Min: I64(60), Max: I64(600), Default: 601, Doc: "Retention bound.",
			},
			want: "above the declared maximum",
		},
		{
			name: "string outside the declared enum",
			field: Field{
				HCL: "mode", Wire: "mode", Kind: KindStringEnum,
				Enum: []string{"fast", "slow"}, Default: "medium", Doc: "Mode.",
			},
			want: "not a declared enum member",
		},
		{
			name: "set item outside the declared enum",
			field: Field{
				HCL: "flags", Wire: "flags", Kind: KindStringSet,
				Enum: []string{"declared_flag"}, Default: []any{"undeclared_flag"}, Doc: "Flags.",
			},
			want: "outside the declared enum",
		},
		{
			name: "set item that violates the item pattern",
			field: Field{
				HCL: "sensitive", Wire: "sensitive", Kind: KindStringSet,
				ItemPattern: PatternSensitiveVarName, Default: []any{"lowercase"}, Doc: "Names.",
			},
			want: "does not match",
		},
		{
			name: "wrong shape entirely",
			field: Field{
				HCL: "vars", Wire: "vars", Kind: KindJSONMap,
				Default: []any{}, Doc: "Vars.",
			},
			want: "is not a JSON object",
		},
		{
			name: "map key outside the portable key grammar",
			field: Field{
				HCL: "vars", Wire: "vars", Kind: KindJSONMap,
				Default: map[string]any{"1 invalid": "value"}, Doc: "Vars.",
			},
			want: "does not match",
		},
		{
			name: "string outside the declared pattern",
			field: Field{
				HCL: "hostname", Wire: "hostname", Kind: KindString,
				Pattern: PatternHostname, MaxLength: 253, Default: "not a hostname", Doc: "Host.",
			},
			want: "does not match",
		},
	} {
		err := semanticForm(testCase.field).Validate()
		if err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Errorf("%s: validate error = %v, want it to contain %q", testCase.name, err, testCase.want)
		}
	}
}

// JSON Schema maxLength counts Unicode code points, not UTF-8 bytes. Authoring
// validation must use the same unit or a portable default accepted by the
// emitted schema can be rejected merely because it contains non-ASCII text.
func TestDefaultStringLengthsCountUnicodeCodePoints(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name  string
		field Field
	}{
		{
			name: "string",
			field: Field{HCL: "label", Wire: "label", Kind: KindString,
				Pattern: `^..$`, MaxLength: 2, Default: "界界", Doc: "Label."},
		},
		{
			name: "ordered string list",
			field: Field{HCL: "labels", Wire: "labels", Kind: KindStringList,
				ItemPattern: `^..$`, MaxLength: 2, MaxItems: 4, Default: []any{"界界"}, Doc: "Labels."},
		},
		{
			name: "string map",
			field: Field{HCL: "labels", Wire: "labels", Kind: KindStringMap,
				ItemPattern: `^..$`, MaxLength: 2, MaxProperties: 4,
				Default: map[string]any{"label": "界界"}, Doc: "Labels."},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if err := semanticForm(testCase.field).Validate(); err != nil {
				t.Fatalf("two-code-point default was rejected by a maxLength 2 field: %v", err)
			}
		})
	}

	overlong := Field{HCL: "label", Wire: "label", Kind: KindString,
		Pattern: `^...$`, MaxLength: 2, Default: "界界界", Doc: "Label."}
	if err := semanticForm(overlong).Validate(); err == nil || !strings.Contains(err.Error(), "maxLength 2") {
		t.Fatalf("three-code-point default error = %v, want maxLength rejection", err)
	}
}

// TestRequiredFieldMustNotDeclareDefault: a required value is never omitted,
// so a default on it would be dead normative text.
func TestRequiredFieldMustNotDeclareDefault(t *testing.T) {
	t.Parallel()
	form := semanticForm(Field{
		HCL: "retention_seconds", Wire: "retentionSeconds", Kind: KindInteger,
		Required: true, Min: I64(60), Max: I64(600), Default: 300,
		Doc: "Retention bound.", Example: 300,
	})
	if err := form.Validate(); err == nil || !strings.Contains(err.Error(), "never omitted") {
		t.Fatalf("validate error = %v, want the required-with-default rule", err)
	}
}

// TestNestedDefaultsAreValidated proves the well-formedness rule reaches
// object-list members, not only top-level properties.
func TestNestedDefaultsAreValidated(t *testing.T) {
	t.Parallel()
	form := semanticForm(Field{
		HCL: "versions", Wire: "versions", Kind: KindObjectList, Required: true, MinItems: 1,
		Doc: "Entries.", Example: []any{map[string]any{"weight": 1}},
		Fields: []Field{{
			HCL: "weight", Wire: "weight", Kind: KindInteger,
			Min: I64(1), Max: I64(10000), Default: 0, Doc: "Weight.",
		}},
	})
	if err := form.Validate(); err == nil || !strings.Contains(err.Error(), "below the declared minimum") {
		t.Fatalf("validate error = %v, want the nested default to be checked", err)
	}
}

func materializeForm() Form {
	return semanticForm(
		Field{
			HCL: "retention_seconds", Wire: "retentionSeconds", Kind: KindInteger,
			Min: I64(60), Max: I64(600), Default: 300, Doc: "Retention bound.",
		},
		Field{
			HCL: "flags", Wire: "flags", Kind: KindStringSet,
			Enum: []string{"declared_flag"}, Default: []any{}, Doc: "Flags.",
		},
		Field{
			HCL: "vars", Wire: "vars", Kind: KindJSONMap,
			Default: map[string]any{}, Doc: "Vars.",
		},
	)
}

// TestOmittedAndWrittenDefaultsProduceOneEffectiveSpec is the whole point of
// the feature: the two spellings are one desired state.
func TestOmittedAndWrittenDefaultsProduceOneEffectiveSpec(t *testing.T) {
	t.Parallel()
	schema := mustDesiredSchema(t, materializeForm())
	omitted := MaterializeDefaults(schema, map[string]any{})
	written := MaterializeDefaults(schema, map[string]any{
		"retentionSeconds": json.Number("300"),
		"flags":            []any{},
		"vars":             map[string]any{},
	})
	if !reflect.DeepEqual(omitted, written) {
		t.Fatalf("omitted = %#v, written = %#v", omitted, written)
	}
	omittedJSON, err := json.Marshal(omitted)
	if err != nil {
		t.Fatal(err)
	}
	writtenJSON, err := json.Marshal(written)
	if err != nil {
		t.Fatal(err)
	}
	if string(omittedJSON) != string(writtenJSON) {
		t.Fatalf("effective specs differ: %s vs %s", omittedJSON, writtenJSON)
	}
}

// TestMaterializationNeverOverwritesAndIsIdempotent covers the two properties
// every caller depends on.
func TestMaterializationNeverOverwritesAndIsIdempotent(t *testing.T) {
	t.Parallel()
	schema := mustDesiredSchema(t, materializeForm())
	written := map[string]any{"retentionSeconds": json.Number("600")}
	once := MaterializeDefaults(schema, written)
	if once["retentionSeconds"] != json.Number("600") {
		t.Fatalf("materialization overwrote a present value: %#v", once["retentionSeconds"])
	}
	twice := MaterializeDefaults(schema, once)
	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("materialization is not idempotent: %#v vs %#v", once, twice)
	}
	if len(written) != 1 {
		t.Fatalf("materialization mutated its argument: %#v", written)
	}
	// A value equal to the default is still a written value and stays put.
	equal := MaterializeDefaults(schema, map[string]any{"retentionSeconds": json.Number("300")})
	if equal["retentionSeconds"] != json.Number("300") {
		t.Fatalf("materialization replaced an explicitly-written default: %#v", equal["retentionSeconds"])
	}
}

// TestMaterializedNumbersAreJSONNumber pins the representation host semantic
// rules depend on: host bodies decode with UseNumber, so a materialized number
// that arrived as a Go int would break every rule that type-asserts
// json.Number.
func TestMaterializedNumbersAreJSONNumber(t *testing.T) {
	t.Parallel()
	schema := mustDesiredSchema(t, materializeForm())
	materialized := MaterializeDefaults(schema, nil)
	number, ok := materialized["retentionSeconds"].(json.Number)
	if !ok {
		t.Fatalf("materialized default is %T, want json.Number", materialized["retentionSeconds"])
	}
	if number.String() != "300" {
		t.Fatalf("materialized default text = %q, want canonical decimal 300", number.String())
	}
	// The same holds after a round trip through the wire, which is how a host
	// actually receives a Definition.
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := MaterializeDefaults(decoded, nil)["retentionSeconds"].(json.Number); !ok {
		t.Fatal("a wire-decoded Definition must still materialize json.Number")
	}
}

// TestMaterializedValuesAreDeepCopies proves a caller cannot reach back into
// the Definition through the spec it was handed.
func TestMaterializedValuesAreDeepCopies(t *testing.T) {
	t.Parallel()
	schema := mustDesiredSchema(t, materializeForm())
	first := MaterializeDefaults(schema, nil)
	vars, ok := first["vars"].(map[string]any)
	if !ok {
		t.Fatalf("materialized vars is %T", first["vars"])
	}
	vars["INJECTED"] = "value"
	second := MaterializeDefaults(schema, nil)
	if len(second["vars"].(map[string]any)) != 0 {
		t.Fatalf("mutating a materialized value reached the Definition: %#v", second["vars"])
	}
}

// TestRecursiveDefaultsRespectSemanticAbsence proves both halves of nested
// defaulting. A present object/object-list is materialized recursively, while
// an absent optional object whose absence is the contract stays absent; child
// defaults never synthesize their parent.
func TestRecursiveDefaultsRespectSemanticAbsence(t *testing.T) {
	t.Parallel()
	form := semanticForm(
		Field{
			HCL: "policy", Wire: "policy", Kind: KindObject,
			Doc:               "Delivery policy. When absent, the target applies no retry policy.",
			AbsenceIsSemantic: true,
			Fields: []Field{{
				HCL: "attempts", Wire: "attempts", Kind: KindInteger,
				Doc: "Attempt count.", Min: I64(1), Max: I64(10), Default: 3,
			}},
		},
		Field{
			HCL: "targets", Wire: "targets", Kind: KindObjectList,
			Doc: "Delivery targets.", Required: true, MinItems: 1, MaxItems: 4,
			Example: []any{map[string]any{"options": map[string]any{}}},
			Fields: []Field{{
				HCL: "options", Wire: "options", Kind: KindObject,
				Doc: "Target options.", Required: true, Example: map[string]any{},
				Fields: []Field{{
					HCL: "timeout_seconds", Wire: "timeoutSeconds", Kind: KindInteger,
					Doc: "Timeout.", Min: I64(1), Max: I64(60), Default: 30,
				}},
			}},
		},
	)
	if err := form.Validate(); err != nil {
		t.Fatal(err)
	}
	schema := mustDesiredSchema(t, form)

	absent := MaterializeDefaults(schema, map[string]any{
		"targets": []any{map[string]any{"options": map[string]any{}}},
	})
	if _, present := absent["policy"]; present {
		t.Fatalf("semantic absence synthesized policy: %#v", absent)
	}
	targets := absent["targets"].([]any)
	options := targets[0].(map[string]any)["options"].(map[string]any)
	if got := options["timeoutSeconds"]; got != json.Number("30") {
		t.Fatalf("recursive object-list default = %#v, want json.Number(30)", got)
	}

	present := MaterializeDefaults(schema, map[string]any{
		"policy":  map[string]any{},
		"targets": []any{map[string]any{"options": map[string]any{}}},
	})
	policy := present["policy"].(map[string]any)
	if got := policy["attempts"]; got != json.Number("3") {
		t.Fatalf("present object default = %#v, want json.Number(3)", got)
	}
}

func TestRecursiveObjectDefaultsAreValidatedAgainstMembers(t *testing.T) {
	t.Parallel()
	form := semanticForm(Field{
		HCL: "policy", Wire: "policy", Kind: KindObject,
		Doc:     "Policy.",
		Default: map[string]any{"attempts": 0, "unknown": true},
		Fields: []Field{{
			HCL: "attempts", Wire: "attempts", Kind: KindInteger,
			Doc: "Attempt count.", Required: true, Example: 1, Min: I64(1), Max: I64(10),
		}},
	})
	err := form.Validate()
	if err == nil || (!strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "minimum")) {
		t.Fatalf("Validate error = %v, want recursive member validation", err)
	}
}
