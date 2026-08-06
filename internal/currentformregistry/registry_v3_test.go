package currentformregistry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedV3RefsMatchGeneratedFamilyCandidateSet(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "forms", "candidates", "edge", "v1alpha1", "candidate-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Family string `json:"family"`
		Forms  []struct {
			Kind          string `json:"kind"`
			Role          string `json:"role"`
			FormRef       V3Ref  `json:"formRef"`
			PackageDigest string `json:"packageDigest"`
		} `json:"forms"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Family != "edge.forms.takoform.com/v1alpha1" {
		t.Fatalf("family = %q", manifest.Family)
	}
	if len(manifest.Forms) != len(v3Refs) {
		t.Fatalf("candidate manifest has %d Forms, provider has %d", len(manifest.Forms), len(v3Refs))
	}
	for _, entry := range manifest.Forms {
		entry.FormRef.PackageDigest = entry.PackageDigest
		key := entry.FormRef.APIVersion + "/" + entry.Kind
		if got := v3Refs[key]; got != entry.FormRef {
			t.Fatalf("provider family candidate %s drifted: got %#v want %#v", key, got, entry.FormRef)
		}
		byKind, err := V3ForKind(entry.FormRef.APIVersion, entry.Kind)
		if err != nil {
			t.Fatal(err)
		}
		if byKind != entry.FormRef {
			t.Fatalf("V3ForKind(%s) = %#v", key, byKind)
		}
	}
}

func TestV3SupportedFormRefsCoversEveryDefault(t *testing.T) {
	t.Parallel()
	defaults := V3All()
	supported := V3SupportedFormRefs()
	if len(supported) < len(defaults) {
		t.Fatalf("supported family FormRefs = %d, want at least %d defaults", len(supported), len(defaults))
	}
	for key, want := range defaults {
		found := false
		for _, ref := range supported {
			if ref == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("family candidate %s is missing from the supported set", key)
		}
	}
	if _, err := V3ForKind("edge.forms.takoform.com/v1alpha1", "NoSuchKind"); err == nil {
		t.Fatal("unknown family kind unexpectedly resolved")
	}
}
