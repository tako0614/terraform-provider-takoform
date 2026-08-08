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
	// supportedRefs overrides the exact-FormRef set that state dispatch accepts.
	// It is empty in every production construction (NewV3FormResource), where
	// currentformregistry is the sole authority. It exists so the lane can be
	// driven with several FormRefs of one Kind — the multi-FormRef case the
	// generated one-entry-per-(group,kind) registry map cannot express yet —
	// without introducing a mutable package-level seam.
	supportedRefs []currentformregistry.V3Ref
}

var (
	_ resource.Resource                = (*v3FormResource)(nil)
	_ resource.ResourceWithImportState = (*v3FormResource)(nil)
	_ resource.ResourceWithConfigure   = (*v3FormResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*v3FormResource)(nil)
)

// NewV3FormResource returns a constructor for one declared family Form.
func NewV3FormResource(form model.Form) func() resource.Resource {
	return func() resource.Resource { return &v3FormResource{form: form} }
}

// newV3FormResources lists every v3-lane resource constructor: the typed
// family members plus the generic exact-FormRef carrier.
func newV3FormResources() []func() resource.Resource {
	out := make([]func() resource.Resource, 0, len(edgeformcatalog.Forms)+1)
	for _, form := range edgeformcatalog.Forms {
		out = append(out, NewV3FormResource(form))
	}
	out = append(out, NewV3GenericResource)
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

// v3DefaultRef is the recommended create target of this Form's kind.
func (r *v3FormResource) v3DefaultRef(diags *diag.Diagnostics) (currentformregistry.V3Ref, bool) {
	ref, err := currentformregistry.V3ForKind(edgeformcatalog.Family.APIVersion(), r.form.Kind)
	if err != nil {
		diags.AddError(r.form.Kind+" FormRef missing",
			"This provider build has no exact candidate "+r.form.Kind+" FormRef: "+err.Error()+" This is a provider bug.")
		return currentformregistry.V3Ref{}, false
	}
	return ref, true
}

// v3SupportedRefs is every exact FormRef this build can read, observe, update,
// and delete.
func (r *v3FormResource) v3SupportedRefs() []currentformregistry.V3Ref {
	if len(r.supportedRefs) > 0 {
		return r.supportedRefs
	}
	return currentformregistry.V3SupportedFormRefs()
}

// v3StateRef resolves the exact FormRef recorded in state against the
// supported multi-FormRef registry. Membership — not equality with the
// current default create target — decides dispatch, so a future Form line
// bump keeps existing state readable. An unknown identity fails closed with
// both identities named; the provider never queries a different exact FormRef
// and interprets a 404 as deletion.
func (r *v3FormResource) v3StateRef(identity v3StateIdentity, diags *diag.Diagnostics) (currentformregistry.V3Ref, bool) {
	got, ok := identity.formRef()
	if !ok {
		diags.AddError(
			"State has no exact v1alpha3 Form identity",
			"The v1alpha3 resource lane fails closed on state without a complete exact FormRef. Retained v2-lane state is never transformed in place; perform an explicit create/import migration.",
		)
		return currentformregistry.V3Ref{}, false
	}
	for _, candidate := range r.v3SupportedRefs() {
		if candidate.APIVersion == got.APIVersion &&
			candidate.Kind == got.Kind &&
			candidate.DefinitionVersion == got.DefinitionVersion &&
			candidate.SchemaDigest == got.SchemaDigest {
			return candidate, true
		}
	}
	defaultRef, _ := currentformregistry.V3ForKind(edgeformcatalog.Family.APIVersion(), r.form.Kind)
	diags.AddError(
		"State Form identity is not supported by this provider",
		fmt.Sprintf(
			"State is bound to %s/%s@%s schema=%s; this provider supports %s/%s@%s schema=%s for kind %s. "+
				"Pin the provider version that wrote the state or perform an explicit create/import migration.",
			got.APIVersion, got.Kind, got.DefinitionVersion, got.SchemaDigest,
			defaultRef.APIVersion, defaultRef.Kind, defaultRef.DefinitionVersion, defaultRef.SchemaDigest,
			r.form.Kind,
		),
	)
	return currentformregistry.V3Ref{}, false
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
	ref, ok := r.v3DefaultRef(&resp.Diagnostics)
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
		spec, specDiags = r.v3SpecFromValues(ctx, values)
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
	res, err := r.data.clientV3.ApplyResource(opCtx, v3RequestResource(ref, values.Name.ValueString(), space, spec), clientv3.Fence{})
	if err != nil {
		// A create the host ACCEPTED can still fail here — most visibly when its
		// long-running Operation outlives create_timeout. Terraform commits the
		// state a failed Create leaves behind, so returning without writing state
		// orphans a resource the host owns and the next plan creates a duplicate.
		// Record the identity that is known before surfacing the failure.
		r.writeV3AcceptedState(ctx, &resp.State, ref, space, values, err, &resp.Diagnostics)
		resp.Diagnostics.AddError("Failed to create "+r.form.Kind, err.Error())
		return
	}
	resp.Diagnostics.Append(r.writeV3State(ctx, &resp.State, ref, space, values, res, false)...)
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
	ref, ok := r.v3StateRef(values.Identity, &resp.Diagnostics)
	if !ok {
		return
	}
	space, ok := v3EffectiveSpace(values.Space, r.data.defaultSpace, &resp.Diagnostics)
	if !ok {
		return
	}
	res, err := r.data.clientV3.GetResource(ctx, space, clientFormRef(ref), values.Name.ValueString())
	if err != nil {
		if errors.Is(err, clientv3.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read "+r.form.Kind, err.Error())
		return
	}
	if !values.UID.IsNull() && !values.UID.IsUnknown() && values.UID.ValueString() != res.Metadata.UID {
		// A same-named resource with a different UID is a different resource
		// (delete + recreate happened out of band). Removing state makes the
		// replacement visible in the next plan instead of silently rebinding.
		resp.Diagnostics.AddWarning(
			r.form.Kind+" was replaced out of band",
			fmt.Sprintf(
				"State records uid %s but the host now serves uid %s for %s/%s. The old resource no longer exists; the next plan will propose recreating it.",
				values.UID.ValueString(), res.Metadata.UID, space, values.Name.ValueString(),
			),
		)
		resp.State.RemoveResource(ctx)
		return
	}
	v3ReportRelationCondition(
		r.form.Kind, space, values.Name.ValueString(), res, r.form.DeclaresUpdate(), &resp.Diagnostics,
	)
	// Only a Form that declares update can converge an out-of-band change in
	// place, so only there does adopting the host's spec produce a plan the
	// next apply can satisfy. For a Form without update every desired attribute
	// forces replacement, and the recorded configuration is preserved.
	resp.Diagnostics.Append(r.writeV3State(ctx, &resp.State, ref, space, values, res, r.form.DeclaresUpdate())...)
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
	ref, ok := r.v3StateRef(stateValues.Identity, &resp.Diagnostics)
	if !ok {
		return
	}
	space, ok := v3EffectiveSpace(values.Space, r.data.defaultSpace, &resp.Diagnostics)
	if !ok {
		return
	}
	spec, specDiags := r.v3SpecFromValues(ctx, values)
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
	res, err := r.data.clientV3.ApplyResource(opCtx, v3RequestResource(ref, values.Name.ValueString(), space, spec), fence)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update "+r.form.Kind, err.Error())
		return
	}
	resp.Diagnostics.Append(r.writeV3State(ctx, &resp.State, ref, space, values, res, false)...)
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
	if !r.v3DesiredSpecUnchanged(ctx, planValues, stateValues, &resp.Diagnostics) {
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
func (r *v3FormResource) v3DesiredSpecUnchanged(ctx context.Context, plan, state v3Values, diags *diag.Diagnostics) bool {
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
	plannedSpec, planDiags := r.v3SpecFromValues(ctx, plan)
	diags.Append(planDiags...)
	stateSpec, stateDiags := r.v3SpecFromValues(ctx, state)
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
	ref, ok := r.v3StateRef(values.Identity, &resp.Diagnostics)
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
	err := r.data.clientV3.DeleteResource(opCtx, space, clientFormRef(ref), values.Name.ValueString(), values.Revision.ValueString())
	if err != nil && !errors.Is(err, clientv3.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete "+r.form.Kind, err.Error())
	}
}

func (r *v3FormResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !r.assertV3Configured(&resp.Diagnostics) {
		return
	}
	ref, ok := r.v3DefaultRef(&resp.Diagnostics)
	if !ok {
		return
	}
	space, name, err := splitImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(name))...)
	if space != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space"), types.StringValue(space))...)
	}
	resp.Diagnostics.Append(setV3FormIdentityState(ctx, &resp.State, ref)...)
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
