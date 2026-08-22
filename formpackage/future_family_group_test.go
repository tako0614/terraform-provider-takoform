package formpackage

import "testing"

// A family minted after the current one must validate against the CURRENT
// schema, not silently fall through to the retained closure.
func TestFutureFamilyGroupUsesTheCurrentSchema(t *testing.T) {
	for _, group := range []string{
		"edge.forms.takoform.com/v1",
		"edge.forms.takoform.com/v1beta2",
		"containers.forms.takoform.com/v1alpha1",
	} {
		if retainedFamilyGroup(group) {
			t.Fatalf("%s must not select the retained family schema", group)
		}
	}
	if !retainedFamilyGroup("edge.forms.takoform.com/v1alpha1") {
		t.Fatal("the withdrawn Edge family must still select the retained schema")
	}
}
