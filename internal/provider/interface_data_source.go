package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

var (
	_ datasource.DataSource              = (*interfaceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*interfaceDataSource)(nil)
)

type interfaceDataSource struct {
	data *providerData
}

func NewInterfaceDataSource() datasource.DataSource {
	return &interfaceDataSource{}
}

type interfaceDataSourceModel struct {
	Name         types.String `tfsdk:"name"`
	Space        types.String `tfsdk:"space"`
	Version      types.String `tfsdk:"version"`
	ResourceKind types.String `tfsdk:"resource_kind"`
	ResourceName types.String `tfsdk:"resource_name"`
	DocumentJSON types.String `tfsdk:"document_json"`
	ValuesJSON   types.String `tfsdk:"values_json"`
	ResourceURI  types.String `tfsdk:"resource_uri"`
	FormKind     types.String `tfsdk:"form_kind"`
}

// interfaceDataSourceType is this data source's address as a practitioner
// writes it. A diagnostic names it so the reader knows which of the two lanes
// the message is about: this data source is a v1alpha2 consumer, and there is
// no v1beta1 interface read to fall back to.
const interfaceDataSourceType = "data.takoform_interface"

func (d *interfaceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_interface"
}

func (d *interfaceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one runtime interface declaration. This grants nothing: authorization and lifecycle stay with the host.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					StringMatches(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`, "name must use the portable interface-name grammar"),
				},
				MarkdownDescription: "Author-defined Interface name, for example `example.runtime`.",
			},
			"space": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					StringSpaceID(),
				},
				MarkdownDescription: "Exact opaque SpaceID to read from. Defaults to the provider's SpaceID.",
			},
			"version": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					StringMatches(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`, "version must use the author-defined interface-version token grammar"),
				},
				MarkdownDescription: "Exact author-defined interface version. It may be omitted only when the visible name has exactly one version; ambiguity fails closed.",
			},
			"resource_kind": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{StringToken()},
				MarkdownDescription: "Portable Resource kind exposing the interface. Configure together with resource_name; omission succeeds only for one visible instance.",
			},
			"resource_name": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					StringMatches(formcatalog.PatternName, "resource_name must use the canonical portable Resource name grammar"),
				},
				MarkdownDescription: "Portable Resource name exposing the interface. Configure together with resource_kind; omission succeeds only for one visible instance.",
			},
			"document_json": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Exact non-secret declaration document, encoded as JSON.",
			},
			"values_json": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved public values, encoded as JSON. Credentials never appear here.",
			},
			"resource_uri": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Host-resolved credential-free HTTPS endpoint for this Interface, when the host reports one. This is runtime location, not Resource state or an authorization grant.",
			},
			"form_kind": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Kind of the exact Form that declared this interface, when reported by the host.",
			},
		},
	}
}

func (d *interfaceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *providerData, got %T. This is a provider bug.", req.ProviderData))
		return
	}
	d.data = data
}

func (d *interfaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// The two nil cases are structurally different and must not share a
	// diagnostic. A nil providerData is a provider bug; a nil v1alpha2 client
	// after a successful Configure is a fact about the ENDPOINT — the v1beta1
	// lane negotiated and this data source's lane did not — and the recorded
	// per-lane error is the only thing that tells the reader which to change.
	// The v2 form resources and the v3 resources both already report it this
	// way; this data source used to answer "Provider not configured" and throw
	// `v2Err` away.
	if d.data == nil {
		resp.Diagnostics.Append(v3Diagnostic{
			Summary:      "Provider not configured",
			ResourceType: interfaceDataSourceType,
			Code:         v3CodeNotConfigured,
			Detail:       "The takoform provider was not configured before use.",
			Repair: "This is a provider bug rather than a configuration fault. Report it with the data source " +
				"name above and the CLI version.",
		}.error())
		return
	}
	if d.data.client == nil {
		resp.Diagnostics.Append(v3LaneDiagnostic(interfaceDataSourceType, "v1alpha2", d.data.v2Err))
		return
	}
	var config interfaceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if field := unknownInterfaceReadField(config); field != "" {
		resp.Diagnostics.AddError(
			"Unknown interface selector",
			fmt.Sprintf("%s must be known before the interface data source can perform a read.", field),
		)
		return
	}
	resourceKind, resourceName := "", ""
	if !config.ResourceKind.IsNull() {
		resourceKind = config.ResourceKind.ValueString()
	}
	if !config.ResourceName.IsNull() {
		resourceName = config.ResourceName.ValueString()
	}
	if (resourceKind == "") != (resourceName == "") {
		resp.Diagnostics.AddError("Incomplete Resource selector", "resource_kind and resource_name must be configured together.")
		return
	}
	requestedVersion := ""
	if !config.Version.IsNull() {
		requestedVersion = config.Version.ValueString()
	}
	space, err := effectiveInterfaceSpace(config.Space, d.data.defaultSpace)
	if err != nil {
		resp.Diagnostics.AddError(
			"Missing Interface Space",
			"Set the interface data source `space` attribute or configure a non-empty provider default Space. No host request was made.",
		)
		return
	}
	config.Space = types.StringValue(space)
	declared, err := d.data.client.GetInterface(ctx, space, client.InterfaceSelector{
		Name: config.Name.ValueString(), Version: requestedVersion,
		ResourceKind: resourceKind, ResourceName: resourceName,
	})
	if err != nil {
		identity := config.Name.ValueString()
		if requestedVersion != "" {
			identity += "@" + requestedVersion
		}
		base := v3Diagnostic{
			ResourceType: interfaceDataSourceType,
			Space:        space,
			Name:         identity,
			Pointer:      "/interfaces/" + config.Name.ValueString(),
			Cause:        err,
		}
		switch {
		case errors.Is(err, client.ErrInterfaceDeclarationsUnsupported):
			base.Summary = "Host does not declare interfaces"
			base.Code = v3CodeInterfaceUnsupported
			base.Detail = "This host does not advertise features.interface_declarations, so it publishes no " +
				"Interface Declarations at all. No interface was read."
			base.Repair = "Remove this data source, or point the provider at a host that advertises " +
				"features.interface_declarations."
		case errors.Is(err, client.ErrInterfaceIdentityAmbiguous):
			base.Summary = "Interface version is ambiguous"
			base.Code = v3CodeCapabilityUnsupported
			base.Repair = "Configure `version` explicitly."
		case errors.Is(err, client.ErrInterfaceInstanceAmbiguous):
			base.Summary = "Interface Resource is ambiguous"
			base.Code = v3CodeCapabilityUnsupported
			base.Repair = "Configure `resource_kind` and `resource_name` explicitly."
		case errors.Is(err, client.ErrNotFound):
			base.Summary = "Interface not found"
			base.Code = v3CodeHostResponseInvalid
			base.Detail = fmt.Sprintf("The host declares no interface %q in space %q.", identity, space)
			base.Repair = "Create or make Ready the resource that materializes this declaration, then re-run."
		default:
			base.Summary = "Unable to read interface"
			resp.Diagnostics.Append(v3HostCallDiagnostic(base.Summary, err, base))
			return
		}
		resp.Diagnostics.Append(base.error())
		return
	}

	config.Version = types.StringValue(declared.Version)
	config.ResourceKind = types.StringValue(declared.Resource.Kind)
	config.ResourceName = types.StringValue(declared.Resource.Name)
	documentJSON, err := encodeInterfaceJSON(declared.Document)
	if err != nil {
		resp.Diagnostics.Append(v3Diagnostic{
			Summary:      "Unable to encode interface document",
			ResourceType: interfaceDataSourceType,
			Space:        space,
			Name:         declared.Name,
			Pointer:      "/document",
			Code:         v3CodeHostResponseInvalid,
			Cause:        err,
			Detail:       "The host's Interface Declaration document could not be re-encoded as JSON.",
			Repair:       "Report the document to the host operator; the provider will not record a partial one.",
		}.error())
		return
	}
	config.DocumentJSON = types.StringValue(documentJSON)
	valuesJSON, err := encodeInterfaceJSON(declared.Values)
	if err != nil {
		resp.Diagnostics.Append(v3Diagnostic{
			Summary:      "Unable to encode interface values",
			ResourceType: interfaceDataSourceType,
			Space:        space,
			Name:         declared.Name,
			Pointer:      "/values",
			Code:         v3CodeHostResponseInvalid,
			Cause:        err,
			Detail:       "The host's Interface Declaration values could not be re-encoded as JSON.",
			Repair:       "Report the declaration to the host operator; the provider will not record partial values.",
		}.error())
		return
	}
	config.ValuesJSON = types.StringValue(valuesJSON)
	config.ResourceURI = interfaceResourceURIValue(declared.ResourceURI)
	if declared.Form == nil {
		config.FormKind = types.StringNull()
	} else {
		config.FormKind = types.StringValue(declared.Form.FormRef.Kind)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func interfaceResourceURIValue(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func effectiveInterfaceSpace(value types.String, fallback string) (string, error) {
	space, err := validatedEffectiveSpace(value, fallback)
	if err != nil {
		return "", fmt.Errorf("interface SpaceID is invalid or missing: %w", err)
	}
	return space, nil
}

func unknownInterfaceReadField(config interfaceDataSourceModel) string {
	for _, candidate := range []struct {
		name  string
		value types.String
	}{
		{name: "name", value: config.Name},
		{name: "space", value: config.Space},
		{name: "version", value: config.Version},
		{name: "resource_kind", value: config.ResourceKind},
		{name: "resource_name", value: config.ResourceName},
	} {
		if candidate.value.IsUnknown() {
			return candidate.name
		}
	}
	return ""
}

func encodeInterfaceJSON(value map[string]any) (string, error) {
	if value == nil {
		return "{}", nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
