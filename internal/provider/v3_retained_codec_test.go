package provider

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/retainededgeformcatalog"
)

func TestRetainedProvider211DefinitionsRemainByteIdentical(t *testing.T) {
	t.Parallel()
	want := map[string][]byte{
		"WorkerVersion":    retainedWorkerVersionDefinition,
		"WorkerDeployment": retainedWorkerDeploymentDefinition,
	}
	for kind, definitionJSON := range want {
		kind, definitionJSON := kind, definitionJSON
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			refs := currentformregistry.V3Current().SupportedRefsFor(currentformregistry.GroupKind{
				APIVersion: retainededgeformcatalog.Family.APIVersion(), Kind: kind,
			})
			if len(refs) != 1 {
				t.Fatalf("retained %s identity count = %d, want 1", kind, len(refs))
			}
			ref := refs[0]
			digest, err := formpackage.DigestCanonicalJSON(definitionJSON)
			if err != nil {
				t.Fatal(err)
			}
			if digest != ref.SchemaDigest {
				t.Fatalf("embedded %s digest = %s, Provider 2.1.1 identity = %s", kind, digest, ref.SchemaDigest)
			}
			if _, err := formpackage.ValidateDefinition(definitionJSON); err != nil {
				t.Fatalf("embedded %s definition is invalid: %v", kind, err)
			}
		})
	}
}

func TestRetainedObjectBucketHistoryIsNotAProvider3Surface(t *testing.T) {
	t.Parallel()
	refs := currentformregistry.V3Current().SupportedRefsFor(currentformregistry.GroupKind{
		APIVersion: retainededgeformcatalog.Family.APIVersion(), Kind: "ObjectBucket",
	})
	if len(refs) != 1 {
		t.Fatalf("retained ObjectBucket identity count = %d, want 1", len(refs))
	}
	ref := refs[0]
	rendered, err := retainededgeformcatalog.RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, form := range rendered {
		if form.Kind != "ObjectBucket" {
			continue
		}
		found = true
		digest, err := formpackage.DigestCanonicalJSON([]byte(form.DefinitionJSON))
		if err != nil {
			t.Fatal(err)
		}
		if digest != ref.SchemaDigest {
			t.Fatalf("retained ObjectBucket digest = %s, ledger = %s", digest, ref.SchemaDigest)
		}
	}
	if !found {
		t.Fatal("retained ObjectBucket definition disappeared from Provider 2.1.1 history")
	}
	if _, ok := v3TerraformResourceTypes().Lookup(ref.ExactKey()); ok {
		t.Fatal("Provider 3 maps retained ObjectBucket")
	}
	if _, ok := v3Codecs().forStateKey(ref.ExactKey()); ok {
		t.Fatal("Provider 3 carries a retained ObjectBucket state codec")
	}
}

func TestRetainedWorkerVersionBucketBindingsRemainReadableStateOnly(t *testing.T) {
	ctx := context.Background()
	host := newV3FakeHost(t)
	resource := v3TestFormResource(t, "WorkerVersion", newV3TestProviderData(t, host))
	schemaResponse := v3SchemaOf(t, resource)

	declared, ok := schemaResponse.Schema.Attributes["bucket_bindings"]
	if !ok {
		t.Fatal("Provider 3 WorkerVersion schema dropped retained bucket_bindings state")
	}
	attribute, ok := declared.(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("bucket_bindings schema = %T, want ListNestedAttribute", declared)
	}
	if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
		t.Fatalf(
			"bucket_bindings flags = computed:%t optional:%t required:%t, want state-only computed",
			attribute.IsComputed(), attribute.IsOptional(), attribute.IsRequired(),
		)
	}

	refs := currentformregistry.V3Current().SupportedRefsFor(currentformregistry.GroupKind{
		APIVersion: retainededgeformcatalog.Family.APIVersion(), Kind: "WorkerVersion",
	})
	if len(refs) != 1 {
		t.Fatalf("retained WorkerVersion identity count = %d, want 1", len(refs))
	}
	ref := refs[0]
	raw, err := os.ReadFile("../../forms/candidates/edge/v1beta1/worker-version/fixtures/desired.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	host.resources[host.resourceKey("WorkerVersion", "worker-version")] = &v3HostRecord{
		uid: "uid-retained", generation: 1, revision: 1,
		apiVersion: ref.APIVersion, kind: ref.Kind, space: "prod", spec: spec,
		form: map[string]any{"formRef": map[string]any{
			"apiVersion": ref.APIVersion, "kind": ref.Kind,
			"definitionVersion": ref.DefinitionVersion, "schemaDigest": ref.SchemaDigest,
		}},
	}

	state := tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}
	for name, value := range map[string]string{
		"name": "worker-version", "space": "prod", "uid": "uid-retained",
		"generation": "1", "revision": "1",
	} {
		if diags := state.SetAttribute(ctx, path.Root(name), types.StringValue(value)); diags.HasError() {
			t.Fatalf("seed state %s: %v", name, diags)
		}
	}
	state = v3SeedStateRef(t, ctx, state, ref)
	binding := types.ObjectValueMust(
		map[string]attr.Type{"name": types.StringType, "target_name": types.StringType},
		map[string]attr.Value{
			"name":        types.StringValue("MEDIA"),
			"target_name": types.StringValue("object-bucket"),
		},
	)
	if diags := state.SetAttribute(
		ctx,
		path.Root("bucket_bindings"),
		types.ListValueMust(binding.Type(ctx), []attr.Value{binding}),
	); diags.HasError() {
		t.Fatalf("seed retained bucket_bindings state: %v", diags)
	}
	readResponse := frameworkresource.ReadResponse{State: state}
	resource.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read retained WorkerVersion state: %v", readResponse.Diagnostics)
	}
	var bindings types.List
	if diags := readResponse.State.GetAttribute(ctx, path.Root("bucket_bindings"), &bindings); diags.HasError() {
		t.Fatalf("read bucket_bindings state: %v", diags)
	}
	if bindings.IsNull() || bindings.IsUnknown() || len(bindings.Elements()) != 1 {
		t.Fatalf("bucket_bindings state = %#v, want the retained MEDIA binding", bindings)
	}
}
