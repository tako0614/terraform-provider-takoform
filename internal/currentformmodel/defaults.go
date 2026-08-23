package currentformmodel

// defaults.go is the single portable-default implementation of the v1beta1
// lane. It answers exactly two questions, and every surface answers them
// through this file so no two of them can drift:
//
//   - is a declared Default well formed for the field that declares it
//     (authoring time, validateFieldDefault); and
//   - what is the EFFECTIVE spec of a desired document that omits defaulted
//     properties (wire time, MaterializeDefaults).
//
// Materialization happens once, at the host's entry point, before the spec is
// digested, stored, or echoed: the effective spec IS the wire spec, so
// omitting a defaulted field and writing its default produce byte-identical
// desired state, the same specDigest, and the same generation.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
)

// MaterializeDefaults returns the effective desired spec of one resource: the
// given spec plus every top-level property whose Form declares a `default` in
// desiredSchema and which the spec does not carry.
//
// The contract, in full:
//
//   - a property that is PRESENT is never overwritten, including when the
//     written value equals the default;
//   - inserted values are deep copies, so a caller can never mutate the
//     Definition through the spec it received;
//   - inserted numbers are json.Number in canonical decimal text, because host
//     bodies decode with UseNumber and portable semantic rules type-assert
//     json.Number;
//   - the function is idempotent: materializing twice equals materializing
//     once;
//   - the argument spec is never mutated; the result is always a fresh map.
func MaterializeDefaults(desiredSchema map[string]any, spec map[string]any) map[string]any {
	properties, _ := desiredSchema["properties"].(map[string]any)
	out := make(map[string]any, len(spec)+len(properties))
	for key, value := range spec {
		out[key] = value
	}
	for name, raw := range properties {
		property, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value, declared := property["default"]
		if !declared {
			continue
		}
		if _, present := out[name]; present {
			continue
		}
		out[name] = canonicalJSONValue(value)
	}
	return out
}

// canonicalJSONValue deep-copies one JSON value and normalizes every number to
// json.Number in canonical decimal text.
func canonicalJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = canonicalJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, canonicalJSONValue(item))
		}
		return out
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case json.Number:
		return typed
	case int:
		return json.Number(strconv.FormatInt(int64(typed), 10))
	case int32:
		return json.Number(strconv.FormatInt(int64(typed), 10))
	case int64:
		return json.Number(strconv.FormatInt(typed, 10))
	case uint:
		return json.Number(strconv.FormatUint(uint64(typed), 10))
	case uint32:
		return json.Number(strconv.FormatUint(uint64(typed), 10))
	case uint64:
		return json.Number(strconv.FormatUint(typed, 10))
	case float32:
		return json.Number(strconv.FormatFloat(float64(typed), 'f', -1, 32))
	case float64:
		return json.Number(strconv.FormatFloat(typed, 'f', -1, 64))
	default:
		return value
	}
}

// validateFieldDefault proves a declared Default is a legal value of the field
// that declares it. A default that its own field's schema would reject is the
// worst possible portability defect: every host would materialize an invalid
// document.
func validateFieldDefault(kind string, field Field) error {
	if field.Default == nil {
		return nil
	}
	fail := func(reason string) error {
		return fmt.Errorf("form %s field %s default %v %s", kind, field.Wire, field.Default, reason)
	}
	switch field.Kind {
	case KindBoolean:
		if _, ok := field.Default.(bool); !ok {
			return fail("is not a boolean")
		}
	case KindInteger:
		value, ok := defaultInteger(field.Default)
		if !ok {
			return fail("is not an integer")
		}
		if field.Min != nil && value < *field.Min {
			return fail(fmt.Sprintf("is below the declared minimum %d", *field.Min))
		}
		if field.Max != nil && value > *field.Max {
			return fail(fmt.Sprintf("is above the declared maximum %d", *field.Max))
		}
	case KindString:
		text, ok := field.Default.(string)
		if !ok {
			return fail("is not a string")
		}
		if err := matchesPattern(field.Pattern, text); err != nil {
			return fail(err.Error())
		}
		if field.Pattern == "" && text == "" {
			return fail("is empty while the field declares minLength 1")
		}
		if field.MaxLength > 0 && len(text) > field.MaxLength {
			return fail(fmt.Sprintf("is longer than the declared maxLength %d", field.MaxLength))
		}
	case KindStringEnum:
		text, ok := field.Default.(string)
		if !ok {
			return fail("is not a string")
		}
		if !containsString(field.Enum, text) {
			return fail("is not a declared enum member")
		}
	case KindStringSet:
		items, ok := defaultSlice(field.Default)
		if !ok {
			return fail("is not an array")
		}
		seen := map[string]struct{}{}
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return fail("carries a non-string item")
			}
			if _, duplicate := seen[text]; duplicate {
				return fail("carries duplicate items while the field is a set")
			}
			seen[text] = struct{}{}
			if len(field.Enum) > 0 {
				if !containsString(field.Enum, text) {
					return fail("carries item " + text + " outside the declared enum")
				}
				continue
			}
			if err := matchesPattern(field.ItemPattern, text); err != nil {
				return fail("carries item " + text + " that " + err.Error())
			}
		}
		if err := checkItemCount(field, len(items)); err != nil {
			return fail(err.Error())
		}
	case KindJSONMap:
		object, ok := field.Default.(map[string]any)
		if !ok {
			return fail("is not a JSON object")
		}
		for key := range object {
			if err := matchesPattern(PortableMapKeyPattern, key); err != nil {
				return fail("carries key " + key + " that " + err.Error())
			}
		}
	case KindResourceRef:
		if err := validateResourceRefDefault(field.TargetKind, field.Default); err != nil {
			return fail(err.Error())
		}
	case KindResourceRefList:
		items, ok := defaultSlice(field.Default)
		if !ok {
			return fail("is not an array")
		}
		for _, item := range items {
			if err := validateResourceRefDefault(field.TargetKind, item); err != nil {
				return fail(err.Error())
			}
		}
		if err := checkItemCount(field, len(items)); err != nil {
			return fail(err.Error())
		}
	case KindBindingList:
		items, ok := defaultSlice(field.Default)
		if !ok {
			return fail("is not an array")
		}
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				return fail("carries a non-object binding entry")
			}
			name, _ := entry["name"].(string)
			if err := matchesPattern(PatternBindingName, name); err != nil {
				return fail("carries a binding name that " + err.Error())
			}
			if err := validateResourceRefDefault(field.TargetKind, entry["resource"]); err != nil {
				return fail(err.Error())
			}
		}
		if err := checkItemCount(field, len(items)); err != nil {
			return fail(err.Error())
		}
	case KindExternalServiceList:
		items, ok := defaultSlice(field.Default)
		if !ok {
			return fail("is not an array")
		}
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				return fail("carries a non-object slot entry")
			}
			name, _ := entry["name"].(string)
			if err := matchesPattern(PatternExternalServiceName, name); err != nil {
				return fail("carries a slot name that " + err.Error())
			}
			service, ok := entry["service"].(map[string]any)
			if !ok {
				return fail("carries a slot with no service declaration")
			}
			if service["apiVersion"] != StandardServiceAPIVersion {
				return fail("carries a slot whose service apiVersion is not " + StandardServiceAPIVersion)
			}
			protocol, _ := service["protocol"].(string)
			if !slices.Contains(ExternalServiceProtocols, protocol) {
				return fail("carries a slot naming protocol " + protocol + ", which is not in the closed vocabulary")
			}
		}
		if err := checkItemCount(field, len(items)); err != nil {
			return fail(err.Error())
		}
	case KindObject:
		if _, ok := field.Default.(map[string]any); !ok {
			return fail("is not a JSON object")
		}
	case KindObjectList:
		items, ok := defaultSlice(field.Default)
		if !ok {
			return fail("is not an array")
		}
		for _, item := range items {
			if _, ok := item.(map[string]any); !ok {
				return fail("carries a non-object entry")
			}
		}
		if err := checkItemCount(field, len(items)); err != nil {
			return fail(err.Error())
		}
	}
	return nil
}

func validateResourceRefDefault(targetKind string, value any) error {
	reference, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("is not a typed {kind, name} reference")
	}
	if kind, _ := reference["kind"].(string); kind != targetKind {
		return fmt.Errorf("does not name target kind %s", targetKind)
	}
	name, _ := reference["name"].(string)
	if err := matchesPattern(PatternResourceName, name); err != nil {
		return fmt.Errorf("names a resource that %s", err.Error())
	}
	return nil
}

func checkItemCount(field Field, count int) error {
	if field.MinItems > 0 && count < field.MinItems {
		return fmt.Errorf("declares fewer than the required %d items", field.MinItems)
	}
	if field.MaxItems > 0 && count > field.MaxItems {
		return fmt.Errorf("declares more than the permitted %d items", field.MaxItems)
	}
	return nil
}

func matchesPattern(pattern, value string) error {
	if pattern == "" {
		return nil
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("cannot be checked against the unparsable pattern %s", pattern)
	}
	if !expression.MatchString(value) {
		return fmt.Errorf("does not match %s", pattern)
	}
	return nil
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func defaultInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func defaultSlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out, true
	default:
		return nil, false
	}
}

// EmptyCollectionDefault reports whether a field's normative default is an
// empty collection. A client surface that drops empty collections would send a
// spec the host then materializes back to the same empty value — the two
// spellings would agree only by accident — so the emitters consult this
// instead of hardcoding which fields may be empty.
func EmptyCollectionDefault(field Field) bool {
	switch typed := field.Default.(type) {
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}
