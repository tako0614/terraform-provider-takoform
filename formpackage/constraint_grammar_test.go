package formpackage

import (
	"encoding/json"
	"os"
	"testing"
)

// TestConstraintEntriesCarryExactlyTheirKindsMembers holds the constraint list
// to a per-kind grammar.
//
// Decision 0049 moved behavioural rules out of the desired schema's extension
// slots and into a first-class list, on the argument that a closed vocabulary
// in one place is what a second implementer can read. A list whose entries
// require only `kind` does not deliver that: `{"kind":"sum"}` validated, and so
// did a `claim` carrying a sum's members. A package gate passed both, and the
// only thing left to notice was a host failing every apply on a rule it could
// not read — which is the divergence between package validation and host
// behaviour this list exists to remove.
func TestConstraintEntriesCarryExactlyTheirKindsMembers(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(
		"../forms/candidates/edge.forms.takoform.com/worker-deployment/definition.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	var base map[string]any
	if err := json.Unmarshal(raw, &base); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateDefinition(canonicalMarshal(t, base)); err != nil {
		t.Fatalf("the published Definition this test builds on does not validate: %v", err)
	}

	for name, testCase := range map[string]struct {
		entry string
		valid bool
	}{
		// Every kind, with exactly what its rule needs.
		"exclusive over a reference":   {`{"kind":"exclusive","reference":"/worker"}`, true},
		"exclusive keyed by a sibling": {`{"kind":"exclusive","reference":"/worker","keyedBy":"/className"}`, true},
		"sum over a list":              {`{"kind":"sum","list":"/targets","member":"weight","total":10000}`, true},
		"claim over a property":        {`{"kind":"claim","property":"/hostname"}`, true},
		"host-assigned output":         {`{"kind":"hostAssigned","output":"/address"}`, true},
		// A kind with none of its members states no rule at all.
		"exclusive with no reference":  {`{"kind":"exclusive"}`, false},
		"sum with nothing to add":      {`{"kind":"sum"}`, false},
		"claim with no property":       {`{"kind":"claim"}`, false},
		"host-assigned with no output": {`{"kind":"hostAssigned"}`, false},
		// A kind carrying another kind's members names two rules and is neither.
		"claim wearing a sum's members":  {`{"kind":"claim","list":"/x","member":"weight","total":10}`, false},
		"claim with an output":           {`{"kind":"claim","property":"/hostname","output":"/address"}`, false},
		"sum missing its total":          {`{"kind":"sum","list":"/targets","member":"weight"}`, false},
		"host-assigned with a reference": {`{"kind":"hostAssigned","output":"/address","reference":"/worker"}`, false},
	} {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var entry map[string]any
			if err := json.Unmarshal([]byte(testCase.entry), &entry); err != nil {
				t.Fatal(err)
			}
			candidate := make(map[string]any, len(base)+1)
			for key, value := range base {
				candidate[key] = value
			}
			candidate["constraints"] = []any{entry}
			_, err := ValidateDefinition(canonicalMarshal(t, candidate))
			if testCase.valid && err != nil {
				t.Fatalf("%s was refused: %v", testCase.entry, err)
			}
			if !testCase.valid && err == nil {
				t.Fatalf("%s was accepted", testCase.entry)
			}
		})
	}
}
