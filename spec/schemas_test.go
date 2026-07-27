// Package spec holds the drift gate between the normative schemas and the
// copies the implementation embeds.
package spec

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestNormativeSchemasMatchTheImplementation proves the Go verifier embeds
// exactly the schemas this specification publishes.
//
// The specification is the source: if these ever disagree, an implementation
// would be enforcing a contract nobody can read.
func TestNormativeSchemasMatchTheImplementation(t *testing.T) {
	implementations := map[string]string{
		"form-ref.schema.json":                           filepath.Join("..", "formpackage", "schemas", "form-ref.schema.json"),
		"form-definition.schema.json":                    filepath.Join("..", "formpackage", "schemas", "form-definition.schema.json"),
		"package-index.schema.json":                      filepath.Join("..", "formpackage", "schemas", "package-index.schema.json"),
		"form-package-revocation.schema.json":            filepath.Join("..", "formpackage", "schemas", "form-package-revocation.schema.json"),
		"form-package-revocation-checkpoint.schema.json": filepath.Join("..", "formpackage", "schemas", "form-package-revocation-checkpoint.schema.json"),
		"host-discovery.schema.json":                     filepath.Join("..", "schemas", "host-discovery.schema.json"),
	}
	entries, err := os.ReadDir("schemas")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		seen++
		implementation, ok := implementations[entry.Name()]
		if !ok {
			t.Fatalf("normative schema %s has no implementation counterpart", entry.Name())
		}
		want, err := os.ReadFile(filepath.Join("schemas", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(implementation)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s drifted from the normative schema", entry.Name())
		}
	}
	if seen != len(implementations) {
		t.Fatalf("normative schema set has %d files, want %d", seen, len(implementations))
	}
}
