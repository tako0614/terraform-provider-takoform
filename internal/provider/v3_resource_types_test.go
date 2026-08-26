package provider

import (
	"context"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

func TestV3ProviderRegistrationRequiresExactTerraformMapping(t *testing.T) {
	t.Parallel()
	form := v3ProviderCurrentFormByKind(t, "ModuleWorker")
	line := v3GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
	registry := mustProviderV3SnapshotAssembly().registry
	ref, err := registry.DefaultCreate(line)
	if err != nil {
		t.Fatal(err)
	}

	mapped, err := newV3ResourceTypeRegistry(registry, []v3ResourceTypeLine{{
		GroupKind: line, ResourceType: "takoform_module_worker",
	}})
	if err != nil {
		t.Fatal(err)
	}
	factories, err := compileV3FormResources(
		[]model.Form{form}, registry, mapped, v3Codecs(),
	)
	if err != nil {
		t.Fatalf("mapped provider registration failed: %v", err)
	}
	var metadata frameworkresource.MetadataResponse
	factories[0]().Metadata(context.Background(), frameworkresource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
	if metadata.TypeName != "takoform_module_worker" {
		t.Fatalf("registered Terraform type = %q", metadata.TypeName)
	}

	empty, err := newV3ResourceTypeRegistry(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compileV3FormResources(
		[]model.Form{form}, registry, empty, v3Codecs(),
	); err == nil || !strings.Contains(err.Error(), ref.ExactKey().String()) {
		t.Fatalf("missing exact mapping error = %v", err)
	}
}

func TestV3ProviderMappingRejectsDuplicatesAndDoesNotFallback(t *testing.T) {
	t.Parallel()
	form := v3ProviderCurrentFormByKind(t, "ModuleWorker")
	line := v3GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
	registry := mustProviderV3SnapshotAssembly().registry
	duplicate := []v3ResourceTypeLine{
		{GroupKind: line, ResourceType: "takoform_module_worker"},
		{GroupKind: line, ResourceType: "takoform_module_worker_alternate"},
	}
	if _, err := newV3ResourceTypeRegistry(registry, duplicate); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate mapping error = %v", err)
	}

	mapped, err := newV3ResourceTypeRegistry(registry, duplicate[:1])
	if err != nil {
		t.Fatal(err)
	}
	ref, err := registry.DefaultCreate(line)
	if err != nil {
		t.Fatal(err)
	}
	wrong := ref.ExactKey()
	wrong.SchemaDigest = "sha256:" + strings.Repeat("f", 64)
	if _, ok := mapped.Lookup(wrong); ok {
		t.Fatal("wrong exact digest fell back to the mapped Form line")
	}
	wrong.APIVersion = "other.forms.example"
	if _, ok := mapped.Lookup(wrong); ok {
		t.Fatal("wrong group fell back to a same-kind provider mapping")
	}
}

func TestV3CurrentMappingOmitsWithdrawnObjectBucketAndRetainedMapping(t *testing.T) {
	t.Parallel()
	assembly := mustProviderV3SnapshotAssembly()
	for _, ref := range assembly.registry.SupportedRefs() {
		if ref.Kind != "ObjectBucket" {
			continue
		}
		if _, ok := v3TerraformResourceTypes().Lookup(ref.ExactKey()); ok {
			t.Fatalf("withdrawn retained ObjectBucket exact ref remains provider-mapped: %s", ref.ExactKey())
		}
	}
}

func TestTerraformTypeMappingCannotChangeFormDigest(t *testing.T) {
	t.Parallel()
	form := v3ProviderCurrentFormByKind(t, "ModuleWorker")
	line := v3GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
	registry := mustProviderV3SnapshotAssembly().registry
	first, err := newV3ResourceTypeRegistry(registry, []v3ResourceTypeLine{{
		GroupKind: line, ResourceType: "takoform_module_worker",
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := newV3ResourceTypeRegistry(registry, []v3ResourceTypeLine{{
		GroupKind: line, ResourceType: "takoform_alternative_module_worker",
	}})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := registry.DefaultCreate(line)
	if err != nil {
		t.Fatal(err)
	}
	firstType, _ := first.Lookup(ref.ExactKey())
	secondType, _ := second.Lookup(ref.ExactKey())
	if firstType == secondType {
		t.Fatal("test mappings do not differ")
	}

	definition, ok := mustProviderV3SnapshotAssembly().snapshot.Definition(ref.FormRef())
	if !ok {
		t.Fatalf("Snapshot Definition for %s is missing", ref.ExactKey())
	}
	digest, err := formpackage.DigestCanonicalJSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	if digest != ref.SchemaDigest {
		t.Fatalf("Form digest = %q, registry ref = %q", digest, ref.SchemaDigest)
	}
}
