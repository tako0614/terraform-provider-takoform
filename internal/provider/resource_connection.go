package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

var resourceConnectionAttrTypes = map[string]attr.Type{
	"name":        types.StringType,
	"resource":    types.StringType,
	"permissions": types.SetType{ElemType: types.StringType},
	"projection":  types.StringType,
}

type resourceConnectionModel struct {
	Name        types.String `tfsdk:"name"`
	Resource    types.String `tfsdk:"resource"`
	Permissions types.Set    `tfsdk:"permissions"`
	Projection  types.String `tfsdk:"projection"`
}

func resourceConnectionAttribute(kind formcatalog.Kind) schema.ListNestedAttribute {
	required := kind.Connections == formcatalog.ConnectionsRequired
	return schema.ListNestedAttribute{
		Optional:    !required,
		Required:    required,
		Description: "Non-secret Service Form connection request metadata. Each Kind/name target resolves only in this Resource's exact space; portable connections cannot select another space. It grants nothing and the host may deny it; bindings, grants, projection materialization, token issuance, authorization, write fencing, and lifecycle are host-owned.",
		Validators:  []validator.List{resourceConnectionsValidator{kind: kind}},
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"name": schema.StringAttribute{
					Required:    true,
					Description: "Connection name as seen by the consumer runtime, for example ASSETS or DATABASE.",
					Validators:  []validator.String{StringMatches(portableConnectionNamePattern, "connection name must use the portable map-key grammar")},
				},
				"resource": schema.StringAttribute{
					Required:    true,
					Description: "Referenced Service Form resource as Kind/name, for example ObjectBucket/assets or Queue/jobs. The host resolves it only in the source Resource's exact space; this field cannot express cross-space selection.",
					Validators:  []validator.String{StringMatches(formcatalog.PatternResourceRef, "resource must be Kind/name using the portable resource-reference grammar")},
				},
				"permissions": schema.SetAttribute{
					Required:    true,
					ElementType: types.StringType,
					Description: "Open permission tokens requested by the consumer. They are not issued grants; the host may deny them.",
					Validators: []validator.Set{
						SetStringsMatch(1, portableCapabilityTokenPattern, "permission must use the portable capability-token grammar"),
					},
				},
				"projection": schema.StringAttribute{
					Required:    true,
					Description: "Open projection token requested by the consumer. Projection materialization remains host-owned and may be denied.",
					Validators:  []validator.String{StringMatches(portableCapabilityTokenPattern, "projection must use the portable capability-token grammar")},
				},
			},
		},
	}
}

type resourceConnectionsValidator struct {
	kind formcatalog.Kind
}

func (v resourceConnectionsValidator) Description(_ context.Context) string {
	if v.kind.MaxConnections > 0 {
		return fmt.Sprintf("%s requires between 1 and %d uniquely named connection(s)", v.kind.Kind, v.kind.MaxConnections)
	}
	if v.kind.Connections == formcatalog.ConnectionsRequired {
		return v.kind.Kind + " requires one or more uniquely named connections"
	}
	return "connection names must be unique"
}

func (v resourceConnectionsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v resourceConnectionsValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var items []resourceConnectionModel
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &items, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if v.kind.Connections == formcatalog.ConnectionsRequired && len(items) == 0 {
		resp.Diagnostics.AddAttributeError(req.Path, "Missing required connection", v.Description(ctx))
	}
	if v.kind.MaxConnections > 0 && len(items) > v.kind.MaxConnections {
		resp.Diagnostics.AddAttributeError(req.Path, "Too many connections", v.Description(ctx))
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.Name.IsNull() || item.Name.IsUnknown() {
			continue
		}
		name := item.Name.ValueString()
		if _, duplicate := seen[name]; duplicate {
			resp.Diagnostics.AddAttributeError(req.Path, "Duplicate connection name",
				"Each connection name must be unique; "+name+" is declared more than once.")
			continue
		}
		seen[name] = struct{}{}
	}
}

func resourceConnectionsToSpec(_ context.Context, kind formcatalog.Kind, value types.List, diags *diag.Diagnostics) map[string]any {
	if value.IsUnknown() {
		diags.AddAttributeError(
			path.Root("connections"),
			"Unknown "+kind.Kind+" connections",
			"connections must be wholly known before the provider can preview or apply this Form.",
		)
		return nil
	}
	if value.IsNull() {
		if kind.Connections == formcatalog.ConnectionsRequired {
			diags.AddAttributeError(path.Root("connections"), "Missing required connection",
				kind.Kind+" requires at least one connection.")
		}
		return nil
	}
	out := map[string]any{}
	for index, raw := range value.Elements() {
		itemPath := path.Root("connections").AtListIndex(index)
		if raw == nil || raw.IsNull() || raw.IsUnknown() {
			diags.AddAttributeError(
				itemPath,
				"Unknown or null "+kind.Kind+" connection",
				"Each connection object must be wholly known and non-null before the provider can preview or apply this Form.",
			)
			continue
		}
		item, ok := raw.(types.Object)
		if !ok {
			diags.AddAttributeError(itemPath, "Invalid "+kind.Kind+" connection", "Connection value is not an object.")
			continue
		}
		attributes := item.Attributes()
		name, nameOK := knownConnectionString(attributes["name"], itemPath.AtName("name"), "name", diags)
		resource, resourceOK := knownConnectionString(attributes["resource"], itemPath.AtName("resource"), "resource", diags)
		projection, projectionOK := knownConnectionString(attributes["projection"], itemPath.AtName("projection"), "projection", diags)
		permissions, permissionsOK := knownConnectionPermissions(attributes["permissions"], itemPath.AtName("permissions"), diags)
		if !nameOK || !resourceOK || !projectionOK || !permissionsOK {
			continue
		}
		if _, duplicate := out[name]; duplicate {
			diags.AddAttributeError(itemPath.AtName("name"), "Duplicate connection name",
				"Each connection name must be unique; "+name+" is declared more than once.")
			continue
		}
		sort.Strings(permissions)
		out[name] = map[string]any{
			"resource":    resource,
			"permissions": permissions,
			"projection":  projection,
		}
	}
	if kind.Connections == formcatalog.ConnectionsRequired && len(out) == 0 {
		diags.AddAttributeError(path.Root("connections"), "Missing required connection",
			kind.Kind+" requires at least one connection.")
	}
	if kind.MaxConnections > 0 && len(out) > kind.MaxConnections {
		diags.AddAttributeError(path.Root("connections"), "Too many connections",
			fmt.Sprintf("%s accepts at most %d connection(s), got %d.", kind.Kind, kind.MaxConnections, len(out)))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func knownConnectionString(value attr.Value, valuePath path.Path, name string, diags *diag.Diagnostics) (string, bool) {
	typed, ok := value.(types.String)
	if !ok || typed.IsNull() || typed.IsUnknown() {
		diags.AddAttributeError(
			valuePath,
			"Unknown or null connection "+name,
			"Connection "+name+" must be wholly known and non-null before the provider can preview or apply this Form.",
		)
		return "", false
	}
	if strings.TrimSpace(typed.ValueString()) == "" {
		diags.AddAttributeError(valuePath, "Empty connection "+name, "Connection "+name+" must not be empty.")
		return "", false
	}
	return typed.ValueString(), true
}

func knownConnectionPermissions(value attr.Value, valuePath path.Path, diags *diag.Diagnostics) ([]string, bool) {
	typed, ok := value.(types.Set)
	if !ok || typed.IsNull() || typed.IsUnknown() {
		diags.AddAttributeError(
			valuePath,
			"Unknown or null connection permissions",
			"Connection permissions must be wholly known and non-null before the provider can preview or apply this Form.",
		)
		return nil, false
	}
	permissions := make([]string, 0, len(typed.Elements()))
	for _, raw := range typed.Elements() {
		member, ok := raw.(types.String)
		if !ok || member.IsNull() || member.IsUnknown() {
			diags.AddAttributeError(
				valuePath,
				"Unknown or null connection permission",
				"Every connection permission must be wholly known and non-null before the provider can preview or apply this Form.",
			)
			return nil, false
		}
		if strings.TrimSpace(member.ValueString()) == "" {
			diags.AddAttributeError(valuePath, "Empty connection permission", "Connection permissions must not be empty.")
			return nil, false
		}
		permissions = append(permissions, member.ValueString())
	}
	return permissions, true
}

func resourceConnectionsFromSpec(ctx context.Context, raw any) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	if raw == nil {
		return types.ListNull(types.ObjectType{AttrTypes: resourceConnectionAttrTypes}), diags
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return types.ListNull(types.ObjectType{AttrTypes: resourceConnectionAttrTypes}), diags
	}
	values := make([]attr.Value, 0, len(rawMap))
	names := make([]string, 0, len(rawMap))
	for name := range rawMap {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		item := rawMap[name]
		spec, ok := item.(map[string]any)
		if !ok {
			continue
		}
		permissions := toStringSlice(spec["permissions"])
		sort.Strings(permissions)
		permissionValues := []attr.Value{}
		for _, permission := range permissions {
			permissionValues = append(permissionValues, types.StringValue(permission))
		}
		permissionSet, d := types.SetValue(types.StringType, permissionValues)
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(types.ObjectType{AttrTypes: resourceConnectionAttrTypes}), diags
		}
		value, d := types.ObjectValue(resourceConnectionAttrTypes, map[string]attr.Value{
			"name":        types.StringValue(name),
			"resource":    types.StringValue(stringFromAny(spec["resource"])),
			"permissions": permissionSet,
			"projection":  types.StringValue(stringFromAny(spec["projection"])),
		})
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(types.ObjectType{AttrTypes: resourceConnectionAttrTypes}), diags
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return types.ListNull(types.ObjectType{AttrTypes: resourceConnectionAttrTypes}), diags
	}
	value, d := types.ListValue(types.ObjectType{AttrTypes: resourceConnectionAttrTypes}, values)
	diags.Append(d...)
	return value, diags
}

func stringFromAny(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
