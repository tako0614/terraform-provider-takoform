package migrationproof

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderMigrationStructuralProof(t *testing.T) {
	report, err := Verify(filepath.Join("..", "..", "conformance", "provider-migration-v1"))
	if err != nil {
		t.Fatal(err)
	}
	if report.StructuralStatus != "complete" || report.MigratedResourceCount != 6 || report.ResourceCount != 10 ||
		len(report.Phases) != 5 || len(report.ExternalBlockers) != 2 {
		t.Fatalf("unexpected report %#v", report)
	}
}

func TestReleaseModeRejectsExternalRequiredPhases(t *testing.T) {
	report, err := Verify(filepath.Join("..", "..", "conformance", "provider-migration-v1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireComplete(report); err == nil || !strings.Contains(err.Error(), "external blocker") {
		t.Fatalf("RequireComplete error = %v, want external blocker rejection", err)
	}
}

func TestReleaseModeAcceptsCompleteMachineEvidence(t *testing.T) {
	report := Report{Phases: []Phase{
		{Name: "state-backup", Status: "complete"},
		{Name: "old-refresh-no-op", Status: "complete"},
		{Name: "approved-remove-import", Status: "complete"},
		{Name: "new-refresh-no-op", Status: "complete"},
		{Name: "old-artifact-lock-rollback", Status: "complete"},
	}}
	if err := RequireComplete(report); err != nil {
		t.Fatalf("complete machine evidence rejected: %v", err)
	}
}

func TestEvidenceValidatorAllowsExternalPhasesToBecomeComplete(t *testing.T) {
	evidence := Evidence{Format: "takoform.provider-migration-evidence@v1", Phases: []Phase{
		{Name: "state-backup", Status: "complete", Evidence: "digest"},
		{Name: "old-refresh-no-op", Status: "complete", Evidence: "runner evidence"},
		{Name: "approved-remove-import", Status: "complete", Evidence: "mapping evidence"},
		{Name: "new-refresh-no-op", Status: "complete", Evidence: "runner evidence"},
		{Name: "old-artifact-lock-rollback", Status: "complete", Evidence: "runner evidence"},
	}}
	if err := validatePhases(evidence); err != nil {
		t.Fatalf("completed evidence rejected: %v", err)
	}
}

func TestMigrationContinuityRejectsDesiredAttributeDrift(t *testing.T) {
	legacy := map[string]any{"name": "jobs", "space": "prod", "max_retries": float64(5), "target": "ignored"}
	current := map[string]any{"name": "jobs", "space": "prod", "max_retries": float64(6)}
	err := compareOverlappingDesiredAttributes("Queue", legacy, current)
	if err == nil || !strings.Contains(err.Error(), "max_retries") {
		t.Fatalf("error = %v, want overlapping desired attribute rejection", err)
	}
}

func TestMigrationContinuityRejectsMissingDesiredOverlap(t *testing.T) {
	err := compareOverlappingDesiredAttributes(
		"ObjectBucket",
		map[string]any{"target": "old-target", "locked": true},
		map[string]any{"storage_class": "standard"},
	)
	if err == nil || !strings.Contains(err.Error(), "no overlapping desired attributes") {
		t.Fatalf("error = %v, want missing desired overlap rejection", err)
	}
}

func TestMigrationContinuityRejectsLineageDrift(t *testing.T) {
	err := validateStateLineage(
		tfState{Version: 4, Lineage: "old-lineage"},
		tfState{Version: 4, Lineage: "new-lineage"},
	)
	if err == nil || !strings.Contains(err.Error(), "lineage") {
		t.Fatalf("error = %v, want lineage rejection", err)
	}
}

func TestMigrationContinuityRejectsSchemaVersionDrift(t *testing.T) {
	err := validateSchemaVersion("Queue", tfStateInstance{SchemaVersion: 0}, tfStateInstance{SchemaVersion: 1})
	if err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("error = %v, want schema_version rejection", err)
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
