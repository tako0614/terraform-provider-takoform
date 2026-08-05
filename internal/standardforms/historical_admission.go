package standardforms

import (
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissioncheckpoint"
)

// verifyHistoricalAdmissionAssignments authenticates the append-only Git
// identities and exact retained set bytes. It intentionally does not rerun a
// historical conformance claim under today's provider/catalog policy: doing
// so would reinterpret old evidence through a different specification.
func verifyHistoricalAdmissionAssignments(root string) error {
	ledger, err := admissioncheckpoint.LoadHistory(root)
	if err != nil {
		return err
	}
	for _, identity := range ledger.Entries {
		ref := "refs/tags/" + identity.Tag
		if identity.Status == "reserved-abandoned" {
			command := exec.Command("git", "-C", root, "show-ref", "--verify", "--quiet", ref)
			err := command.Run()
			if err == nil {
				return fmt.Errorf("reserved-abandoned admission identity %s exists and must never be minted", identity.Tag)
			}
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				return fmt.Errorf("inspect reserved admission identity %s: %w", identity.Tag, err)
			}
			continue
		}
		tagObject, err := historicalGitOutput(root, "rev-parse", "--verify", ref+"^{tag}")
		if err != nil || tagObject != identity.TagObject {
			return fmt.Errorf("historical admission tag %s object is %q, want %s: %w", identity.Tag, tagObject, identity.TagObject, err)
		}
		objectType, err := historicalGitOutput(root, "cat-file", "-t", identity.TagObject)
		if err != nil || objectType != "tag" {
			return fmt.Errorf("historical admission identity %s is not the pinned annotated tag object", identity.Tag)
		}
		commit, err := historicalGitOutput(root, "rev-parse", "--verify", ref+"^{}")
		if err != nil || commit != identity.Commit {
			return fmt.Errorf("historical admission tag %s commit is %q, want %s: %w", identity.Tag, commit, identity.Commit, err)
		}
		tree, err := historicalGitOutput(root, "rev-parse", identity.Commit+":"+identity.RetainedRoot)
		if err != nil || tree != identity.RetainedTree {
			return fmt.Errorf("historical admission tag %s retained tree is %q, want %s: %w", identity.Tag, tree, identity.RetainedTree, err)
		}
		setPath := path.Join(identity.RetainedRoot, "standard-admission-set.json")
		setRaw, err := exec.Command("git", "-C", root, "show", identity.Commit+":"+setPath).Output()
		if err != nil {
			return fmt.Errorf("read historical admission set %s at %s: %w", setPath, identity.Tag, err)
		}
		if digest := formpackage.DigestBytes(setRaw); digest != identity.SetDigest {
			return fmt.Errorf("historical admission tag %s set digest is %s, want %s", identity.Tag, digest, identity.SetDigest)
		}
	}
	return nil
}

func historicalGitOutput(root string, arguments ...string) (string, error) {
	output, err := exec.Command("git", append([]string{"-C", root}, arguments...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output)), nil
}
