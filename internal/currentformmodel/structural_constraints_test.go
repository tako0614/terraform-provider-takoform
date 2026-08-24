package currentformmodel

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func structuralConstraintForm() Form {
	form := semanticForm(
		Field{HCL: "minimum", Wire: "minimum", Kind: KindInteger, Required: true, Min: I64(0), Max: I64(100), Doc: "Lower bound.", Example: 1},
		Field{HCL: "maximum", Wire: "maximum", Kind: KindInteger, Required: true, Min: I64(0), Max: I64(100), Doc: "Upper bound.", Example: 10},
		Field{HCL: "indexes", Wire: "indexes", Kind: KindObjectList, MaxItems: 8, Default: []any{}, Doc: "Named indexes. Omitting it declares no indexes.", Fields: []Field{
			{HCL: "name", Wire: "name", Kind: KindString, Required: true, Pattern: PatternResourceName, MaxLength: ResourceNameMaxLength, Doc: "Unique index name.", Example: "by-name"},
			{HCL: "enabled", Wire: "enabled", Kind: KindBoolean, Required: true, Doc: "Whether enabled.", Example: true},
		}},
	)
	form.RequiresHostAPI = "forms.takoform.com/v1"
	form.StructuralConstraints = []Constraint{
		{Kind: ConstraintOrderedPair, References: []string{"/minimum", "/maximum"}},
		{Kind: ConstraintUniqueBy, List: "/indexes", Member: "name"},
	}
	return form
}

func TestStructuralConstraintGrammarAndSchemaDomains(t *testing.T) {
	t.Parallel()
	form := structuralConstraintForm()
	if err := form.Validate(); err != nil {
		t.Fatal(err)
	}
	want := form.StructuralConstraints
	if got := form.Constraints(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Constraints = %#v, want %#v", got, want)
	}
	schema, err := form.DesiredSchema(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DeriveRelationsWithConstraints(schema, form.Constraints()); err != nil {
		t.Fatalf("rendered structural constraint domains were refused: %v", err)
	}
}

func TestStructuralConstraintsRejectWrongPointersAndDomains(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*Form){
		"ordered pair repeats a pointer": func(form *Form) {
			form.StructuralConstraints[0].References[1] = "/minimum"
		},
		"ordered pair points at optional value": func(form *Form) {
			form.Fields[1].Required = false
			form.Fields[1].Default = 10
		},
		"ordered pair points at non-number": func(form *Form) {
			form.StructuralConstraints[0].References[1] = "/indexes"
		},
		"uniqueBy points at non-list": func(form *Form) {
			form.StructuralConstraints[1].List = "/minimum"
		},
		"uniqueBy member is optional": func(form *Form) {
			form.Fields[2].Fields[0].Required = false
			form.Fields[2].Fields[0].Default = "by-name"
		},
		"uniqueBy member is non-scalar": func(form *Form) {
			form.StructuralConstraints[1].Member = "missing"
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			form := structuralConstraintForm()
			mutate(&form)
			if err := form.Validate(); err == nil || !strings.Contains(err.Error(), "constraint") &&
				!strings.Contains(err.Error(), "orderedPair") && !strings.Contains(err.Error(), "uniqueBy") {
				t.Fatalf("Validate error = %v, want structural constraint refusal", err)
			}
		})
	}
}

func TestStructuralConstraintsRequireV1Beta4HostLane(t *testing.T) {
	t.Parallel()
	form := structuralConstraintForm()
	form.RequiresHostAPI = "forms.takoform.com/v1beta4"
	if err := form.Validate(); err == nil || !strings.Contains(err.Error(), "forms.takoform.com/v1") {
		t.Fatalf("Validate error = %v, want mechanism-derived stable-v1 minimum", err)
	}
}

func TestValidateStructuralConstraintValuesRejectsOrderingAndDuplicateMembers(t *testing.T) {
	t.Parallel()
	constraints := structuralConstraintForm().StructuralConstraints
	valid := map[string]any{
		"minimum": json.Number("1"), "maximum": json.Number("10"),
		"indexes": []any{map[string]any{"name": "by-name"}, map[string]any{"name": "by-time"}},
	}
	if err := ValidateStructuralConstraintValues(constraints, valid); err != nil {
		t.Fatalf("valid structural values were rejected: %v", err)
	}

	wrongOrder := map[string]any{
		"minimum": json.Number("11"), "maximum": json.Number("10"), "indexes": []any{},
	}
	if err := ValidateStructuralConstraintValues(constraints, wrongOrder); err == nil || !strings.Contains(err.Error(), "<=") {
		t.Fatalf("orderedPair error = %v, want ordering rejection", err)
	}

	duplicate := map[string]any{
		"minimum": json.Number("1"), "maximum": json.Number("10"),
		"indexes": []any{map[string]any{"name": "by-name"}, map[string]any{"name": "by-name"}},
	}
	if err := ValidateStructuralConstraintValues(constraints, duplicate); err == nil || !strings.Contains(err.Error(), "repeat") {
		t.Fatalf("uniqueBy error = %v, want duplicate rejection", err)
	}
}
