package formpackage

import (
	"strings"
	"testing"
)

func TestDecodeStrictIJSONValidatesRawDocumentBeforeTypedDecode(t *testing.T) {
	type metadata struct {
		Space string `json:"space"`
	}
	type document struct {
		Metadata metadata `json:"metadata"`
	}

	for _, test := range []struct {
		name    string
		raw     []byte
		wantErr string
	}{
		{
			name:    "duplicate nested member",
			raw:     []byte(`{"metadata":{"space":"one","space":"two"}}`),
			wantErr: "duplicate object name",
		},
		{
			name:    "invalid utf8",
			raw:     []byte{'{', '"', 'm', 'e', 't', 'a', 'd', 'a', 't', 'a', '"', ':', '"', 0xff, '"', '}'},
			wantErr: "valid UTF-8",
		},
		{
			name:    "unknown typed field",
			raw:     []byte(`{"metadata":{"space":"one","authority":"host"}}`),
			wantErr: `unknown field "authority"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var decoded document
			err := DecodeStrictIJSON(test.raw, &decoded)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("DecodeStrictIJSON error = %v, want %q", err, test.wantErr)
			}
		})
	}

	var decoded document
	if err := DecodeStrictIJSON(
		[]byte(`{"metadata":{"space":"exact"}}`),
		&decoded,
	); err != nil {
		t.Fatal(err)
	}
	if decoded.Metadata.Space != "exact" {
		t.Fatalf("decoded Space = %q", decoded.Metadata.Space)
	}
}
