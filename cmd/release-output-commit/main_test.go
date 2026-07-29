package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProbeAndCommitUseCreateOnlyDirectoryRename(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux renameat2 capability is intentionally required")
	}
	parent := t.TempDir()
	var output bytes.Buffer
	if err := run([]string{"probe", "--parent", parent}, &output); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if output.String() != `{"format":"takoform.release-output-commit@v1","status":"verified"}`+"\n" {
		t.Fatalf("probe output = %q", output.String())
	}

	source := filepath.Join(parent, "source")
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "marker"), []byte("verified\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run(
		[]string{"commit", "--source", source, "--target", target},
		&output,
	); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(target, "marker")); err != nil || string(raw) != "verified\n" {
		t.Fatalf("committed marker = %q, %v", raw, err)
	}
}

func TestCommitDoesNotOverwriteDestinationThatAppearsAtCommit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux renameat2 capability is intentionally required")
	}
	parent := t.TempDir()
	source := filepath.Join(parent, "source")
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "marker"), []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	realRename := renameNoReplaceOperation
	t.Cleanup(func() { renameNoReplaceOperation = realRename })
	var racerInfo os.FileInfo
	renameNoReplaceOperation = func(observedSource, observedTarget string) error {
		if observedSource != source || observedTarget != target {
			t.Fatalf("rename operands = %q -> %q", observedSource, observedTarget)
		}
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "marker"), []byte("racer\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		info, infoErr := os.Lstat(target)
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		racerInfo = info
		return realRename(observedSource, observedTarget)
	}
	var output bytes.Buffer
	err = run([]string{"commit", "--source", source, "--target", target}, &output)
	if err == nil || !strings.Contains(err.Error(), "atomic create-only output commit") {
		t.Fatalf("collision error = %v", err)
	}
	for path, want := range map[string]string{
		filepath.Join(source, "marker"): "source\n",
		filepath.Join(target, "marker"): "racer\n",
	} {
		raw, readErr := os.ReadFile(path)
		if readErr != nil || string(raw) != want {
			t.Fatalf("%s = %q, %v; want %q", path, raw, readErr, want)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("failed commit emitted success output: %q", output.String())
	}
	after, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if racerInfo == nil || !os.SameFile(racerInfo, after) {
		t.Fatal("destination inode changed across failed atomic commit")
	}
	afterSource, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(sourceInfo, afterSource) {
		t.Fatal("source inode changed across failed atomic commit")
	}
}
