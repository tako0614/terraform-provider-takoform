//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package formpackage

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestVerifyDirectoryRejectsDeviceLikeEntries(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, nil)
	if err := syscall.Mkfifo(filepath.Join(root, "named-pipe.txt"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := VerifyDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("VerifyDirectory error = %v, want non-regular-file failure", err)
	}
}

func TestSecureOpenRelativeRejectsFinalSymlinkSwap(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, nil)
	rootHandle, _, err := openStablePackageRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer rootHandle.Close()
	filePath := filepath.Join(root, "definition.json")
	if err := os.Rename(filePath, filePath+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("definition.json.original", filePath); err != nil {
		t.Fatal(err)
	}
	handle, err := secureOpenRelative(rootHandle, root, "definition.json")
	if handle != nil {
		handle.Close()
	}
	if err == nil {
		t.Fatal("secureOpenRelative followed a final symlink")
	}
}

func TestSecureOpenRelativeRejectsSymlinkedDirectoryComponent(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, nil)
	rootHandle, _, err := openStablePackageRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer rootHandle.Close()
	fixtures := filepath.Join(root, "fixtures")
	if err := os.Rename(fixtures, fixtures+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("fixtures.original", fixtures); err != nil {
		t.Fatal(err)
	}
	handle, err := secureOpenRelative(rootHandle, root, "fixtures/desired.json")
	if handle != nil {
		handle.Close()
	}
	if err == nil {
		t.Fatal("secureOpenRelative followed a symlinked directory component")
	}
}

func TestReadBoundedRegularFileRejectsReplacedDirectoryComponent(t *testing.T) {
	t.Parallel()
	root := makeValidPackage(t, nil)
	files, err := inventoryDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	rootHandle, _, err := openStablePackageRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer rootHandle.Close()
	fixtures := filepath.Join(root, "fixtures")
	if err := os.Rename(fixtures, fixtures+".original"); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(fixtures, "desired.json"), []byte(`{"name":"replacement"}`), 0o644)
	_, err = readBoundedRegularFile(rootHandle, root, "fixtures/desired.json", maxPayloadBytes, files["fixtures/desired.json"])
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("readBoundedRegularFile error = %v, want component replacement identity fence", err)
	}
}
