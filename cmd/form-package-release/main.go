// Command form-package-release builds deterministic, data-only release
// evidence for Takoform Form Packages and security revocation statements. It
// never signs, tags, uploads, or publishes. Those operations are confined to
// the protected GitHub Actions workflows that call this command.
package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha1" // SPDX 2.3 package verification code requires SHA-1.
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	canonicalization = "RFC8785"
	sourceRepository = "github.com/tako0614/terraform-provider-takoform"
	packageWorkflow  = ".github/workflows/form-package-release.yml"
	setWorkflow      = ".github/workflows/standard-form-package-set-release.yml"
	revokeWorkflow   = ".github/workflows/form-package-revocation.yml"
	bundleMediaType  = "application/vnd.dev.sigstore.bundle.v0.3+json"
)

var (
	packageTagPattern    = regexp.MustCompile(`^forms/(k-[a-z2-7]{2,103})/v(` + semverPattern + `)$`)
	revocationTagPattern = regexp.MustCompile(`^forms/revocations/v(` + semverPattern + `)$`)
	revocationPath       = regexp.MustCompile(`^forms/revocations(?:/checkpoints)?/[0-9A-Za-z.+-]+\.json$`)
	kindPattern          = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,63}$`)
	commitPattern        = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

const semverPattern = `(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?`

type releaseAsset struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

type releaseManifest struct {
	SchemaVersion       int                     `json:"schemaVersion"`
	ReleaseType         string                  `json:"releaseType"`
	Tag                 string                  `json:"tag"`
	SourceRepository    string                  `json:"sourceRepository"`
	SourceCommit        string                  `json:"sourceCommit"`
	ToolingCommit       string                  `json:"toolingCommit"`
	Workflow            string                  `json:"workflow"`
	PackageVersion      string                  `json:"packageVersion,omitempty"`
	ReleaseID           string                  `json:"releaseId,omitempty"`
	PackageDigest       string                  `json:"packageDigest"`
	FormRef             formpackage.FormRef     `json:"formRef"`
	CheckpointSequence  uint64                  `json:"checkpointSequence,omitempty"`
	CheckpointDigest    string                  `json:"checkpointDigest,omitempty"`
	Canonicalization    string                  `json:"canonicalization"`
	SignedSubject       string                  `json:"signedSubject"`
	SignatureBundle     string                  `json:"signatureBundle"`
	SignatureMediaType  string                  `json:"signatureMediaType"`
	PublisherPolicy     publisherPolicyEvidence `json:"publisherPolicy"`
	Assets              []releaseAsset          `json:"assets"`
	PublicationReady    bool                    `json:"publicationReady"`
	PublicationBlockers []string                `json:"publicationBlockers"`
}

type publisherPolicyEvidence struct {
	OIDCIssuer    string `json:"oidcIssuer"`
	Identity      string `json:"identity"`
	TagPattern    string `json:"tagPattern"`
	ToolingCommit string `json:"toolingCommit"`
}

type statement struct {
	Type          string             `json:"_type"`
	Subject       []statementSubject `json:"subject"`
	PredicateType string             `json:"predicateType"`
	Predicate     map[string]any     `json:"predicate"`
}

type statementSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "form-package-release:", err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer) error {
	if len(arguments) == 0 {
		return usageError()
	}
	switch arguments[0] {
	case "build-package":
		return runBuildPackage(arguments[1:], output)
	case "build-revocation":
		return runBuildRevocation(arguments[1:], output)
	case "finalize-bundle":
		return runFinalize(arguments[1:], output)
	case "build-standard-set":
		return runBuildStandardSet(arguments[1:], output)
	case "finalize-standard-set":
		return runFinalizeStandardSet(arguments[1:], output)
	case "verify-standard-set-candidate":
		return runVerifyStandardSetCandidate(arguments[1:], output)
	case "build-standard-set-readback":
		return runBuildStandardSetReadback(arguments[1:], output)
	case "check-revocations":
		return runCheckRevocations(arguments[1:])
	default:
		return usageError()
	}
}

func usageError() error {
	return errors.New("usage: form-package-release <build-package|build-revocation|finalize-bundle|build-standard-set|finalize-standard-set|verify-standard-set-candidate|build-standard-set-readback|check-revocations> [options]")
}

func runBuildPackage(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("build-package", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repo := flags.String("repo", ".", "repository root")
	tag := flags.String("tag", "", "exact forms/<release-id>/v<semver> tag")
	packageDir := flags.String("package-dir", "", "candidate-only package source override")
	outputDir := flags.String("output", "", "new output directory")
	toolingCommit := flags.String("tooling-commit", "", "exact protected-main release-tooling commit")
	allowUntagged := flags.Bool("allow-untagged-candidate", false, "permit non-publishable local candidate without an attached tag")
	allowDirty := flags.Bool("allow-dirty-candidate", false, "permit non-publishable local candidate from a dirty tree")
	coordinatedSet := flags.Bool("coordinated-standard-set", false, "bind this package to the root-activated standard set workflow")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *tag == "" || *outputDir == "" || !commitPattern.MatchString(*toolingCommit) {
		return usageError()
	}
	matches := packageTagPattern.FindStringSubmatch(*tag)
	if matches == nil {
		return fmt.Errorf("package tag must match forms/k-<lowercase-base32-kind>/v<semver>")
	}
	releaseID, version := matches[1], matches[2]
	tagKind, err := kindFromReleaseID(releaseID)
	if err != nil {
		return err
	}
	if *packageDir != "" && !*allowUntagged {
		return fmt.Errorf("--package-dir is allowed only with --allow-untagged-candidate")
	}
	if *packageDir == "" {
		*packageDir = filepath.Join(*repo, "forms", "releases", releaseID, version)
	}
	evidence, err := inspectSource(*repo, *tag, *allowUntagged, *allowDirty)
	if err != nil {
		return err
	}
	report, err := formpackage.VerifyDirectory(*packageDir)
	if err != nil {
		return fmt.Errorf("verify Form Package: %w", err)
	}
	indexRaw, err := os.ReadFile(filepath.Join(*packageDir, formpackage.PackageIndexFilename))
	if err != nil {
		return err
	}
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		return err
	}
	if index.PackageVersion != version {
		return fmt.Errorf("tag version %q does not match packageVersion %q", version, index.PackageVersion)
	}
	if tagKind != index.FormRef.Kind {
		return fmt.Errorf("tag release id %q decodes to kind %q, not FormRef kind %q", releaseID, tagKind, index.FormRef.Kind)
	}
	canonicalIndex, err := formpackage.Canonicalize(indexRaw)
	if err != nil {
		return err
	}
	base := "takoform-form-" + releaseID + "_" + version
	workflow := packageWorkflow
	tagPattern := "refs/tags/forms/k-*/v*"
	if *coordinatedSet {
		workflow = setWorkflow
		tagPattern = "refs/tags/standard-forms/v*"
	}
	policy := publisherPolicy(workflow, tagPattern, *toolingCommit)
	if *coordinatedSet {
		policy.Identity = "https://" + sourceRepository + "/" + workflow + "@refs/tags/standard-forms/v" + version
	}
	manifest := releaseManifest{
		SchemaVersion: 1, ReleaseType: "form-package", Tag: *tag,
		SourceRepository: sourceRepository, SourceCommit: evidence.commit, ToolingCommit: *toolingCommit,
		Workflow: workflow, PackageVersion: version, ReleaseID: releaseID,
		PackageDigest: report.PackageDigest, FormRef: report.FormRef,
		Canonicalization: canonicalization, SignedSubject: base + "_package-index.json",
		SignatureBundle: base + "_package-index.sigstore.json", SignatureMediaType: bundleMediaType,
		PublisherPolicy:  policy,
		PublicationReady: false, PublicationBlockers: evidence.blockers,
	}
	if err := createOutput(*outputDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outputDir, manifest.SignedSubject), canonicalIndex, 0o644); err != nil {
		return err
	}
	archiveName := base + ".tar.gz"
	if err := writePackageArchive(filepath.Join(*outputDir, archiveName), *packageDir, canonicalIndex, index.Files); err != nil {
		return err
	}
	assets, err := describeAssets(*outputDir, []namedMedia{
		{name: manifest.SignedSubject, mediaType: "application/vnd.takoform.package-index.v1+json"},
		{name: archiveName, mediaType: "application/gzip"},
	})
	if err != nil {
		return err
	}
	sbomName := base + "_sbom.spdx.json"
	sbom, err := createPackageSBOM(index, report, evidence.commitTime, canonicalIndex, *packageDir)
	if err != nil {
		return err
	}
	if err := writeCanonicalJSON(filepath.Join(*outputDir, sbomName), sbom); err != nil {
		return err
	}
	sbomAsset, err := describeAsset(filepath.Join(*outputDir, sbomName), sbomName, "application/spdx+json")
	if err != nil {
		return err
	}
	assets = append(assets, sbomAsset)
	provenanceName := base + "_provenance.intoto.json"
	provenance := createProvenance(*tag, workflow, evidence.commit, *toolingCommit, assets[:2])
	if err := writeCanonicalJSON(filepath.Join(*outputDir, provenanceName), provenance); err != nil {
		return err
	}
	provenanceAsset, err := describeAsset(filepath.Join(*outputDir, provenanceName), provenanceName, "application/vnd.in-toto+json")
	if err != nil {
		return err
	}
	manifest.Assets = append(assets, provenanceAsset)
	if err := writeJSON(filepath.Join(*outputDir, "release-manifest.json"), manifest); err != nil {
		return err
	}
	return writeJSONTo(output, manifest)
}

func runBuildRevocation(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("build-revocation", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repo := flags.String("repo", ".", "repository root")
	tag := flags.String("tag", "", "exact forms/revocations/v<semver> tag")
	statementPath := flags.String("statement", "", "candidate-only statement override")
	checkpointPath := flags.String("checkpoint", "", "candidate-only cumulative checkpoint override")
	outputDir := flags.String("output", "", "new output directory")
	toolingCommit := flags.String("tooling-commit", "", "exact protected-main release-tooling commit")
	allowUntagged := flags.Bool("allow-untagged-candidate", false, "permit non-publishable local candidate without an attached tag")
	allowDirty := flags.Bool("allow-dirty-candidate", false, "permit non-publishable local candidate from a dirty tree")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *tag == "" || *outputDir == "" || !commitPattern.MatchString(*toolingCommit) {
		return usageError()
	}
	matches := revocationTagPattern.FindStringSubmatch(*tag)
	if matches == nil {
		return fmt.Errorf("revocation tag must match forms/revocations/v<semver>")
	}
	version := matches[1]
	if *statementPath != "" && !*allowUntagged {
		return fmt.Errorf("--statement is allowed only with --allow-untagged-candidate")
	}
	if *checkpointPath != "" && !*allowUntagged {
		return fmt.Errorf("--checkpoint is allowed only with --allow-untagged-candidate")
	}
	if *statementPath == "" {
		*statementPath = filepath.Join(*repo, "forms", "revocations", version+".json")
	}
	if *checkpointPath == "" {
		*checkpointPath = filepath.Join(*repo, "forms", "revocations", "checkpoints", version+".json")
	}
	evidence, err := inspectSource(*repo, *tag, *allowUntagged, *allowDirty)
	if err != nil {
		return err
	}
	revocation, canonicalStatement, checkpoint, canonicalCheckpoint, err := verifyRevocationSourceChain(*statementPath, *checkpointPath)
	if err != nil {
		return err
	}
	if revocation.StatementVersion != version {
		return fmt.Errorf("tag version %q does not match statementVersion %q", version, revocation.StatementVersion)
	}
	if checkpoint.CheckpointVersion != version || checkpoint.Sequence != revocation.Sequence {
		return fmt.Errorf("tag version %q does not match current checkpoint version/sequence", version)
	}
	checkpointDigest, err := formpackage.DigestCanonicalJSON(canonicalCheckpoint)
	if err != nil {
		return err
	}
	base := "takoform-form-revocation_" + version
	manifest := releaseManifest{
		SchemaVersion: 1, ReleaseType: "form-package-revocation", Tag: *tag,
		SourceRepository: sourceRepository, SourceCommit: evidence.commit, ToolingCommit: *toolingCommit,
		Workflow: revokeWorkflow, PackageVersion: version,
		PackageDigest: revocation.PackageDigest, FormRef: revocation.FormRef,
		CheckpointSequence: checkpoint.Sequence, CheckpointDigest: checkpointDigest,
		Canonicalization: canonicalization, SignedSubject: base + "_checkpoint.json",
		SignatureBundle: base + "_checkpoint.sigstore.json", SignatureMediaType: bundleMediaType,
		PublisherPolicy:  publisherPolicy(revokeWorkflow, "refs/tags/forms/revocations/v*", *toolingCommit),
		PublicationReady: false, PublicationBlockers: evidence.blockers,
	}
	if err := createOutput(*outputDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outputDir, manifest.SignedSubject), canonicalCheckpoint, 0o644); err != nil {
		return err
	}
	checkpointAsset, err := describeAsset(filepath.Join(*outputDir, manifest.SignedSubject), manifest.SignedSubject, "application/vnd.takoform.form-package-revocation-checkpoint.v1+json")
	if err != nil {
		return err
	}
	statementName := base + "_statement.json"
	if err := os.WriteFile(filepath.Join(*outputDir, statementName), canonicalStatement, 0o644); err != nil {
		return err
	}
	statementAsset, err := describeAsset(filepath.Join(*outputDir, statementName), statementName, "application/vnd.takoform.form-package-revocation.v1+json")
	if err != nil {
		return err
	}
	provenanceName := base + "_provenance.intoto.json"
	provenance := createProvenance(*tag, revokeWorkflow, evidence.commit, *toolingCommit, []releaseAsset{checkpointAsset, statementAsset})
	if err := writeCanonicalJSON(filepath.Join(*outputDir, provenanceName), provenance); err != nil {
		return err
	}
	provenanceAsset, err := describeAsset(filepath.Join(*outputDir, provenanceName), provenanceName, "application/vnd.in-toto+json")
	if err != nil {
		return err
	}
	manifest.Assets = []releaseAsset{checkpointAsset, statementAsset, provenanceAsset}
	if err := writeJSON(filepath.Join(*outputDir, "release-manifest.json"), manifest); err != nil {
		return err
	}
	return writeJSONTo(output, manifest)
}

func runFinalize(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("finalize-bundle", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	outputDir := flags.String("output", "", "release output directory")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *outputDir == "" {
		return usageError()
	}
	manifestPath := filepath.Join(*outputDir, "release-manifest.json")
	var manifest releaseManifest
	if err := readStrictJSON(manifestPath, &manifest); err != nil {
		return err
	}
	bundlePath := filepath.Join(*outputDir, manifest.SignatureBundle)
	if err := validateSigstoreBundle(bundlePath); err != nil {
		return err
	}
	bundle, err := describeAsset(bundlePath, manifest.SignatureBundle, bundleMediaType)
	if err != nil {
		return err
	}
	for _, asset := range manifest.Assets {
		if asset.Name == bundle.Name {
			return fmt.Errorf("signature bundle is already finalized")
		}
	}
	manifest.Assets = append(manifest.Assets, bundle)
	sort.Slice(manifest.Assets, func(i, j int) bool { return manifest.Assets[i].Name < manifest.Assets[j].Name })
	manifest.PublicationReady = len(manifest.PublicationBlockers) == 0
	if err := writeJSON(manifestPath, manifest); err != nil {
		return err
	}
	if err := writeChecksums(*outputDir); err != nil {
		return err
	}
	return writeJSONTo(output, manifest)
}

func runCheckRevocations(arguments []string) error {
	flags := flag.NewFlagSet("check-revocations", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repo := flags.String("repo", ".", "repository root")
	base := flags.String("base", "", "base commit")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *base == "" {
		return usageError()
	}
	changed, err := git(*repo, "diff", "--name-status", "--find-renames", *base+"...HEAD", "--", "forms/revocations")
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(strings.NewReader(changed))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		for _, path := range fields[1:] {
			if revocationPath.MatchString(filepath.ToSlash(path)) && status != "A" {
				return fmt.Errorf("released revocation statements are append-only; %s has git status %s", path, status)
			}
		}
	}
	return scanner.Err()
}

type sourceEvidence struct {
	commit     string
	commitTime time.Time
	blockers   []string
}

func inspectSource(repo, tag string, allowUntagged, allowDirty bool) (sourceEvidence, error) {
	commit, err := git(repo, "rev-parse", "HEAD")
	if err != nil {
		return sourceEvidence{}, err
	}
	dirty, err := git(repo, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return sourceEvidence{}, err
	}
	blockers := []string{}
	if strings.TrimSpace(dirty) != "" {
		if !allowDirty {
			return sourceEvidence{}, fmt.Errorf("release source tree is dirty")
		}
		blockers = append(blockers, "source tree is dirty")
	}
	tagCommit, tagErr := git(repo, "rev-list", "-n", "1", tag)
	if tagErr != nil || strings.TrimSpace(tagCommit) != strings.TrimSpace(commit) {
		if !allowUntagged {
			return sourceEvidence{}, fmt.Errorf("exact release tag %s is not attached to HEAD", tag)
		}
		blockers = append(blockers, "exact release tag is not attached to HEAD")
	}
	timestamp, err := git(repo, "show", "-s", "--format=%cI", strings.TrimSpace(commit))
	if err != nil {
		return sourceEvidence{}, err
	}
	commitTime, err := time.Parse(time.RFC3339, strings.TrimSpace(timestamp))
	if err != nil {
		return sourceEvidence{}, fmt.Errorf("parse source commit timestamp: %w", err)
	}
	return sourceEvidence{commit: strings.TrimSpace(commit), commitTime: commitTime.UTC(), blockers: blockers}, nil
}

func publisherPolicy(workflow, tagPattern, toolingCommit string) publisherPolicyEvidence {
	return publisherPolicyEvidence{
		OIDCIssuer: "https://token.actions.githubusercontent.com",
		Identity:   "https://" + sourceRepository + "/" + workflow + "@refs/heads/main",
		TagPattern: tagPattern, ToolingCommit: toolingCommit,
	}
}

type revocationSourceEntry struct {
	statement formpackage.RevocationStatement
	canonical []byte
}

type checkpointSourceEntry struct {
	checkpoint formpackage.RevocationCheckpoint
	canonical  []byte
}

// verifyRevocationSourceChain closes the complete repository-backed statement
// and checkpoint history. A publisher cannot omit, reorder, rewrite, or fork an
// earlier revocation while still producing a valid current release.
func verifyRevocationSourceChain(statementPath, checkpointPath string) (formpackage.RevocationStatement, []byte, formpackage.RevocationCheckpoint, []byte, error) {
	statements := map[uint64]revocationSourceEntry{}
	statementEntries, err := os.ReadDir(filepath.Dir(statementPath))
	if err != nil {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
	}
	for _, entry := range statementEntries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(filepath.Dir(statementPath), entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
		}
		statement, err := formpackage.ValidateRevocationStatement(raw)
		if err != nil {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("validate %s: %w", path, err)
		}
		if entry.Name() != statement.StatementVersion+".json" {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("revocation statement %s must be named %s.json", path, statement.StatementVersion)
		}
		if _, exists := statements[statement.Sequence]; exists {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("duplicate revocation statement sequence %d", statement.Sequence)
		}
		canonical, err := formpackage.Canonicalize(raw)
		if err != nil {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
		}
		statements[statement.Sequence] = revocationSourceEntry{statement: statement, canonical: canonical}
	}

	checkpoints := map[uint64]checkpointSourceEntry{}
	checkpointEntries, err := os.ReadDir(filepath.Dir(checkpointPath))
	if err != nil {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
	}
	for _, entry := range checkpointEntries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(filepath.Dir(checkpointPath), entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
		}
		checkpoint, err := formpackage.ValidateRevocationCheckpoint(raw)
		if err != nil {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("validate %s: %w", path, err)
		}
		if entry.Name() != checkpoint.CheckpointVersion+".json" {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("revocation checkpoint %s must be named %s.json", path, checkpoint.CheckpointVersion)
		}
		if _, exists := checkpoints[checkpoint.Sequence]; exists {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("duplicate revocation checkpoint sequence %d", checkpoint.Sequence)
		}
		canonical, err := formpackage.Canonicalize(raw)
		if err != nil {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
		}
		checkpoints[checkpoint.Sequence] = checkpointSourceEntry{checkpoint: checkpoint, canonical: canonical}
	}

	selectedStatementRaw, err := os.ReadFile(statementPath)
	if err != nil {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
	}
	selectedStatement, err := formpackage.ValidateRevocationStatement(selectedStatementRaw)
	if err != nil {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
	}
	selectedStatementCanonical, err := formpackage.Canonicalize(selectedStatementRaw)
	if err != nil {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
	}
	selectedCheckpointRaw, err := os.ReadFile(checkpointPath)
	if err != nil {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
	}
	selectedCheckpoint, err := formpackage.ValidateRevocationCheckpoint(selectedCheckpointRaw)
	if err != nil {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
	}
	selectedCheckpointCanonical, err := formpackage.Canonicalize(selectedCheckpointRaw)
	if err != nil {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
	}
	sequence := selectedStatement.Sequence
	if selectedCheckpoint.Sequence != sequence || uint64(len(statements)) != sequence || uint64(len(checkpoints)) != sequence {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("revocation source must contain exactly sequences 1 through %d for both statements and checkpoints", sequence)
	}

	var previousCheckpointDigest string
	for current := uint64(1); current <= sequence; current++ {
		_, ok := statements[current]
		if !ok {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("missing revocation statement sequence %d", current)
		}
		checkpointEntry, ok := checkpoints[current]
		if !ok {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("missing revocation checkpoint sequence %d", current)
		}
		if current == 1 {
			if checkpointEntry.checkpoint.PreviousCheckpointDigest != nil {
				return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("revocation checkpoint sequence 1 cannot have a predecessor")
			}
		} else if checkpointEntry.checkpoint.PreviousCheckpointDigest == nil || *checkpointEntry.checkpoint.PreviousCheckpointDigest != previousCheckpointDigest {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("revocation checkpoint sequence %d does not extend sequence %d", current, current-1)
		}
		for index := uint64(1); index <= current; index++ {
			priorStatement := statements[index]
			statementDigest, err := formpackage.DigestCanonicalJSON(priorStatement.canonical)
			if err != nil {
				return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
			}
			checkpointIndex := checkpointEntry.checkpoint.Entries[index-1]
			if checkpointIndex.Sequence != index || checkpointIndex.StatementVersion != priorStatement.statement.StatementVersion ||
				checkpointIndex.StatementDigest != statementDigest || checkpointIndex.PackageDigest != priorStatement.statement.PackageDigest ||
				!reflect.DeepEqual(checkpointIndex.FormRef, priorStatement.statement.FormRef) {
				return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("revocation checkpoint sequence %d does not exactly commit statement sequence %d", current, index)
			}
		}
		previousCheckpointDigest, err = formpackage.DigestCanonicalJSON(checkpointEntry.canonical)
		if err != nil {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
		}
	}

	selectedStatementEntry, ok := statements[sequence]
	if !ok || !bytes.Equal(selectedStatementEntry.canonical, selectedStatementCanonical) {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("selected revocation statement is not the current source entry")
	}
	selectedCheckpointEntry, ok := checkpoints[sequence]
	if !ok || !bytes.Equal(selectedCheckpointEntry.canonical, selectedCheckpointCanonical) {
		return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, fmt.Errorf("selected revocation checkpoint is not the current source entry")
	}
	return selectedStatement, selectedStatementEntry.canonical, selectedCheckpoint, selectedCheckpointEntry.canonical, nil
}

func releaseIDForKind(kind string) (string, error) {
	if !kindPattern.MatchString(kind) {
		return "", fmt.Errorf("kind %q is outside the FormRef ASCII identity grammar", kind)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(kind))
	return "k-" + strings.ToLower(encoded), nil
}

func kindFromReleaseID(releaseID string) (string, error) {
	if !strings.HasPrefix(releaseID, "k-") || len(releaseID) < 4 || len(releaseID) > 105 {
		return "", fmt.Errorf("release id %q is outside k-<lowercase-base32-kind>", releaseID)
	}
	encoded := strings.TrimPrefix(releaseID, "k-")
	if strings.ToLower(encoded) != encoded {
		return "", fmt.Errorf("release id %q must be lowercase", releaseID)
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(encoded))
	if err != nil {
		return "", fmt.Errorf("decode release id %q: %w", releaseID, err)
	}
	kind := string(raw)
	canonical, err := releaseIDForKind(kind)
	if err != nil || canonical != releaseID {
		return "", fmt.Errorf("release id %q is not the canonical encoding of a FormRef kind", releaseID)
	}
	return kind, nil
}

func createOutput(path string) error {
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return fmt.Errorf("output path %s already exists", path)
		}
		return err
	}
	return os.MkdirAll(path, 0o755)
}

func writePackageArchive(output, packageDir string, canonicalIndex []byte, files []formpackage.PackageFile) error {
	handle, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	failed := true
	defer func() {
		handle.Close()
		if failed {
			_ = os.Remove(output)
		}
	}()
	gzipWriter, err := gzip.NewWriterLevel(handle, gzip.BestCompression)
	if err != nil {
		return err
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	entries := append([]formpackage.PackageFile(nil), files...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	if err := writeTarFile(tarWriter, formpackage.PackageIndexFilename, canonicalIndex); err != nil {
		return err
	}
	for _, file := range entries {
		raw, err := os.ReadFile(filepath.Join(packageDir, filepath.FromSlash(file.Path)))
		if err != nil {
			return err
		}
		if formpackage.DigestBytes(raw) != file.Digest || int64(len(raw)) != file.Size {
			return fmt.Errorf("payload %s changed after package verification", file.Path)
		}
		if err := writeTarFile(tarWriter, file.Path, raw); err != nil {
			return err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return err
	}
	if err := gzipWriter.Close(); err != nil {
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}
	failed = false
	return nil
}

func writeTarFile(writer *tar.Writer, name string, raw []byte) error {
	header := &tar.Header{
		Name: name, Mode: 0o644, Size: int64(len(raw)), ModTime: time.Unix(0, 0).UTC(),
		AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(), Format: tar.FormatPAX,
	}
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	_, err := writer.Write(raw)
	return err
}

type namedMedia struct{ name, mediaType string }

func describeAssets(root string, values []namedMedia) ([]releaseAsset, error) {
	result := make([]releaseAsset, 0, len(values))
	for _, value := range values {
		asset, err := describeAsset(filepath.Join(root, value.name), value.name, value.mediaType)
		if err != nil {
			return nil, err
		}
		result = append(result, asset)
	}
	return result, nil
}

func describeAsset(path, name, mediaType string) (releaseAsset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return releaseAsset{}, err
	}
	return releaseAsset{Name: name, MediaType: mediaType, Size: int64(len(raw)), Digest: formpackage.DigestBytes(raw)}, nil
}

func createPackageSBOM(index formpackage.PackageIndex, report formpackage.VerificationReport, created time.Time, canonicalIndex []byte, packageDir string) (map[string]any, error) {
	files := []map[string]any{}
	fileIDs := []string{}
	verificationDigests := []string{}
	appendFile := func(name string, raw []byte) {
		sha256Value := sha256.Sum256(raw)
		sha1Value := sha1.Sum(raw)
		fileID := "SPDXRef-File-" + spdxID(name) + "-" + hex.EncodeToString(sha256Value[:6])
		verificationDigests = append(verificationDigests, hex.EncodeToString(sha1Value[:]))
		fileIDs = append(fileIDs, fileID)
		files = append(files, map[string]any{
			"fileName":         "./" + name,
			"SPDXID":           fileID,
			"checksums":        []map[string]string{{"algorithm": "SHA256", "checksumValue": hex.EncodeToString(sha256Value[:])}},
			"licenseConcluded": "NOASSERTION", "licenseInfoInFiles": []string{"NOASSERTION"}, "copyrightText": "NOASSERTION",
		})
	}
	appendFile(formpackage.PackageIndexFilename, canonicalIndex)
	for _, file := range index.Files {
		raw, err := os.ReadFile(filepath.Join(packageDir, filepath.FromSlash(file.Path)))
		if err != nil {
			return nil, err
		}
		appendFile(file.Path, raw)
	}
	sort.Strings(verificationDigests)
	verificationInput := strings.Join(verificationDigests, "")
	verificationCode := sha1.Sum([]byte(verificationInput))
	relationships := []map[string]string{{"spdxElementId": "SPDXRef-DOCUMENT", "relationshipType": "DESCRIBES", "relatedSpdxElement": "SPDXRef-Package"}}
	for _, fileID := range fileIDs {
		relationships = append(relationships, map[string]string{
			"spdxElementId": "SPDXRef-Package", "relationshipType": "CONTAINS", "relatedSpdxElement": fileID,
		})
	}
	return map[string]any{
		"spdxVersion": "SPDX-2.3", "dataLicense": "CC0-1.0", "SPDXID": "SPDXRef-DOCUMENT",
		"name":              "Takoform Form Package " + index.FormRef.Kind + " " + index.PackageVersion,
		"documentNamespace": "https://forms.takoform.com/spdx/package/" + strings.TrimPrefix(report.PackageDigest, "sha256:"),
		"creationInfo":      map[string]any{"creators": []string{"Tool: takoform-form-package-release"}, "created": created.Format(time.RFC3339)},
		"packages": []map[string]any{{
			"name": index.FormRef.Kind, "SPDXID": "SPDXRef-Package", "versionInfo": index.PackageVersion,
			"downloadLocation": "NOASSERTION", "filesAnalyzed": true,
			"packageVerificationCode": map[string]string{"packageVerificationCodeValue": hex.EncodeToString(verificationCode[:])},
			"licenseConcluded":        "NOASSERTION", "licenseDeclared": "NOASSERTION", "copyrightText": "NOASSERTION",
		}},
		"files": files, "relationships": relationships,
	}, nil
}

func createProvenance(tag, workflow, sourceCommit, toolingCommit string, assets []releaseAsset) statement {
	subjects := make([]statementSubject, 0, len(assets))
	for _, asset := range assets {
		subjects = append(subjects, statementSubject{Name: asset.Name, Digest: map[string]string{"sha256": strings.TrimPrefix(asset.Digest, "sha256:")}})
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].Name < subjects[j].Name })
	return statement{
		Type: "https://in-toto.io/Statement/v1", Subject: subjects, PredicateType: "https://slsa.dev/provenance/v1",
		Predicate: map[string]any{
			"buildDefinition": map[string]any{
				"buildType":          "https://forms.takoform.com/buildtypes/data-release/v1",
				"externalParameters": map[string]string{"tag": tag},
				"internalParameters": map[string]string{"canonicalization": canonicalization},
				"resolvedDependencies": []map[string]any{
					{"name": "tagged-release-source", "uri": "git+https://" + sourceRepository, "digest": map[string]string{"gitCommit": sourceCommit}},
					{"name": "protected-main-release-tooling", "uri": "git+https://" + sourceRepository, "digest": map[string]string{"gitCommit": toolingCommit}},
				},
			},
			"runDetails": map[string]any{"builder": map[string]string{"id": "https://" + sourceRepository + "/" + workflow + "@" + toolingCommit}},
		},
	}
}

func spdxID(value string) string {
	var builder strings.Builder
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsDigit(current) || current == '.' || current == '-' {
			builder.WriteRune(current)
		} else {
			builder.WriteRune('-')
		}
	}
	return builder.String()
}

func validateSigstoreBundle(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return fmt.Errorf("Sigstore bundle is not I-JSON: %w", err)
	}
	var bundle map[string]any
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return fmt.Errorf("decode Sigstore bundle: %w", err)
	}
	// Bundle v0.3 is the JSON encoding of dev.sigstore.bundle.v1.Bundle.
	// Its content oneof is encoded as a top-level messageSignature or
	// dsseEnvelope field, not beneath a synthetic content object.
	verification, verificationOK := bundle["verificationMaterial"].(map[string]any)
	tlog, tlogOK := verification["tlogEntries"].([]any)
	_, signatureOK := bundle["messageSignature"].(map[string]any)
	if bundle["mediaType"] != bundleMediaType || !verificationOK || !tlogOK || len(tlog) == 0 || !signatureOK {
		return fmt.Errorf("Sigstore bundle lacks v0.3 message signature or transparency-log inclusion evidence")
	}
	return nil
}

func writeChecksums(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "SHA256SUMS" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	var result strings.Builder
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		digest := sha256.Sum256(raw)
		fmt.Fprintf(&result, "%s  %s\n", hex.EncodeToString(digest[:]), name)
	}
	return os.WriteFile(filepath.Join(root, "SHA256SUMS"), []byte(result.String()), 0o644)
}

func writeJSON(path string, value any) error {
	handle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if err := writeJSONTo(handle, value); err != nil {
		handle.Close()
		return err
	}
	return handle.Close()
}

func writeCanonicalJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, canonical, 0o644)
}

func writeJSONTo(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func readStrictJSON(path string, destination any) error {
	handle, err := os.Open(path)
	if err != nil {
		return err
	}
	defer handle.Close()
	decoder := json.NewDecoder(handle)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON or parse error: %w", err)
	}
	return nil
}

func git(repo string, arguments ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", repo}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(arguments, " "), strings.TrimSpace(string(output)), err)
	}
	return string(output), nil
}
