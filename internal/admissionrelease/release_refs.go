package admissionrelease

import (
	"fmt"
	"os/exec"
	"strings"
)

const providerSigningFingerprint = "3510E75E05BBCC303B92D77934FC18AC897FB709"

type gitReleaseRefVerifier struct{}

func (gitReleaseRefVerifier) VerifyReleaseRefs(root string, set Set, readback ProviderRegistryReadback) error {
	head, err := resolveCommit(root, "HEAD")
	if err != nil {
		return err
	}
	admissionCommit, err := resolveTagCommit(root, set.AdmissionReleaseTag)
	if err != nil {
		return fmt.Errorf("admission release tag: %w", err)
	}
	if admissionCommit != head {
		return fmt.Errorf("admission release tag %q resolves to %s, want checked-out commit %s", set.AdmissionReleaseTag, admissionCommit, head)
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
