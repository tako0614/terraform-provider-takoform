package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

func v3EmptyRaw(t *testing.T, ctx context.Context, schemaResponse frameworkresource.SchemaResponse) tftypes.Value {
	t.Helper()
	return tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil)
}

func v3SchemaOf(t *testing.T, candidate frameworkresource.Resource) frameworkresource.SchemaResponse {
	t.Helper()
	var schemaResponse frameworkresource.SchemaResponse
	candidate.Schema(context.Background(), frameworkresource.SchemaRequest{}, &schemaResponse)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResponse.Diagnostics)
	}
	return schemaResponse
}

func v3PlanWith(t *testing.T, ctx context.Context, schemaResponse frameworkresource.SchemaResponse, values map[string]attr.Value) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}
	for name, value := range values {
		if diags := plan.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("set plan %s: %v", name, diags)
		}
	}
	return plan
}

func v3StateString(t *testing.T, ctx context.Context, state tfsdk.State, name string) types.String {
	t.Helper()
	var value types.String
	if diags := state.GetAttribute(ctx, path.Root(name), &value); diags.HasError() {
		t.Fatalf("get state %s: %v", name, diags)
	}
	return value
}

func TestV3ModuleWorkerCreateReadDeleteRoundTrip(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":  types.StringValue("module-worker"),
		"space": types.StringValue("prod"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	for name, want := range map[string]string{
		"uid": "uid-1", "generation": "1", "revision": "1",
		"form_api_version": "edge.forms.takoform.com/v1alpha1", "form_kind": "ModuleWorker",
		"outputs_json": "{}",
	} {
		if got := v3StateString(t, ctx, createResponse.State, name).ValueString(); got != want {
			t.Errorf("created state %s = %q, want %q", name, got, want)
		}
	}
	var ready types.Bool
	if diags := createResponse.State.GetAttribute(ctx, path.Root("ready"), &ready); diags.HasError() || !ready.ValueBool() {
		t.Errorf("created state ready = %v (%v), want true", ready, createResponse.State.Raw)
	}

	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, readResponse.State, "uid").ValueString(); got != "uid-1" {
		t.Fatalf("read state uid = %q, want uid-1", got)
	}

	deleteResponse := frameworkresource.DeleteResponse{}
	resource.Delete(ctx, frameworkresource.DeleteRequest{State: readResponse.State}, &deleteResponse)
	if deleteResponse.Diagnostics.HasError() {
		t.Fatalf("delete: %v", deleteResponse.Diagnostics)
	}
	if host.eventIndex("delete:ModuleWorker/module-worker") < 0 {
		t.Fatalf("delete never reached the host: %v", host.events)
	}
}

func TestV3Apply202OperationPathResolvesToResource(t *testing.T) {
	host := newV3FakeHost(t)
	host.apply202 = true
	resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name": types.StringValue("module-worker"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create through 202 operation: %v", createResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, createResponse.State, "uid").ValueString(); got != "uid-1" {
		t.Fatalf("202-created state uid = %q, want uid-1", got)
	}
}

func TestV3WorkerBundleCreateUploadsBlobsThenApplies(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	dir := t.TempDir()
	workerBytes := []byte("export default { fetch() { return new Response(\"ok\") } }\n")
	workerFile := filepath.Join(dir, "worker.mjs")
	if err := os.WriteFile(workerFile, workerBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(workerBytes)
	wantDigest := "sha256:" + hex.EncodeToString(sum[:])

	moduleType := v3WorkerBundleModuleType()
	modules := types.ListValueMust(moduleType, []attr.Value{
		types.ObjectValueMust(moduleType.AttrTypes, map[string]attr.Value{
			"name":         types.StringValue("worker.mjs"),
			"content_type": types.StringValue("application/javascript+module"),
			"content_file": types.StringValue(workerFile),
			"size":         types.Int64Unknown(),
			"digest":       types.StringUnknown(),
		}),
	})
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":        types.StringValue("worker-bundle"),
		"main_module": types.StringValue("worker.mjs"),
		"modules":     modules,
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}

	// The host must receive: upload start, the blob, the manifest commit, and
	// only then the WorkerBundle apply.
	start := host.eventIndex("artifact-start")
	blob := host.eventIndex("blob:" + wantDigest)
	commit := host.eventIndex("commit")
	apply := host.eventPrefixIndex("apply:WorkerBundle/")
	if start < 0 || blob < 0 || commit < 0 || apply < 0 || !(start < blob && blob < commit && commit < apply) {
		t.Fatalf("event order = %v, want artifact-start < blob < commit < apply", host.events)
	}
	if !reflect.DeepEqual(host.blobs[wantDigest], workerBytes) {
		t.Fatalf("host blob bytes differ from the module file")
	}

	var stateModules types.List
	if diags := createResponse.State.GetAttribute(ctx, path.Root("modules"), &stateModules); diags.HasError() {
		t.Fatalf("state modules: %v", diags)
	}
	element := stateModules.Elements()[0].(types.Object).Attributes()
	if got := element["digest"].(types.String).ValueString(); got != wantDigest {
		t.Fatalf("state module digest = %q, want %q", got, wantDigest)
	}
	if got := element["size"].(types.Int64).ValueInt64(); got != int64(len(workerBytes)) {
		t.Fatalf("state module size = %d, want %d", got, len(workerBytes))
	}
	if got := element["content_file"].(types.String).ValueString(); got != workerFile {
		t.Fatalf("state module content_file = %q, want the local path %q", got, workerFile)
	}
}

// TestV3WorkerBundleModifyPlanDetectsChangedBytes proves plan-time byte
// identity: rewriting a module file at an UNCHANGED content_file path makes
// the plan carry the new size and digest and forces replacement.
func TestV3WorkerBundleModifyPlanDetectsChangedBytes(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	dir := t.TempDir()
	originalBytes := []byte("export default { fetch() { return new Response(\"one\") } }\n")
	workerFile := filepath.Join(dir, "worker.mjs")
	if err := os.WriteFile(workerFile, originalBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	moduleType := v3WorkerBundleModuleType()
	modules := types.ListValueMust(moduleType, []attr.Value{
		types.ObjectValueMust(moduleType.AttrTypes, map[string]attr.Value{
			"name":         types.StringValue("worker.mjs"),
			"content_type": types.StringValue("application/javascript+module"),
			"content_file": types.StringValue(workerFile),
			"size":         types.Int64Unknown(),
			"digest":       types.StringUnknown(),
		}),
	})
	createPlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":        types.StringValue("worker-bundle"),
		"main_module": types.StringValue("worker.mjs"),
		"modules":     modules,
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: createPlan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}

	// Rewrite the module bytes at the SAME path.
	changedBytes := []byte("export default { fetch() { return new Response(\"two\") } }\n")
	if err := os.WriteFile(workerFile, changedBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(changedBytes)
	wantDigest := "sha256:" + hex.EncodeToString(sum[:])

	// Terraform's proposed new state for an unchanged config equals prior
	// state; without plan-time byte identity this plan would claim no change.
	proposed := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	modifyResponse := frameworkresource.ModifyPlanResponse{Plan: proposed}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State: createResponse.State,
		Plan:  proposed,
	}, &modifyResponse)
	if modifyResponse.Diagnostics.HasError() {
		t.Fatalf("modify plan: %v", modifyResponse.Diagnostics)
	}
	var plannedModules types.List
	if diags := modifyResponse.Plan.GetAttribute(ctx, path.Root("modules"), &plannedModules); diags.HasError() {
		t.Fatalf("planned modules: %v", diags)
	}
	element := plannedModules.Elements()[0].(types.Object).Attributes()
	if got := element["digest"].(types.String).ValueString(); got != wantDigest {
		t.Fatalf("planned module digest = %q, want the rewritten bytes' digest %q", got, wantDigest)
	}
	if got := element["size"].(types.Int64).ValueInt64(); got != int64(len(changedBytes)) {
		t.Fatalf("planned module size = %d, want %d", got, len(changedBytes))
	}
	if !modifyResponse.RequiresReplace.Contains(path.Root("modules")) {
		t.Fatalf("changed module bytes did not force replacement: %v", modifyResponse.RequiresReplace)
	}

	// Restoring the original bytes removes the diff: the proposed plan (prior
	// state identity) is left untouched and no replacement is forced.
	if err := os.WriteFile(workerFile, originalBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	unchangedProposed := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	unchangedResponse := frameworkresource.ModifyPlanResponse{Plan: unchangedProposed}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State: createResponse.State,
		Plan:  unchangedProposed,
	}, &unchangedResponse)
	if unchangedResponse.Diagnostics.HasError() {
		t.Fatalf("unchanged modify plan: %v", unchangedResponse.Diagnostics)
	}
	if len(unchangedResponse.RequiresReplace) != 0 {
		t.Fatalf("identical bytes forced replacement: %v", unchangedResponse.RequiresReplace)
	}
	if !unchangedResponse.Plan.Raw.Equal(createResponse.State.Raw) {
		t.Fatal("identical bytes changed the proposed plan")
	}
}

// TestV3WorkerBundleTimeoutOnlyUpdateSucceedsWithoutHostMutation proves the
// one legal in-place update of a revision-role resource: changing only the
// provider-side timeout attributes writes the plan to state without any host
// call, while an update carrying a desired-spec change stays a hard error.
func TestV3WorkerBundleTimeoutOnlyUpdateSucceedsWithoutHostMutation(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerBundle", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	dir := t.TempDir()
	workerFile := filepath.Join(dir, "worker.mjs")
	if err := os.WriteFile(workerFile, []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	moduleType := v3WorkerBundleModuleType()
	modules := types.ListValueMust(moduleType, []attr.Value{
		types.ObjectValueMust(moduleType.AttrTypes, map[string]attr.Value{
			"name":         types.StringValue("worker.mjs"),
			"content_type": types.StringValue("application/javascript+module"),
			"content_file": types.StringValue(workerFile),
			"size":         types.Int64Unknown(),
			"digest":       types.StringUnknown(),
		}),
	})
	createPlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":        types.StringValue("worker-bundle"),
		"main_module": types.StringValue("worker.mjs"),
		"modules":     modules,
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: createPlan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	hostEventsAfterCreate := len(host.events)

	// A timeout-only change: the plan is the prior state with a new
	// create_timeout. It must succeed with no host traffic.
	timeoutPlan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	if diags := timeoutPlan.SetAttribute(ctx, path.Root("create_timeout"), types.StringValue("25m")); diags.HasError() {
		t.Fatalf("set planned create_timeout: %v", diags)
	}
	updateResponse := frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: timeoutPlan, State: createResponse.State}, &updateResponse)
	if updateResponse.Diagnostics.HasError() {
		t.Fatalf("timeout-only update: %v", updateResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, updateResponse.State, "create_timeout").ValueString(); got != "25m" {
		t.Fatalf("updated state create_timeout = %q, want 25m", got)
	}
	if got := v3StateString(t, ctx, updateResponse.State, "uid").ValueString(); got != "uid-1" {
		t.Fatalf("timeout-only update lost state uid: %q", got)
	}
	if len(host.events) != hostEventsAfterCreate {
		t.Fatalf("timeout-only update reached the host: %v", host.events[hostEventsAfterCreate:])
	}

	// A desired-spec change planned as an in-place update stays a hard error
	// and still performs no host mutation.
	specPlan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: createResponse.State.Raw}
	if diags := specPlan.SetAttribute(ctx, path.Root("main_module"), types.StringValue("other.mjs")); diags.HasError() {
		t.Fatalf("set planned main_module: %v", diags)
	}
	rejectedResponse := frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: specPlan, State: createResponse.State}, &rejectedResponse)
	if !rejectedResponse.Diagnostics.HasError() {
		t.Fatal("spec-changing in-place update was accepted")
	}
	if len(host.events) != hostEventsAfterCreate {
		t.Fatalf("rejected update reached the host: %v", host.events[hostEventsAfterCreate:])
	}
}

func TestV3WorkerVersionBindingBlocksUseResourceWireKey(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	bindingType := v3BindingObjectType()
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":               types.StringValue("worker-version"),
		"worker":             types.StringValue("module-worker"),
		"bundle":             types.StringValue("worker-bundle"),
		"compatibility_date": types.StringValue("2026-08-06"),
		"handlers":           types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
		"kv_bindings": types.ListValueMust(bindingType, []attr.Value{
			types.ObjectValueMust(bindingType.AttrTypes, map[string]attr.Value{
				"name":        types.StringValue("CACHE"),
				"target_name": types.StringValue("edge-kv-namespace"),
			}),
		}),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	if len(host.applySpecs) == 0 {
		t.Fatal("apply never reached the host")
	}
	spec := host.applySpecs[len(host.applySpecs)-1]
	want := []any{map[string]any{
		"name":     "CACHE",
		"resource": map[string]any{"kind": "EdgeKVNamespace", "name": "edge-kv-namespace"},
	}}
	if !reflect.DeepEqual(spec["kvBindings"], want) {
		t.Fatalf("kvBindings wire shape = %#v, want %#v", spec["kvBindings"], want)
	}
	if entry, ok := spec["kvBindings"].([]any)[0].(map[string]any); ok {
		if _, hasTarget := entry["target"]; hasTarget {
			t.Fatal("binding element must carry `resource`, never `target`")
		}
	}
	if !reflect.DeepEqual(spec["worker"], map[string]any{"kind": "ModuleWorker", "name": "module-worker"}) {
		t.Fatalf("worker reference wire shape = %#v", spec["worker"])
	}
}

func TestV3DeploymentWeightSumValidator(t *testing.T) {
	elementType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"worker_version": types.StringType,
		"weight":         types.Int64Type,
	}}
	buildRequest := func(weights ...int64) validator.ListRequest {
		elements := make([]attr.Value, 0, len(weights))
		for _, weight := range weights {
			elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
				"worker_version": types.StringValue("worker-version"),
				"weight":         types.Int64Value(weight),
			}))
		}
		return validator.ListRequest{
			Path:        path.Root("versions"),
			ConfigValue: types.ListValueMust(elementType, elements),
		}
	}

	var rejected validator.ListResponse
	v3WeightSumValidator{}.ValidateList(context.Background(), buildRequest(9999), &rejected)
	if !rejected.Diagnostics.HasError() {
		t.Fatal("weight sum 9999 was accepted")
	}
	var accepted validator.ListResponse
	v3WeightSumValidator{}.ValidateList(context.Background(), buildRequest(4000, 6000), &accepted)
	if accepted.Diagnostics.HasError() {
		t.Fatalf("weight sum 10000 was rejected: %v", accepted.Diagnostics)
	}
}

func TestV3UpdateCarriesGenerationAndUIDFence(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerCronTrigger", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	createPlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":   types.StringValue("worker-cron-trigger"),
		"worker": types.StringValue("module-worker"),
		"cron":   types.StringValue("0 3 * * *"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: createPlan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}

	updatePlan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name":   types.StringValue("worker-cron-trigger"),
		"space":  types.StringValue("prod"),
		"worker": types.StringValue("module-worker"),
		"cron":   types.StringValue("15 0 * * *"),
	})
	updateResponse := frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: updatePlan, State: createResponse.State}, &updateResponse)
	if updateResponse.Diagnostics.HasError() {
		t.Fatalf("update: %v", updateResponse.Diagnostics)
	}

	updateHeaders := host.applyHeaders[len(host.applyHeaders)-1]
	if got := updateHeaders.Get("Takoform-Expected-Generation"); got != "1" {
		t.Fatalf("update generation fence = %q, want %q", got, "1")
	}
	if got := updateHeaders.Get("If-None-Match"); got != "" {
		t.Fatalf("update must not send If-None-Match, got %q", got)
	}
	updateBody := host.applyBodies[len(host.applyBodies)-1]
	if got, _ := updateBody["expectedUid"].(string); got != "uid-1" {
		t.Fatalf("update expectedUid = %q, want uid-1", got)
	}
	if got := v3StateString(t, ctx, updateResponse.State, "generation").ValueString(); got != "2" {
		t.Fatalf("updated state generation = %q, want 2", got)
	}
	if got := v3StateString(t, ctx, updateResponse.State, "revision").ValueString(); got != "2" {
		t.Fatalf("updated state revision = %q, want 2", got)
	}
}

func TestV3ReadRejectsUnknownStateFormRefBeforeHost(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "ModuleWorker", newV3TestProviderData(t, host))
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	state := tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}
	for name, value := range map[string]string{
		"name":                    "module-worker",
		"space":                   "prod",
		"uid":                     "uid-1",
		"generation":              "1",
		"revision":                "1",
		"form_api_version":        "other.example.com/v1alpha1",
		"form_kind":               "ModuleWorker",
		"form_definition_version": "0.1.0",
		"form_schema_digest":      "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	} {
		if diags := state.SetAttribute(ctx, path.Root(name), types.StringValue(value)); diags.HasError() {
			t.Fatalf("seed state %s: %v", name, diags)
		}
	}
	readResponse := frameworkresource.ReadResponse{State: state}
	resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResponse)
	if !readResponse.Diagnostics.HasError() {
		t.Fatal("unknown state FormRef was accepted")
	}
	detail := readResponse.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "other.example.com/v1alpha1") {
		t.Fatalf("diagnostic does not name the state identity: %s", detail)
	}
	defaultRef, err := currentformregistry.V3ForKind(edgeformcatalog.Family.APIVersion(), "ModuleWorker")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, defaultRef.SchemaDigest) {
		t.Fatalf("diagnostic does not name the supported identity: %s", detail)
	}
	if host.getRequests != 0 {
		t.Fatalf("unsupported state FormRef reached the host with %d GETs", host.getRequests)
	}
}

func TestV3GenericResourceAppliesWithExactRef(t *testing.T) {
	host := newV3FakeHost(t)
	data := newV3TestProviderData(t, host)
	generic := &v3GenericResource{data: data}
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, generic)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"form_api_version":        types.StringValue("forms.example.com/v1alpha1"),
		"form_kind":               types.StringValue("Widget"),
		"form_definition_version": types.StringValue("1.0.0"),
		"form_schema_digest":      types.StringValue("sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"),
		"name":                    types.StringValue("my-widget"),
		"spec_json":               types.StringValue(`{"size":3}`),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	generic.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	if host.eventIndex("apply:Widget/my-widget") < 0 {
		t.Fatalf("generic apply never reached the host: %v", host.events)
	}
	spec := host.applySpecs[len(host.applySpecs)-1]
	if got, _ := spec["size"].(float64); got != 3 {
		t.Fatalf("generic spec = %#v, want size 3", spec)
	}
	if got := v3StateString(t, ctx, createResponse.State, "uid").ValueString(); got != "uid-1" {
		t.Fatalf("generic created state uid = %q, want uid-1", got)
	}
	if got := v3StateString(t, ctx, createResponse.State, "spec_json").ValueString(); got != `{"size":3}` {
		t.Fatalf("generic state spec_json = %q, want the authored text", got)
	}
}

func TestV3GenericResourceRejectsRetainedGroups(t *testing.T) {
	host := newV3FakeHost(t)
	generic := &v3GenericResource{data: newV3TestProviderData(t, host)}
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, generic)

	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"form_api_version":        types.StringValue("forms.takoform.com/v1alpha2"),
		"form_kind":               types.StringValue("ObjectBucket"),
		"form_definition_version": types.StringValue("0.1.0"),
		"form_schema_digest":      types.StringValue("sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"),
		"name":                    types.StringValue("assets"),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	generic.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if !createResponse.Diagnostics.HasError() {
		t.Fatal("retained frozen group was accepted as a v1alpha3 FormRef")
	}
	if len(host.applySpecs) != 0 {
		t.Fatal("retained-group apply reached the host")
	}
}
