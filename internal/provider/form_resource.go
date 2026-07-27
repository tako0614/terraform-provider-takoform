package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

// formResource implements every table-driven portable Form.
//
// One implementation serves every kind because a Form is data: its typed
// fields, grammars, defaults, and replacement rules all come from the
// declaration in internal/formcatalog. A new Form is a new catalogue entry,
// never a new copy of this lifecycle.
type formResource struct {
	kind formcatalog.Kind
	data *providerData
}

var (
	_ resource.Resource                = (*formResource)(nil)
	_ resource.ResourceWithImportState = (*formResource)(nil)
)

// NewFormResource returns a constructor for one declared Form.
func NewFormResource(kind formcatalog.Kind) func() resource.Resource {
	return func() resource.Resource { return &formResource{kind: kind} }
}

func (r *formResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = r.kind.ResourceType
}

func (r *formResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := commonFormAttributes()
	if r.kind.Artifact {
		for name, attribute := range artifactSourceAttributes() {
			attrs[name] = attribute
		}
	}
	if r.kind.Connections == formcatalog.ConnectionsRequired {
		attrs["connections"] = requiredResourceConnectionAttribute()
	} else if r.kind.Connections == formcatalog.ConnectionsOptional {
		attrs["connections"] = resourceConnectionAttribute()
	}
	for _, field := range r.kind.Fields {
		attrs[field.HCL] = fieldAttribute(field)
	}
	resp.Schema = schema.Schema{Description: r.kind.Description, Attributes: attrs}
}

func fieldAttribute(field formcatalog.Field) schema.Attribute {
	switch field.Type {
	case formcatalog.TypeBool:
		attribute := schema.BoolAttribute{
			Optional: !field.Required, Required: field.Required, Description: field.Doc,
		}
		if field.Immutable {
			attribute.PlanModifiers = []planmodifier.Bool{boolplanmodifier.RequiresReplace()}
		}
		if field.Default == "true" || field.Default == "false" {
			attribute.Computed = true
			attribute.Default = booldefault.StaticBool(field.Default == "true")
		}
		return attribute
	case formcatalog.TypeInt:
		attribute := schema.Int64Attribute{
			Optional: !field.Required, Required: field.Required, Description: field.Doc,
			Validators: int64Validators(field),
		}
		if field.Immutable {
			attribute.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
		}
		return attribute
	case formcatalog.TypeIntSet:
		attribute := schema.SetAttribute{
			Optional: !field.Required, Required: field.Required, Description: field.Doc,
			ElementType: types.Int64Type,
			Validators:  []validator.Set{SetInt64Range(field.MinItems, boundOr(field.Min, 0), boundOr(field.Max, 1<<31))},
		}
		if field.Immutable {
			attribute.PlanModifiers = []planmodifier.Set{setplanmodifier.RequiresReplace()}
		}
		return attribute
	case formcatalog.TypeStringSet:
		attribute := schema.SetAttribute{
			Optional: !field.Required, Required: field.Required, Description: field.Doc,
			ElementType: types.StringType, Validators: setValidators(field),
		}
		if field.Immutable {
			attribute.PlanModifiers = []planmodifier.Set{setplanmodifier.RequiresReplace()}
		}
		return attribute
	default:
		attribute := schema.StringAttribute{
			Optional: !field.Required, Required: field.Required, Description: field.Doc,
			Validators: stringValidators(field),
		}
		if field.Default != "" {
			attribute.Computed = true
			attribute.Default = stringdefault.StaticString(field.Default)
		}
		if field.Immutable {
			attribute.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
		}
		return attribute
	}
}

func stringValidators(field formcatalog.Field) []validator.String {
	if len(field.Enum) > 0 {
		return []validator.String{StringOneOf(field.Enum...)}
	}
	if pattern, ok := field.Grammar.Pattern(); ok {
		return []validator.String{StringMatches(pattern, field.Grammar.Message(field.HCL))}
	}
	return nil
}

func setValidators(field formcatalog.Field) []validator.Set {
	if len(field.Enum) > 0 {
		return []validator.Set{SetStringsOneOf(field.MinItems, field.Enum...)}
	}
	if pattern, ok := field.Grammar.Pattern(); ok {
		return []validator.Set{SetStringsMatch(field.MinItems, pattern, field.Grammar.Message(field.HCL))}
	}
	return []validator.Set{SetStringsNonEmpty(field.MinItems)}
}

func int64Validators(field formcatalog.Field) []validator.Int64 {
	var validators []validator.Int64
	if field.Min != nil {
		validators = append(validators, Int64AtLeast(*field.Min))
	}
	if field.Max != nil {
		validators = append(validators, Int64AtMost(*field.Max))
	}
	return validators
}

func boundOr(value *int64, fallback int64) int64 {
	if value == nil {
		return fallback
	}
	return *value
}

func commonFormAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Required:    true,
			Description: "Resource name. Changing it replaces the resource.",
			Validators:  []validator.String{StringMatches(formcatalog.PatternName, "name must not be blank")},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"space": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Description: "Space for this resource. Overrides the provider default; changing it replaces the resource.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"id": schema.StringAttribute{
			Computed:    true,
			Description: "Takoform resource identifier.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"resource_version": schema.StringAttribute{
			Computed:    true,
			Description: "Opaque desired-generation fence returned by the Form host.",
		},
		"drift_status": schema.StringAttribute{
			Computed:    true,
			Description: "Read-only native observation result: current, drifted, or missing.",
		},
		"portability": schema.StringAttribute{
			Computed:    true,
			Description: "Host-reported portability assessment.",
		},
		"outputs": schema.MapAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "Sanitized public outputs returned by the host.",
		},
	}
}

func artifactSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"artifact_path": schema.StringAttribute{
			Optional:    true,
			Description: "OpenTofu-runner-local path to a prebuilt artifact.",
		},
		"artifact_url": schema.StringAttribute{
			Optional:    true,
			Description: "HTTPS URL to an immutable artifact. Requires artifact_sha256.",
		},
		"artifact_ref": schema.StringAttribute{
			Optional:    true,
			Description: "Host-allocated opaque immutable artifact reference. Requires artifact_sha256.",
		},
		"artifact_sha256": schema.StringAttribute{
			Optional:    true,
			Description: "Expected artifact SHA-256 digest for artifact_url or artifact_ref.",
		},
	}
}

func (r *formResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// formValues is the plan or state of one Form, read generically.
type formValues struct {
	Name            types.String
	Space           types.String
	ResourceVersion types.String
	Fields          map[string]attr.Value
	Connections     types.List
	Artifact        artifactSourceValues
}

func (r *formResource) valuesFrom(ctx context.Context, getter interface {
	GetAttribute(context.Context, path.Path, any) diag.Diagnostics
}) (formValues, diag.Diagnostics) {
	var diags diag.Diagnostics
	values := formValues{Fields: map[string]attr.Value{}, Artifact: nullArtifactSourceValues()}
	diags.Append(getter.GetAttribute(ctx, path.Root("name"), &values.Name)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("space"), &values.Space)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("resource_version"), &values.ResourceVersion)...)
	if r.kind.Connections != formcatalog.ConnectionsAbsent {
		diags.Append(getter.GetAttribute(ctx, path.Root("connections"), &values.Connections)...)
	}
	if r.kind.Artifact {
		diags.Append(getter.GetAttribute(ctx, path.Root("artifact_path"), &values.Artifact.Path)...)
		diags.Append(getter.GetAttribute(ctx, path.Root("artifact_url"), &values.Artifact.URL)...)
		diags.Append(getter.GetAttribute(ctx, path.Root("artifact_ref"), &values.Artifact.Ref)...)
		diags.Append(getter.GetAttribute(ctx, path.Root("artifact_sha256"), &values.Artifact.SHA256)...)
	}
	for _, field := range r.kind.Fields {
		switch field.Type {
		case formcatalog.TypeBool:
			var value types.Bool
			diags.Append(getter.GetAttribute(ctx, path.Root(field.HCL), &value)...)
			values.Fields[field.HCL] = value
		case formcatalog.TypeInt:
			var value types.Int64
			diags.Append(getter.GetAttribute(ctx, path.Root(field.HCL), &value)...)
			values.Fields[field.HCL] = value
		case formcatalog.TypeIntSet, formcatalog.TypeStringSet:
			var value types.Set
			diags.Append(getter.GetAttribute(ctx, path.Root(field.HCL), &value)...)
			values.Fields[field.HCL] = value
		default:
			var value types.String
			diags.Append(getter.GetAttribute(ctx, path.Root(field.HCL), &value)...)
			values.Fields[field.HCL] = value
		}
	}
	return values, diags
}

// toResource projects declared values onto the portable wire spec. Only
// declared fields travel, and an unknown plan value never becomes desired
// state.
func (r *formResource) toResource(ctx context.Context, values formValues) (*client.Resource, string, diag.Diagnostics) {
	var diags diag.Diagnostics
	spec := map[string]any{"name": strings.TrimSpace(values.Name.ValueString())}
	if r.kind.Artifact {
		source, sourceDiags := values.Artifact.toSpec(r.kind.ResourceType)
		diags.Append(sourceDiags...)
		if source != nil {
			spec["source"] = source
		}
	}
	if r.kind.Connections != formcatalog.ConnectionsAbsent {
		if connections := resourceConnectionsToSpec(ctx, values.Connections, &diags); len(connections) > 0 {
			spec["connections"] = connections
		}
	}
	for _, field := range r.kind.Fields {
		value, ok := values.Fields[field.HCL]
		if !ok || value == nil || value.IsNull() || value.IsUnknown() {
			continue
		}
		switch typed := value.(type) {
		case types.Bool:
			spec[field.Wire] = typed.ValueBool()
		case types.Int64:
			spec[field.Wire] = typed.ValueInt64()
		case types.String:
			trimmed := strings.TrimSpace(typed.ValueString())
			if trimmed == "" {
				continue
			}
			spec[field.Wire] = trimmed
		case types.Set:
			if field.Type == formcatalog.TypeIntSet {
				var members []int64
				diags.Append(typed.ElementsAs(ctx, &members, false)...)
				if len(members) > 0 {
					spec[field.Wire] = members
				}
				continue
			}
			var members []string
			diags.Append(typed.ElementsAs(ctx, &members, false)...)
			if len(members) > 0 {
				spec[field.Wire] = members
			}
		}
	}
	space := strings.TrimSpace(values.Space.ValueString())
	if space == "" {
		space = r.data.defaultSpace
	}
	if space == "" {
		diags.AddAttributeError(path.Root("space"), "Missing space",
			"Set the resource space or configure the provider default space.")
	}
	body := &client.Resource{
		APIVersion: client.APIVersion, Kind: r.kind.Kind,
		Metadata: client.Metadata{Name: spec["name"].(string), Space: space},
		Spec:     spec,
	}
	return body, space, diags
}

func (r *formResource) assertConfigured(diags *diag.Diagnostics) bool {
	if r.data == nil || r.data.client == nil {
		diags.AddError(
			"Provider not configured",
			"The takoform provider was not configured before use. This is usually a provider bug.",
		)
		return false
	}
	if _, ok := r.data.forms[r.kind.Kind]; !ok {
		diags.AddError(r.kind.Kind+" FormRef missing",
			"This provider build has no exact candidate "+r.kind.Kind+" FormRef. This is a provider bug.")
		return false
	}
	return true
}

func (r *formResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if !r.assertConfigured(&resp.Diagnostics) {
		return
	}
	values, diags := r.valuesFrom(ctx, req.Plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.put(ctx, values, &resp.State, &resp.Diagnostics)
}

func (r *formResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if !r.assertConfigured(&resp.Diagnostics) {
		return
	}
	values, diags := r.valuesFrom(ctx, req.Plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if values.ResourceVersion.IsNull() || values.ResourceVersion.IsUnknown() {
		var state formValues
		state, diags = r.valuesFrom(ctx, req.State)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		values.ResourceVersion = state.ResourceVersion
	}
	r.put(ctx, values, &resp.State, &resp.Diagnostics)
}

func (r *formResource) put(ctx context.Context, values formValues, state *tfsdk.State, diags *diag.Diagnostics) {
	body, space, bodyDiags := r.toResource(ctx, values)
	diags.Append(bodyDiags...)
	if diags.HasError() {
		return
	}
	form := r.data.forms[r.kind.Kind]
	body.Form = &form
	if !values.ResourceVersion.IsNull() && !values.ResourceVersion.IsUnknown() {
		body.Metadata.ResourceVersion = values.ResourceVersion.ValueString()
	}
	r.data.serviceFormMutate.Lock()
	defer r.data.serviceFormMutate.Unlock()
	res, err := r.data.client.PutResource(ctx, r.kind.Kind, body.Metadata.Name, body)
	if err != nil {
		diags.AddError("Failed to apply "+r.kind.Kind, err.Error())
		return
	}
	diags.Append(r.setState(ctx, state, res, space, values, false)...)
}

func (r *formResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if !r.assertConfigured(&resp.Diagnostics) {
		return
	}
	values, diags := r.valuesFrom(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	space := strings.TrimSpace(values.Space.ValueString())
	if space == "" {
		space = r.data.defaultSpace
	}
	if space == "" {
		resp.Diagnostics.AddAttributeError(path.Root("space"), "Missing space",
			"Import as SPACE/NAME or configure the provider space before reading this resource.")
		return
	}
	form := r.data.forms[r.kind.Kind]
	res, err := observeResourceForRead(ctx, r.data.client, r.kind.Kind, values.Name.ValueString(), space, form)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read "+r.kind.Kind, err.Error())
		return
	}
	// A read is the only place host-observed desired state may replace what
	// was configured: that is what makes import populate state and what makes
	// an out-of-band change visible as a plan diff.
	resp.Diagnostics.Append(r.setState(ctx, &resp.State, res, space, values, true)...)
}

func (r *formResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if !r.assertConfigured(&resp.Diagnostics) {
		return
	}
	values, diags := r.valuesFrom(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	space := strings.TrimSpace(values.Space.ValueString())
	if space == "" {
		space = r.data.defaultSpace
	}
	if space == "" {
		resp.Diagnostics.AddAttributeError(path.Root("space"), "Missing space",
			"Configure the provider space before deleting this resource.")
		return
	}
	r.data.serviceFormMutate.Lock()
	defer r.data.serviceFormMutate.Unlock()
	form := r.data.forms[r.kind.Kind]
	if err := r.data.client.DeleteResource(ctx, r.kind.Kind, values.Name.ValueString(), space,
		client.MutationFence{ResourceVersion: values.ResourceVersion.ValueString(), Form: form}); err != nil {
		resp.Diagnostics.AddError("Failed to delete "+r.kind.Kind, err.Error())
	}
}

func (r *formResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	space, name := splitImportID(req.ID)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(name))...)
	if space != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space"), types.StringValue(space))...)
	}
}

// setState writes host-owned observation back into Terraform state. Desired
// values stay exactly as configured; only identity, fence, drift, portability,
// and sanitized outputs come from the host.
func (r *formResource) setState(ctx context.Context, state *tfsdk.State, res *client.Resource, space string, values formValues, refresh bool) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.Append(state.SetAttribute(ctx, path.Root("name"), types.StringValue(res.Metadata.Name))...)
	diags.Append(state.SetAttribute(ctx, path.Root("space"), types.StringValue(space))...)
	diags.Append(state.SetAttribute(ctx, path.Root("id"), types.StringValue(resourceIDForKind(res, space, r.kind.Kind, res.Metadata.Name)))...)
	diags.Append(state.SetAttribute(ctx, path.Root("resource_version"), optionalString(res.Metadata.ResourceVersion))...)

	driftStatus := types.StringNull()
	portability := types.StringNull()
	outputs := types.MapNull(types.StringType)
	if res.Status != nil {
		driftStatus = optionalString(res.Status.DriftStatus)
		portability = optionalString(res.Status.Portability)
		if res.Status.Portability == "" {
			portability = optionalString(res.Status.Resolution.Portability)
		}
		mapped, mapDiags := types.MapValueFrom(ctx, types.StringType, outputsToStringMap(res.Status.Outputs))
		diags.Append(mapDiags...)
		outputs = mapped
	}
	diags.Append(state.SetAttribute(ctx, path.Root("drift_status"), driftStatus)...)
	diags.Append(state.SetAttribute(ctx, path.Root("portability"), portability)...)
	diags.Append(state.SetAttribute(ctx, path.Root("outputs"), outputs)...)

	if r.kind.Artifact {
		source := artifactSourceValuesFromSpec(res.Spec["source"])
		if refresh || (values.Artifact.Path.IsNull() && values.Artifact.URL.IsNull() && values.Artifact.Ref.IsNull()) {
			values.Artifact = source
		}
		diags.Append(state.SetAttribute(ctx, path.Root("artifact_path"), values.Artifact.Path)...)
		diags.Append(state.SetAttribute(ctx, path.Root("artifact_url"), values.Artifact.URL)...)
		diags.Append(state.SetAttribute(ctx, path.Root("artifact_ref"), values.Artifact.Ref)...)
		diags.Append(state.SetAttribute(ctx, path.Root("artifact_sha256"), values.Artifact.SHA256)...)
	}
	if r.kind.Connections != formcatalog.ConnectionsAbsent {
		connections := values.Connections
		if refresh || connections.IsNull() || connections.IsUnknown() {
			refreshed, connectionDiags := resourceConnectionsFromSpec(ctx, res.Spec["connections"])
			diags.Append(connectionDiags...)
			connections = refreshed
		}
		diags.Append(state.SetAttribute(ctx, path.Root("connections"), connections)...)
	}
	for _, field := range r.kind.Fields {
		value := values.Fields[field.HCL]
		if refresh || value == nil || value.IsUnknown() {
			value = fieldValueFromSpec(ctx, field, res.Spec[field.Wire], &diags)
		}
		// A portable default is the value a host applies when the field is
		// absent, so state must show it rather than a null the next plan would
		// try to fill.
		if value.IsNull() && field.Type == formcatalog.TypeString && field.Default != "" {
			value = types.StringValue(field.Default)
		}
		diags.Append(state.SetAttribute(ctx, path.Root(field.HCL), value)...)
	}
	return diags
}

func fieldValueFromSpec(ctx context.Context, field formcatalog.Field, raw any, diags *diag.Diagnostics) attr.Value {
	switch field.Type {
	case formcatalog.TypeBool:
		if value, ok := raw.(bool); ok {
			return types.BoolValue(value)
		}
		return types.BoolNull()
	case formcatalog.TypeInt:
		return int64FromSpec(raw)
	case formcatalog.TypeIntSet:
		if raw == nil {
			return types.SetNull(types.Int64Type)
		}
		set, setDiags := types.SetValueFrom(ctx, types.Int64Type, toInt64Slice(raw))
		diags.Append(setDiags...)
		return set
	case formcatalog.TypeStringSet:
		if raw == nil {
			return types.SetNull(types.StringType)
		}
		set, setDiags := types.SetValueFrom(ctx, types.StringType, toStringSlice(raw))
		diags.Append(setDiags...)
		return set
	default:
		return optionalStringFromAny(raw)
	}
}

func optionalString(value string) types.String {
	if strings.TrimSpace(value) == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func splitImportID(id string) (space, name string) {
	if index := strings.Index(id, "/"); index > 0 {
		return id[:index], id[index+1:]
	}
	return "", id
}
