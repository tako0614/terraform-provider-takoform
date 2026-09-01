package provider

// v3_revision_names.go makes an immutable revision safe to author.
//
// A `revision`-role Form (spec/decisions/0009) is immutable by role: the host
// refuses every update to one, so any desired change is a REPLACEMENT. Under a
// name the author pins, that replacement cannot complete in either order. The
// destroy-then-create order fails on the destroy with `dependency_in_use`
// (409), because the Worker Version that executes the bundle — or the Worker
// Deployment that weights the version — still holds a live relation to it. The
// create-before-destroy order fails on the create with `invalid_argument`
// (400), because the name is still occupied and a create carries no generation
// fence. Neither order mutates anything, and no plan repairs it: the apply
// simply cannot proceed.
//
// The way out is not a better order, it is a different NAME. A revision is
// identified by its content, so its host name is derived from that content:
// changed bytes are a new name, the new revision is created beside the old one,
// the deployment moves onto it, and only then is the old one removable. That is
// the whole of the fix, and it is why `name` on a revision Form is
// Optional+Computed rather than Required.
//
// Framework contract, which this file is careful about because getting it wrong
// produces either "Provider produced inconsistent result after apply" or a
// perpetual diff:
//
//   - Terraform proposes the PRIOR state value for an Optional+Computed
//     attribute whose configuration is null. Leaving that proposal alone would
//     keep the old name across a content change — exactly the deadlock. So the
//     plan overwrites it, and adds `name` to RequiresReplace itself, because
//     attribute-level plan modifiers have already run by the time ModifyPlan
//     changes the value.
//   - Whatever the plan makes KNOWN, apply must write byte-identically. Both
//     sides derive the name from the same content through the same function
//     (v3DerivedRevisionName), so they cannot disagree.
//   - Where the plan cannot derive the name — a content_file that does not
//     exist yet, or a reference that is still unknown — the planned value stays
//     unknown on a create and untouched on a replacement. Unknown is honest and
//     apply fills it; untouched keeps the replacement visible to the safety
//     check below, which is the diagnostic the author needs.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// v3RevisionNameDigestLength is how much of the content digest the derived name
// carries. Twelve hex characters is 48 bits: long enough that two revisions of
// one worker never collide in practice, short enough that the name stays
// readable in a plan, a URL, and an error message. A collision is not silent —
// two different contents deriving one name would fail on the host's own
// `If-None-Match: *` create fence rather than overwriting anything.
const v3RevisionNameDigestLength = 12

// v3RevisionOwnerDigestLength is how much of the owner digest the derived name
// carries, and it is the same width for the same reasons.
//
// The owner travels as a DIGEST rather than as a literal prefix so the derived
// name has a fixed width. A literal owner would make the longest legal owner
// name derive a revision name past the 63-character portable grammar, and the
// only way to keep that inside the grammar would be to refuse owner names the
// provider itself accepts — which is precisely the module-narrows-the-provider
// defect this same review found elsewhere.
const v3RevisionOwnerDigestLength = 12

// v3RevisionOwnerAttribute is the attribute naming who owns a derived revision.
const v3RevisionOwnerAttribute = "revision_owner"

// derivesRevisionName reports whether this Form's host name is a function of
// its content. It is exactly the revision role: an identity Form's name is the
// author's chosen, stable handle, a deployment's and an attachment's names are
// the author's too, and only a revision is a thing whose whole identity IS what
// it contains.
func (r *v3FormResource) derivesRevisionName() bool { return r.form.Role == model.RoleRevision }

// v3RevisionNamePrefix is the human half of a derived revision name. It is read
// from the Form's own slug with the family's `worker-` qualifier removed, so
// `worker-bundle` derives `bundle-<digest>` and `worker-version` derives
// `version-<digest>` without either string being written twice.
func v3RevisionNamePrefix(form model.Form) string {
	prefix := strings.TrimPrefix(form.Slug, "worker-")
	if prefix == "" {
		return form.Slug
	}
	return prefix
}

// v3DerivedRevisionName renders one derived name from a content digest and the
// owner that declares this revision.
//
// The digest is the canonical `sha256:<hex>` spelling every content address in
// this lane uses; anything else is refused rather than truncated into a name
// that looks derived and is not. The owner is folded in as a digest of its own,
// so a name states BOTH facts a Terraform address needs: which bytes this
// revision is, and whose revision it is. Changing the bytes still changes the
// name — the content half is untouched — and two owners of byte-identical
// output no longer land on one host address.
func v3DerivedRevisionName(prefix, owner, digest string) (string, bool) {
	hexDigest, found := strings.CutPrefix(digest, "sha256:")
	if !found || len(hexDigest) < v3RevisionNameDigestLength || owner == "" {
		return "", false
	}
	ownerSum := sha256.Sum256([]byte(owner))
	ownerHex := hex.EncodeToString(ownerSum[:])
	return prefix + "-" +
		hexDigest[:v3RevisionNameDigestLength] + "-" +
		ownerHex[:v3RevisionOwnerDigestLength], true
}

// v3ContentDigest is the content address one revision's name is derived from.
//
// An artifact-backed revision's whole desired state is already a content
// address — the manifest digest — so the name is derived straight from it: the
// same manifest always produces the same revision name for one owner. Every
// other revision Form is named by the canonical digest of its desired spec.
func v3ContentDigest(artifactBacked bool, spec map[string]any) (string, bool) {
	if artifactBacked {
		digest, ok := spec["manifestDigest"].(string)
		return digest, ok && digest != ""
	}
	canonical, err := json.Marshal(spec)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), true
}

// v3ContentDigestWithApplyKey extends the provider-side identity of a
// WorkerVersion revision when an explicit apply idempotency key is present.
// The key represents sealed runtime material that is not part of the portable
// desired spec, so it is bound only into this derived host NAME. Keeping the
// empty-key path on v3ContentDigest is important: existing configurations with
// the option unset retain byte-identical derived names.
func v3ContentDigestWithApplyKey(artifactBacked bool, spec map[string]any, applyKey string) (string, bool) {
	digest, ok := v3ContentDigest(artifactBacked, spec)
	if !ok || applyKey == "" {
		return digest, ok
	}
	identity := struct {
		ContentDigest    string `json:"contentDigest"`
		ApplyIdempotency string `json:"applyIdempotencyKey"`
	}{
		ContentDigest:    digest,
		ApplyIdempotency: applyKey,
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), true
}

// v3RevisionNameFromSpec derives the host name of one revision from its
// resolved desired spec and its declared owner. The optional apply key is a
// provider-only WorkerVersion identity input; a missing key preserves the
// historical derivation exactly.
func (r *v3FormResource) v3RevisionNameFromSpec(spec map[string]any, owner string, applyKeys ...string) (string, bool) {
	if v3FormRequiresArtifactRule(r.form) && r.artifact == nil {
		return "", false
	}
	applyKey := ""
	if r.supportsApplyIdempotencyKey() && len(applyKeys) > 0 {
		applyKey = applyKeys[0]
	}
	digest, ok := v3ContentDigestWithApplyKey(r.v3ArtifactBackedRevision(), spec, applyKey)
	if !ok {
		return "", false
	}
	return v3DerivedRevisionName(v3RevisionNamePrefix(r.form), owner, digest)
}

// v3EnsureRevisionName supplies the derived name at APPLY time for a revision
// whose plan left it unknown. It is the same derivation the plan runs, over the
// same spec, so a plan that did resolve the name gets the identical value back.
func (r *v3FormResource) v3EnsureRevisionName(values *v3Values, spec map[string]any, diags *diag.Diagnostics) bool {
	if _, known := v3PlanKnownString(values.Name); known {
		return true
	}
	if !r.derivesRevisionName() {
		diags.Append(v3Diagnostic{
			Summary:      "Unknown " + r.form.Kind + " name",
			ResourceType: r.resourceTypeName(),
			Pointer:      "/metadata/name",
			Code:         v3CodeNameUnresolved,
			Repair:       "Set `name` to a value that is wholly known before this apply runs.",
		}.error())
		return false
	}
	applyKey := ""
	if r.supportsApplyIdempotencyKey() {
		if values.ApplyIdempotencyKey.IsUnknown() {
			diags.AddAttributeError(
				path.Root(v3ApplyIdempotencyKeyAttribute),
				"Unknown WorkerVersion apply idempotency key",
				"The provider-only `apply_idempotency_key` must be known before an immutable WorkerVersion revision name can be derived.",
			)
			return false
		}
		applyKey = v3StateStringValue(values.ApplyIdempotencyKey)
	}
	owner, owned := v3PlanKnownString(values.RevisionOwner)
	if !owned {
		diags.Append(r.v3RevisionOwnerMissing())
		return false
	}
	derived, ok := r.v3RevisionNameFromSpec(spec, owner, applyKey)
	if !ok {
		diags.Append(v3Diagnostic{
			Summary:      "Cannot derive the " + r.form.Kind + " revision name",
			ResourceType: r.resourceTypeName(),
			Pointer:      "/metadata/name",
			Code:         v3CodeNameUnresolved,
			Repair: "This revision's content did not resolve to a content digest, so no name follows from it. " +
				"Set `name` explicitly on this resource, remembering that an immutable revision must not " +
				"reuse a name a live revision still holds.",
		}.error())
		return false
	}
	values.Name = types.StringValue(derived)
	return true
}

// v3RevisionOwnerMissing is the refusal to derive a name that states only the
// bytes.
//
// A content digest names the BYTES. It is a name, not an ownership claim, and
// two configurations built from identical output hold identical bytes — so a
// name derived from content alone hands one host address to two Terraform
// resources. Terraform requires exactly one owner per address, and no owner is
// recoverable from the content, from the space, or from the resource address
// (the framework never shows a provider its own address). It therefore has to
// be DECLARED, and the provider refuses to derive a name without it rather than
// minting one that two owners can both reach.
func (r *v3FormResource) v3RevisionOwnerMissing() diag.Diagnostic {
	return v3Diagnostic{
		Summary:      "Deriving this " + r.form.Kind + " name needs to know whose revision it is.",
		ResourceType: r.resourceTypeName(),
		Pointer:      "/metadata/name",
		Code:         v3CodeRevisionOwnerMissing,
		Detail: "A derived revision name is a function of this revision's CONTENT, and two independent " +
			"resources built from identical content derive identical names. A Terraform address has exactly " +
			"one owner, so the derivation also needs the owner, and nothing in the content says who that is.",
		Repair: "Set `" + v3RevisionOwnerAttribute + "` to the stable name of whatever owns this revision — " +
			"the `takoform_module_worker` it belongs to is the usual answer — or pin `name` yourself. The " +
			"official `worker-app` module sets it for you.",
	}.error()
}

// v3PlanRevisionName writes the derived name into the plan.
//
// It runs after the bundle's own plan step, so a locally authored bundle's
// manifest digest is already resolved in the plan when the name is derived from
// it, and after the version's spec can be projected from wholly known planned
// values.
func (r *v3FormResource) v3PlanRevisionName(
	ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse,
) {
	if !r.derivesRevisionName() || resp.Plan.Raw.IsNull() {
		return
	}
	// A configured name belongs to the author. The provider does not overwrite
	// it, and the safety check below is what protects it.
	var configured, configuredOwner types.String
	if req.Config.Schema != nil && !req.Config.Raw.IsNull() {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &configured)...)
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(v3RevisionOwnerAttribute), &configuredOwner)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if !configured.IsNull() {
		// A pinned name settles the whole question, so an owner beside it decides
		// nothing. Accepting both silently would leave an author believing the
		// owner separates two revisions when the pinned name is what collides.
		if !configuredOwner.IsNull() {
			resp.Diagnostics.Append(v3Diagnostic{
				Summary:      "A pinned " + r.form.Kind + " name leaves `" + v3RevisionOwnerAttribute + "` with nothing to decide.",
				ResourceType: r.resourceTypeName(),
				Name:         configured.ValueString(),
				Pointer:      "/metadata/name",
				Code:         v3CodeRevisionOwnerIgnored,
				Detail: "`" + v3RevisionOwnerAttribute + "` distinguishes two owners of identical content when the " +
					"provider DERIVES the name. This resource pins `name`, so the derivation never runs and the " +
					"owner has no effect on anything.",
				Repair: "Remove `" + v3RevisionOwnerAttribute + "`, or remove `name` and let the provider derive it.",
			}.error())
		}
		return
	}
	if configuredOwner.IsNull() {
		resp.Diagnostics.Append(r.v3RevisionOwnerMissing())
		return
	}
	var plannedApplyKey types.String
	if r.supportsApplyIdempotencyKey() {
		resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), &plannedApplyKey)...)
		if resp.Diagnostics.HasError() || plannedApplyKey.IsUnknown() {
			return
		}
	}
	spec, resolved := r.v3PlannedSpec(ctx, resp)
	if resp.Diagnostics.HasError() {
		return
	}
	owner, owned := v3PlanKnownString(configuredOwner)
	var derived types.String
	if resolved && owned {
		name, ok := r.v3RevisionNameFromSpec(spec, owner, v3StateStringValue(plannedApplyKey))
		if !ok {
			return
		}
		derived = types.StringValue(name)
	} else if !req.State.Raw.IsNull() {
		if r.supportsApplyIdempotencyKey() {
			var priorApplyKey types.String
			resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), &priorApplyKey)...)
			if resp.Diagnostics.HasError() {
				return
			}
			if !priorApplyKey.Equal(plannedApplyKey) {
				// A fresh, plan-known key necessarily derives a fresh revision
				// identity even when another desired input is not known until apply.
				// Do not carry the consumed prior key's name into Create.
				derived = types.StringUnknown()
			} else {
				return
			}
		} else {
			// A replacement whose content did not resolve keeps the recorded name.
			// Marking it unknown here would propose a replacement on every plan for
			// a resource that may not have changed at all; leaving it alone hands
			// the case to v3PlanImmutableRevisionSafety, which states the problem.
			return
		}
	} else {
		derived = types.StringUnknown()
	}
	var planned types.String
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("name"), &planned)...)
	if resp.Diagnostics.HasError() || planned.Equal(derived) {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("name"), derived)...)
	if resp.Diagnostics.HasError() || req.State.Raw.IsNull() {
		return
	}
	if !resp.RequiresReplace.Contains(path.Root("name")) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("name"))
	}
}

// v3ApplyIdentity is the complete Host apply identity/body projection for a
// WorkerVersion. The provider-only key is deliberately not part of this value:
// callers compare it separately so a changed key can be recognized as the
// supported new-runtime-input path. Every other member is either in the Host
// resource address or in the exact apply body.
type v3ApplyIdentity struct {
	Ref   v3FormRefValue
	Name  string
	Space string
	Spec  map[string]any
}

func (identity v3ApplyIdentity) equal(other v3ApplyIdentity) bool {
	if identity.Ref != other.Ref || identity.Name != other.Name || identity.Space != other.Space {
		return false
	}
	equal, err := v3CanonicalJSONEqual(identity.Spec, other.Spec)
	return err == nil && equal
}

// v3ApplyIdentityFrom projects one existing or planned WorkerVersion onto the
// inputs the provider would send to Host ApplyResource. It returns known=false
// when any identity/body member is unknown or cannot be encoded, allowing the
// caller to fail closed without manufacturing a replacement signal from an
// unrelated timeout or computed output.
func (r *v3FormResource) v3ApplyIdentityFrom(ctx context.Context, getter v3AttributeGetter) (v3ApplyIdentity, bool) {
	values, diags := r.v3ValuesFrom(ctx, getter)
	if diags.HasError() {
		return v3ApplyIdentity{}, false
	}
	ref, complete := values.Identity.formRef()
	if !complete {
		return v3ApplyIdentity{}, false
	}
	codec, supported := r.codecTable().forStateKey(ref.exactKey())
	if !supported {
		return v3ApplyIdentity{}, false
	}
	name, known := v3PlanKnownString(values.Name)
	if !known {
		return v3ApplyIdentity{}, false
	}
	for _, field := range codec.Form.Fields {
		value, carried := values.Fields[v3AttributeName(field)]
		if !carried || value == nil || value.IsUnknown() {
			return v3ApplyIdentity{}, false
		}
	}
	spec, specDiags := r.v3SpecFromValues(ctx, codec, values)
	if specDiags.HasError() {
		return v3ApplyIdentity{}, false
	}
	fallback := ""
	if r.data != nil {
		fallback = r.data.defaultSpace
	}
	space, err := validatedEffectiveSpace(values.Space, fallback)
	if err != nil {
		return v3ApplyIdentity{}, false
	}
	return v3ApplyIdentity{Ref: ref, Name: name, Space: space, Spec: spec}, true
}

func (r *v3FormResource) v3ApplyIdempotencyKeyDiagnostic(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	code, summary, detail, repair string,
) diag.Diagnostic {
	var name, space types.String
	_ = req.State.GetAttribute(ctx, path.Root("name"), &name)
	_ = req.State.GetAttribute(ctx, path.Root("space"), &space)
	return v3Diagnostic{
		Summary:      summary,
		ResourceType: r.resourceTypeName(),
		Space:        space.ValueString(),
		Name:         name.ValueString(),
		Pointer:      "/apply_idempotency_key",
		Code:         code,
		Detail:       detail,
		Repair:       repair,
	}.error()
}

func (r *v3FormResource) v3ConfiguredNameIsDerived(ctx context.Context, req resource.ModifyPlanRequest) bool {
	if req.Config.Schema == nil || req.Config.Raw.IsNull() {
		return false
	}
	var configured types.String
	return !req.Config.GetAttribute(ctx, path.Root("name"), &configured).HasError() && configured.IsNull()
}

func (r *v3FormResource) v3HostAddressFrom(
	ctx context.Context, getter v3AttributeGetter,
) (name string, space string, known bool) {
	var resourceName, resourceSpace types.String
	if getter.GetAttribute(ctx, path.Root("name"), &resourceName).HasError() ||
		getter.GetAttribute(ctx, path.Root("space"), &resourceSpace).HasError() {
		return "", "", false
	}
	name, known = v3PlanKnownString(resourceName)
	if !known {
		return "", "", false
	}
	fallback := ""
	if r.data != nil {
		fallback = r.data.defaultSpace
	}
	space, err := validatedEffectiveSpace(resourceSpace, fallback)
	return name, space, err == nil
}

func (r *v3FormResource) v3ImmutableSameNameDiagnostic(
	ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse,
) diag.Diagnostic {
	var prior types.String
	_ = req.State.GetAttribute(ctx, path.Root("name"), &prior)
	codec, _ := r.v3PlanCodec(ctx, resp)
	return v3Diagnostic{
		Summary:      "This immutable revision cannot be safely replaced under the same host name.",
		ResourceType: r.resourceTypeName(),
		Name:         prior.ValueString(),
		Ref:          codec.Ref,
		Pointer:      "/metadata/name",
		Code:         v3CodeImmutableRevisionSameName,
		Detail: fmt.Sprintf(
			"%s is a %s-role Form, so a host refuses every update to it and this change is a replacement. "+
				"Replacing it under %q completes in neither order: destroy-then-create fails the destroy with "+
				"dependency_in_use (409) while a live relation still holds this revision, and "+
				"create_before_destroy fails the create with invalid_argument (400) because the name is still "+
				"occupied. Nothing is mutated either way, and no later plan repairs it.",
			r.form.Kind, r.form.Role, prior.ValueString(),
		),
		Repair: "Use a new revision name or the official worker-app module.",
	}.error()
}

// v3PlanApplyIdempotencyKeySafety refuses to reuse a non-null WorkerVersion
// operation key for a second apply or recovery. An explicit runtime-input
// reference is provider policy with single-consumption semantics: once that
// key has been used, it cannot be replayed for another immutable revision or
// recovery. Host v1 also binds the request to its resource address and exact
// desired fingerprint; a changed fingerprint with a replayed key is invalid.
// A changed key (including setting or unsetting it) is the supported new
// runtime-input path, and no-op plans remain untouched.
func (r *v3FormResource) v3PlanApplyIdempotencyKeySafety(
	ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse,
) {
	if !r.supportsApplyIdempotencyKey() || req.State.Raw.IsNull() || resp.Plan.Raw.IsNull() {
		return
	}
	var prior, planned types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), &prior)...)
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root(v3ApplyIdempotencyKeyAttribute), &planned)...)
	if resp.Diagnostics.HasError() || prior.IsUnknown() {
		return
	}
	if planned.IsUnknown() {
		detail := "The existing WorkerVersion has a consumed explicit runtime-input reference, but the planned key is unknown. The provider cannot prove whether a replacement or recovery would replay that reference, so it refuses the plan before any Host mutation."
		if prior.IsNull() {
			detail = "The existing legacy or imported WorkerVersion has no recorded apply idempotency key, while the planned key is unknown. The provider cannot prove that the replacement derives a new immutable Host identity or avoids an occupied address, so it refuses the plan before any Host mutation."
		}
		resp.Diagnostics.Append(r.v3ApplyIdempotencyKeyDiagnostic(
			ctx, req,
			v3CodeApplyIdempotencyKeyUnknown,
			"This WorkerVersion plan cannot resolve its apply idempotency key.",
			detail,
			"Make `apply_idempotency_key` wholly known, or change it to a new value before planning the replacement.",
		))
		return
	}
	if prior.IsNull() && planned.IsNull() {
		return
	}
	if planned.IsNull() || !prior.Equal(planned) {
		priorName, priorSpace, priorKnown := r.v3HostAddressFrom(ctx, req.State)
		plannedName, plannedSpace, plannedKnown := r.v3HostAddressFrom(ctx, resp.Plan)
		if !priorKnown || !plannedKnown {
			if r.v3ConfiguredNameIsDerived(ctx, req) {
				var derivedName types.String
				if !resp.Plan.GetAttribute(ctx, path.Root("name"), &derivedName).HasError() && derivedName.IsUnknown() {
					return
				}
			}
			resp.Diagnostics.Append(r.v3ApplyIdempotencyKeyDiagnostic(
				ctx, req,
				v3CodeApplyIdempotencyKeyUnknown,
				"This WorkerVersion plan cannot prove that its replacement has a new Host address.",
				"The apply idempotency key changes, but the configured WorkerVersion name or space is not wholly known. The provider cannot prove that replacement avoids the occupied immutable Host address, so it refuses the plan before any Host mutation.",
				"Make `name` and `space` wholly known and choose a new Host address, or remove `name` and use the provider-derived immutable revision name.",
			))
			return
		}
		if priorName == plannedName && priorSpace == plannedSpace {
			resp.Diagnostics.Append(r.v3ImmutableSameNameDiagnostic(ctx, req, resp))
		}
		return
	}
	var relation types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(v3RelationDriftAttribute), &relation)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if relation.IsUnknown() {
		resp.Diagnostics.Append(r.v3ApplyIdempotencyKeyDiagnostic(
			ctx, req,
			v3CodeApplyIdempotencyKeyUnknown,
			"This WorkerVersion plan cannot prove its relation state.",
			"The existing WorkerVersion has a consumed explicit runtime-input reference, but relation recovery state is unknown. The provider cannot prove that this plan is not a recovery apply that would replay that reference, so it refuses the plan before any Host mutation.",
			"Refresh until relation recovery state is known, or change `apply_idempotency_key` before applying.",
		))
		return
	}
	if !relation.IsNull() {
		resp.Diagnostics.Append(r.v3ApplyIdempotencyKeyDiagnostic(
			ctx, req,
			v3CodeApplyIdempotencyKeyReuse,
			"This WorkerVersion recovery cannot reuse its apply idempotency key.",
			"The existing WorkerVersion carries a consumed explicit runtime-input reference and relation recovery would issue another apply. Provider policy treats that reference as single-consumption and will not replay it for recovery. The provider refuses this plan before any Host mutation.",
			"Change `apply_idempotency_key` so the provider derives a new revision name, or remove it to use a new deterministic operation key.",
		))
		return
	}
	priorIdentity, priorKnown := r.v3ApplyIdentityFrom(ctx, req.State)
	plannedIdentity, plannedKnown := r.v3ApplyIdentityFrom(ctx, resp.Plan)
	if !priorKnown || !plannedKnown {
		resp.Diagnostics.Append(r.v3ApplyIdempotencyKeyDiagnostic(
			ctx, req,
			v3CodeApplyIdempotencyKeyReuse,
			"This WorkerVersion replacement cannot reuse its apply idempotency key.",
			"The existing WorkerVersion has a consumed explicit runtime-input reference, but the provider cannot prove that the planned Host address, exact Form identity, or desired fingerprint is unchanged. It refuses to risk a second apply with that reference before any Host mutation.",
			"Change `apply_idempotency_key` so the provider derives a new revision name, or remove it to use a new deterministic operation key.",
		))
		return
	}
	if !priorIdentity.equal(plannedIdentity) {
		resp.Diagnostics.Append(r.v3ApplyIdempotencyKeyDiagnostic(
			ctx, req,
			v3CodeApplyIdempotencyKeyReuse,
			"This WorkerVersion replacement cannot reuse its apply idempotency key.",
			"The existing WorkerVersion has a consumed explicit runtime-input reference, but this plan changes the Host resource address, exact Form identity, or desired fingerprint. Provider policy treats the explicit reference as single-consumption and will not replay it for the second apply. The provider refuses this plan before any Host mutation.",
			"Change `apply_idempotency_key` so the provider derives a new revision name, or remove it to use a new deterministic operation key.",
		))
	}
}

// v3PlannedSpec projects the planned desired spec of one resource, reporting
// whether every value it needs was wholly known. It never adds a diagnostic for
// an unknown value: a plan legitimately carries unknowns, and the caller decides
// what that means.
func (r *v3FormResource) v3PlannedSpec(
	ctx context.Context, resp *resource.ModifyPlanResponse,
) (map[string]any, bool) {
	values, diags := r.v3ValuesFrom(ctx, resp.Plan)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return nil, false
	}
	if r.v3ArtifactBackedRevision() {
		digest, known := v3PlanKnownString(values.Fields["manifest_digest"])
		if !known {
			return nil, false
		}
		return map[string]any{"manifestDigest": digest}, true
	}
	codec, ok := r.v3PlanCodec(ctx, resp)
	if !ok {
		return nil, false
	}
	spec := map[string]any{}
	for _, field := range codec.Form.Fields {
		name := v3AttributeName(field)
		value, carried := values.Fields[name]
		if !carried {
			return nil, false
		}
		if value == nil || value.IsNull() {
			continue
		}
		if value.IsUnknown() {
			return nil, false
		}
		wire, fieldDiags := v3FieldToWire(ctx, codec.Form.Family.APIVersion(), field, name, value)
		if fieldDiags.HasError() {
			return nil, false
		}
		if wire != nil {
			spec[field.Wire] = wire
		}
	}
	// Apply always materializes the exact Form defaults before deriving a name
	// or sending desired state. Plan must canonicalize the same logical spec;
	// otherwise a provider-computed operation key (and the derived revision
	// name that includes it) would drift solely because framework defaults were
	// not run by a direct or older planner.
	return model.MaterializeDefaults(codec.DesiredSchema, spec), true
}

// v3PlanCodec resolves the codec a plan should encode under: the identity
// recorded in the planned state when there is one, and this build's create
// target otherwise. A plan that cannot resolve one is not an error here — the
// lifecycle path raises that, with the full fail-closed diagnostic.
func (r *v3FormResource) v3PlanCodec(ctx context.Context, resp *resource.ModifyPlanResponse) (v3FormCodec, bool) {
	var identity v3StateIdentity
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("form_api_version"), &identity.APIVersion)...)
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("form_kind"), &identity.Kind)...)
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("form_definition_version"), &identity.DefinitionVersion)...)
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("form_schema_digest"), &identity.SchemaDigest)...)
	if resp.Diagnostics.HasError() {
		return v3FormCodec{}, false
	}
	if ref, complete := identity.formRef(); complete {
		if codec, supported := r.codecTable().forStateKey(ref.exactKey()); supported {
			return codec, true
		}
		return v3FormCodec{}, false
	}
	codec, err := r.codecTable().defaultCreate(v3GroupKind{
		APIVersion: r.form.Family.APIVersion(), Kind: r.form.Kind,
	})
	return codec, err == nil
}

// v3PlanImmutableRevisionSafety refuses, at plan time, the replacement that
// cannot complete.
//
// The condition is narrow on purpose: a `revision`-role resource that already
// exists, a plan that replaces it for some reason other than the relation-drift
// recovery marker, and a planned host name identical to the recorded one. That
// is exactly the shape both apply orders fail on, and nothing else is refused —
// a replacement onto a NEW name is the supported path, and the drift-recovery
// replacement of decision 0015 keeps its own remedy, which already names what
// the operator must do.
func (r *v3FormResource) v3PlanImmutableRevisionSafety(
	ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse,
) {
	if !r.derivesRevisionName() || req.State.Raw.IsNull() || resp.Plan.Raw.IsNull() {
		return
	}
	replaced := false
	for _, candidate := range resp.RequiresReplace {
		if !candidate.Equal(path.Root(v3RelationDriftAttribute)) {
			replaced = true
		}
	}
	if !replaced {
		return
	}
	var prior, planned types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("name"), &prior)...)
	resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("name"), &planned)...)
	if resp.Diagnostics.HasError() || planned.IsUnknown() || !planned.Equal(prior) {
		return
	}
	resp.Diagnostics.Append(r.v3ImmutableSameNameDiagnostic(ctx, req, resp))
}
