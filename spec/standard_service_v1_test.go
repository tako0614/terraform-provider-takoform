package spec

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileSchemaSet(t *testing.T, paths []string, id string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		resourceID := document.(map[string]any)["$id"].(string)
		if err := compiler.AddResource(resourceID, document); err != nil {
			t.Fatalf("add %s: %v", path, err)
		}
	}
	compiled, err := compiler.Compile(id)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func TestStandardServiceRefV1IsOpenButNamespaced(t *testing.T) {
	t.Parallel()
	compiled := compileSchemaSet(
		t,
		[]string{"schemas/standard-service-ref-v1.schema.json"},
		"https://forms.takoform.com/schemas/standards/v1/standard-service-ref.schema.json",
	)
	unknown := map[string]any{
		"apiVersion": "standards.takoform.com/v1",
		"protocol":   "dev.example.quantum-cache",
	}
	if err := compiled.Validate(unknown); err != nil {
		t.Fatalf("unknown grammar-valid protocol was rejected: %v", err)
	}
	for _, invalid := range []string{"s3-compatible", "Com.Amazonaws.S3", "com..s3"} {
		document := map[string]any{"apiVersion": "standards.takoform.com/v1", "protocol": invalid}
		if err := compiled.Validate(document); err == nil {
			t.Errorf("invalid protocol %q was accepted", invalid)
		}
	}
}

func TestHostSupportProfileV1CarriesExactOpenServiceRef(t *testing.T) {
	t.Parallel()
	compiled := compileSchemaSet(t, []string{
		"schemas/form-ref-v1.schema.json",
		"schemas/interface-ref-v1alpha1.schema.json",
		"schemas/binding-ref-v1alpha2.schema.json",
		"schemas/standard-service-ref-v1.schema.json",
		"schemas/host-support-profile-v1.schema.json",
	}, "https://forms.takoform.com/schemas/support/v1/host-support-profile.schema.json")
	for _, satisfiable := range []bool{true, false} {
		profile := map[string]any{
			"apiVersion": "support.takoform.com/v1",
			"kind":       "StandardServiceSupport",
			"serviceRef": map[string]any{
				"apiVersion": "standards.takoform.com/v1",
				"protocol":   "dev.example.quantum-cache",
			},
			"satisfiable": satisfiable,
		}
		if err := compiled.Validate(profile); err != nil {
			t.Fatalf("satisfiable=%v profile was rejected: %v", satisfiable, err)
		}
	}
}
