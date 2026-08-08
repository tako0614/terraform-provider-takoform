package provider

// v3_generic_resource.go is takoform_resource: the generic Host API v1alpha3
// carrier for third-party Forms. The configuration names the exact FormRef
// (namespaced group, kind, definition version, schema digest) and one JSON
// desired spec; the provider validates the FormRef grammar locally, fetches
// nothing at plan time, and applies through the v1alpha3 client, whose
// EnsureFormAvailable fails closed when the host does not support the exact
// ref.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

type v3GenericResource struct {
	data *providerData
}

var (
	_ resource.Resource               = (*v3GenericResource)(nil)
	_ resource.ResourceWithConfigure  = (*v3GenericResource)(nil)
	_ resource.ResourceWithModifyPlan = (*v3GenericResource)(nil)
)

// NewV3GenericResource constructs the generic exact-FormRef resource.
func NewV3GenericResource() resource.Resource { return &v3GenericResource{} }

func (r *v3GenericResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "takoform_resource"
}

func (r *v3GenericResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *v3GenericResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generic Host API v1alpha3 resource carrying any Form by exact FormRef. " +
			"The provider validates the FormRef grammar and applies the JSON desired spec; the host " +
			"fails closed when it does not support the exact ref. Requires provider v2.1.0 or later.",
		Attributes: map[string]schema.Attribute{
			"form_api_version": schema.StringAttribute{
				Required: true,
				Description: "Exact namespaced Form group/version, for example forms.example.com/v1alpha1. " +
					"The frozen forms.takoform.com/v1alpha1 and /v1alpha2 groups are rejected.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"form_kind": schema.StringAttribute{
				Required:      true,
				Description:   "Exact Form kind.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"form_definition_version": schema.StringAttribute{
				Required:      true,
				Description:   "Exact immutable Form definition version.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"form_schema_digest": schema.StringAttribute{
				Required:      true,
				Description:   "Exact immutable Form schema digest (sha256:<hex>).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Portable resource name (metadata.name). Changing it replaces the resource.",
				Validators: []validator.String{StringMatches(
					model.PatternResourceName,
					"name must start with a lowercase letter and contain only lowercase letters, digits, or hyphens",
				)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"space": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Exact opaque SpaceID for this resource. Overrides the provider default; changing it replaces the resource.",
				Validators:  []validator.String{StringSpaceID()},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"spec_json": schema.StringAttribute{
				Optional:    true,
				Description: "Desired spec as one JSON object; omitted means the empty spec.",
				Validators:  []validator.String{v3JSONObjectValidator{}},
			},
			"pending_operation_id":   v3PendingOperationIDAttribute(),
			v3RelationDriftAttribute: v3RelationDriftReasonAttribute(),
			"uid": schema.StringAttribute{
				Computed:      true,
				Description:   "Host-issued immutable resource identity.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"generation": schema.StringAttribute{
				Computed:    true,
				Description: "Canonical decimal desired-state generation fence.",
			},
			"revision": schema.StringAttribute{
				Computed:    true,
				Description: "Canonical decimal representation revision; deletes fence on it via If-Match.",
			},
			"ready": schema.BoolAttribute{
				Computed:    true,
				Description: "True when the host reports the closed Ready condition with status True.",
			},
			"outputs_json": schema.StringAttribute{
				Computed:    true,
				Description: "JSON-serialized status.outputs document; \"{}\" when absent.",
			},
			"create_timeout": schema.StringAttribute{
				Optional:    true,
				Description: "Overall create deadline as a Go duration (default \"20m\").",
				Validators:  []validator.String{v3DurationValidator{}},
			},
			"update_timeout": schema.StringAttribute{
				Optional:    true,
				Description: "Overall update deadline as a Go duration (default \"20m\").",
				Validators:  []validator.String{v3DurationValidator{}},
			},
			"delete_timeout": schema.StringAttribute{
				Optional:    true,
				Description: "Overall delete deadline as a Go duration (default \"30m\").",
				Validators:  []validator.String{v3DurationValidator{}},
			},
		},
	}
}

type v3GenericValues struct {
	Identity      v3StateIdentity
	Name          types.String
	Space         types.String
	SpecJSON      types.String
	UID           types.String
	Generation    types.String
	Revision      types.String
	CreateTimeout types.String
	UpdateTimeout types.String
	DeleteTimeout types.String
}

func v3GenericValuesFrom(ctx context.Context, getter v3AttributeGetter) (v3GenericValues, diag.Diagnostics) {
	var values v3GenericValues
	var diags diag.Diagnostics
	diags.Append(getter.GetAttribute(ctx, path.Root("form_api_version"), &values.Identity.APIVersion)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_kind"), &values.Identity.Kind)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_definition_version"), &values.Identity.DefinitionVersion)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_schema_digest"), &values.Identity.SchemaDigest)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("name"), &values.Name)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("space"), &values.Space)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("spec_json"), &values.SpecJSON)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("uid"), &values.UID)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("generation"), &values.Generation)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("revision"), &values.Revision)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("create_timeout"), &values.CreateTimeout)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("update_timeout"), &values.UpdateTimeout)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("delete_timeout"), &values.DeleteTimeout)...)
	return values, diags
}

// exactRef validates the configured FormRef grammar without consulting any
// registry: third-party groups are exactly what this resource exists for.
func (values v3GenericValues) exactRef(diags *diag.Diagnostics) (clientv3.FormRef, bool) {
	got, ok := values.Identity.formRef()
	if !ok {
		diags.AddError(
			"Incomplete exact FormRef",
			"form_api_version, form_kind, form_definition_version, and form_schema_digest must all be wholly known non-empty values.",
		)
		return clientv3.FormRef{}, false
	}
	ref := clientv3.FormRef{
		APIVersion:        got.APIVersion,
		Kind:              got.Kind,
		DefinitionVersion: got.DefinitionVersion,
		SchemaDigest:      got.SchemaDigest,
	}
	if err := clientv3.ValidateFormRef(ref); err != nil {
		diags.AddError("Invalid exact FormRef", err.Error())
		return clientv3.FormRef{}, false
	}
	return ref, true
}

func (values v3GenericValues) spec(diags *diag.Diagnostics) (map[string]any, bool) {
	if values.SpecJSON.IsUnknown() {
		diags.AddAttributeError(path.Root("spec_json"), "Unknown spec_json",
			"spec_json must be wholly known before the provider can apply this resource.")
		return nil, false
	}
	if values.SpecJSON.IsNull() || values.SpecJSON.ValueString() == "" {
		return map[string]any{}, true
	}
	parsed, err := v3ParseJSONObject(values.SpecJSON.ValueString())
	if err != nil {
		diags.AddAttributeError(path.Root("spec_json"), "Invalid JSON object", err.Error())
		return nil, false
	}
	return parsed, true
}

func (r *v3GenericResource) assertConfigured(diags *diag.Diagnostics) bool {
	return assertV3Lane(r.data, "takoform_resource", diags)
}

func (r *v3GenericResource) apply(
	ctx context.Context,
	values v3GenericValues,
	fence clientv3.Fence,
	timeoutValue types.String,
	timeoutAttr string,
	fallback time.Duration,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	ref, ok := values.exactRef(diags)
	if !ok {
		return
	}
	space, ok := v3EffectiveSpace(values.Space, r.data.defaultSpace, diags)
	if !ok {
		return
	}
	spec, ok := values.spec(diags)
	if !ok {
		return
	}
	timeout, ok := v3Timeout(timeoutValue, timeoutAttr, fallback, diags)
	if !ok {
		return
	}
	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	body := &clientv3.Resource{
		APIVersion: ref.APIVersion,
		Kind:       ref.Kind,
		Form:       &clientv3.FormReference{FormRef: ref},
		Metadata:   clientv3.Metadata{Name: values.Name.ValueString(), Space: space},
		Spec:       spec,
	}
	res, err := r.data.clientV3.ApplyResource(opCtx, body, fence)
	if err != nil {
		// See v3FormResource.writeV3AcceptedState: a create the host accepted but
		// that never completed verifiably must still leave enough identity in
		// state for the next plan to reconcile instead of creating a duplicate.
		// Updates already have prior state and must not have it overwritten by a
		// partial record, so only the create fence takes this path.
		if fence.ExpectedGeneration == "" {
			r.writeAcceptedState(ctx, state, ref, space, values, err, diags)
		}
		diags.AddError("Failed to apply takoform_resource", err.Error())
		return
	}
	diags.Append(r.writeState(ctx, state, ref, space, values, res)...)
}

// writeAcceptedState is the generic-carrier form of
// v3FormResource.writeV3AcceptedState.
func (r *v3GenericResource) writeAcceptedState(
	ctx context.Context,
	state *tfsdk.State,
	ref clientv3.FormRef,
	space string,
	values v3GenericValues,
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
	diags.Append(r.writeState(ctx, state, ref, space, values, partial)...)
	diags.Append(state.SetAttribute(ctx, path.Root("pending_operation_id"), v3OptionalStateString(accepted.OperationID))...)
	diags.AddWarning(
		"takoform_resource was accepted by the host but did not complete",
		v3AcceptedRecoveryDetail(accepted, space, values.Name.ValueString(), ref.Kind),
	)
}

// ModifyPlan makes a broken relation recoverable through the generic carrier
// too. The carrier always exposes an in-place update, so the proposed remedy
// is a re-apply of the same desired spec: a host re-resolves and re-pins every
// relation on any accepted apply.
func (r *v3GenericResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	v3PlanRelationRecovery(ctx, req.State, true, resp)
}

func (r *v3GenericResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if !r.assertConfigured(&resp.Diagnostics) {
		return
	}
	values, diags := v3GenericValuesFrom(ctx, req.Plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, values, clientv3.Fence{}, values.CreateTimeout, "create_timeout", v3DefaultCreateTimeout, &resp.State, &resp.Diagnostics)
}

func (r *v3GenericResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if !r.assertConfigured(&resp.Diagnostics) {
		return
	}
	values, diags := v3GenericValuesFrom(ctx, req.Plan)
	resp.Diagnostics.Append(diags...)
	stateValues, stateDiags := v3GenericValuesFrom(ctx, req.State)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	fence := clientv3.Fence{
		ExpectedUID:        stateValues.UID.ValueString(),
		ExpectedGeneration: stateValues.Generation.ValueString(),
	}
	r.apply(ctx, values, fence, values.UpdateTimeout, "update_timeout", v3DefaultUpdateTimeout, &resp.State, &resp.Diagnostics)
}

func (r *v3GenericResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if !r.assertConfigured(&resp.Diagnostics) {
		return
	}
	values, diags := v3GenericValuesFrom(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ref, ok := values.exactRef(&resp.Diagnostics)
	if !ok {
		return
	}
	space, ok := v3EffectiveSpace(values.Space, r.data.defaultSpace, &resp.Diagnostics)
	if !ok {
		return
	}
	res, err := r.data.clientV3.GetResource(ctx, space, ref, values.Name.ValueString())
	if err != nil {
		if errors.Is(err, clientv3.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read takoform_resource", err.Error())
		return
	}
	if !values.UID.IsNull() && !values.UID.IsUnknown() && values.UID.ValueString() != res.Metadata.UID {
		resp.Diagnostics.AddWarning(
			"takoform_resource was replaced out of band",
			fmt.Sprintf(
				"State records uid %s but the host now serves uid %s for %s/%s. The old resource no longer exists; the next plan will propose recreating it.",
				values.UID.ValueString(), res.Metadata.UID, space, values.Name.ValueString(),
			),
		)
		resp.State.RemoveResource(ctx)
		return
	}
	// The generic carrier exposes an in-place update for every Form it can
	// carry, so a broken relation is reported as an in-place remedy; a Form
	// whose Definition omits update refuses that apply at the host, naming the
	// missing capability.
	v3ReportRelationCondition(ref.Kind, space, values.Name.ValueString(), res, true, &resp.Diagnostics)
	// spec_json keeps the author's formatting while it is semantically equal
	// to the host's current spec; a real out-of-band change adopts the host's
	// canonical serialization so the next plan shows the drift.
	if !v3SpecJSONEquals(values.SpecJSON, res.Spec) {
		text, err := v3MarshalJSON(res.Spec)
		if err != nil {
			resp.Diagnostics.AddError("Host spec is not serializable", err.Error())
			return
		}
		values.SpecJSON = types.StringValue(text)
	}
	resp.Diagnostics.Append(r.writeState(ctx, &resp.State, ref, space, values, res)...)
}

func (r *v3GenericResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if !r.assertConfigured(&resp.Diagnostics) {
		return
	}
	values, diags := v3GenericValuesFrom(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ref, ok := values.exactRef(&resp.Diagnostics)
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
	err := r.data.clientV3.DeleteResource(opCtx, space, ref, values.Name.ValueString(), values.Revision.ValueString())
	if err != nil && !errors.Is(err, clientv3.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete takoform_resource", err.Error())
	}
}

func (r *v3GenericResource) writeState(
	ctx context.Context,
	state *tfsdk.State,
	ref clientv3.FormRef,
	space string,
	values v3GenericValues,
	res *clientv3.Resource,
) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.Append(state.SetAttribute(ctx, path.Root("form_api_version"), types.StringValue(ref.APIVersion))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_kind"), types.StringValue(ref.Kind))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_definition_version"), types.StringValue(ref.DefinitionVersion))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_schema_digest"), types.StringValue(ref.SchemaDigest))...)
	diags.Append(state.SetAttribute(ctx, path.Root("name"), types.StringValue(res.Metadata.Name))...)
	diags.Append(state.SetAttribute(ctx, path.Root("space"), types.StringValue(space))...)
	diags.Append(state.SetAttribute(ctx, path.Root("spec_json"), values.SpecJSON)...)
	diags.Append(state.SetAttribute(ctx, path.Root("uid"), types.StringValue(res.Metadata.UID))...)
	diags.Append(state.SetAttribute(ctx, path.Root("generation"), types.StringValue(res.Metadata.Generation))...)
	diags.Append(state.SetAttribute(ctx, path.Root("revision"), types.StringValue(res.Metadata.Revision))...)
	diags.Append(state.SetAttribute(ctx, path.Root("ready"), types.BoolValue(clientv3.ResourceReady(res)))...)
	diags.Append(state.SetAttribute(ctx, path.Root("outputs_json"), v3OutputsJSON(res, &diags))...)
	// A verified representation settles any earlier accepted-but-unfinished
	// mutation: there is nothing left to resume.
	diags.Append(state.SetAttribute(ctx, path.Root("pending_operation_id"), types.StringNull())...)
	diags.Append(state.SetAttribute(ctx, path.Root(v3RelationDriftAttribute), v3RelationDriftState(res))...)
	diags.Append(state.SetAttribute(ctx, path.Root("create_timeout"), values.CreateTimeout)...)
	diags.Append(state.SetAttribute(ctx, path.Root("update_timeout"), values.UpdateTimeout)...)
	diags.Append(state.SetAttribute(ctx, path.Root("delete_timeout"), values.DeleteTimeout)...)
	return diags
}

// v3SpecJSONEquals reports whether the configured spec_json is semantically
// the host spec, comparing JSON documents rather than text.
func v3SpecJSONEquals(specJSON types.String, hostSpec map[string]any) bool {
	var configured map[string]any
	if specJSON.IsNull() || specJSON.IsUnknown() || specJSON.ValueString() == "" {
		configured = map[string]any{}
	} else if err := json.Unmarshal([]byte(specJSON.ValueString()), &configured); err != nil {
		return false
	}
	normalized := hostSpec
	if normalized == nil {
		normalized = map[string]any{}
	}
	configuredRaw, err := json.Marshal(configured)
	if err != nil {
		return false
	}
	hostRaw, err := json.Marshal(normalized)
	if err != nil {
		return false
	}
	var left, right any
	if json.Unmarshal(configuredRaw, &left) != nil || json.Unmarshal(hostRaw, &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}
