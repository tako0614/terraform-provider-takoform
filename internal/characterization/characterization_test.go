package characterization

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestCompatibilityCandidateSchemasValidateFixtures(t *testing.T) {
	t.Parallel()
	if err := validateEvidenceSchemas(testEvidenceRoot()); err != nil {
		t.Fatalf("validate Draft 2020-12 fixture set: %v", err)
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

func TestCompatibilityCandidateSchemasRejectInvalidFixtures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		path        string
		old         string
		replacement string
	}{
		{
			name:        "desired api version",
			path:        filepath.Join("fixtures", "desired.json"),
			old:         `"apiVersion": "forms.takoform.com/v1alpha1"`,
			replacement: `"apiVersion": "invalid.example/v9"`,
		},
		{
			name:        "observed status composition",
			path:        filepath.Join("fixtures", "observed.json"),
			old:         `"phase": "Ready"`,
			replacement: `"phaseInvalid": "Ready"`,
		},
		{
			name:        "host discovery capability",
			path:        filepath.Join("fixtures", "discovery.json"),
			old:         `"service_forms": true`,
			replacement: `"service_forms": false`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			target := filepath.Join(t.TempDir(), "candidate")
			copyTree(t, testEvidenceRoot(), target)
			mutateFile(t, filepath.Join(target, test.path), test.old, test.replacement)
			if err := validateEvidenceSchemas(target); err == nil {
				t.Fatal("Draft 2020-12 validation accepted invalid fixture")
			}
		})
	}
}

func TestCompatibilityCandidateSchemasRejectOpenReferences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		reference   string
		wantMessage string
	}{
		{name: "path escape", reference: "../outside.schema.json", wantMessage: "path-escaping"},
		{name: "network", reference: "https://schemas.example.test/host.json", wantMessage: "network"},
		{name: "unregistered local", reference: "missing.schema.json", wantMessage: "outside the closed candidate evidence set"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			target := filepath.Join(t.TempDir(), "candidate")
			copyTree(t, testEvidenceRoot(), target)
			mutateFile(
				t,
				filepath.Join(target, "schemas", "discovery.schema.json"),
				`"$ref": "host-discovery.schema.json"`,
				`"$ref": "`+test.reference+`"`,
			)
			err := validateEvidenceSchemas(target)
			if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("schema reference error = %v, want message containing %q", err, test.wantMessage)
			}
		})
	}
}

func TestCompatibilityCandidateEvidenceRejectsVendoredSchemaTamper(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "candidate")
	copyTree(t, testEvidenceRoot(), target)
	mutateFile(
		t,
		filepath.Join(target, "schemas", "host-discovery.schema.json"),
		`"minItems": 1`,
		`"minItems": 2`,
	)
	if _, err := Verify(target); err == nil {
		t.Fatal("Verify accepted tampered vendored transitive schema")
	}
}

func TestCompatibilityCandidateRejectsMissingProviderSemantics(t *testing.T) {
	t.Parallel()
	document, err := LoadCases[ProviderSchemaCase](testEvidenceRoot(), "providerSchema")
	if err != nil {
		t.Fatalf("load provider schema cases: %v", err)
	}
	for caseIndex := range document.Cases {
		for attributeIndex := range document.Cases[caseIndex].Attributes {
			attribute := &document.Cases[caseIndex].Attributes[attributeIndex]
			if attribute.Validators > 0 {
				attribute.ValidatorSemantics = nil
				if err := validateSchemaCases(document.Cases); err == nil {
					t.Fatal("provider schema accepted a validator count without semantic fingerprints")
				}
				return
			}
		}
	}
	t.Fatal("fixture has no validator semantics to mutate")
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

func mutateFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	mutated := bytes.Replace(raw, []byte(old), []byte(replacement), 1)
	if bytes.Equal(mutated, raw) {
		t.Fatalf("mutation %q was not found in %s", old, path)
	}
	if err := os.WriteFile(path, mutated, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
