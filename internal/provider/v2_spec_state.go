package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/tako0614/terraform-provider-takoform/internal/edgev2"
	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
)

func v2WireName(hcl string) string {
	if hcl == "vars_json" {
		return "vars"
	}
	parts := strings.Split(hcl, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

func v2SpecFromPlan(ctx context.Context, plan tfsdk.Plan, def v2FormDef) (map[string]any, error) {
	if def.spec == nil {
		return map[string]any{}, nil
	}
	var value types.Object
	if diags := plan.GetAttribute(ctx, path.Root("spec"), &value); diags.HasError() {
		return nil, errors.New("typed spec could not be decoded")
	}
	if value.IsNull() || value.IsUnknown() {
		return nil, errors.New("typed spec must be known")
	}
	result, err := v2ObjectToWire(value, def.spec)
	if err != nil {
		return nil, err
	}
	encoded, err := jsonv2.Marshal(result)
	if err != nil {
		return nil, errors.New("typed spec cannot be encoded as strict JSON")
	}
	if _, err := edgev2.Decode(def.form, encoded); err != nil {
		return nil, fmt.Errorf("typed public spec does not match the exact Form: %w", err)
	}
	return result, nil
}

func v2ObjectToWire(value types.Object, attrs map[string]schema.Attribute) (map[string]any, error) {
	out := make(map[string]any)
	for key, attribute := range attrs {
		item := value.Attributes()[key]
		if item == nil || item.IsUnknown() {
			return nil, fmt.Errorf("spec.%s must be wholly known", key)
		}
		if item.IsNull() {
			continue
		}
		if key == "vars_json" {
			str, ok := item.(types.String)
			if !ok {
				return nil, errors.New("vars_json must be a string")
			}
			parsed, err := jsonv2.Decode([]byte(str.ValueString()))
			if err != nil {
				return nil, errors.New("vars_json must be strict JSON")
			}
			obj, ok := parsed.(map[string]any)
			if !ok {
				return nil, errors.New("vars_json must be a JSON object")
			}
			out["vars"] = obj
			continue
		}
		wire, err := v2ValueToWire(item, attribute)
		if err != nil {
			return nil, fmt.Errorf("spec.%s: %w", key, err)
		}
		out[v2WireName(key)] = wire
	}
	return out, nil
}

func v2ValueToWire(value attr.Value, attribute schema.Attribute) (any, error) {
	if value.IsUnknown() || value.IsNull() {
		return nil, errors.New("required value is unknown or null")
	}
	switch v := value.(type) {
	case types.String:
		return v.ValueString(), nil
	case types.Int64:
		return json.Number(fmt.Sprintf("%d", v.ValueInt64())), nil
	case types.Bool:
		return v.ValueBool(), nil
	case types.Object:
		nested, ok := attribute.(schema.SingleNestedAttribute)
		if !ok {
			return nil, errors.New("unexpected nested object")
		}
		return v2ObjectToWire(v, nested.Attributes)
	case types.List:
		out := make([]any, 0, len(v.Elements()))
		for _, item := range v.Elements() {
			var itemAttribute schema.Attribute
			if nested, ok := attribute.(schema.ListNestedAttribute); ok {
				itemAttribute = schema.SingleNestedAttribute{Attributes: nested.NestedObject.Attributes}
			} else {
				itemAttribute = schema.StringAttribute{}
			}
			converted, err := v2ValueToWire(item, itemAttribute)
			if err != nil {
				return nil, err
			}
			out = append(out, converted)
		}
		return out, nil
	default:
		return nil, errors.New("unsupported typed spec attribute")
	}
}

func v2StateSpec(ctx context.Context, def v2FormDef, spec map[string]any) (attr.Value, error) {
	encoded, err := jsonv2.Marshal(spec)
	if err != nil {
		return nil, errors.New("Host spec is not strict JSON")
	}
	if _, err := edgev2.Decode(def.form, encoded); err != nil {
		return nil, errors.New("Host spec does not match the exact Form")
	}
	if def.spec == nil {
		return nil, nil
	}
	return v2ObjectFromWire(ctx, def.spec, spec)
}

func v2ObjectFromWire(ctx context.Context, attrs map[string]schema.Attribute, wire map[string]any) (types.Object, error) {
	attrTypes := make(map[string]attr.Type, len(attrs))
	values := make(map[string]attr.Value, len(attrs))
	for key, schemaAttr := range attrs {
		attrTypes[key] = schemaAttr.GetType()
		item, exists := wire[v2WireName(key)]
		if !exists {
			values[key] = v2Null(schemaAttr.GetType())
			continue
		}
		if key == "vars_json" {
			encoded, err := jsonv2.Marshal(item)
			if err != nil {
				return types.Object{}, err
			}
			values[key] = types.StringValue(string(encoded))
			continue
		}
		converted, err := v2ValueFromWire(ctx, schemaAttr, item)
		if err != nil {
			return types.Object{}, fmt.Errorf("Host spec field %s is invalid: %w", key, err)
		}
		values[key] = converted
	}
	object, diags := types.ObjectValue(attrTypes, values)
	if diags.HasError() {
		return types.Object{}, errors.New("Host spec cannot enter typed state")
	}
	return object, nil
}

func v2ValueFromWire(ctx context.Context, attribute schema.Attribute, wire any) (attr.Value, error) {
	switch typed := attribute.(type) {
	case schema.StringAttribute:
		value, ok := wire.(string)
		if !ok {
			return nil, errors.New("expected string")
		}
		return types.StringValue(value), nil
	case schema.Int64Attribute:
		number, ok := wire.(json.Number)
		if !ok {
			return nil, errors.New("expected integer")
		}
		value, err := number.Int64()
		if err != nil {
			return nil, err
		}
		return types.Int64Value(value), nil
	case schema.BoolAttribute:
		value, ok := wire.(bool)
		if !ok {
			return nil, errors.New("expected boolean")
		}
		return types.BoolValue(value), nil
	case schema.SingleNestedAttribute:
		object, ok := wire.(map[string]any)
		if !ok {
			return nil, errors.New("expected object")
		}
		return v2ObjectFromWire(ctx, typed.Attributes, object)
	case schema.ListAttribute:
		list, ok := wire.([]any)
		if !ok {
			return nil, errors.New("expected list")
		}
		values := make([]attr.Value, 0, len(list))
		for _, item := range list {
			value, ok := item.(string)
			if !ok {
				return nil, errors.New("expected string list")
			}
			values = append(values, types.StringValue(value))
		}
		result, diags := types.ListValue(typed.ElementType, values)
		if diags.HasError() {
			return nil, errors.New("invalid list")
		}
		return result, nil
	case schema.ListNestedAttribute:
		list, ok := wire.([]any)
		if !ok {
			return nil, errors.New("expected nested list")
		}
		values := make([]attr.Value, 0, len(list))
		for _, item := range list {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("expected object in list")
			}
			value, err := v2ObjectFromWire(ctx, typed.NestedObject.Attributes, object)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		listType, ok := typed.GetType().(basetypes.ListType)
		if !ok {
			return nil, errors.New("invalid nested list schema")
		}
		result, diags := types.ListValue(listType.ElemType, values)
		if diags.HasError() {
			return nil, errors.New("invalid nested list")
		}
		return result, nil
	default:
		return nil, errors.New("unsupported typed spec attribute")
	}
}

func v2Null(t attr.Type) attr.Value {
	switch typed := t.(type) {
	case basetypes.StringType:
		return types.StringNull()
	case basetypes.Int64Type:
		return types.Int64Null()
	case basetypes.BoolType:
		return types.BoolNull()
	case basetypes.ObjectType:
		return types.ObjectNull(typed.AttrTypes)
	case basetypes.ListType:
		return types.ListNull(typed.ElemType)
	default:
		panic("unsupported v2 schema type")
	}
}
