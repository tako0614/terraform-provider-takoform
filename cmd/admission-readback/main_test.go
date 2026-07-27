package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/providerlifecycle"
)

// TestRetainedRegistryReadbackBelongsToTheRetiredGeneration proves the
// retained Registry evidence stays readable while this build refuses to
// reissue it.
//
// That evidence was produced by a provider that implemented the published
// Forms. This build implements the rebuilt portable set, so regenerating the
// readback must fail closed rather than restamp an old proof with a new
// provider's identity.
func TestRetainedRegistryReadbackBelongsToTheRetiredGeneration(t *testing.T) {
	root, err := providerlifecycle.RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "admission", "v1", "registry", "provider-readback.json")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	var readback admissionrelease.ProviderRegistryReadback
	if err := json.Unmarshal(want, &readback); err != nil {
		t.Fatal(err)
	}
	if readback.ProviderReleaseCommit == "" {
		t.Fatal("retained readback lost its provider release commit")
	}

	output := filepath.Join(t.TempDir(), "provider-readback.json")
	args := []string{
		"registry",
		"--matrix", filepath.Join(root, "admission", "v1", "registry", "provider-lifecycle-matrix.json"),
		"--provider-release-commit", readback.ProviderReleaseCommit,
		"--output", output,
	}
	if err := run(args); err == nil {
		t.Fatal("a provider that no longer implements the published Forms reissued their Registry readback")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("failed readback left an artifact behind: %v", err)
	}
	after, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, want) {
		t.Fatal("failed readback attempt changed the retained evidence")
	}
}
