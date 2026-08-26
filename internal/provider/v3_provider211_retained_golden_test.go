package provider

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const v3Provider211RetainedGoldenPath = "testdata/v3-provider211-retained-golden.json"

// v3Provider211RetainedGolden is deliberately independent of the current
// registry. The registry is the implementation under test; this fixture is
// the immutable Provider 2.1.1 release evidence it must continue to match.
type v3Provider211RetainedGolden struct {
	Family             string                          `json:"family"`
	FormMaturity       string                          `json:"formMaturity"`
	Format             string                          `json:"format"`
	Identities         []v3Provider211RetainedIdentity `json:"identities"`
	IdentityCount      int                             `json:"identityCount"`
	PortableAPIVersion string                          `json:"portableApiVersion"`
	ProviderVersion    string                          `json:"providerVersion"`
	ReadableCount      int                             `json:"readableCount"`
	SourceCommit       string                          `json:"sourceCommit"`
	SourceTag          string                          `json:"sourceTag"`
}

type v3Provider211RetainedIdentity struct {
	CanonicalImport   string    `json:"canonicalImport"`
	FormRef           v3FormRef `json:"formRef"`
	Provider3Mapping  string    `json:"provider3Mapping"`
	Provider3Readable bool      `json:"provider3Readable"`
	ResourceType      string    `json:"resourceType"`
}

// TestV3Provider211RetainedGoldenLocksImmutableHistory proves all fifteen
// exact Provider 2.1.1/v1beta1 identities, including the ObjectBucket history
// that remains in the registry but deliberately crosses no Provider 3
// mapping/codec boundary.
func TestV3Provider211RetainedGoldenLocksImmutableHistory(t *testing.T) {
	t.Parallel()
	want := readV3Provider211RetainedGolden(t)
	if want.Format != "takoform.provider211-retained-golden@v1" ||
		want.ProviderVersion != "2.1.1" || want.SourceTag != "v2.1.1" ||
		want.SourceCommit != "9810570d542434efcf177543de9d463bbfda0d09" ||
		want.PortableAPIVersion != "forms.takoform.com/v1beta1" ||
		want.Family != "edge.forms.takoform.com/v1beta1" ||
		want.FormMaturity != "experimental" {
		t.Fatalf("retained Provider 2.1.1 source fence drifted: %#v", want)
	}
	if want.IdentityCount != 15 || len(want.Identities) != want.IdentityCount {
		t.Fatalf("retained identity count = %d/%d, want 15", want.IdentityCount, len(want.Identities))
	}
	if want.ReadableCount != 14 {
		t.Fatalf("retained readable count = %d, want 14", want.ReadableCount)
	}

	registry := mustProviderV3SnapshotAssembly().registry
	types := v3TerraformResourceTypes()
	codecs := v3Codecs()
	seen := make(map[v3ExactFormKey]struct{}, len(want.Identities))
	readable := 0
	for _, expected := range want.Identities {
		ref := expected.FormRef
		if ref.APIVersion != want.Family || ref.Kind == "" || ref.DefinitionVersion != "0.1.0" ||
			!formpackage.ValidDigest(ref.SchemaDigest) || !formpackage.ValidDigest(ref.PackageDigest) {
			t.Fatalf("invalid retained exact FormRef: %#v", ref)
		}
		if _, duplicate := seen[ref.ExactKey()]; duplicate {
			t.Fatalf("duplicate retained exact FormRef %s", ref.ExactKey())
		}
		seen[ref.ExactKey()] = struct{}{}

		got, supported := registry.Lookup(ref.ExactKey())
		if !supported || got != ref {
			t.Fatalf("current registry changed retained %s: got %#v supported=%t", ref.ExactKey(), got, supported)
		}

		mappedType, mapped := types.Lookup(ref.ExactKey())
		_, codec := codecs.forStateKey(ref.ExactKey())
		if expected.Provider3Readable {
			readable++
			if expected.CanonicalImport != "supported" || expected.Provider3Mapping == "" ||
				expected.Provider3Mapping != mappedType || !mapped || !codec {
				t.Fatalf("retained %s Provider 3 disposition = mapping %q/%t codec %t import %q; fixture=%q", ref.Kind, mappedType, mapped, codec, expected.CanonicalImport, expected.Provider3Mapping)
			}
		} else {
			if ref.Kind != "ObjectBucket" || expected.CanonicalImport != "structurally-impossible" ||
				expected.Provider3Mapping != "" || mapped || codec {
				t.Fatalf("retained %s unexpectedly crossed Provider 3 boundary: fixture=%#v mapped=%t codec=%t", ref.Kind, expected, mapped, codec)
			}
		}
	}
	if readable != want.ReadableCount {
		t.Fatalf("fixture marks %d readable identities, want %d", readable, want.ReadableCount)
	}
}

func readV3Provider211RetainedGolden(t *testing.T) v3Provider211RetainedGolden {
	t.Helper()
	raw, err := os.ReadFile(v3Provider211RetainedGoldenPath)
	if err != nil {
		t.Fatalf("read %s: %v", v3Provider211RetainedGoldenPath, err)
	}
	document := bytes.TrimSuffix(raw, []byte("\n"))
	canonical, err := formpackage.Canonicalize(document)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", v3Provider211RetainedGoldenPath, err)
	}
	if !bytes.Equal(document, canonical) || len(raw)-len(document) > 1 {
		t.Fatalf("%s is not RFC 8785 canonical JSON with at most one final newline", v3Provider211RetainedGoldenPath)
	}
	var golden v3Provider211RetainedGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode %s: %v", v3Provider211RetainedGoldenPath, err)
	}
	return golden
}
