package formregistry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseRefsMatchStandardPackageSet(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "forms", "standard-package-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		Packages []struct {
			Kind    string `json:"kind"`
			FormRef struct {
				APIVersion        string `json:"apiVersion"`
				Kind              string `json:"kind"`
				DefinitionVersion string `json:"definitionVersion"`
				SchemaDigest      string `json:"schemaDigest"`
			} `json:"formRef"`
			PackageDigest string `json:"packageDigest"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(raw, &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Packages) != len(releaseRefs) {
		t.Fatalf("inventory has %d packages; provider compiled %d refs", len(inventory.Packages), len(releaseRefs))
	}
	for _, pkg := range inventory.Packages {
		got, err := ForKind(pkg.Kind)
		if err != nil {
			t.Fatal(err)
		}
		want := Ref{
			APIVersion:        pkg.FormRef.APIVersion,
			Kind:              pkg.FormRef.Kind,
			DefinitionVersion: pkg.FormRef.DefinitionVersion,
			SchemaDigest:      pkg.FormRef.SchemaDigest,
			PackageDigest:     pkg.PackageDigest,
		}
		if got != want {
			t.Fatalf("release FormRef for %s drifted:\n got %#v\nwant %#v", pkg.Kind, got, want)
		}
	}
}
