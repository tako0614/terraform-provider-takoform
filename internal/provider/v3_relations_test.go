package provider

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

// TestV3EveryReferenceCarriesTheExactGroup proves the provider sends the full
// three-member reference for every reference shape it can render — a top-level
// reference, a nested object-list member, and a typed binding element — while
// the HCL surface stays the bare target name.
func TestV3EveryReferenceCarriesTheExactGroup(t *testing.T) {
	ctx := context.Background()
	group := edgeformcatalog.Family.APIVersion()

	deployment, _ := edgeformcatalog.ByKind("WorkerDeployment")
	var versions model.Field
	for _, field := range deployment.Fields {
		if field.Wire == "versions" {
			versions = field
		}
	}
	if versions.Wire == "" {
		t.Fatal("WorkerDeployment declares no versions field")
	}
	elementType := v3ObjectListType(versions)
	list := types.ListValueMust(elementType, []attr.Value{
		types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
			"worker_version": types.StringValue("worker-version"),
			"weight":         types.Int64Value(10000),
		}),
	})
	wire, diags := v3FieldToWire(ctx, group, versions, "versions", list)
	if diags.HasError() {
		t.Fatalf("versions projection: %v", diags)
	}
	entries, _ := wire.([]any)
	if len(entries) != 1 {
		t.Fatalf("versions wire = %#v", wire)
	}
	entry, _ := entries[0].(map[string]any)
	want := map[string]any{"apiVersion": group, "kind": "WorkerVersion", "name": "worker-version"}
	if !reflect.DeepEqual(entry["workerVersion"], want) {
		t.Fatalf("nested reference = %#v, want %#v", entry["workerVersion"], want)
	}

	// The same rule for the top-level reference of the same Form.
	var worker model.Field
	for _, field := range deployment.Fields {
		if field.Wire == "worker" {
			worker = field
		}
	}
	workerWire, diags := v3FieldToWire(ctx, group, worker, "worker", types.StringValue("module-worker"))
	if diags.HasError() {
		t.Fatalf("worker projection: %v", diags)
	}
	if !reflect.DeepEqual(workerWire, map[string]any{
		"apiVersion": group, "kind": "ModuleWorker", "name": "module-worker",
	}) {
		t.Fatalf("top-level reference = %#v", workerWire)
	}
}

// TestV3RelationConditionWarnsAndNamesTheRemedy proves the read-side contract:
// a resource whose stored relation no longer resolves is reported as a WARNING
// naming the pointer, both uids, and the apply that repairs it. A hard error
// here would abort the refresh and take the repairing plan with it.
func TestV3RelationConditionWarnsAndNamesTheRemedy(t *testing.T) {
	cases := []struct {
		name           string
		condition      clientv3.Condition
		declaresUpdate bool
		wantWarning    bool
		wantDetail     []string
		wantState      string
	}{
		{
			name: "ready",
			condition: clientv3.Condition{
				Type: "Ready", Status: "True", Reason: "Available",
			},
		},
		{
			name: "unrelated not-ready reason",
			condition: clientv3.Condition{
				Type: "Ready", Status: "False", Reason: "Provisioning",
			},
		},
		{
			name: "target incarnation changed on a Form without update",
			condition: clientv3.Condition{
				Type: "Ready", Status: "False", Reason: "ExternalChange",
				HostReason: "relation /kvBindings/0/resource changed incarnation from uid uid-1 to uid uid-2",
			},
			wantWarning: true,
			wantDetail:  []string{"/kvBindings/0/resource", "uid-1", "uid-2", "replacing this resource"},
			wantState:   "ExternalChange",
		},
		{
			name: "target gone on a Form that declares update",
			condition: clientv3.Condition{
				Type: "Ready", Status: "False", Reason: "DependencyMissing",
				HostReason: "relation /worker target uid uid-3 no longer exists",
			},
			declaresUpdate: true,
			wantWarning:    true,
			wantDetail:     []string{"/worker", "uid-3", "re-applying this resource in place", "re-created first"},
			wantState:      "DependencyMissing",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var diags diag.Diagnostics
			res := &clientv3.Resource{
				Status: &clientv3.Status{Conditions: []clientv3.Condition{testCase.condition}},
			}
			v3ReportRelationCondition(
				"WorkerVersion", "conformance", "worker-version", res, testCase.declaresUpdate, &diags,
			)
			if diags.HasError() {
				t.Fatalf("a relation report failed the read: %v", diags)
			}
			recorded := v3RelationDriftState(res)
			if len(diags.Warnings()) != map[bool]int{true: 1, false: 0}[testCase.wantWarning] {
				t.Fatalf("warnings = %v, wantWarning = %v", diags.Warnings(), testCase.wantWarning)
			}
			if !testCase.wantWarning {
				if !recorded.IsNull() {
					t.Fatalf("a healthy resource recorded drift reason %q", recorded.ValueString())
				}
				return
			}
			detail := diags.Warnings()[0].Detail()
			for _, want := range testCase.wantDetail {
				if !strings.Contains(detail, want) {
					t.Fatalf("warning detail %q does not name %q", detail, want)
				}
			}
			if recorded.ValueString() != testCase.wantState {
				t.Fatalf("recorded drift reason = %q, want %q", recorded, testCase.wantState)
			}
		})
	}
}

// TestV3ReadOfBrokenRelationKeepsStateAndPlansReplacement drives the complete
// recovery path of a Form that declares no update: the read warns and keeps
// the resource, the next plan proposes replacement, and the apply that follows
// clears the recorded reason.
func TestV3ReadOfBrokenRelationKeepsStateAndPlansReplacement(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	if resource.form.DeclaresUpdate() {
		t.Fatal("this test needs the Form whose every desired field forces replacement")
	}
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	configured := map[string]attr.Value{
		"name":     types.StringValue("worker-version"),
		"worker":   types.StringValue("module-worker"),
		"bundle":   types.StringValue("worker-bundle"),
		"handlers": types.SetValueMust(types.StringType, []attr.Value{types.StringValue("fetch")}),
	}
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{
		Plan: v3PlanWith(t, ctx, schemaResponse, configured),
	}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	if got := v3StateString(t, ctx, createResponse.State, v3RelationDriftAttribute); !got.IsNull() {
		t.Fatalf("a freshly created resource recorded drift reason %q", got.ValueString())
	}

	// The referenced worker is replaced out of band.
	host.relationDriftReason = "ExternalChange"
	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("a broken relation failed the read: %v", readResponse.Diagnostics)
	}
	if len(readResponse.Diagnostics.Warnings()) != 1 {
		t.Fatalf("read warnings = %v, want exactly one", readResponse.Diagnostics.Warnings())
	}
	if readResponse.State.Raw.IsNull() {
		t.Fatal("the read removed the resource from state, so no plan can repair it")
	}
	if got := v3StateString(t, ctx, readResponse.State, "uid").ValueString(); got != "uid-1" {
		t.Fatalf("read state uid = %q, want the resource left intact", got)
	}
	if got := v3StateString(t, ctx, readResponse.State, v3RelationDriftAttribute).ValueString(); got != "ExternalChange" {
		t.Fatalf("read state %s = %q, want ExternalChange", v3RelationDriftAttribute, got)
	}

	// The refreshed state is what the next plan is computed from: the recorded
	// reason is cleared in the plan (so the plan is not a no-op) and the
	// resource is marked for replacement.
	proposed := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: readResponse.State.Raw}
	modifyResponse := frameworkresource.ModifyPlanResponse{Plan: proposed}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State: readResponse.State,
		Plan:  proposed,
		Config: tfsdk.Config{
			Schema: schemaResponse.Schema,
			Raw:    v3EmptyRaw(t, ctx, schemaResponse),
		},
	}, &modifyResponse)
	if modifyResponse.Diagnostics.HasError() {
		t.Fatalf("plan: %v", modifyResponse.Diagnostics)
	}
	if !modifyResponse.RequiresReplace.Contains(path.Root(v3RelationDriftAttribute)) {
		t.Fatalf("plan does not propose replacement: %v", modifyResponse.RequiresReplace)
	}
	var planned types.String
	if diags := modifyResponse.Plan.GetAttribute(
		ctx, path.Root(v3RelationDriftAttribute), &planned,
	); diags.HasError() {
		t.Fatalf("read planned %s: %v", v3RelationDriftAttribute, diags)
	}
	if !planned.IsNull() {
		t.Fatalf("planned %s = %q; an unchanged plan is a no-op Terraform never applies",
			v3RelationDriftAttribute, planned.ValueString())
	}

	// The apply the plan reaches re-pins the relation, and the next read is
	// clean again.
	host.relationDriftReason = ""
	recovered := frameworkresource.ReadResponse{State: readResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: readResponse.State}, &recovered)
	if recovered.Diagnostics.HasError() {
		t.Fatalf("read after recovery: %v", recovered.Diagnostics)
	}
	if got := v3StateString(t, ctx, recovered.State, v3RelationDriftAttribute); !got.IsNull() {
		t.Fatalf("a re-pinned relation left %s = %q", v3RelationDriftAttribute, got.ValueString())
	}
}

// TestV3BrokenRelationOnUpdatableFormPlansInPlace proves the other half of the
// remedy: a Form that declares update needs no replacement, because the host
// re-resolves and re-pins every relation on any accepted apply. The plan still
// has to carry a change, or Terraform would never call apply at all.
func TestV3BrokenRelationOnUpdatableFormPlansInPlace(t *testing.T) {
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerCronTrigger", newV3TestProviderData(t, host))
	if !resource.form.DeclaresUpdate() {
		t.Fatal("this test needs a Form that declares an in-place update")
	}
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{
		Plan: v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
			"name":   types.StringValue("worker-cron-trigger"),
			"worker": types.StringValue("module-worker"),
			"cron":   types.StringValue("0 3 * * *"),
		}),
	}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}

	host.relationDriftReason = "DependencyMissing"
	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("a broken relation failed the read: %v", readResponse.Diagnostics)
	}

	proposed := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: readResponse.State.Raw}
	modifyResponse := frameworkresource.ModifyPlanResponse{Plan: proposed}
	resource.ModifyPlan(ctx, frameworkresource.ModifyPlanRequest{
		State: readResponse.State,
		Plan:  proposed,
		Config: tfsdk.Config{
			Schema: schemaResponse.Schema,
			Raw:    v3EmptyRaw(t, ctx, schemaResponse),
		},
	}, &modifyResponse)
	if modifyResponse.Diagnostics.HasError() {
		t.Fatalf("plan: %v", modifyResponse.Diagnostics)
	}
	if len(modifyResponse.RequiresReplace) != 0 {
		t.Fatalf("an update-capable Form proposed replacement: %v", modifyResponse.RequiresReplace)
	}
	var planned types.String
	if diags := modifyResponse.Plan.GetAttribute(
		ctx, path.Root(v3RelationDriftAttribute), &planned,
	); diags.HasError() {
		t.Fatalf("read planned %s: %v", v3RelationDriftAttribute, diags)
	}
	if !planned.IsNull() {
		t.Fatalf("planned %s = %q; the plan must differ from state or no apply runs",
			v3RelationDriftAttribute, planned.ValueString())
	}
}
