package provider

// v3_conditions.go projects the host's closed status conditions into Terraform
// state.
//
// The wire carries a LIST of conditions — each with a closed type, a status, a
// closed portable reason, an optional host-specific reason and message, and the
// time it last changed — together with the desired generation the status
// reflects (spec/host-api/v1beta1.md, spec/decisions/0011). Flattening all of
// that to one boolean answered "is it ready" and threw away "why is it not",
// which is exactly the question an operator has at the moment a plan surprises
// them. Reading it then meant leaving Terraform for the host's own surfaces
// (spec/decisions/0018).
//
// The attribute is Computed and carries no plan modifier. Conditions are
// RENDERED state, not desired state: under the derived-rendering rule a
// resource's conditions change when a DIFFERENT resource is mutated — a
// deployment makes its worker Ready, a deleted target makes its source report
// DependencyMissing — with no spec change anywhere. Holding the previous value
// with UseStateForUnknown would therefore assert something the host has already
// contradicted, and making the attribute Optional would invite a configuration
// to declare a value the host owns and diff against it forever.

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
)

// v3ConditionAttributeTypes is the element shape of the conditions list.
var v3ConditionAttributeTypes = map[string]attr.Type{
	"type":                 types.StringType,
	"status":               types.StringType,
	"reason":               types.StringType,
	"message":              types.StringType,
	"host_reason":          types.StringType,
	"observed_generation":  types.StringType,
	"last_transition_time": types.StringType,
}

var v3ConditionObjectType = types.ObjectType{AttrTypes: v3ConditionAttributeTypes}

// v3ConditionsAttribute declares the typed, read-only condition list.
func v3ConditionsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Computed: true,
		Description: "Every status condition the host reports, in the order it reports them. " +
			"Conditions are host-owned rendered state: they change when this resource changes AND when " +
			"another resource this one depends on changes, without any desired spec changing.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"type": schema.StringAttribute{
					Computed: true,
					Description: "Closed condition type: `Ready`, `Reconciling`, `Degraded`, `Drifted`, " +
						"`Blocked`, or `Deleting`.",
				},
				"status": schema.StringAttribute{
					Computed:    true,
					Description: "`True`, `False`, or `Unknown`.",
				},
				"reason": schema.StringAttribute{
					Computed: true,
					Description: "Closed portable reason. Two conforming hosts name one state with one reason, " +
						"so this is the member to branch on.",
				},
				"message": schema.StringAttribute{
					Computed:    true,
					Description: "Human-readable detail, null when the host supplies none.",
				},
				"host_reason": schema.StringAttribute{
					Computed: true,
					Description: "Host-specific, non-portable detail naming exactly what is wrong — a relation " +
						"pointer, both sides of an incarnation change, a missing handler. Null when the host " +
						"supplies none, and never a value to branch on.",
				},
				"observed_generation": schema.StringAttribute{
					Computed: true,
					Description: "The desired-state generation this status reflects. When it lags `generation`, " +
						"the host has not yet observed the newest desired spec.",
				},
				"last_transition_time": schema.StringAttribute{
					Computed:    true,
					Description: "RFC 3339 time this condition last changed status.",
				},
			},
		},
	}
}

// v3ConditionsState projects one host representation's conditions. A response
// carrying no conditions becomes the EMPTY list rather than null: "the host
// reported none" is a fact, and null would read as "not known yet" on a value
// that has already been answered.
func v3ConditionsState(res *clientv3.Resource, diags *diag.Diagnostics) types.List {
	if res == nil || res.Status == nil || len(res.Status.Conditions) == 0 {
		return types.ListValueMust(v3ConditionObjectType, []attr.Value{})
	}
	observedGeneration := res.Status.ObservedGeneration
	elements := make([]attr.Value, 0, len(res.Status.Conditions))
	for _, condition := range res.Status.Conditions {
		element, elementDiags := types.ObjectValue(v3ConditionAttributeTypes, map[string]attr.Value{
			"type":                 types.StringValue(condition.Type),
			"status":               types.StringValue(condition.Status),
			"reason":               types.StringValue(condition.Reason),
			"message":              v3OptionalStateString(condition.Message),
			"host_reason":          v3OptionalStateString(condition.HostReason),
			"observed_generation":  v3OptionalStateString(observedGeneration),
			"last_transition_time": v3OptionalStateString(condition.LastTransitionTime),
		})
		diags.Append(elementDiags...)
		elements = append(elements, element)
	}
	list, listDiags := types.ListValue(v3ConditionObjectType, elements)
	diags.Append(listDiags...)
	if listDiags.HasError() {
		return types.ListValueMust(v3ConditionObjectType, []attr.Value{})
	}
	return list
}

// v3SetConditionsState writes the projected conditions and the derived `ready`
// convenience together, so the boolean can never disagree with the list it is
// derived from.
func v3SetConditionsState(ctx context.Context, state *tfsdk.State, res *clientv3.Resource) diag.Diagnostics {
	var diags diag.Diagnostics
	conditions := v3ConditionsState(res, &diags)
	diags.Append(state.SetAttribute(ctx, path.Root("conditions"), conditions)...)
	diags.Append(state.SetAttribute(ctx, path.Root("ready"), types.BoolValue(clientv3.ResourceReady(res)))...)
	return diags
}
