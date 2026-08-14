package formpublication_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/formpublication"
)

func TestVerifyHistoricalCheckpointAuthenticatesExactCommittedPublication(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	tag := "forms/admissions/v1.0.6"
	checkpoint := formpublication.HistoricalCheckpoint{
		Tag:       tag,
		TagObject: testGitText(t, root, "rev-parse", "--verify", "refs/tags/"+tag+"^{tag}"),
		Commit:    testGitText(t, root, "rev-parse", "--verify", "refs/tags/"+tag+"^{commit}"),
	}

	sets, err := formpublication.VerifyHistoricalCheckpoint(root, checkpoint)
	if err != nil {
		t.Fatalf("verify exact historical publication: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Entries) != exactTestPackageCount {
		t.Fatalf("historical publication closure = %d sets / %d entries, want 1 / %d",
			len(sets), historicalEntryCount(sets), exactTestPackageCount)
	}

	t.Run("assigned tag object drift", func(t *testing.T) {
		drifted := checkpoint
		drifted.TagObject = strings.Repeat("0", 40)
		_, err := formpublication.VerifyHistoricalCheckpoint(root, drifted)
		if err == nil || !strings.Contains(err.Error(), "differs from the assigned identity") {
			t.Fatalf("tag-object drift error = %v", err)
		}
	})
	t.Run("assigned commit drift", func(t *testing.T) {
		drifted := checkpoint
		drifted.Commit = strings.Repeat("0", 40)
		_, err := formpublication.VerifyHistoricalCheckpoint(root, drifted)
		if err == nil || !strings.Contains(err.Error(), "header differs") {
			t.Fatalf("commit drift error = %v", err)
		}
	})
}

func historicalEntryCount(sets []formpublication.Set) int {
	if len(sets) == 0 {
		return 0
	}
	return len(sets[0].Entries)
}

func testGitText(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	raw, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(raw)))
	}
	return strings.TrimSpace(string(raw))
}
