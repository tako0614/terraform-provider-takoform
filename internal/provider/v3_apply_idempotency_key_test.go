package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	frameworkschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const testWorkerVersionApplyKey = "runtime-input/reference-v1"

func TestV3WorkerVersionApplyIdempotencyKeyLifecycle(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":                         types.StringValue("worker-version"),
		"worker":                       types.StringValue("module-worker"),
		"bundle":                       types.StringValue("worker-bundle"),
		"handlers":                     types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
		v3ApplyIdempotencyKeyAttribute: types.StringValue(testWorkerVersionApplyKey),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create WorkerVersion: %v", createResponse.Diagnostics)
	}
	if len(host.applyHeaders) != 1 || host.applyHeaders[0].Get("Idempotency-Key") != testWorkerVersionApplyKey {
		t.Fatalf("WorkerVersion apply headers = %#v, want exact Idempotency-Key %q", host.applyHeaders, testWorkerVersionApplyKey)
	}
	if host.runtimeInputPuts != 0 {
		t.Fatalf("ordinary WorkerVersion made %d private runtime-input calls", host.runtimeInputPuts)
	}
	rawBody, err := json.Marshal(host.applyBodies[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawBody), testWorkerVersionApplyKey) || strings.Contains(string(rawBody), v3ApplyIdempotencyKeyAttribute) {
		t.Fatalf("provider-only apply key leaked into Host JSON: %s", rawBody)
	}
	if got := v3StateString(t, ctx, createResponse.State, v3ApplyIdempotencyKeyAttribute).ValueString(); got != testWorkerVersionApplyKey {
		t.Fatalf("created WorkerVersion key state = %q, want %q", got, testWorkerVersionApplyKey)
	}

	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read WorkerVersion: %v", readResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, readResponse.State, v3ApplyIdempotencyKeyAttribute).ValueString(); got != testWorkerVersionApplyKey {
		t.Fatalf("read WorkerVersion key state = %q, want %q", got, testWorkerVersionApplyKey)
	}
}

func TestV3WorkerVersionApplyIdempotencyKeyAcceptedCreateRecoveryPreservesState(t *testing.T) {
	host := newV3FakeHost(t)
	host.apply202Pending = true
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":     types.StringValue("worker-version"),
		"worker":   types.StringValue("module-worker"),
		"bundle":   types.StringValue("worker-bundle"),
		"handlers": types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
		// Leave enough time for the accepted HTTP response even when the full Go
		// suite is running packages concurrently. The fake operation itself never
		// settles, so this still deterministically exercises recovery state.
		"create_timeout":               types.StringValue("400ms"),
		v3ApplyIdempotencyKeyAttribute: types.StringValue(testWorkerVersionApplyKey),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if !createResponse.Diagnostics.HasError() {
		t.Fatal("accepted-but-pending WorkerVersion create reported success")
	}
	if createResponse.State.Raw.IsNull() {
		t.Fatal("accepted-but-pending WorkerVersion create wrote no recoverable state")
	}
	if got := v3StateString(t, ctx, createResponse.State, v3ApplyIdempotencyKeyAttribute); got.IsNull() || got.ValueString() != testWorkerVersionApplyKey {
		t.Fatalf("accepted WorkerVersion key state = %#v, want exact %q", got, testWorkerVersionApplyKey)
	}
	if got := v3StateString(t, ctx, createResponse.State, "pending_operation_id").ValueString(); got != "op_apply_pending" {
		t.Fatalf("accepted WorkerVersion pending operation = %q, want op_apply_pending", got)
	}
	if len(host.applyHeaders) != 1 || host.applyHeaders[0].Get("Idempotency-Key") != testWorkerVersionApplyKey {
		t.Fatalf("accepted WorkerVersion apply headers = %#v, want exact Idempotency-Key %q", host.applyHeaders, testWorkerVersionApplyKey)
	}
}

func TestV3WorkerVersionApplyIdempotencyKeyImportLeavesStateNull(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	response := frameworkresource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.ImportState(ctx, frameworkresource.ImportStateRequest{ID: "worker-version"}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("import WorkerVersion: %v", response.Diagnostics)
	}
	if got := v3StateString(t, ctx, response.State, v3ApplyIdempotencyKeyAttribute); !got.IsNull() {
		t.Fatalf("imported WorkerVersion key state = %#v, want null", got)
	}
}

func TestV3WorkerVersionApplyIdempotencyKeyChangesDerivedName(t *testing.T) {
	resource := v3TestFormResource(t, "WorkerVersion", nil)
	spec := map[string]any{
		"worker": map[string]any{
			"apiVersion": "edge.forms.takoform.com", "kind": "ModuleWorker", "name": "module-worker",
		},
		"bundle": map[string]any{
			"apiVersion": "edge.forms.takoform.com", "kind": "WorkerBundle", "name": "worker-bundle",
		},
		"handlers": []any{"fetch"},
	}
	unkeyed, ok := resource.v3RevisionNameFromSpec(spec, "counter")
	if !ok {
		t.Fatal("derived WorkerVersion name without a key failed")
	}
	keyedA, ok := resource.v3RevisionNameFromSpec(spec, "counter", "key-a")
	if !ok {
		t.Fatal("derived WorkerVersion name with key-a failed")
	}
	keyedAgain, _ := resource.v3RevisionNameFromSpec(spec, "counter", "key-a")
	keyedB, ok := resource.v3RevisionNameFromSpec(spec, "counter", "key-b")
	if !ok {
		t.Fatal("derived WorkerVersion name with key-b failed")
	}
	if keyedA != keyedAgain {
		t.Fatalf("same apply key derived %q then %q", keyedA, keyedAgain)
	}
	if keyedA == keyedB || keyedA == unkeyed {
		t.Fatalf("apply key did not contribute to derived identity: unset=%q key-a=%q key-b=%q", unkeyed, keyedA, keyedB)
	}
	if !strings.HasPrefix(unkeyed, "version-") || !strings.HasPrefix(keyedA, "version-") {
		t.Fatalf("derived WorkerVersion names have wrong prefix: unset=%q keyed=%q", unkeyed, keyedA)
	}

	schemaResponse := v3SchemaOf(t, resource)
	attribute, ok := schemaResponse.Schema.Attributes[v3ApplyIdempotencyKeyAttribute]
	if !ok || attribute.GetType().String() != types.StringType.String() {
		t.Fatal("WorkerVersion schema has no typed apply_idempotency_key attribute")
	}
	if !attribute.IsOptional() || attribute.IsRequired() || !attribute.IsComputed() {
		t.Fatalf("apply_idempotency_key schema flags optional=%t required=%t computed=%t", attribute.IsOptional(), attribute.IsRequired(), attribute.IsComputed())
	}
	stringAttribute, ok := attribute.(frameworkschema.StringAttribute)
	if !ok || len(stringAttribute.PlanModifiers) == 0 {
		t.Fatal("apply_idempotency_key schema does not require replacement")
	}
}

func TestV3WorkerVersionPlanKnownFreshKeyDerivesFreshReplacementName(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"worker":                       types.StringValue("module-worker"),
		"bundle":                       types.StringValue("worker-bundle"),
		"handlers":                     types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
		v3RevisionOwnerAttribute:       types.StringValue(v3TestRevisionOwner),
		v3ApplyIdempotencyKeyAttribute: types.StringValue("plan-known-key-a"),
	}
	created := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, configured)}, &created)
	if created.Diagnostics.HasError() {
		t.Fatalf("create derived WorkerVersion: %v", created.Diagnostics)
	}
	priorName := v3StateString(t, ctx, created.State, "name").ValueString()

	planned := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: created.State.Raw.Copy()}
	if diags := planned.SetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), types.StringValue("plan-known-key-b")); diags.HasError() {
		t.Fatalf("set fresh apply key: %v", diags)
	}
	nextConfig := mapsClone(configured)
	nextConfig[v3ApplyIdempotencyKeyAttribute] = types.StringValue("plan-known-key-b")
	response := frameworkresource.ModifyPlanResponse{Plan: planned}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  created.State,
		Plan:   planned,
		Config: v3ConfigWith(t, ctx, schemaResponse, nextConfig),
	}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("plan-known fresh key replacement was refused: %v", response.Diagnostics)
	}
	var nextName types.String
	if diags := response.Plan.GetAttribute(ctx, path.Root("name"), &nextName); diags.HasError() {
		t.Fatalf("read fresh planned name: %v", diags)
	}
	if nextName.IsUnknown() || nextName.ValueString() == "" || nextName.ValueString() == priorName {
		t.Fatalf("fresh key planned name = %#v, prior = %q", nextName, priorName)
	}
	if !response.RequiresReplace.Contains(path.Root("name")) {
		t.Fatalf("fresh derived name did not request replacement: %v", response.RequiresReplace)
	}
}

func TestV3WorkerVersionPinnedNameKeyChangeIsRefused(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	oldPlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":                         types.StringValue("worker-version"),
		"worker":                       types.StringValue("module-worker"),
		"bundle":                       types.StringValue("worker-bundle"),
		"handlers":                     types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
		v3ApplyIdempotencyKeyAttribute: types.StringValue("key-a"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: oldPlan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create pinned WorkerVersion: %v", createResponse.Diagnostics)
	}
	changed := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	if diags := changed.SetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), types.StringValue("key-b")); diags.HasError() {
		t.Fatalf("set changed apply key: %v", diags)
	}
	// Attribute plan modifiers are evaluated separately by the framework and
	// are not present in the resource-level callback response yet.
	response := frameworkresource.ModifyPlanResponse{Plan: changed}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State: createResponse.State,
		Plan:  changed,
		Config: tfsdk.Config{
			Schema: schemaResponse.Schema,
			Raw:    changed.Raw,
		},
	}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("pinned WorkerVersion key change was not refused")
	}
	diagnostic := response.Diagnostics.Errors()[0]
	detail := diagnostic.Detail()
	if !strings.Contains(diagnostic.Summary(), "immutable revision") || !strings.Contains(detail, "Code: "+v3CodeImmutableRevisionSameName) {
		t.Fatalf("pinned key-change diagnostic = %q: %q", diagnostic.Summary(), detail)
	}

	same := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	sameResponse := frameworkresource.ModifyPlanResponse{Plan: same}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State: createResponse.State,
		Plan:  same,
		Config: tfsdk.Config{
			Schema: schemaResponse.Schema,
			Raw:    same.Raw,
		},
	}, &sameResponse)
	if sameResponse.Diagnostics.HasError() || len(sameResponse.RequiresReplace) != 0 {
		t.Fatalf("same apply key was not a no-op: diagnostics=%v replacement=%v", sameResponse.Diagnostics, sameResponse.RequiresReplace)
	}
}

func TestV3WorkerVersionPinnedNameKeyAdoptionIsRefusedWithoutFrameworkReplacementPaths(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"name":     types.StringValue("worker-version"),
		"worker":   types.StringValue("module-worker"),
		"bundle":   types.StringValue("worker-bundle"),
		"handlers": types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
	}
	created := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, configured)}, &created)
	if created.Diagnostics.HasError() {
		t.Fatalf("create unkeyed pinned WorkerVersion: %v", created.Diagnostics)
	}
	if prior := v3StateString(t, ctx, created.State, v3ApplyIdempotencyKeyAttribute); !prior.IsNull() {
		t.Fatalf("unkeyed prior state = %#v, want null", prior)
	}

	planned := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: created.State.Raw.Copy()}
	if diags := planned.SetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), types.StringValue("adopted-plan-known-key")); diags.HasError() {
		t.Fatalf("set adopted key: %v", diags)
	}
	nextConfig := mapsClone(configured)
	nextConfig[v3ApplyIdempotencyKeyAttribute] = types.StringValue("adopted-plan-known-key")
	response := frameworkresource.ModifyPlanResponse{Plan: planned}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  created.State,
		Plan:   planned,
		Config: v3ConfigWith(t, ctx, schemaResponse, nextConfig),
	}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("pinned WorkerVersion adopted a key while retaining the occupied Host address")
	}
	found := false
	for _, diagnostic := range response.Diagnostics.Errors() {
		if strings.Contains(diagnostic.Detail(), "Code: "+v3CodeImmutableRevisionSameName) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("pinned adoption diagnostics omitted %s: %v", v3CodeImmutableRevisionSameName, response.Diagnostics)
	}
}

func TestV3WorkerVersionUnknownReplacementKeyFailsClosedBeforeAnyDestroy(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	priorConfig := map[string]attr.Value{
		"worker":                       types.StringValue("module-worker"),
		"bundle":                       types.StringValue("worker-bundle"),
		"handlers":                     types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
		v3RevisionOwnerAttribute:       types.StringValue(v3TestRevisionOwner),
		v3ApplyIdempotencyKeyAttribute: types.StringValue("consumed-key"),
	}
	created := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, priorConfig)}, &created)
	if created.Diagnostics.HasError() {
		t.Fatalf("create derived WorkerVersion: %v", created.Diagnostics)
	}

	planned := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: created.State.Raw.Copy()}
	if diags := planned.SetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), types.StringUnknown()); diags.HasError() {
		t.Fatalf("set computed apply key: %v", diags)
	}
	nextConfig := mapsClone(priorConfig)
	nextConfig[v3ApplyIdempotencyKeyAttribute] = types.StringUnknown()
	response := frameworkresource.ModifyPlanResponse{Plan: planned}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  created.State,
		Plan:   planned,
		Config: v3ConfigWith(t, ctx, schemaResponse, nextConfig),
	}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("unknown replacement key was accepted without proof that it differs from the consumed key")
	}
	var plannedName types.String
	if diags := response.Plan.GetAttribute(ctx, path.Root("name"), &plannedName); diags.HasError() {
		t.Fatalf("read planned derived name: %v", diags)
	}
	if plannedName.IsUnknown() || plannedName.ValueString() == "" {
		t.Fatalf("failed unknown-key plan discarded the recorded identity: %#v", plannedName)
	}
	found := false
	for _, diagnostic := range response.Diagnostics.Errors() {
		if strings.Contains(diagnostic.Detail(), "Code: "+v3CodeApplyIdempotencyKeyUnknown) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("unknown replacement diagnostics omitted %s: %v", v3CodeApplyIdempotencyKeyUnknown, response.Diagnostics)
	}
}

func TestV3WorkerVersionPinnedNameSameKeySpecReplacementIsRefusedWithoutFrameworkReplacementPaths(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	createPlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":                         types.StringValue("worker-version"),
		"worker":                       types.StringValue("module-worker"),
		"bundle":                       types.StringValue("worker-bundle"),
		"handlers":                     types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
		v3ApplyIdempotencyKeyAttribute: types.StringValue(testWorkerVersionApplyKey),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: createPlan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create derived WorkerVersion: %v", createResponse.Diagnostics)
	}
	if len(host.applyHeaders) != 1 {
		t.Fatalf("initial WorkerVersion apply count = %d, want 1", len(host.applyHeaders))
	}

	changedPlan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	if diags := changedPlan.SetAttribute(ctx, path.Root("handlers"), types.SetValueMust(types.StringType, []attr.Value{types.StringValue("scheduled")})); diags.HasError() {
		t.Fatalf("set changed handlers: %v", diags)
	}
	// Resource.ModifyPlan does not receive attribute-level replacement paths
	// from the framework. Keep this empty to exercise the real callback shape.
	response := frameworkresource.ModifyPlanResponse{Plan: changedPlan}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State: createResponse.State,
		Plan:  changedPlan,
		Config: v3ConfigWith(t, ctx, schemaResponse, map[string]attr.Value{
			"name":                         types.StringValue("worker-version"),
			"worker":                       types.StringValue("module-worker"),
			"bundle":                       types.StringValue("worker-bundle"),
			"handlers":                     types.SetValueMust(types.StringType, []attr.Value{types.StringValue("scheduled")}),
			v3ApplyIdempotencyKeyAttribute: types.StringValue(testWorkerVersionApplyKey),
		}),
	}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("same-key WorkerVersion spec replacement was not refused at plan")
	}
	if len(host.applyHeaders) != 1 {
		t.Fatalf("same-key replacement caused a Host mutation; apply count = %d", len(host.applyHeaders))
	}
	found := false
	for _, diagnostic := range response.Diagnostics.Errors() {
		if strings.Contains(diagnostic.Detail(), "Code: "+v3CodeApplyIdempotencyKeyReuse) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("same-key replacement diagnostics omitted %s: %v", v3CodeApplyIdempotencyKeyReuse, response.Diagnostics)
	}
}

func TestV3WorkerVersionSameKeySpaceReplacementIsRefusedWithoutFrameworkReplacementPaths(t *testing.T) {
	host := newV3FakeHost(t)
	data := newV3TestProviderData(t, host)
	data.defaultSpace = "space-a"
	resource := v3TestFormResource(t, "WorkerVersion", data)
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"name":                         types.StringValue("worker-version"),
		"space":                        types.StringValue("space-a"),
		"worker":                       types.StringValue("module-worker"),
		"bundle":                       types.StringValue("worker-bundle"),
		"handlers":                     types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
		v3ApplyIdempotencyKeyAttribute: types.StringValue(testWorkerVersionApplyKey),
	}
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, configured)}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create pinned WorkerVersion: %v", createResponse.Diagnostics)
	}

	changedPlan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw.Copy()}
	if diags := changedPlan.SetAttribute(ctx, path.Root("space"), types.StringValue("space-b")); diags.HasError() {
		t.Fatalf("set changed space: %v", diags)
	}
	changedConfig := mapsClone(configured)
	changedConfig["space"] = types.StringValue("space-b")
	response := frameworkresource.ModifyPlanResponse{Plan: changedPlan}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  createResponse.State,
		Plan:   changedPlan,
		Config: v3ConfigWith(t, ctx, schemaResponse, changedConfig),
	}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("same-key WorkerVersion space replacement was not refused at plan")
	}
	found := false
	for _, diagnostic := range response.Diagnostics.Errors() {
		if strings.Contains(diagnostic.Detail(), "Code: "+v3CodeApplyIdempotencyKeyReuse) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("same-key space replacement diagnostics omitted %s: %v", v3CodeApplyIdempotencyKeyReuse, response.Diagnostics)
	}
}

func mapsClone(values map[string]attr.Value) map[string]attr.Value {
	clone := make(map[string]attr.Value, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func TestV3WorkerVersionRelationRecoveryWithConsumedKeyFailsClosed(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"worker":                       types.StringValue("module-worker"),
		"bundle":                       types.StringValue("worker-bundle"),
		"handlers":                     types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
		v3RevisionOwnerAttribute:       types.StringValue(v3TestRevisionOwner),
		v3ApplyIdempotencyKeyAttribute: types.StringValue(testWorkerVersionApplyKey),
	}
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, configured)}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create derived WorkerVersion: %v", createResponse.Diagnostics)
	}
	state := tfsdk.State{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw.Copy()}
	if diags := state.SetAttribute(ctx, path.Root(v3RelationDriftAttribute), types.StringValue("ExternalChange")); diags.HasError() {
		t.Fatalf("set relation drift state: %v", diags)
	}
	plan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: state.Raw.Copy()}
	response := frameworkresource.ModifyPlanResponse{Plan: plan}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State:  state,
		Plan:   plan,
		Config: v3ConfigWith(t, ctx, schemaResponse, configured),
	}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("relation recovery reused a consumed WorkerVersion key")
	}
	if len(host.applyHeaders) != 1 {
		t.Fatalf("relation recovery caused a Host mutation; apply count = %d", len(host.applyHeaders))
	}
	found := false
	for _, diagnostic := range response.Diagnostics.Errors() {
		if strings.Contains(diagnostic.Detail(), "Code: "+v3CodeApplyIdempotencyKeyReuse) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("relation recovery diagnostics omitted %s: %v", v3CodeApplyIdempotencyKeyReuse, response.Diagnostics)
	}
}
