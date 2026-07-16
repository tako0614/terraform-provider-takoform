//go:build unix

package formpackage

import (
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
