package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/retainededgeformcatalog"
)

// TestWriteProviderV3SnapshotArtifacts is the explicit comparison-only writer
// for the immutable W07 artifacts. It reads the legacy catalog assembly only
// when the update flag is set; production parsing and assembly never import an
// official family package. W08 deletes this writer with the legacy path after
// the W02-W07 parity review.
func TestWriteProviderV3SnapshotArtifacts(t *testing.T) {
	if os.Getenv("TAKOFORM_UPDATE_PROVIDER_V3_SNAPSHOT") != "1" {
		t.Skip("set TAKOFORM_UPDATE_PROVIDER_V3_SNAPSHOT=1 to rewrite the checked-in Provider 3 artifacts")
	}

	repositoryRoot := filepath.Join("..", "..")
	artifactRoot := filepath.Join("artifacts", "v3")
	legacyRegistry := currentformregistry.V3Current()
	legacyTypes := legacyV3TerraformResourceTypes()
	projection := v3ProviderProjection{Format: providerV3ProjectionFormat, HostAPI: providerV3HostAPI}

	currentForms := legacyProviderV3CurrentForms()
	for order, form := range currentForms {
		ref, err := legacyRegistry.DefaultCreate(currentformregistry.GroupKind{
			APIVersion: form.Family.APIVersion(), Kind: form.Kind,
		})
		if err != nil {
			t.Fatal(err)
		}
		resourceType, ok := legacyTypes.Lookup(ref.ExactKey())
		if !ok {
			t.Fatalf("legacy current Form %s has no resource type", ref.ExactKey())
		}
		projection.Forms = append(projection.Forms, v3ProjectedForm{Generation: v3ProjectionCurrent, Ref: ref, Form: form})
		registrationOrder := order
		projection.Resources = append(projection.Resources, v3ProjectedResource{
			Ref: ref, ResourceType: resourceType, Register: true, RegistrationOrder: &registrationOrder,
			Artifact: legacyV3ArtifactProjection(form.Kind),
		})
		projection.DefaultCreates = append(projection.DefaultCreates, ref)
		projection.ReadableRefs = append(projection.ReadableRefs, ref)
	}

	retainedRendered, err := retainededgeformcatalog.RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	if len(retainedRendered) != len(retainededgeformcatalog.Forms) {
		t.Fatalf("retained rendered/model count = %d/%d", len(retainedRendered), len(retainededgeformcatalog.Forms))
	}
	for index, form := range retainededgeformcatalog.Forms {
		refs := legacyRegistry.SupportedRefsFor(currentformregistry.GroupKind{
			APIVersion: form.Family.APIVersion(), Kind: form.Kind,
		})
		if len(refs) != 1 {
			t.Fatalf("retained Form %s resolves %d exact refs", form.Kind, len(refs))
		}
		ref := refs[0]
		definition := []byte(retainedRendered[index].DefinitionJSON)
		if frozen, ok := retainedFrozenDefinition(form.Kind); ok {
			definition = frozen
		}
		generation := v3ProjectionRetained
		if _, mapped := legacyTypes.Lookup(ref.ExactKey()); !mapped {
			generation = v3ProjectionUnreadable
		}
		projection.Forms = append(projection.Forms, v3ProjectedForm{
			Generation: generation, Ref: ref, Form: form, Definition: json.RawMessage(definition),
		})
		if generation == v3ProjectionUnreadable {
			continue
		}
		resourceType, _ := legacyTypes.Lookup(ref.ExactKey())
		projection.Resources = append(projection.Resources, v3ProjectedResource{Ref: ref, ResourceType: resourceType})
		projection.ReadableRefs = append(projection.ReadableRefs, ref)
	}

	sort.Slice(projection.Forms, func(i, j int) bool { return lessV3ProjectionRef(projection.Forms[i].Ref, projection.Forms[j].Ref) })
	sort.Slice(projection.Resources, func(i, j int) bool {
		return lessV3ProjectionRef(projection.Resources[i].Ref, projection.Resources[j].Ref)
	})
	sort.Slice(projection.DefaultCreates, func(i, j int) bool {
		return lessV3ProjectionRef(projection.DefaultCreates[i], projection.DefaultCreates[j])
	})
	sort.Slice(projection.ReadableRefs, func(i, j int) bool {
		return lessV3ProjectionRef(projection.ReadableRefs[i], projection.ReadableRefs[j])
	})
	projectionRaw := writeProviderV3JSON(t, filepath.Join(artifactRoot, "projection.json"), projection)
	projectionDigest, err := formpackage.DigestCanonicalJSON(projectionRaw)
	if err != nil {
		t.Fatal(err)
	}

	closure := v3ArtifactClosure{
		Format:     providerV3ArtifactClosureFormat,
		Projection: v3ArtifactClosureFile{Path: "projection.json", Digest: projectionDigest},
	}
	for _, form := range currentForms {
		ref, err := legacyRegistry.DefaultCreate(currentformregistry.GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind})
		if err != nil {
			t.Fatal(err)
		}
		packageRoot := filepath.Join(repositoryRoot, "forms", "candidates", form.Family.APIVersion(), form.Slug)
		report, err := formpackage.VerifyDirectory(packageRoot)
		if err != nil {
			t.Fatalf("verify source package %s: %v", packageRoot, err)
		}
		if report.FormRef.APIVersion != ref.APIVersion || report.FormRef.Kind != ref.Kind ||
			report.FormRef.DefinitionVersion != ref.DefinitionVersion || report.FormRef.SchemaDigest != ref.SchemaDigest ||
			report.PackageDigest != ref.PackageDigest {
			t.Fatalf("source package %s does not match legacy ref %#v: %#v", packageRoot, ref, report)
		}
		closure.Packages = append(closure.Packages, v3ArtifactClosurePackage{
			Root:    filepath.ToSlash(filepath.Join("packages", form.Family.APIVersion(), form.Slug)),
			FormRef: report.FormRef, PackageDigest: report.PackageDigest,
		})
	}
	sort.Slice(closure.Packages, func(i, j int) bool { return closure.Packages[i].Root < closure.Packages[j].Root })

	closure.Interfaces = legacyV3InterfaceClosure(t, repositoryRoot)
	closure.Bindings = legacyV3BindingClosure(t, repositoryRoot)
	writeProviderV3JSON(t, filepath.Join(artifactRoot, "closure.json"), closure)

	if _, err := loadProviderV3Assembly(os.DirFS(artifactRoot), "."); err != nil {
		t.Fatalf("generated Provider 3 artifact closure does not assemble: %v", err)
	}
}

func legacyV3ArtifactProjection(kind string) *v3ArtifactProjection {
	if kind == workerBundleKind {
		return &v3ArtifactProjection{
			Mode: v3ArtifactModeWorkerBundle, ManifestKind: workerBundleKind,
			MaximumFiles: workerBundleMaxModules, MaximumFileSize: workerBundleMaxModuleBytes,
			MediaTypes: modelBundleMediaTypes(),
		}
	}
	manifestKind, fileBundle := v3FileBundleManifestKind(kind)
	if !fileBundle {
		return nil
	}
	projection := &v3ArtifactProjection{
		Mode: v3ArtifactModeFileBundle, ManifestKind: manifestKind,
		MaximumFiles: artifactBundleMaxFiles, MaximumFileSize: artifactFileMaxBytes,
	}
	if kind == sqliteMigrationSetKind {
		projection.MediaTypes = []string{"application/sql"}
	}
	return projection
}

func modelBundleMediaTypes() []string {
	return append([]string(nil), workerBundleMediaTypes...)
}

func legacyV3InterfaceClosure(t *testing.T, repositoryRoot string) []v3ArtifactClosureInterface {
	t.Helper()
	var candidates struct {
		Interfaces []struct {
			Name         string `json:"name"`
			Version      string `json:"version"`
			SchemaDigest string `json:"schemaDigest"`
		} `json:"interfaces"`
	}
	readProviderV3JSON(t, filepath.Join(repositoryRoot, "interfaces", "candidates", "v1alpha1", "candidate-set.json"), &candidates)
	entries := make([]v3ArtifactClosureInterface, 0, len(candidates.Interfaces))
	for _, candidate := range candidates.Interfaces {
		path := filepath.ToSlash(filepath.Join("interfaces", candidate.Name, "definition.json"))
		raw, err := os.ReadFile(filepath.Join("artifacts", "v3", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		var identity struct {
			APIVersion string `json:"apiVersion"`
			Name       string `json:"name"`
			Version    string `json:"version"`
		}
		if err := formpackage.ValidateInterfaceDefinition(raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &identity); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, v3ArtifactClosureInterface{Path: path, Ref: formpackage.InterfaceRef{
			APIVersion: identity.APIVersion, Name: identity.Name, Version: identity.Version, SchemaDigest: candidate.SchemaDigest,
		}})
	}
	return entries
}

func legacyV3BindingClosure(t *testing.T, repositoryRoot string) []v3ArtifactClosureBinding {
	t.Helper()
	var candidates struct {
		Bindings []struct {
			Name         string `json:"name"`
			Version      string `json:"version"`
			SchemaDigest string `json:"schemaDigest"`
		} `json:"bindings"`
	}
	readProviderV3JSON(t, filepath.Join(repositoryRoot, "bindings", "candidates", "v1alpha2", "candidate-set.json"), &candidates)
	entries := make([]v3ArtifactClosureBinding, 0, len(candidates.Bindings))
	for _, candidate := range candidates.Bindings {
		path := filepath.ToSlash(filepath.Join("bindings", candidate.Name, "definition.json"))
		raw, err := os.ReadFile(filepath.Join("artifacts", "v3", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		var identity struct {
			APIVersion string `json:"apiVersion"`
			Name       string `json:"name"`
			Version    string `json:"version"`
		}
		if err := formpackage.ValidateBindingDefinition(raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &identity); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, v3ArtifactClosureBinding{Path: path, Ref: formpackage.BindingRef{
			APIVersion: identity.APIVersion, Name: identity.Name, Version: identity.Version, SchemaDigest: candidate.SchemaDigest,
		}})
	}
	return entries
}

func lessV3ProjectionRef(left, right currentformregistry.V3Ref) bool {
	leftKey, rightKey := left.ExactKey(), right.ExactKey()
	if leftKey.APIVersion != rightKey.APIVersion {
		return leftKey.APIVersion < rightKey.APIVersion
	}
	if leftKey.Kind != rightKey.Kind {
		return leftKey.Kind < rightKey.Kind
	}
	if leftKey.DefinitionVersion != rightKey.DefinitionVersion {
		return leftKey.DefinitionVersion < rightKey.DefinitionVersion
	}
	return leftKey.SchemaDigest < rightKey.SchemaDigest
}

func writeProviderV3JSON(t *testing.T, path string, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return raw
}

func readProviderV3JSON(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := formpackage.DigestCanonicalJSON(raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}
