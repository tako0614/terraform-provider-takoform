package provider

// v3_schema_evolution.go states — and enforces what can be enforced of — the
// rule that relates a moving Form line to a static Terraform resource schema.
//
// The two evolve on different clocks and neither can wait for the other. A Form
// line advances by publishing a new immutable definition, and decision 0017
// already settles what that costs persisted state: the provider dispatches on
// the exact FormRef recorded in state and carries one codec per supported
// identity, so a resource stays addressable as itself. A Terraform resource
// SCHEMA cannot do that. There is exactly one `takoform_worker_version` schema
// in a build, every resource of that type is decoded through it, and a
// configuration written against it is source a user maintains.
//
// So the rule is about the schema, not about the codec:
//
// The SAME Terraform resource type is kept when every existing attribute keeps
// exactly its meaning and the change is one of
//
//   - adding an Optional attribute,
//   - adding a Computed attribute (a declared output included),
//   - relaxing validation, and
//   - adding an enum value that breaks neither an existing host nor existing
//     state.
//
// A NEW Terraform resource type is required for
//
//   - removing an attribute,
//   - changing an attribute's type,
//   - making an attribute Required,
//   - changing a declared output's type,
//   - changing the Form's lifecycle role,
//   - changing the identity or the replacement unit, and
//   - any other semantic break.
//
// A Form that breaks `takoform_worker_version`'s schema becomes
// `takoform_worker_version_v2`, or a different Form kind. Both then exist in
// one build: the old type keeps serving the state written under it through its
// own codec, and the new type is what new configurations write.
//
// The rule is enforced two ways, because half of it is mechanical and half is
// not. v3SurfaceBaseline (testdata/v3-schema-baseline.json) is a FLOOR: a test
// derives the current surface from the live schemas and refuses any difference
// that is not on the allowed list above, naming the resource type that would
// have to be minted. What no test can decide is whether an attribute kept its
// MEANING, so the rule states it and review carries it.
//
// The version below is the other half of the same rule: the schema version is
// what lets a resource type outlive a change to its own state layout, and a
// state upgrader is how state written at an earlier version becomes readable
// at the current one. The v1alpha3 lane needs one registered even while the
// upgrade is an identity, because the day it is not an identity is the day it
// is too late to add the mechanism.

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// v3SchemaVersion is the persisted state layout of every v1alpha3-lane
// resource.
//
// Version 1 is the first numbered layout of this lane. The lane ships in
// provider v2.1.0, an unpublished source candidate, so no user state exists at
// version 0 outside development; registering the upgrader anyway is what makes
// the mechanism real rather than aspirational, and it is exercised by
// TestV3StateUpgraderRoundTrip.
const v3SchemaVersion int64 = 1

var _ resource.ResourceWithUpgradeState = (*v3FormResource)(nil)

// UpgradeState carries state written under an earlier layout of this resource
// type forward to the current one.
//
// The version 0 upgrade is a whole-value carry: the v1alpha3 state model did
// not change shape between the two versions, so every attribute keeps its name,
// its type, and its meaning, and the honest upgrade is the identity. It is
// declared with a PriorSchema so the framework decodes the old value against a
// real schema rather than handing over raw JSON this code would have to parse —
// a hand-written parse is where a state upgrader silently loses an attribute.
func (r *v3FormResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	prior := r.v3PriorSchema(ctx, 0)
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &prior,
			StateUpgrader: func(
				_ context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse,
			) {
				if req.State == nil {
					resp.Diagnostics.Append(v3Diagnostic{
						Summary:      "State to upgrade is missing",
						ResourceType: r.form.ResourceType,
						Pointer:      "/metadata",
						Code:         v3CodeProviderBug,
						Detail:       "Terraform asked for a state upgrade and supplied no prior state.",
						Repair:       "This is a provider or CLI bug. Report it with the CLI version.",
					}.error())
					return
				}
				resp.State = tfsdk.State{Schema: resp.State.Schema, Raw: req.State.Raw}
			},
		},
	}
}

// v3PriorSchema renders the state layout of one earlier schema version.
//
// Version 0 is structurally the current schema: `name` moved from Required to
// Optional+Computed on a revision Form, which changes what a CONFIGURATION may
// write and not what STATE holds, so the prior value decodes unchanged. A
// future version that does move the layout adds its own branch here rather than
// reusing this one.
func (r *v3FormResource) v3PriorSchema(ctx context.Context, version int64) schema.Schema {
	var response resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &response)
	prior := response.Schema
	prior.Version = version
	return prior
}
