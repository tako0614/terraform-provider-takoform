package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
)

func TestProviderConnectionInheritsTheSourceResourceSpace(t *testing.T) {
	t.Parallel()

	kind, ok := currentformcatalog.ByKind("EdgeWorker")
	if !ok {
		t.Fatal("EdgeWorker is not declared")
	}
	attribute := resourceConnectionAttribute(kind)
	description := strings.ToLower(attribute.Description)
	for _, required := range []string{
		"exact space",
		"cannot select another space",
	} {
		if !strings.Contains(description, required) {
			t.Errorf("Connection description omits %q: %q", required, attribute.Description)
		}
	}
	if _, selectable := attribute.NestedObject.Attributes["space"]; selectable {
		t.Fatal("portable Connection unexpectedly exposes a Space selector")
	}

	value := types.ListValueMust(
		types.ObjectType{AttrTypes: resourceConnectionAttrTypes},
		[]attr.Value{connectionValueForSpaceTest("origin", "EdgeWorker/app")},
	)
	var diags diag.Diagnostics
	wire := resourceConnectionsToSpec(context.Background(), kind, value, &diags)
	if diags.HasError() {
		t.Fatalf("project Connection: %v", diags)
	}
	origin, ok := wire["origin"].(map[string]any)
	if !ok {
		t.Fatalf("wire Connection = %#v", wire)
	}
	if _, selectable := origin["space"]; selectable {
		t.Fatalf("wire Connection unexpectedly carries a Space selector: %#v", origin)
	}
}

func connectionValueForSpaceTest(name, resource string) attr.Value {
	return types.ObjectValueMust(resourceConnectionAttrTypes, map[string]attr.Value{
		"name":        types.StringValue(name),
		"resource":    types.StringValue(resource),
		"permissions": types.SetValueMust(types.StringType, []attr.Value{types.StringValue("request")}),
		"projection":  types.StringValue("http.route.v1"),
	})
}
