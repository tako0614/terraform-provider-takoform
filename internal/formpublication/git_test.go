package formpublication

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireCommitAncestorRejectsRepositoryGrafts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runGitTestCommand(t, root, "init", "--quiet", "--initial-branch=main")
	runGitTestCommand(t, root, "config", "user.name", "Takoform Test")
	runGitTestCommand(t, root, "config", "user.email", "test@takoform.invalid")
	writeGitTestFile(t, filepath.Join(root, "main.txt"), "main\n")
	runGitTestCommand(t, root, "add", "main.txt")
	runGitTestCommand(t, root, "commit", "--quiet", "-m", "main")
	mainCommit := strings.TrimSpace(runGitTestCommand(t, root, "rev-parse", "HEAD"))

	runGitTestCommand(t, root, "checkout", "--quiet", "--orphan", "unrelated")
	runGitTestCommand(t, root, "rm", "--quiet", "--force", "main.txt")
	writeGitTestFile(t, filepath.Join(root, "unrelated.txt"), "unrelated\n")
	runGitTestCommand(t, root, "add", "unrelated.txt")
	runGitTestCommand(t, root, "commit", "--quiet", "-m", "unrelated")
	unrelatedCommit := strings.TrimSpace(runGitTestCommand(t, root, "rev-parse", "HEAD"))

	grafts := filepath.Join(root, ".git", "info", "grafts")
	writeGitTestFile(t, grafts, mainCommit+" "+unrelatedCommit+"\n")
	if err := requireCommitAncestor(root, "unrelated fixture", unrelatedCommit, mainCommit); err == nil ||
		!strings.Contains(err.Error(), "Git graft authority is forbidden") {
		t.Fatalf("repository graft was not rejected before ancestry verification: %v", err)
	}
}

func runGitTestCommand(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func writeGitTestFile(t *testing.T, name, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
