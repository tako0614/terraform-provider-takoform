package provider

// v3_state.go carries the state model of the Host API v1alpha3 resource
// lane: reading plan/state values generically, and projecting host responses
// back into Terraform state. State identity is space/apiVersion/kind/uid;
// the packageDigest is audit evidence only and never identity
// (spec/decisions/0011).

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

func setV3FormIdentityState(ctx context.Context, state *tfsdk.State, ref currentformregistry.V3Ref) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.Append(state.SetAttribute(ctx, path.Root("form_api_version"), types.StringValue(ref.APIVersion))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_kind"), types.StringValue(ref.Kind))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_definition_version"), types.StringValue(ref.DefinitionVersion))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_schema_digest"), types.StringValue(ref.SchemaDigest))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_package_digest"), types.StringValue(ref.PackageDigest))...)
	return diags
}

// writeV3State projects one host response into Terraform state.
// adoptHostSpec selects whether declared desired fields are re-read from the
// response spec (a read of a Form that declares update adopts out-of-band
// changes so the next plan shows the drift) or preserved from the plan/state
// (creates, updates, and reads of a Form with no in-place update path).
func (r *v3FormResource) writeV3State(
	ctx context.Context,
	state *tfsdk.State,
	ref currentformregistry.V3Ref,
	space string,
	values v3Values,
	res *clientv3.Resource,
	adoptHostSpec bool,
) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.Append(state.SetAttribute(ctx, path.Root("name"), types.StringValue(res.Metadata.Name))...)
	diags.Append(state.SetAttribute(ctx, path.Root("space"), types.StringValue(space))...)
	diags.Append(state.SetAttribute(ctx, path.Root("uid"), types.StringValue(res.Metadata.UID))...)
	diags.Append(state.SetAttribute(ctx, path.Root("generation"), types.StringValue(res.Metadata.Generation))...)
	diags.Append(state.SetAttribute(ctx, path.Root("revision"), types.StringValue(res.Metadata.Revision))...)
	diags.Append(state.SetAttribute(ctx, path.Root("ready"), types.BoolValue(clientv3.ResourceReady(res)))...)
	diags.Append(state.SetAttribute(ctx, path.Root("outputs_json"), v3OutputsJSON(res, &diags))...)
	diags.Append(setV3FormIdentityState(ctx, state, ref)...)
	// A verified representation settles any earlier accepted-but-unfinished
	// mutation: there is nothing left to resume.
	diags.Append(state.SetAttribute(ctx, path.Root("pending_operation_id"), types.StringNull())...)
	diags.Append(state.SetAttribute(ctx, path.Root("create_timeout"), values.CreateTimeout)...)
	if r.form.DeclaresUpdate() {
		diags.Append(state.SetAttribute(ctx, path.Root("update_timeout"), values.UpdateTimeout)...)
	}
	diags.Append(state.SetAttribute(ctx, path.Root("delete_timeout"), values.DeleteTimeout)...)
	if r.form.Kind == workerBundleKind {
		diags.Append(r.writeWorkerBundleState(ctx, state, values, res)...)
		return diags
	}
	for _, field := range r.form.Fields {
		name := v3AttributeName(field)
		value := values.Fields[name]
		if adoptHostSpec || value == nil || value.IsUnknown() {
			value = v3FieldValueFromSpec(ctx, field, res.Spec[field.Wire], &diags)
		}
		diags.Append(state.SetAttribute(ctx, path.Root(name), value)...)
	}
	return diags
}

// writeV3AcceptedState records the identity of a mutation the host ACCEPTED
// but that produced no verified representation (clientv3.AcceptedError): the
// long-running Operation did not finish before the deadline, failed, or came
// back unreadable. Terraform commits the state a failed Create leaves behind,
// so writing nothing here orphans a resource the host now owns and the next
// plan proposes creating it a second time.
//
// What is written is exactly what is known without trusting an unverified
// response: the client-owned name and space, the exact FormRef the mutation
// targeted, the planned desired fields, and the host-issued uid plus the
// operation id when the accepted operation disclosed them. That is enough for
// the next Read to re-read the resource and reconcile.
//
// An error that is not an accepted mutation writes nothing: the host never
// took the request, so there is no resource to record.
func (r *v3FormResource) writeV3AcceptedState(
	ctx context.Context,
	state *tfsdk.State,
	ref currentformregistry.V3Ref,
	space string,
	values v3Values,
	err error,
	diags *diag.Diagnostics,
) {
	var accepted *clientv3.AcceptedError
	if !errors.As(err, &accepted) {
		return
	}
	partial := &clientv3.Resource{
		APIVersion: ref.APIVersion,
		Kind:       ref.Kind,
		Metadata: clientv3.Metadata{
			Name:  values.Name.ValueString(),
			Space: space,
			UID:   accepted.UID,
		},
	}
	diags.Append(r.writeV3State(ctx, state, ref, space, values, partial, false)...)
	diags.Append(state.SetAttribute(ctx, path.Root("pending_operation_id"), v3OptionalStateString(accepted.OperationID))...)
	diags.AddWarning(
		r.form.Kind+" was accepted by the host but did not complete",
		v3AcceptedRecoveryDetail(accepted, space, values.Name.ValueString(), r.form.Kind),
	)
}

// v3AcceptedRecoveryDetail explains what state now holds and what the operator
// should do next.
func v3AcceptedRecoveryDetail(accepted *clientv3.AcceptedError, space, name, kind string) string {
	detail := fmt.Sprintf(
		"The host accepted this %s mutation, so %s/%s may exist even though no verified representation came back. "+
			"State records the name, space, and exact Form identity (plus pending_operation_id when the host named an "+
			"operation) so the next plan reconciles the existing resource instead of creating a duplicate. "+
			"Run a refresh once the host settles.",
		kind, space, name,
	)
	if accepted.OperationID != "" {
		detail += " Host operation: " + accepted.OperationID + "."
	}
	if accepted.UID != "" {
		detail += " Host uid: " + accepted.UID + "."
	}
	return detail
}

// v3OptionalStateString renders an empty string as a null state value.
func v3OptionalStateString(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

// v3OutputsJSON serializes the typed status outputs deterministically;
// an absent outputs document is the empty object.
func v3OutputsJSON(res *clientv3.Resource, diags *diag.Diagnostics) types.String {
	if res.Status == nil || len(res.Status.Outputs) == 0 {
		return types.StringValue("{}")
	}
	raw, err := v3MarshalJSON(res.Status.Outputs)
	if err != nil {
		diags.AddError("Host outputs are not serializable", err.Error())
		return types.StringValue("{}")
	}
	return types.StringValue(raw)
}

// v3Values is the generically-read plan or state of one v3-lane resource.
type v3Values struct {
	Name          types.String
	Space         types.String
	UID           types.String
	Generation    types.String
	Revision      types.String
	Identity      v3StateIdentity
	CreateTimeout types.String
	UpdateTimeout types.String
	DeleteTimeout types.String
	// Fields is keyed by the HCL attribute name (v3AttributeName).
	Fields map[string]attr.Value
}

type v3StateIdentity struct {
	APIVersion        types.String
	Kind              types.String
	DefinitionVersion types.String
	SchemaDigest      types.String
}

type v3FormRefValue struct {
	APIVersion, Kind, DefinitionVersion, SchemaDigest string
}

func (identity v3StateIdentity) formRef() (v3FormRefValue, bool) {
	for _, value := range []types.String{identity.APIVersion, identity.Kind, identity.DefinitionVersion, identity.SchemaDigest} {
		if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
			return v3FormRefValue{}, false
		}
	}
	return v3FormRefValue{
		APIVersion:        identity.APIVersion.ValueString(),
		Kind:              identity.Kind.ValueString(),
		DefinitionVersion: identity.DefinitionVersion.ValueString(),
		SchemaDigest:      identity.SchemaDigest.ValueString(),
	}, true
}

type v3AttributeGetter interface {
	GetAttribute(context.Context, path.Path, any) diag.Diagnostics
}

func (r *v3FormResource) v3ValuesFrom(ctx context.Context, getter v3AttributeGetter) (v3Values, diag.Diagnostics) {
	values, diags := v3CommonValuesFrom(ctx, getter, r.form.DeclaresUpdate())
	if r.form.Kind == workerBundleKind {
		var manifestDigest, mainModule types.String
		var modules types.List
		diags.Append(getter.GetAttribute(ctx, path.Root("manifest_digest"), &manifestDigest)...)
		diags.Append(getter.GetAttribute(ctx, path.Root("main_module"), &mainModule)...)
		diags.Append(getter.GetAttribute(ctx, path.Root("modules"), &modules)...)
		values.Fields["manifest_digest"] = manifestDigest
		values.Fields["main_module"] = mainModule
		values.Fields["modules"] = modules
		return values, diags
	}
	for _, field := range r.form.Fields {
		name := v3AttributeName(field)
		switch field.Kind {
		case model.KindInteger:
			var value types.Int64
			diags.Append(getter.GetAttribute(ctx, path.Root(name), &value)...)
			values.Fields[name] = value
		case model.KindBoolean:
			var value types.Bool
			diags.Append(getter.GetAttribute(ctx, path.Root(name), &value)...)
			values.Fields[name] = value
		case model.KindStringSet:
			var value types.Set
			diags.Append(getter.GetAttribute(ctx, path.Root(name), &value)...)
			values.Fields[name] = value
		case model.KindBindingList, model.KindObjectList, model.KindResourceRefList:
			var value types.List
			diags.Append(getter.GetAttribute(ctx, path.Root(name), &value)...)
			values.Fields[name] = value
		default:
			var value types.String
			diags.Append(getter.GetAttribute(ctx, path.Root(name), &value)...)
			values.Fields[name] = value
		}
	}
	return values, diags
}

func v3CommonValuesFrom(ctx context.Context, getter v3AttributeGetter, hasUpdate bool) (v3Values, diag.Diagnostics) {
	var diags diag.Diagnostics
	values := v3Values{Fields: map[string]attr.Value{}}
	diags.Append(getter.GetAttribute(ctx, path.Root("name"), &values.Name)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("space"), &values.Space)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("uid"), &values.UID)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("generation"), &values.Generation)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("revision"), &values.Revision)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_api_version"), &values.Identity.APIVersion)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_kind"), &values.Identity.Kind)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_definition_version"), &values.Identity.DefinitionVersion)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_schema_digest"), &values.Identity.SchemaDigest)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("create_timeout"), &values.CreateTimeout)...)
	if hasUpdate {
		diags.Append(getter.GetAttribute(ctx, path.Root("update_timeout"), &values.UpdateTimeout)...)
	}
	diags.Append(getter.GetAttribute(ctx, path.Root("delete_timeout"), &values.DeleteTimeout)...)
	return values, diags
}
