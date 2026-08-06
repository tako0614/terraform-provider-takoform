package formpackage

// fixture_stage_test.go covers the optional-stage guards of package
// verification. The v1alpha3 family lane made observedSchema optional (the
// envelope owns status) and outputSchema has always been optional, so a
// definition can declare a fixture for a stage whose schema does not exist.
// Verification must answer with a clear verification error; before these
// guards the observed stage dereferenced a nil compiled schema and
// VerifyDirectory panicked on a well-formed, schema-valid package.

import (
	"path/filepath"
	"strings"
	"testing"
)

// makeStagedFamilyPackage builds a v1alpha4 family package that declares the
// named stage fixture, optionally with the matching schema.
func makeStagedFamilyPackage(t *testing.T, stage string, declareSchema bool, negative bool) string {
	t.Helper()
	root := t.TempDir()

	stageSchema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"title":                "Example " + stage + " document",
		"required":             []any{"state"},
		"properties": map[string]any{
			"state": map[string]any{"type": "string"},
		},
	}
	stageDocument := []byte(`{"state":"ready"}`)
	if negative {
		// Invalid against the stage schema, which is exactly what a negative
		// fixture asserts.
		stageDocument = []byte(`{"state":7}`)
	}
	stagePath := "fixtures/" + stage + ".json"

	definition := makeFamilyDefinition()
	if declareSchema {
		switch stage {
		case "observed":
			definition["observedSchema"] = stageSchema
		case "output":
			definition["outputSchema"] = stageSchema
		}
	}
	if negative {
		definition["negativeConformanceFixtures"] = []any{
			map[string]any{
				"name":            "reject-" + stage,
				"stage":           stage,
				"inputPath":       stagePath,
				"expectedFailure": "schema_validation_failed",
			},
		}
	} else {
		fixture := map[string]any{"name": "canonical", "desiredPath": "fixtures/desired.json"}
		fixture[stage+"Path"] = stagePath
		definition["conformanceFixtures"] = []any{fixture}
		delete(definition, "negativeConformanceFixtures")
	}

	definitionRaw := canonicalMarshal(t, definition)
	desiredRaw := []byte(`{"mainModule":"worker.mjs","vars":{"LOG_LEVEL":"info","nested":{"depth":2}}}`)
	writeFixtureFile(t, filepath.Join(root, "definition.json"), definitionRaw, 0o644)
	writeFixtureFile(t, filepath.Join(root, "fixtures", "desired.json"), desiredRaw, 0o644)
	writeFixtureFile(t, filepath.Join(root, filepath.FromSlash(stagePath)), stageDocument, 0o644)

	index := map[string]any{
		"apiVersion": FamilyPackageAPIVersion,
		"kind":       PackageKind,
		"formRef": map[string]any{
			"apiVersion":        definition["apiVersion"],
			"kind":              definition["kind"],
			"definitionVersion": definition["definitionVersion"],
			"schemaDigest":      mustDigestCanonical(t, definitionRaw),
		},
		"definitionPath": "definition.json",
		"files": []any{
			fileEntry("definition.json", DefinitionMediaType, definitionRaw),
			fileEntry("fixtures/desired.json", "application/json", desiredRaw),
			fileEntry(stagePath, "application/json", stageDocument),
		},
	}
	writeFixtureFile(t, filepath.Join(root, PackageIndexFilename), canonicalMarshal(t, index), 0o644)
	return root
}

func TestVerifyDirectoryRejectsFixturesForUndeclaredOptionalStages(t *testing.T) {
	t.Parallel()
	for name, testCase := range map[string]struct {
		stage    string
		negative bool
		want     string
	}{
		"positive fixture observedPath without observedSchema": {
			stage: "observed", want: "declares observedPath but Form Definition has no observedSchema",
		},
		"positive fixture outputPath without outputSchema": {
			stage: "output", want: "declares outputPath but Form Definition has no outputSchema",
		},
		"negative fixture observed stage without observedSchema": {
			stage: "observed", negative: true, want: `stage "observed" has no schema`,
		},
		"negative fixture output stage without outputSchema": {
			stage: "output", negative: true, want: `stage "output" has no schema`,
		},
	} {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := makeStagedFamilyPackage(t, testCase.stage, false, testCase.negative)
			_, err := VerifyDirectory(root)
			if err == nil {
				t.Fatalf("package declaring a %s fixture with no %sSchema verified", testCase.stage, testCase.stage)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("verification error = %v, want it to name the missing stage schema (%q)", err, testCase.want)
			}
		})
	}
}

// TestVerifyDirectoryAcceptsFixturesForDeclaredOptionalStages is the positive
// control: with the stage schema present the same packages verify, so the
// guards above reject the missing schema and nothing else.
func TestVerifyDirectoryAcceptsFixturesForDeclaredOptionalStages(t *testing.T) {
	t.Parallel()
	for name, testCase := range map[string]struct {
		stage    string
		negative bool
	}{
		"positive observed fixture": {stage: "observed"},
		"positive output fixture":   {stage: "output"},
		"negative observed fixture": {stage: "observed", negative: true},
		"negative output fixture":   {stage: "output", negative: true},
	} {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := makeStagedFamilyPackage(t, testCase.stage, true, testCase.negative)
			report, err := VerifyDirectory(root)
			if err != nil {
				t.Fatalf("package declaring both the %s fixture and its schema failed: %v", testCase.stage, err)
			}
			if report.FileCount != 3 {
				t.Fatalf("unexpected report: %+v", report)
			}
		})
	}
}

// TestVerifyDirectoryStillRequiresTheDesiredStage keeps the guards from
// weakening the one stage every Form epoch requires.
func TestVerifyDirectoryStillRequiresTheDesiredStage(t *testing.T) {
	t.Parallel()
	root := makeFamilyPackage(t, func(definition map[string]any) {
		delete(definition, "desiredSchema")
	})
	if _, err := VerifyDirectory(root); err == nil {
		t.Fatal("a Form Definition without desiredSchema verified")
	}
}
