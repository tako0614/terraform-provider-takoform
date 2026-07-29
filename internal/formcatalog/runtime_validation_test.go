package formcatalog

import (
	"encoding/json"
	"testing"
)

func TestRuntimeDocumentsAcceptOnlyExactClosedFormContracts(t *testing.T) {
	t.Parallel()

	for _, kind := range Kinds {
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			t.Parallel()

			if err := kind.ValidateObserved(kind.CanonicalObserved()); err != nil {
				t.Fatalf("canonical observed document: %v", err)
			}
			if err := kind.ValidateObserved(kind.ForeignKindObserved()); err == nil {
				t.Error("observed document accepted a foreign Form kind id")
			}
			if err := kind.ValidateOutput(kind.CanonicalOutput()); err != nil {
				t.Fatalf("canonical output document: %v", err)
			}
			if err := kind.ValidateDesired(kind.CanonicalDesired()); err != nil {
				t.Fatalf("canonical desired document: %v", err)
			}

			observedAtMaximum := cloneValue(kind.CanonicalObserved()).(map[string]any)
			observedAtMaximum["generation"] = json.Number("9223372036854775807")
			if err := kind.ValidateObserved(observedAtMaximum); err != nil {
				t.Fatalf("maximum observed generation: %v", err)
			}
			outputAtMaximum := cloneValue(kind.CanonicalOutput()).(map[string]any)
			outputAtMaximum["generation"] = json.Number("9223372036854775807")
			if err := kind.ValidateOutput(outputAtMaximum); err != nil {
				t.Fatalf("maximum output generation: %v", err)
			}
			observedOverflow := cloneValue(kind.CanonicalObserved()).(map[string]any)
			observedOverflow["generation"] = json.Number("9223372036854775808")
			if err := kind.ValidateObserved(observedOverflow); err == nil {
				t.Error("observed document accepted an overflowing generation")
			}
			outputOverflow := cloneValue(kind.CanonicalOutput()).(map[string]any)
			outputOverflow["generation"] = json.Number("9223372036854775808")
			if err := kind.ValidateOutput(outputOverflow); err == nil {
				t.Error("output document accepted an overflowing generation")
			}

			observed := cloneValue(kind.CanonicalObserved()).(map[string]any)
			observed["selectedTarget"] = "host-private-target"
			if err := kind.ValidateObserved(observed); err == nil {
				t.Error("observed document accepted an undeclared host target")
			}

			output := cloneValue(kind.CanonicalOutput()).(map[string]any)
			output["credential"] = "must-not-enter-state"
			if err := kind.ValidateOutput(output); err == nil {
				t.Error("output document accepted an undeclared credential")
			}
		})
	}
}

func TestRuntimeDesiredRejectsAuthorityInsideTypedMap(t *testing.T) {
	t.Parallel()

	kind, ok := ByKind("EdgeWorker")
	if !ok {
		t.Fatal("EdgeWorker is not declared")
	}
	desired := cloneValue(kind.CanonicalDesired()).(map[string]any)
	configuration := desired["configuration"].(map[string]any)
	configuration["credential"] = "must-not-enter-state"
	if err := kind.ValidateDesired(desired); err == nil {
		t.Fatal("desired document accepted a credential inside a typed map")
	}
}
