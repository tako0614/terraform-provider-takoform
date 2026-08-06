package provider

// v3_resources.go derives the typed HCL surface of every Edge Platform
// Family resource from its single catalog declaration
// (internal/edgeformcatalog) and maps values between that surface and the
// portable v1alpha3 wire spec.
//
// Surface conventions of the v3 lane:
//   - resource references (worker, bundle, queue, ...) are the target's
//     resource NAME as a plain string; the wire carries the typed
//     {"kind": <TargetKind>, "name": ...} reference object.
//   - binding lists (kv_bindings, ...) are nested lists of
//     {name, target_name}; the wire element is
//     {"name": ..., "resource": {"kind": <TargetKind>, "name": target_name}}.
//     The wire key is `resource`, never `target`.
//   - the data-only vars map is authored as `vars_json`, an Optional string
//     holding one JSON object; it is parsed and sent as the wire object.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	workerBundleKind     = "WorkerBundle"
	workerDeploymentKind = "WorkerDeployment"

	// workerDeploymentWeightSum is the exact basis-point total every
	// WorkerDeployment versions list must reach.
	workerDeploymentWeightSum = 10000
)

// v3AttributeName maps one catalog field to its HCL attribute name. The
// data-only JSON map surfaces as <field>_json because its values are
// arbitrary bounded JSON, which HCL maps of strings cannot carry faithfully.
func v3AttributeName(field model.Field) string {
	if field.Kind == model.KindJSONMap {
		return field.HCL + "_json"
	}
	return field.HCL
}

func (r *v3FormResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := v3CommonAttributes(r.form.Role != model.RoleRevision)
	if r.form.Kind == workerBundleKind {
		for name, attribute := range workerBundleAttributes() {
			attrs[name] = attribute
		}
	} else {
		for _, field := range r.form.Fields {
			attrs[v3AttributeName(field)] = v3FieldAttribute(r.form, field)
		}
	}
	description := r.form.Description +
		" Role: " + string(r.form.Role) + " (Host API v1alpha3 lane)."
	resp.Schema = schema.Schema{
		Description: description,
		Attributes:  attrs,
	}
}

// v3CommonAttributes is the shared v3 state model: client-owned identity
// (name, space), host-owned identity and fences (uid, generation, revision),
// the exact Form identity, readiness, typed outputs, and operation timeouts.
// The packageDigest is deliberately NOT part of state identity; it is an
// audit-only computed attribute (spec/decisions/0011).
func v3CommonAttributes(hasUpdate bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
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
		"uid": schema.StringAttribute{
			Computed:    true,
			Description: "Host-issued immutable resource identity. Delete followed by re-create of the same name yields a new UID.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"generation": schema.StringAttribute{
			Computed:    true,
			Description: "Canonical decimal desired-state generation; increments only when the portable desired spec changes. Update fences carry it as the expected generation.",
		},
		"revision": schema.StringAttribute{
			Computed:    true,
			Description: "Canonical decimal representation revision; increments whenever the full representation changes. Deletes fence on it via If-Match.",
		},
		"ready": schema.BoolAttribute{
			Computed:    true,
			Description: "True when the host reports the closed Ready condition with status True.",
		},
		"outputs_json": schema.StringAttribute{
			Computed:    true,
			Description: "JSON-serialized status.outputs document of the last read or apply; \"{}\" when the Form publishes no outputs.",
		},
		"form_api_version": schema.StringAttribute{
			Computed:      true,
			Description:   "Exact namespaced Form group/version recorded in state; reads dispatch on this exact identity.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"form_kind": schema.StringAttribute{
			Computed:      true,
			Description:   "Exact Form kind recorded in state.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"form_definition_version": schema.StringAttribute{
			Computed:      true,
			Description:   "Exact immutable Form definition version recorded in state.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"form_schema_digest": schema.StringAttribute{
			Computed:      true,
			Description:   "Exact immutable Form schema digest recorded in state.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"form_package_digest": schema.StringAttribute{
			Computed: true,
			Description: "Audit-only digest of the Form Package this provider build carries for the exact FormRef. " +
				"It is distribution provenance, never resource identity, and never enters queries or fences.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"pending_operation_id": v3PendingOperationIDAttribute(),
		"create_timeout": schema.StringAttribute{
			Optional:    true,
			Description: "Overall create deadline as a Go duration (default \"20m\").",
			Validators:  []validator.String{v3DurationValidator{}},
		},
		"delete_timeout": schema.StringAttribute{
			Optional:    true,
			Description: "Overall delete deadline as a Go duration (default \"30m\").",
			Validators:  []validator.String{v3DurationValidator{}},
		},
	}
	if hasUpdate {
		attrs["update_timeout"] = schema.StringAttribute{
			Optional:    true,
			Description: "Overall update deadline as a Go duration (default \"20m\").",
			Validators:  []validator.String{v3DurationValidator{}},
		}
	}
	return attrs
}

// v3PendingOperationIDAttribute declares the internal-recovery-only record of
// an accepted-but-unfinished mutation. It is NOT part of resource identity and
// carries no host authority: it exists purely so a create the host ACCEPTED as
// a long-running Operation — but that did not reach a terminal state before
// the operation deadline — is not silently orphaned. State written on that path
// records the operation id here so the operator (and the next plan, which
// reconciles against the recorded name/space/FormRef) can see what to resume.
// Every successful read or apply clears it back to null.
func v3PendingOperationIDAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Computed: true,
		Description: "Internal recovery only. Host operation id of a mutation the host accepted but that did not " +
			"reach a terminal state before the operation deadline; null in steady state and cleared by the next " +
			"successful read or apply. It is not resource identity and configurations must not depend on it.",
	}
}

// v3FieldAttribute maps one declared catalog field to its framework schema
// attribute. Immutable fields — and every field of a revision-role Form —
// require replacement.
func v3FieldAttribute(form model.Form, field model.Field) schema.Attribute {
	replace := field.Immutable || form.Role == model.RoleRevision
	switch field.Kind {
	case model.KindBoolean:
		attribute := schema.BoolAttribute{
			Required: field.Required, Optional: !field.Required, Description: field.Doc,
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.Bool{boolplanmodifier.RequiresReplace()}
		}
		return attribute
	case model.KindInteger:
		var validators []validator.Int64
		if field.Min != nil {
			validators = append(validators, Int64AtLeast(*field.Min))
		}
		if field.Max != nil {
			validators = append(validators, Int64AtMost(*field.Max))
		}
		attribute := schema.Int64Attribute{
			Required: field.Required, Optional: !field.Required, Description: field.Doc,
			Validators: validators,
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
		}
		return attribute
	case model.KindStringSet:
		var validators []validator.Set
		if len(field.Enum) > 0 {
			validators = append(validators, SetStringsOneOf(field.MinItems, field.Enum...))
		} else {
			validators = append(validators, SetStringsMatch(
				field.MinItems, field.ItemPattern,
				field.HCL+" items must match "+field.ItemPattern,
			))
		}
		attribute := schema.SetAttribute{
			Required: field.Required, Optional: !field.Required, Description: field.Doc,
			ElementType: types.StringType, Validators: validators,
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.Set{setplanmodifier.RequiresReplace()}
		}
		return attribute
	case model.KindJSONMap:
		attribute := schema.StringAttribute{
			Optional: !field.Required, Required: field.Required,
			Description: field.Doc + " Authored as one JSON object string (for example jsonencode({...})); the provider sends the parsed object.",
			Validators:  []validator.String{v3JSONObjectValidator{}},
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
		}
		return attribute
	case model.KindResourceRef:
		attribute := schema.StringAttribute{
			Required: field.Required, Optional: !field.Required,
			Description: field.Doc + " Set the name of the target " + field.TargetKind + " resource.",
			Validators: []validator.String{StringMatches(
				model.PatternResourceName,
				field.HCL+" must be a portable resource name",
			)},
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
		}
		return attribute
	case model.KindBindingList:
		attribute := schema.ListNestedAttribute{
			Optional: !field.Required, Required: field.Required, Description: field.Doc,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Required:    true,
						Description: "JavaScript identifier the binding is projected under.",
						Validators: []validator.String{StringMatches(
							model.PatternBindingName,
							"binding name must be a JavaScript identifier",
						)},
					},
					"target_name": schema.StringAttribute{
						Required:    true,
						Description: "Name of the target " + field.TargetKind + " resource.",
						Validators: []validator.String{StringMatches(
							model.PatternResourceName,
							"target_name must be a portable resource name",
						)},
					},
				},
			},
			Validators: []validator.List{v3ListSizeValidator{minItems: field.MinItems, maxItems: field.MaxItems}},
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
		}
		return attribute
	case model.KindObjectList:
		nested := map[string]schema.Attribute{}
		for _, member := range field.Fields {
			nested[member.HCL] = v3NestedMemberAttribute(member)
		}
		validators := []validator.List{v3ListSizeValidator{minItems: field.MinItems, maxItems: field.MaxItems}}
		if form.Kind == workerDeploymentKind && field.Wire == "versions" {
			validators = append(validators, v3WeightSumValidator{})
		}
		attribute := schema.ListNestedAttribute{
			Optional: !field.Required, Required: field.Required, Description: field.Doc,
			NestedObject: schema.NestedAttributeObject{Attributes: nested},
			Validators:   validators,
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
		}
		return attribute
	default:
		// KindString, KindStringEnum, KindDateString.
		var validators []validator.String
		switch {
		case len(field.Enum) > 0:
			validators = append(validators, StringOneOf(field.Enum...))
		case field.Kind == model.KindDateString:
			validators = append(validators, StringMatches(model.PatternDate, field.HCL+" must be a YYYY-MM-DD calendar date"))
		case field.Pattern != "":
			validators = append(validators, StringMatches(field.Pattern, field.HCL+" must match "+field.Pattern))
		}
		attribute := schema.StringAttribute{
			Required: field.Required, Optional: !field.Required, Description: field.Doc,
			Validators: validators,
		}
		if replace {
			attribute.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
		}
		return attribute
	}
}

// v3NestedMemberAttribute maps one object-list member field. A nested
// resource reference flattens to the target's name string, mirroring the
// top-level convention.
func v3NestedMemberAttribute(member model.Field) schema.Attribute {
	switch member.Kind {
	case model.KindInteger:
		var validators []validator.Int64
		if member.Min != nil {
			validators = append(validators, Int64AtLeast(*member.Min))
		}
		if member.Max != nil {
			validators = append(validators, Int64AtMost(*member.Max))
		}
		return schema.Int64Attribute{
			Required: member.Required, Optional: !member.Required, Description: member.Doc,
			Validators: validators,
		}
	case model.KindResourceRef:
		return schema.StringAttribute{
			Required: member.Required, Optional: !member.Required,
			Description: member.Doc + " Set the name of the target " + member.TargetKind + " resource.",
			Validators: []validator.String{StringMatches(
				model.PatternResourceName,
				member.HCL+" must be a portable resource name",
			)},
		}
	case model.KindStringEnum:
		return schema.StringAttribute{
			Required: member.Required, Optional: !member.Required, Description: member.Doc,
			Validators: []validator.String{StringOneOf(member.Enum...)},
		}
	default:
		var validators []validator.String
		if member.Pattern != "" {
			validators = append(validators, StringMatches(member.Pattern, member.HCL+" must match "+member.Pattern))
		}
		return schema.StringAttribute{
			Required: member.Required, Optional: !member.Required, Description: member.Doc,
			Validators: validators,
		}
	}
}

// v3SpecFromValues projects planned values onto the portable wire spec. Only
// declared fields travel, and an unknown plan value never becomes desired
// state.
func (r *v3FormResource) v3SpecFromValues(ctx context.Context, values v3Values) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	if values.Name.IsUnknown() || values.Name.IsNull() {
		diags.AddAttributeError(
			path.Root("name"),
			"Unknown "+r.form.Kind+" name",
			"name must be wholly known before the provider can apply this Form.",
		)
	}
	spec := map[string]any{}
	for _, field := range r.form.Fields {
		name := v3AttributeName(field)
		value, ok := values.Fields[name]
		if !ok || value == nil || value.IsNull() {
			continue
		}
		if value.IsUnknown() {
			diags.AddAttributeError(
				path.Root(name),
				"Unknown "+r.form.Kind+" field",
				name+" must be wholly known before the provider can apply this Form.",
			)
			continue
		}
		wire, fieldDiags := v3FieldToWire(ctx, field, name, value)
		diags.Append(fieldDiags...)
		if fieldDiags.HasError() {
			continue
		}
		if wire != nil {
			spec[field.Wire] = wire
		}
	}
	return spec, diags
}

func v3FieldToWire(ctx context.Context, field model.Field, attrName string, value attr.Value) (any, diag.Diagnostics) {
	var diags diag.Diagnostics
	switch field.Kind {
	case model.KindBoolean:
		return value.(types.Bool).ValueBool(), diags
	case model.KindInteger:
		return value.(types.Int64).ValueInt64(), diags
	case model.KindStringSet:
		set := value.(types.Set)
		var members []string
		for _, member := range set.Elements() {
			if member == nil || member.IsNull() || member.IsUnknown() {
				diags.AddAttributeError(
					path.Root(attrName),
					"Unknown or null set member",
					attrName+" must contain only wholly known, non-null values.",
				)
				return nil, diags
			}
		}
		diags.Append(set.ElementsAs(ctx, &members, false)...)
		if len(members) == 0 {
			return nil, diags
		}
		return members, diags
	case model.KindJSONMap:
		text := value.(types.String).ValueString()
		parsed, err := v3ParseJSONObject(text)
		if err != nil {
			diags.AddAttributeError(path.Root(attrName), "Invalid JSON object", err.Error())
			return nil, diags
		}
		if len(parsed) == 0 {
			return nil, diags
		}
		return parsed, diags
	case model.KindResourceRef:
		return map[string]any{"kind": field.TargetKind, "name": value.(types.String).ValueString()}, diags
	case model.KindBindingList:
		list := value.(types.List)
		out := make([]any, 0, len(list.Elements()))
		for index, element := range list.Elements() {
			object, objectDiags := v3KnownObject(attrName, index, element)
			diags.Append(objectDiags...)
			if objectDiags.HasError() {
				return nil, diags
			}
			attributes := object.Attributes()
			name, nameDiags := v3KnownString(attrName, "name", attributes["name"])
			target, targetDiags := v3KnownString(attrName, "target_name", attributes["target_name"])
			diags.Append(nameDiags...)
			diags.Append(targetDiags...)
			if diags.HasError() {
				return nil, diags
			}
			out = append(out, map[string]any{
				"name": name,
				// The wire key is `resource`, never `target` (decision 0010).
				"resource": map[string]any{"kind": field.TargetKind, "name": target},
			})
		}
		if len(out) == 0 {
			return nil, diags
		}
		return out, diags
	case model.KindObjectList:
		list := value.(types.List)
		out := make([]any, 0, len(list.Elements()))
		for index, element := range list.Elements() {
			object, objectDiags := v3KnownObject(attrName, index, element)
			diags.Append(objectDiags...)
			if objectDiags.HasError() {
				return nil, diags
			}
			attributes := object.Attributes()
			entry := map[string]any{}
			for _, member := range field.Fields {
				memberValue, present := attributes[member.HCL]
				if !present || memberValue == nil || memberValue.IsNull() {
					continue
				}
				if memberValue.IsUnknown() {
					diags.AddAttributeError(
						path.Root(attrName),
						"Unknown nested value",
						fmt.Sprintf("%s[%d].%s must be wholly known.", attrName, index, member.HCL),
					)
					return nil, diags
				}
				switch member.Kind {
				case model.KindInteger:
					entry[member.Wire] = memberValue.(types.Int64).ValueInt64()
				case model.KindResourceRef:
					entry[member.Wire] = map[string]any{
						"kind": member.TargetKind, "name": memberValue.(types.String).ValueString(),
					}
				default:
					entry[member.Wire] = memberValue.(types.String).ValueString()
				}
			}
			out = append(out, entry)
		}
		if len(out) == 0 {
			return nil, diags
		}
		return out, diags
	default:
		text := value.(types.String).ValueString()
		if text == "" {
			return nil, diags
		}
		return text, diags
	}
}

func v3KnownObject(attrName string, index int, element attr.Value) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	object, ok := element.(types.Object)
	if !ok || object.IsNull() || object.IsUnknown() {
		diags.AddAttributeError(
			path.Root(attrName),
			"Unknown or null nested object",
			fmt.Sprintf("%s[%d] must be a wholly known object.", attrName, index),
		)
		return types.Object{}, diags
	}
	return object, diags
}

func v3KnownString(attrName, member string, value attr.Value) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	text, ok := value.(types.String)
	if !ok || text.IsNull() || text.IsUnknown() {
		diags.AddAttributeError(
			path.Root(attrName),
			"Unknown or null nested value",
			attrName+"."+member+" must be wholly known and non-null.",
		)
		return "", diags
	}
	return text.ValueString(), diags
}

// v3FieldValueFromSpec is the inverse projection: one wire spec value back to
// its typed HCL representation, used by reads that adopt out-of-band change.
func v3FieldValueFromSpec(ctx context.Context, field model.Field, raw any, diags *diag.Diagnostics) attr.Value {
	switch field.Kind {
	case model.KindBoolean:
		if value, ok := raw.(bool); ok {
			return types.BoolValue(value)
		}
		return types.BoolNull()
	case model.KindInteger:
		return int64FromSpec(raw)
	case model.KindStringSet:
		if raw == nil {
			return types.SetNull(types.StringType)
		}
		set, setDiags := types.SetValueFrom(ctx, types.StringType, toStringSlice(raw))
		diags.Append(setDiags...)
		return set
	case model.KindJSONMap:
		if raw == nil {
			return types.StringNull()
		}
		text, err := v3MarshalJSON(raw)
		if err != nil {
			diags.AddError("Host spec value is not serializable", err.Error())
			return types.StringNull()
		}
		return types.StringValue(text)
	case model.KindResourceRef:
		if ref, ok := raw.(map[string]any); ok {
			if name, ok := ref["name"].(string); ok {
				return types.StringValue(name)
			}
		}
		return types.StringNull()
	case model.KindBindingList:
		elementType := v3BindingObjectType()
		items, ok := raw.([]any)
		if !ok {
			return types.ListNull(elementType)
		}
		elements := make([]attr.Value, 0, len(items))
		for _, item := range items {
			entry, _ := item.(map[string]any)
			name, _ := entry["name"].(string)
			var targetName string
			if ref, ok := entry["resource"].(map[string]any); ok {
				targetName, _ = ref["name"].(string)
			}
			elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, map[string]attr.Value{
				"name":        types.StringValue(name),
				"target_name": types.StringValue(targetName),
			}))
		}
		list, listDiags := types.ListValue(elementType, elements)
		diags.Append(listDiags...)
		return list
	case model.KindObjectList:
		elementType := v3ObjectListType(field)
		items, ok := raw.([]any)
		if !ok {
			return types.ListNull(elementType)
		}
		elements := make([]attr.Value, 0, len(items))
		for _, item := range items {
			entry, _ := item.(map[string]any)
			memberValues := map[string]attr.Value{}
			for _, member := range field.Fields {
				switch member.Kind {
				case model.KindInteger:
					memberValues[member.HCL] = int64FromSpec(entry[member.Wire])
				case model.KindResourceRef:
					name := ""
					if ref, ok := entry[member.Wire].(map[string]any); ok {
						name, _ = ref["name"].(string)
					}
					memberValues[member.HCL] = optionalString(name)
				default:
					text, _ := entry[member.Wire].(string)
					memberValues[member.HCL] = optionalString(text)
				}
			}
			elements = append(elements, types.ObjectValueMust(elementType.AttrTypes, memberValues))
		}
		list, listDiags := types.ListValue(elementType, elements)
		diags.Append(listDiags...)
		return list
	default:
		return optionalStringFromAny(raw)
	}
}

func v3BindingObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":        types.StringType,
		"target_name": types.StringType,
	}}
}

func v3ObjectListType(field model.Field) types.ObjectType {
	memberTypes := map[string]attr.Type{}
	for _, member := range field.Fields {
		switch member.Kind {
		case model.KindInteger:
			memberTypes[member.HCL] = types.Int64Type
		default:
			memberTypes[member.HCL] = types.StringType
		}
	}
	return types.ObjectType{AttrTypes: memberTypes}
}

// v3ParseJSONObject parses one strict JSON object; anything else fails.
//
// The literal `null` is rejected explicitly: encoding/json accepts it into a
// map target and leaves the map nil, so it would pass as "valid JSON" and then
// travel to the host as an absent object rather than the desired state the
// configuration actually names. Callers surface the returned error against the
// attribute path, so the diagnostic names vars_json / spec_json itself.
func v3ParseJSONObject(text string) (map[string]any, error) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("value must be one JSON object: %w", err)
	}
	if parsed == nil {
		return nil, errors.New("value must be one JSON object, not the literal null; omit the attribute instead")
	}
	return parsed, nil
}

// v3MarshalJSON renders JSON deterministically (Go sorts object keys).
func v3MarshalJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// v3DurationValidator validates optional Go-duration timeout attributes.
type v3DurationValidator struct{}

func (v3DurationValidator) Description(context.Context) string {
	return "value must be a positive Go duration such as \"20m\""
}

func (v v3DurationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v3DurationValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, ok := v3ParseTimeout(req.ConfigValue.ValueString()); !ok {
		resp.Diagnostics.AddAttributeError(
			req.Path, "Invalid operation timeout",
			"value must be a positive Go duration such as \"20m\".",
		)
	}
}

// v3JSONObjectValidator validates that a configured string is one JSON
// object.
type v3JSONObjectValidator struct{}

func (v3JSONObjectValidator) Description(context.Context) string {
	return "value must be one JSON object"
}

func (v v3JSONObjectValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v3JSONObjectValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := v3ParseJSONObject(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON object", err.Error())
	}
}

// v3ListSizeValidator enforces catalog min/max list cardinality.
type v3ListSizeValidator struct {
	minItems, maxItems int
}

func (v v3ListSizeValidator) Description(context.Context) string {
	return fmt.Sprintf("list must declare between %d and %d items", v.minItems, v.maxItems)
}

func (v v3ListSizeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v v3ListSizeValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	count := len(req.ConfigValue.Elements())
	if v.minItems > 0 && count < v.minItems {
		resp.Diagnostics.AddAttributeError(req.Path, "Too few items",
			fmt.Sprintf("at least %d item(s) required, got %d", v.minItems, count))
	}
	if v.maxItems > 0 && count > v.maxItems {
		resp.Diagnostics.AddAttributeError(req.Path, "Too many items",
			fmt.Sprintf("at most %d item(s) allowed, got %d", v.maxItems, count))
	}
}

// v3WeightSumValidator proves WorkerDeployment traffic weights sum to exactly
// 10000 basis points. The sum is checked only when every weight is known; a
// schema cannot add weights, so the provider does (and the host re-proves).
type v3WeightSumValidator struct{}

func (v3WeightSumValidator) Description(context.Context) string {
	return fmt.Sprintf("version weights must sum to exactly %d basis points", workerDeploymentWeightSum)
}

func (v v3WeightSumValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v3WeightSumValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var sum int64
	for _, element := range req.ConfigValue.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			return
		}
		weight, ok := object.Attributes()["weight"].(types.Int64)
		if !ok || weight.IsNull() || weight.IsUnknown() {
			return
		}
		sum += weight.ValueInt64()
	}
	if sum != workerDeploymentWeightSum {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid deployment weight total",
			fmt.Sprintf("version weights must sum to exactly %d basis points, got %d", workerDeploymentWeightSum, sum),
		)
	}
}

// v3ParseTimeout parses one positive Go duration.
func v3ParseTimeout(text string) (time.Duration, bool) {
	parsed, err := time.ParseDuration(text)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}
