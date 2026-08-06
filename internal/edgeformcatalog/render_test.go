package edgeformcatalog

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// TestRenderedFormsVerifyAsV1Alpha4Packages proves the complete authoring
// pipeline end to end: every catalog Form renders to a definition whose
// staged package — definition, canonical fixture, and negative fixtures —
// passes formpackage.VerifyDirectory under the v1alpha4 index profile.
func TestRenderedFormsVerifyAsV1Alpha4Packages(t *testing.T) {
	t.Parallel()
	forms, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	if len(forms) != len(Forms) {
		t.Fatalf("rendered %d forms, want %d", len(forms), len(Forms))
	}
	for _, form := range forms {
		form := form
		t.Run(form.Slug, func(t *testing.T) {
			t.Parallel()
			if len(form.Definition.NegativeFixtures) == 0 {
				t.Fatal("every Form must carry at least one negative fixture")
			}
			root := t.TempDir()
			definitionRaw := []byte(form.DefinitionJSON)
			writeFile(t, filepath.Join(root, "definition.json"), definitionRaw)
			payloadPaths := []string{"definition.json"}
			payloads := map[string][]byte{"definition.json": definitionRaw}
			for name, document := range form.Fixtures {
				raw, err := marshalIndented(document)
				if err != nil {
					t.Fatal(err)
				}
				relative := "fixtures/" + name
				writeFile(t, filepath.Join(root, "fixtures", name), []byte(raw))
				payloadPaths = append(payloadPaths, relative)
				payloads[relative] = []byte(raw)
			}
			slices.Sort(payloadPaths)
			files := make([]any, 0, len(payloadPaths))
			for _, relative := range payloadPaths {
				mediaType := "application/json"
				if relative == "definition.json" {
					mediaType = formpackage.DefinitionMediaType
				}
				files = append(files, map[string]any{
					"path":      relative,
					"mediaType": mediaType,
					"size":      len(payloads[relative]),
					"digest":    formpackage.DigestBytes(payloads[relative]),
				})
			}
			schemaDigest, err := formpackage.DigestCanonicalJSON(definitionRaw)
			if err != nil {
				t.Fatal(err)
			}
			index := map[string]any{
				"apiVersion": formpackage.FamilyPackageAPIVersion,
				"kind":       formpackage.PackageKind,
				"formRef": map[string]any{
					"apiVersion":        Family.APIVersion(),
					"kind":              form.Kind,
					"definitionVersion": form.Definition.DefinitionVersion,
					"schemaDigest":      schemaDigest,
				},
				"definitionPath": "definition.json",
				"files":          files,
			}
			indexRaw, err := marshalIndented(index)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(root, formpackage.PackageIndexFilename), []byte(indexRaw))
			report, err := formpackage.VerifyDirectory(root)
			if err != nil {
				t.Fatal(err)
			}
			if report.FormRef.Kind != form.Kind || report.FormRef.APIVersion != Family.APIVersion() {
				t.Fatalf("verified FormRef = %+v", report.FormRef)
			}
		})
	}
}

func TestRenderedDefinitionsOmitNameAndEnvelopeFields(t *testing.T) {
	t.Parallel()
	forms, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	for _, form := range forms {
		properties, ok := form.Definition.DesiredSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s desired schema has no properties object", form.Kind)
		}
		for _, forbidden := range []string{"name", "id", "generation", "ready", "portability"} {
			if _, present := properties[forbidden]; present {
				t.Errorf("%s desired schema declares envelope field %q", form.Kind, forbidden)
			}
		}
		if form.Definition.ObservedSchema != nil || form.Definition.OutputSchema != nil {
			t.Errorf("%s declares observed/output schemas; MVP forms declare none", form.Kind)
		}
	}
}

// TestContractDefinitionsSatisfyNormativeSchemas validates every rendered
// Interface and Binding Definition against the normative spec schemas.
func TestContractDefinitionsSatisfyNormativeSchemas(t *testing.T) {
	t.Parallel()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	ids := map[string]string{
		"https://forms.takoform.com/schemas/interfaces/v1alpha1/interface-ref.schema.json":        "interface-ref-v1alpha1.schema.json",
		"https://forms.takoform.com/schemas/interfaces/v1alpha1/interface-definition.schema.json": "interface-definition-v1alpha1.schema.json",
		"https://forms.takoform.com/schemas/bindings/v1alpha1/binding-definition.schema.json":     "binding-definition-v1alpha1.schema.json",
	}
	for id, file := range ids {
		raw, err := os.ReadFile(filepath.Join("..", "..", "spec", "schemas", file))
		if err != nil {
			t.Fatal(err)
		}
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(id, value); err != nil {
			t.Fatal(err)
		}
	}
	interfaceSchema, err := compiler.Compile("https://forms.takoform.com/schemas/interfaces/v1alpha1/interface-definition.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	bindingSchema, err := compiler.Compile("https://forms.takoform.com/schemas/bindings/v1alpha1/binding-definition.schema.json")
	if err != nil {
		t.Fatal(err)
	}

	interfaces, err := RenderInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(interfaces) != 5 {
		t.Fatalf("interface catalog has %d entries, want 5", len(interfaces))
	}
	for _, contract := range interfaces {
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(contract.DefinitionJSON)))
		if err != nil {
			t.Fatalf("%s: %v", contract.Name, err)
		}
		if err := interfaceSchema.Validate(value); err != nil {
			t.Errorf("interface %s does not satisfy the normative schema: %v", contract.Name, err)
		}
		digest, err := formpackage.DigestCanonicalJSON([]byte(contract.DefinitionJSON))
		if err != nil {
			t.Fatal(err)
		}
		if digest != contract.SchemaDigest {
			t.Errorf("interface %s digest drifted", contract.Name)
		}
	}

	bindings, err := RenderBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 5 {
		t.Fatalf("binding catalog has %d entries, want 5", len(bindings))
	}
	interfaceDigests := map[string]string{}
	for _, contract := range interfaces {
		interfaceDigests[contract.Name] = contract.SchemaDigest
	}
	definitions, err := BindingDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for index, contract := range bindings {
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(contract.DefinitionJSON)))
		if err != nil {
			t.Fatalf("%s: %v", contract.Name, err)
		}
		if err := bindingSchema.Validate(value); err != nil {
			t.Errorf("binding %s does not satisfy the normative schema: %v", contract.Name, err)
		}
		target := definitions[index].TargetInterface
		if interfaceDigests[target.Name] != target.SchemaDigest {
			t.Errorf("binding %s embeds a stale digest for interface %s", contract.Name, target.Name)
		}
	}
}

func TestQueueProducerProjectsOnlySubmission(t *testing.T) {
	t.Parallel()
	definitions, err := BindingDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		if definition.Name != "module-worker.queue-producer" {
			continue
		}
		operations := definition.RuntimeProjection.Operations
		if len(operations) != 2 || operations[0] != "send" || operations[1] != "sendBatch" {
			t.Fatalf("queue-producer projects %v", operations)
		}
		return
	}
	t.Fatal("module-worker.queue-producer is not in the catalog")
}

func writeFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
