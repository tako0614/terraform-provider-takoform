package formpublication

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
)

const (
	maxHistoricalFiles = 2048
	maxHistoricalBytes = maxClosureBytes + (32 << 20)
)

// HistoricalCheckpoint pins one assigned historical admission tag to its
// reviewed annotated-tag object and peeled commit. The caller must obtain
// these values from the append-only admission identity ledger, not from a
// mutable ref lookup.
type HistoricalCheckpoint struct {
	Tag       string
	TagObject string
	Commit    string
}

// VerifyHistoricalCheckpoint authenticates every portable Form publication
// closure retained by one assigned historical admission checkpoint. It reads
// the closure from the exact checkpoint commit, never from the checked-out
// worktree. Older checkpoints that predate the all-Form publication format
// legitimately return an empty result after their tag identity is verified.
func VerifyHistoricalCheckpoint(
	repositoryRoot string,
	checkpoint HistoricalCheckpoint,
) ([]Set, error) {
	if err := verifyHistoricalCheckpointTag(repositoryRoot, checkpoint); err != nil {
		return nil, fmt.Errorf("historical checkpoint %s: %w", checkpoint.Tag, err)
	}

	setPaths, err := historicalPublicationSetPaths(repositoryRoot, checkpoint.Commit)
	if err != nil {
		return nil, fmt.Errorf("historical checkpoint %s publication discovery: %w", checkpoint.Tag, err)
	}
	verified := make([]Set, 0, len(setPaths))
	for _, setPath := range setPaths {
		retainedRoot := path.Dir(setPath)
		temporary, err := os.MkdirTemp("", "takoform-historical-publication-*")
		if err != nil {
			return nil, err
		}
		func() {
			defer os.RemoveAll(temporary)
			if err = materializeHistoricalPublication(
				repositoryRoot, checkpoint.Commit, retainedRoot, temporary,
			); err != nil {
				return
			}
			var expected admissionrelease.CandidateSet
			expected, err = candidateSetFromPublication(temporary)
			if err != nil {
				err = fmt.Errorf("%s candidate projection: %w", retainedRoot, err)
				return
			}
			var set Set
			set, err = Verify(repositoryRoot, temporary, temporary, expected)
			if err != nil {
				err = fmt.Errorf("%s: %w", retainedRoot, err)
				return
			}
			verified = append(verified, set)
		}()
		if err != nil {
			return nil, fmt.Errorf("historical checkpoint %s publication: %w", checkpoint.Tag, err)
		}
	}
	return verified, nil
}

func verifyHistoricalCheckpointTag(repositoryRoot string, checkpoint HistoricalCheckpoint) error {
	if checkpoint.Tag == "" || strings.HasPrefix(checkpoint.Tag, "-") ||
		strings.ContainsAny(checkpoint.Tag, "\x00\n\r") ||
		!commitPattern.MatchString(checkpoint.TagObject) ||
		!commitPattern.MatchString(checkpoint.Commit) {
		return fmt.Errorf("ledger identity is not one exact tag object and commit")
	}
	object, err := gitText(
		repositoryRoot, "rev-parse", "--verify", "refs/tags/"+checkpoint.Tag+"^{tag}",
	)
	if err != nil {
		return fmt.Errorf("resolve annotated tag object: %w", err)
	}
	if strings.TrimSpace(object) != checkpoint.TagObject {
		return fmt.Errorf("annotated tag object differs from the assigned identity")
	}
	objectType, err := gitText(repositoryRoot, "cat-file", "-t", checkpoint.TagObject)
	if err != nil {
		return err
	}
	if strings.TrimSpace(objectType) != "tag" {
		return fmt.Errorf("assigned tag object is not an annotated tag")
	}
	raw, err := gitBytes(repositoryRoot, "cat-file", "tag", checkpoint.TagObject)
	if err != nil {
		return err
	}
	if err := verifyTagHeader(raw, checkpoint.Tag, checkpoint.Commit); err != nil {
		return err
	}
	peeled, err := resolveExactCommit(repositoryRoot, "refs/tags/"+checkpoint.Tag)
	if err != nil {
		return fmt.Errorf("peel annotated tag: %w", err)
	}
	if peeled != checkpoint.Commit {
		return fmt.Errorf("annotated tag does not peel to the assigned commit")
	}
	head, err := resolveExactCommit(repositoryRoot, "HEAD")
	if err != nil {
		return fmt.Errorf("resolve repository HEAD: %w", err)
	}
	if err := requireCommitAncestor(
		repositoryRoot, "historical admission checkpoint", checkpoint.Commit, head,
	); err != nil {
		return err
	}
	return nil
}

func historicalPublicationSetPaths(repositoryRoot, commit string) ([]string, error) {
	listing, err := gitBytes(
		repositoryRoot, "ls-tree", "-r", "-z", "--full-tree", commit, "--", "admission",
	)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, 1)
	for _, record := range bytes.Split(bytes.TrimSuffix(listing, []byte{0}), []byte{0}) {
		if len(record) == 0 {
			continue
		}
		metadata, rawPath, ok := bytes.Cut(record, []byte{'\t'})
		fields := strings.Fields(string(metadata))
		if !ok || len(fields) != 3 || fields[1] != "blob" ||
			!commitPattern.MatchString(fields[2]) || !utf8.Valid(rawPath) {
			return nil, fmt.Errorf("admission tree contains an unsupported Git entry")
		}
		entryPath := string(rawPath)
		if path.Base(entryPath) != SetFilename {
			continue
		}
		if fields[0] != "100644" || path.Dir(path.Dir(entryPath)) != "admission" ||
			path.Clean(entryPath) != entryPath {
			return nil, fmt.Errorf("publication set %q is not one canonical admission generation file", entryPath)
		}
		paths = append(paths, entryPath)
	}
	return paths, nil
}

func materializeHistoricalPublication(
	repositoryRoot, commit, retainedRoot, destination string,
) error {
	if err := validateRelativePath(retainedRoot); err != nil ||
		path.Dir(retainedRoot) != "admission" {
		return fmt.Errorf("retained root %q is not one admission generation", retainedRoot)
	}
	listing, err := gitBytes(
		repositoryRoot, "ls-tree", "-r", "-z", "--full-tree", commit, "--", retainedRoot,
	)
	if err != nil {
		return err
	}
	records := bytes.Split(bytes.TrimSuffix(listing, []byte{0}), []byte{0})
	if len(records) == 0 || len(records) > maxHistoricalFiles {
		return fmt.Errorf("historical publication has %d files, want 1..%d", len(records), maxHistoricalFiles)
	}
	total := int64(0)
	for _, record := range records {
		metadata, rawPath, ok := bytes.Cut(record, []byte{'\t'})
		fields := strings.Fields(string(metadata))
		if !ok || len(fields) != 3 || fields[0] != "100644" || fields[1] != "blob" ||
			!commitPattern.MatchString(fields[2]) || !utf8.Valid(rawPath) {
			return fmt.Errorf("historical publication contains an unsupported Git entry")
		}
		entryPath := string(rawPath)
		prefix := retainedRoot + "/"
		if !strings.HasPrefix(entryPath, prefix) {
			return fmt.Errorf("historical publication entry escaped retained root")
		}
		relative := strings.TrimPrefix(entryPath, prefix)
		if err := validateRelativePath(relative); err != nil {
			return fmt.Errorf("historical publication entry: %w", err)
		}
		sizeText, err := gitText(repositoryRoot, "cat-file", "-s", fields[2])
		if err != nil {
			return err
		}
		size, err := strconv.ParseInt(strings.TrimSpace(sizeText), 10, 64)
		if err != nil || size <= 0 || size > maxAssetBytes {
			return fmt.Errorf("historical publication file %q has invalid size", relative)
		}
		total += size
		if total > maxHistoricalBytes {
			return fmt.Errorf("historical publication exceeds %d bytes", maxHistoricalBytes)
		}
		raw, err := gitBytes(repositoryRoot, "cat-file", "blob", fields[2])
		if err != nil {
			return err
		}
		if int64(len(raw)) != size {
			return fmt.Errorf("historical publication file %q changed while read", relative)
		}
		name := filepath.Join(destination, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(name, raw, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func candidateSetFromPublication(publicationRoot string) (admissionrelease.CandidateSet, error) {
	setRaw, err := readRelativeRegularFile(publicationRoot, SetFilename, maxSetBytes)
	if err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	canonical, err := formpackage.Canonicalize(setRaw)
	if err != nil || !bytes.Equal(setRaw, canonical) {
		return admissionrelease.CandidateSet{}, fmt.Errorf("%s is not canonical I-JSON", SetFilename)
	}
	var set Set
	if err := decodeStrictJSON(setRaw, &set); err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	if err := validateSetIdentity(set); err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	planRaw, err := readRelativeRegularFile(publicationRoot, set.SourcePlan.Path, maxReleasePlanBytes)
	if err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	if formpackage.DigestBytes(planRaw) != set.SourcePlan.SHA256 {
		return admissionrelease.CandidateSet{}, fmt.Errorf("source release plan digest mismatch")
	}
	plan, err := decodeReleasePlan(planRaw)
	if err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	if err := validateAuthorityReleasePlan(plan); err != nil {
		return admissionrelease.CandidateSet{}, err
	}
	expected := admissionrelease.CandidateSet{
		Generation: portableGeneration,
		Entries:    make([]admissionrelease.Candidate, 0, len(plan.Releases)),
	}
	for _, planned := range plan.Releases {
		expected.Entries = append(expected.Entries, admissionrelease.Candidate{
			Kind: planned.Kind, Slug: planned.Slug, PackagePath: planned.SourcePath,
			FormRef: planned.FormRef, PackageDigest: planned.PackageDigest,
		})
	}
	return expected, nil
}
