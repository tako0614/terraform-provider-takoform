package currentformmodel

import "testing"

// referencingSchema is the smallest desired schema that declares one relation,
// so a constraint naming it has something real to attach to.
func referencingSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"worker": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"apiVersion", "kind", "name"},
				"properties": map[string]any{
					"apiVersion": map[string]any{"type": "string", "const": "edge.forms.takoform.com"},
					"kind":       map[string]any{"type": "string", "const": "ModuleWorker"},
					"name":       map[string]any{"type": "string", "minLength": 1, "maxLength": 63},
				},
				"x-takoform-target-formrefs": []any{
					map[string]any{
						"apiVersion":        "edge.forms.takoform.com",
						"kind":              "ModuleWorker",
						"definitionVersion": "0.1.0",
						"schemaDigest": "sha256:0000000000000000000000000000000000" +
							"000000000000000000000000000000",
					},
				},
			},
		},
	}
}

// TestConstraintVocabularyIsClosed proves an entry this host cannot act on is
// REFUSED rather than skipped.
//
// The derivation used to `continue` past everything that was not an exclusive
// hold. A Form could then declare a rule of a kind this host does not
// implement, install cleanly, and enforce nothing — the Definition promising a
// constraint no one keeps, with no signal anywhere. The lane document now says
// a host refuses such a Form at install time, and this is what makes that true
// rather than aspirational.
func TestConstraintVocabularyIsClosed(t *testing.T) {
	t.Parallel()
	for name, testCase := range map[string]struct {
		constraint Constraint
		valid      bool
	}{
		"exclusive over a declared reference": {Constraint{Kind: ConstraintExclusive, Reference: "/worker"}, true},
		"summed member":                       {Constraint{Kind: ConstraintSum, List: "/targets", Member: "weight", Total: 10000}, true},
		"claimed property":                    {Constraint{Kind: ConstraintClaim, Property: "/hostname"}, true},
		"host-assigned output":                {Constraint{Kind: ConstraintHostAssigned, Output: "/address"}, true},

		"exclusive naming nothing":       {Constraint{Kind: ConstraintExclusive}, false},
		"exclusive naming no reference":  {Constraint{Kind: ConstraintExclusive, Reference: "/absent"}, false},
		"sum naming nothing":             {Constraint{Kind: ConstraintSum}, false},
		"sum with no member":             {Constraint{Kind: ConstraintSum, List: "/targets", Total: 1}, false},
		"claim naming no property":       {Constraint{Kind: ConstraintClaim}, false},
		"host-assigned naming no output": {Constraint{Kind: ConstraintHostAssigned}, false},
		"a kind nobody implements":       {Constraint{Kind: "somethingNew", Reference: "/worker"}, false},
	} {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := DeriveRelationsWithConstraints(referencingSchema(), []Constraint{testCase.constraint})
			if testCase.valid && err != nil {
				t.Fatalf("%+v was refused: %v", testCase.constraint, err)
			}
			if !testCase.valid && err == nil {
				t.Fatalf("%+v was accepted", testCase.constraint)
			}
		})
	}
}
