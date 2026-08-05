package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

func TestConnectionSchemaStatesRequestOnlyAuthorityBoundary(t *testing.T) {
	t.Parallel()

	kind, _ := currentformcatalog.ByKind("EdgeWorker")
	description := strings.ToLower(resourceConnectionAttribute(kind).Description)
	for _, required := range []string{
		"request", "grants nothing", "host may deny", "bindings", "token issuance",
		"authorization", "write fencing", "lifecycle", "host-owned",
	} {
		if !strings.Contains(description, required) {
			t.Errorf("connection description does not state %q boundary: %q", required, description)
		}
	}
	if strings.Contains(description, "materializes concrete grants") {
		t.Fatalf("connection description still claims the host materializes grants: %q", description)
	}
}

func TestConnectionProjectionEnforcesCardinalityAndUniqueNames(t *testing.T) {
	t.Parallel()

	schedule, _ := currentformcatalog.ByKind("Schedule")
	edgeWorker, _ := currentformcatalog.ByKind("EdgeWorker")
	cases := []struct {
		name      string
		kind      formcatalog.Kind
		items     []attr.Value
		wantError bool
	}{
		{name: "required empty", kind: schedule, wantError: true},
		{name: "duplicate names", kind: schedule, items: []attr.Value{
			connectionValue("target", "Workflow/first"),
			connectionValue("target", "Workflow/second"),
		}, wantError: true},
		{name: "exactly one rejects two", kind: schedule, items: []attr.Value{
			connectionValue("first", "Workflow/first"),
			connectionValue("second", "Workflow/second"),
		}, wantError: true},
		{name: "optional connections accept two", kind: edgeWorker, items: []attr.Value{
			connectionValue("first", "ContainerService/first"),
			connectionValue("second", "ContainerService/second"),
		}},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := types.ListValueMust(types.ObjectType{AttrTypes: resourceConnectionAttrTypes}, test.items)
			var diags diag.Diagnostics
			resourceConnectionsToSpec(context.Background(), test.kind, value, &diags)
			if diags.HasError() != test.wantError {
				t.Fatalf("diagnostics = %v, want error %t", diags, test.wantError)
			}
		})
	}
}

func connectionValue(name, resource string) attr.Value {
	return types.ObjectValueMust(resourceConnectionAttrTypes, map[string]attr.Value{
		"name":        types.StringValue(name),
		"resource":    types.StringValue(resource),
		"permissions": types.SetValueMust(types.StringType, []attr.Value{types.StringValue("request")}),
		"projection":  types.StringValue("portable.binding.v1"),
	})
}
