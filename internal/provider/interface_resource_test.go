package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

func TestDesiredInterfaceUsesProviderSpaceWhenComputedResourceSpaceIsUnknown(t *testing.T) {
	model := interfaceResourceModel{
		Name:               types.StringValue("app.launcher"),
		Version:            types.StringValue("1"),
		Space:              types.StringUnknown(),
		ResourceKind:       types.StringValue("HttpService"),
		ResourceName:       types.StringValue("app"),
		DocumentJSON:       types.StringValue(`{"launcher":true}`),
		DocumentSchemaJSON: types.StringNull(),
		InputsJSON:         types.StringValue("[]"),
		ResourceURIInput:   types.StringNull(),
	}

	desired, space, err := desiredInterface(model, "workspace_1", "")
	if err != nil {
		t.Fatalf("desiredInterface returned error: %v", err)
	}
	if space != "workspace_1" {
		t.Fatalf("space = %q, want provider default", space)
	}
	if desired.Name != "app.launcher" || desired.Resource.Name != "app" {
		t.Fatalf("unexpected declaration: %#v", desired)
	}
}

func TestInterfaceIdentityStillRejectsUnknownStoredSpace(t *testing.T) {
	model := interfaceResourceModel{
		Name:         types.StringValue("app.launcher"),
		Version:      types.StringValue("1"),
		Space:        types.StringUnknown(),
		ResourceKind: types.StringValue("HttpService"),
		ResourceName: types.StringValue("app"),
	}

	if field := unknownInterfaceIdentityField(model); field != "space" {
		t.Fatalf("unknown identity field = %q, want space", field)
	}
}

func TestApplyDeclaredInterfacePreservesEquivalentPlannedJSON(t *testing.T) {
	inputsJSON := `[{"name":"origin","pointer":"/url","source":"output"}]`
	model := interfaceResourceModel{
		DocumentJSON:       types.StringValue(`{"launcher":true,"display":{"title":"App"}}`),
		DocumentSchemaJSON: types.StringNull(),
		InputsJSON:         types.StringValue(inputsJSON),
		ResourceURIInput:   types.StringNull(),
	}
	declared := client.DeclaredInterface{
		Name:     "app.launcher",
		Version:  "1",
		Resource: client.InterfaceResourceRef{Kind: "HttpService", Name: "app"},
		Document: map[string]any{
			"display":  map[string]any{"title": "App"},
			"launcher": true,
		},
		Inputs: []formpackage.InterfaceInputDeclaration{{
			Name: "origin", Source: "output", Pointer: "/url",
		}},
	}

	if err := applyDeclaredInterface(&model, "workspace_1", declared); err != nil {
		t.Fatalf("applyDeclaredInterface returned error: %v", err)
	}
	if model.InputsJSON.ValueString() != inputsJSON {
		t.Fatalf("inputs_json = %q, want planned JSON preserved", model.InputsJSON.ValueString())
	}
}
