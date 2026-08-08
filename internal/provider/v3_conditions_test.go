package provider

// v3_conditions_test.go proves the host's condition list reaches Terraform
// state intact, and that `ready` stays a derived convenience over it
// (spec/decisions/0018).

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type v3ConditionState struct {
	Type               types.String `tfsdk:"type"`
	Status             types.String `tfsdk:"status"`
	Reason             types.String `tfsdk:"reason"`
	Message            types.String `tfsdk:"message"`
	HostReason         types.String `tfsdk:"host_reason"`
	ObservedGeneration types.String `tfsdk:"observed_generation"`
	LastTransitionTime types.String `tfsdk:"last_transition_time"`
}

func v3StateConditions(t *testing.T, ctx context.Context, state tfsdk.State) []v3ConditionState {
	t.Helper()
	var conditions []v3ConditionState
	if diags := state.GetAttribute(ctx, path.Root("conditions"), &conditions); diags.HasError() {
		t.Fatalf("get state conditions: %v", diags)
	}
	return conditions
}

// TestV3ConditionsReachStateWithTheirReasons drives one create against a host
// reporting Ready=False for a relation whose target moved. The whole condition
// — its closed reason, the host's free-form detail, and the generation the
// status reflects — has to be readable from state, because the operator's
// question at that moment is WHY, and answering it must not require leaving
// Terraform.
func TestV3ConditionsReachStateWithTheirReasons(t *testing.T) {
	host := newV3FakeHost(t)
	host.relationDriftReason = "ExternalChange"
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

	conditions := v3StateConditions(t, ctx, createResponse.State)
	if len(conditions) != 1 {
		t.Fatalf("state carries %d conditions, want the one the host reported", len(conditions))
	}
	condition := conditions[0]
	if condition.Type.ValueString() != "Ready" || condition.Status.ValueString() != "False" {
		t.Fatalf("condition = %s/%s, want Ready/False", condition.Type.ValueString(), condition.Status.ValueString())
	}
	if condition.Reason.ValueString() != "ExternalChange" {
		t.Fatalf("condition reason = %q, want ExternalChange", condition.Reason.ValueString())
	}
	if condition.HostReason.IsNull() || condition.HostReason.ValueString() == "" {
		t.Fatal("the host's explanatory detail did not reach state")
	}
	if condition.ObservedGeneration.ValueString() != "1" {
		t.Fatalf("condition observed_generation = %q, want 1", condition.ObservedGeneration.ValueString())
	}
	if condition.LastTransitionTime.ValueString() == "" {
		t.Fatal("condition last_transition_time did not reach state")
	}

	// `ready` is derived from the same list and cannot disagree with it.
	var ready types.Bool
	if diags := createResponse.State.GetAttribute(ctx, path.Root("ready"), &ready); diags.HasError() {
		t.Fatalf("get state ready: %v", diags)
	}
	if ready.ValueBool() {
		t.Fatal("ready = true while the Ready condition reports False")
	}

	// A resource the host reports as Ready carries the same shape with status
	// True, so the list is populated on the ordinary path too.
	host.relationDriftReason = ""
	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResponse.Diagnostics)
	}
	readConditions := v3StateConditions(t, ctx, readResponse.State)
	if len(readConditions) != 1 || readConditions[0].Status.ValueString() != "True" ||
		readConditions[0].Reason.ValueString() != "Available" {
		t.Fatalf("read conditions = %+v, want one Available/True condition", readConditions)
	}
	if !readConditions[0].Message.IsNull() {
		t.Fatalf("an absent message became %q rather than null", readConditions[0].Message.ValueString())
	}
	if !readConditions[0].HostReason.IsNull() {
		t.Fatalf("an absent hostReason became %q rather than null", readConditions[0].HostReason.ValueString())
	}
}

// TestV3ConditionsAreComputedOnly proves the attribute cannot become a
// perpetual diff. Conditions are rendered state that changes without any spec
// change (PR 4's derived-rendering rule), so the schema must neither accept a
// configured value nor pin the previous one across a plan.
func TestV3ConditionsAreComputedOnly(t *testing.T) {
	for _, kind := range []string{"ModuleWorker", "WorkerDeployment"} {
		resource := v3TestFormResource(t, kind, newV3TestProviderData(t, newV3FakeHost(t)))
		attribute, present := v3SchemaOf(t, resource).Schema.Attributes["conditions"]
		if !present {
			t.Fatalf("%s declares no conditions attribute", kind)
		}
		if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
			t.Fatalf("%s conditions must be Computed only, got computed=%v optional=%v required=%v",
				kind, attribute.IsComputed(), attribute.IsOptional(), attribute.IsRequired())
		}
	}
}
