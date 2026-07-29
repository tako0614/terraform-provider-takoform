package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

func TestCreateAndUpdateRejectUnknownPlanBeforeHost(t *testing.T) {
	var hostRequests atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/.well-known/takoform" {
			writeProviderDiscovery(t, w, server.URL)
			return
		}
		hostRequests.Add(1)
		http.Error(w, "unknown plan must not reach this host", http.StatusInternalServerError)
	}))
	defer server.Close()

	kind := applyTestKind(t, "ObjectBucket")
	resource := &formResource{
		kind: kind,
		data: &providerData{
			client:       mustDiscoveredProviderClient(t, server),
			defaultSpace: "prod",
			forms:        providerCandidateForms(),
		},
	}
	ctx := context.Background()
	var schemaResponse frameworkresource.SchemaResponse
	resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	emptyRaw := func() tftypes.Value {
		return tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil)
	}
	unknownPlan := func() tfsdk.Plan {
		plan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: emptyRaw()}
		if diagnostics := plan.SetAttribute(ctx, path.Root("name"), types.StringUnknown()); diagnostics.HasError() {
			t.Fatalf("set unknown name: %v", diagnostics)
		}
		if diagnostics := plan.SetAttribute(ctx, path.Root("space"), types.StringValue("prod")); diagnostics.HasError() {
			t.Fatalf("set plan Space: %v", diagnostics)
		}
		return plan
	}

	hostRequests.Store(0)
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: emptyRaw()},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: unknownPlan()}, &createResponse)
	if !createResponse.Diagnostics.HasError() {
		t.Fatal("Create accepted an unknown planned Form field")
	}
	if got := hostRequests.Load(); got != 0 {
		t.Fatalf("Create host requests = %d, want 0", got)
	}

	state := tfsdk.State{Schema: schemaResponse.Schema, Raw: emptyRaw()}
	for name, value := range map[string]types.String{
		"name": types.StringValue("object-bucket"), "space": types.StringValue("prod"),
		"resource_version": types.StringValue("1"),
	} {
		if diagnostics := state.SetAttribute(ctx, path.Root(name), value); diagnostics.HasError() {
			t.Fatalf("set state %s: %v", name, diagnostics)
		}
	}
	if diagnostics := setFormIdentityState(ctx, &state, providerCandidateForms()[kind.Kind]); diagnostics.HasError() {
		t.Fatalf("set state Form identity: %v", diagnostics)
	}
	hostRequests.Store(0)
	updateResponse := frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: emptyRaw()},
	}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: unknownPlan(), State: state}, &updateResponse)
	if !updateResponse.Diagnostics.HasError() {
		t.Fatal("Update accepted an unknown planned Form field")
	}
	if got := hostRequests.Load(); got != 0 {
		t.Fatalf("Update host requests = %d, want 0", got)
	}
}

func TestFormApplyRejectsUnknownDesiredValuesBeforeHost(t *testing.T) {
	var hostRequests atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/.well-known/takoform" {
			writeProviderDiscovery(t, w, server.URL)
			return
		}
		hostRequests.Add(1)
		http.Error(w, "planned values must not reach this host", http.StatusInternalServerError)
	}))
	defer server.Close()

	providerClient := mustDiscoveredProviderClient(t, server)
	tests := []struct {
		name   string
		kind   string
		mutate func(*formValues)
	}{
		{
			name: "top-level name scalar",
			kind: "ObjectBucket",
			mutate: func(values *formValues) {
				values.Name = types.StringUnknown()
			},
		},
		{
			name: "top-level string scalar",
			kind: "ContainerService",
			mutate: func(values *formValues) {
				values.Fields["image"] = types.StringUnknown()
			},
		},
		{
			name: "top-level boolean scalar",
			kind: "ObjectBucket",
			mutate: func(values *formValues) {
				values.Fields["versioning"] = types.BoolUnknown()
			},
		},
		{
			name: "top-level integer scalar",
			kind: "VectorIndex",
			mutate: func(values *formValues) {
				values.Fields["dimensions"] = types.Int64Unknown()
			},
		},
		{
			name: "top-level set",
			kind: "ObjectBucket",
			mutate: func(values *formValues) {
				values.Fields["access_protocols"] = types.SetUnknown(types.StringType)
			},
		},
		{
			name: "set member",
			kind: "ObjectBucket",
			mutate: func(values *formValues) {
				values.Fields["access_protocols"] = types.SetValueMust(
					types.StringType,
					[]attr.Value{types.StringUnknown()},
				)
			},
		},
		{
			name: "top-level map",
			kind: "EdgeWorker",
			mutate: func(values *formValues) {
				values.Fields["configuration"] = types.MapUnknown(types.StringType)
			},
		},
		{
			name: "map member",
			kind: "EdgeWorker",
			mutate: func(values *formValues) {
				values.Fields["configuration"] = types.MapValueMust(
					types.StringType,
					map[string]attr.Value{"LOG_LEVEL": types.StringUnknown()},
				)
			},
		},
		{
			name: "top-level connections list",
			kind: "HttpRoute",
			mutate: func(values *formValues) {
				values.Connections = types.ListUnknown(types.ObjectType{AttrTypes: resourceConnectionAttrTypes})
			},
		},
		{
			name: "connection object",
			kind: "HttpRoute",
			mutate: func(values *formValues) {
				values.Connections = types.ListValueMust(
					types.ObjectType{AttrTypes: resourceConnectionAttrTypes},
					[]attr.Value{types.ObjectUnknown(resourceConnectionAttrTypes)},
				)
			},
		},
		{
			name: "artifact URL",
			kind: "EdgeWorker",
			mutate: func(values *formValues) {
				values.Artifact.URL = types.StringUnknown()
			},
		},
		{
			name: "artifact digest",
			kind: "EdgeWorker",
			mutate: func(values *formValues) {
				values.Artifact.SHA256 = types.StringUnknown()
			},
		},
		{
			name: "artifact media type",
			kind: "EdgeWorker",
			mutate: func(values *formValues) {
				values.Artifact.MediaType = types.StringUnknown()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind := applyTestKind(t, test.kind)
			values := canonicalApplyTestValues(t, kind)
			test.mutate(&values)
			resource := &formResource{
				kind: kind,
				data: &providerData{
					client:       providerClient,
					defaultSpace: "prod",
					forms:        providerCandidateForms(),
				},
			}
			hostRequests.Store(0)
			var state tfsdk.State
			var diagnostics diag.Diagnostics
			resource.put(context.Background(), values, &state, &diagnostics)
			if !diagnostics.HasError() {
				t.Fatal("unknown planned value was accepted")
			}
			if got := hostRequests.Load(); got != 0 {
				t.Fatalf("host requests = %d, want 0", got)
			}
		})
	}
}

func TestFormApplyRejectsEveryUnknownOrNullConnectionFieldBeforeHost(t *testing.T) {
	var hostRequests atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/.well-known/takoform" {
			writeProviderDiscovery(t, w, server.URL)
			return
		}
		hostRequests.Add(1)
		http.Error(w, "invalid connection must not reach this host", http.StatusInternalServerError)
	}))
	defer server.Close()

	providerClient := mustDiscoveredProviderClient(t, server)
	stringCases := []struct {
		name  string
		field string
		value attr.Value
	}{
		{name: "unknown name", field: "name", value: types.StringUnknown()},
		{name: "null name", field: "name", value: types.StringNull()},
		{name: "unknown resource", field: "resource", value: types.StringUnknown()},
		{name: "null resource", field: "resource", value: types.StringNull()},
		{name: "unknown projection", field: "projection", value: types.StringUnknown()},
		{name: "null projection", field: "projection", value: types.StringNull()},
	}
	tests := make([]struct {
		name   string
		object func() attr.Value
	}, 0, len(stringCases)+5)
	for _, test := range stringCases {
		test := test
		tests = append(tests, struct {
			name   string
			object func() attr.Value
		}{
			name: test.name,
			object: func() attr.Value {
				attributes := validApplyTestConnectionAttributes()
				attributes[test.field] = test.value
				return types.ObjectValueMust(resourceConnectionAttrTypes, attributes)
			},
		})
	}
	tests = append(tests,
		struct {
			name   string
			object func() attr.Value
		}{
			name: "unknown permissions",
			object: func() attr.Value {
				attributes := validApplyTestConnectionAttributes()
				attributes["permissions"] = types.SetUnknown(types.StringType)
				return types.ObjectValueMust(resourceConnectionAttrTypes, attributes)
			},
		},
		struct {
			name   string
			object func() attr.Value
		}{
			name: "null permissions",
			object: func() attr.Value {
				attributes := validApplyTestConnectionAttributes()
				attributes["permissions"] = types.SetNull(types.StringType)
				return types.ObjectValueMust(resourceConnectionAttrTypes, attributes)
			},
		},
		struct {
			name   string
			object func() attr.Value
		}{
			name: "unknown permission member",
			object: func() attr.Value {
				attributes := validApplyTestConnectionAttributes()
				attributes["permissions"] = types.SetValueMust(
					types.StringType,
					[]attr.Value{types.StringUnknown()},
				)
				return types.ObjectValueMust(resourceConnectionAttrTypes, attributes)
			},
		},
		struct {
			name   string
			object func() attr.Value
		}{
			name: "null permission member",
			object: func() attr.Value {
				attributes := validApplyTestConnectionAttributes()
				attributes["permissions"] = types.SetValueMust(
					types.StringType,
					[]attr.Value{types.StringNull()},
				)
				return types.ObjectValueMust(resourceConnectionAttrTypes, attributes)
			},
		},
		struct {
			name   string
			object func() attr.Value
		}{
			name: "null connection object",
			object: func() attr.Value {
				return types.ObjectNull(resourceConnectionAttrTypes)
			},
		},
	)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind := applyTestKind(t, "HttpRoute")
			values := canonicalApplyTestValues(t, kind)
			values.Connections = types.ListValueMust(
				types.ObjectType{AttrTypes: resourceConnectionAttrTypes},
				[]attr.Value{test.object()},
			)
			resource := &formResource{
				kind: kind,
				data: &providerData{
					client:       providerClient,
					defaultSpace: "prod",
					forms:        providerCandidateForms(),
				},
			}
			hostRequests.Store(0)
			var state tfsdk.State
			var diagnostics diag.Diagnostics
			resource.put(context.Background(), values, &state, &diagnostics)
			if !diagnostics.HasError() {
				t.Fatal("unknown or null nested connection field was accepted")
			}
			if got := hostRequests.Load(); got != 0 {
				t.Fatalf("host requests = %d, want 0", got)
			}
		})
	}
}

func TestFormApplyValidatesExactDesiredSchemaBeforeHost(t *testing.T) {
	var hostRequests atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/.well-known/takoform" {
			writeProviderDiscovery(t, w, server.URL)
			return
		}
		hostRequests.Add(1)
		http.Error(w, "schema-invalid desired state must not reach this host", http.StatusInternalServerError)
	}))
	defer server.Close()

	kind := applyTestKind(t, "ObjectBucket")
	values := canonicalApplyTestValues(t, kind)
	values.Fields["storage_class"] = types.StringValue("not-in-the-current-schema")
	resource := &formResource{
		kind: kind,
		data: &providerData{
			client:       mustDiscoveredProviderClient(t, server),
			defaultSpace: "prod",
			forms:        providerCandidateForms(),
		},
	}
	var state tfsdk.State
	var diagnostics diag.Diagnostics
	resource.put(context.Background(), values, &state, &diagnostics)
	if !diagnostics.HasError() {
		t.Fatal("desired state outside the exact current desiredSchema was accepted")
	}
	if got := hostRequests.Load(); got != 0 {
		t.Fatalf("host requests = %d, want 0", got)
	}
}

func TestFormApplyPreservesOptionalNullAndComputedSpacePlanningSemantics(t *testing.T) {
	kind := applyTestKind(t, "EdgeWorker")
	values := canonicalApplyTestValues(t, kind)
	values.Space = types.StringUnknown()
	values.Connections = types.ListNull(types.ObjectType{AttrTypes: resourceConnectionAttrTypes})
	values.Fields["runtime"] = types.StringNull()
	values.Fields["runtime_version"] = types.StringNull()
	values.Fields["request_timeout_seconds"] = types.Int64Null()
	values.Fields["concurrency"] = types.Int64Null()
	values.Fields["configuration"] = types.MapNull(types.StringType)

	resource := &formResource{
		kind: kind,
		data: &providerData{defaultSpace: "provider-default"},
	}
	body, space, diagnostics := resource.toResource(context.Background(), values)
	if diagnostics.HasError() {
		t.Fatalf("optional null plan was rejected: %v", diagnostics)
	}
	if body == nil {
		t.Fatal("optional null plan produced no Resource")
	}
	if space != "provider-default" || body.Metadata.Space != "provider-default" {
		t.Fatalf("effective Space = %q / %q, want provider-default", space, body.Metadata.Space)
	}
	for _, absent := range []string{
		"connections", "runtime", "runtimeVersion", "requestTimeoutSeconds", "concurrency", "configuration",
	} {
		if _, exists := body.Spec[absent]; exists {
			t.Errorf("optional null field %s entered desired state: %#v", absent, body.Spec[absent])
		}
	}
}

func TestEveryCanonicalFormPlanPassesTheExactDesiredSchemaGate(t *testing.T) {
	for _, kind := range formcatalog.Kinds {
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			values := canonicalApplyTestValues(t, kind)
			resource := &formResource{
				kind: kind,
				data: &providerData{defaultSpace: "prod"},
			}
			body, _, diagnostics := resource.toResource(context.Background(), values)
			if diagnostics.HasError() {
				t.Fatalf("canonical plan was rejected: %v", diagnostics)
			}
			if body == nil {
				t.Fatal("canonical plan produced no Resource")
			}
			if err := kind.ValidateDesired(body.Spec); err != nil {
				t.Fatalf("projected desired state: %v", err)
			}
		})
	}
}

func applyTestKind(t *testing.T, name string) formcatalog.Kind {
	t.Helper()
	kind, ok := formcatalog.ByKind(name)
	if !ok {
		t.Fatalf("%s Form is not declared", name)
	}
	return kind
}

func canonicalApplyTestValues(t *testing.T, kind formcatalog.Kind) formValues {
	t.Helper()
	ctx := context.Background()
	desired := kind.CanonicalDesired()
	values := formValues{
		Name:     types.StringValue(desired["name"].(string)),
		Space:    types.StringValue("prod"),
		Fields:   make(map[string]attr.Value, len(kind.Fields)),
		Artifact: nullArtifactSourceValues(),
	}
	if kind.Artifact {
		values.Artifact = artifactSourceValuesFromSpec(desired["source"])
	}
	if kind.Connections != formcatalog.ConnectionsAbsent {
		var diagnostics diag.Diagnostics
		values.Connections, diagnostics = resourceConnectionsFromSpec(ctx, desired["connections"])
		if diagnostics.HasError() {
			t.Fatalf("canonical connections: %v", diagnostics)
		}
	}
	for _, field := range kind.Fields {
		var diagnostics diag.Diagnostics
		values.Fields[field.HCL] = fieldValueFromSpec(ctx, field, desired[field.Wire], &diagnostics)
		if diagnostics.HasError() {
			t.Fatalf("canonical %s: %v", field.HCL, diagnostics)
		}
	}
	return values
}

func validApplyTestConnectionAttributes() map[string]attr.Value {
	return map[string]attr.Value{
		"name":        types.StringValue("application"),
		"resource":    types.StringValue("EdgeWorker/edge-worker"),
		"permissions": types.SetValueMust(types.StringType, []attr.Value{types.StringValue("request")}),
		"projection":  types.StringValue("http.route.v1"),
	}
}
