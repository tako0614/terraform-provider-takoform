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
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
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

// v3RevisionNameFromSpec derives the host name of one revision from its
// resolved desired spec and its declared owner.
func (r *v3FormResource) v3RevisionNameFromSpec(spec map[string]any, owner string) (string, bool) {
	if v3FormRequiresArtifactRule(r.form) && r.artifact == nil {
		return "", false
	}
	digest, ok := v3ContentDigest(r.v3ArtifactBackedRevision(), spec)
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
	owner, owned := v3PlanKnownString(values.RevisionOwner)
	if !owned {
		diags.Append(r.v3RevisionOwnerMissing())
		return false
	}
	derived, ok := r.v3RevisionNameFromSpec(spec, owner)
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
	spec, resolved := r.v3PlannedSpec(ctx, resp)
	if resp.Diagnostics.HasError() {
		return
	}
	owner, owned := v3PlanKnownString(configuredOwner)
	var derived types.String
	if resolved && owned {
		name, ok := r.v3RevisionNameFromSpec(spec, owner)
		if !ok {
			return
		}
		derived = types.StringValue(name)
	} else if !req.State.Raw.IsNull() {
		// A replacement whose content did not resolve keeps the recorded name.
		// Marking it unknown here would propose a replacement on every plan for
		// a resource that may not have changed at all; leaving it alone hands
		// the case to v3PlanImmutableRevisionSafety, which states the problem.
		return
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
	return spec, true
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
	codec, err := r.codecTable().defaultCreate(currentformregistry.GroupKind{
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
	codec, _ := r.v3PlanCodec(ctx, resp)
	resp.Diagnostics.Append(v3Diagnostic{
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
	}.error())
}
