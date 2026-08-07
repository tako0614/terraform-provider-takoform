package currentformmodel

import (
	"fmt"
	"sort"
)

// Portable string grammars. Every pattern is RE2-safe, so the same expression
// constrains the Terraform schema, the Draft 2020-12 desired schema, and a
// host's own validator without any of them needing a different engine. These
// are declared locally on purpose: the v1alpha2 catalog is retained prior art
// and the family lane must not inherit silent edits from it.
const (
	// PatternResourceName is the metadata.name grammar of the v1alpha3
	// envelope, reused wherever a Form references another resource by name.
	PatternResourceName = `^[a-z][a-z0-9-]{0,62}$`
	// PatternBindingName is the JavaScript identifier grammar worker-family
	// binding names use (decision 0010).
	PatternBindingName = `^[A-Za-z_$][A-Za-z0-9_$]*$`
	// PatternRelativePath is a non-escaping relative module path.
	PatternRelativePath = `^[A-Za-z0-9_][A-Za-z0-9._-]*(?:/[A-Za-z0-9_][A-Za-z0-9._-]*)*$`
	// PatternHostname is a dotted DNS hostname.
	PatternHostname = `^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$`
	// PatternCron is the small interoperable five-field cron subset shared
	// with the retained catalog. Schedules are interpreted in UTC only.
	PatternCron = `^(?:[0-9]|[1-5][0-9]) (?:[0-9]|1[0-9]|2[0-3]) (?:\*|[1-9]|[12][0-9]|3[01]) (?:\*|[1-9]|1[0-2]) (?:\*|[0-6])$`
	// PatternCanonicalSHA256 is the algorithm-prefixed lowercase digest used
	// by every exact Takoform identity.
	PatternCanonicalSHA256 = `^sha256:[0-9a-f]{64}$`
	// PatternDate is a calendar date, YYYY-MM-DD.
	PatternDate = `^\d{4}-\d{2}-\d{2}$`
	// PatternSensitiveVarName names one host-supplied sensitive value.
	PatternSensitiveVarName = `^[A-Z][A-Z0-9_]{0,63}$`

	// PortableMapKeyPattern is the reviewed key grammar of the data-only map
	// escape; formpackage requires this exact propertyNames declaration.
	PortableMapKeyPattern = `^[A-Za-z][A-Za-z0-9._-]{0,63}$`
	portableMapPolicyKey  = "x-takoform-fieldPolicy"
	portableMapPolicy     = "portable-data-only-v1"

	// ResourceNameMaxLength bounds every portable resource name.
	ResourceNameMaxLength = 63
	// jsonMapMaxDepth bounds nested containers inside a KindJSONMap value.
	jsonMapMaxDepth = 8
	// jsonMapMaxKeys bounds keys per object and items per array at every
	// nesting level of a KindJSONMap value.
	jsonMapMaxKeys = 64
	// jsonMapMaxStringLength bounds every string leaf of a KindJSONMap value.
	jsonMapMaxStringLength = 8192
	// bindingListMaxItems bounds declared bindings per binding list.
	bindingListMaxItems = 64
	// bindingNameMaxLength bounds one binding instance name.
	bindingNameMaxLength = 64
)

// DesiredSchema derives the Draft 2020-12 closed desired schema of a Form.
// It never contains a "name" property: the v1alpha3 envelope owns
// metadata.name (decision 0011).
func (f Form) DesiredSchema() map[string]any {
	properties := map[string]any{}
	var required []string
	needsJSONMapDefs := false
	for _, field := range f.Fields {
		properties[field.Wire] = field.jsonSchema()
		if field.Required {
			required = append(required, field.Wire)
		}
		if field.usesJSONMap() {
			needsJSONMapDefs = true
		}
	}
	sort.Strings(required)
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"title":                f.Title + " desired state",
		"description":          f.Description,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	if needsJSONMapDefs {
		schema["$defs"] = jsonMapValueDefinitions()
	}
	return schema
}

func (f Field) usesJSONMap() bool {
	if f.Kind == KindJSONMap {
		return true
	}
	for _, member := range f.Fields {
		if member.usesJSONMap() {
			return true
		}
	}
	return false
}

func (f Field) jsonSchema() map[string]any {
	schema := f.jsonSchemaShape()
	if f.Doc != "" {
		schema["description"] = f.Doc
	}
	// The portable default travels inside the desired schema, on the property
	// that declares it. That is what makes it portable at all: it reaches every
	// host through the Form Definition the host already installed, so an
	// omitted optional field means one thing everywhere (decision 0008).
	if f.Default != nil {
		schema["default"] = cloneValue(f.Default)
	}
	return schema
}

func (f Field) jsonSchemaShape() map[string]any {
	switch f.Kind {
	case KindBoolean:
		return map[string]any{"type": "boolean"}
	case KindInteger:
		schema := map[string]any{"type": "integer"}
		if f.Min != nil {
			schema["minimum"] = *f.Min
		}
		if f.Max != nil {
			schema["maximum"] = *f.Max
		}
		return schema
	case KindString:
		schema := map[string]any{"type": "string"}
		if f.Pattern != "" {
			schema["pattern"] = f.Pattern
		} else {
			schema["minLength"] = 1
		}
		if f.MaxLength > 0 {
			schema["maxLength"] = f.MaxLength
		}
		return schema
	case KindStringEnum:
		return map[string]any{"type": "string", "enum": anySlice(f.Enum)}
	case KindDateString:
		return map[string]any{"type": "string", "pattern": PatternDate}
	case KindStringSet:
		items := map[string]any{"type": "string"}
		if len(f.Enum) > 0 {
			items["enum"] = anySlice(f.Enum)
		} else {
			items["pattern"] = f.ItemPattern
		}
		return f.arraySchema(items)
	case KindJSONMap:
		// The reviewed typed-map escape: exact key policy, values bounded by
		// the finite depth chain in $defs. formpackage's portable schema
		// verifier proves this shape closed.
		return map[string]any{
			"type":                 "object",
			"maxProperties":        jsonMapMaxKeys,
			"propertyNames":        portableMapKeys(),
			"additionalProperties": map[string]any{"$ref": "#/$defs/jsonValueDepth1"},
		}
	case KindResourceRef:
		return resourceRefSchema(f.TargetKind)
	case KindResourceRefList:
		return f.arraySchema(resourceRefSchema(f.TargetKind))
	case KindBindingList:
		schema := f.arraySchema(map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"name", "resource"},
			"properties": map[string]any{
				"name": map[string]any{
					"type":      "string",
					"pattern":   PatternBindingName,
					"maxLength": bindingNameMaxLength,
				},
				"resource": resourceRefSchema(f.TargetKind),
			},
		})
		if f.MaxItems == 0 {
			schema["maxItems"] = bindingListMaxItems
		}
		return schema
	case KindObject:
		return objectSchema(f.Fields)
	case KindObjectList:
		return f.arraySchema(objectSchema(f.Fields))
	default:
		panic(fmt.Sprintf("unknown field kind %q", f.Kind))
	}
}

func (f Field) arraySchema(items map[string]any) map[string]any {
	schema := map[string]any{"type": "array", "uniqueItems": true, "items": items}
	if f.MinItems > 0 {
		schema["minItems"] = f.MinItems
	}
	if f.MaxItems > 0 {
		schema["maxItems"] = f.MaxItems
	}
	return schema
}

func objectSchema(fields []Field) map[string]any {
	properties := map[string]any{}
	var required []string
	for _, member := range fields {
		properties[member.Wire] = member.jsonSchema()
		if member.Required {
			required = append(required, member.Wire)
		}
	}
	sort.Strings(required)
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func resourceRefSchema(targetKind string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"kind", "name"},
		"properties": map[string]any{
			"kind": map[string]any{"type": "string", "const": targetKind},
			"name": map[string]any{
				"type":      "string",
				"minLength": 1,
				"maxLength": ResourceNameMaxLength,
				"pattern":   PatternResourceName,
			},
		},
	}
}

func portableMapKeys() map[string]any {
	return map[string]any{
		"type":               "string",
		"pattern":            PortableMapKeyPattern,
		portableMapPolicyKey: portableMapPolicy,
	}
}

// jsonMapValueDefinitions unrolls the bounded any-JSON-value schema of the
// data-only vars map. Cyclic references are rejected by the portable closure
// proof, so the depth bound is expressed as a finite chain: containers at
// depth N admit values at depth N+1, and the deepest level admits scalars
// only.
func jsonMapValueDefinitions() map[string]any {
	defs := map[string]any{}
	for depth := 1; depth < jsonMapMaxDepth; depth++ {
		defs[fmt.Sprintf("jsonValueDepth%d", depth)] = map[string]any{
			"type":                 []any{"array", "boolean", "null", "number", "object", "string"},
			"maxLength":            jsonMapMaxStringLength,
			"maxItems":             jsonMapMaxKeys,
			"maxProperties":        jsonMapMaxKeys,
			"items":                map[string]any{"$ref": fmt.Sprintf("#/$defs/jsonValueDepth%d", depth+1)},
			"propertyNames":        portableMapKeys(),
			"additionalProperties": map[string]any{"$ref": fmt.Sprintf("#/$defs/jsonValueDepth%d", depth+1)},
		}
	}
	defs[fmt.Sprintf("jsonValueDepth%d", jsonMapMaxDepth)] = map[string]any{
		"type":      []any{"boolean", "null", "number", "string"},
		"maxLength": jsonMapMaxStringLength,
	}
	return defs
}

func anySlice(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
