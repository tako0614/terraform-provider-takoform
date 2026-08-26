package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	frameworkschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/retainededgeformcatalog"
)

const v3Provider3GoldenPath = "testdata/v3-provider3-golden.json"

// v3Provider3Golden is a checked-in, canonical lock of the current Provider 3
// surface. It was captured from the immutable Provider 3.0.0 source and is
// compared with the current provider's public registration/schema/codec APIs.
// There is deliberately no in-test rewrite switch: changing this baseline is
// an explicit release-evidence decision, not an ordinary golden refresh.
type v3Provider3Golden struct {
	Format                string                               `json:"format"`
	SourceTag             string                               `json:"sourceTag"`
	SourceCommit          string                               `json:"sourceCommit"`
	ReleaseEvidenceDigest string                               `json:"releaseEvidenceDigest"`
	ResourceCount         int                                  `json:"resourceCount"`
	Resources             map[string]v3Provider3GoldenResource `json:"resources"`
}

type v3Provider3GoldenResource struct {
	FormRef       currentformregistry.V3Ref             `json:"formRef"`
	SchemaVersion int64                                 `json:"schemaVersion"`
	Attributes    map[string]v3Provider3GoldenAttribute `json:"attributes"`
	Codec         v3Provider3GoldenCodec                `json:"codec"`
}

// v3Provider3GoldenAttribute captures the complete public/state shape rather
// than only the top-level type string. Nested attribute flags are part of the
// Terraform contract too, so List/Set/Map/SingleNested attributes recurse into
// their declared object members.
type v3Provider3GoldenAttribute struct {
	Type      string                                `json:"type"`
	Required  bool                                  `json:"required"`
	Optional  bool                                  `json:"optional"`
	Computed  bool                                  `json:"computed"`
	Sensitive bool                                  `json:"sensitive"`
	Nested    map[string]v3Provider3GoldenAttribute `json:"nested,omitempty"`
}

// The two refs make dispatch auditable in the golden itself. State dispatch
// must use the exact recorded identity; new resources use the one current
// create target. PackageDigest is retained on both refs as provenance even
// though it is deliberately excluded from ExactFormKey.
type v3Provider3GoldenCodec struct {
	StateRef         currentformregistry.V3Ref `json:"stateRef"`
	DefaultCreateRef currentformregistry.V3Ref `json:"defaultCreateRef"`
}

func TestV3Provider3GoldenLocksCurrentSurface(t *testing.T) {
	want := readV3Provider3Golden(t)
	got := deriveV3Provider3Golden(t)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Provider 3 golden drifted:\n%s", v3Provider3GoldenDiff(t, want, got))
	}
}

// TestV3Provider3GoldenRetainsProvider211History keeps the Provider 2.1.1
// compatibility boundary visible beside the current 31-resource lock. The
// historical ObjectBucket identity remains in the registry for old state, but
// Provider 3 must neither map nor decode it. Other retained identities remain
// mapped and exact-codec readable. This test only reads the existing registry
// and retained catalog; it never rewrites release ledgers.
func TestV3Provider3GoldenRetainsProvider211History(t *testing.T) {
	registry := currentformregistry.V3Current()
	types := v3TerraformResourceTypes()
	codecs := v3Codecs()
	retainedGroup := retainededgeformcatalog.Family.APIVersion()

	retained := make([]currentformregistry.V3Ref, 0)
	for _, ref := range registry.SupportedRefs() {
		if ref.APIVersion == retainedGroup {
			retained = append(retained, ref)
		}
	}
	if len(retained) != 15 {
		t.Fatalf("retained Provider 2.1.1 identity count = %d, want 15", len(retained))
	}
	for _, ref := range retained {
		mapped := false
		if _, ok := types.Lookup(ref.ExactKey()); ok {
			mapped = true
		}
		_, codecSupported := codecs.forStateKey(ref.ExactKey())
		if ref.Kind == "ObjectBucket" {
			if mapped || codecSupported {
				t.Fatalf("retained ObjectBucket crossed the Provider 3 boundary: mapped=%t codec=%t", mapped, codecSupported)
			}
			continue
		}
		if !mapped || !codecSupported {
			t.Fatalf("retained %s is no longer readable through its exact Provider 2.1.1 identity: mapped=%t codec=%t", ref.Kind, mapped, codecSupported)
		}
	}
}

func deriveV3Provider3Golden(t *testing.T) v3Provider3Golden {
	t.Helper()
	const wantResourceCount = 31
	forms := providerV3CurrentForms()
	if len(forms) != wantResourceCount {
		t.Fatalf("current Provider 3 Form projection = %d, want %d", len(forms), wantResourceCount)
	}

	ctx := context.Background()
	registry := currentformregistry.V3Current()
	types := v3TerraformResourceTypes()
	codecs := v3Codecs()
	resources := make(map[string]v3Provider3GoldenResource, len(forms))
	seenRefs := make(map[currentformregistry.ExactFormKey]string, len(forms))
	for _, form := range forms {
		if form.Kind == "ObjectBucket" {
			t.Fatal("withdrawn ObjectBucket appeared in the current Provider 3 projection")
		}
		if group := form.Family.APIVersion(); group == "" || containsSlash(group) {
			t.Fatalf("current Form %s/%s has a versioned or empty family group", group, form.Kind)
		}
		line := currentformregistry.GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
		ref, err := registry.DefaultCreate(line)
		if err != nil {
			t.Fatalf("default create ref for %s/%s: %v", line.APIVersion, line.Kind, err)
		}
		if ref.APIVersion != line.APIVersion || ref.Kind != line.Kind {
			t.Fatalf("default create ref %s does not preserve %s/%s", ref.ExactKey(), line.APIVersion, line.Kind)
		}
		if !formpackage.ValidDigest(ref.SchemaDigest) || !formpackage.ValidDigest(ref.PackageDigest) {
			t.Fatalf("current %s/%s has invalid exact digest(s): %#v", line.APIVersion, line.Kind, ref)
		}
		key := ref.ExactKey()
		if prior, duplicate := seenRefs[key]; duplicate {
			t.Fatalf("exact current FormRef %s is mapped by both %s and %s", key, prior, form.Kind)
		}

		resourceType, mapped := types.Lookup(key)
		if !mapped || resourceType == "" {
			t.Fatalf("current exact FormRef %s has no Terraform resource mapping", key)
		}
		if prior, duplicate := resources[resourceType]; duplicate {
			t.Fatalf("Terraform resource type %q is not unique: existing %#v, new %s/%s", resourceType, prior, line.APIVersion, line.Kind)
		}
		seenRefs[key] = resourceType

		codec, codecSupported := codecs.forStateKey(key)
		if !codecSupported {
			t.Fatalf("current exact FormRef %s has no exact codec dispatch entry", key)
		}
		if codec.Ref != ref {
			t.Fatalf("codec ref for %s = %#v, want %#v", key, codec.Ref, ref)
		}
		if codec.Form.Family.APIVersion() != ref.APIVersion || codec.Form.Kind != ref.Kind || codec.Form.DefinitionVersion != ref.DefinitionVersion {
			t.Fatalf("codec declaration for %s does not carry its exact Form identity: %#v", key, codec.Form)
		}
		if codec.DesiredSchema == nil {
			t.Fatalf("codec for %s has no decoded desired schema", key)
		}
		defaultCodec, err := codecs.defaultCreate(line)
		if err != nil {
			t.Fatalf("default codec for %s/%s: %v", line.APIVersion, line.Kind, err)
		}
		if defaultCodec.Ref != ref {
			t.Fatalf("default codec ref for %s = %#v, want %#v", key, defaultCodec.Ref, ref)
		}

		candidate := v3Provider3CurrentResourceHarness(t, form, resourceType, nil, codecs)
		var response frameworkresource.SchemaResponse
		candidate.Schema(ctx, frameworkresource.SchemaRequest{}, &response)
		if response.Diagnostics.HasError() {
			t.Fatalf("schema for %s: %v", resourceType, response.Diagnostics)
		}
		if response.Schema.Version != 1 {
			t.Fatalf("schema version for %s = %d, want Provider 3 version 1", resourceType, response.Schema.Version)
		}
		if response.Schema.Version != v3SchemaVersion {
			t.Fatalf("schema version for %s = %d, want lane version %d", resourceType, response.Schema.Version, v3SchemaVersion)
		}
		attributes := deriveV3Provider3GoldenAttributes(response.Schema.Attributes)
		if len(attributes) == 0 {
			t.Fatalf("schema for %s is empty", resourceType)
		}

		resources[resourceType] = v3Provider3GoldenResource{
			FormRef:       ref,
			SchemaVersion: response.Schema.Version,
			Attributes:    attributes,
			Codec: v3Provider3GoldenCodec{
				StateRef:         codec.Ref,
				DefaultCreateRef: defaultCodec.Ref,
			},
		}
	}

	if len(resources) != wantResourceCount || len(seenRefs) != wantResourceCount {
		t.Fatalf("current Provider 3 mapping is not exhaustive and unique: resources=%d refs=%d", len(resources), len(seenRefs))
	}
	currentRefs := 0
	for _, ref := range registry.SupportedRefs() {
		if containsSlash(ref.APIVersion) {
			continue
		}
		currentRefs++
		if _, present := seenRefs[ref.ExactKey()]; !present {
			t.Fatalf("generated current registry ref %s is absent from the Provider 3 mapping", ref.ExactKey())
		}
	}
	if currentRefs != wantResourceCount {
		t.Fatalf("generated current registry has %d refs, want %d", currentRefs, wantResourceCount)
	}

	return v3Provider3Golden{
		Format:                "takoform.provider3-golden@v1",
		SourceTag:             "v3.0.0",
		SourceCommit:          "a225cfa7c84aa551981cc8ad56c9a281fa6e051a",
		ReleaseEvidenceDigest: "sha256:9802d520cbcdd94c58464c4f14c121f365d90e363b09090caacb9633afcd94d7",
		ResourceCount:         wantResourceCount,
		Resources:             resources,
	}
}

// v3Provider3CurrentResourceHarness obtains the resource through the same
// production registration factory exercised by Terraform. This helper lives
// in a Provider-3 characterization file so the immutable v3.0.0 baseline gate
// copies it together with every consumer. On the current provider that factory
// is the Snapshot/projection seam and therefore carries the exact projected
// artifact rule; no test reconstructs or infers that metadata by Kind.
func v3Provider3CurrentResourceHarness(
	t *testing.T,
	form model.Form,
	resourceType string,
	data *providerData,
	codecs *v3CodecTable,
) *v3FormResource {
	t.Helper()
	for _, factory := range newV3FormResources() {
		candidate, ok := factory().(*v3FormResource)
		if !ok {
			continue
		}
		if candidate.form.Family.APIVersion() != form.Family.APIVersion() ||
			candidate.form.Kind != form.Kind ||
			candidate.form.DefinitionVersion != form.DefinitionVersion {
			continue
		}
		if resourceType != "" && candidate.resourceType != resourceType {
			t.Fatalf("production resource type for %s/%s = %q, want %q", form.Family.APIVersion(), form.Kind, candidate.resourceType, resourceType)
		}
		if data != nil {
			candidate.data = data
		}
		if codecs != nil {
			candidate.codecs = codecs
		}
		return candidate
	}
	t.Fatalf("production registration has no exact current resource for %s/%s@%s", form.Family.APIVersion(), form.Kind, form.DefinitionVersion)
	return nil
}

func deriveV3Provider3GoldenAttributes(attributes map[string]frameworkschema.Attribute) map[string]v3Provider3GoldenAttribute {
	result := make(map[string]v3Provider3GoldenAttribute, len(attributes))
	for name, attribute := range attributes {
		result[name] = deriveV3Provider3GoldenAttribute(attribute)
	}
	return result
}

func deriveV3Provider3GoldenAttribute(attribute frameworkschema.Attribute) v3Provider3GoldenAttribute {
	result := v3Provider3GoldenAttribute{
		Type:      attribute.GetType().String(),
		Required:  attribute.IsRequired(),
		Optional:  attribute.IsOptional(),
		Computed:  attribute.IsComputed(),
		Sensitive: attribute.IsSensitive(),
	}
	switch typed := attribute.(type) {
	case frameworkschema.SingleNestedAttribute:
		result.Nested = deriveV3Provider3GoldenAttributes(typed.Attributes)
	case frameworkschema.ListNestedAttribute:
		result.Nested = deriveV3Provider3GoldenAttributes(typed.NestedObject.Attributes)
	case frameworkschema.SetNestedAttribute:
		result.Nested = deriveV3Provider3GoldenAttributes(typed.NestedObject.Attributes)
	case frameworkschema.MapNestedAttribute:
		result.Nested = deriveV3Provider3GoldenAttributes(typed.NestedObject.Attributes)
	}
	return result
}

func readV3Provider3Golden(t *testing.T) v3Provider3Golden {
	t.Helper()
	raw, err := os.ReadFile(v3Provider3GoldenPath)
	if err != nil {
		t.Fatalf("read %s: %v", v3Provider3GoldenPath, err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", v3Provider3GoldenPath, err)
	}
	if !bytes.Equal(raw, canonical) {
		t.Fatalf("%s is not RFC 8785 canonical JSON", v3Provider3GoldenPath)
	}
	var golden v3Provider3Golden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode %s: %v", v3Provider3GoldenPath, err)
	}
	if golden.Format != "takoform.provider3-golden@v1" {
		t.Fatalf("golden format = %q, want takoform.provider3-golden@v1", golden.Format)
	}
	if golden.SourceTag != "v3.0.0" || golden.SourceCommit != "a225cfa7c84aa551981cc8ad56c9a281fa6e051a" ||
		golden.ReleaseEvidenceDigest != "sha256:9802d520cbcdd94c58464c4f14c121f365d90e363b09090caacb9633afcd94d7" {
		t.Fatalf("golden immutable source fence drifted: tag=%q commit=%q evidence=%q", golden.SourceTag, golden.SourceCommit, golden.ReleaseEvidenceDigest)
	}
	releaseEvidence, err := os.ReadFile("testdata/v3-provider3-release-evidence.json")
	if err != nil {
		t.Fatalf("read Provider 3 release evidence: %v", err)
	}
	if digest := formpackage.DigestBytes(releaseEvidence); digest != golden.ReleaseEvidenceDigest {
		t.Fatalf("Provider 3 release evidence digest = %s, golden pins %s", digest, golden.ReleaseEvidenceDigest)
	}
	if golden.ResourceCount != len(golden.Resources) {
		t.Fatalf("golden resourceCount = %d, resources = %d", golden.ResourceCount, len(golden.Resources))
	}
	return golden
}

func v3Provider3GoldenDiff(t *testing.T, want, got v3Provider3Golden) string {
	t.Helper()
	wantRaw, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		return fmt.Sprintf("marshal want: %v", err)
	}
	gotRaw, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		return fmt.Sprintf("marshal got: %v", err)
	}
	return fmt.Sprintf("want:\n%s\ngot:\n%s", wantRaw, gotRaw)
}

func containsSlash(value string) bool {
	for _, char := range value {
		if char == '/' {
			return true
		}
	}
	return false
}
