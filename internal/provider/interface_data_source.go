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
	FormKind     types.String `tfsdk:"form_kind"`
}

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
				MarkdownDescription: "Declared interface name, for example `mcp.server`.",
			},
			"space": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Space to read from. Defaults to the provider's space.",
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
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{StringToken()},
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
	if d.data == nil || d.data.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The takoform provider is not configured.")
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
	space := effectiveSpace(config.Space, d.data.defaultSpace)
	declared, err := d.data.client.GetInterface(ctx, space, client.InterfaceSelector{
		Name: config.Name.ValueString(), Version: requestedVersion,
		ResourceKind: resourceKind, ResourceName: resourceName,
	})
	if err != nil {
		switch {
		case errors.Is(err, client.ErrInterfaceDeclarationsUnsupported):
			resp.Diagnostics.AddError("Host does not declare interfaces", "This host does not advertise features.interface_declarations.")
		case errors.Is(err, client.ErrInterfaceIdentityAmbiguous):
			resp.Diagnostics.AddError("Interface version is ambiguous", err.Error()+"; configure version explicitly.")
		case errors.Is(err, client.ErrInterfaceInstanceAmbiguous):
			resp.Diagnostics.AddError("Interface Resource is ambiguous", err.Error()+"; configure resource_kind and resource_name explicitly.")
		case errors.Is(err, client.ErrNotFound):
			identity := config.Name.ValueString()
			if requestedVersion != "" {
				identity += "@" + requestedVersion
			}
			resp.Diagnostics.AddError("Interface not found", fmt.Sprintf("The host declares no interface %q in space %q.", identity, space))
		default:
			resp.Diagnostics.AddError("Unable to read interface", err.Error())
		}
		return
	}

	config.Version = types.StringValue(declared.Version)
	config.ResourceKind = types.StringValue(declared.Resource.Kind)
	config.ResourceName = types.StringValue(declared.Resource.Name)
	documentJSON, err := encodeInterfaceJSON(declared.Document)
	if err != nil {
		resp.Diagnostics.AddError("Unable to encode interface document", err.Error())
		return
	}
	config.DocumentJSON = types.StringValue(documentJSON)
	valuesJSON, err := encodeInterfaceJSON(declared.Values)
	if err != nil {
		resp.Diagnostics.AddError("Unable to encode interface values", err.Error())
		return
	}
	config.ValuesJSON = types.StringValue(valuesJSON)
	if declared.Form == nil {
		config.FormKind = types.StringNull()
	} else {
		config.FormKind = types.StringValue(declared.Form.FormRef.Kind)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
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
