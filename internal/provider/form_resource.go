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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
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
// declaration selected by internal/currentformcatalog. A new Form is a new
// catalogue entry, never a new copy of this lifecycle.
type formResource struct {
	kind formcatalog.Kind
	data *providerData
}

var (
	_ resource.Resource                 = (*formResource)(nil)
	_ resource.ResourceWithModifyPlan   = (*formResource)(nil)
	_ resource.ResourceWithImportState  = (*formResource)(nil)
	_ resource.ResourceWithUpgradeState = (*formResource)(nil)
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
		attrs["connections"] = resourceConnectionAttribute(r.kind)
	} else if r.kind.Connections == formcatalog.ConnectionsOptional {
		attrs["connections"] = resourceConnectionAttribute(r.kind)
	}
	for _, field := range r.kind.Fields {
		attrs[field.HCL] = fieldAttribute(field)
	}
	resp.Schema = schema.Schema{
		Version:     1,
		Description: r.kind.Description,
		Attributes:  attrs,
	}
}

// UpgradeState deliberately rejects version-zero state without transforming
// it. Version-zero state has no exact Form identity, so provider v2 cannot
// prove which historical release or exact FormRef created it.
func (r *formResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			StateUpgrader: func(_ context.Context, _ resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				resp.Diagnostics.AddError(
					"Provider v2 requires explicit Form migration",
					"Schema-version-zero state cannot be transformed in place because it does not record an exact Form identity. State was not modified and no Resource lifecycle request was made. Pin the exact historical provider, or create/import the Resource explicitly into a v1alpha2 Form managed by provider v2.",
				)
			},
		},
	}
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
	case formcatalog.TypeStringMap:
		attribute := schema.MapAttribute{
			Optional: !field.Required, Required: field.Required, Description: field.Doc,
			ElementType: types.StringType,
			Validators:  []validator.Map{MapKeysMatch(formcatalog.PortableMapKeyPattern, field.HCL+" keys must use the portable map-key grammar")},
		}
		if field.Immutable {
			attribute.PlanModifiers = []planmodifier.Map{mapplanmodifier.RequiresReplace()}
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
			Description: "Lowercase URL-safe resource name. Changing it replaces the resource.",
			Validators:  []validator.String{StringMatches(formcatalog.PatternName, "name must start with a lowercase letter and contain only lowercase letters, digits, or hyphens")},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"space": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Description: "Exact opaque SpaceID for this resource. Overrides the provider default; changing it replaces the resource.",
			Validators: []validator.String{
				StringSpaceID(),
			},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"id": schema.StringAttribute{
			Computed:    true,
			Description: "Canonical Kind/name identifier synthesized by the provider; a host identifier is never trusted as state identity.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"resource_version": schema.StringAttribute{
			Computed:    true,
			Description: "Canonical decimal desired-generation fence in the portable range 1..9223372036854775807.",
		},
		"form_api_version": schema.StringAttribute{
			Computed:    true,
			Description: "Exact portable API version recorded when this state was created or imported.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"form_kind": schema.StringAttribute{
			Computed:    true,
			Description: "Exact Form kind recorded when this state was created or imported.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"form_definition_version": schema.StringAttribute{
			Computed:    true,
			Description: "Exact immutable Form definition version recorded in provider state.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"form_schema_digest": schema.StringAttribute{
			Computed:    true,
			Description: "Exact immutable Form schema digest recorded in provider state.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"form_package_digest": schema.StringAttribute{
			Computed:    true,
			Description: "Exact immutable Form Package digest recorded in provider state.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"drift_status": schema.StringAttribute{
			Computed:    true,
			Description: "Current or drifted, derived only from the exact Form-validated observed document.",
		},
		"portability": schema.StringAttribute{
			Computed:    true,
			Description: "Portability token accepted only when the exact Form-validated observed and output documents agree.",
		},
		"outputs": schema.MapAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "Exact Form-validated public output document projected to string values; undeclared host keys are rejected.",
		},
	}
}

func artifactSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"artifact_url": schema.StringAttribute{
			Required:    true,
			Description: "Absolute credential-free HTTPS URL from which any conforming host can fetch the prebuilt artifact; userinfo, query, and fragment are forbidden because this value persists in nonsensitive state.",
			Validators:  []validator.String{StringMatches(formcatalog.PatternCredentialFreeHTTPSURL, "artifact_url must be an absolute credential-free https URL without userinfo, query, or fragment")},
		},
		"artifact_sha256": schema.StringAttribute{
			Required:    true,
			Description: "Canonical sha256-prefixed lowercase digest binding artifact_url to exact immutable bytes.",
			Validators:  []validator.String{StringMatches(formcatalog.PatternCanonicalSHA256, formcatalog.GrammarCanonicalSHA256.Message("artifact_sha256"))},
		},
		"artifact_media_type": schema.StringAttribute{
			Required:    true,
			Description: "Lowercase IANA-style media type describing how a conforming host interprets the artifact bytes.",
			Validators:  []validator.String{StringMatches(formcatalog.PatternMediaType, "artifact_media_type must be a lowercase type/subtype token without parameters")},
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
	FormIdentity    formStateIdentity
	Fields          map[string]attr.Value
	Connections     types.List
	Artifact        artifactSourceValues
}

type formStateIdentity struct {
	APIVersion        types.String
	Kind              types.String
	DefinitionVersion types.String
	SchemaDigest      types.String
	PackageDigest     types.String
}

func (r *formResource) valuesFrom(ctx context.Context, getter interface {
	GetAttribute(context.Context, path.Path, any) diag.Diagnostics
}) (formValues, diag.Diagnostics) {
	var diags diag.Diagnostics
	values := formValues{Fields: map[string]attr.Value{}, Artifact: nullArtifactSourceValues()}
	diags.Append(getter.GetAttribute(ctx, path.Root("name"), &values.Name)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("space"), &values.Space)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("resource_version"), &values.ResourceVersion)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_api_version"), &values.FormIdentity.APIVersion)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_kind"), &values.FormIdentity.Kind)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_definition_version"), &values.FormIdentity.DefinitionVersion)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_schema_digest"), &values.FormIdentity.SchemaDigest)...)
	diags.Append(getter.GetAttribute(ctx, path.Root("form_package_digest"), &values.FormIdentity.PackageDigest)...)
	if r.kind.Connections != formcatalog.ConnectionsAbsent {
		diags.Append(getter.GetAttribute(ctx, path.Root("connections"), &values.Connections)...)
	}
	if r.kind.Artifact {
		diags.Append(getter.GetAttribute(ctx, path.Root("artifact_url"), &values.Artifact.URL)...)
		diags.Append(getter.GetAttribute(ctx, path.Root("artifact_sha256"), &values.Artifact.SHA256)...)
		diags.Append(getter.GetAttribute(ctx, path.Root("artifact_media_type"), &values.Artifact.MediaType)...)
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
		case formcatalog.TypeStringMap:
			var value types.Map
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
	spec := map[string]any{}
	if values.Name.IsUnknown() {
		diags.AddAttributeError(
			path.Root("name"),
			"Unknown "+r.kind.Kind+" field",
			"name must be wholly known before the provider can preview or apply this Form.",
		)
	} else if !values.Name.IsNull() {
		if name := strings.TrimSpace(values.Name.ValueString()); name != "" {
			spec["name"] = name
		}
	}
	if r.kind.Artifact {
		source, sourceDiags := values.Artifact.toSpec(r.kind.ResourceType)
		diags.Append(sourceDiags...)
		if source != nil {
			spec["source"] = source
		}
	}
	if r.kind.Connections != formcatalog.ConnectionsAbsent {
		if connections := resourceConnectionsToSpec(ctx, r.kind, values.Connections, &diags); len(connections) > 0 {
			spec["connections"] = connections
		}
	}
	for _, field := range r.kind.Fields {
		value, ok := values.Fields[field.HCL]
		if !ok || value == nil || value.IsNull() {
			continue
		}
		if value.IsUnknown() {
			diags.AddAttributeError(
				path.Root(field.HCL),
				"Unknown "+r.kind.Kind+" field",
				field.HCL+" must be wholly known before the provider can preview or apply this Form.",
			)
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
		case types.Map:
			whollyKnown := true
			for key, member := range typed.Elements() {
				if member == nil || member.IsNull() || member.IsUnknown() {
					diags.AddAttributeError(
						path.Root(field.HCL).AtMapKey(key),
						"Unknown or null "+r.kind.Kind+" map value",
						field.HCL+" must contain only wholly known, non-null values before the provider can preview or apply this Form.",
					)
					whollyKnown = false
				}
			}
			if !whollyKnown {
				continue
			}
			var entries map[string]string
			diags.Append(typed.ElementsAs(ctx, &entries, false)...)
			if len(entries) > 0 {
				spec[field.Wire] = entries
			}
		case types.Set:
			whollyKnown := true
			for _, member := range typed.Elements() {
				if member == nil || member.IsNull() || member.IsUnknown() {
					diags.AddAttributeError(
						path.Root(field.HCL),
						"Unknown or null "+r.kind.Kind+" set member",
						field.HCL+" must contain only wholly known, non-null values before the provider can preview or apply this Form.",
					)
					whollyKnown = false
					break
				}
			}
			if !whollyKnown {
				continue
			}
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
	if err := r.kind.ValidateDesired(spec); err != nil {
		diags.AddError(
			"Invalid "+r.kind.Kind+" desired state",
			"The planned values do not satisfy the exact desiredSchema compiled into this provider, so no host request was made: "+err.Error(),
		)
	}
	violations, err := r.kind.ConditionalViolations(spec)
	if err != nil {
		diags.AddError("Invalid Form constraint", err.Error())
	} else {
		for _, violation := range violations {
			attribute := camelToSnake(violation.WireField)
			if field, ok := fieldByWire(r.kind, violation.WireField); ok {
				attribute = field.HCL
			}
			diags.AddAttributeError(
				path.Root(attribute),
				"Invalid "+r.kind.Kind+" field combination",
				violation.Detail+".",
			)
		}
	}
	space, err := validatedEffectiveSpace(values.Space, r.data.defaultSpace)
	if err != nil {
		diags.AddAttributeError(
			path.Root("space"),
			"Invalid or missing SpaceID",
			"Set a valid resource SpaceID or configure a valid provider default: "+err.Error(),
		)
	}
	if diags.HasError() {
		return nil, space, diags
	}
	name, _ := spec["name"].(string)
	body := &client.Resource{
		APIVersion: client.APIVersion, Kind: r.kind.Kind,
		Metadata: client.Metadata{Name: name, Space: space},
		Spec:     spec,
	}
	return body, space, diags
}

func (r *formResource) assertConfigured(diags *diag.Diagnostics) bool {
	if r.data != nil && r.data.client == nil && r.data.v2Err != nil {
		// The provider configured, but only the v1beta1 lane negotiated.
		// Surface the recorded v1alpha2 negotiation error for this v2-lane
		// resource instead of a generic wiring failure.
		diags.AddError(
			"Takoform v1alpha2 lane unavailable",
			"The configured endpoint did not negotiate the Host API v1alpha2 lane required by "+
				r.kind.ResourceType+". "+r.data.v2Err.Error(),
		)
		return false
	}
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

func (r *formResource) assertStateFormIdentity(values formValues, diags *diag.Diagnostics) bool {
	got, ok := values.FormIdentity.reference()
	if !ok {
		diags.AddError(
			"State has no exact Form identity",
			"Provider v2 will not guess a Form identity for older state. Pin the exact provider version that wrote the state and perform an explicit create/import migration. Provider v1 remains the Legacy v1alpha1 client.",
		)
		return false
	}
	// The FormRef is the identity fence; the packageDigest recorded in state
	// is distribution provenance, not contract identity (a host may serve the
	// same FormRef from a re-packaged distribution). Membership in the
	// supported set — not equality with the current default create target —
	// decides Read/Update/Delete, so a future Form line bump keeps existing
	// state readable instead of stranding it on an old provider pin. When the
	// supported set is unpopulated (partial constructions), the defaults are
	// the supported set, preserving the strict single-line behavior.
	supported := r.data.supported
	if len(supported) == 0 {
		for _, form := range r.data.forms {
			supported = append(supported, form)
		}
	}
	for _, candidate := range supported {
		if candidate.FormRef == got.FormRef {
			return true
		}
	}
	defaultRef := r.data.forms[r.kind.Kind]
	diags.AddError(
		"State Form identity is not supported by this provider",
		fmt.Sprintf(
			"State is bound to %s; this provider supports %s for kind %s. "+
				"Pin the provider version that wrote the state or perform an explicit create/import migration; "+
				"the provider will not query a different exact FormRef and interpret a 404 as deletion.",
			formatFormIdentity(got), formatFormIdentity(defaultRef), r.kind.Kind,
		),
	)
	return false
}

func (identity formStateIdentity) reference() (client.InstalledFormReference, bool) {
	values := []types.String{
		identity.APIVersion,
		identity.Kind,
		identity.DefinitionVersion,
		identity.SchemaDigest,
		identity.PackageDigest,
	}
	for _, value := range values {
		if value.IsNull() || value.IsUnknown() || strings.TrimSpace(value.ValueString()) == "" {
			return client.InstalledFormReference{}, false
		}
	}
	return client.InstalledFormReference{
		FormRef: client.FormRef{
			APIVersion:        identity.APIVersion.ValueString(),
			Kind:              identity.Kind.ValueString(),
			DefinitionVersion: identity.DefinitionVersion.ValueString(),
			SchemaDigest:      identity.SchemaDigest.ValueString(),
		},
		PackageDigest: identity.PackageDigest.ValueString(),
	}, true
}

func formatFormIdentity(identity client.InstalledFormReference) string {
	return fmt.Sprintf(
		"%s/%s@%s schema=%s package=%s",
		identity.FormRef.APIVersion,
		identity.FormRef.Kind,
		identity.FormRef.DefinitionVersion,
		identity.FormRef.SchemaDigest,
		identity.PackageDigest,
	)
}

// ModifyPlan surfaces the host's reviewed preview at plan time, so a user's
// `terraform plan` includes host validation instead of discovering it only at
// apply. The preview is a read-only validation fence: the framework release
// pinned here does not carry plan private state into apply, so apply performs
// its own authoritative review. A host that is unavailable or rejects the
// planned desired state produces a warning and apply reports the error.
func (r *formResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if !r.assertConfigured(&resp.Diagnostics) {
		return
	}
	// Destroy plans have no desired spec to review.
	if req.Plan.Raw.IsNull() {
		return
	}
	// Plans built from interpolated/unknown configuration cannot be reviewed
	// yet; apply reviews the resolved values.
	if !req.Config.Raw.IsFullyKnown() {
		return
	}
	values, diags := r.valuesFrom(ctx, req.Plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, space, bodyDiags := r.toResource(ctx, values)
	resp.Diagnostics.Append(bodyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	form := r.data.forms[r.kind.Kind]
	body.Form = &form
	if !values.ResourceVersion.IsNull() && !values.ResourceVersion.IsUnknown() {
		body.Metadata.ResourceVersion = values.ResourceVersion.ValueString()
	}
	operation := "create"
	if !req.State.Raw.IsNull() {
		operation = "update"
	}
	previewCtx, cancel := context.WithTimeout(ctx, planPreviewTimeout)
	defer cancel()
	if err := r.data.client.EnsureFormAvailable(previewCtx, space, form, operation); err != nil {
		resp.Diagnostics.AddWarning(
			"Takoform host preview unavailable",
			"The host could not confirm the exact Form for this plan: "+err.Error()+" Apply will re-validate.",
		)
		return
	}
	if _, err := r.data.client.PreviewResource(previewCtx, body); err != nil {
		resp.Diagnostics.AddWarning(
			"Takoform host preview rejected the plan",
			"The host reviewed the planned desired state and rejected it: "+err.Error()+" Apply will report the authoritative error.",
		)
	}
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
	stateValues, stateDiags := r.valuesFrom(ctx, req.State)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() || !r.assertStateFormIdentity(stateValues, &resp.Diagnostics) {
		return
	}
	if values.ResourceVersion.IsNull() || values.ResourceVersion.IsUnknown() {
		values.ResourceVersion = stateValues.ResourceVersion
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
	res, err := r.data.client.PutResource(ctx, r.kind.Kind, body.Metadata.Name, body)
	if err != nil {
		diags.AddError("Failed to apply "+r.kind.Kind, err.Error())
		return
	}
	diags.Append(r.setState(ctx, state, body.Metadata.Name, body.Spec, res, space, values, false)...)
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
	if !r.assertStateFormIdentity(values, &resp.Diagnostics) {
		return
	}
	space, err := validatedEffectiveSpace(values.Space, r.data.defaultSpace)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("space"),
			"Invalid or missing SpaceID",
			"Import as SPACE/NAME or configure a valid provider SpaceID before reading this resource: "+err.Error(),
		)
		return
	}
	form := r.data.forms[r.kind.Kind]
	res, currentSpec, err := observeResourceForRead(ctx, r.data.client, r.kind.Kind, values.Name.ValueString(), space, form)
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
	resp.Diagnostics.Append(r.setState(
		ctx,
		&resp.State,
		values.Name.ValueString(),
		currentSpec,
		res,
		space,
		values,
		true,
	)...)
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
	if !r.assertStateFormIdentity(values, &resp.Diagnostics) {
		return
	}
	space, err := validatedEffectiveSpace(values.Space, r.data.defaultSpace)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("space"),
			"Invalid or missing SpaceID",
			"Configure a valid provider SpaceID before deleting this resource: "+err.Error(),
		)
		return
	}
	form := r.data.forms[r.kind.Kind]
	if err := r.data.client.DeleteResource(ctx, r.kind.Kind, values.Name.ValueString(), space,
		client.MutationFence{ResourceVersion: values.ResourceVersion.ValueString(), Form: form}); err != nil {
		resp.Diagnostics.AddError("Failed to delete "+r.kind.Kind, err.Error())
	}
}

func (r *formResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !r.assertConfigured(&resp.Diagnostics) {
		return
	}
	space, name, err := splitImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(name))...)
	if space != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space"), types.StringValue(space))...)
	}
	resp.Diagnostics.Append(setFormIdentityState(ctx, &resp.State, r.data.forms[r.kind.Kind])...)
}

// setState writes an exact Form-validated portable projection into Terraform
// state. Create/update responses must repeat the configured spec; Read
// observations must repeat the exact current spec returned by the preceding
// generation-fenced GET. The provider synthesizes canonical identity locally
// and never persists arbitrary host desired state or status.
func (r *formResource) setState(
	ctx context.Context,
	state *tfsdk.State,
	expectedName string,
	expectedSpec map[string]any,
	res *client.Resource,
	space string,
	values formValues,
	refresh bool,
) diag.Diagnostics {
	var diags diag.Diagnostics
	projection, err := validatePortableStateProjection(r.kind, expectedName, expectedSpec, res)
	if err != nil {
		diags.AddError(
			"Host returned invalid portable Resource",
			"The Resource was not written to state because its desired/observed/output documents did not satisfy the exact "+
				r.kind.Kind+" Form contract: "+err.Error(),
		)
		return diags
	}
	diags.Append(state.SetAttribute(ctx, path.Root("name"), types.StringValue(res.Metadata.Name))...)
	diags.Append(state.SetAttribute(ctx, path.Root("space"), types.StringValue(space))...)
	diags.Append(state.SetAttribute(ctx, path.Root("id"), types.StringValue(projection.ID))...)
	diags.Append(state.SetAttribute(ctx, path.Root("resource_version"), optionalString(res.Metadata.ResourceVersion))...)
	diags.Append(setFormIdentityState(ctx, state, r.data.forms[r.kind.Kind])...)

	outputs, outputDiags := types.MapValueFrom(ctx, types.StringType, projection.Outputs)
	diags.Append(outputDiags...)
	diags.Append(state.SetAttribute(ctx, path.Root("drift_status"), types.StringValue(projection.DriftStatus))...)
	diags.Append(state.SetAttribute(ctx, path.Root("portability"), types.StringValue(projection.Portability))...)
	diags.Append(state.SetAttribute(ctx, path.Root("outputs"), outputs)...)

	if r.kind.Artifact {
		source := artifactSourceValuesFromSpec(res.Spec["source"])
		if refresh || values.Artifact.URL.IsNull() {
			values.Artifact = source
		}
		diags.Append(state.SetAttribute(ctx, path.Root("artifact_url"), values.Artifact.URL)...)
		diags.Append(state.SetAttribute(ctx, path.Root("artifact_sha256"), values.Artifact.SHA256)...)
		diags.Append(state.SetAttribute(ctx, path.Root("artifact_media_type"), values.Artifact.MediaType)...)
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

func setFormIdentityState(ctx context.Context, state *tfsdk.State, identity client.InstalledFormReference) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.Append(state.SetAttribute(ctx, path.Root("form_api_version"), types.StringValue(identity.FormRef.APIVersion))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_kind"), types.StringValue(identity.FormRef.Kind))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_definition_version"), types.StringValue(identity.FormRef.DefinitionVersion))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_schema_digest"), types.StringValue(identity.FormRef.SchemaDigest))...)
	diags.Append(state.SetAttribute(ctx, path.Root("form_package_digest"), types.StringValue(identity.PackageDigest))...)
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
	case formcatalog.TypeStringMap:
		if raw == nil {
			return types.MapNull(types.StringType)
		}
		mapped, mapDiags := types.MapValueFrom(ctx, types.StringType, toStringMap(raw))
		diags.Append(mapDiags...)
		return mapped
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

func splitImportID(id string) (space, name string, err error) {
	if index := strings.Index(id, "/"); index >= 0 {
		space, name = id[:index], id[index+1:]
		if validationErr := client.ValidateSpaceID(space); validationErr != nil {
			return "", "", fmt.Errorf("import ID SpaceID is invalid: %w", validationErr)
		}
	} else {
		name = id
	}
	if !portableNamePattern.MatchString(name) {
		return "", "", fmt.Errorf(
			"import ID Resource name %q does not match the canonical PatternName grammar",
			name,
		)
	}
	return space, name, nil
}
