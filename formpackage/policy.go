package formpackage

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"unicode"
)

// forbiddenNormalizedFields is intentionally exact. Substring matching is
// unsafe here: for example, the legitimate JSON Schema keyword "description"
// contains "script". Variants with camel/snake/kebab boundaries are covered
// by forbiddenFieldTokens below.
var forbiddenNormalizedFields = stringSet(
	// Credentials, authentication, and secret-bearing connection material.
	"credential", "credentials", "credentialid", "credentialids", "credentialref", "credentialrefs", "credentialname", "credentialvalue",
	"secret", "secrets", "secretid", "secretids", "secretref", "secretrefs", "secretname", "secretvalue",
	"password", "passwords", "passphrase", "privatekey", "privatekeyid", "privatekeyref", "apikey", "apikeyid", "apikeyref",
	"apikeyvalue", "privatekeypem", "sshprivatekey",
	"token", "tokens", "tokenid", "tokenref", "accesstoken", "refreshtoken", "idtoken", "bearertoken",
	"authorization", "authorizationheader", "authheader", "bearer", "oauth", "oauthclient", "oauthclientid", "oauthclientsecret", "oidcclientsecret",
	"sessioncookie", "sessiontoken", "cookie", "cookies", "connectionstring", "signingkey", "sshkey",

	// Operator, backend, account, placement, and live capacity authority.
	"operator", "operators", "operatorid", "operatorpolicy", "account", "accounts", "accountid",
	"target", "targets", "targetid", "targetpool", "targetpoolid", "poolid",
	"capacity", "activecapacity", "regioncapacity", "backendmanager", "managerid", "manageridentifier",
	"provider", "providerid", "providername", "providerconfig", "backend", "backendid",
	"implementationid", "selectedimplementation", "region", "regions", "regionid", "zone", "zones", "zoneid", "placement",

	// Commercial and service-operation authority.
	"price", "prices", "pricing", "priceid", "unitprice", "monthlyprice", "sku", "skus",
	"billing", "billingplan", "billingaccount", "invoice", "invoices", "invoiceid",
	"payment", "payments", "paymentid", "paymentmethod", "paymentmethods",
	"currency", "currencies", "currencycode", "tax", "taxes", "taxcode", "taxrate",
	"quota", "quotas", "sla", "slapolicy", "servicelevelagreement", "supportpolicy",
	"serviceoffering", "serviceofferings", "serviceofferingid", "subscription", "subscriptions", "entitlement", "entitlements",

	// Executable or host-extension material.
	"binary", "code", "exec", "executable", "command", "commands", "script", "scripts",
	"sourcecode", "validationcode", "adapter", "adaptercode", "runtimecode", "bytecode",
	"webassembly", "wasm", "plugin", "plugins",
)

var forbiddenFieldTokens = stringSet(
	"credential", "secret", "password", "passphrase", "token", "authorization", "bearer", "oauth", "cookie",
	"operator", "account", "target", "capacity", "provider", "backend", "implementation", "region", "zone", "placement",
	"price", "pricing", "sku", "billing", "invoice", "payment", "currency", "tax", "quota", "sla", "subscription", "entitlement",
	"binary", "code", "exec", "executable", "command", "script", "bytecode", "wasm", "plugin",
)

// Plurals are listed rather than derived by trimming suffixes. Generic
// singularization would turn unrelated portable words into sensitive tokens
// and recreate the substring false positives this policy avoids.
var forbiddenFieldPluralTokens = stringSet(
	"credentials", "secrets", "passwords", "passphrases", "tokens", "authorizations", "bearers", "oauths", "cookies",
	"operators", "accounts", "targets", "capacities", "providers", "backends", "implementations", "regions", "zones", "placements",
	"prices", "pricings", "skus", "billings", "invoices", "payments", "currencies", "taxes", "quotas", "slas", "subscriptions", "entitlements",
	"binaries", "codes", "execs", "executables", "commands", "scripts", "bytecodes", "wasms", "plugins",
)

// Some sensitive concepts are compounds whose individual words are useful in
// portable schemas. Match reviewed boundary-delimited token sequences instead
// of unsafe substrings: "apiKeyValue" is forbidden, while "description" and
// prose containing "API key" are unaffected because only field names enter
// this function.
var forbiddenFieldTokenSequences = [][]string{
	{"api", "key"},
	{"private", "key"},
	{"ssh", "key"},
	{"signing", "key"},
	{"service", "offering"},
	{"backend", "manager"},
	{"manager", "id"},
	{"manager", "identifier"},
}

var forbiddenNormalizedCompoundBases = []string{
	"apikey",
	"privatekey",
	"sshkey",
	"sshprivatekey",
	"signingkey",
	"serviceoffering",
	"backendmanager",
	"managerid",
	"manageridentifier",
}

var forbiddenCompoundQualifiers = stringSet(
	"id", "ids", "identifier", "identifiers", "ref", "refs", "name", "names",
	"value", "values", "pem", "material", "fingerprint", "header", "path", "file",
	"config", "configuration", "label", "labels",
)

func rejectForbiddenContent(value any, location string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.HasSuffix(location, "desiredSchema.properties") &&
				isReviewedBoundedDesiredFieldSchema(key, child) {
				if err := rejectForbiddenContent(child, location+"."+key); err != nil {
					return err
				}
				continue
			}
			if isForbiddenFieldName(key) {
				return fmt.Errorf("forbidden field %q at %s", key, location)
			}
			if err := rejectForbiddenContent(child, location+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := rejectForbiddenContent(child, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

// rejectForbiddenDesiredFixtureContent applies the same reviewed top-level
// desired-property exceptions as the Definition schema, but only when that
// exact schema proves the field is bounded and closed. Nested values still run
// through the ordinary lexical policy, so an admitted `command` list cannot
// hide a script/backend command object beneath it.
func rejectForbiddenDesiredFixtureContent(value any, desiredSchema map[string]any, location string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return rejectForbiddenContent(value, location)
	}
	properties, _ := desiredSchema["properties"].(map[string]any)
	for key, child := range object {
		if fieldSchema, declared := properties[key]; declared && isReviewedBoundedDesiredFieldSchema(key, fieldSchema) {
			if err := rejectForbiddenContent(child, location+"."+key); err != nil {
				return err
			}
			continue
		}
		if isForbiddenFieldName(key) {
			return fmt.Errorf("forbidden field %q at %s", key, location)
		}
		if err := rejectForbiddenContent(child, location+"."+key); err != nil {
			return err
		}
	}
	return nil
}

func isReviewedBoundedDesiredFieldSchema(name string, value any) bool {
	switch name {
	case "command":
		return isBoundedDeclarativeCommandSchema(value)
	case "concurrencyTarget":
		return isBoundedDeclarativeIntegerSchema(value)
	case "target":
		return isCanonicalResourceTargetSchema(value) || isClosedTaggedTargetUnionSchema(value)
	default:
		return false
	}
}

// isCanonicalResourceTargetSchema recognizes the exact closed relation value
// emitted by the current authoring model. The field name "target" is otherwise
// forbidden because an open backend/operator target is not portable Form
// state. This exception is therefore conditional on both halves of the
// contract being present: the closed {apiVersion,kind,name} value and exactly
// one reviewed, digest-bound target-contract annotation.
func isCanonicalResourceTargetSchema(value any) bool {
	schema, ok := value.(map[string]any)
	if !ok || schema["type"] != "object" || schema["additionalProperties"] != false {
		return false
	}
	if !hasOnlyKeys(schema,
		"type", "additionalProperties", "required", "properties", "description", "default",
		"x-takoform-target-formrefs", "x-takoform-required-interface", "x-takoform-required-entrypoint",
	) || !sameStringSet(schema["required"], "apiVersion", "kind", "name") {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || !hasOnlyKeys(properties, "apiVersion", "kind", "name") || len(properties) != 3 {
		return false
	}
	apiVersion, apiVersionOK := closedStringConst(properties["apiVersion"])
	kind, kindOK := closedStringConst(properties["kind"])
	if !apiVersionOK || !kindOK || !isCanonicalResourceNameSchema(properties["name"]) {
		return false
	}
	if description, present := schema["description"]; present {
		if text, ok := description.(string); !ok || text == "" {
			return false
		}
	}
	if entrypoint, present := schema["x-takoform-required-entrypoint"]; present {
		if text, ok := entrypoint.(string); !ok || text == "" {
			return false
		}
	}
	if defaultValue, present := schema["default"]; present && !isCanonicalResourceTargetValue(defaultValue, apiVersion, kind) {
		return false
	}

	exactRefs, hasExactRefs := schema["x-takoform-target-formrefs"]
	requiredInterface, hasRequiredInterface := schema["x-takoform-required-interface"]
	if hasExactRefs == hasRequiredInterface {
		return false
	}
	if hasExactRefs {
		return isReviewedExactFormRefs(exactRefs, apiVersion, kind)
	}
	return isReviewedRequiredInterface(requiredInterface)
}

// isClosedTaggedTargetUnionSchema recognizes a target whose discriminator
// selects one of a bounded set of closed object branches. Every branch must
// contain at least one canonical ResourceTarget carrying its own reviewed
// exact-Form or required-Interface annotation. Merely being a oneOf, or merely
// containing an object named target, is not enough.
func isClosedTaggedTargetUnionSchema(value any) bool {
	schema, ok := value.(map[string]any)
	if !ok || !hasOnlyKeys(schema, "oneOf", "x-takoform-discriminator", "description", "default") {
		return false
	}
	discriminator, ok := schema["x-takoform-discriminator"].(string)
	if !ok || discriminator == "" {
		return false
	}
	if description, present := schema["description"]; present {
		if text, ok := description.(string); !ok || text == "" {
			return false
		}
	}
	branches, ok := schema["oneOf"].([]any)
	if !ok || len(branches) < 2 || len(branches) > 16 {
		return false
	}
	tags := make(map[string]struct{}, len(branches))
	for _, branchValue := range branches {
		branch, ok := branchValue.(map[string]any)
		if !ok || branch["type"] != "object" || branch["additionalProperties"] != false ||
			!hasOnlyKeys(branch, "type", "additionalProperties", "required", "properties") {
			return false
		}
		properties, ok := branch["properties"].(map[string]any)
		if !ok || !stringSetContains(branch["required"], discriminator) {
			return false
		}
		tag, ok := closedStringConst(properties[discriminator])
		if !ok {
			return false
		}
		if _, duplicate := tags[tag]; duplicate {
			return false
		}
		tags[tag] = struct{}{}
		hasReviewedTarget := false
		for name, property := range properties {
			if name != discriminator && containsCanonicalResourceTargetSchema(property) {
				hasReviewedTarget = true
				break
			}
		}
		if !hasReviewedTarget {
			return false
		}
	}
	return true
}

func containsCanonicalResourceTargetSchema(value any) bool {
	if isCanonicalResourceTargetSchema(value) {
		return true
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		for _, property := range properties {
			if containsCanonicalResourceTargetSchema(property) {
				return true
			}
		}
	}
	if items, present := schema["items"]; present && containsCanonicalResourceTargetSchema(items) {
		return true
	}
	return false
}

func closedStringConst(value any) (string, bool) {
	schema, ok := value.(map[string]any)
	if !ok || len(schema) != 2 || schema["type"] != "string" {
		return "", false
	}
	constant, ok := schema["const"].(string)
	return constant, ok && constant != ""
}

func isCanonicalResourceNameSchema(value any) bool {
	schema, ok := value.(map[string]any)
	if !ok || !hasOnlyKeys(schema, "type", "minLength", "maxLength", "pattern") || len(schema) != 4 || schema["type"] != "string" {
		return false
	}
	minimum, minimumOK := wholeNumber(schema["minLength"])
	maximum, maximumOK := wholeNumber(schema["maxLength"])
	pattern, patternOK := schema["pattern"].(string)
	return minimumOK && maximumOK && minimum == 1 && maximum >= minimum && patternOK && pattern != ""
}

func isCanonicalResourceTargetValue(value any, apiVersion, kind string) bool {
	target, ok := value.(map[string]any)
	if !ok || len(target) != 3 || !hasOnlyKeys(target, "apiVersion", "kind", "name") {
		return false
	}
	name, nameOK := target["name"].(string)
	return target["apiVersion"] == apiVersion && target["kind"] == kind && nameOK && name != ""
}

func isReviewedExactFormRefs(value any, apiVersion, kind string) bool {
	refs, ok := value.([]any)
	if !ok || len(refs) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(refs))
	for _, value := range refs {
		ref, ok := value.(map[string]any)
		if !ok || len(ref) != 4 || !hasOnlyKeys(ref, "apiVersion", "kind", "definitionVersion", "schemaDigest") ||
			ref["apiVersion"] != apiVersion || ref["kind"] != kind {
			return false
		}
		version, versionOK := ref["definitionVersion"].(string)
		digest, digestOK := ref["schemaDigest"].(string)
		if !versionOK || version == "" || !digestOK || !isCanonicalSHA256(digest) {
			return false
		}
		identity := version + "\x00" + digest
		if _, duplicate := seen[identity]; duplicate {
			return false
		}
		seen[identity] = struct{}{}
	}
	return true
}

func isReviewedRequiredInterface(value any) bool {
	ref, ok := value.(map[string]any)
	if !ok || len(ref) != 4 || !hasOnlyKeys(ref, "apiVersion", "name", "version", "schemaDigest") {
		return false
	}
	for _, key := range []string{"apiVersion", "name", "version"} {
		text, ok := ref[key].(string)
		if !ok || text == "" {
			return false
		}
	}
	digest, ok := ref["schemaDigest"].(string)
	return ok && isCanonicalSHA256(digest)
}

func isCanonicalSHA256(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, char := range value[len("sha256:"):] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func hasOnlyKeys(value map[string]any, allowed ...string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		set[key] = struct{}{}
	}
	for key := range value {
		if _, ok := set[key]; !ok {
			return false
		}
	}
	return true
}

func sameStringSet(value any, want ...string) bool {
	values, ok := stringValues(value)
	if !ok || len(values) != len(want) {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range want {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return len(seen) == len(want)
}

func stringSetContains(value any, want string) bool {
	values, ok := stringValues(value)
	if !ok {
		return false
	}
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringValues(value any) ([]string, bool) {
	switch values := value.(type) {
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, false
			}
			result = append(result, text)
		}
		return result, true
	case []string:
		return values, true
	default:
		return nil, false
	}
}

// isBoundedDeclarativeCommandSchema recognizes only the process-configuration
// shape used by a Container-like Form: an ordered, bounded list of bounded,
// non-empty-pattern strings. It does not exempt a command string, a script
// body, an unbounded argv vector, or a runtime document containing a command
// member. Those remain executable payload vocabulary and fail closed.
func isBoundedDeclarativeCommandSchema(value any) bool {
	schema, ok := value.(map[string]any)
	if !ok || schema["type"] != "array" || !positiveWholeNumber(schema["maxItems"]) {
		return false
	}
	items, ok := schema["items"].(map[string]any)
	if !ok || items["type"] != "string" || !positiveWholeNumber(items["maxLength"]) {
		return false
	}
	pattern, ok := items["pattern"].(string)
	return ok && pattern != ""
}

func isBoundedDeclarativeIntegerSchema(value any) bool {
	schema, ok := value.(map[string]any)
	if !ok || schema["type"] != "integer" {
		return false
	}
	minimum, minimumOK := wholeNumber(schema["minimum"])
	maximum, maximumOK := wholeNumber(schema["maximum"])
	return minimumOK && maximumOK && minimum <= maximum
}

func positiveWholeNumber(value any) bool {
	number, ok := wholeNumber(value)
	return ok && number > 0
}

func wholeNumber(value any) (float64, bool) {
	if value == nil {
		return 0, false
	}
	if number, ok := value.(json.Number); ok {
		integer, err := number.Int64()
		return float64(integer), err == nil
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(reflected.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(reflected.Uint()), true
	case reflect.Float32, reflect.Float64:
		number := reflected.Float()
		return number, !math.IsInf(number, 0) && !math.IsNaN(number) && math.Trunc(number) == number
	default:
		return 0, false
	}
}

// reviewedAnnotationFields is the closed set of `x-takoform-*` JSON Schema
// annotations this specification defines. It is an exemption from the
// sensitive-name policy below, and a deliberately EXACT one.
//
// That policy exists to keep author-declared fields from carrying credentials,
// operator authority, or commercial policy: an author who writes `target` is
// almost always naming a backend the portable lane must not know about. These
// seven names are not author-declared at all — they are specification
// vocabulary with fixed meanings, emitted by the authoring model and read by
// every host — and one of them, `x-takoform-target-formrefs`, uses `target` in
// its portable sense: the resource a relation points at. Listing the exact keys
// rather than exempting the whole `x-takoform-` prefix keeps a future
// annotation from smuggling a sensitive name in behind the namespace.
var reviewedAnnotationFields = stringSet(
	"x-takoform-fieldPolicy",
	"x-takoform-binding",
	"x-takoform-target-formrefs",
	"x-takoform-required-interface",
	"x-takoform-discriminator",
	"x-takoform-standard-services",
	"x-takoform-required-entrypoint",
)

func isForbiddenFieldName(value string) bool {
	if _, reviewed := reviewedAnnotationFields[value]; reviewed {
		return false
	}
	normalized := normalizeFieldName(value)
	if _, forbidden := forbiddenNormalizedFields[normalized]; forbidden {
		return true
	}
	for _, singular := range forbiddenNormalizedCompoundBases {
		for _, base := range []string{singular, singular + "s"} {
			if normalized == base {
				return true
			}
			if !strings.HasPrefix(normalized, base) {
				continue
			}
			if _, forbidden := forbiddenCompoundQualifiers[strings.TrimPrefix(normalized, base)]; forbidden {
				return true
			}
		}
	}
	tokens := splitFieldNameTokens(value)
	for _, token := range tokens {
		if isForbiddenToken(token) {
			return true
		}
	}
	for _, sequence := range forbiddenFieldTokenSequences {
		if containsTokenSequence(tokens, sequence) {
			return true
		}
	}
	return false
}

func containsTokenSequence(tokens, sequence []string) bool {
	if len(sequence) == 0 || len(tokens) < len(sequence) {
		return false
	}
	for start := 0; start <= len(tokens)-len(sequence); start++ {
		matched := true
		for offset := range sequence {
			if !matchesCompoundToken(tokens[start+offset], sequence[offset]) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func isForbiddenToken(token string) bool {
	if _, forbidden := forbiddenFieldTokens[token]; forbidden {
		return true
	}
	_, forbidden := forbiddenFieldPluralTokens[token]
	return forbidden
}

func matchesCompoundToken(actual, singular string) bool {
	if actual == singular {
		return true
	}
	switch singular {
	case "id":
		return actual == "ids"
	case "identifier":
		return actual == "identifiers"
	case "key":
		return actual == "keys"
	case "manager":
		return actual == "managers"
	case "offering":
		return actual == "offerings"
	default:
		return false
	}
}

func normalizeFieldName(value string) string {
	var normalized strings.Builder
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(unicode.ToLower(character))
		}
	}
	return normalized.String()
}

func splitFieldNameTokens(value string) []string {
	runes := []rune(value)
	tokens := []string{}
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(current.String()))
		current.Reset()
	}
	for index, character := range runes {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) {
			flush()
			continue
		}
		if current.Len() > 0 {
			previous := runes[index-1]
			nextIsLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			caseBoundary := unicode.IsUpper(character) && (unicode.IsLower(previous) || unicode.IsDigit(previous) || (unicode.IsUpper(previous) && nextIsLower))
			digitBoundary := unicode.IsDigit(character) != unicode.IsDigit(previous)
			if caseBoundary || digitBoundary {
				flush()
			}
		}
		current.WriteRune(character)
	}
	flush()
	return tokens
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
