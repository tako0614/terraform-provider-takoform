package admissioncheckpoint

import (
	"path/filepath"
	"testing"
)

func TestCommittedAdmissionHistoryIsClosedAndValid(t *testing.T) {
	ledger, err := LoadHistory(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.Entries) != 7 || ledger.Entries[5].Status != "assigned-historical" || ledger.Entries[6].Status != "assigned-historical" {
		t.Fatalf("unexpected admission history: %#v", ledger.Entries)
	}
}

func TestAdmissionHistoryHasNoCurrentAssignment(t *testing.T) {
	ledger, err := LoadHistory(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range ledger.Entries {
		if entry.Status == "assigned-current" {
			t.Fatalf("retired namespace retained current authority: %#v", entry)
		}
	}
}
