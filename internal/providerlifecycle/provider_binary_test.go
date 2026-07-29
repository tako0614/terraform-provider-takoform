package providerlifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyProviderBinaryCopiesExactExecutableBytes(t *testing.T) {
	source := filepath.Join(t.TempDir(), "terraform-provider-takoform_v1.0.0")
	want := []byte("\x7fELF-exact-final-provider")
	if err := os.WriteFile(source, want, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "bin", "terraform-provider-takoform")

	if err := copyProviderBinary(source, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("copied provider bytes = %q, want %q", got, want)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o755 {
		t.Fatalf("copied provider mode = %v, want regular 0755", info.Mode())
	}
}

func TestCopyProviderBinaryRejectsNonAbsoluteNonRegularOrNonExecutableSource(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "terraform-provider-takoform")
	if err := copyProviderBinary("relative-provider", destination); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative provider error = %v, want absolute-path rejection", err)
	}

	directory := t.TempDir()
	if err := copyProviderBinary(directory, destination); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("directory provider error = %v, want regular-file rejection", err)
	}

	nonExecutable := filepath.Join(t.TempDir(), "provider")
	if err := os.WriteFile(nonExecutable, []byte("provider"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyProviderBinary(nonExecutable, destination); err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("non-executable provider error = %v, want executable rejection", err)
	}

	executable := filepath.Join(t.TempDir(), "provider")
	if err := os.WriteFile(executable, []byte("provider"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(t.TempDir(), "provider-link")
	if err := os.Symlink(executable, symlink); err != nil {
		t.Fatal(err)
	}
	if err := copyProviderBinary(symlink, destination); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("symlink provider error = %v, want regular-file rejection", err)
	}
}

func TestVerifyProviderBinaryUnchangedRejectsByteMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terraform-provider-takoform")
	if err := os.WriteFile(path, []byte("exact extracted provider bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	expectedSHA256, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyProviderBinaryUnchanged(path, expectedSHA256); err != nil {
		t.Fatalf("unchanged provider binary rejected: %v", err)
	}

	if err := os.WriteFile(path, []byte("mutated provider bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyProviderBinaryUnchanged(path, expectedSHA256); err == nil || !strings.Contains(err.Error(), "bytes changed during lifecycle") {
		t.Fatalf("mutated provider binary error = %v, want byte-mutation rejection", err)
	}
}
