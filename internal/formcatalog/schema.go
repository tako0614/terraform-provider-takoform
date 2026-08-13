package formcatalog

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
)

// Portable string grammars. Every pattern is RE2-safe so the same expression
// constrains the Terraform schema, the Draft 2020-12 desired schema, and a
// host's own validator without any of them needing a different engine.
const (
	patternIPv4Body     = `(?:(?:25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])\.){3}(?:25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])`
	patternIPv6Body     = `(?:(?:[0-9A-Fa-f]{1,4}:){7}[0-9A-Fa-f]{1,4}|(?:[0-9A-Fa-f]{1,4}:){1,7}:|(?:[0-9A-Fa-f]{1,4}:){1,6}:[0-9A-Fa-f]{1,4}|(?:[0-9A-Fa-f]{1,4}:){1,5}(?::[0-9A-Fa-f]{1,4}){1,2}|(?:[0-9A-Fa-f]{1,4}:){1,4}(?::[0-9A-Fa-f]{1,4}){1,3}|(?:[0-9A-Fa-f]{1,4}:){1,3}(?::[0-9A-Fa-f]{1,4}){1,4}|(?:[0-9A-Fa-f]{1,4}:){1,2}(?::[0-9A-Fa-f]{1,4}){1,5}|[0-9A-Fa-f]{1,4}:(?:(?::[0-9A-Fa-f]{1,4}){1,6})|:(?:(?::[0-9A-Fa-f]{1,4}){1,7}|:))`
	patternUint16Body   = `(?:[0-9]|[1-9][0-9]{1,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])`
	patternUint8Body    = `(?:[0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])`
	patternHostnameBody = `[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+`
	// \s is the portable ASCII subset. The literal code points add the rest
	// of Unicode White_Space plus U+FEFF without relying on engine-specific
	// Unicode property syntax.
	patternURLWhitespaceBody = `\s` + "\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

	PatternToken    = `^[A-Za-z][A-Za-z0-9._:-]{0,127}$`
	PatternVersion  = `^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$`
	PatternClass    = `^[A-Za-z_$][A-Za-z0-9_$]*$`
	PatternTimezone = `^[A-Za-z][A-Za-z0-9._:/+-]{0,127}$`
	// PatternCron intentionally defines a small interoperable subset rather
	// than pretending every host agrees on extensions such as names, lists,
	// ranges, or steps.
	PatternCron             = `^(?:[0-9]|[1-5][0-9]) (?:[0-9]|1[0-9]|2[0-3]) (?:\*|[1-9]|[12][0-9]|3[01]) (?:\*|[1-9]|1[0-2]) (?:\*|[0-6])$`
	PatternHostname         = `^` + patternHostnameBody + `$`
	PatternPath             = `^/[A-Za-z0-9._~!$&'()*+,;=:@%/-]*$`
	PatternIPv4Address      = `^` + patternIPv4Body + `$`
	PatternIPv6Address      = `^` + patternIPv6Body + `$`
	PatternCIDR             = `^(?:` + patternIPv4Body + `/(?:[0-9]|[12][0-9]|3[0-2])|` + patternIPv6Body + `/(?:[0-9]|[1-9][0-9]|1[01][0-9]|12[0-8]))$`
	PatternDNSRelativeName  = `^(?:@|(?:\*\.)?[A-Za-z0-9_](?:[A-Za-z0-9_-]{0,61}[A-Za-z0-9_])?(?:\.[A-Za-z0-9_](?:[A-Za-z0-9_-]{0,61}[A-Za-z0-9_])?)*)$`
	PatternOCIDigest        = `^[^@\s]+@sha256:[A-Fa-f0-9]{64}$`
	PatternSHA256           = `^(sha256:)?[A-Fa-f0-9]{64}$`
	PatternMailbox          = `^[^@\s]+@[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$`
	PatternMailboxLocalPart = `^[A-Za-z0-9][A-Za-z0-9.!#$%&'*+/=?^_{|}~-]{0,63}$`
	PatternHTTPSURL         = `^https://[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+(?::[0-9]{1,5})?(?:/[^\s?#]*)?(?:\?[^\s#]*)?(?:#[^\s]*)?$`
	// PatternRetainedCredentialFreeHTTPSURL is the broader historical pattern
	// carried by EdgeWorker@3.0.0. The Terraform schema must be able to decode
	// either recorded codec; the exact selected Form Definition performs the
	// final validation before any host request.
	PatternRetainedCredentialFreeHTTPSURL = `^https://` + patternHostnameBody + `(?::[0-9]{1,5})?(?:/[^\s?#]*)?$`
	PatternCredentialFreeHTTPSURL         = `^https://` + patternHostnameBody + `(?::[0-9]{1,5})?(?:/[^` + patternURLWhitespaceBody + `?#]*)?$`
	PatternMediaType                      = `^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$`
	PatternRecordData                     = `^\S(.*\S)?$`
	PatternRelativePath                   = `^[A-Za-z0-9_][A-Za-z0-9._-]*(?:/[A-Za-z0-9_][A-Za-z0-9._-]*)*$`
	PatternDNSMX                          = `^` + patternUint16Body + ` ` + patternHostnameBody + `$`
	PatternDNSSRV                         = `^` + patternUint16Body + ` ` + patternUint16Body + ` ` + patternUint16Body + ` ` + patternHostnameBody + `$`
	PatternDNSCAA                         = `^` + patternUint8Body + ` [A-Za-z0-9]+ \S(?:.*\S)?$`
	PatternName                           = `^[a-z][a-z0-9-]{0,62}$`
	PatternResourceRef                    = `^[A-Z][A-Za-z0-9]{0,63}/[a-z][a-z0-9-]{0,62}$`
)

var credentialFreeHTTPSURLPattern = regexp.MustCompile(PatternCredentialFreeHTTPSURL)
var retainedCredentialFreeHTTPSURLPattern = regexp.MustCompile(PatternRetainedCredentialFreeHTTPSURL)

// ValidCredentialFreeHTTPSURL is the shared runtime validator for portable,
// nonsensitive HTTPS coordinates. PatternCredentialFreeHTTPSURL is the same
// definition embedded in every normative JSON Schema.
func ValidCredentialFreeHTTPSURL(value string) bool {
	if !credentialFreeHTTPSURLPattern.MatchString(value) {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil &&
		parsed.Scheme == "https" &&
		parsed.Host != "" &&
		parsed.User == nil &&
		parsed.RawQuery == "" &&
		parsed.Fragment == ""
}

// ValidRetainedCredentialFreeHTTPSURL validates the decode union needed by a
// provider that supports EdgeWorker@3.0.0 and EdgeWorker@4.0.0. Exact codec
// validation still runs afterward, so a current Form never inherits the
// historical pattern.
func ValidRetainedCredentialFreeHTTPSURL(value string) bool {
	if !retainedCredentialFreeHTTPSURLPattern.MatchString(value) {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil &&
		parsed.Scheme == "https" &&
		parsed.Host != "" &&
		parsed.User == nil &&
		parsed.RawQuery == "" &&
		parsed.Fragment == ""
}

// NameMaxLength bounds every portable Resource name.
const NameMaxLength = 63

// MaxPortableGeneration is the largest desired-state generation representable
// without overflow by every v1 host, client, and provider fence.
const MaxPortableGeneration int64 = 9223372036854775807

// Pattern returns the RE2 pattern for a grammar, and whether one applies.
func (g Grammar) Pattern() (string, bool) {
	switch g {
	case GrammarToken:
		return PatternToken, true
	case GrammarVersion:
		return PatternVersion, true
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
	case GrammarDNSRelativeName:
		return PatternDNSRelativeName, true
	case GrammarOCIDigest:
		return PatternOCIDigest, true
	case GrammarMailbox:
		return PatternMailbox, true
	case GrammarMailboxLocalPart:
		return PatternMailboxLocalPart, true
	case GrammarHTTPSURL:
		return PatternHTTPSURL, true
	case GrammarCredentialFreeHTTPSURL:
		return PatternCredentialFreeHTTPSURL, true
	case GrammarRecordData:
		return PatternRecordData, true
	case GrammarRelativePath:
		return PatternRelativePath, true
	case GrammarSHA256:
		return PatternSHA256, true
	default:
		return "", false
	}
}

// Message is the human-readable constraint used by provider diagnostics.
func (g Grammar) Message(field string) string {
	switch g {
	case GrammarToken:
		return field + " must use the portable capability-token grammar"
	case GrammarVersion:
		return field + " must use the portable version-token grammar"
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
	case GrammarDNSRelativeName:
		return field + " must be a relative DNS name or @"
	case GrammarOCIDigest:
		return field + " must be an OCI reference pinned by sha256 digest"
	case GrammarMailbox:
		return field + " must be an email address"
	case GrammarMailboxLocalPart:
		return field + " must be a portable mailbox local part"
	case GrammarHTTPSURL:
		return field + " must be an absolute https URL"
	case GrammarCredentialFreeHTTPSURL:
		return field + " must be an absolute credential-free https URL without userinfo, query, or fragment"
	case GrammarRecordData:
		return field + " must not be blank"
	case GrammarRelativePath:
		return field + " must be a non-escaping relative artifact path"
	case GrammarSHA256:
		return field + " must be a sha256 digest"
	default:
		return field + " is invalid"
	}
}

// DesiredSchema derives the Draft 2020-12 desired schema of a Form from its
// declared fields. It is the single source the Form Definition is built from.
func (k Kind) DesiredSchema() map[string]any {
	if k.exactDefinition != nil {
		return cloneValue(k.exactDefinition.value.DesiredSchema).(map[string]any)
	}
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
		for key, value := range connectionDefinitions(k) {
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
	if len(k.Constraints) > 0 {
		constraints := make([]any, 0, len(k.Constraints))
		for _, constraint := range k.Constraints {
			constraints = append(constraints, constraint.jsonSchema(properties, required))
		}
		schema["allOf"] = constraints
	}
	return schema
}

func (constraint ConditionalConstraint) jsonSchema(baseProperties map[string]any, required []string) map[string]any {
	whenValue := map[string]any{"enum": constraint.WhenValues}
	if len(constraint.WhenValues) == 1 {
		whenValue = map[string]any{"const": constraint.WhenValues[0]}
	}
	ifProperties := cloneValue(baseProperties).(map[string]any)
	ifProperties[constraint.WhenField] = whenValue
	thenProperties := cloneValue(baseProperties).(map[string]any)
	for field, pattern := range constraint.StringSetItemPatterns {
		fieldSchema := cloneValue(thenProperties[field]).(map[string]any)
		items := cloneValue(fieldSchema["items"]).(map[string]any)
		items["pattern"] = pattern
		fieldSchema["items"] = items
		thenProperties[field] = fieldSchema
	}
	for field, maxItems := range constraint.StringSetMaxItems {
		fieldSchema := cloneValue(thenProperties[field]).(map[string]any)
		fieldSchema["maxItems"] = maxItems
		thenProperties[field] = fieldSchema
	}
	for _, field := range constraint.ForbidFields {
		delete(thenProperties, field)
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": required, "properties": cloneValue(baseProperties),
		"if": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": required, "properties": ifProperties,
		},
		"then": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": required, "properties": thenProperties,
		},
	}
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
	case TypeStringMap:
		// A pure typed map is the only open-key escape the portable schema
		// policy allows, and its keys are held to the same forbidden-field
		// vocabulary as declared fields.
		return map[string]any{
			"type": "object", "propertyNames": portableMapKeys(),
			"additionalProperties": map[string]any{"type": "string", "maxLength": 4096},
		}
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
		if f.Grammar == GrammarHTTPSURL || f.Grammar == GrammarCredentialFreeHTTPSURL {
			schema["format"] = "uri"
		}
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

// NegativeCase is one input a conforming host must reject, named after the
// field whose constraint it violates.
type NegativeCase struct {
	Name    string
	Field   Field
	Desired map[string]any
}

// ConstraintViolation is one cross-field condition rejected by both the
// generated desired schema and the typed provider before it contacts a host.
type ConstraintViolation struct {
	WireField string
	Detail    string
}

// ConditionalViolations evaluates the same small cross-field vocabulary that
// DesiredSchema projects into Draft 2020-12. Keeping this evaluator beside the
// schema derivation prevents the provider from accepting a value that a Form
// Package requires every host to reject.
func (k Kind) ConditionalViolations(desired map[string]any) ([]ConstraintViolation, error) {
	var violations []ConstraintViolation
	for _, constraint := range k.Constraints {
		when, ok := desired[constraint.WhenField].(string)
		if !ok || !containsString(constraint.WhenValues, when) {
			continue
		}
		for _, field := range constraint.ForbidFields {
			if _, present := desired[field]; present {
				violations = append(violations, ConstraintViolation{
					WireField: field,
					Detail: fmt.Sprintf("%s is not valid when %s is %s",
						field, constraint.WhenField, when),
				})
			}
		}
		for field, pattern := range constraint.StringSetItemPatterns {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("%s constraint %s has invalid pattern for %s: %w",
					k.Kind, constraint.Name, field, err)
			}
			for _, member := range stringMembers(desired[field]) {
				if compiled.MatchString(member) {
					continue
				}
				violations = append(violations, ConstraintViolation{
					WireField: field,
					Detail: fmt.Sprintf("%s must match the %s contract when %s is %s",
						field, constraint.Name, constraint.WhenField, when),
				})
				break
			}
		}
		for field, maxItems := range constraint.StringSetMaxItems {
			if members := stringMembers(desired[field]); len(members) > maxItems {
				violations = append(violations, ConstraintViolation{
					WireField: field,
					Detail: fmt.Sprintf("%s accepts at most %d item(s) when %s is %s",
						field, maxItems, constraint.WhenField, when),
				})
			}
		}
	}
	return violations, nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func stringMembers(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, member := range typed {
			if text, ok := member.(string); ok {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

// NegativeCases builds one rejectable input per declared counter-example, so
// every constraint a Form states has something that proves it is enforced.
// It also removes every required desired key one at a time. Requiredness is a
// portable contract in its own right: a schema, provider, or host that only
// exercises invalid values could otherwise accept an omitted required value.
func (k Kind) NegativeCases() ([]NegativeCase, error) {
	var cases []NegativeCase
	appendMissing := func(name, wire string) {
		desired := cloneValue(k.CanonicalDesired()).(map[string]any)
		delete(desired, wire)
		cases = append(cases, NegativeCase{Name: "missing-" + name, Desired: desired})
	}
	appendMissing("name", "name")
	if k.Artifact {
		appendMissing("source", "source")
	}
	if k.Connections == ConnectionsRequired {
		appendMissing("connections", "connections")
	}
	for _, field := range k.Fields {
		if field.Required {
			appendMissing(field.HCL, field.Wire)
		}
	}
	for _, field := range k.Fields {
		counter, ok := field.counterExample()
		if !ok {
			continue
		}
		desired := cloneValue(k.CanonicalDesired()).(map[string]any)
		desired[field.Wire] = cloneValue(counter)
		cases = append(cases, NegativeCase{Name: field.HCL, Field: field, Desired: desired})
	}
	if k.Artifact {
		for _, invalid := range []struct {
			name string
			url  string
		}{
			{name: "artifact-url-userinfo", url: "https://builder@artifacts.portable-conformance.invalid/app.tar"},
			{name: "artifact-url-query", url: "https://artifacts.portable-conformance.invalid/app.tar?download=1"},
			{name: "artifact-url-fragment", url: "https://artifacts.portable-conformance.invalid/app.tar#archive"},
		} {
			desired := cloneValue(k.CanonicalDesired()).(map[string]any)
			source := cloneValue(desired["source"]).(map[string]any)
			source["artifactUrl"] = invalid.url
			desired["source"] = source
			cases = append(cases, NegativeCase{Name: invalid.name, Desired: desired})
		}
		desired := cloneValue(k.CanonicalDesired()).(map[string]any)
		source := cloneValue(desired["source"]).(map[string]any)
		source["artifactSha256"] = "not-a-sha256"
		desired["source"] = source
		cases = append(cases, NegativeCase{Name: "artifact-source", Desired: desired})
	}
	if k.Connections == ConnectionsRequired {
		desired := cloneValue(k.CanonicalDesired()).(map[string]any)
		desired["connections"] = map[string]any{}
		cases = append(cases, NegativeCase{Name: "connections-required", Desired: desired})
	}
	if k.MaxConnections > 0 {
		desired := cloneValue(k.CanonicalDesired()).(map[string]any)
		connections := desired["connections"].(map[string]any)
		var example any
		for _, connection := range connections {
			example = cloneValue(connection)
			break
		}
		for len(connections) <= k.MaxConnections {
			connections[fmt.Sprintf("overflow%d", len(connections))] = cloneValue(example)
		}
		cases = append(cases, NegativeCase{Name: "connections-cardinality", Desired: desired})
	}
	for _, constraint := range k.Constraints {
		if constraint.Name == "" || len(constraint.CounterExample) == 0 {
			return nil, fmt.Errorf("%s declares an unprovable conditional constraint", k.Kind)
		}
		desired := cloneValue(k.CanonicalDesired()).(map[string]any)
		for field, value := range constraint.CounterExample {
			desired[field] = cloneValue(value)
		}
		cases = append(cases, NegativeCase{Name: constraint.Name, Desired: desired})
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("%s declares no counter-example field", k.Kind)
	}
	return cases, nil
}

// NegativeDesired is the first rejectable input, retained for callers that
// only need to know a Form has one.
func (k Kind) NegativeDesired() (map[string]any, error) {
	cases, err := k.NegativeCases()
	if err != nil {
		return nil, err
	}
	return cases[0].Desired, nil
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
	if k.exactDefinition != nil {
		return append([]string(nil), k.exactDefinition.value.ImmutableFields...)
	}
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
	if k.exactDefinition != nil {
		return cloneValue(k.exactDefinition.value.OutputSchema).(map[string]any)
	}
	required := []string{"generation", "id", "kind", "name", "portability"}
	properties := map[string]any{
		"id":          map[string]any{"type": "string", "pattern": `^` + k.Kind + `/[a-z][a-z0-9-]{0,62}$`},
		"kind":        map[string]any{"type": "string", "const": k.Kind},
		"name":        map[string]any{"type": "string", "minLength": 1, "maxLength": NameMaxLength, "pattern": PatternName},
		"generation":  map[string]any{"type": "integer", "minimum": 1, "maximum": MaxPortableGeneration},
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

// ForeignKindObserved is the negative observed fixture proving that a host
// cannot substitute another Form kind while retaining an otherwise valid
// portable resource name and lifecycle document.
func (k Kind) ForeignKindObserved() map[string]any {
	observed := cloneValue(k.CanonicalObserved()).(map[string]any)
	observed["id"] = "OtherKind/" + k.FixtureName()
	return observed
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

// ObservedSchema is this Form's lifecycle status contract. The status shape is
// shared, but its id is kind-bound; equality with the requested Resource name
// is enforced by the provider/host lifecycle that has both documents.
func (k Kind) ObservedSchema() map[string]any {
	if k.exactDefinition != nil {
		return cloneValue(k.exactDefinition.value.ObservedSchema).(map[string]any)
	}
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
		"additionalProperties": false,
		"required":             []string{"driftedFields", "generation", "id", "imported", "portability", "ready"},
		"properties": map[string]any{
			"id":          map[string]any{"type": "string", "pattern": `^` + k.Kind + `/[a-z][a-z0-9-]{0,62}$`},
			"ready":       map[string]any{"type": "boolean"},
			"generation":  map[string]any{"type": "integer", "minimum": 1, "maximum": MaxPortableGeneration},
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

func connectionDefinitions(kind Kind) map[string]any {
	connections := map[string]any{
		"type": "object", "propertyNames": portableMapKeys(),
		"additionalProperties": map[string]any{"$ref": "#/$defs/connection"},
	}
	if kind.Connections == ConnectionsRequired {
		connections["minProperties"] = 1
	}
	if kind.MaxConnections > 0 {
		connections["maxProperties"] = kind.MaxConnections
	}
	return map[string]any{
		"connections": connections,
		"connection": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"permissions", "projection", "resource"},
			"properties": map[string]any{
				"resource": map[string]any{"type": "string", "pattern": PatternResourceRef},
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
		"artifactSource": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"artifactMediaType", "artifactSha256", "artifactUrl"},
			"properties": map[string]any{
				"artifactUrl":       map[string]any{"type": "string", "format": "uri", "pattern": PatternCredentialFreeHTTPSURL},
				"artifactSha256":    map[string]any{"$ref": "#/$defs/sha256"},
				"artifactMediaType": map[string]any{"type": "string", "pattern": PatternMediaType},
			},
		},
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

// PortableMapKeyPattern is the key grammar every portable typed map uses.
const PortableMapKeyPattern = `^[A-Za-z][A-Za-z0-9._-]{0,63}$`

// counterExample returns the value this field's constraint must reject.
//
// A declared CounterExample wins; otherwise one is derived from the constraint
// the field already states. Deriving rather than hand-writing means a field
// cannot state a constraint and quietly ship no proof that it is enforced, and
// a wrong derivation fails loudly at generation because the package verifier
// requires every negative fixture to be rejected by the schema.
func (f Field) CounterExampleValue() (any, bool) { return f.counterExample() }

func (f Field) counterExample() (any, bool) {
	if f.CounterExample != nil {
		return f.CounterExample, true
	}
	switch f.Type {
	case TypeBool:
		return nil, false
	case TypeInt:
		if f.Min != nil {
			return *f.Min - 1, true
		}
		if f.Max != nil {
			return *f.Max + 1, true
		}
		return nil, false
	case TypeIntSet:
		if f.Min != nil {
			return []any{*f.Min - 1}, true
		}
		if f.Max != nil {
			return []any{*f.Max + 1}, true
		}
		return nil, false
	case TypeStringMap:
		// Keys carry the portable map-key grammar, so a key that cannot be
		// parsed is the constraint worth proving.
		return map[string]any{"1 invalid key": "value"}, true
	case TypeStringSet:
		if member, ok := invalidStringValue(f); ok {
			return []any{member}, true
		}
		return nil, false
	default:
		return invalidStringValue(f)
	}
}

func invalidStringValue(f Field) (any, bool) {
	if len(f.Enum) > 0 {
		return "not-a-declared-choice", true
	}
	switch f.Grammar {
	case GrammarToken, GrammarClass:
		return "not a token", true
	case GrammarVersion:
		return "not a version", true
	case GrammarTimezone:
		return "not a timezone", true
	case GrammarCron:
		return "0 0 * *", true
	case GrammarHostname, GrammarDomain:
		return "not a hostname", true
	case GrammarPath:
		return "relative/path", true
	case GrammarCIDR:
		return "10.0.0.0", true
	case GrammarDNSRelativeName:
		return "not/a/name", true
	case GrammarOCIDigest:
		return "registry.invalid/image:latest", true
	case GrammarMailbox:
		return "no-at-sign", true
	case GrammarMailboxLocalPart:
		return "bad@local", true
	case GrammarHTTPSURL:
		return "http://insecure.invalid/callback", true
	case GrammarCredentialFreeHTTPSURL:
		return "https://artifact.invalid/archive.tar?download=1", true
	case GrammarRecordData:
		return " ", true
	case GrammarRelativePath:
		return "../escape", true
	case GrammarSHA256:
		return "not-a-sha256", true
	default:
		return nil, false
	}
}
