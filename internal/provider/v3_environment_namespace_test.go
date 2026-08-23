package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// v3BindingListValue renders one binding list configuration value.
func v3BindingListValue(t *testing.T, entries ...[2]string) types.List {
	t.Helper()
	elementType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"name": types.StringType, "target_name": types.StringType,
	}}
	elements := make([]attr.Value, 0, len(entries))
	for _, entry := range entries {
		object, diags := types.ObjectValue(elementType.AttrTypes, map[string]attr.Value{
			"name": types.StringValue(entry[0]), "target_name": types.StringValue(entry[1]),
		})
		if diags.HasError() {
			t.Fatalf("binding element: %v", diags)
		}
		elements = append(elements, object)
	}
	list, diags := types.ListValue(elementType, elements)
	if diags.HasError() {
		t.Fatalf("binding list: %v", diags)
	}
	return list
}

func v3StringSetValue(t *testing.T, members ...string) types.Set {
	t.Helper()
	elements := make([]attr.Value, 0, len(members))
	for _, member := range members {
		elements = append(elements, types.StringValue(member))
	}
	set, diags := types.SetValue(types.StringType, elements)
	if diags.HasError() {
		t.Fatalf("string set: %v", diags)
	}
	return set
}

func v3ConfigWith(
	t *testing.T,
	ctx context.Context,
	schemaResponse frameworkresource.SchemaResponse,
	values map[string]attr.Value,
) tfsdk.Config {
	t.Helper()
	written := v3PlanWith(t, ctx, schemaResponse, values)
	return tfsdk.Config{Schema: schemaResponse.Schema, Raw: written.Raw.Copy()}
}

// TestV3EnvironmentNamespaceRejectedAtPlanTime proves the provider refuses a
// collision in the single module environment namespace WITHOUT a host round
// trip. The host re-proves the same rule before it mutates anything; the point
// of doing it here is that the author sees the offending attribute, in their own
// configuration, during plan.
func TestV3EnvironmentNamespaceRejectedAtPlanTime(t *testing.T) {
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, newV3FakeHost(t)))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	cases := []struct {
		name    string
		values  map[string]attr.Value
		refused bool
		// mentions is a fragment the diagnostic detail must carry.
		mentions string
	}{
		{
			name: "a vars key against a binding name",
			values: map[string]attr.Value{
				"kv_bindings": v3BindingListValue(t, [2]string{"STORE", "cache"}),
				"vars_json":   types.StringValue(`{"STORE":"https://store.invalid"}`),
			},
			refused: true, mentions: "STORE",
		},
		{
			name: "a sealed value name against a binding name",
			values: map[string]attr.Value{
				"kv_bindings":             v3BindingListValue(t, [2]string{"STORE", "cache"}),
				"required_sensitive_vars": v3StringSetValue(t, "STORE"),
			},
			refused: true, mentions: "STORE",
		},
		{
			name: "two binding lists on one name",
			values: map[string]attr.Value{
				"kv_bindings":     v3BindingListValue(t, [2]string{"STORE", "cache"}),
				"bucket_bindings": v3BindingListValue(t, [2]string{"STORE", "media"}),
			},
			refused: true, mentions: "STORE",
		},
		{
			name: "one binding list naming the same binding twice",
			values: map[string]attr.Value{
				"kv_bindings": v3BindingListValue(t,
					[2]string{"STORE", "cache"}, [2]string{"STORE", "other-cache"}),
			},
			refused: true, mentions: "STORE",
		},
		{
			name: "distinct names across every source",
			values: map[string]attr.Value{
				"kv_bindings":             v3BindingListValue(t, [2]string{"STORE", "cache"}),
				"bucket_bindings":         v3BindingListValue(t, [2]string{"MEDIA", "media"}),
				"vars_json":               types.StringValue(`{"STORE_URL":"https://store.invalid"}`),
				"required_sensitive_vars": v3StringSetValue(t, "STORE_TOKEN_NAME"),
			},
		},
		{
			name:   "nothing declared at all",
			values: map[string]attr.Value{"name": types.StringValue("worker-version")},
		},
	}
	for _, testCase := range cases {
		var response frameworkresource.ValidateConfigResponse
		resource.ValidateConfig(ctx, frameworkresource.ValidateConfigRequest{
			Config: v3ConfigWith(t, ctx, schemaResponse, testCase.values),
		}, &response)
		if !testCase.refused {
			if response.Diagnostics.HasError() {
				t.Fatalf("%s was refused: %v", testCase.name, response.Diagnostics)
			}
			continue
		}
		if !response.Diagnostics.HasError() {
			t.Fatalf("%s was accepted at plan time", testCase.name)
		}
		found := false
		for _, diagnostic := range response.Diagnostics.Errors() {
			if strings.Contains(diagnostic.Detail(), testCase.mentions) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: no diagnostic names %q: %v", testCase.name, testCase.mentions, response.Diagnostics)
		}
	}
}

// TestV3EnvironmentNamespaceIgnoresUnknownValues proves the plan-time rule never
// guesses. A name that is not known until apply cannot be proved to collide, so
// the provider stays silent and the host decides.
func TestV3EnvironmentNamespaceIgnoresUnknownValues(t *testing.T) {
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, newV3FakeHost(t)))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	elementType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"name": types.StringType, "target_name": types.StringType,
	}}
	var response frameworkresource.ValidateConfigResponse
	resource.ValidateConfig(ctx, frameworkresource.ValidateConfigRequest{
		Config: v3ConfigWith(t, ctx, schemaResponse, map[string]attr.Value{
			"kv_bindings": types.ListUnknown(elementType),
			"vars_json":   types.StringValue(`{"STORE":"https://store.invalid"}`),
		}),
	}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("an unknown binding list was treated as a collision: %v", response.Diagnostics)
	}
}

// TestV3EnvironmentNamespaceIsDeclaredOnce proves the model marks exactly the
// fields that project names, so the provider rule and the host rule are reading
// the same declaration rather than two hand-maintained lists.
func TestV3EnvironmentNamespaceIsDeclaredOnce(t *testing.T) {
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, newV3FakeHost(t)))
	declared := map[string]string{}
	for _, entry := range resource.form.EnvironmentNameFields() {
		declared[entry.Field.Wire] = entry.Source
	}
	want := map[string]string{
		"vars":                  "map-keys",
		"requiredSensitiveVars": "items",
		"kvBindings":            "binding-names",
		"bucketBindings":        "binding-names",
		"sqliteBindings":        "binding-names",
		"queueProducerBindings": "binding-names",
		"serviceBindings":       "binding-names",
		// The slot list's NAMES are not what joins the namespace: each slot
		// projects a protocol-fixed member set, and the uniqueness rule is
		// stated over that closure (decision 0045).
		"externalServices": "external-service-projections",
	}
	if len(declared) != len(want) {
		t.Fatalf("environment name fields = %v, want %v", declared, want)
	}
	for wire, source := range want {
		if declared[wire] != source {
			t.Fatalf("%s projects %q, want %q", wire, declared[wire], source)
		}
	}
}
