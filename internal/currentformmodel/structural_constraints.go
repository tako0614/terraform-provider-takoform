package currentformmodel

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
)

// validateStructuralConstraints proves the authoring declarations behind the
// two stable-v1 cross-property structural rules. These checks establish that the
// published pointers name values in the promised domain; enforcement against
// each desired resource remains a Host responsibility.
func validateStructuralConstraints(form Form) error {
	for index, constraint := range form.StructuralConstraints {
		if err := validateStructuralConstraintShape(constraint); err != nil {
			return fmt.Errorf("form %s structural constraint %d: %w", form.Kind, index, err)
		}
		switch constraint.Kind {
		case ConstraintOrderedPair:
			for _, pointer := range constraint.References {
				field, pathRequired, err := fieldAtConstraintPointer(form.Fields, pointer)
				if err != nil {
					return fmt.Errorf("form %s orderedPair pointer %s: %w", form.Kind, pointer, err)
				}
				if field.Kind != KindInteger || !pathRequired {
					return fmt.Errorf(
						"form %s orderedPair pointer %s must name a required integer field",
						form.Kind, pointer,
					)
				}
			}
		case ConstraintUniqueBy:
			list, _, err := fieldAtConstraintPointer(form.Fields, constraint.List)
			if err != nil {
				return fmt.Errorf("form %s uniqueBy list %s: %w", form.Kind, constraint.List, err)
			}
			if list.Kind != KindObjectList {
				return fmt.Errorf("form %s uniqueBy list %s is %s, want object-list", form.Kind, constraint.List, list.Kind)
			}
			member, found := directField(list.Fields, constraint.Member)
			if !found || !member.Required || !scalarConstraintFieldKind(member.Kind) {
				return fmt.Errorf(
					"form %s uniqueBy member %s on %s must be one required scalar field",
					form.Kind, constraint.Member, constraint.List,
				)
			}
		}
	}
	return nil
}

func validateStructuralConstraintShape(constraint Constraint) error {
	foreign := func(allowReferences, allowListMember bool) error {
		if constraint.Reference != "" || constraint.KeyedBy != "" || constraint.Total != 0 ||
			constraint.Property != "" || constraint.Output != "" || constraint.Anchor != "" ||
			constraint.Members != "" || constraint.Through != "" {
			return fmt.Errorf("constraint kind %s carries members from another constraint grammar", constraint.Kind)
		}
		if !allowReferences && len(constraint.References) != 0 {
			return fmt.Errorf("constraint kind %s does not define references", constraint.Kind)
		}
		if !allowListMember && (constraint.List != "" || constraint.Member != "") {
			return fmt.Errorf("constraint kind %s does not define list or member", constraint.Kind)
		}
		return nil
	}
	switch constraint.Kind {
	case ConstraintOrderedPair:
		if err := foreign(true, false); err != nil {
			return err
		}
		if len(constraint.References) != 2 || constraint.References[0] == constraint.References[1] {
			return fmt.Errorf("orderedPair requires exactly two distinct numeric pointers")
		}
		for _, pointer := range constraint.References {
			if err := validateConstraintPointer(pointer, 0, 0); err != nil {
				return fmt.Errorf("orderedPair reference: %w", err)
			}
		}
	case ConstraintUniqueBy:
		if err := foreign(false, true); err != nil {
			return err
		}
		if err := validateConstraintPointer(constraint.List, 0, 0); err != nil {
			return fmt.Errorf("uniqueBy list: %w", err)
		}
		if constraint.Member == "" || strings.ContainsAny(constraint.Member, "~/") || len(constraint.Member) > 64 {
			return fmt.Errorf("uniqueBy member %q is not one direct scalar member name", constraint.Member)
		}
	default:
		return fmt.Errorf("constraint kind %q is not a structural constraint", constraint.Kind)
	}
	return nil
}

func fieldAtConstraintPointer(fields []Field, pointer string) (Field, bool, error) {
	if err := validateConstraintPointer(pointer, 0, 0); err != nil {
		return Field{}, false, err
	}
	required := true
	current := fields
	tokens := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for index, raw := range tokens {
		field, found := directField(current, unescapeJSONPointerToken(raw))
		if !found {
			return Field{}, false, fmt.Errorf("does not name a declared field")
		}
		required = required && field.Required
		if index == len(tokens)-1 {
			return field, required, nil
		}
		if field.Kind != KindObject {
			return Field{}, false, fmt.Errorf("traverses %s before the final token", field.Kind)
		}
		current = field.Fields
	}
	return Field{}, false, fmt.Errorf("does not name a declared field")
}

func directField(fields []Field, wire string) (Field, bool) {
	for _, field := range fields {
		if field.Wire == wire {
			return field, true
		}
	}
	return Field{}, false
}

func scalarConstraintFieldKind(kind FieldKind) bool {
	switch kind {
	case KindString, KindStringEnum, KindInteger, KindBoolean:
		return true
	default:
		return false
	}
}

// validateStructuralConstraintAgainstSchema repeats the same proof at the
// rendered-schema seam. DeriveRelationsWithConstraints is also used by hosts
// installing arbitrary Definitions, so recognizing a kind without proving its
// pointers would turn a closed vocabulary into an unenforced promise.
func validateStructuralConstraintAgainstSchema(schema map[string]any, constraint Constraint) error {
	if err := validateStructuralConstraintShape(constraint); err != nil {
		return err
	}
	switch constraint.Kind {
	case ConstraintOrderedPair:
		for _, pointer := range constraint.References {
			node, required, err := schemaNodeAtConstraintPointer(schema, pointer)
			if err != nil {
				return fmt.Errorf("orderedPair pointer %s: %w", pointer, err)
			}
			if !required || (node["type"] != "integer" && node["type"] != "number") {
				return fmt.Errorf("orderedPair pointer %s must name a required numeric schema", pointer)
			}
		}
	case ConstraintUniqueBy:
		list, _, err := schemaNodeAtConstraintPointer(schema, constraint.List)
		if err != nil {
			return fmt.Errorf("uniqueBy list %s: %w", constraint.List, err)
		}
		if list["type"] != "array" {
			return fmt.Errorf("uniqueBy list %s must name an array schema", constraint.List)
		}
		items, ok := list["items"].(map[string]any)
		if !ok || items["type"] != "object" {
			return fmt.Errorf("uniqueBy list %s must contain object schemas", constraint.List)
		}
		properties, ok := items["properties"].(map[string]any)
		if !ok || !containsString(anyStrings(items["required"]), constraint.Member) {
			return fmt.Errorf("uniqueBy member %s must be required on %s items", constraint.Member, constraint.List)
		}
		member, ok := properties[constraint.Member].(map[string]any)
		if !ok || !scalarSchemaType(member["type"]) {
			return fmt.Errorf("uniqueBy member %s on %s must be scalar", constraint.Member, constraint.List)
		}
	}
	return nil
}

func schemaNodeAtConstraintPointer(root map[string]any, pointer string) (map[string]any, bool, error) {
	if err := validateConstraintPointer(pointer, 0, 0); err != nil {
		return nil, false, err
	}
	current := root
	required := true
	for _, raw := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		name := unescapeJSONPointerToken(raw)
		properties, ok := current["properties"].(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("traverses a schema with no properties")
		}
		next, ok := properties[name].(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("does not name a declared property")
		}
		required = required && containsString(anyStrings(current["required"]), name)
		current = next
	}
	return current, required, nil
}

func scalarSchemaType(value any) bool {
	typeName, ok := value.(string)
	if !ok {
		return false
	}
	switch typeName {
	case "string", "integer", "number", "boolean":
		return true
	default:
		return false
	}
}

// ValidateStructuralConstraintValues applies the two provider-neutral
// structural rules to one desired value that has already satisfied the Form's
// desired JSON Schema. It performs no coercion: orderedPair accepts JSON
// numbers only, and uniqueBy compares scalar values within their JSON type
// (all mathematically equal JSON-number spellings share one key).
//
// Hosts are responsible for calling this atomically in their own admission
// path. Keeping the mechanism here lets provider and Host implementations
// consume one rule instead of re-defining Form semantics independently.
func ValidateStructuralConstraintValues(constraints []Constraint, desired map[string]any) error {
	for index, constraint := range constraints {
		switch constraint.Kind {
		case ConstraintOrderedPair:
			if err := validateStructuralConstraintShape(constraint); err != nil {
				return fmt.Errorf("structural constraint %d: %w", index, err)
			}
			leftValue, leftOK := valueAtJSONPointer(desired, constraint.References[0])
			rightValue, rightOK := valueAtJSONPointer(desired, constraint.References[1])
			left, leftNumber := exactJSONNumber(leftValue)
			right, rightNumber := exactJSONNumber(rightValue)
			if !leftOK || !rightOK || !leftNumber || !rightNumber {
				return fmt.Errorf("orderedPair requires present numeric values at %s and %s", constraint.References[0], constraint.References[1])
			}
			if left.Cmp(right) > 0 {
				return fmt.Errorf("orderedPair requires %s <= %s", constraint.References[0], constraint.References[1])
			}
		case ConstraintUniqueBy:
			if err := validateStructuralConstraintShape(constraint); err != nil {
				return fmt.Errorf("structural constraint %d: %w", index, err)
			}
			value, present := valueAtJSONPointer(desired, constraint.List)
			if !present {
				// An optional list is permitted when its desired schema assigns
				// omission a portable meaning; no present elements means there is
				// nothing whose member can collide.
				continue
			}
			items, ok := value.([]any)
			if !ok {
				return fmt.Errorf("uniqueBy list %s is not an array", constraint.List)
			}
			seen := make(map[string]int, len(items))
			for itemIndex, raw := range items {
				item, ok := raw.(map[string]any)
				if !ok {
					return fmt.Errorf("uniqueBy list %s item %d is not an object", constraint.List, itemIndex)
				}
				member, present := item[constraint.Member]
				key, scalar := structuralScalarKey(member)
				if !present || !scalar {
					return fmt.Errorf("uniqueBy list %s item %d has no scalar member %s", constraint.List, itemIndex, constraint.Member)
				}
				if prior, duplicate := seen[key]; duplicate {
					return fmt.Errorf(
						"uniqueBy list %s items %d and %d repeat member %s",
						constraint.List, prior, itemIndex, constraint.Member,
					)
				}
				seen[key] = itemIndex
			}
		}
	}
	return nil
}

func structuralScalarKey(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return "string:" + typed, true
	case bool:
		return "boolean:" + strconv.FormatBool(typed), true
	default:
		number, ok := exactJSONNumber(value)
		if !ok {
			return "", false
		}
		return "number:" + number.RatString(), true
	}
}

func exactJSONNumber(value any) (*big.Rat, bool) {
	var text string
	switch typed := value.(type) {
	case json.Number:
		text = typed.String()
	default:
		reflected := reflect.ValueOf(value)
		if !reflected.IsValid() {
			return nil, false
		}
		switch reflected.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			text = strconv.FormatInt(reflected.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			text = strconv.FormatUint(reflected.Uint(), 10)
		case reflect.Float32, reflect.Float64:
			value := reflected.Float()
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, false
			}
			text = strconv.FormatFloat(value, 'g', -1, reflected.Type().Bits())
		default:
			return nil, false
		}
	}
	number, ok := new(big.Rat).SetString(text)
	return number, ok
}
