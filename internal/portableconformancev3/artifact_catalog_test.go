package portableconformancev3

import (
	"path/filepath"
	"testing"
)

func TestPublisherArtifactCatalogLoadsExactCurrentEdgeSet(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	contract, err := Verify(filepath.Join(repoRoot, "conformance", "takoform-v1", "family-host", "edge", "portable-host"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadArtifactFamilyCatalog(repoRoot, contract, "internal/provider/artifacts/publisher")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Forms) != 17 {
		t.Fatalf("publisher catalog Forms = %d, want 17", len(catalog.Forms))
	}
	if catalog.line("edge.forms.takoform.com", "WorkerVersion", "0.3.0") == nil {
		t.Fatal("publisher catalog omitted WorkerVersion@0.3.0")
	}
	if catalog.line("edge.forms.takoform.com", "WorkerVersion", "0.2.0") != nil {
		t.Fatal("publisher catalog retained withdrawn WorkerVersion@0.2.0")
	}
	if catalog.line("edge.forms.takoform.com", "ObjectBucket", "0.1.0") == nil {
		t.Fatal("publisher catalog omitted ObjectBucket@0.1.0")
	}
}
