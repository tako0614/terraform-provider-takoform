// Package spec holds the compile and drift gates for normative schemas.
package spec

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestNormativeSchemasMatchTheImplementation proves every schema that has a
// runtime copy is byte-identical to its normative source. A normative-only
// protocol schema is instead consumed by conformance tooling and is compiled
// in host_api_wire_test.go.
//
// The specification is the source: if these ever disagree, an implementation
// would be enforcing a contract nobody can read.
func TestNormativeSchemasMatchTheImplementation(t *testing.T) {
	implementations := map[string]string{
		"form-ref-v1beta1.schema.json":                   filepath.Join("..", "formpackage", "schemas", "form-ref-v1beta1.schema.json"),
		"form-definition-v1beta1.schema.json":            filepath.Join("..", "formpackage", "schemas", "form-definition-v1beta1.schema.json"),
		"form-definition-v1beta2.schema.json":            filepath.Join("..", "formpackage", "schemas", "form-definition-v1beta2.schema.json"),
		"package-index-v1alpha4.schema.json":             filepath.Join("..", "formpackage", "schemas", "package-index-v1alpha4.schema.json"),
		"interface-ref-v1alpha1.schema.json":             filepath.Join("..", "formpackage", "schemas", "interface-ref-v1alpha1.schema.json"),
		"binding-ref-v1alpha1.schema.json":               filepath.Join("..", "formpackage", "schemas", "binding-ref-v1alpha1.schema.json"),
		"binding-ref-v1alpha2.schema.json":               filepath.Join("..", "formpackage", "schemas", "binding-ref-v1alpha2.schema.json"),
		"form-ref-v1beta2.schema.json":                   filepath.Join("..", "formpackage", "schemas", "form-ref-v1beta2.schema.json"),
		"package-index-v1alpha5.schema.json":             filepath.Join("..", "formpackage", "schemas", "package-index-v1alpha5.schema.json"),
		"form-package-revocation.schema.json":            filepath.Join("..", "formpackage", "schemas", "form-package-revocation.schema.json"),
		"form-package-revocation-checkpoint.schema.json": filepath.Join("..", "formpackage", "schemas", "form-package-revocation-checkpoint.schema.json"),
	}
	normativeOnly := map[string]bool{
		"host-api-wire-v1beta1.schema.json":         true,
		"host-discovery-v1beta1.schema.json":        true,
		"interface-definition-v1alpha1.schema.json": true,
		"binding-definition-v1alpha1.schema.json":   true,
		"binding-definition-v1alpha2.schema.json":   true,
		"artifact-manifest-v1alpha1.schema.json":    true,
		"operation-v1alpha1.schema.json":            true,
		"host-support-profile-v1alpha1.schema.json": true,
		"host-support-profile-v1alpha2.schema.json": true,
		"host-api-wire-v1beta4.schema.json":         true,
		"host-discovery-v1beta4.schema.json":        true,
		"operation-v1alpha2.schema.json":            true,
		"standard-service-ref-v1alpha1.schema.json": true,
	}
	entries, err := os.ReadDir("schemas")
	if err != nil {
		t.Fatal(err)
	}
	seenImplementations := 0
	seenNormativeOnly := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		implementation, ok := implementations[entry.Name()]
		if !ok {
			if normativeOnly[entry.Name()] {
				seenNormativeOnly++
				continue
			}
			t.Fatalf("normative schema %s is neither copied by an implementation nor declared normative-only", entry.Name())
		}
		seenImplementations++
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
	if seenImplementations != len(implementations) || seenNormativeOnly != len(normativeOnly) {
		t.Fatalf(
			"normative schema set has %d implementation copies and %d normative-only schemas, want %d and %d",
			seenImplementations,
			seenNormativeOnly,
			len(implementations),
			len(normativeOnly),
		)
	}
}
