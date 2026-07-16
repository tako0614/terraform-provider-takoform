package characterization

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompatibilityCandidateEvidenceVerifies(t *testing.T) {
	t.Parallel()
	root := testEvidenceRoot()
	report, err := Verify(root)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.KindCount != len(ExpectedKinds) || report.FileCount != len(expectedEvidenceFiles()) {
		t.Fatalf("verification report = %#v", report)
	}

	checkedIn, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	rendered, err := RenderManifest(root)
	if err != nil {
		t.Fatalf("RenderManifest: %v", err)
	}
	if !reflect.DeepEqual(rendered, checkedIn) {
		t.Fatal("checked-in manifest does not equal deterministic render")
	}
}

func TestCompatibilityCandidateEvidenceRejectsFixtureDrift(t *testing.T) {
	t.Parallel()
	source := testEvidenceRoot()
	target := filepath.Join(t.TempDir(), "candidate")
	copyTree(t, source, target)

	path := filepath.Join(target, "fixtures", "desired.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read copied fixture: %v", err)
	}
	mutated := bytes.Replace(raw, []byte(`"name": "edge"`), []byte(`"name": "edge-drift"`), 1)
	if bytes.Equal(mutated, raw) {
		t.Fatal("test mutation did not change fixture")
	}
	if err := os.WriteFile(path, mutated, 0o644); err != nil {
		t.Fatalf("write mutated fixture: %v", err)
	}
	if _, err := Verify(target); err == nil {
		t.Fatal("Verify accepted fixture bytes that differ from the manifest")
	}
}

func testEvidenceRoot() string {
	return filepath.Join("..", "..", "conformance", "compatibility-candidate-v1")
}

func copyTree(t *testing.T, source, target string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, raw, 0o644)
	})
	if err != nil {
		t.Fatalf("copy evidence tree: %v", err)
	}
}
