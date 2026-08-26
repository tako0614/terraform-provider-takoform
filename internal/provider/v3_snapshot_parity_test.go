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
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
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

func TestProviderV3SnapshotAssemblyMatchesLegacyExactProjection(t *testing.T) {
	assembly := mustProviderV3SnapshotAssembly()
	legacyForms := legacyProviderV3CurrentForms()
	if !canonicalJSONEqual(assembly.currentForms, legacyForms) {
		t.Fatal("Snapshot current Form models differ from the comparison-only catalog projection")
	}

	legacyRegistry := currentformregistry.V3Current()
	if !canonicalJSONEqual(assembly.registry.SupportedRefs(), legacyRegistry.SupportedRefs()) {
		t.Fatal("Snapshot supported exact FormRef history differs from the legacy registry")
	}
	for _, form := range legacyForms {
		line := currentformregistry.GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
		legacyRef, legacyErr := legacyRegistry.DefaultCreate(line)
		projectedRef, projectedErr := assembly.registry.DefaultCreate(line)
		if legacyErr != nil || projectedErr != nil || legacyRef != projectedRef {
			t.Fatalf("default-create parity for %s/%s: legacy=%#v/%v projected=%#v/%v", line.APIVersion, line.Kind, legacyRef, legacyErr, projectedRef, projectedErr)
		}
	}

	legacyTypes := legacyV3TerraformResourceTypes()
	for _, ref := range legacyRegistry.SupportedRefs() {
		legacyType, legacyMapped := legacyTypes.Lookup(ref.ExactKey())
		projectedType, projectedMapped := assembly.resourceTypes.Lookup(ref.ExactKey())
		if legacyMapped != projectedMapped || legacyType != projectedType {
			t.Fatalf("resource mapping parity for %s: legacy=%q/%t projected=%q/%t", ref.ExactKey(), legacyType, legacyMapped, projectedType, projectedMapped)
		}
	}

	legacyCodecs := legacyV3Codecs()
	if len(legacyCodecs.codecs) != len(assembly.codecs.codecs) {
		t.Fatalf("codec count parity: legacy=%d projected=%d", len(legacyCodecs.codecs), len(assembly.codecs.codecs))
	}
	for key, legacyCodec := range legacyCodecs.codecs {
		projectedCodec, ok := assembly.codecs.codecs[key]
		if !ok || !canonicalJSONEqual(legacyCodec, projectedCodec) {
			t.Fatalf("exact codec parity failed for %s", key)
		}
	}

	for _, family := range providerV3CurrentFamilies() {
		rendered, err := family.render()
		if err != nil {
			t.Fatal(err)
		}
		if len(rendered) != len(family.forms) {
			t.Fatalf("legacy rendered/model count = %d/%d for %s", len(rendered), len(family.forms), family.family.APIVersion())
		}
		for index, form := range family.forms {
			ref, err := legacyRegistry.DefaultCreate(currentformregistry.GroupKind{APIVersion: family.family.APIVersion(), Kind: form.Kind})
			if err != nil {
				t.Fatal(err)
			}
			snapshotRaw, ok := assembly.snapshot.Definition(formpackage.FormRef{
				APIVersion: ref.APIVersion, Kind: ref.Kind, DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
			})
			if !ok {
				t.Fatalf("Snapshot Definition missing for %s", ref.ExactKey())
			}
			legacyCanonical, err := formpackage.Canonicalize([]byte(rendered[index].DefinitionJSON))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(snapshotRaw, legacyCanonical) {
				t.Fatalf("Snapshot Definition bytes differ from legacy rendered identity %s", ref.ExactKey())
			}
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
