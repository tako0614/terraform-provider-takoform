package currentformregistry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedRefsMatchGeneratedCandidateSet(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "forms", "candidates", "v1alpha2", "candidate-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Forms []struct {
			Kind          string `json:"kind"`
			FormRef       Ref    `json:"formRef"`
			PackageDigest string `json:"packageDigest"`
		} `json:"forms"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Forms) != len(refs) {
		t.Fatalf("candidate manifest has %d Forms, provider has %d", len(manifest.Forms), len(refs))
	}
	for _, entry := range manifest.Forms {
		entry.FormRef.PackageDigest = entry.PackageDigest
		if got := refs[entry.Kind]; got != entry.FormRef {
			t.Fatalf("provider candidate %s drifted: got %#v want %#v", entry.Kind, got, entry.FormRef)
		}
	}
}

// TestSupportedFormRefsCoversEveryDefault ensures the Read/Observe/Update/
// Delete surface always includes the default create target for each kind.
// When a Form line advances, SupportedFormRefs must keep the older FormRef
// alongside the new default so existing state stays readable.
func TestSupportedFormRefsCoversEveryDefault(t *testing.T) {
	t.Parallel()
	defaults := All()
	supported := SupportedFormRefs()
	if len(supported) < len(defaults) {
		t.Fatalf("supported FormRefs = %d, want at least %d defaults", len(supported), len(defaults))
	}
	for kind, want := range defaults {
		found := false
		for _, ref := range supported {
			if ref.APIVersion == want.APIVersion && ref.Kind == want.Kind &&
				ref.DefinitionVersion == want.DefinitionVersion && ref.SchemaDigest == want.SchemaDigest {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("kind %s default FormRef %s/%s@%s is missing from the supported set",
				kind, want.APIVersion, want.Kind, want.DefinitionVersion)
		}
	}
}

// TestSupportedFormRefsAreDistinctAndExact guards against duplicate or
// incomplete entries entering the generated set.
func TestSupportedFormRefsAreDistinctAndExact(t *testing.T) {
	t.Parallel()
	supported := SupportedFormRefs()
	seen := map[string]bool{}
	for _, ref := range supported {
		identity := ref.APIVersion + "/" + ref.Kind + "@" + ref.DefinitionVersion + " " + ref.SchemaDigest
		if seen[identity] {
			t.Errorf("duplicate supported FormRef: %s", identity)
		}
		seen[identity] = true
		if ref.APIVersion != APIVersion || ref.Kind == "" || ref.SchemaDigest == "" || ref.PackageDigest == "" {
			t.Errorf("incomplete supported FormRef: %#v", ref)
		}
	}
}
