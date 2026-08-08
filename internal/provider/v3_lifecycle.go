package provider

// v3_lifecycle.go is the shared lifecycle core of the Host API v1alpha3
// resource lane (spec/decisions/0013): every typed Edge Family resource
// drives the same create/read/update/delete/import flow over the v1alpha3
// client.
// State identity is space/apiVersion/kind/uid (spec/decisions/0011); desired
// mutations fence on the expected generation, deletes fence on the revision,
// and there is no global mutation mutex.

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

// Default per-resource operation timeouts of the v3 lane. Each resource may
// override them through the optional create_timeout/update_timeout/
// delete_timeout attributes (time.ParseDuration strings such as "20m").
const (
	v3DefaultCreateTimeout = 20 * time.Minute
	v3DefaultUpdateTimeout = 20 * time.Minute
	v3DefaultDeleteTimeout = 30 * time.Minute
)

// v3FormResource implements one typed Edge Platform Family Form over the
// shared v1alpha3 lifecycle core. The Form declaration is data
// (internal/edgeformcatalog); a new family member is a new catalog entry.
type v3FormResource struct {
	form model.Form
	data *providerData
	// codecs is the exact-FormRef dispatch table: the registry of identities
	// this resource can serve together with the per-ref codec that decodes
	// state written under each one. Production constructions carry the build's
	// own table (v3Codecs); it is a real dependency, not an override, so the
	// same code path serves one definition version and several.
	codecs *v3CodecTable
}

var (
	_ resource.Resource                = (*v3FormResource)(nil)
	_ resource.ResourceWithImportState = (*v3FormResource)(nil)
	_ resource.ResourceWithConfigure   = (*v3FormResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*v3FormResource)(nil)
)

// NewV3FormResource returns a constructor for one declared family Form.
func NewV3FormResource(form model.Form) func() resource.Resource {
	return func() resource.Resource { return &v3FormResource{form: form, codecs: v3Codecs()} }
}

// newV3FormResources lists every v3-lane resource constructor: exactly the
// typed family members, one per catalog Form.
//
// There is deliberately no generic exact-FormRef carrier. A resource that
// accepts an arbitrary third-party FormRef and an opaque JSON spec can back
// none of what an exact reference promises: the v1alpha3 Form Definition
// response is a closed envelope carrying only identity, display name,
// description, and desiredSchema, so a client can neither recompute the
// canonical definition digest the FormRef pins nor read the Form's role.
// Shipping the carrier anyway would have offered reach with no verification
// behind it (spec/decisions/0021).
func newV3FormResources() []func() resource.Resource {
	out := make([]func() resource.Resource, 0, len(edgeformcatalog.Forms))
	for _, form := range edgeformcatalog.Forms {
		out = append(out, NewV3FormResource(form))
	}
	return out
}

func (r *v3FormResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = r.form.ResourceType
}

func (r *v3FormResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("Expected *providerData, got %T. This is a provider bug.", req.ProviderData),
		)
		return
	}
	r.data = data
}

// assertV3Configured requires the v1alpha3 lane. When the endpoint only
// negotiated v1alpha2, the recorded per-lane negotiation error is the
// diagnostic, so the user sees why this resource cannot work while the v2
// resources can.
func (r *v3FormResource) assertV3Configured(diags *diag.Diagnostics) bool {
	return assertV3Lane(r.data, r.form.ResourceType, diags)
}

func assertV3Lane(data *providerData, resourceType string, diags *diag.Diagnostics) bool {
	if data == nil {
		diags.AddError(
			"Provider not configured",
			"The takoform provider was not configured before use. This is usually a provider bug.",
		)
		return false
	}
	if data.clientV3 == nil {
		detail := "The configured endpoint did not negotiate the Host API v1alpha3 lane required by " + resourceType + "."
		if data.v3Err != nil {
			detail += " " + data.v3Err.Error()
		}
		diags.AddError("Takoform v1alpha3 lane unavailable", detail)
		return false
	}
	return true
}

// codecTable is the exact-FormRef dispatch table this resource serves. A
// resource constructed without one falls back to the build's own table, so a
// direct struct literal is never silently registry-less.
func (r *v3FormResource) codecTable() *v3CodecTable {
	if r.codecs != nil {
		return r.codecs
	}
	return v3Codecs()
}

// v3DefaultCodec is the recommended create target of this Form's kind together
// with the codec that encodes a spec for it. Only Create uses it: an existing
// resource dispatches on the identity recorded in state.
func (r *v3FormResource) v3DefaultCodec(diags *diag.Diagnostics) (v3FormCodec, bool) {
	codec, err := r.codecTable().defaultCreate(r.form.Kind)
	if err != nil {
		diags.AddError(r.form.Kind+" FormRef missing",
			"This provider build has no exact candidate "+r.form.Kind+" FormRef: "+err.Error()+" This is a provider bug.")
		return v3FormCodec{}, false
	}
	return codec, true
}

// v3StateCodec resolves the exact FormRef recorded in state against the
// supported multi-FormRef registry, and returns the codec that decodes state
// written under it. Membership — not equality with the current default create
// target — decides dispatch, so a Form line bump keeps existing state
// readable. An identity this build cannot decode fails closed naming the state
// ref and every ref of that kind the build knows; the provider never queries a
// different exact FormRef and interprets a 404 as deletion.
func (r *v3FormResource) v3StateCodec(identity v3StateIdentity, diags *diag.Diagnostics) (v3FormCodec, bool) {
	got, ok := identity.formRef()
	if !ok {
		diags.AddError(
			"State has no exact v1alpha3 Form identity",
			"The v1alpha3 resource lane fails closed on state without a complete exact FormRef. Retained v2-lane state is never transformed in place; perform an explicit create/import migration.",
		)
		return v3FormCodec{}, false
	}
	table := r.codecTable()
	if codec, supported := table.forStateKey(got.exactKey()); supported {
		return codec, true
	}
	diags.Append(v3UnsupportedStateRefError(r.form.Kind, got, table.knownRefsForKind(r.form.Kind)))
	return v3FormCodec{}, false
}

func clientFormRef(ref currentformregistry.V3Ref) clientv3.FormRef {
	return clientv3.FormRef{
		APIVersion:        ref.APIVersion,
		Kind:              ref.Kind,
		DefinitionVersion: ref.DefinitionVersion,
		SchemaDigest:      ref.SchemaDigest,
	}
}

// v3RequestResource builds the wire envelope for one apply. The request never
// carries a packageDigest: the digest is audit evidence of the host's own
// installation, never client-asserted identity.
func v3RequestResource(ref currentformregistry.V3Ref, name, space string, spec map[string]any) *clientv3.Resource {
	return &clientv3.Resource{
		APIVersion: ref.APIVersion,
		Kind:       ref.Kind,
		Form:       &clientv3.FormReference{FormRef: clientFormRef(ref)},
		Metadata:   clientv3.Metadata{Name: name, Space: space},
		Spec:       spec,
	}
}

func (r *v3FormResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if !r.assertV3Configured(&resp.Diagnostics) {
		return
	}
	values, diags := r.v3ValuesFrom(ctx, req.Plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	codec, ok := r.v3DefaultCodec(&resp.Diagnostics)
	if !ok {
		return
	}
	space, ok := v3EffectiveSpace(values.Space, r.data.defaultSpace, &resp.Diagnostics)
	if !ok {
		return
	}
	var spec map[string]any
	var bundle v3BundleAuthoring
	if r.form.Kind == workerBundleKind {
		// A worker bundle is authored either by referencing a committed manifest
		// or from local module files whose bytes travel through the
		// content-addressed artifact upload, never through state. Both modes
		// resolve to the same one-field desired spec.
		resolved, bundleDiags := r.workerBundleAuthoring(&values)
		resp.Diagnostics.Append(bundleDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		bundle = resolved
		spec = bundle.Spec()
	} else {
		var specDiags diag.Diagnostics
		spec, specDiags = r.v3SpecFromValues(ctx, codec, values)
		resp.Diagnostics.Append(specDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	timeout, ok := v3Timeout(values.CreateTimeout, "create_timeout", v3DefaultCreateTimeout, &resp.Diagnostics)
	if !ok {
		return
	}
	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if bundle.Local {
		// The digest the commit RETURNED is the desired state: a client never
		// asserts an artifact identity the host has not issued.
		committed, ok := r.uploadWorkerBundle(opCtx, bundle, &resp.Diagnostics)
		if !ok {
			return
		}
		spec = map[string]any{"manifestDigest": committed}
		values.Fields["manifest_digest"] = types.StringValue(committed)
	}
	res, err := r.data.clientV3.ApplyResource(opCtx, v3RequestResource(codec.Ref, values.Name.ValueString(), space, spec), clientv3.Fence{})
	if err != nil {
		// A create the host ACCEPTED can still fail here — most visibly when its
		// long-running Operation outlives create_timeout. Terraform commits the
		// state a failed Create leaves behind, so returning without writing state
		// orphans a resource the host owns and the next plan creates a duplicate.
		// Record the identity that is known before surfacing the failure.
		r.writeV3AcceptedState(ctx, &resp.State, codec, space, values, err, &resp.Diagnostics)
		resp.Diagnostics.AddError("Failed to create "+r.form.Kind, err.Error())
		return
	}
	resp.Diagnostics.Append(r.writeV3State(ctx, &resp.State, codec, space, values, res, false)...)
}

func (r *v3FormResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if !r.assertV3Configured(&resp.Diagnostics) {
		return
	}
	values, diags := r.v3ValuesFrom(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	codec, ok := r.v3StateCodec(values.Identity, &resp.Diagnostics)
	if !ok {
		return
	}
	space, ok := v3EffectiveSpace(values.Space, r.data.defaultSpace, &resp.Diagnostics)
	if !ok {
		return
	}
	// An accepted-but-unfinished mutation is consulted BEFORE the resource is
	// read: on a host where the resource does not exist until the operation
	// commits, a 404 during that window is not deletion.
	resume := v3ResumePendingOperation(
		ctx, r.data.clientV3, r.form.Kind, v3PendingRequest{
			OperationID: v3StateStringValue(values.PendingOperationID),
			Ref:         clientFormRef(codec.Ref),
			Space:       space,
			Name:        values.Name.ValueString(),
			StateUID:    v3StateStringValue(values.UID),
		}, &resp.Diagnostics)
	if resume.Stop {
		return
	}
	res, err := r.data.clientV3.GetResource(ctx, space, clientFormRef(codec.Ref), values.Name.ValueString())
	if err != nil {
		if errors.Is(err, clientv3.ErrNotFound) {
			if !resume.RemoveOnAbsent {
				// The host accepted a mutation that has not committed. Absence is
				// the pending window, not a deletion, so state stays exactly as it
				// is and the recorded operation id survives for the next read.
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read "+r.form.Kind, err.Error())
		return
	}
	if !v3RequireStateUID(r.form.Kind, space, values.Name.ValueString(), v3StateStringValue(values.UID), res, &resp.Diagnostics) {
		// State is DELIBERATELY kept: the resource is still under management and
		// the operator must choose which incarnation it names.
		return
	}
	v3ReportRelationCondition(
		r.form.Kind, space, values.Name.ValueString(), res, r.form.DeclaresUpdate(), &resp.Diagnostics,
	)
	// Only a Form that declares update can converge an out-of-band change in
	// place, so only there does adopting the host's spec produce a plan the
	// next apply can satisfy. For a Form without update every desired attribute
	// forces replacement, and the recorded configuration is preserved.
	resp.Diagnostics.Append(r.writeV3State(ctx, &resp.State, codec, space, values, res, r.form.DeclaresUpdate())...)
	// A read that settled a representation while the accepted operation is
	// still running keeps the marker: the operation has not committed, so there
	// is still something to resume.
	if resume.KeepMarker {
		resp.Diagnostics.Append(resp.State.SetAttribute(
			ctx, path.Root("pending_operation_id"), values.PendingOperationID)...)
	}
}

// v3RelationConditionReasons are the two portable reasons a host uses when a
// stored cross-resource relation no longer resolves to the resource it was
// bound to. Both mean the same thing to a client: this resource is pinned to an
// incarnation that is gone, and only a re-apply can re-resolve the reference.
var v3RelationConditionReasons = map[string]string{
	"ExternalChange":    "a referenced resource was replaced by a different incarnation with the same name",
	"DependencyMissing": "a referenced resource no longer exists",
}

// v3RelationDrift reports the relation-drift condition the host published, if
// any: the exact portable reason and the prose summary of what it means.
func v3RelationDrift(res *clientv3.Resource) (reason, summary, hostReason string, drifted bool) {
	condition := clientv3.ResourceCondition(res, "Ready")
	if condition == nil || condition.Status != "False" {
		return "", "", "", false
	}
	summary, tracked := v3RelationConditionReasons[condition.Reason]
	if !tracked {
		return "", "", "", false
	}
	return condition.Reason, summary, condition.HostReason, true
}

// v3RelationDriftState projects that condition onto the recovery attribute:
// the portable reason while a relation is broken, null once it resolves again.
func v3RelationDriftState(res *clientv3.Resource) types.String {
	reason, _, _, drifted := v3RelationDrift(res)
	if !drifted {
		return types.StringNull()
	}
	return types.StringValue(reason)
}

// v3ReportRelationCondition WARNS about a relation the host reports as broken,
// and never fails the read.
//
// Failing here would take the remedy away. A read is Terraform's refresh, and
// the plan that repairs the resource is computed from the refreshed state, so
// an error aborts the very plan that would fix it. Skipping refresh is no
// escape either: for a revision Form the host refuses every apply to an
// existing resource, so the resource would stay pinned to a dead target until
// someone edited state by hand. The warning keeps the resource in state, and
// the recorded relation_drift_reason lets the next plan propose the apply that
// re-resolves the reference (v3PlanRelationRecovery).
func v3ReportRelationCondition(
	kind, space, name string,
	res *clientv3.Resource,
	declaresUpdate bool,
	diags *diag.Diagnostics,
) {
	reason, summary, hostReason, drifted := v3RelationDrift(res)
	if !drifted {
		return
	}
	remedy := "replacing this resource, because its Form declares no in-place update"
	if declaresUpdate {
		remedy = "re-applying this resource in place"
	}
	detail := fmt.Sprintf(
		"The host reports %s/%s as not ready with reason %s: %s. It stays pinned to the incarnation it was applied against, because the host never re-binds a reference by name.",
		space, name, reason, summary,
	)
	if hostReason != "" {
		detail += " The host names the relation pointer and both uids: " + hostReason + "."
	}
	detail += " The next plan proposes " + remedy +
		", and that apply re-resolves the reference against the resource that exists now." +
		" A target that no longer exists must be re-created first, or this resource removed with it."
	diags.AddWarning(kind+" references a resource that changed out of band", detail)
}

// ModifyPlan carries the two plan-time facts of the v3 lane: a relation the
// host reports as broken is planned into an apply that can repair it, and a
// worker bundle's identity follows its bytes.
func (r *v3FormResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	v3PlanRelationRecovery(ctx, req.State, r.form.DeclaresUpdate(), resp)
	if r.form.Kind == workerBundleKind {
		r.modifyWorkerBundlePlan(ctx, req, resp)
	}
}

// v3PlanRelationRecovery turns a recorded relation break into a plan Terraform
// can act on.
//
// Clearing the recorded reason in the plan is what makes the plan non-empty:
// Terraform treats a planned state identical to prior state as a no-op and
// never calls apply, so a replacement marker on its own would be dropped
// silently. Clearing it is also the honest prediction, because a successful
// apply is exactly what clears it.
//
// Which apply is proposed follows the Form's own capability. A Form that
// declares update recovers in place — the host re-resolves and re-pins every
// relation on any accepted apply, so a spec-identical apply is the whole
// remedy. A Form that declares none has no in-place apply at all: the host
// refuses every apply to the existing resource, so the only reachable remedy
// is replacement.
func v3PlanRelationRecovery(
	ctx context.Context,
	state tfsdk.State,
	declaresUpdate bool,
	resp *resource.ModifyPlanResponse,
) {
	// A create resolves every relation for the first time and a destroy needs
	// no repair; only an existing resource can carry a stale pin.
	if state.Raw.IsNull() || resp.Plan.Raw.IsNull() {
		return
	}
	var recorded types.String
	resp.Diagnostics.Append(state.GetAttribute(ctx, path.Root(v3RelationDriftAttribute), &recorded)...)
	if resp.Diagnostics.HasError() || recorded.IsNull() || recorded.IsUnknown() {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(v3RelationDriftAttribute), types.StringNull())...)
	if resp.Diagnostics.HasError() || declaresUpdate {
		return
	}
	if !resp.RequiresReplace.Contains(path.Root(v3RelationDriftAttribute)) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root(v3RelationDriftAttribute))
	}
}

func (r *v3FormResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if !r.assertV3Configured(&resp.Diagnostics) {
		return
	}
	if !r.form.DeclaresUpdate() {
		// This Form declares no update capability, so every DESIRED attribute
		// requires replacement and the only legal in-place update is one
		// confined to the provider-side operation timeout attributes. That
		// change mutates no host state and makes no host call.
		r.updateProviderSideTimeouts(ctx, req, resp)
		return
	}
	values, diags := r.v3ValuesFrom(ctx, req.Plan)
	resp.Diagnostics.Append(diags...)
	stateValues, stateDiags := r.v3ValuesFrom(ctx, req.State)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	codec, ok := r.v3StateCodec(stateValues.Identity, &resp.Diagnostics)
	if !ok {
		return
	}
	space, ok := v3EffectiveSpace(values.Space, r.data.defaultSpace, &resp.Diagnostics)
	if !ok {
		return
	}
	// The spec travels under the identity recorded in STATE, encoded by that
	// ref's own field set: an update is a mutation of the resource that exists,
	// never a silent migration onto the current create default.
	spec, specDiags := r.v3SpecFromValues(ctx, codec, values)
	resp.Diagnostics.Append(specDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	fence := clientv3.Fence{
		ExpectedUID:        stateValues.UID.ValueString(),
		ExpectedGeneration: stateValues.Generation.ValueString(),
	}
	timeout, ok := v3Timeout(values.UpdateTimeout, "update_timeout", v3DefaultUpdateTimeout, &resp.Diagnostics)
	if !ok {
		return
	}
	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res, err := r.data.clientV3.ApplyResource(opCtx, v3RequestResource(codec.Ref, values.Name.ValueString(), space, spec), fence)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update "+r.form.Kind, err.Error())
		return
	}
	resp.Diagnostics.Append(r.writeV3State(ctx, &resp.State, codec, space, values, res, false)...)
}

// updateProviderSideTimeouts handles the one in-place update a Form without
// the update capability accepts: a change confined to the provider-side
// create_timeout/delete_timeout attributes. The planned wire spec is compared
// against state; when identical, the planned timeouts are written over the
// prior state without any host call. Any desired difference should have been
// forced into a replacement by the plan modifiers, so it stays a hard error.
func (r *v3FormResource) updateProviderSideTimeouts(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	planValues, diags := r.v3ValuesFrom(ctx, req.Plan)
	resp.Diagnostics.Append(diags...)
	stateValues, stateDiags := r.v3ValuesFrom(ctx, req.State)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	codec, ok := r.v3StateCodec(stateValues.Identity, &resp.Diagnostics)
	if !ok {
		return
	}
	if !r.v3DesiredSpecUnchanged(ctx, codec, planValues, stateValues, &resp.Diagnostics) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError(
				r.form.Kind+" declares no update capability",
				"This Form has no mutable desired field, so desired changes replace the resource. "+
					"This in-place update carries a desired-spec change, which is a provider bug.",
			)
		}
		return
	}
	resp.State.Raw = req.State.Raw
	resp.State.Schema = req.State.Schema
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("create_timeout"), planValues.CreateTimeout)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("delete_timeout"), planValues.DeleteTimeout)...)
	if r.form.Kind == workerBundleKind {
		// The bundle's desired state is unchanged, but its LOCAL authoring may
		// have moved — a manifest_digest reference replaced by the files that
		// commit that exact manifest. Those attributes are provider-side facts,
		// so recording the planned ones settles the plan without any host call.
		resp.Diagnostics.Append(resp.State.SetAttribute(
			ctx, path.Root("main_module"), v3StateStringOf(planValues.Fields["main_module"]))...)
		modules, ok := planValues.Fields["modules"].(types.List)
		if !ok || modules.IsUnknown() {
			modules = types.ListNull(v3WorkerBundleModuleType())
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("modules"), modules)...)
	}
}

// v3DesiredSpecUnchanged reports whether a planned in-place update leaves the
// desired identity and wire spec exactly as recorded in state. A worker bundle
// resolves both sides through its authoring modes and compares the resulting
// manifest digest, because that digest IS its desired state: local files that
// commit the manifest already in state are the same bundle, not a change.
// Every other Form compares the projected wire spec.
func (r *v3FormResource) v3DesiredSpecUnchanged(ctx context.Context, codec v3FormCodec, plan, state v3Values, diags *diag.Diagnostics) bool {
	if !plan.Name.Equal(state.Name) || !plan.Space.Equal(state.Space) {
		return false
	}
	if r.form.Kind == workerBundleKind {
		planned, plannedDiags := r.workerBundleAuthoring(&plan)
		diags.Append(plannedDiags...)
		if diags.HasError() {
			return false
		}
		// The recorded manifest digest is the desired state as the host holds
		// it. Re-deriving the prior side from the local files instead would
		// compare the working tree against itself and call a real byte change no
		// change at all.
		recorded, ok := v3PlanKnownString(state.Fields["manifest_digest"])
		return ok && planned.Digest == recorded
	}
	plannedSpec, planDiags := r.v3SpecFromValues(ctx, codec, plan)
	diags.Append(planDiags...)
	stateSpec, stateDiags := r.v3SpecFromValues(ctx, codec, state)
	diags.Append(stateDiags...)
	if diags.HasError() {
		return false
	}
	return reflect.DeepEqual(plannedSpec, stateSpec)
}

func (r *v3FormResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if !r.assertV3Configured(&resp.Diagnostics) {
		return
	}
	values, diags := r.v3ValuesFrom(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	codec, ok := r.v3StateCodec(values.Identity, &resp.Diagnostics)
	if !ok {
		return
	}
	space, ok := v3EffectiveSpace(values.Space, r.data.defaultSpace, &resp.Diagnostics)
	if !ok {
		return
	}
	timeout, ok := v3Timeout(values.DeleteTimeout, "delete_timeout", v3DefaultDeleteTimeout, &resp.Diagnostics)
	if !ok {
		return
	}
	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := r.data.clientV3.DeleteResource(opCtx, space, clientFormRef(codec.Ref), values.Name.ValueString(), values.Revision.ValueString())
	if err != nil && !errors.Is(err, clientv3.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete "+r.form.Kind, err.Error())
	}
}

// ImportState adopts one existing resource into state under an EXACT identity.
//
// Three forms are accepted (v3ParseImportID): the canonical JSON object, which
// names the exact FormRef and therefore the incarnation an older definition
// version created; and the short `NAME` / `SPACE/NAME` forms, which resolve to
// the default create ref. The short forms stay because they are what almost
// every import is, and the JSON form exists because a delimiter-joined string
// cannot carry a SpaceID safely — a SpaceID is opaque UTF-8 that may contain
// any character but `/`, so no separator choice both escapes it and stays
// readable.
func (r *v3FormResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !r.assertV3Configured(&resp.Diagnostics) {
		return
	}
	identity, err := v3ParseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	codec, ok := r.v3ImportCodec(identity, &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(identity.Name))...)
	if identity.Space != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space"), types.StringValue(identity.Space))...)
	}
	resp.Diagnostics.Append(setV3FormIdentityState(ctx, &resp.State, codec.Ref)...)
}

// v3ImportCodec resolves the codec one import names: the exact recorded
// identity when the canonical JSON form supplied one, the default create ref
// otherwise. An exact identity this build cannot decode fails closed — the
// import must not silently rebind the resource onto the current definition
// version, which is exactly the bug the short forms had.
func (r *v3FormResource) v3ImportCodec(identity v3ImportIdentity, diags *diag.Diagnostics) (v3FormCodec, bool) {
	if !identity.HasFormRef {
		return r.v3DefaultCodec(diags)
	}
	if identity.Ref.Kind != r.form.Kind {
		diags.AddError(
			"Import ID names another Form kind",
			fmt.Sprintf(
				"The import identity names kind %q, but this resource type carries %q.",
				identity.Ref.Kind, r.form.Kind,
			),
		)
		return v3FormCodec{}, false
	}
	table := r.codecTable()
	codec, supported := table.forStateKey(identity.Ref.exactKey())
	if !supported {
		diags.Append(v3UnsupportedImportRefError(identity.Ref, table.knownRefsForKind(r.form.Kind)))
		return v3FormCodec{}, false
	}
	return codec, true
}

// v3EffectiveSpace resolves and validates the effective SpaceID like the v2
// lane: the resource attribute wins over the provider default; the exact
// value is preserved.
func v3EffectiveSpace(value types.String, fallback string, diags *diag.Diagnostics) (string, bool) {
	space, err := validatedEffectiveSpace(value, fallback)
	if err != nil {
		diags.AddAttributeError(
			path.Root("space"),
			"Invalid or missing SpaceID",
			"Set a valid resource SpaceID or configure a valid provider default: "+err.Error(),
		)
		return "", false
	}
	return space, true
}

// v3Timeout parses one optional duration attribute with its lane default.
func v3Timeout(value types.String, attribute string, fallback time.Duration, diags *diag.Diagnostics) (time.Duration, bool) {
	if value.IsNull() || value.IsUnknown() {
		return fallback, true
	}
	parsed, err := time.ParseDuration(value.ValueString())
	if err != nil || parsed <= 0 {
		diags.AddAttributeError(
			path.Root(attribute),
			"Invalid operation timeout",
			attribute+" must be a positive Go duration such as \"20m\".",
		)
		return 0, false
	}
	return parsed, true
}
