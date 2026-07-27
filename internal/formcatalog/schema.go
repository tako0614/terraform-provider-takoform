package formcatalog

import (
	"fmt"
	"sort"
)

// Portable string grammars. Every pattern is RE2-safe so the same expression
// constrains the Terraform schema, the Draft 2020-12 desired schema, and a
// host's own validator without any of them needing a different engine.
const (
	PatternToken      = `^[A-Za-z][A-Za-z0-9._:-]{0,127}$`
	PatternClass      = `^[A-Za-z_$][A-Za-z0-9_$]*$`
	PatternTimezone   = `^[A-Za-z][A-Za-z0-9._:/+-]{0,127}$`
	PatternCron       = `^\S+ \S+ \S+ \S+ \S+$`
	PatternHostname   = `^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$`
	PatternPath       = `^/[A-Za-z0-9._~!$&'()*+,;=:@%/-]*$`
	PatternCIDR       = `^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$|^[0-9A-Fa-f:]{2,45}/[0-9]{1,3}$`
	PatternOCIDigest  = `^[^@\s]+@sha256:[A-Fa-f0-9]{64}$`
	PatternMailbox    = `^[^@\s]+@[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$`
	PatternHTTPSURL   = `^https://\S+$`
	PatternRecordData = `^\S(.*\S)?$`
	PatternName       = `.*\S.*`
)

// NameMaxLength bounds every portable Resource name.
const NameMaxLength = 128

// Pattern returns the RE2 pattern for a grammar, and whether one applies.
func (g Grammar) Pattern() (string, bool) {
	switch g {
	case GrammarToken:
		return PatternToken, true
	case GrammarClass:
		return PatternClass, true
	case GrammarTimezone:
		return PatternTimezone, true
	case GrammarCron:
		return PatternCron, true
	case GrammarHostname, GrammarDomain:
		return PatternHostname, true
	case GrammarPath:
		return PatternPath, true
	case GrammarCIDR:
		return PatternCIDR, true
	case GrammarOCIDigest:
		return PatternOCIDigest, true
	case GrammarMailbox:
		return PatternMailbox, true
	case GrammarHTTPSURL:
		return PatternHTTPSURL, true
	case GrammarRecordData:
		return PatternRecordData, true
	default:
		return "", false
	}
}

// Message is the human-readable constraint used by provider diagnostics.
func (g Grammar) Message(field string) string {
	switch g {
	case GrammarToken:
		return field + " must use the portable capability-token grammar"
	case GrammarClass:
		return field + " must use the portable runtime class grammar"
	case GrammarTimezone:
		return field + " must use the portable timezone grammar"
	case GrammarCron:
		return field + " must be a portable five-field cron expression"
	case GrammarHostname:
		return field + " must be a dotted DNS hostname"
	case GrammarDomain:
		return field + " must be a dotted DNS domain name"
	case GrammarPath:
		return field + " must be an absolute URL path"
	case GrammarCIDR:
		return field + " must be an address block in CIDR notation"
	case GrammarOCIDigest:
		return field + " must be an OCI reference pinned by sha256 digest"
	case GrammarMailbox:
		return field + " must be an email address"
	case GrammarHTTPSURL:
		return field + " must be an absolute https URL"
	case GrammarRecordData:
		return field + " must not be blank"
	default:
		return field + " is invalid"
	}
}

// DesiredSchema derives the Draft 2020-12 desired schema of a Form from its
// declared fields. It is the single source the Form Definition is built from.
func (k Kind) DesiredSchema() map[string]any {
	properties := map[string]any{
		"name": map[string]any{
			"type": "string", "minLength": 1, "maxLength": NameMaxLength, "pattern": PatternName,
		},
	}
	required := []string{"name"}
	defs := map[string]any{}

	if k.Artifact {
		for key, value := range artifactDefinitions() {
			defs[key] = value
		}
		properties["source"] = map[string]any{"$ref": "#/$defs/artifactSource"}
		required = append(required, "source")
	}
	if k.Connections != ConnectionsAbsent {
		for key, value := range connectionDefinitions() {
			defs[key] = value
		}
		properties["connections"] = map[string]any{"$ref": "#/$defs/connections"}
		if k.Connections == ConnectionsRequired {
			required = append(required, "connections")
		}
	}
	for _, field := range k.Fields {
		properties[field.Wire] = field.jsonSchema()
		if field.Required {
			required = append(required, field.Wire)
		}
	}
	sort.Strings(required)

	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
		"additionalProperties": false, "required": required, "properties": properties,
	}
	if len(defs) > 0 {
		schema["$defs"] = defs
	}
	return schema
}

func (f Field) jsonSchema() map[string]any {
	switch f.Type {
	case TypeBool:
		return map[string]any{"type": "boolean"}
	case TypeInt:
		return integerSchema(f.Min, f.Max)
	case TypeIntSet:
		items := integerSchema(f.Min, f.Max)
		return arraySchema(items, f.MinItems)
	case TypeStringSet:
		return arraySchema(f.stringSchema(), f.MinItems)
	default:
		schema := f.stringSchema()
		if f.Default != "" {
			schema["default"] = f.Default
		}
		return schema
	}
}

func (f Field) stringSchema() map[string]any {
	schema := map[string]any{"type": "string"}
	if len(f.Enum) > 0 {
		values := make([]any, 0, len(f.Enum))
		for _, value := range f.Enum {
			values = append(values, value)
		}
		schema["enum"] = values
		return schema
	}
	if pattern, ok := f.Grammar.Pattern(); ok {
		schema["pattern"] = pattern
		return schema
	}
	schema["minLength"] = 1
	return schema
}

func integerSchema(minimum, maximum *int64) map[string]any {
	schema := map[string]any{"type": "integer"}
	if minimum != nil {
		schema["minimum"] = *minimum
	}
	if maximum != nil {
		schema["maximum"] = *maximum
	}
	return schema
}

func arraySchema(items map[string]any, minItems int) map[string]any {
	schema := map[string]any{"type": "array", "uniqueItems": true, "items": items}
	if minItems > 0 {
		schema["minItems"] = minItems
	}
	return schema
}

// CanonicalDesired builds the exact desired fixture of a Form. A fixture is
// real input a host can attempt, never a placeholder.
func (k Kind) CanonicalDesired() map[string]any {
	desired := map[string]any{"name": k.FixtureName()}
	if k.Artifact {
		desired["source"] = cloneValue(k.ArtifactExample)
	}
	if k.Connections != ConnectionsAbsent && k.ConnectionExample != nil {
		desired["connections"] = cloneValue(k.ConnectionExample)
	}
	for _, field := range k.Fields {
		if field.Example == nil {
			continue
		}
		desired[field.Wire] = cloneValue(field.Example)
	}
	return desired
}

// NegativeDesired builds the exact input a conforming host must reject. It
// mutates one declared counter-example, so the negative case always tests a
// constraint this Form actually states.
func (k Kind) NegativeDesired() (map[string]any, error) {
	for _, field := range k.Fields {
		if field.CounterExample == nil {
			continue
		}
		negative := cloneValue(k.CanonicalDesired()).(map[string]any)
		negative[field.Wire] = cloneValue(field.CounterExample)
		return negative, nil
	}
	return nil, fmt.Errorf("%s declares no counter-example field", k.Kind)
}

// FixtureName is the Resource name used by this Form's fixtures.
func (k Kind) FixtureName() string { return k.Slug }

// DesiredKeys lists the exact wire keys of the canonical fixture, sorted.
func (k Kind) DesiredKeys() []string {
	desired := k.CanonicalDesired()
	keys := make([]string, 0, len(desired))
	for key := range desired {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ImmutableFields lists the JSON Pointers a host must treat as replacement
// triggers. Every Form fixes its name.
func (k Kind) ImmutableFields() []string {
	pointers := []string{"/name"}
	for _, field := range k.Fields {
		if field.Immutable {
			pointers = append(pointers, "/"+field.Wire)
		}
	}
	sort.Strings(pointers)
	return pointers
}

// OutputSchema derives the sanitized public output contract of a Form.
//
// Outputs carry identity and portability evidence only. A field is echoed
// here only because a declared interface resolves it, never because a host
// wanted to publish something about its own implementation.
func (k Kind) OutputSchema() map[string]any {
	required := []string{"generation", "id", "kind", "name", "portability"}
	properties := map[string]any{
		"id":          map[string]any{"type": "string", "minLength": 1},
		"kind":        map[string]any{"type": "string", "const": k.Kind},
		"name":        map[string]any{"type": "string", "minLength": 1},
		"generation":  map[string]any{"type": "integer", "minimum": 1},
		"portability": map[string]any{"type": "string", "pattern": PatternToken},
	}
	for _, field := range k.interfaceResolvedFields() {
		properties[field.Wire] = field.jsonSchema()
		required = append(required, field.Wire)
	}
	sort.Strings(required)
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
		"additionalProperties": false, "required": required, "properties": properties,
	}
}

// CanonicalOutput builds the exact output fixture of a Form.
func (k Kind) CanonicalOutput() map[string]any {
	name := k.FixtureName()
	output := map[string]any{
		"id": k.Kind + "/" + name, "kind": k.Kind, "name": name,
		"generation": 1, "portability": "portable",
	}
	for _, field := range k.interfaceResolvedFields() {
		output[field.Wire] = cloneValue(field.Example)
	}
	return output
}

// CanonicalObserved builds the exact observed fixture of a Form.
func (k Kind) CanonicalObserved() map[string]any {
	return map[string]any{
		"id": k.Kind + "/" + k.FixtureName(), "ready": true, "generation": 1,
		"imported": true, "portability": "portable", "driftedFields": []any{},
	}
}

// interfaceResolvedFields lists declared fields a declared interface reads
// through the Form's own output document.
func (k Kind) interfaceResolvedFields() []Field {
	var resolved []Field
	for _, declared := range k.Interfaces {
		for _, extra := range declared.ExtraInputs {
			for _, field := range k.Fields {
				if field.Wire == extra {
					resolved = append(resolved, field)
				}
			}
		}
	}
	return resolved
}

// ObservedSchema is the lifecycle status contract every Form shares.
func ObservedSchema() map[string]any {
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
		"additionalProperties": false,
		"required":             []string{"driftedFields", "generation", "id", "imported", "portability", "ready"},
		"properties": map[string]any{
			"id":          map[string]any{"type": "string", "minLength": 1},
			"ready":       map[string]any{"type": "boolean"},
			"generation":  map[string]any{"type": "integer", "minimum": 1},
			"imported":    map[string]any{"type": "boolean"},
			"portability": map[string]any{"type": "string", "pattern": PatternToken},
			"driftedFields": map[string]any{
				"type": "array", "uniqueItems": true,
				"items": map[string]any{"type": "string", "pattern": `^(?:/(?:[^~/]|~0|~1)*)+$`},
			},
		},
	}
}

func portableMapKeys() map[string]any {
	return map[string]any{
		"type": "string", "pattern": `^[A-Za-z][A-Za-z0-9._-]{0,63}$`,
		"x-takoform-fieldPolicy": "portable-data-only-v1",
	}
}

func connectionDefinitions() map[string]any {
	return map[string]any{
		"connections": map[string]any{
			"type": "object", "propertyNames": portableMapKeys(),
			"additionalProperties": map[string]any{"$ref": "#/$defs/connection"},
		},
		"connection": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"permissions", "projection", "resource"},
			"properties": map[string]any{
				"resource": map[string]any{"type": "string", "pattern": `^\S+$`},
				"permissions": map[string]any{
					"type": "array", "minItems": 1, "uniqueItems": true,
					"items": map[string]any{"type": "string", "pattern": PatternToken},
				},
				"projection": map[string]any{"type": "string", "pattern": PatternToken},
			},
		},
	}
}

func artifactDefinitions() map[string]any {
	return map[string]any{
		"artifactSource": map[string]any{"oneOf": []any{
			map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"artifactPath"},
				"properties": map[string]any{"artifactPath": map[string]any{"type": "string", "minLength": 1}},
			},
			map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"artifactSha256", "artifactUrl"},
				"properties": map[string]any{
					"artifactUrl":    map[string]any{"type": "string", "format": "uri", "pattern": "^https://"},
					"artifactSha256": map[string]any{"$ref": "#/$defs/sha256"},
				},
			},
			map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"artifactRef", "artifactSha256"},
				"properties": map[string]any{
					"artifactRef":    map[string]any{"type": "string", "minLength": 1},
					"artifactSha256": map[string]any{"$ref": "#/$defs/sha256"},
				},
			},
		}},
		"sha256": map[string]any{"type": "string", "pattern": `^(sha256:)?[A-Fa-f0-9]{64}$`},
	}
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = cloneValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, cloneValue(item))
		}
		return out
	default:
		return value
	}
}
