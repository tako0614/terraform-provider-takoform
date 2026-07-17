package migrationproof

import (
	"path/filepath"
	"testing"
)

func TestProviderMigrationStructuralProof(t *testing.T) {
	report, err := Verify(filepath.Join("..", "..", "conformance", "provider-migration-v1"))
	if err != nil {
		t.Fatal(err)
	}
	if report.StructuralStatus != "complete" || report.MigratedResourceCount != 6 || report.ResourceCount != 10 ||
		len(report.Phases) != 5 || len(report.ExternalBlockers) != 3 {
		t.Fatalf("unexpected report %#v", report)
	}
}

func TestPrivateStateRejectionRecursesIntoConnectionLists(t *testing.T) {
	state := map[string]any{
		"connections": []any{map[string]any{"projection": map[string]any{"credential_token": "redacted"}}},
	}
	if err := rejectPrivateState(state); err == nil {
		t.Fatal("expected nested credential state to be rejected")
	}
}
