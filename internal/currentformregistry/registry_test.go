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
