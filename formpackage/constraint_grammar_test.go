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
	base["requiresHostApi"] = "forms.takoform.com/v1"
	if _, err := ValidateDefinition(canonicalMarshal(t, base)); err != nil {
		t.Fatalf("the published Definition this test builds on does not validate: %v", err)
	}
	desired := base["desiredSchema"].(map[string]any)
	properties := desired["properties"].(map[string]any)
	properties["otherWorker"] = properties["worker"]
	properties["minimum"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 1000}
	properties["maximum"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 1000}
	properties["optionalNumber"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 1000}
	properties["indexes"] = map[string]any{
		"type": "array", "maxItems": 8,
		"items": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"name"},
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 63},
			},
		},
	}
	required, _ := desired["required"].([]any)
	desired["required"] = append(required, "minimum", "maximum")

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
		"ordered numeric pair":         {`{"kind":"orderedPair","references":["/minimum","/maximum"]}`, true},
		"unique object-list member":    {`{"kind":"uniqueBy","list":"/indexes","member":"name"}`, true},
		"acyclic relation":             {`{"kind":"acyclic","reference":"/worker"}`, true},
		"distinct relation pair":       {`{"kind":"distinctPair","references":["/worker","/otherWorker"]}`, true},
		"unique relation pair":         {`{"kind":"uniquePair","references":["/worker","/otherWorker"]}`, true},
		"same resolved target":         {`{"kind":"sameResolvedTarget","anchor":"/worker","members":"/versions/*/workerVersion","through":"/worker"}`, true},
		// A kind with none of its members states no rule at all.
		"exclusive with no reference":  {`{"kind":"exclusive"}`, false},
		"sum with nothing to add":      {`{"kind":"sum"}`, false},
		"claim with no property":       {`{"kind":"claim"}`, false},
		"host-assigned with no output": {`{"kind":"hostAssigned"}`, false},
		// A kind carrying another kind's members names two rules and is neither.
		"claim wearing a sum's members":    {`{"kind":"claim","list":"/x","member":"weight","total":10}`, false},
		"claim with an output":             {`{"kind":"claim","property":"/hostname","output":"/address"}`, false},
		"sum missing its total":            {`{"kind":"sum","list":"/targets","member":"weight"}`, false},
		"host-assigned with a reference":   {`{"kind":"hostAssigned","output":"/address","reference":"/worker"}`, false},
		"ordered pair with one value":      {`{"kind":"orderedPair","references":["/minimum"]}`, false},
		"ordered pair with optional value": {`{"kind":"orderedPair","references":["/minimum","/optionalNumber"]}`, false},
		"unique list with absent member":   {`{"kind":"uniqueBy","list":"/indexes","member":"missing"}`, false},
		"unique list with foreign member":  {`{"kind":"uniqueBy","list":"/indexes","member":"name","total":1}`, false},
		"acyclic with no reference":        {`{"kind":"acyclic"}`, false},
		"distinct with one relation":       {`{"kind":"distinctPair","references":["/worker"]}`, false},
		"unique with duplicate relation":   {`{"kind":"uniquePair","references":["/worker","/worker"]}`, false},
		"same target missing through":      {`{"kind":"sameResolvedTarget","anchor":"/worker","members":"/versions/*/workerVersion"}`, false},
		"acyclic non-relation pointer":     {`{"kind":"acyclic","reference":"/notDeclared"}`, false},
		"pair contains non-relation":       {`{"kind":"distinctPair","references":["/worker","/notDeclared"]}`, false},
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
