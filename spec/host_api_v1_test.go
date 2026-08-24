package spec

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestStableFormRefIsExactAndVersionless(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("schemas/form-ref-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	id := "https://forms.takoform.com/schemas/v1/form-ref.schema.json"
	if err := compiler.AddResource(id, document); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(id)
	if err != nil {
		t.Fatal(err)
	}
	exact := map[string]any{
		"apiVersion": "queue.forms.takoform.com", "kind": "PullQueue",
		"definitionVersion": "0.1.0",
		"schemaDigest":      "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	if err := compiled.Validate(exact); err != nil {
		t.Fatalf("versionless exact FormRef rejected: %v", err)
	}
	for name, invalid := range map[string]map[string]any{
		"versioned family": func() map[string]any {
			copy := cloneJSONMap(t, exact)
			copy["apiVersion"] = "queue.forms.takoform.com/v1"
			return copy
		}(),
		"missing digest": func() map[string]any {
			copy := cloneJSONMap(t, exact)
			delete(copy, "schemaDigest")
			return copy
		}(),
		"latest selector": func() map[string]any {
			copy := cloneJSONMap(t, exact)
			copy["definitionVersion"] = "latest"
			return copy
		}(),
	} {
		if err := compiled.Validate(invalid); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestStableHostDocumentsUseOnlyStableIdentities(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"host-api/v1.md",
		"host-api/operations-v1.json",
		"schemas/form-ref-v1.schema.json",
		"schemas/host-api-wire-v1.schema.json",
		"schemas/host-discovery-v1.schema.json",
		"schemas/operation-v1.schema.json",
		"schemas/host-support-profile-v1.schema.json",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range [][]byte{
			[]byte("ObjectBucket"), []byte("edge.objects"), []byte("module-worker.object-bucket"),
		} {
			if bytes.Contains(raw, forbidden) {
				t.Errorf("%s contains withdrawn identity %q", path, forbidden)
			}
		}
	}

	var operations struct {
		Format   string `json:"format"`
		APIGroup string `json:"apiGroup"`
	}
	raw, err := os.ReadFile("host-api/operations-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &operations); err != nil {
		t.Fatal(err)
	}
	if operations.Format != "takoform.host-api@v1" || operations.APIGroup != "forms.takoform.com/v1" {
		t.Fatalf("stable operation table identity = %#v", operations)
	}
	if bytes.Contains(raw, []byte("{formVersion}")) || bytes.Contains(raw, []byte(`"name": "version"`)) {
		t.Fatal("stable operation table retained a family-version path or query selector")
	}
	for _, withdrawnRef := range [][]byte{
		[]byte("schemas/operations/v1alpha2/operation.schema.json"),
		[]byte("schemas/support/v1alpha2/host-support-profile.schema.json"),
	} {
		if bytes.Contains(raw, withdrawnRef) {
			t.Fatalf("stable operation table retained pre-stable schema ref %q", withdrawnRef)
		}
	}
}

func TestStableWireUsesStableSchemaRefsAndVersionlessResourceGroups(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("schemas/host-api-wire-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, withdrawn := range [][]byte{
		[]byte("forms.takoform.com/v1alpha2"),
		[]byte("schemas/operations/v1alpha2/operation.schema.json"),
		[]byte("schemas/support/v1alpha2/host-support-profile.schema.json"),
	} {
		if bytes.Contains(raw, withdrawn) {
			t.Fatalf("stable wire retained pre-stable identity %q", withdrawn)
		}
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	definitions := wire["$defs"].(map[string]any)
	resourceCore := definitions["resourceCore"].(map[string]any)
	properties := resourceCore["properties"].(map[string]any)
	apiVersion := properties["apiVersion"].(map[string]any)
	pattern, _ := apiVersion["pattern"].(string)
	if pattern == "" {
		t.Fatal("stable resource apiVersion has no versionless family grammar")
	}
	compiler := jsonschema.NewCompiler()
	const resourceID = "https://forms.takoform.com/schemas/v1/test-resource-api-version.schema.json"
	if err := compiler.AddResource(resourceID, map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     resourceID,
		"type":    "string",
		"pattern": pattern,
	}); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(resourceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := compiled.Validate("queue.forms.takoform.com"); err != nil {
		t.Fatalf("versionless resource group rejected: %v", err)
	}
	if err := compiled.Validate("queue.forms.takoform.com/v1"); err == nil {
		t.Fatal("stable resource apiVersion accepted a versioned family group")
	}
}

func cloneJSONMap(t *testing.T, input map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	return output
}
