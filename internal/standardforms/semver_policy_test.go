package standardforms

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormSemVerPatchRequiresCanonicalDesiredSchemaEquivalence(t *testing.T) {
	t.Parallel()
	releases := []formSchemaRelease{
		testFormSchemaRelease(t, "Example", "1.0.0", `{
			"type":"object",
			"properties":{"name":{"type":"string"}},
			"required":["name"],
			"additionalProperties":false
		}`),
		testFormSchemaRelease(t, "Example", "1.0.1", `{
			"additionalProperties":false,
			"required":["name"],
			"properties":{"name":{"type":"string"}},
			"type":"object"
		}`),
	}
	if err := verifyFormSemVerSequence(releases); err != nil {
		t.Fatalf("canonically equivalent patch schemas were rejected: %v", err)
	}

	releases[1] = testFormSchemaRelease(t, "Example", "1.0.1", `{
		"type":"object",
		"properties":{"name":{"type":"string"},"description":{"type":"string"}},
		"required":["name"],
		"additionalProperties":false
	}`)
	err := verifyFormSemVerSequence(releases)
	if err == nil || !strings.Contains(err.Error(), "patch release") ||
		!strings.Contains(err.Error(), "canonically equivalent") {
		t.Fatalf("patch schema drift error = %v", err)
	}
}

func TestFormSemVerMinorProvesBackwardsAcceptance(t *testing.T) {
	t.Parallel()
	old := testFormSchemaRelease(t, "Example", "1.0.0", `{
		"type":"object",
		"properties":{
			"name":{"type":"string","minLength":2,"maxLength":32,"pattern":"^[a-z]+$"}
		},
		"required":["name"],
		"additionalProperties":false
	}`)
	compatible := testFormSchemaRelease(t, "Example", "1.1.0", `{
		"type":"object",
		"properties":{
			"name":{"type":"string","minLength":1,"maxLength":64},
			"description":{"type":"string"}
		},
		"required":["name"],
		"additionalProperties":false
	}`)
	if err := verifyFormSemVerSequence([]formSchemaRelease{old, compatible}); err != nil {
		t.Fatalf("backwards-compatible minor schema was rejected: %v", err)
	}
}

func TestFormSemVerMinorFailsClosedForTighteningOrUnprovedChanges(t *testing.T) {
	t.Parallel()
	old := testFormSchemaRelease(t, "Example", "1.0.0", `{
		"type":"object",
		"properties":{"name":{"type":"string"}},
		"required":["name"],
		"additionalProperties":false
	}`)
	tests := map[string]string{
		"new required field": `{
			"type":"object",
			"properties":{"name":{"type":"string"},"region":{"type":"string"}},
			"required":["name","region"],
			"additionalProperties":false
		}`,
		"changed conditional": `{
			"type":"object",
			"properties":{"name":{"type":"string"}},
			"required":["name"],
			"additionalProperties":false,
			"allOf":[{"properties":{"name":{"minLength":2}}}]
		}`,
	}
	for name, schema := range tests {
		name, schema := name, schema
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			next := testFormSchemaRelease(t, "Example", "1.1.0", schema)
			err := verifyFormSemVerSequence([]formSchemaRelease{old, next})
			if err == nil || !strings.Contains(err.Error(), "cannot prove backwards acceptance") ||
				!strings.Contains(err.Error(), "major") {
				t.Fatalf("minor incompatibility error = %v", err)
			}
		})
	}
}

func TestFormSemVerMinorOptionalFieldRequiresClosedOldObject(t *testing.T) {
	t.Parallel()
	old := testFormSchemaRelease(t, "Example", "1.0.0", `{
		"type":"object",
		"properties":{"name":{"type":"string"}}
	}`)
	next := testFormSchemaRelease(t, "Example", "1.1.0", `{
		"type":"object",
		"properties":{"name":{"type":"string"},"description":{"type":"string"}}
	}`)
	err := verifyFormSemVerSequence([]formSchemaRelease{old, next})
	if err == nil || !strings.Contains(err.Error(), "previously allowed arbitrary values") {
		t.Fatalf("open-object optional field error = %v", err)
	}
}

func TestCommittedFormReleaseHistorySatisfiesSemVerPolicy(t *testing.T) {
	t.Parallel()
	if err := VerifyFormSemVerHistory(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}

func testFormSchemaRelease(t *testing.T, kind, version, raw string) formSchemaRelease {
	t.Helper()
	var schema any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseStableFormVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	return formSchemaRelease{
		Kind:          kind,
		Version:       parsed,
		DesiredSchema: schema,
	}
}
