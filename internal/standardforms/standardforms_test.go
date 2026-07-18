package standardforms

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCommittedStableSetVerifies(t *testing.T) {
	t.Parallel()
	if err := Verify(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}

func TestProviderReleaseGateFailsClosedWithoutExternalAdmission(t *testing.T) {
	t.Parallel()
	err := VerifyReleaseReady(filepath.Join("..", ".."))
	if err == nil || !strings.Contains(err.Error(), "missing admission/v1/standard-admission-set.json") {
		t.Fatalf("release gate error = %v", err)
	}
}
