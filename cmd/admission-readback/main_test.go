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

func TestRunCreatesExactReadbackWithoutOverwriting(t *testing.T) {
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

	output := filepath.Join(t.TempDir(), "provider-readback.json")
	args := []string{
		"registry",
		"--matrix", filepath.Join(root, "admission", "v1", "registry", "provider-lifecycle-matrix.json"),
		"--provider-release-commit", readback.ProviderReleaseCommit,
		"--output", output,
	}
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated readback is not byte-for-byte equal to the retained subject")
	}
	if err := run(args); err == nil {
		t.Fatal("second write unexpectedly overwrote the retained readback")
	}
	after, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, want) {
		t.Fatal("failed overwrite attempt changed the retained readback")
	}
}
