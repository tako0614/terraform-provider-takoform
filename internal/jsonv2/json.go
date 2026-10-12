// Package jsonv2 provides strict, lossless JSON boundaries shared by internal
// Provider v2 codecs. It contains no protocol or domain-specific behavior.
package jsonv2

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"unicode/utf8"
)

// Validate checks that data is exactly one strict JSON value, with valid UTF-8,
// paired Unicode surrogate escapes, and no duplicate object keys.
func Validate(data []byte) error {
	if !utf8.Valid(data) || !validSurrogateEscapes(data) {
		return errors.New("invalid JSON encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeValue(decoder); err != nil {
		return errors.New("invalid JSON value")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}

// Decode validates and decodes one JSON value while preserving numbers as
// json.Number rather than converting them to lossy float64 values.
func Decode(data []byte) (any, error) {
	if err := Validate(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("invalid JSON value")
	}
	return value, nil
}

// Marshal rejects invalid Unicode in original Go values, marshals once, and
// validates the emitted JSON before returning it.
func Marshal(value any) ([]byte, error) {
	if err := ValidateValue(value); err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("value is not valid JSON")
	}
	if err := Validate(data); err != nil {
		return nil, errors.New("value emitted invalid JSON")
	}
	return data, nil
}

// ValidateValue checks original Go string content recursively before encoding
// can replace invalid UTF-8 with U+FFFD. RawMessage is checked as raw bytes.
func ValidateValue(value any) error {
	if !validUTF8Value(reflect.ValueOf(value), 0) {
		return errors.New("value contains invalid Unicode or excessive nesting")
	}
	return nil
}

func validUTF8Value(value reflect.Value, depth int) bool {
	if depth > 1024 || !value.IsValid() {
		return depth <= 1024
	}
	if value.Type() == reflect.TypeOf(json.RawMessage(nil)) {
		if value.IsNil() {
			return true
		}
		return Validate(value.Bytes()) == nil
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return true
		}
		return validUTF8Value(value.Elem(), depth+1)
	case reflect.String:
		return utf8.ValidString(value.String())
	case reflect.Map:
		if value.IsNil() {
			return true
		}
		iterator := value.MapRange()
		for iterator.Next() {
			if !validUTF8Value(iterator.Key(), depth+1) || !validUTF8Value(iterator.Value(), depth+1) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if !validUTF8Value(value.Index(index), depth+1) {
				return false
			}
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			field := value.Type().Field(index)
			if field.PkgPath != "" || field.Tag.Get("json") == "-" {
				continue
			}
			if !validUTF8Value(value.Field(index), depth+1) {
				return false
			}
		}
	}
	return true
}

func consumeValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid object key")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("duplicate object key")
			}
			seen[key] = struct{}{}
			if err := consumeValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("unterminated object")
		}
		return nil
	case '[':
		for decoder.More() {
			if err := consumeValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("unterminated array")
		}
		return nil
	default:
		return errors.New("unexpected JSON delimiter")
	}
}

func validSurrogateEscapes(data []byte) bool {
	for index := 0; index < len(data); {
		if data[index] != '"' {
			index++
			continue
		}
		index++
		closed := false
		for index < len(data) {
			switch data[index] {
			case '"':
				index++
				closed = true
			case '\\':
				index++
				if index >= len(data) {
					return false
				}
				if data[index] == 'u' {
					unit, ok := hexCodeUnit(data, index+1)
					if !ok {
						index++
						continue
					}
					index += 5
					if unit >= 0xd800 && unit <= 0xdbff {
						if index+6 > len(data) || data[index] != '\\' || data[index+1] != 'u' {
							return false
						}
						low, valid := hexCodeUnit(data, index+2)
						if !valid || low < 0xdc00 || low > 0xdfff {
							return false
						}
						index += 6
					} else if unit >= 0xdc00 && unit <= 0xdfff {
						return false
					}
				} else {
					index++
				}
			default:
				index++
			}
			if closed {
				break
			}
		}
		if !closed {
			return false
		}
	}
	return true
}

func hexCodeUnit(data []byte, start int) (uint16, bool) {
	if start+4 > len(data) {
		return 0, false
	}
	var value uint16
	for _, digit := range data[start : start+4] {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value |= uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value |= uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value |= uint16(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}
