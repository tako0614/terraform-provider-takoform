package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

var (
	_ resource.Resource                = (*interfaceResource)(nil)
	_ resource.ResourceWithImportState = (*interfaceResource)(nil)
)

type interfaceResource struct {
	data *providerData
}

func NewInterfaceResource() resource.Resource {
	return &interfaceResource{}
}

type interfaceResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Version            types.String `tfsdk:"version"`
	Space              types.String `tfsdk:"space"`
	ResourceKind       types.String `tfsdk:"resource_kind"`
	ResourceName       types.String `tfsdk:"resource_name"`
	DocumentJSON       types.String `tfsdk:"document_json"`
	DocumentSchemaJSON types.String `tfsdk:"document_schema_json"`
	InputsJSON         types.String `tfsdk:"inputs_json"`
	ResourceURIInput   types.String `tfsdk:"resource_uri_input"`
	ValuesJSON         types.String `tfsdk:"values_json"`
	ResourceURI        types.String `tfsdk:"resource_uri"`
	ResourceVersion    types.String `tfsdk:"resource_version"`
}

func (r *interfaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_interface"
}

func (r *interfaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Declares one generic, data-only application interface. The host owns resolution and authorization; this resource has no protocol-specific behavior.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Portable compound address. This is not a host Interface record id.",
			},
			"name": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					StringMatches(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`, "name must use the portable interface-name grammar"),
				},
				PlanModifiers:       replace,
				MarkdownDescription: "Author-defined Interface name.",
			},
			"version": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					StringMatches(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`, "version must use the author-defined interface-version token grammar"),
				},
				PlanModifiers:       replace,
				MarkdownDescription: "Exact author-defined Interface version.",
			},
			"space": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "Space containing the exposing Resource. Defaults to the provider Space.",
			},
			"resource_kind": schema.StringAttribute{
				Required:            true,
				Validators:          []validator.String{StringToken()},
				PlanModifiers:       replace,
				MarkdownDescription: "Portable kind of the Resource exposing this Interface.",
			},
			"resource_name": schema.StringAttribute{
				Required:            true,
				Validators:          []validator.String{StringToken()},
				PlanModifiers:       replace,
				MarkdownDescription: "Portable name of the Resource exposing this Interface.",
			},
			"document_json": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact application-authored, non-secret JSON object. Takoform does not interpret its protocol.",
			},
			"document_schema_json": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional Draft 2020-12 JSON Schema for document_json.",
			},
			"inputs_json": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("[]"),
				MarkdownDescription: "JSON array of deterministic input declarations. No expressions, commands, network requests, or credentials.",
			},
			"resource_uri_input": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of the single resource_uri input, when the host should provide a canonical HTTPS resource URI.",
			},
			"values_json": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved public values. Credentials never appear here.",
			},
			"resource_uri": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved credential-free HTTPS resource URI, when declared.",
			},
			"resource_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Opaque optimistic-concurrency fence returned by the host.",
			},
		},
	}
}

func (r *interfaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *providerData, got %T. This is a provider bug.", req.ProviderData))
		return
	}
	r.data = data
}

func (r *interfaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan interfaceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, "", &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *interfaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if !r.ready(&resp.Diagnostics) {
		return
	}
	var state interfaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	selector, space, ok := interfaceIdentity(state, r.data.defaultSpace, &resp.Diagnostics)
	if !ok {
		return
	}
	declared, err := r.data.client.GetInterface(ctx, space, selector)
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Interface declaration", err.Error())
		return
	}
	if err := applyDeclaredInterface(&state, space, declared); err != nil {
		resp.Diagnostics.AddError("Invalid Interface declaration response", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *interfaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan interfaceResourceModel
	var state interfaceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, state.ResourceVersion.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *interfaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if !r.ready(&resp.Diagnostics) {
		return
	}
	var state interfaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	selector, space, ok := interfaceIdentity(state, r.data.defaultSpace, &resp.Diagnostics)
	if !ok {
		return
	}
	if err := r.data.client.DeleteInterface(ctx, space, selector, state.ResourceVersion.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete Interface declaration", err.Error())
	}
}

func (r *interfaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var identity []string
	if err := json.Unmarshal([]byte(req.ID), &identity); err != nil || len(identity) != 5 {
		resp.Diagnostics.AddError(
			"Invalid Interface import identity",
			`Use the resource id emitted by takoform_interface: ["space","resource kind","resource name","interface name","interface version"].`,
		)
		return
	}
	for index, value := range identity {
		if strings.TrimSpace(value) == "" {
			resp.Diagnostics.AddError("Invalid Interface import identity", fmt.Sprintf("Identity component %d is empty.", index))
			return
		}
	}
	for _, attribute := range []struct {
		path  string
		value string
	}{
		{path: "id", value: req.ID},
		{path: "space", value: identity[0]},
		{path: "resource_kind", value: identity[1]},
		{path: "resource_name", value: identity[2]},
		{path: "name", value: identity[3]},
		{path: "version", value: identity[4]},
	} {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(attribute.path), attribute.value)...)
	}
}

func (r *interfaceResource) write(ctx context.Context, model *interfaceResourceModel, resourceVersion string, diagnostics interfaceDiagnostics) {
	if !r.ready(diagnostics) {
		return
	}
	desired, space, err := desiredInterface(*model, r.data.defaultSpace, resourceVersion)
	if err != nil {
		diagnostics.AddError("Invalid Interface declaration", err.Error())
		return
	}
	declared, err := r.data.client.PutInterface(ctx, space, desired)
	if err != nil {
		if errors.Is(err, client.ErrInterfaceDeclarationWritesUnsupported) {
			diagnostics.AddError("Host does not accept Interface declarations", "This host does not advertise features.interface_declaration_writes.")
		} else {
			diagnostics.AddError("Unable to write Interface declaration", err.Error())
		}
		return
	}
	if err := applyDeclaredInterface(model, space, declared); err != nil {
		diagnostics.AddError("Invalid Interface declaration response", err.Error())
	}
}

// interfaceDiagnostics is the common subset used by framework diagnostic
// collections. Keeping the helper small avoids coupling lifecycle methods.
type interfaceDiagnostics interface {
	AddError(summary string, detail string)
}

func (r *interfaceResource) ready(diagnostics interfaceDiagnostics) bool {
	if r.data == nil || r.data.client == nil {
		diagnostics.AddError("Provider not configured", "The takoform provider is not configured.")
		return false
	}
	return true
}

func desiredInterface(model interfaceResourceModel, defaultSpace, resourceVersion string) (client.DeclaredInterface, string, error) {
	if field := unknownInterfaceResourceField(model); field != "" {
		return client.DeclaredInterface{}, "", fmt.Errorf("%s must be known before applying the declaration", field)
	}
	space := effectiveSpace(model.Space, defaultSpace)
	if strings.TrimSpace(space) == "" {
		return client.DeclaredInterface{}, "", errors.New("space must be configured on the resource or provider")
	}
	document, err := decodeJSONObject(model.DocumentJSON.ValueString(), "document_json")
	if err != nil {
		return client.DeclaredInterface{}, "", err
	}
	var documentSchema map[string]any
	if !model.DocumentSchemaJSON.IsNull() && model.DocumentSchemaJSON.ValueString() != "" {
		documentSchema, err = decodeJSONObject(model.DocumentSchemaJSON.ValueString(), "document_schema_json")
		if err != nil {
			return client.DeclaredInterface{}, "", err
		}
	}
	var inputs []formpackage.InterfaceInputDeclaration
	if err := json.Unmarshal([]byte(model.InputsJSON.ValueString()), &inputs); err != nil {
		return client.DeclaredInterface{}, "", fmt.Errorf("inputs_json must be a JSON array of Interface input declarations: %w", err)
	}
	resourceURIInput := ""
	if !model.ResourceURIInput.IsNull() {
		resourceURIInput = model.ResourceURIInput.ValueString()
	}
	return client.DeclaredInterface{
		Name: model.Name.ValueString(), Version: model.Version.ValueString(),
		Resource: client.InterfaceResourceRef{
			Kind: model.ResourceKind.ValueString(), Name: model.ResourceName.ValueString(),
		},
		Document: document, DocumentSchema: documentSchema, Inputs: inputs,
		ResourceURIInput: resourceURIInput, ResourceVersion: resourceVersion,
	}, space, nil
}

func interfaceIdentity(model interfaceResourceModel, defaultSpace string, diagnostics interfaceDiagnostics) (client.InterfaceSelector, string, bool) {
	if field := unknownInterfaceIdentityField(model); field != "" {
		diagnostics.AddError("Unknown Interface identity", field+" must be known before reading the declaration.")
		return client.InterfaceSelector{}, "", false
	}
	space := effectiveSpace(model.Space, defaultSpace)
	if strings.TrimSpace(space) == "" {
		diagnostics.AddError("Missing Interface Space", "Configure space on the resource or provider.")
		return client.InterfaceSelector{}, "", false
	}
	return client.InterfaceSelector{
		Name: model.Name.ValueString(), Version: model.Version.ValueString(),
		ResourceKind: model.ResourceKind.ValueString(), ResourceName: model.ResourceName.ValueString(),
	}, space, true
}

func unknownInterfaceIdentityField(model interfaceResourceModel) string {
	for _, candidate := range []struct {
		name  string
		value types.String
	}{
		{name: "name", value: model.Name},
		{name: "version", value: model.Version},
		{name: "space", value: model.Space},
		{name: "resource_kind", value: model.ResourceKind},
		{name: "resource_name", value: model.ResourceName},
	} {
		if candidate.value.IsUnknown() || candidate.value.IsNull() {
			return candidate.name
		}
	}
	return ""
}

func applyDeclaredInterface(model *interfaceResourceModel, space string, declared client.DeclaredInterface) error {
	documentJSON, err := encodeInterfaceJSON(declared.Document)
	if err != nil {
		return err
	}
	valuesJSON, err := encodeInterfaceJSON(declared.Values)
	if err != nil {
		return err
	}
	inputsJSON, err := json.Marshal(declared.Inputs)
	if err != nil {
		return err
	}
	model.Space = types.StringValue(space)
	model.Name = types.StringValue(declared.Name)
	model.Version = types.StringValue(declared.Version)
	model.ResourceKind = types.StringValue(declared.Resource.Kind)
	model.ResourceName = types.StringValue(declared.Resource.Name)
	model.DocumentJSON = preserveEquivalentJSON(model.DocumentJSON, documentJSON)
	if declared.DocumentSchema == nil {
		model.DocumentSchemaJSON = types.StringNull()
	} else {
		encoded, err := encodeInterfaceJSON(declared.DocumentSchema)
		if err != nil {
			return err
		}
		model.DocumentSchemaJSON = preserveEquivalentJSON(model.DocumentSchemaJSON, encoded)
	}
	model.InputsJSON = preserveEquivalentJSON(model.InputsJSON, string(inputsJSON))
	if declared.ResourceURIInput == "" {
		model.ResourceURIInput = types.StringNull()
	} else {
		model.ResourceURIInput = types.StringValue(declared.ResourceURIInput)
	}
	model.ValuesJSON = types.StringValue(valuesJSON)
	if declared.ResourceURI == "" {
		model.ResourceURI = types.StringNull()
	} else {
		if parsed, err := url.Parse(declared.ResourceURI); err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return errors.New("resource_uri is not an absolute HTTPS URI")
		}
		model.ResourceURI = types.StringValue(declared.ResourceURI)
	}
	model.ResourceVersion = types.StringValue(declared.ResourceVersion)
	id, _ := json.Marshal([]string{
		space, declared.Resource.Kind, declared.Resource.Name, declared.Name, declared.Version,
	})
	model.ID = types.StringValue(string(id))
	return nil
}

func preserveEquivalentJSON(current types.String, encoded string) types.String {
	if current.IsNull() || current.IsUnknown() || current.ValueString() == "" {
		return types.StringValue(encoded)
	}
	var currentValue any
	var encodedValue any
	if json.Unmarshal([]byte(current.ValueString()), &currentValue) == nil &&
		json.Unmarshal([]byte(encoded), &encodedValue) == nil {
		currentCanonical, currentErr := json.Marshal(currentValue)
		encodedCanonical, encodedErr := json.Marshal(encodedValue)
		if currentErr == nil && encodedErr == nil && string(currentCanonical) == string(encodedCanonical) {
			return current
		}
	}
	return types.StringValue(encoded)
}

func decodeJSONObject(raw, field string) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
		if err == nil {
			err = errors.New("top-level value is not an object")
		}
		return nil, fmt.Errorf("%s must be a JSON object: %w", field, err)
	}
	return value, nil
}

func unknownInterfaceResourceField(model interfaceResourceModel) string {
	for _, candidate := range []struct {
		name  string
		value types.String
	}{
		{name: "name", value: model.Name},
		{name: "version", value: model.Version},
		{name: "resource_kind", value: model.ResourceKind},
		{name: "resource_name", value: model.ResourceName},
		{name: "document_json", value: model.DocumentJSON},
		{name: "document_schema_json", value: model.DocumentSchemaJSON},
		{name: "inputs_json", value: model.InputsJSON},
		{name: "resource_uri_input", value: model.ResourceURIInput},
	} {
		if candidate.value.IsUnknown() {
			return candidate.name
		}
	}
	return ""
}
