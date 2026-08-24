package currentformmodel

import (
	"strings"
	"testing"
)

func TestMechanismsRejectRequiresHostAPIUnderdeclaration(t *testing.T) {
	t.Parallel()

	constraint := resolvedUIDConstraintForm(Constraint{
		Kind: ConstraintAcyclic, Reference: "/deadLetter/queue",
	})
	constraint.RequiresHostAPI = "forms.takoform.com/v1beta1"

	tagged := taggedTargetForm()
	tagged.RequiresHostAPI = "forms.takoform.com/v1beta1"

	externalService := semanticForm(Field{
		HCL: "external_services", Wire: "externalServices", Kind: KindExternalServiceList,
		Doc:     "External standard services. Omitting it declares no external service.",
		Default: []any{}, Example: []any{},
	})
	externalService.Role = RoleRevision

	entrypointField := exactRefField("function", "function", "function.forms.takoform.com", "Function")
	entrypointField.RequiredEntrypoint = "fetch"
	entrypoint := semanticForm(entrypointField)

	for name, form := range map[string]Form{
		"resolved UID constraint": constraint,
		"tagged object":           tagged,
		"external service slot":   externalService,
		"required entrypoint":     entrypoint,
	} {
		name, form := name, form
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := form.Validate()
			if err == nil || !strings.Contains(err.Error(), "requiresHostApi") || !strings.Contains(err.Error(), "forms.takoform.com/v1") {
				t.Fatalf("Validate error = %v, want mechanism-derived stable-v1 lower-bound refusal", err)
			}
		})
	}
}

func TestMechanismHostAPIMinimumAcceptsSameOrNewerLane(t *testing.T) {
	t.Parallel()
	for _, lane := range []string{
		"forms.takoform.com/v1",
		"forms.takoform.com/v2beta1",
	} {
		form := taggedTargetForm()
		form.RequiresHostAPI = lane
		if err := form.Validate(); err != nil {
			t.Errorf("requiresHostApi %s was rejected: %v", lane, err)
		}
	}
}

func TestMechanismHostAPIMinimumRejectsEarlierPrerelease(t *testing.T) {
	t.Parallel()
	form := taggedTargetForm()
	form.RequiresHostAPI = "forms.takoform.com/v1beta4"
	if err := form.Validate(); err == nil || !strings.Contains(err.Error(), "forms.takoform.com/v1") {
		t.Fatalf("Validate error = %v, want occupied beta4 lane below stable v1", err)
	}
}

func TestLegacyMechanismsRetainTheV1Beta1Minimum(t *testing.T) {
	t.Parallel()
	plain := semanticForm(Field{
		HCL: "label", Wire: "label", Kind: KindString,
		Doc: "Bounded label.", Required: true, Example: "stable", MaxLength: 32,
	})
	legacyEntrypoint := semanticForm(Field{
		HCL: "worker", Wire: "worker", Kind: KindResourceRef,
		Doc: "Retained worker activation.", Required: true,
		Example: map[string]any{
			"apiVersion": "edge.forms.takoform.com/v1beta1", "kind": "ModuleWorker", "name": "worker",
		},
		TargetKind: "ModuleWorker", Target: testInterfaceContract(), RequiredEntrypoint: "fetch",
	})
	legacyEntrypoint.Family.Version = "v1beta1"
	for name, form := range map[string]Form{
		"plain":               plain,
		"retained entrypoint": legacyEntrypoint,
	} {
		if err := form.Validate(); err != nil {
			t.Errorf("%s v1beta1 Form was rejected: %v", name, err)
		}
	}
}
