package provider

import (
	"context"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

func TestV3ProviderRegistrationRequiresExactTerraformMapping(t *testing.T) {
	t.Parallel()
	form, ok := edgeformcatalog.ByKind("ModuleWorker")
	if !ok {
		t.Fatal("ModuleWorker is not declared")
	}
	line := currentformregistry.GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
	ref, err := currentformregistry.V3Current().DefaultCreate(line)
	if err != nil {
		t.Fatal(err)
	}

	mapped, err := newV3ResourceTypeRegistry(currentformregistry.V3Current(), []v3ResourceTypeLine{{
		GroupKind: line, ResourceType: "takoform_module_worker",
	}})
	if err != nil {
		t.Fatal(err)
	}
	factories, err := compileV3FormResources(
		[]model.Form{form}, currentformregistry.V3Current(), mapped, v3Codecs(),
	)
	if err != nil {
		t.Fatalf("mapped provider registration failed: %v", err)
	}
	var metadata frameworkresource.MetadataResponse
	factories[0]().Metadata(context.Background(), frameworkresource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
	if metadata.TypeName != "takoform_module_worker" {
		t.Fatalf("registered Terraform type = %q", metadata.TypeName)
	}

	empty, err := newV3ResourceTypeRegistry(currentformregistry.V3Current(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compileV3FormResources(
		[]model.Form{form}, currentformregistry.V3Current(), empty, v3Codecs(),
	); err == nil || !strings.Contains(err.Error(), ref.ExactKey().String()) {
		t.Fatalf("missing exact mapping error = %v", err)
	}
}

func TestV3ProviderMappingRejectsDuplicatesAndDoesNotFallback(t *testing.T) {
	t.Parallel()
	form, _ := edgeformcatalog.ByKind("ModuleWorker")
	line := currentformregistry.GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
	duplicate := []v3ResourceTypeLine{
		{GroupKind: line, ResourceType: "takoform_module_worker"},
		{GroupKind: line, ResourceType: "takoform_module_worker_alternate"},
	}
	if _, err := newV3ResourceTypeRegistry(currentformregistry.V3Current(), duplicate); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate mapping error = %v", err)
	}

	mapped, err := newV3ResourceTypeRegistry(currentformregistry.V3Current(), duplicate[:1])
	if err != nil {
		t.Fatal(err)
	}
	ref, err := currentformregistry.V3Current().DefaultCreate(line)
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
	currentGroup := edgeformcatalog.Family.APIVersion()
	for _, line := range providerV3ResourceTypeLines() {
		if line.GroupKind.APIVersion == currentGroup && (line.GroupKind.Kind == "ObjectBucket" || strings.Contains(line.ResourceType, "object_bucket")) {
			t.Fatalf("current provider mapping retains withdrawn ObjectBucket: %#v", line)
		}
	}
	if refs := currentformregistry.V3Current().SupportedRefsFor(currentformregistry.GroupKind{
		APIVersion: currentGroup, Kind: "ObjectBucket",
	}); len(refs) != 0 {
		t.Fatalf("current registry unexpectedly carries ObjectBucket refs: %#v", refs)
	}
	for _, ref := range currentformregistry.V3Current().SupportedRefs() {
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
	form, _ := edgeformcatalog.ByKind("ModuleWorker")
	line := currentformregistry.GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
	first, err := newV3ResourceTypeRegistry(currentformregistry.V3Current(), []v3ResourceTypeLine{{
		GroupKind: line, ResourceType: "takoform_module_worker",
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := newV3ResourceTypeRegistry(currentformregistry.V3Current(), []v3ResourceTypeLine{{
		GroupKind: line, ResourceType: "takoform_alternative_module_worker",
	}})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := currentformregistry.V3Current().DefaultCreate(line)
	if err != nil {
		t.Fatal(err)
	}
	firstType, _ := first.Lookup(ref.ExactKey())
	secondType, _ := second.Lookup(ref.ExactKey())
	if firstType == secondType {
		t.Fatal("test mappings do not differ")
	}

	rendered, err := edgeformcatalog.RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range rendered {
		if candidate.Kind != form.Kind {
			continue
		}
		digest, err := formpackage.DigestCanonicalJSON([]byte(candidate.DefinitionJSON))
		if err != nil {
			t.Fatal(err)
		}
		if digest != ref.SchemaDigest {
			t.Fatalf("Form digest = %q, registry ref = %q", digest, ref.SchemaDigest)
		}
		return
	}
	t.Fatal("rendered ModuleWorker definition is missing")
}
