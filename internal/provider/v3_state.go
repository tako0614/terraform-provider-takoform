package provider

// v3_state.go carries the state model of the Host API v1beta1 resource
// lane: reading plan/state values generically, and projecting host responses
// back into Terraform state. State identity is space/apiVersion/kind/uid;
// the packageDigest is audit evidence only and never identity
// (spec/decisions/0011).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
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
	codec v3FormCodec,
	space string,
	values v3Values,
	res *clientv3.Resource,
	adoptHostSpec bool,
) diag.Diagnostics {
	// Every caller of this entry point holds a representation the host answered
	// with, so every declared output must be in it. The one write that does not
	// is writeV3AcceptedState, which reaches the private form below.
	return r.writeV3StateFrom(ctx, state, codec, space, values, res, adoptHostSpec, v3VerifiedRepresentation)
}

// v3ResponseKind distinguishes the two state writes an apply can produce. It is
// a named type rather than a second bare bool because the two paths differ in
// exactly one contractual way — whether the host answered with a representation
// — and that difference decides whether a missing output is a null or a fault.
type v3ResponseKind bool

const (
	// v3VerifiedRepresentation is a response the host answered with. A response
	// carrying status carries every output its Form declares, so an omitted or
	// wrongly-typed one is the host's fault and is reported.
	v3VerifiedRepresentation v3ResponseKind = true
	// v3AcceptedWithoutRepresentation is the recovery write of a mutation the
	// host ACCEPTED but did not finish. There is no representation, so every
	// output is legitimately null (spec/decisions/0017).
	v3AcceptedWithoutRepresentation v3ResponseKind = false
)

func (r *v3FormResource) writeV3StateFrom(
	ctx context.Context,
	state *tfsdk.State,
	codec v3FormCodec,
	space string,
	values v3Values,
	res *clientv3.Resource,
	adoptHostSpec bool,
	response v3ResponseKind,
) diag.Diagnostics {
	var diags diag.Diagnostics
	ref := codec.Ref
	// A representation from the Host is untrusted input. Validate the complete
	// desired document against the schema carried by THIS exact FormRef before
	// writing even identity or status into Terraform state. In particular, the
	// provider must not quietly turn a missing/defaulted/nested field into null
	// or decode an older/newer Form's shape under this identity.
	if response == v3VerifiedRepresentation {
		if err := v3ValidateHostSpec(codec, res.Spec); err != nil {
			diags.Append(v3InvalidHostSpecError(codec, err))
			return diags
		}
	}
	diags.Append(state.SetAttribute(ctx, path.Root("name"), types.StringValue(res.Metadata.Name))...)
	diags.Append(state.SetAttribute(ctx, path.Root("space"), types.StringValue(space))...)
	diags.Append(state.SetAttribute(ctx, path.Root("uid"), types.StringValue(res.Metadata.UID))...)
	diags.Append(state.SetAttribute(ctx, path.Root("generation"), types.StringValue(res.Metadata.Generation))...)
	diags.Append(state.SetAttribute(ctx, path.Root("revision"), types.StringValue(res.Metadata.Revision))...)
	diags.Append(v3SetConditionsState(ctx, state, res)...)
	diags.Append(state.SetAttribute(ctx, path.Root("outputs_json"), v3OutputsJSON(res, &diags))...)
	diags.Append(v3SetOutputState(ctx, state, codec.Form, res, response)...)
	diags.Append(setV3FormIdentityState(ctx, state, ref)...)
	// A verified representation settles any earlier accepted-but-unfinished
	// mutation: there is nothing left to resume.
	diags.Append(state.SetAttribute(ctx, path.Root("pending_operation_id"), types.StringNull())...)
	// Whether a relation is broken is re-derived from THIS representation on
	// every write, so an apply that re-pinned the reference clears the recovery
	// signal by the same rule that set it.
	diags.Append(state.SetAttribute(ctx, path.Root(v3RelationDriftAttribute), v3RelationDriftState(res))...)
	// The owner is authoring input the host never echoes, so it is carried
	// through from the configuration rather than read back from the response.
	if r.derivesRevisionName() {
		diags.Append(state.SetAttribute(ctx, path.Root(v3RevisionOwnerAttribute), values.RevisionOwner)...)
	}
	diags.Append(state.SetAttribute(ctx, path.Root("create_timeout"), values.CreateTimeout)...)
	if r.form.DeclaresUpdate() {
		diags.Append(state.SetAttribute(ctx, path.Root("update_timeout"), values.UpdateTimeout)...)
	}
	diags.Append(state.SetAttribute(ctx, path.Root("delete_timeout"), values.DeleteTimeout)...)
	if r.form.Kind == workerBundleKind {
		diags.Append(r.writeWorkerBundleState(ctx, state, values, res)...)
		return diags
	}
	if _, fileArtifact := v3FileBundleManifestKind(r.form.Kind); fileArtifact {
		diags.Append(r.writeFileBundleState(ctx, state, values, res)...)
		return diags
	}
	// The state ref's OWN codec decides which fields exist and how they decode:
	// state written under an earlier definition version is read back through
	// that definition's declarations, never through the current schema's
	// (spec/decisions/0017).
	for _, field := range codec.Form.Fields {
		name := v3AttributeName(field)
		value, carried := values.Fields[name]
		if !carried {
			diags.Append(v3CodecFieldMissingError(codec, name))
			continue
		}
		if adoptHostSpec || value == nil || value.IsUnknown() {
			value = v3FieldValueFromSpec(ctx, codec.Form.Family.APIVersion(), field, res.Spec[field.Wire], &diags)
		}
		diags.Append(state.SetAttribute(ctx, path.Root(name), value)...)
	}
	return diags
}

// v3ValidateHostSpec validates a Host representation through shared,
// provider-neutral Form semantics. The exact Definition schema rejects
// missing required and explicit-null typed fields. Portable defaults are
// required to have been materialized by the Host before it echoes the
// representation; only properties whose absence is semantic may remain
// absent. Structural constraints are evaluated by the shared Form model, not
// reimplemented as provider-specific rules.
func v3ValidateHostSpec(codec v3FormCodec, spec map[string]any) error {
	if codec.DesiredSchema == nil {
		return errors.New("exact Form codec has no desired schema")
	}
	if err := formpackage.ValidateDesiredInstance(codec.DesiredSchema, spec); err != nil {
		return err
	}
	materialized := model.MaterializeDefaults(codec.DesiredSchema, spec)
	equal, err := v3CanonicalJSONEqual(spec, materialized)
	if err != nil {
		return fmt.Errorf("compare Host spec with its materialized defaults: %w", err)
	}
	if !equal {
		return errors.New("Host spec omits one or more portable defaulted fields")
	}
	if err := model.ValidateStructuralConstraintValues(codec.Form.StructuralConstraints, spec); err != nil {
		return fmt.Errorf("Host spec violates a structural Form constraint: %w", err)
	}
	return nil
}

func v3CanonicalJSONEqual(left, right any) (bool, error) {
	leftRaw, err := json.Marshal(left)
	if err != nil {
		return false, err
	}
	rightRaw, err := json.Marshal(right)
	if err != nil {
		return false, err
	}
	leftCanonical, err := formpackage.Canonicalize(leftRaw)
	if err != nil {
		return false, err
	}
	rightCanonical, err := formpackage.Canonicalize(rightRaw)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftCanonical, rightCanonical), nil
}

func v3InvalidHostSpecError(codec v3FormCodec, err error) diag.Diagnostic {
	return diag.NewErrorDiagnostic(
		"Host returned a spec that violates its exact Form",
		fmt.Sprintf(
			"The Host representation at /spec does not satisfy %s: %v. "+
				"No part of this representation was written to Terraform state.",
			codec.Ref.ExactKey(), err,
		),
	)
}

// v3CodecFieldMissingError is the fail-closed diagnostic for a codec that
// declares a field this build's compiled resource schema has no attribute for.
// It can only happen when an exact ref's field set was registered against a
// schema that cannot represent it, and the honest answer is to refuse rather
// than to write a partial spec that silently drops desired state.
func v3CodecFieldMissingError(codec v3FormCodec, attribute string) diag.Diagnostic {
	return diag.NewErrorDiagnostic(
		"Form codec declares a field this provider build cannot carry",
		fmt.Sprintf(
			"The codec for %s declares %q, which this provider's compiled %s schema does not carry. "+
				"The provider refuses to encode or decode a partial spec. This is a provider bug.",
			codec.Ref.ExactKey(), attribute, codec.Ref.Kind,
		),
	)
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
	codec v3FormCodec,
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
		APIVersion: codec.Ref.APIVersion,
		Kind:       codec.Ref.Kind,
		Metadata: clientv3.Metadata{
			Name:  values.Name.ValueString(),
			Space: space,
			UID:   accepted.UID,
		},
	}
	diags.Append(r.writeV3StateFrom(
		ctx, state, codec, space, values, partial, false, v3AcceptedWithoutRepresentation,
	)...)
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

// v3SetOutputState writes the Form's declared outputs as typed state.
//
// It runs on EVERY state write, including the one an accepted-but-unfinished
// mutation leaves behind, where the host returned no representation and every
// output is therefore null. That completeness is the contract with the
// framework: a Computed attribute the provider leaves unset after an apply
// whose plan marked it unknown is "Provider produced inconsistent result after
// apply", and it is unset precisely on the recovery paths nobody exercises by
// hand (spec/decisions/0017).
//
// Null is right on that path and ONLY on that path. A response the host
// answered with is required to carry every declared output with its declared
// type, so converting a missing or wrongly-typed one into a quiet null would
// hand the practitioner a resource whose whole purpose — an address, a URL —
// reads as null in state and in every expression that consumes it, with the
// host's fault nowhere on screen. On a verified representation the write
// reports it instead, naming the output and what was wrong.
//
// The declarations come from the STATE ref's own codec, like every desired
// field: a resource recorded under an earlier definition version publishes that
// definition's outputs, never whatever this build's current Form declares.
func v3SetOutputState(
	ctx context.Context,
	state *tfsdk.State,
	form model.Form,
	res *clientv3.Resource,
	response v3ResponseKind,
) diag.Diagnostics {
	var diags diag.Diagnostics
	var outputs map[string]any
	if res.Status != nil {
		outputs = res.Status.Outputs
	}
	for _, output := range form.Outputs {
		raw, present := outputs[output.Wire]
		if response == v3VerifiedRepresentation {
			if problem := v3OutputViolation(output, raw, present, res.Status != nil); problem != "" {
				diags.AddError(
					"Host response does not satisfy the declared output "+output.Wire,
					fmt.Sprintf(
						"The %s Form declares the output %q, so every %s representation the host answers with "+
							"carries it: %s. The provider does not record a null in place of a value the Form's "+
							"outputSchema requires, because a null here reads as \"not assigned yet\" in every "+
							"expression that consumes it. This is a fault in the host, not in the configuration.",
						form.Kind, output.Wire, form.Kind, problem,
					),
				)
			}
		}
		diags.Append(state.SetAttribute(
			ctx, path.Root(output.AttributeName()), v3OutputValue(output, raw),
		)...)
	}
	return diags
}

// v3OutputViolation reports why one declared output of a verified
// representation is unusable, or "" when it is what the Form declares.
//
// Each branch is exactly the condition under which v3OutputValue would produce
// a null from a value the host DID send, which is the case that must not pass
// silently: the two are one rule, stated once as a projection and once as a
// diagnostic.
func v3OutputViolation(output model.Field, raw any, present, carriesStatus bool) string {
	switch {
	case !carriesStatus:
		return "the response carries no status at all, so it carries no outputs"
	case !present:
		return "status.outputs omits it"
	case raw == nil:
		return "status.outputs carries it as null"
	}
	switch output.Kind {
	case model.KindBoolean:
		if _, ok := raw.(bool); !ok {
			return fmt.Sprintf("it is declared boolean and the host returned %s", v3OutputTypeProse(raw))
		}
	case model.KindInteger:
		if int64FromSpec(raw).IsNull() {
			return fmt.Sprintf("it is declared integer and the host returned %s", v3OutputTypeProse(raw))
		}
	default:
		// KindString and KindStringEnum. An empty string is a value the schema
		// bounds, not a type error, so it is left to the host's outputSchema and
		// to the conformance lane that enforces it.
		if _, ok := raw.(string); !ok {
			return fmt.Sprintf("it is declared string and the host returned %s", v3OutputTypeProse(raw))
		}
	}
	return ""
}

// v3OutputTypeProse names what a host actually returned, in JSON's vocabulary
// rather than Go's, because the practitioner is reading the host's response.
func v3OutputTypeProse(raw any) string {
	switch value := raw.(type) {
	case string:
		return fmt.Sprintf("the string %q", value)
	case bool:
		return fmt.Sprintf("the boolean %v", value)
	case json.Number:
		return "the number " + value.String()
	case float64:
		return fmt.Sprintf("the number %v", value)
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	}
	return fmt.Sprintf("%T", raw)
}

// v3OutputsJSON serializes the whole status outputs document deterministically;
// an absent outputs document is the empty object.
//
// It carries EVERYTHING the host returned, including every value that also has
// a typed attribute. Narrowing it to "the members no schema describes" was the
// alternative, and it would silently break every existing configuration that
// reads a now-typed key out of it: the expression would keep parsing, keep
// evaluating, and start producing null. Keeping the document whole makes the
// typed attributes a strictly additive surface — an author moves to `.url` when
// they want to, not when a provider upgrade forces them to — and leaves
// outputs_json doing the one job it is now for: reaching an output the Form's
// outputSchema does not describe.
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
	Name       types.String
	Space      types.String
	UID        types.String
	Generation types.String
	Revision   types.String
	Identity   v3StateIdentity
	// PendingOperationID is the recovery record of a mutation the host accepted
	// but that produced no verified representation. A read consults it before it
	// reads the resource (v3ResumePendingOperation).
	PendingOperationID types.String
	// RevisionOwner names who owns a derived revision. It is provider-side
	// authoring input that decides the derived NAME and never reaches the wire.
	RevisionOwner types.String
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

// exactKey is the registry lookup key of one recorded identity: the WHOLE
// four-member tuple, never a group+kind prefix of it.
func (value v3FormRefValue) exactKey() currentformregistry.ExactFormKey {
	return currentformregistry.ExactFormKey{
		APIVersion:        value.APIVersion,
		Kind:              value.Kind,
		DefinitionVersion: value.DefinitionVersion,
		SchemaDigest:      value.SchemaDigest,
	}
}

type v3AttributeGetter interface {
	GetAttribute(context.Context, path.Path, any) diag.Diagnostics
}

func (r *v3FormResource) v3ValuesFrom(ctx context.Context, getter v3AttributeGetter) (v3Values, diag.Diagnostics) {
	values, diags := v3CommonValuesFrom(ctx, getter, r.form.DeclaresUpdate())
	if r.derivesRevisionName() {
		diags.Append(getter.GetAttribute(ctx, path.Root(v3RevisionOwnerAttribute), &values.RevisionOwner)...)
	}
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
	if _, fileArtifact := v3FileBundleManifestKind(r.form.Kind); fileArtifact {
		var manifestDigest types.String
		var files types.List
		diags.Append(getter.GetAttribute(ctx, path.Root("manifest_digest"), &manifestDigest)...)
		diags.Append(getter.GetAttribute(ctx, path.Root("files"), &files)...)
		values.Fields["manifest_digest"] = manifestDigest
		values.Fields["files"] = files
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
		case model.KindStringList, model.KindBindingList, model.KindObjectList, model.KindResourceRefList,
			model.KindExternalServiceList:
			var value types.List
			diags.Append(getter.GetAttribute(ctx, path.Root(name), &value)...)
			values.Fields[name] = value
		case model.KindStringMap, model.KindStringSetMap:
			var value types.Map
			diags.Append(getter.GetAttribute(ctx, path.Root(name), &value)...)
			values.Fields[name] = value
		case model.KindObject, model.KindTaggedObject:
			var value types.Object
			diags.Append(getter.GetAttribute(ctx, path.Root(name), &value)...)
			values.Fields[name] = value
		case model.KindString, model.KindStringEnum, model.KindJSONMap, model.KindResourceRef:
			var value types.String
			diags.Append(getter.GetAttribute(ctx, path.Root(name), &value)...)
			values.Fields[name] = value
		default:
			diags.AddAttributeError(
				path.Root(name),
				"Unsupported Form field kind",
				fmt.Sprintf("Field %s uses unsupported FieldKind %q; the provider refuses a lossy fallback.", field.Wire, field.Kind),
			)
		}
	}
	if r.form.Kind == workerVersionKind {
		var value types.List
		diags.Append(getter.GetAttribute(ctx, path.Root(v3RetainedBucketBindingsAttribute), &value)...)
		values.Fields[v3RetainedBucketBindingsAttribute] = value
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
	diags.Append(getter.GetAttribute(ctx, path.Root("pending_operation_id"), &values.PendingOperationID)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("create_timeout"), &values.CreateTimeout)...)
	if hasUpdate {
		diags.Append(getter.GetAttribute(ctx, path.Root("update_timeout"), &values.UpdateTimeout)...)
	}
	diags.Append(getter.GetAttribute(ctx, path.Root("delete_timeout"), &values.DeleteTimeout)...)
	return values, diags
}
