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
	"strconv"
	"unicode/utf8"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
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
	if spec == nil {
		spec = map[string]any{}
	}
	materialized, ok := materializeSchemaValue(desiredSchema, spec).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return materialized
}

func materializeSchemaValue(schema map[string]any, value any) any {
	if discriminator, _ := schema[TaggedObjectDiscriminatorAnnotationKey].(string); discriminator != "" {
		object, ok := stringKeyedObject(value)
		if !ok {
			return cloneValue(value)
		}
		tag, _ := object[discriminator].(string)
		branches, _ := schema["oneOf"].([]any)
		for _, raw := range branches {
			branch, _ := raw.(map[string]any)
			properties, _ := branch["properties"].(map[string]any)
			declared, _ := constantString(properties[discriminator])
			if declared == tag {
				return materializeSchemaValue(branch, object)
			}
		}
		return cloneValue(value)
	}
	schemaType, _ := schema["type"].(string)
	switch schemaType {
	case "object":
		object, ok := stringKeyedObject(value)
		if !ok {
			return cloneValue(value)
		}
		properties, _ := schema["properties"].(map[string]any)
		additional, _ := schema["additionalProperties"].(map[string]any)
		out := make(map[string]any, len(object)+len(properties))
		for key, item := range object {
			child, _ := properties[key].(map[string]any)
			if child == nil {
				child = additional
			}
			if child == nil {
				out[key] = cloneValue(item)
				continue
			}
			out[key] = materializeSchemaValue(child, item)
		}
		for name, raw := range properties {
			if _, present := out[name]; present {
				continue
			}
			property, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			declared, hasDefault := property["default"]
			if !hasDefault {
				continue
			}
			out[name] = materializeSchemaValue(property, canonicalJSONValue(declared))
		}
		return out
	case "array":
		items, ok := defaultSlice(value)
		if !ok {
			return cloneValue(value)
		}
		itemSchema, _ := schema["items"].(map[string]any)
		out := make([]any, 0, len(items))
		for _, item := range items {
			if itemSchema == nil {
				out = append(out, cloneValue(item))
				continue
			}
			out = append(out, materializeSchemaValue(itemSchema, item))
		}
		return out
	default:
		return cloneValue(value)
	}
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
func validateFieldDefault(kind, sourceGroup string, field Field) error {
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
		if field.MaxLength > 0 && utf8.RuneCountInString(text) > field.MaxLength {
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
	case KindStringList:
		items, ok := defaultSlice(field.Default)
		if !ok {
			return fail("is not an array")
		}
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return fail("carries a non-string item")
			}
			if len(field.Enum) > 0 {
				if !containsString(field.Enum, text) {
					return fail("carries item " + text + " outside the declared enum")
				}
				continue
			}
			if err := matchesPattern(field.ItemPattern, text); err != nil {
				return fail("carries item " + text + " that " + err.Error())
			}
			if utf8.RuneCountInString(text) > field.MaxLength {
				return fail(fmt.Sprintf("carries item %s longer than the declared maxLength %d", text, field.MaxLength))
			}
		}
		if err := checkItemCount(field, len(items)); err != nil {
			return fail(err.Error())
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
	case KindStringMap:
		object, ok := stringKeyedObject(field.Default)
		if !ok {
			return fail("is not a string map")
		}
		if err := validateStringMapDefault(field, object, false); err != nil {
			return fail(err.Error())
		}
	case KindStringSetMap:
		object, ok := stringKeyedObject(field.Default)
		if !ok {
			return fail("is not a string-set map")
		}
		if err := validateStringMapDefault(field, object, true); err != nil {
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
		target, err := field.effectiveResourceTarget(sourceGroup)
		if err != nil {
			return fail(err.Error())
		}
		if err := validateResourceRefDefault(target.Group, target.Kind, field.Default); err != nil {
			return fail(err.Error())
		}
	case KindResourceRefList:
		target, err := field.effectiveResourceTarget(sourceGroup)
		if err != nil {
			return fail(err.Error())
		}
		items, ok := defaultSlice(field.Default)
		if !ok {
			return fail("is not an array")
		}
		for _, item := range items {
			if err := validateResourceRefDefault(target.Group, target.Kind, item); err != nil {
				return fail(err.Error())
			}
		}
		if err := checkItemCount(field, len(items)); err != nil {
			return fail(err.Error())
		}
	case KindBindingList:
		target, err := field.effectiveResourceTarget(sourceGroup)
		if err != nil {
			return fail(err.Error())
		}
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
			if err := validateResourceRefDefault(target.Group, target.Kind, entry["resource"]); err != nil {
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
			apiVersion, _ := service["apiVersion"].(string)
			protocol, _ := service["protocol"].(string)
			if err := formpackage.ValidateStandardServiceRef(formpackage.StandardServiceRef{
				APIVersion: apiVersion,
				Protocol:   protocol,
			}); err != nil {
				return fail("carries an invalid standard service: " + err.Error())
			}
		}
		if err := checkItemCount(field, len(items)); err != nil {
			return fail(err.Error())
		}
	case KindObject:
		object, ok := stringKeyedObject(field.Default)
		if !ok {
			return fail("is not a JSON object")
		}
		if err := validateObjectDefault(kind, sourceGroup, field, object); err != nil {
			return fail(err.Error())
		}
	case KindObjectList:
		items, ok := defaultSlice(field.Default)
		if !ok {
			return fail("is not an array")
		}
		for _, item := range items {
			object, ok := stringKeyedObject(item)
			if !ok {
				return fail("carries a non-object entry")
			}
			if err := validateObjectDefault(kind, sourceGroup, field, object); err != nil {
				return fail(err.Error())
			}
		}
		if err := checkItemCount(field, len(items)); err != nil {
			return fail(err.Error())
		}
	case KindTaggedObject:
		object, ok := stringKeyedObject(field.Default)
		if !ok {
			return fail("is not a tagged JSON object")
		}
		if err := validateTaggedObjectDefault(kind, sourceGroup, field, object); err != nil {
			return fail(err.Error())
		}
	}
	return nil
}

func validateTaggedObjectDefault(kind, sourceGroup string, field Field, object map[string]any) error {
	tag, ok := object[field.Discriminator].(string)
	if !ok || tag == "" {
		return fmt.Errorf("carries no string discriminator %s", field.Discriminator)
	}
	for _, variant := range field.Variants {
		if variant.Tag != tag {
			continue
		}
		members := make(map[string]any, len(object)-1)
		for name, value := range object {
			if name != field.Discriminator {
				members[name] = value
			}
		}
		variantField := field
		variantField.Fields = variant.Fields
		return validateObjectDefault(kind, sourceGroup, variantField, members)
	}
	return fmt.Errorf("carries unknown discriminator value %q", tag)
}

func validateObjectDefault(kind, sourceGroup string, field Field, object map[string]any) error {
	members := make(map[string]Field, len(field.Fields))
	for _, member := range field.Fields {
		members[member.Wire] = member
	}
	for name := range object {
		if _, known := members[name]; !known {
			return fmt.Errorf("carries unknown object member %s", name)
		}
	}
	for name, member := range members {
		value, present := object[name]
		if !present {
			if member.Required {
				return fmt.Errorf("omits required object member %s", name)
			}
			continue
		}
		candidate := member
		candidate.Default = value
		if err := validateFieldDefault(kind, sourceGroup, candidate); err != nil {
			return err
		}
	}
	return nil
}

func validateStringMapDefault(field Field, object map[string]any, setValues bool) error {
	if err := checkPropertyCount(field, len(object)); err != nil {
		return err
	}
	for key, raw := range object {
		if err := matchesPattern(PortableMapKeyPattern, key); err != nil {
			return fmt.Errorf("carries key %s that %s", key, err)
		}
		values := []any{raw}
		if setValues {
			items, ok := defaultSlice(raw)
			if !ok {
				return fmt.Errorf("carries key %s with a non-array set", key)
			}
			if err := checkItemCount(field, len(items)); err != nil {
				return fmt.Errorf("carries key %s whose set %s", key, err)
			}
			values = items
		}
		seen := map[string]struct{}{}
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return fmt.Errorf("carries key %s with a non-string value", key)
			}
			if setValues {
				if _, duplicate := seen[text]; duplicate {
					return fmt.Errorf("carries key %s with duplicate set value %q", key, text)
				}
				seen[text] = struct{}{}
			}
			if len(field.Enum) > 0 {
				if !containsString(field.Enum, text) {
					return fmt.Errorf("carries key %s with value %q outside the declared enum", key, text)
				}
				continue
			}
			if err := matchesPattern(field.ItemPattern, text); err != nil {
				return fmt.Errorf("carries key %s with value %q that %s", key, text, err)
			}
			if utf8.RuneCountInString(text) > field.MaxLength {
				return fmt.Errorf("carries key %s with value longer than the declared maxLength %d", key, field.MaxLength)
			}
		}
	}
	return nil
}

func validateResourceRefDefault(targetGroup, targetKind string, value any) error {
	reference, ok := stringKeyedObject(value)
	if !ok {
		return fmt.Errorf("is not a typed {apiVersion, kind, name} reference")
	}
	if len(reference) != 3 {
		return fmt.Errorf("is not a closed {apiVersion, kind, name} reference")
	}
	if group, _ := reference["apiVersion"].(string); group != targetGroup {
		return fmt.Errorf("does not name target group %s", targetGroup)
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

func checkPropertyCount(field Field, count int) error {
	if field.MinProperties > 0 && count < field.MinProperties {
		return fmt.Errorf("declares fewer than the required %d properties", field.MinProperties)
	}
	if field.MaxProperties > 0 && count > field.MaxProperties {
		return fmt.Errorf("declares more than the permitted %d properties", field.MaxProperties)
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

func stringKeyedObject(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out, true
	case map[string][]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
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
