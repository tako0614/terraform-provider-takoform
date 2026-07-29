package admissionrelease

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissioncheckpoint"
)

const providerSigningFingerprint = "3510E75E05BBCC303B92D77934FC18AC897FB709"

type gitReleaseRefVerifier struct{}
type gitMaterialRefVerifier struct{}

func (gitReleaseRefVerifier) VerifyReleaseRefs(root string, set Set, readback ProviderRegistryReadback) error {
	return verifyGitReleaseRefs(root, set, readback, true)
}

func (gitMaterialRefVerifier) VerifyReleaseRefs(root string, set Set, readback ProviderRegistryReadback) error {
	return verifyGitReleaseRefs(root, set, readback, false)
}

func verifyGitReleaseRefs(root string, set Set, readback ProviderRegistryReadback, requireAdmissionTag bool) error {
	head, err := resolveCommit(root, "HEAD")
	if err != nil {
		return err
	}
	if requireAdmissionTag {
		admissionCommit, err := resolveTagCommit(root, set.AdmissionReleaseTag)
		if err != nil {
			return fmt.Errorf("admission checkpoint tag: %w", err)
		}
		if admissionCommit != head {
			if err := requireCommitAncestor(root, "admission checkpoint", admissionCommit, head); err != nil {
				return err
			}
		}
		if set.Generation == "ga-core-v2" {
			if err := verifyCurrentAdmissionTag(root, set.AdmissionReleaseTag, admissionCommit, head); err != nil {
				return err
			}
		}
	}

	if err := requireTagCommit(root, "provider release", readback.ProviderReleaseTag, readback.ProviderReleaseCommit); err != nil {
		return err
	}
	tagType, err := gitOutput(root, "cat-file", "-t", "refs/tags/"+readback.ProviderReleaseTag)
	if err != nil || strings.TrimSpace(tagType) != "tag" {
		return fmt.Errorf("provider release tag %q must be an annotated signed tag", readback.ProviderReleaseTag)
	}
	verification, err := gitOutput(root, "verify-tag", "--raw", readback.ProviderReleaseTag)
	if err != nil {
		return fmt.Errorf("provider release tag %q signature: %w\n%s", readback.ProviderReleaseTag, err, verification)
	}
	if !hasPinnedProviderSignature(verification) {
		return fmt.Errorf("provider release tag %q is not signed by pinned fingerprint %s", readback.ProviderReleaseTag, providerSigningFingerprint)
	}

	for _, entry := range set.Entries {
		if err := requireTagCommit(root, entry.Kind+" package release", entry.ReleaseTag, entry.ReleaseCommit); err != nil {
			return err
		}
		if err := requireCommitAncestor(root, entry.Kind+" release tooling", entry.ReleaseToolingCommit, head); err != nil {
			return err
		}
	}
	return nil
}

func verifyCurrentAdmissionTag(root, tag, admissionCommit, head string) error {
	descriptor, _, err := admissioncheckpoint.LoadCurrent(root)
	if err != nil {
		return fmt.Errorf("admission checkpoint descriptor: %w", err)
	}
	if tag != descriptor.Tag {
		return fmt.Errorf("admission checkpoint tag %q does not equal descriptor tag %q", tag, descriptor.Tag)
	}
	tagType, err := gitOutput(root, "cat-file", "-t", "refs/tags/"+tag)
	if err != nil || strings.TrimSpace(tagType) != "tag" {
		return fmt.Errorf("admission checkpoint tag %q must be annotated", tag)
	}
	taggedAdmissionTree, err := gitOutput(root, "rev-parse", admissionCommit+":"+descriptor.RetainedRoot)
	if err != nil {
		return fmt.Errorf("resolve tagged admission tree: %w", err)
	}
	headAdmissionTree, err := gitOutput(root, "rev-parse", head+":"+descriptor.RetainedRoot)
	if err != nil {
		return fmt.Errorf("resolve checked-out admission tree: %w", err)
	}
	if strings.TrimSpace(taggedAdmissionTree) != strings.TrimSpace(headAdmissionTree) {
		return fmt.Errorf("checked-out %s bytes differ from admission checkpoint commit %s", descriptor.RetainedRoot, admissionCommit)
	}

	tree, err := gitOutput(root, "rev-parse", admissionCommit+"^{tree}")
	if err != nil {
		return fmt.Errorf("resolve admission checkpoint source tree: %w", err)
	}
	descriptorRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(admissioncheckpoint.CurrentDescriptorPath)))
	if err != nil {
		return err
	}
	identityLedgerRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(admissioncheckpoint.CurrentIdentityLedgerPath)))
	if err != nil {
		return err
	}
	setRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(descriptor.RetainedRoot), setManifestName))
	if err != nil {
		return err
	}
	expected := currentAdmissionTagMessage(
		descriptor.Version,
		descriptor.Generation,
		admissionCommit,
		strings.TrimSpace(tree),
		formpackage.DigestBytes(descriptorRaw),
		formpackage.DigestBytes(identityLedgerRaw),
		formpackage.DigestBytes(setRaw),
	)
	tagObject, err := gitOutput(root, "cat-file", "tag", "refs/tags/"+tag)
	if err != nil {
		return fmt.Errorf("read admission checkpoint tag object: %w", err)
	}
	separator := strings.Index(tagObject, "\n\n")
	if separator < 0 || tagObject[separator+2:] != expected {
		return fmt.Errorf("admission checkpoint tag %q message does not bind the exact retained commit/tree/descriptor/identity-ledger/set", tag)
	}
	return nil
}

func currentAdmissionTagMessage(version, generation, commit, tree, descriptorDigest, identityLedgerDigest, setDigest string) string {
	return fmt.Sprintf(
		"Activate Standard Form admission v%s\n\ngeneration %s\ncommit %s\ntree %s\nversion-descriptor %s\nidentity-ledger %s\nstandard-admission-set %s\n",
		version,
		generation,
		commit,
		tree,
		descriptorDigest,
		identityLedgerDigest,
		setDigest,
	)
}

func requireCommitAncestor(root, label, commit, descendant string) error {
	if !releaseCommitPattern.MatchString(commit) || !releaseCommitPattern.MatchString(descendant) {
		return fmt.Errorf("%s commit ancestry requires exact lowercase 40-hex commits", label)
	}
	if _, err := gitOutput(root, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return fmt.Errorf("%s commit %s is not retained in source history", label, commit)
	}
	if _, err := gitOutput(root, "merge-base", "--is-ancestor", commit, descendant); err != nil {
		return fmt.Errorf("%s commit %s is not an ancestor of admission commit %s", label, commit, descendant)
	}
	return nil
}

func requireTagCommit(root, label, tag, expectedCommit string) error {
	commit, err := resolveTagCommit(root, tag)
	if err != nil {
		return fmt.Errorf("%s tag: %w", label, err)
	}
	if commit != expectedCommit {
		return fmt.Errorf("%s tag %q resolves to %s, want retained commit %s", label, tag, commit, expectedCommit)
	}
	return nil
}

func hasPinnedProviderSignature(verification string) bool {
	for _, line := range strings.Split(verification, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "[GNUPG:]" && fields[1] == "VALIDSIG" && fields[2] == providerSigningFingerprint {
			return true
		}
	}
	return false
}

func resolveTagCommit(root, tag string) (string, error) {
	return resolveCommit(root, "refs/tags/"+tag)
}

func resolveCommit(root, ref string) (string, error) {
	output, err := gitOutput(root, "rev-list", "-n", "1", ref)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", ref, err)
	}
	commit := strings.TrimSpace(output)
	if !releaseCommitPattern.MatchString(commit) {
		return "", fmt.Errorf("resolve %q returned invalid commit %q", ref, commit)
	}
	return commit, nil
}

func gitOutput(root string, arguments ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.CombinedOutput()
	return string(output), err
}
