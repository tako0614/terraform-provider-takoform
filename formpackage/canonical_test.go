package formpackage

import (
	"strings"
	"testing"
)

func TestCanonicalizeRFC8785OrderingAndNumbers(t *testing.T) {
	t.Parallel()
	input := []byte("{\"d\":0.000001,\"c\":1e-7,\"b\":1e30,\"a\":\"\\u20ac\"}")
	canonical, err := Canonicalize(input)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"a\":\"€\",\"b\":1e+30,\"c\":1e-7,\"d\":0.000001}"
	if string(canonical) != want {
		t.Fatalf("canonical = %s, want %s", canonical, want)
	}
}

func TestCanonicalizeRejectsNonIJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   []byte
		message string
	}{
		{name: "duplicate", input: []byte(`{"a":1,"a":2}`), message: "duplicate object name"},
		{name: "escaped duplicate", input: []byte(`{"a":1,"\u0061":2}`), message: "duplicate object name"},
		{name: "invalid utf8", input: []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}, message: "valid UTF-8"},
		{name: "high surrogate", input: []byte(`{"x":"\ud800"}`), message: "unpaired high surrogate"},
		{name: "low surrogate", input: []byte(`{"x":"\udc00"}`), message: "unpaired low surrogate"},
		{name: "negative zero integer", input: []byte(`{"x":-0}`), message: "negative zero"},
		{name: "negative zero decimal", input: []byte(`{"x":-0.000}`), message: "negative zero"},
		{name: "negative zero exponent", input: []byte(`{"x":-0e99}`), message: "negative zero"},
		{name: "infinity", input: []byte(`{"x":1e9999}`), message: "not finite"},
		{name: "nan", input: []byte(`{"x":NaN}`), message: "invalid number"},
		{name: "trailing", input: []byte(`{}[]`), message: "trailing"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Canonicalize(test.input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Canonicalize error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func TestDigestCanonicalJSONIgnoresInsignificantRepresentation(t *testing.T) {
	t.Parallel()
	left, err := DigestCanonicalJSON([]byte("{\n  \"b\": 1.0, \"a\": \"x\"\n}"))
	if err != nil {
		t.Fatal(err)
	}
	right, err := DigestCanonicalJSON([]byte(`{"a":"x","b":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if left != right || !ValidDigest(left) {
		t.Fatalf("canonical digests differ or are invalid: %q %q", left, right)
	}
}

func TestCanonicalizeRejectsExcessiveNesting(t *testing.T) {
	t.Parallel()
	input := []byte(strings.Repeat("[", maxJSONDepth+1) + "0" + strings.Repeat("]", maxJSONDepth+1))
	_, err := Canonicalize(input)
	if err == nil || !strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("Canonicalize error = %v, want nesting limit", err)
	}
}

func TestValidDigestRequiresLowercaseExactForm(t *testing.T) {
	t.Parallel()
	valid := "sha256:" + strings.Repeat("a", 64)
	if !ValidDigest(valid) {
		t.Fatalf("expected %q to be valid", valid)
	}
	for _, invalid := range []string{
		strings.Repeat("a", 64),
		"sha256:" + strings.Repeat("A", 64),
		"sha512:" + strings.Repeat("a", 64),
		"sha256:" + strings.Repeat("a", 63),
	} {
		if ValidDigest(invalid) {
			t.Fatalf("expected %q to be invalid", invalid)
		}
	}
}
