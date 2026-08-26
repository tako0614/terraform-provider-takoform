package portableconformancev3

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// genericMemoryResolvedTraversalLimit is owned by the independent memory
// subject. It repeats the protocol's fail-closed bound as data instead of
// importing the ReferenceHost evaluator's implementation constant.
const genericMemoryResolvedTraversalLimit = 256

// genericMemoryPointerValue evaluates an RFC 6901 pointer over ordinary JSON
// values. Unlike the Host helper, this implementation also traverses arrays;
// array tokens use canonical unsigned decimal indices so ambiguous spellings
// such as /items/01 fail closed.
func genericMemoryPointerValue(document map[string]any, pointer string) (any, bool) {
	if document == nil || pointer == "" || !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	var current any = document
	for _, rawToken := range strings.Split(pointer[1:], "/") {
		token, ok := genericMemoryDecodePointerToken(rawToken)
		if !ok {
			return nil, false
		}
		switch container := current.(type) {
		case map[string]any:
			current, ok = container[token]
			if !ok {
				return nil, false
			}
		case []any:
			index, ok := genericMemoryArrayIndex(token, len(container))
			if !ok {
				return nil, false
			}
			current = container[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func genericMemoryDecodePointerToken(raw string) (string, bool) {
	var decoded strings.Builder
	decoded.Grow(len(raw))
	for index := 0; index < len(raw); index++ {
		if raw[index] != '~' {
			decoded.WriteByte(raw[index])
			continue
		}
		if index+1 >= len(raw) {
			return "", false
		}
		index++
		switch raw[index] {
		case '0':
			decoded.WriteByte('~')
		case '1':
			decoded.WriteByte('/')
		default:
			return "", false
		}
	}
	return decoded.String(), true
}

func genericMemoryArrayIndex(token string, length int) (int, bool) {
	if token == "" || (len(token) > 1 && token[0] == '0') {
		return 0, false
	}
	for index := range token {
		if token[index] < '0' || token[index] > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseUint(token, 10, 63)
	if err != nil || parsed >= uint64(length) {
		return 0, false
	}
	return int(parsed), true
}

// genericMemoryCanonicalNumber parses only JSON-number grammar, then asks the
// standard exact rational type to retain its mathematical value. Validating
// the grammar here is important: big.Rat also accepts fractions, base prefixes
// and non-JSON exponent forms which a desired document may never carry.
func genericMemoryCanonicalNumber(value any) (*big.Rat, bool) {
	text, ok := genericMemoryNumberText(value)
	if !ok || !genericMemoryJSONNumberGrammar(text) {
		return nil, false
	}
	parsed, ok := new(big.Rat).SetString(text)
	return parsed, ok
}

func genericMemoryNumberText(value any) (string, bool) {
	switch typed := value.(type) {
	case json.Number:
		return typed.String(), true
	case int:
		return strconv.FormatInt(int64(typed), 10), true
	case int8:
		return strconv.FormatInt(int64(typed), 10), true
	case int16:
		return strconv.FormatInt(int64(typed), 10), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case uint:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint8:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint16:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	case float32:
		if math.IsNaN(float64(typed)) || math.IsInf(float64(typed), 0) {
			return "", false
		}
		return strconv.FormatFloat(float64(typed), 'g', -1, 32), true
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return "", false
		}
		return strconv.FormatFloat(typed, 'g', -1, 64), true
	default:
		return "", false
	}
}

func genericMemoryJSONNumberGrammar(text string) bool {
	if text == "" {
		return false
	}
	index := 0
	if text[index] == '-' {
		index++
		if index == len(text) {
			return false
		}
	}
	if text[index] == '0' {
		index++
		if index < len(text) && text[index] >= '0' && text[index] <= '9' {
			return false
		}
	} else {
		if text[index] < '1' || text[index] > '9' {
			return false
		}
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			index++
		}
	}
	if index < len(text) && text[index] == '.' {
		index++
		fractionStart := index
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			index++
		}
		if index == fractionStart {
			return false
		}
	}
	if index < len(text) && (text[index] == 'e' || text[index] == 'E') {
		index++
		if index < len(text) && (text[index] == '+' || text[index] == '-') {
			index++
		}
		exponentStart := index
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			index++
		}
		if index == exponentStart {
			return false
		}
	}
	return index == len(text)
}

func genericMemoryScalarKey(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return "text\x00" + typed, true
	case bool:
		if typed {
			return "truth\x001", true
		}
		return "truth\x000", true
	default:
		number, ok := genericMemoryCanonicalNumber(value)
		if !ok {
			return "", false
		}
		return "numeric\x00" + number.RatString(), true
	}
}
