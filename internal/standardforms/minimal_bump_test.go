package standardforms

import "testing"

func releaseWith(kind, version string, schema any) formSchemaRelease {
	parsed, err := parseStableFormVersion(version)
	if err != nil {
		panic(err)
	}
	return formSchemaRelease{Kind: kind, Version: parsed, DesiredSchema: schema}
}

func objectSchema(properties map[string]any, required []any) any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             required,
	}
}

func TestCandidateMajorMustBeEarned(t *testing.T) {
	base := objectSchema(map[string]any{
		"name": map[string]any{"type": "string"},
	}, []any{"name"})
	history := []formSchemaRelease{releaseWith("Example", "2.0.0", base)}

	// Adding an optional field keeps every earlier document valid. Taking a
	// major line for it spends a migration every consumer has to perform.
	additive := objectSchema(map[string]any{
		"name":      map[string]any{"type": "string"},
		"schemaUrl": map[string]any{"type": "string"},
	}, []any{"name"})
	if err := verifyCandidateBumpIsMinimal(
		history, releaseWith("Example", "3.0.0", additive),
	); err == nil {
		t.Fatal("an additive change must not be allowed to open a major line")
	}
	if err := verifyCandidateBumpIsMinimal(
		history, releaseWith("Example", "2.1.0", additive),
	); err != nil {
		t.Fatalf("an additive change must be allowed as a minor: %v", err)
	}

	// Narrowing genuinely breaks earlier documents, so a major is earned.
	narrowed := objectSchema(map[string]any{
		"name": map[string]any{"type": "string", "pattern": "^[a-z]+$"},
	}, []any{"name"})
	if err := verifyCandidateBumpIsMinimal(
		history, releaseWith("Example", "3.0.0", narrowed),
	); err != nil {
		t.Fatalf("a narrowing change must be allowed as a major: %v", err)
	}

	// Published history that already over-bumped is not re-judged: those
	// versions are immutable and blocking on them would punish everyone.
	messy := []formSchemaRelease{
		releaseWith("Example", "1.0.1", base),
		releaseWith("Example", "2.0.0", base),
	}
	if err := verifyCandidateBumpIsMinimal(
		messy, releaseWith("Example", "2.1.0", additive),
	); err != nil {
		t.Fatalf("history over-bumps must not block a correct candidate: %v", err)
	}
}
