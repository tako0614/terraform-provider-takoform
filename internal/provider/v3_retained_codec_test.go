package provider

import (
	"testing"

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
