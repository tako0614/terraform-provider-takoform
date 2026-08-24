package portableconformancev3

import (
	"encoding/json"
	"math/big"
	"strings"
)

// validateStructuralConstraints enforces the two cross-property stable-v1
// constraints whose operands are wholly inside one desired document. The
// install boundary has already proved their pointers and scalar domains; these
// checks still fail closed on a missing or malformed runtime operand so a
// damaged Definition or materializer cannot turn a declaration into a no-op.
func validateStructuralConstraints(form *InstalledForm, spec map[string]any) *hostError {
	for _, constraint := range form.Constraints {
		switch constraint.Kind {
		case "orderedPair":
			if len(constraint.References) != 2 {
				return stableError("invalid_argument", "the installed orderedPair declaration does not carry exactly two numeric pointers")
			}
			leftValue, leftPresent := desiredValueAtPointer(spec, constraint.References[0])
			rightValue, rightPresent := desiredValueAtPointer(spec, constraint.References[1])
			left, leftNumeric := exactJSONNumber(leftValue)
			right, rightNumeric := exactJSONNumber(rightValue)
			if !leftPresent || !rightPresent || !leftNumeric || !rightNumeric {
				return stableError(
					"invalid_argument",
					"orderedPair requires concrete numeric operands at "+constraint.References[0]+" and "+constraint.References[1],
				)
			}
			if left.Cmp(right) > 0 {
				return stableError(
					"invalid_argument",
					"orderedPair requires "+constraint.References[0]+" to be less than or equal to "+constraint.References[1],
				)
			}
		case "uniqueBy":
			value, present := desiredValueAtPointer(spec, constraint.List)
			elements, list := value.([]any)
			if !present || !list || constraint.Member == "" {
				return stableError(
					"invalid_argument",
					"uniqueBy requires an object list at "+constraint.List+" and one declared scalar member",
				)
			}
			seen := map[string]int{}
			for index, element := range elements {
				entry, object := element.(map[string]any)
				member, memberPresent := entry[constraint.Member]
				key, scalar := canonicalScalarKey(member)
				if !object || !memberPresent || !scalar {
					return stableError(
						"invalid_argument",
						"uniqueBy "+constraint.List+" item is missing required scalar member "+constraint.Member,
					)
				}
				if first, duplicate := seen[key]; duplicate {
					return stableError(
						"invalid_argument",
						"uniqueBy member "+constraint.Member+" repeats at "+constraint.List+
							" items "+jsonIndex(first)+" and "+jsonIndex(index),
					)
				}
				seen[key] = index
			}
		}
	}
	return nil
}

func desiredValueAtPointer(spec map[string]any, pointer string) (any, bool) {
	if pointer == "" || !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	var current any = spec
	for _, raw := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		name := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[name]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func exactJSONNumber(value any) (*big.Rat, bool) {
	var text string
	switch typed := value.(type) {
	case json.Number:
		text = typed.String()
	case int:
		text = jsonIndex(typed)
	case int64:
		text = new(big.Int).SetInt64(typed).String()
	case float64:
		text = json.Number(jsonNumberText(typed)).String()
	default:
		return nil, false
	}
	number, ok := new(big.Rat).SetString(text)
	return number, ok
}

func jsonNumberText(value float64) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func canonicalScalarKey(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return "string\x00" + typed, true
	case bool:
		if typed {
			return "boolean\x00true", true
		}
		return "boolean\x00false", true
	default:
		number, ok := exactJSONNumber(value)
		if !ok {
			return "", false
		}
		// RatString gives every JSON spelling of one numeric value one key,
		// while the type prefix keeps the string "1" distinct from number 1.
		return "number\x00" + number.RatString(), true
	}
}

func jsonIndex(value int) string {
	return new(big.Int).SetInt64(int64(value)).String()
}
