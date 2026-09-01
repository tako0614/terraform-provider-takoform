package standardforms

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentFamilyInventoryIsProviderNeutralAndComplete(t *testing.T) {
	t.Parallel()

	wantGroups := []string{"edge.forms.takoform.com"}
	wantCounts := []int{17}
	families := currentFamilies()
	if len(families) != len(wantGroups) {
		t.Fatalf("family count = %d, want %d", len(families), len(wantGroups))
	}
	total := 0
	for index, family := range families {
		if family.Group != wantGroups[index] {
			t.Fatalf("family[%d] = %q, want %q", index, family.Group, wantGroups[index])
		}
		if len(family.Forms) != wantCounts[index] {
			t.Fatalf("%s Form count = %d, want %d", family.Group, len(family.Forms), wantCounts[index])
		}
		total += len(family.Forms)
	}
	if total != 17 {
		t.Fatalf("current Form count = %d, want 17", total)
	}

	seenTypes := map[string]string{}
	for _, family := range families {
		for _, form := range family.Forms {
			resourceType, err := providerReferenceTerraformType(form)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(resourceType, "takoform_") {
				t.Fatalf("provider reference type for %s/%s = %q", family.Group, form.Kind, resourceType)
			}
			if previous, duplicate := seenTypes[resourceType]; duplicate {
				t.Fatalf("provider reference type %q belongs to both %s and %s/%s", resourceType, previous, family.Group, form.Kind)
			}
			seenTypes[resourceType] = family.Group + "/" + form.Kind
		}
	}
	if len(seenTypes) != 17 {
		t.Fatalf("provider reference type count = %d, want 17", len(seenTypes))
	}
	if _, ok := seenTypes["takoform_edge_object_bucket"]; !ok {
		t.Fatal("provider reference projection omits the publisher's Edge ObjectBucket Form")
	}
}

func TestPublishedSurfaceInventoryCoversEveryCurrentProviderMapping(t *testing.T) {
	t.Parallel()

	surfaces := renderPublishedSurfaces()
	if len(surfaces) != 35 {
		t.Fatalf("published surface count = %d, want 35 (17 docs, 17 examples, inventory)", len(surfaces))
	}
	paths := map[string]bool{}
	for _, surface := range surfaces {
		if paths[surface.path] {
			t.Fatalf("duplicate published surface %s", surface.path)
		}
		paths[surface.path] = true
	}
	for _, family := range currentFamilies() {
		for _, form := range family.Forms {
			resourceType, err := providerReferenceTerraformType(form)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{
				filepath.ToSlash(filepath.Join("docs", "resources", providerDocBasename(resourceType))),
				filepath.ToSlash(filepath.Join("examples", "resources", resourceType, "resource.tf")),
			} {
				if !paths[path] {
					t.Errorf("missing generated public surface for %s/%s: %s", family.Group, form.Kind, path)
				}
			}
		}
	}
}

func TestAllFamilyPublishedSurfacesUseTheirOwnExactIdentity(t *testing.T) {
	t.Parallel()

	forms := map[string]map[string]bool{
		"edge.forms.takoform.com/ModuleWorker": {
			"exact FormRef `edge.forms.takoform.com/ModuleWorker`":                                                                                true,
			"takoform-forms/blob/3231633605b737ce5279d7fc020b4780568e7091/forms/candidates/edge.forms.takoform.com/module-worker/definition.json": true,
		},
		"edge.forms.takoform.com/SQLiteDatabase": {
			"exact FormRef `edge.forms.takoform.com/SQLiteDatabase`": true,
		},
		"edge.forms.takoform.com/WorkerVersion": {},
	}
	for _, family := range currentFamilies() {
		for _, form := range family.Forms {
			key := family.Group + "/" + form.Kind
			needles, checked := forms[key]
			if !checked {
				continue
			}
			content := v3ResourceDoc(form) + v3ExampleHCL(form)
			for needle := range needles {
				if !strings.Contains(content, needle) {
					t.Errorf("%s generated surface is missing %q", key, needle)
				}
			}
			ref := providerReferenceSurfaceForForm(form)
			for _, needle := range []string{
				fmt.Sprintf(`"apiVersion": %q`, ref.FormRef.APIVersion),
				fmt.Sprintf(`"kind": %q`, ref.FormRef.Kind),
				fmt.Sprintf(`"definitionVersion": %q`, ref.FormRef.DefinitionVersion),
				fmt.Sprintf(`"schemaDigest": %q`, ref.FormRef.SchemaDigest),
			} {
				if !strings.Contains(content, needle) {
					t.Errorf("%s generated surface is missing exact FormRef member %q", key, needle)
				}
			}
			if ref.PackageDigest != "" {
				needle := fmt.Sprintf("`packageDigest` — Form Package digest (separate from FormRef; embedded Provider provenance): `%s`", ref.PackageDigest)
				if !strings.Contains(content, needle) {
					t.Errorf("%s generated surface is missing package provenance %q", key, needle)
				}
			}
			delete(forms, key)
		}
	}
	for key := range forms {
		t.Errorf("current Provider mapping inventory is missing test subject %s", key)
	}
}

func TestWorkerVersionRuntimeInputDocumentationPinsProviderScopedApplyOnlyTransport(t *testing.T) {
	t.Parallel()

	var document string
	for _, surface := range renderPublishedSurfaces() {
		if surface.path == "docs/resources/worker_version.md" {
			document = string(surface.content)
			break
		}
	}
	if document == "" {
		t.Fatal("current Form inventory is missing WorkerVersion")
	}
	const limitContract = "The map is limited to 1..64 bindings, each value to 1..32768 bytes of UTF-8 text without NUL, and the runner dispatch to 1 MiB total. These limits are separate from the value-free public apply envelope: `publicApply.path` is limited to 8,192 UTF-8 bytes and `publicApply.body` is limited to 1,048,576 UTF-8 bytes. An overlong path, body, binding, or dispatch is refused before a commitment, private preparation, or public Host mutation."
	if !strings.Contains(document, limitContract) {
		t.Fatalf("WorkerVersion runtime-input documentation does not pin the provider-scoped Apply-only limits:\n%s", document)
	}
	for _, required := range []string{"runtime_input_nonce", "runtime_inputs", "ephemeral root variable", "different Provider instance"} {
		if !strings.Contains(document, required) {
			t.Errorf("WorkerVersion runtime-input documentation is missing %q", required)
		}
	}
}

func TestCurrentPublishedSurfacesExcludeProvider3AggregateForms(t *testing.T) {
	t.Parallel()

	content := ""
	for _, surface := range renderPublishedSurfaces() {
		content += surface.path + "\n" + string(surface.content) + "\n"
	}
	for _, forbidden := range []string{
		"function.forms.takoform.com",
		"container.forms.takoform.com",
		"table.forms.takoform.com",
		"queue.forms.takoform.com",
		"topic.forms.takoform.com",
		"schedule.forms.takoform.com",
		"vector.forms.takoform.com",
		"takoform_function",
		"takoform_serverless_container_service",
		"takoform_table",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("current publisher-selected Provider surface still contains withdrawn Provider 3 aggregate identity %q", forbidden)
		}
	}
}
