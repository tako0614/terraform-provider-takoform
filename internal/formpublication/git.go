package formpublication

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func verifyGitAuthority(repositoryRoot, publicationRoot string, set Set) error {
	objectFormat, err := gitText(repositoryRoot, "rev-parse", "--show-object-format")
	if err != nil {
		return err
	}
	if strings.TrimSpace(objectFormat) != gitObjectFormat ||
		strings.TrimSpace(objectFormat) != set.GitObjectFormat {
		return fmt.Errorf("repository object format is not the retained SHA-1 format")
	}
	shallow, err := gitText(repositoryRoot, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return err
	}
	if strings.TrimSpace(shallow) != "false" {
		return fmt.Errorf("source repository must be non-shallow")
	}
	head, err := resolveExactCommit(repositoryRoot, "HEAD")
	if err != nil {
		return fmt.Errorf("resolve repository HEAD: %w", err)
	}
	if err := requireCommitAncestor(
		repositoryRoot, "protected main", set.ProtectedMainCommit, head,
	); err != nil {
		return err
	}

	planRaw, err := readRelativeRegularFile(
		publicationRoot, set.SourcePlan.Path, maxReleasePlanBytes,
	)
	if err != nil {
		return err
	}
	trustedRootRaw, err := readRelativeRegularFile(
		publicationRoot, set.VerificationPolicy.TrustedRoot.Path, maxTrustedRootBytes,
	)
	if err != nil {
		return err
	}
	if err := requireGitBlobBytes(
		repositoryRoot, set.ProtectedMainCommit, set.SourcePlan.SourcePath, planRaw,
	); err != nil {
		return fmt.Errorf("protected-main release plan: %w", err)
	}
	if err := requireGitBlobBytes(
		repositoryRoot, set.ProtectedMainCommit,
		set.VerificationPolicy.TrustedRoot.SourcePath, trustedRootRaw,
	); err != nil {
		return fmt.Errorf("protected-main trusted root: %w", err)
	}

	verifiedCommits := map[string]struct{}{set.ProtectedMainCommit: {}}
	verifiedAuthority := make(map[string]struct{})
	verifiedSources := make(map[string]struct{})
	for position, entry := range set.Entries {
		for label, commit := range map[string]string{
			"source": entry.SourceCommit, "tooling": entry.ToolingCommit,
		} {
			if _, ok := verifiedCommits[commit]; ok {
				continue
			}
			if err := requireCommitAncestor(
				repositoryRoot, label, commit, set.ProtectedMainCommit,
			); err != nil {
				return fmt.Errorf("entries[%d] %s", position, err)
			}
			verifiedCommits[commit] = struct{}{}
		}
		if err := requireCommitAncestor(
			repositoryRoot, "source", entry.SourceCommit, entry.ToolingCommit,
		); err != nil {
			return fmt.Errorf("entries[%d] %s", position, err)
		}
		if err := verifyExactTag(repositoryRoot, entry); err != nil {
			return fmt.Errorf("entries[%d] tag: %w", position, err)
		}

		if _, ok := verifiedAuthority[entry.ToolingCommit]; !ok {
			plan, err := readRelativeRegularFile(
				publicationRoot, entry.ReleasePlan.Path, maxReleasePlanBytes,
			)
			if err != nil {
				return fmt.Errorf("entries[%d] retained authority plan: %w", position, err)
			}
			if err := requireGitBlobBytes(
				repositoryRoot, entry.ToolingCommit, entry.ReleasePlan.SourcePath, plan,
			); err != nil {
				return fmt.Errorf("entries[%d] tooling authority plan: %w", position, err)
			}
			root, err := readRelativeRegularFile(
				publicationRoot, entry.TrustedRoot.Path, maxTrustedRootBytes,
			)
			if err != nil {
				return fmt.Errorf("entries[%d] retained authority root: %w", position, err)
			}
			if err := requireGitBlobBytes(
				repositoryRoot, entry.ToolingCommit, entry.TrustedRoot.SourcePath, root,
			); err != nil {
				return fmt.Errorf("entries[%d] tooling authority root: %w", position, err)
			}
			verifiedAuthority[entry.ToolingCommit] = struct{}{}
		}

		sourceKey := entry.SourceCommit + "\x00" + entry.ToolingCommit + "\x00" + entry.SourcePath
		if _, ok := verifiedSources[sourceKey]; !ok {
			if err := requireSourceUnchanged(
				repositoryRoot, entry.SourceCommit, entry.ToolingCommit, entry.SourcePath,
			); err != nil {
				return fmt.Errorf("entries[%d] package source: %w", position, err)
			}
			verifiedSources[sourceKey] = struct{}{}
		}
	}
	return nil
}

func verifyExactTag(repositoryRoot string, entry Entry) error {
	tagRef := "refs/tags/" + entry.Tag
	object, err := gitText(
		repositoryRoot, "rev-parse", "--verify", tagRef+"^{tag}",
	)
	if err != nil {
		return fmt.Errorf("resolve annotated object: %w", err)
	}
	if strings.TrimSpace(object) != entry.TagObjectOID {
		return fmt.Errorf(
			"annotated object is %s, want %s",
			strings.TrimSpace(object), entry.TagObjectOID,
		)
	}
	objectType, err := gitText(repositoryRoot, "cat-file", "-t", entry.TagObjectOID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(objectType) != "tag" {
		return fmt.Errorf("retained tag object is not an annotated tag")
	}
	raw, err := gitBytes(repositoryRoot, "cat-file", "tag", entry.TagObjectOID)
	if err != nil {
		return err
	}
	if err := verifyTagHeader(raw, entry.Tag, entry.SourceCommit); err != nil {
		return err
	}
	peeled, err := gitText(repositoryRoot, "rev-parse", "--verify", tagRef+"^{commit}")
	if err != nil {
		return fmt.Errorf("peel annotated tag: %w", err)
	}
	if strings.TrimSpace(peeled) != entry.PeeledCommit ||
		entry.PeeledCommit != entry.SourceCommit {
		return fmt.Errorf("annotated tag does not peel to retained source commit")
	}
	return nil
}

func verifyTagHeader(raw []byte, expectedTag, expectedCommit string) error {
	headerEnd := bytes.Index(raw, []byte("\n\n"))
	if headerEnd < 0 {
		return fmt.Errorf("annotated tag object is missing its header separator")
	}
	lines := bytes.Split(raw[:headerEnd], []byte("\n"))
	expected := [][]byte{
		[]byte("object " + expectedCommit),
		[]byte("type commit"),
		[]byte("tag " + expectedTag),
	}
	if len(lines) < len(expected) {
		return fmt.Errorf("annotated tag object omits object/type/tag headers")
	}
	for index := range expected {
		if !bytes.Equal(lines[index], expected[index]) {
			return fmt.Errorf("annotated tag %s header differs from retained identity", strings.Fields(string(expected[index]))[0])
		}
	}
	for _, line := range lines[len(expected):] {
		field, _, _ := bytes.Cut(line, []byte(" "))
		if bytes.Equal(field, []byte("object")) ||
			bytes.Equal(field, []byte("type")) ||
			bytes.Equal(field, []byte("tag")) {
			return fmt.Errorf("annotated tag object duplicates %s header", field)
		}
	}
	return nil
}

func requireGitBlobBytes(repositoryRoot, commit, sourcePath string, retained []byte) error {
	if !commitPattern.MatchString(commit) {
		return fmt.Errorf("commit is not an exact lowercase SHA-1 object")
	}
	if err := validateRelativePath(sourcePath); err != nil {
		return err
	}
	listing, err := gitBytes(
		repositoryRoot, "ls-tree", "-z", "--full-tree", commit, "--", sourcePath,
	)
	if err != nil {
		return err
	}
	record := bytes.TrimSuffix(listing, []byte{0})
	metadata, entryPath, ok := bytes.Cut(record, []byte{'\t'})
	fields := strings.Fields(string(metadata))
	if !ok || string(entryPath) != sourcePath || len(fields) != 3 ||
		fields[0] != "100644" || fields[1] != "blob" ||
		!commitPattern.MatchString(fields[2]) {
		return fmt.Errorf("%q is not one exact ordinary Git blob", sourcePath)
	}
	committed, err := gitBytes(repositoryRoot, "show", commit+":"+sourcePath)
	if err != nil {
		return err
	}
	if !bytes.Equal(committed, retained) {
		return fmt.Errorf("retained %q bytes differ from exact commit %s", sourcePath, commit)
	}
	return nil
}

func requireSourceUnchanged(repositoryRoot, sourceCommit, toolingCommit, sourcePath string) error {
	if !commitPattern.MatchString(sourceCommit) || !commitPattern.MatchString(toolingCommit) {
		return fmt.Errorf("source/tooling commits must be exact lowercase SHA-1 objects")
	}
	if err := validateRelativePath(sourcePath); err != nil {
		return err
	}
	if _, err := gitBytes(
		repositoryRoot, "diff", "--quiet", "--no-ext-diff", "--no-textconv",
		sourceCommit, toolingCommit, "--", sourcePath,
	); err != nil {
		return fmt.Errorf("source changed between tagged source and tooling authority: %w", err)
	}
	return nil
}

func requireCommitAncestor(repositoryRoot, label, commit, descendant string) error {
	if !commitPattern.MatchString(commit) || !commitPattern.MatchString(descendant) {
		return fmt.Errorf("%s ancestry requires exact lowercase SHA-1 objects", label)
	}
	resolved, err := resolveExactCommit(repositoryRoot, commit)
	if err != nil {
		return fmt.Errorf("%s commit %s is unavailable: %w", label, commit, err)
	}
	if resolved != commit {
		return fmt.Errorf("%s commit does not resolve to the retained object", label)
	}
	if commit == descendant {
		return nil
	}
	if _, err := gitBytes(
		repositoryRoot, "merge-base", "--is-ancestor", commit, descendant,
	); err != nil {
		return fmt.Errorf("%s commit %s is not an ancestor of %s", label, commit, descendant)
	}
	return nil
}

func resolveExactCommit(repositoryRoot, ref string) (string, error) {
	output, err := gitText(repositoryRoot, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(output)
	if !commitPattern.MatchString(commit) {
		return "", fmt.Errorf("%q did not resolve to one lowercase SHA-1 commit", ref)
	}
	return commit, nil
}

func gitText(repositoryRoot string, arguments ...string) (string, error) {
	raw, err := gitBytes(repositoryRoot, arguments...)
	return string(raw), err
}

func gitBytes(repositoryRoot string, arguments ...string) ([]byte, error) {
	executable, err := trustedGitExecutable()
	if err != nil {
		return nil, err
	}
	if err := rejectGitGrafts(executable, repositoryRoot); err != nil {
		return nil, err
	}
	return runIsolatedGit(executable, repositoryRoot, arguments...)
}

func rejectGitGrafts(executable, repositoryRoot string) error {
	output, err := runIsolatedGit(
		executable,
		repositoryRoot,
		"rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir",
	)
	if err != nil {
		return fmt.Errorf("resolve isolated Git authority directories: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != 2 || !filepath.IsAbs(lines[0]) || !filepath.IsAbs(lines[1]) {
		return fmt.Errorf("resolve isolated Git authority directories returned an ambiguous result")
	}
	seen := make(map[string]struct{}, len(lines))
	for _, directory := range lines {
		grafts := filepath.Clean(filepath.Join(directory, "info", "grafts"))
		if _, duplicate := seen[grafts]; duplicate {
			continue
		}
		seen[grafts] = struct{}{}
		if _, err := os.Lstat(grafts); err == nil {
			return fmt.Errorf("repository-local Git graft authority is forbidden: %s", grafts)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect repository-local Git graft authority %s: %w", grafts, err)
		}
	}
	return nil
}

func runIsolatedGit(executable, repositoryRoot string, arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	prefix := []string{
		"--no-replace-objects",
		"-c", "advice.graftFileDeprecated=false",
		"-c", "core.fsmonitor=false",
		"-c", "credential.helper=",
		"-c", "core.hooksPath=/dev/null",
		"-C", repositoryRoot,
	}
	command := exec.CommandContext(ctx, executable, append(prefix, arguments...)...)
	command.Env = isolatedGitEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git %s: %w", strings.Join(arguments, " "), ctx.Err())
		}
		return nil, fmt.Errorf(
			"git %s: %w: %s",
			strings.Join(arguments, " "), err, strings.TrimSpace(string(output)),
		)
	}
	return output, nil
}

func trustedGitExecutable() (string, error) {
	for _, candidate := range []string{
		"/usr/bin/git",
		"/usr/local/bin/git",
		"/opt/homebrew/bin/git",
	} {
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 ||
			!info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("no fixed absolute git executable is available")
}

func isolatedGitEnvironment() []string {
	source := os.Environ()
	environment := make([]string, 0, len(source)+8)
	for _, value := range source {
		name, _, ok := strings.Cut(value, "=")
		if !ok || strings.HasPrefix(name, "GIT_") ||
			strings.HasPrefix(name, "GPG_") || name == "GNUPGHOME" {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_GRAFT_FILE=/dev/null",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
	)
}
