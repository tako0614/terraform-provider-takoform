package provider

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

func TestProviderV3ProductionSeamsUseSnapshotAssembly(t *testing.T) {
	assembly := mustProviderV3SnapshotAssembly()
	if got := v3TerraformResourceTypes(); got != assembly.resourceTypes {
		t.Fatal("production resource-type seam does not return the Snapshot assembly registry")
	}
	if got := v3Codecs(); got != assembly.codecs {
		t.Fatal("production codec seam does not return the Snapshot assembly codec table")
	}
	if !canonicalJSONEqual(providerV3CurrentForms(), assembly.currentForms) {
		t.Fatal("production current-Forms seam does not return the Snapshot assembly projection")
	}
	factories := newV3FormResources()
	if len(factories) != len(assembly.currentForms) {
		t.Fatalf("production factories = %d, Snapshot forms = %d", len(factories), len(assembly.currentForms))
	}
	for index, factory := range factories {
		resource, ok := factory().(*v3FormResource)
		if !ok {
			t.Fatalf("production factory %d returned %T", index, factory())
		}
		if resource.codecs != assembly.codecs {
			t.Fatalf("production factory %d does not carry the Snapshot codec table", index)
		}
		key := assembly.projection.currentOrder[index]
		mapping := assembly.projection.resources[key]
		if resource.resourceType != mapping.ResourceType || !canonicalJSONEqual(resource.form, assembly.projection.forms[key].Form) {
			t.Fatalf("production factory %d does not carry exact projected Form/resource mapping %s", index, key)
		}
		if !canonicalJSONEqual(resource.artifact, mapping.Artifact) {
			t.Fatalf("production factory %d does not carry exact projected artifact rule %s", index, key)
		}
	}
}

// v3ProviderCurrentFormByKind is test lookup over the already verified
// embedded Provider projection. It exists only for older harnesses whose
// public helper accepts a kind string; all production dispatch remains exact
// GroupKind/ FormRef lookup through the assembly registry.
func v3ProviderCurrentFormByKind(t *testing.T, kind string) model.Form {
	t.Helper()
	for _, form := range mustProviderV3SnapshotAssembly().currentForms {
		if form.Kind == kind {
			return form
		}
	}
	t.Fatalf("embedded Provider projection has no current Form kind %q", kind)
	return model.Form{}
}

func v3ProviderArtifactForForm(t *testing.T, form model.Form) (*v3ArtifactProjection, bool) {
	t.Helper()
	assembly := mustProviderV3SnapshotAssembly()
	ref, err := assembly.registry.DefaultCreate(v3GroupKind{
		APIVersion: form.Family.APIVersion(), Kind: form.Kind,
	})
	if err != nil {
		t.Fatalf("embedded Provider projection has no default-create ref for %s/%s: %v", form.Family.APIVersion(), form.Kind, err)
	}
	resource, ok := assembly.projection.resources[ref.ExactKey()]
	if !ok {
		t.Fatalf("embedded Provider projection has no resource mapping for %s", ref.ExactKey())
	}
	return cloneV3ArtifactProjection(resource.Artifact), resource.Artifact != nil
}

func v3ProviderArtifactProjectionForKind(t *testing.T, kind string) (*v3ArtifactProjection, bool) {
	t.Helper()
	return v3ProviderArtifactForForm(t, v3ProviderCurrentFormByKind(t, kind))
}

func TestProviderV3SnapshotAssemblyUsesExactProjectedDefinitions(t *testing.T) {
	assembly := mustProviderV3SnapshotAssembly()
	if len(assembly.currentForms) != len(assembly.projection.currentOrder) {
		t.Fatalf("embedded current Forms = %d, projection registration order = %d", len(assembly.currentForms), len(assembly.projection.currentOrder))
	}
	for index, form := range assembly.currentForms {
		key := assembly.projection.currentOrder[index]
		projected, ok := assembly.projection.forms[key]
		if !ok {
			t.Fatalf("projection registration order %d references missing Form %s", index, key)
		}
		if projected.Generation != v3ProjectionCurrent || projected.Form.Kind != form.Kind ||
			projected.Form.Family.APIVersion() != form.Family.APIVersion() ||
			projected.Form.DefinitionVersion != form.DefinitionVersion {
			t.Fatalf("current Form %d does not match exact projection %s", index, key)
		}
		definitionRaw, ok := assembly.snapshot.Definition(formpackage.FormRef{
			APIVersion: key.APIVersion, Kind: key.Kind,
			DefinitionVersion: key.DefinitionVersion, SchemaDigest: key.SchemaDigest,
		})
		if !ok {
			t.Fatalf("Snapshot Definition missing for %s", key)
		}
		canonical, err := formpackage.Canonicalize(definitionRaw)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(definitionRaw, canonical) {
			t.Fatalf("Snapshot Definition %s is not canonical", key)
		}
		codec, ok := assembly.codecs.codecs[key]
		if !ok || codec.Form.Kind != form.Kind {
			t.Fatalf("embedded codec for %s does not carry its projected Form", key)
		}
	}
}

func TestProviderV3SnapshotPathImportsNoOfficialCatalog(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not identify the Provider source directory")
	}
	directory := filepath.Dir(thisFile)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	legacyReferences := map[string]struct{}{
		"legacyProviderV3CurrentForms":   {},
		"legacyV3TerraformResourceTypes": {},
		"legacyV3Codecs":                 {},
		"providerV3CurrentFamilies":      {},
		"providerV3ResourceTypeLines":    {},
		"V3Current":                      {},
		"V3ForKind":                      {},
		"V3Registry":                     {},
	}
	declaredProductionSeams := map[string]string{}
	artifactRuleFound := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		sourcePath := filepath.Join(directory, name)
		parsed, err := parser.ParseFile(token.NewFileSet(), sourcePath, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			pathValue, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(pathValue, "formcatalog") {
				t.Errorf("official catalog import %q appears in production source %s", pathValue, name)
			}
		}

		parsed, err = parser.ParseFile(token.NewFileSet(), sourcePath, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok {
				if _, forbidden := legacyReferences[identifier.Name]; forbidden {
					t.Errorf("production source %s references comparison-only seam %s", name, identifier.Name)
				}
			}
			return true
		})
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Name.Name == "v3ArtifactRule" {
				artifactRuleFound = true
				if name != "v3_snapshot_projection.go" {
					t.Errorf("v3ArtifactRule is declared in %s, want v3_snapshot_projection.go", name)
				}
				forbiddenFallbacks := map[string]struct{}{
					"providerV3SnapshotAssembly":     {},
					"mustProviderV3SnapshotAssembly": {},
					"v3TerraformResourceTypes":       {},
					"DefaultCreate":                  {},
					"Lookup":                         {},
					"Artifact":                       {},
				}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					identifier, ok := node.(*ast.Ident)
					if ok {
						if _, forbidden := forbiddenFallbacks[identifier.Name]; forbidden {
							t.Errorf("v3ArtifactRule contains runtime projection fallback %s", identifier.Name)
						}
					}
					return true
				})
			}
			switch function.Name.Name {
			case "providerV3CurrentForms", "v3TerraformResourceTypes", "v3Codecs":
				declaredProductionSeams[function.Name.Name] = name
			}
		}
	}
	if !artifactRuleFound {
		t.Error("production source does not declare v3ArtifactRule")
	}
	wantSeams := []string{"providerV3CurrentForms", "v3Codecs", "v3TerraformResourceTypes"}
	sort.Strings(wantSeams)
	for _, seam := range wantSeams {
		if declaredProductionSeams[seam] != "v3_snapshot_assembly.go" {
			t.Errorf("production seam %s is declared in %q, want v3_snapshot_assembly.go", seam, declaredProductionSeams[seam])
		}
	}
}
