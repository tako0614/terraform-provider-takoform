package jsonv2

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

type namedText string

type rawJSON struct{}

func (rawJSON) MarshalJSON() ([]byte, error) { return []byte(`{"x":1,"x":2}`), nil }

func TestValidateStrictJSONUnicodeAndDuplicateKeys(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "valid unicode and paired surrogate", data: []byte(`{"literal":"café","escaped":"\uD834\uDD1E"}`), want: true},
		{name: "duplicate key", data: []byte(`{"x":1,"x":2}`)},
		{name: "escaped duplicate key", data: []byte(`{"x":1,"\u0078":2}`)},
		{name: "nested duplicate key", data: []byte(`{"outer":{"x":1,"x":2}}`)},
		{name: "unpaired high surrogate", data: []byte(`{"x":"\uD800"}`)},
		{name: "unpaired low surrogate", data: []byte(`{"x":"\uDC00"}`)},
		{name: "mismatched surrogate pair", data: []byte(`{"x":"\uD800\u0041"}`)},
		{name: "invalid raw utf8", data: []byte{'"', 0xff, '"'}},
		{name: "trailing value", data: []byte(`{} []`)},
		{name: "malformed JSON", data: []byte(`{"x":}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.data)
			if (err == nil) != tt.want {
				t.Fatalf("Validate(%q) error = %v, want valid=%v", tt.data, err, tt.want)
			}
		})
	}
}

func TestDecodePreservesJSONNumberPrecision(t *testing.T) {
	value, err := Decode([]byte(`{"integer":9007199254740993,"decimal":1.234567890123456789}`))
	if err != nil {
		t.Fatal(err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("Decode() type = %T, want object", value)
	}
	if got := object["integer"]; got != json.Number("9007199254740993") {
		t.Fatalf("integer = %#v, want exact json.Number", got)
	}
	if got := object["decimal"]; got != json.Number("1.234567890123456789") {
		t.Fatalf("decimal = %#v, want exact json.Number", got)
	}
}

func TestMarshalRejectsInvalidOriginalUnicode(t *testing.T) {
	invalid := string([]byte{0xff})
	for name, value := range map[string]any{
		"nested string": map[string]any{"nested": []any{invalid}},
		"map key":       map[string]any{invalid: "value"},
		"named string":  map[string]any{"value": namedText(invalid)},
		"raw message":   json.RawMessage([]byte{'"', 0xff, '"'}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Marshal(value)
			if err == nil {
				t.Fatal("Marshal() accepted invalid original Unicode")
			}
			if strings.Contains(err.Error(), invalid) {
				t.Fatalf("error echoed input bytes: %v", err)
			}
		})
	}
}

func TestMarshalPreservesNumbersAndValidatesEmittedJSON(t *testing.T) {
	data, err := Marshal(map[string]any{"value": json.Number("9007199254740993")})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"value":9007199254740993}` {
		t.Fatalf("Marshal() = %s, number lost precision", data)
	}
	if _, err := Marshal(math.NaN()); err == nil {
		t.Fatal("Marshal() accepted NaN")
	}
	if _, err := Marshal(rawJSON{}); err == nil {
		t.Fatal("Marshal() accepted duplicate keys emitted by Marshaler")
	}
	if _, err := Marshal(json.RawMessage(`"\uD800"`)); err == nil {
		t.Fatal("Marshal() accepted an emitted unpaired surrogate")
	}
}
