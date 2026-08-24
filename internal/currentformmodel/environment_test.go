package currentformmodel

import (
	"strings"
	"testing"
)

// collidingForm declares one binding list and one vars map whose canonical
// Examples claim the same environment name.
func collidingForm(varsExample map[string]any) Form {
	return Form{
		Family: Family{Group: "edge.forms.takoform.com", Version: "v1alpha1"},
		Kind:   "WorkerVersion", Slug: "worker-version",
		RequiresHostAPI: "forms.takoform.com/v1beta1", Role: RoleRevision, Title: "Worker Version", DefinitionVersion: "0.1.0",
		Fields: []Field{
			{HCL: "vars", Wire: "vars", Kind: KindJSONMap, Default: map[string]any{},
				ProjectsEnvironmentNames: true, Doc: "Environment values.", Example: varsExample},
			{HCL: "kv_bindings", Wire: "kvBindings", Kind: KindBindingList,
				TargetKind: "EdgeKVNamespace", BindingType: "module-worker.edge-kv",
				Target:  testInterfaceContract(),
				Default: []any{}, Doc: "Typed bindings.",
				Example: []any{map[string]any{
					"name": "CACHE",
					"resource": map[string]any{
						"apiVersion": "edge.forms.takoform.com/v1beta1",
						"kind":       "EdgeKVNamespace", "name": "edge-kv-namespace",
					},
				}}},
		},
	}
}

// TestValidateEnvironmentNamespaceRejectsCollidingExamples proves the one
// collision the authoring model can decide: a Form whose own canonical Examples
// claim a name twice would ship a fixture every conforming host must refuse.
func TestValidateEnvironmentNamespaceRejectsCollidingExamples(t *testing.T) {
	err := ValidateEnvironmentNamespace(collidingForm(map[string]any{"CACHE": "https://cache.invalid"}))
	if err == nil {
		t.Fatal("a Form whose examples claim CACHE twice was accepted")
	}
	if !strings.Contains(err.Error(), "CACHE") || !strings.Contains(err.Error(), "kvBindings") {
		t.Fatalf("the refusal does not name the colliding name and field: %v", err)
	}
	if err := ValidateEnvironmentNamespace(
		collidingForm(map[string]any{"CACHE_URL": "https://cache.invalid"}),
	); err != nil {
		t.Fatalf("distinct example names were refused: %v", err)
	}
}

// TestProjectsEnvironmentNamesIsBoundedToNameCarryingKinds proves the marker
// cannot be attached to a field whose values are not names.
func TestProjectsEnvironmentNamesIsBoundedToNameCarryingKinds(t *testing.T) {
	form := Form{
		Family: Family{Group: "edge.forms.takoform.com", Version: "v1alpha1"},
		Kind:   "WorkerVersion", Slug: "worker-version",
		RequiresHostAPI: "forms.takoform.com/v1beta1", Role: RoleRevision, Title: "Worker Version", DefinitionVersion: "0.1.0",
		Fields: []Field{{
			HCL: "activation_delay_seconds", Wire: "activationDelaySeconds", Kind: KindInteger,
			Required: true, Doc: "A duration in seconds, which names nothing in a runtime environment.", Example: 30,
			ProjectsEnvironmentNames: true,
		}},
	}
	err := form.Validate()
	if err == nil || !strings.Contains(err.Error(), "ProjectsEnvironmentNames") {
		t.Fatalf("an integer field marked as projecting environment names was accepted: %v", err)
	}
}
