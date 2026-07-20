package formregistry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCandidateRefsMatchStandardPackageSet(t *testing.T) {
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
	if len(inventory.Packages) != len(candidateRefs) {
		t.Fatalf("inventory has %d packages; provider compiled %d refs", len(inventory.Packages), len(candidateRefs))
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
			t.Fatalf("candidate FormRef for %s drifted:\n got %#v\nwant %#v", pkg.Kind, got, want)
		}
	}
}

func TestSuccessorRefsMatchVersionedInventoryWithoutReplacingHistoricalDefault(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "forms", "sql-database-v2-package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		Kind              string `json:"kind"`
		DefinitionVersion string `json:"definitionVersion"`
		FormRef           struct {
			APIVersion        string `json:"apiVersion"`
			Kind              string `json:"kind"`
			DefinitionVersion string `json:"definitionVersion"`
			SchemaDigest      string `json:"schemaDigest"`
		} `json:"formRef"`
		PackageDigest string `json:"packageDigest"`
	}
	if err := json.Unmarshal(raw, &inventory); err != nil {
		t.Fatal(err)
	}
	got, err := ForKindVersion(inventory.Kind, inventory.DefinitionVersion)
	if err != nil {
		t.Fatal(err)
	}
	want := Ref{
		APIVersion: inventory.FormRef.APIVersion, Kind: inventory.FormRef.Kind,
		DefinitionVersion: inventory.FormRef.DefinitionVersion, SchemaDigest: inventory.FormRef.SchemaDigest,
		PackageDigest: inventory.PackageDigest,
	}
	if got != want {
		t.Fatalf("successor FormRef drifted:\n got %#v\nwant %#v", got, want)
	}
	historical, err := ForKind(inventory.Kind)
	if err != nil {
		t.Fatal(err)
	}
	if historical.DefinitionVersion != "1.0.1" {
		t.Fatalf("historical default was replaced: %#v", historical)
	}
}
