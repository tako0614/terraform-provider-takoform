package provider

// v3_lifecycle.go is the shared lifecycle core of the Host API v1beta1
// resource lane (spec/decisions/0013): every typed Edge Family resource
// drives the same create/read/update/delete/import flow over the v1beta1
// client.
// State identity is space/apiVersion/kind/uid (spec/decisions/0011); desired
// mutations fence on the expected generation, deletes fence on the revision,
// and there is no global mutation mutex.

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// Default per-resource operation timeouts of the v3 lane. Each resource may
// override them through the optional create_timeout/update_timeout/
// delete_timeout attributes (time.ParseDuration strings such as "20m").
const (
	v3DefaultCreateTimeout = 20 * time.Minute
	v3DefaultUpdateTimeout = 20 * time.Minute
	v3DefaultDeleteTimeout = 30 * time.Minute
)

// v3FormResource implements one typed current-family Form over the shared
// stable-v1 lifecycle core. Its authoring model and exact identity come from
// the Provider-owned embedded projection.
type v3FormResource struct {
	form         model.Form
	resourceType string
	artifact     *v3ArtifactProjection
	data         *providerData
	// providerSurface selects the provider-only authoring additions carried by
	// this in-process resource. Production registration uses the current
	// surface. Historical golden tests construct the explicit v3.0.0 surface so
	// release evidence remains a projection of that immutable source rather
	// than a comparison against today's additive schema.
	providerSurface v3ProviderSurface
	// codecs is the exact-FormRef dispatch table: the registry of identities
	// this resource can serve together with the per-ref codec that decodes
	// state written under each one. Production constructions carry the build's
	// own table (v3Codecs); it is a real dependency, not an override, so the
	// same code path serves one definition version and several.
	codecs *v3CodecTable
}

type v3ProviderSurface uint8

const (
	v3ProviderSurfaceCurrent v3ProviderSurface = iota
	v3ProviderSurfaceV30
)

func (r *v3FormResource) supportsApplyIdempotencyKey() bool {
	return r.providerSurface == v3ProviderSurfaceCurrent && r.form.Kind == workerVersionKind
}

var (
	_ resource.Resource                = (*v3FormResource)(nil)
	_ resource.ResourceWithImportState = (*v3FormResource)(nil)
	_ resource.ResourceWithConfigure   = (*v3FormResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*v3FormResource)(nil)
)

// NewV3FormResource returns a constructor for one declared family Form.
func NewV3FormResource(form model.Form) func() resource.Resource {
	assembly := mustProviderV3SnapshotAssembly()
	factories, err := compileV3FormResources(
		[]model.Form{form}, assembly.registry, assembly.resourceTypes, assembly.codecs,
	)
	if err != nil {
		panic(err)
	}
	return factories[0]
}

// newV3FormResources lists every v3-lane resource constructor: exactly the
// typed family members, one per catalog Form.
//
// There is deliberately no generic exact-FormRef carrier. A resource that
// accepts an arbitrary publisher FormRef and an opaque JSON spec can back
// none of what an exact reference promises: the v1beta1 Form Definition
// response is a closed envelope carrying only identity, display name,
// description, and desiredSchema, so a client can neither recompute the
// canonical definition digest the FormRef pins nor read the Form's role.
// Shipping the carrier anyway would have offered reach with no verification
// behind it (spec/decisions/0021).
func newV3FormResources() []func() resource.Resource {
	assembly := mustProviderV3SnapshotAssembly()
	out, err := compileV3FormResources(
		assembly.currentForms, assembly.registry, assembly.resourceTypes, assembly.codecs,
	)
	if err != nil {
		panic(err)
	}
	return out
}

func (r *v3FormResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = r.resourceTypeName()
}

func (r *v3FormResource) resourceTypeName() string {
	if r.resourceType != "" {
		return r.resourceType
	}
	assembly, err := providerV3SnapshotAssembly()
	if err != nil {
		return ""
	}
	ref, err := assembly.registry.DefaultCreate(v3GroupKind{
		APIVersion: r.form.Family.APIVersion(), Kind: r.form.Kind,
	})
	if err != nil {
		return ""
	}
	resourceType, _ := assembly.resourceTypes.Lookup(ref.ExactKey())
	return resourceType
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

// assertV3Configured requires the v1beta1 lane. When the endpoint only
// negotiated v1alpha2, the recorded per-lane negotiation error is the
// diagnostic, so the user sees why this resource cannot work while the v2
// resources can.
func (r *v3FormResource) assertV3Configured(diags *diag.Diagnostics) bool {
	return assertV3Lane(r.data, r.resourceTypeName(), diags)
}

func assertV3Lane(data *providerData, resourceType string, diags *diag.Diagnostics) bool {
	if data == nil {
		diags.Append(v3Diagnostic{
			Summary:      "Provider not configured",
			ResourceType: resourceType,
			Code:         v3CodeNotConfigured,
			Detail:       "The takoform provider was not configured before use.",
			Repair: "This is a provider bug rather than a configuration fault. Report it with the resource type " +
				"above and the CLI version.",
		}.error())
		return false
	}
	if data.clientV3 == nil {
		// The lane label is READ from the client rather than written here: a
		// hardcoded one drifts the moment the lane moves, and it drifts in the
		// one place an operator is already confused.
		diags.Append(v3LaneDiagnostic(resourceType, clientv3.APIVersion, data.v3Err))
		return false
	}
	return true
}

// v3LaneDiagnostic is the one shape a resource or data source uses when the
// lane it needs did not negotiate. The recorded per-lane error is the whole
// point of the diagnostic: the provider configured successfully — the OTHER
// lane answered — so "not configured" would be false, and the endpoint's own
// reason is the only thing that tells the reader whether to change the endpoint,
// the credential, or the resource.
func v3LaneDiagnostic(resourceType, lane string, laneErr error) diag.Diagnostic {
	return v3Diagnostic{
		Summary:      "Takoform " + lane + " lane unavailable",
		ResourceType: resourceType,
		Code:         v3CodeLaneUnavailable,
		Cause:        laneErr,
		Detail: "The configured endpoint did not negotiate the Host API " + lane + " lane required by " +
			resourceType + ". The provider itself configured normally: the other lane negotiated, so resources " +
			"on that lane keep working.",
		Repair: "Point `endpoint` at a host that serves the " + lane + " lane, or remove the " + resourceType +
			" entries from this configuration.",
	}.error()
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
	codec, err := r.codecTable().defaultCreate(v3GroupKind{
		APIVersion: r.form.Family.APIVersion(), Kind: r.form.Kind,
	})
	if err != nil {
		diags.Append(v3Diagnostic{
			Summary:      r.form.Kind + " FormRef missing",
			ResourceType: r.resourceTypeName(),
			Pointer:      "/form",
			Code:         v3CodeProviderBug,
			Cause:        err,
			Detail:       "This provider build carries no exact create target for " + r.form.Kind + ".",
			Repair:       "This is a provider bug. Pin a provider build whose registry carries this Form kind.",
		}.error())
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
		diags.Append(v3Diagnostic{
			Summary:      "State has no exact v1beta1 Form identity",
			ResourceType: r.resourceTypeName(),
			Pointer:      "/form",
			Code:         v3CodeStateRefMissing,
			Detail: "The v1beta1 resource lane fails closed on state without a complete exact FormRef, because " +
				"every read, update, and delete addresses the resource under the identity it was applied under.",
			Repair: "Retained v2-lane state is never transformed in place. Perform an explicit create or import " +
				"migration onto this lane.",
		}.error())
		return v3FormCodec{}, false
	}
	table := r.codecTable()
	if codec, supported := table.forStateKey(got.exactKey()); supported {
		return codec, true
	}
	diags.Append(v3UnsupportedStateRefError(r.form.Kind, got, table.knownRefsForKind(r.form.Kind)))
	return v3FormCodec{}, false
}

func clientFormRef(ref v3FormRef) clientv3.FormRef {
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
func v3RequestResource(ref v3FormRef, name, space string, spec map[string]any) *clientv3.Resource {
	return &clientv3.Resource{
		APIVersion: ref.APIVersion,
		Kind:       ref.Kind,
		Form:       &clientv3.FormReference{FormRef: clientFormRef(ref)},
		Metadata:   clientv3.Metadata{Name: name, Space: space},
		Spec:       spec,
	}
}

// v3ApplyOptions carries the one provider-only apply input. WorkerVersion is
// the sole resource that can set it; every other Form uses the existing client
// fallback. Unknown values are refused before any artifact upload or host
// mutation so a plan cannot accidentally fall back to a different operation
// identity.
func (r *v3FormResource) v3ApplyOptions(values v3Values, diags *diag.Diagnostics) (clientv3.ApplyResourceOptions, bool) {
	if !r.supportsApplyIdempotencyKey() || values.ApplyIdempotencyKey.IsNull() {
		return clientv3.ApplyResourceOptions{}, true
	}
	if values.ApplyIdempotencyKey.IsUnknown() {
		diags.AddAttributeError(
			path.Root(v3ApplyIdempotencyKeyAttribute),
			"Unknown WorkerVersion apply idempotency key",
			"The provider-only `apply_idempotency_key` must be known before apply so retries and recovery reuse the same Host operation identity.",
		)
		return clientv3.ApplyResourceOptions{}, false
	}
	key := values.ApplyIdempotencyKey.ValueString()
	if err := clientv3.ValidateIdempotencyKey(key); err != nil {
		diags.AddAttributeError(
			path.Root(v3ApplyIdempotencyKeyAttribute),
			"Invalid WorkerVersion apply idempotency key",
			err.Error(),
		)
		return clientv3.ApplyResourceOptions{}, false
	}
	return clientv3.ApplyResourceOptions{IdempotencyKey: key}, true
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
	var fileBundle v3FileBundleAuthoring
	if _, workerBundle := r.v3WorkerBundleArtifact(); workerBundle {
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
	} else if _, fileArtifact := r.v3FileBundleArtifact(); fileArtifact {
		resolved, artifactDiags := r.fileBundleAuthoring(&values)
		resp.Diagnostics.Append(artifactDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		fileBundle = resolved
		spec = fileBundle.Spec()
	} else {
		var specDiags diag.Diagnostics
		spec, specDiags = r.v3SpecFromValues(ctx, codec, values)
		resp.Diagnostics.Append(specDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	runtimeInputs, ok := r.v3RuntimeInputsForApply(values, codec.Ref, space, spec, &resp.Diagnostics)
	if !ok {
		return
	}
	if runtimeInputs != nil {
		defer runtimeInputs.release()
	}
	applyOptions, ok := r.v3ApplyOptions(values, &resp.Diagnostics)
	if !ok {
		return
	}
	// An immutable revision is named by its content, and the content is only
	// wholly resolved here. A plan that already derived the name derives the
	// same one from the same spec, so state and plan cannot disagree.
	if !r.v3EnsureRevisionName(&values, spec, &resp.Diagnostics) {
		return
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
	if fileBundle.Local {
		committed, ok := r.uploadFileBundle(opCtx, fileBundle, &resp.Diagnostics)
		if !ok {
			return
		}
		spec = map[string]any{"manifestDigest": committed}
		values.Fields["manifest_digest"] = types.StringValue(committed)
	}
	requestResource := v3RequestResource(codec.Ref, values.Name.ValueString(), space, spec)
	var res *clientv3.Resource
	var err error
	if runtimeInputs != nil {
		res, err = r.data.clientV3.ApplyResourceWithRuntimeInputs(
			opCtx,
			requestResource,
			clientv3.Fence{},
			applyOptions.IdempotencyKey,
			runtimeInputs.CanonicalPublicOrigin,
			runtimeInputs.Bindings,
		)
	} else {
		res, err = r.data.clientV3.ApplyResourceWithOptions(
			opCtx,
			requestResource,
			clientv3.Fence{},
			applyOptions,
		)
	}
	if err != nil {
		// A create the host ACCEPTED can still fail here — most visibly when its
		// long-running Operation outlives create_timeout. Terraform commits the
		// state a failed Create leaves behind, so returning without writing state
		// orphans a resource the host owns and the next plan creates a duplicate.
		// Record the identity that is known before surfacing the failure.
		r.writeV3AcceptedState(ctx, &resp.State, codec, space, values, err, &resp.Diagnostics)
		resp.Diagnostics.Append(v3HostCallDiagnostic("Failed to create "+r.form.Kind, err, v3Diagnostic{
			ResourceType: r.resourceTypeName(),
			Space:        space,
			Name:         values.Name.ValueString(),
			Ref:          codec.Ref,
			Pointer:      "/spec",
		}))
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
		resp.Diagnostics.Append(v3HostCallDiagnostic("Failed to read "+r.form.Kind, err, v3Diagnostic{
			ResourceType: r.resourceTypeName(),
			Space:        space,
			Name:         values.Name.ValueString(),
			Ref:          codec.Ref,
			Pointer:      "/metadata",
			ExpectedUID:  v3StateStringValue(values.UID),
			OperationID:  v3StateStringValue(values.PendingOperationID),
		}))
		return
	}
	if !v3RequireStateUID(r.form.Kind, space, values.Name.ValueString(), v3StateStringValue(values.UID), res, &resp.Diagnostics) {
		// State is DELIBERATELY kept: the resource is still under management and
		// the operator must choose which incarnation it names.
		return
	}
	v3ReportRelationCondition(
		r.form.Kind, r.resourceTypeName(), space, values.Name.ValueString(), codec.Ref,
		res, r.form.DeclaresUpdate(), &resp.Diagnostics,
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
	kind, resourceType, space, name string,
	ref v3FormRef,
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
	pointer, expectedUID, currentUID := v3ParseRelationHostReason(hostReason)
	diags.Append(v3Diagnostic{
		Summary:      kind + " references a resource that changed out of band",
		ResourceType: resourceType,
		Space:        space,
		Name:         name,
		Ref:          ref,
		Pointer:      pointer,
		ExpectedUID:  expectedUID,
		CurrentUID:   currentUID,
		Code:         v3CodeRelationTargetChanged,
		Host:         &v3HostFault{HostCode: hostReason},
		Detail: fmt.Sprintf(
			"The host reports this resource as not ready with the portable reason %s: %s. It stays pinned to "+
				"the incarnation it was applied against, because the host never re-binds a reference by name.",
			reason, summary,
		),
		Repair: "The next plan proposes " + remedy +
			", and that apply re-resolves the reference against the resource that exists now." +
			" A target that no longer exists must be re-created first, or this resource removed with it.",
	}.warning())
}

// v3ParseRelationHostReason lifts the relation pointer and the two uids out of
// the host's free-form `hostReason`.
//
// The portable condition carries only the closed reason; WHICH relation moved
// and from which incarnation is host-specific detail, so the lane puts it in
// `hostReason` rather than inventing structure for it. A host that writes the
// lane's own phrasing gets those three facts promoted into the diagnostic's
// identity block; any other spelling simply leaves them out, and the raw
// hostReason is rendered either way, so nothing the host said is ever lost.
func v3ParseRelationHostReason(hostReason string) (pointer, expectedUID, currentUID string) {
	rest, found := strings.CutPrefix(hostReason, "relation ")
	if !found {
		return "", "", ""
	}
	pointer, _, _ = strings.Cut(rest, " ")
	if !strings.HasPrefix(pointer, "/") {
		return "", "", ""
	}
	switch {
	case strings.Contains(rest, " changed incarnation from uid "):
		_, after, _ := strings.Cut(rest, " changed incarnation from uid ")
		expectedUID, after, _ = strings.Cut(after, " ")
		if _, to, ok := strings.Cut(after, " to uid "); ok {
			currentUID, _, _ = strings.Cut(to, " ")
		}
	case strings.Contains(rest, " no longer exists"):
		if _, after, ok := strings.Cut(rest, " uid "); ok {
			expectedUID, _, _ = strings.Cut(after, " ")
		}
	}
	return pointer, expectedUID, currentUID
}

// ModifyPlan carries the plan-time facts of the v3 lane, in the one order they
// depend on each other:
//
//  1. a relation the host reports as broken is planned into an apply that can
//     repair it;
//  2. a worker bundle's identity follows its bytes, so the manifest digest is
//     resolved before anything reads it;
//  3. an immutable revision's host name is derived from that resolved content;
//  4. a replacement that would still land on the recorded name is refused,
//     because neither apply order can complete it;
//  5. what the host declares it supports is decided HERE rather than at apply.
func (r *v3FormResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	v3PlanRelationRecovery(ctx, req.State, r.form.DeclaresUpdate(), resp)
	if _, workerBundle := r.v3WorkerBundleArtifact(); workerBundle {
		r.modifyWorkerBundlePlan(ctx, req, resp)
	} else if _, fileArtifact := r.v3FileBundleArtifact(); fileArtifact {
		r.modifyFileBundlePlan(ctx, req, resp)
	}
	r.v3PlanRuntimeInputs(ctx, req, resp)
	r.v3PlanRevisionName(ctx, req, resp)
	r.v3PlanApplyIdempotencyKeySafety(ctx, req, resp)
	r.v3PlanImmutableRevisionSafety(ctx, req, resp)
	r.v3PlanHostSupport(ctx, resp)
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
	applyOptions, ok := r.v3ApplyOptions(values, &resp.Diagnostics)
	if !ok {
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
	res, err := r.data.clientV3.ApplyResourceWithOptions(
		opCtx,
		v3RequestResource(codec.Ref, values.Name.ValueString(), space, spec),
		fence,
		applyOptions,
	)
	if err != nil {
		resp.Diagnostics.Append(v3HostCallDiagnostic("Failed to update "+r.form.Kind, err, v3Diagnostic{
			ResourceType:       r.resourceTypeName(),
			Space:              space,
			Name:               values.Name.ValueString(),
			Ref:                codec.Ref,
			Pointer:            "/spec",
			ExpectedUID:        fence.ExpectedUID,
			ExpectedGeneration: fence.ExpectedGeneration,
			ExpectedRevision:   v3StateStringValue(stateValues.Revision),
		}))
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
	if _, workerBundle := r.v3WorkerBundleArtifact(); workerBundle {
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
	} else if _, fileArtifact := r.v3FileBundleArtifact(); fileArtifact {
		files, ok := planValues.Fields["files"].(types.List)
		if !ok || files.IsUnknown() {
			files = types.ListNull(v3ArtifactFileType())
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("files"), files)...)
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
	if _, workerBundle := r.v3WorkerBundleArtifact(); workerBundle {
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
	if _, fileArtifact := r.v3FileBundleArtifact(); fileArtifact {
		planned, plannedDiags := r.fileBundleAuthoring(&plan)
		diags.Append(plannedDiags...)
		if diags.HasError() {
			return false
		}
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
	// The delete fence is the desired GENERATION, not the revision state
	// happens to hold. A destroy removes dependents first, and removing one
	// re-renders whatever was rendered from it — so by the time this call is
	// made, the revision the plan read is routinely stale through this very
	// destroy (spec/decisions/0011, 0016 rule 9). The generation is not: it
	// moves only when some client changes desired state.
	//
	// The recorded uid travels with it. It is not a second fence — it names the
	// incarnation in the delete's Idempotency-Key, so a host still holding the
	// record of an earlier delete of this NAME cannot answer this one with it
	// (clientv3.incarnationKey). State always has one here: uid and generation
	// are written from the same verified representation.
	err := r.data.clientV3.DeleteResource(
		opCtx, space, clientFormRef(codec.Ref), values.Name.ValueString(),
		v3StateStringValue(values.UID), values.Generation.ValueString(),
	)
	if err != nil && !errors.Is(err, clientv3.ErrNotFound) {
		resp.Diagnostics.Append(v3HostCallDiagnostic("Failed to delete "+r.form.Kind, err, v3Diagnostic{
			ResourceType:       r.resourceTypeName(),
			Space:              space,
			Name:               values.Name.ValueString(),
			Ref:                codec.Ref,
			Pointer:            "/metadata",
			ExpectedUID:        v3StateStringValue(values.UID),
			ExpectedGeneration: values.Generation.ValueString(),
		}))
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
		resp.Diagnostics.Append(v3Diagnostic{
			Summary:      "Invalid import ID",
			ResourceType: r.resourceTypeName(),
			Pointer:      "/metadata/name",
			Code:         v3CodeImportIDInvalid,
			Cause:        err,
			Repair: "Import either the short `NAME` or `SPACE/NAME` form, which resolves to this build's default " +
				"create ref, or the canonical JSON object " +
				`{"space","apiVersion","kind","definitionVersion","schemaDigest","name"}`,
		}.error())
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
