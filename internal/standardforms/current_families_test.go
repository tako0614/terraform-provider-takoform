package standardforms

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCurrentFamilyInventoryIsProviderNeutralAndComplete(t *testing.T) {
	t.Parallel()

	wantGroups := []string{
		"edge.forms.takoform.com",
		"function.forms.takoform.com",
		"container.forms.takoform.com",
		"table.forms.takoform.com",
		"queue.forms.takoform.com",
		"topic.forms.takoform.com",
		"schedule.forms.takoform.com",
		"vector.forms.takoform.com",
	}
	wantCounts := []int{16, 4, 5, 1, 1, 2, 1, 1}
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
		for _, form := range family.Forms {
			if strings.Contains(form.Kind, "ObjectBucket") {
				t.Fatalf("current Provider 3 mapping includes withdrawn Form %s/%s", family.Group, form.Kind)
			}
		}
		total += len(family.Forms)
	}
	if total != 31 {
		t.Fatalf("current Form count = %d, want 31", total)
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
	if len(seenTypes) != 31 {
		t.Fatalf("provider reference type count = %d, want 31", len(seenTypes))
	}
}

func TestPublishedSurfaceInventoryCoversEveryCurrentProviderMapping(t *testing.T) {
	t.Parallel()

	surfaces := renderPublishedSurfaces()
	if len(surfaces) != 63 {
		t.Fatalf("published surface count = %d, want 63 (31 docs, 31 examples, inventory)", len(surfaces))
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
			"current exact FormRef `edge.forms.takoform.com/ModuleWorker`":                                                                        true,
			"takoform-forms/blob/026f862975b9adb0e2bfd9c6214a5e6691dfb596/forms/candidates/edge.forms.takoform.com/module-worker/definition.json": true,
		},
		"container.forms.takoform.com/ContainerCustomDomain": {
			"current exact FormRef `container.forms.takoform.com/ContainerCustomDomain`": true,
			"embedded Provider projection":                true,
			"target `ContainerService` resource":          true,
			`"apiVersion":"container.forms.takoform.com"`: true,
		},
		"function.forms.takoform.com/FunctionVersion": {
			"current exact FormRef `function.forms.takoform.com/FunctionVersion`": true,
		},
		"container.forms.takoform.com/ContainerRevision": {},
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
			if owner := map[string]string{
				"function.forms.takoform.com/FunctionVersion":    "function",
				"container.forms.takoform.com/ContainerRevision": "container-service",
			}[key]; owner != "" {
				pattern := regexp.MustCompile(`revision_owner\s*=\s*"` + regexp.QuoteMeta(owner) + `"`)
				if !pattern.MatchString(content) {
					t.Errorf("%s generated example does not use revision owner %q", key, owner)
				}
			}
			delete(forms, key)
		}
	}
	for key := range forms {
		t.Errorf("current Provider mapping inventory is missing test subject %s", key)
	}
}

func TestScheduleExampleProjectsNestedResourceRefsToProviderNames(t *testing.T) {
	t.Parallel()

	var scheduleExample string
	for _, family := range currentFamilies() {
		if family.Group != "schedule.forms.takoform.com" {
			continue
		}
		for _, form := range family.Forms {
			if form.Kind == "Schedule" {
				scheduleExample = v3ExampleHCL(form)
			}
		}
	}
	if scheduleExample == "" {
		t.Fatal("current family inventory is missing Schedule")
	}
	if !strings.Contains(scheduleExample, `"queue" = "scheduled-work"`) {
		t.Fatalf("Schedule example does not project target.queue to the provider's resource-name attribute:\n%s", scheduleExample)
	}
	for _, forbidden := range []string{
		`"queue" = {`,
		`"apiVersion" = "queue.forms.takoform.com"`,
		`"kind" = "PullQueue"`,
	} {
		if strings.Contains(scheduleExample, forbidden) {
			t.Errorf("Schedule example leaks wire-only ResourceRef member %q:\n%s", forbidden, scheduleExample)
		}
	}
}
