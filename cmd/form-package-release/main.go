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
	"encoding/base64"
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

	sigstoreroot "github.com/sigstore/sigstore-go/pkg/root"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	canonicalization = "RFC8785"
	sourceRepository = "github.com/tako0614/terraform-provider-takoform"
	packageWorkflow  = ".github/workflows/form-package-release.yml"
	revokeWorkflow   = ".github/workflows/form-package-revocation.yml"
	bundleMediaType  = "application/vnd.dev.sigstore.bundle.v0.3+json"
	trustedRootPath  = "admission/v4/trust/trusted-root.json"

	revocationVerificationFormat = "takoform.form-package-revocation-directory-verification@v1"
	maxRevocationAssetBytes      = 64 << 20
	maxTrustedRootBytes          = 4 << 20
)

var (
	packageTagPattern    = regexp.MustCompile(`^forms/(k-[a-z2-7]{2,103})/v(` + semverPattern + `)$`)
	revocationTagPattern = regexp.MustCompile(`^forms/revocations/v(` + semverPattern + `)$`)
	revocationPath       = regexp.MustCompile(`^forms/revocations(?:/checkpoints)?/[0-9A-Za-z.+-]+\.json$`)
	kindPattern          = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,63}$`)
	commitPattern        = regexp.MustCompile(`^[0-9a-f]{40}$`)
	gitObjectPattern     = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
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

type revocationDirectoryVerification struct {
	Format              string                        `json:"format"`
	SemanticStatus      string                        `json:"semanticStatus"`
	CryptographicStatus string                        `json:"cryptographicStatus"`
	Tag                 string                        `json:"tag"`
	Version             string                        `json:"version"`
	SourceCommit        string                        `json:"sourceCommit"`
	ToolingCommit       string                        `json:"toolingCommit"`
	CheckpointSequence  uint64                        `json:"checkpointSequence"`
	CheckpointDigest    string                        `json:"checkpointDigest"`
	PackageDigest       string                        `json:"packageDigest"`
	FormRef             formpackage.FormRef           `json:"formRef"`
	TrustedRoot         revocationVerificationFile    `json:"trustedRoot"`
	Assets              []revocationVerificationAsset `json:"assets"`
}

type revocationVerificationFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type revocationVerificationAsset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
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
	case "check-revocations":
		return runCheckRevocations(arguments[1:])
	case "verify-revocation-directory":
		return runVerifyRevocationDirectory(arguments[1:], output)
	default:
		return usageError()
	}
}

func usageError() error {
	return errors.New("usage: form-package-release <build-package|build-revocation|finalize-bundle|check-revocations|verify-revocation-directory> [options]")
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
	policy := publisherPolicy(workflow, "refs/tags/forms/k-*/v*", *toolingCommit)
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

func runVerifyRevocationDirectory(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("verify-revocation-directory", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	assetRoot := flags.String("asset-root", "", "directory containing exactly one revocation release's six assets")
	sourceRoot := flags.String("source-root", "", "repository root containing the exact revocation source chain")
	tag := flags.String("tag", "", "exact forms/revocations/v<semver> tag")
	sourceCommit := flags.String("source-commit", "", "exact 40-character revocation source commit")
	toolingCommit := flags.String("tooling-commit", "", "exact 40-character protected-main tooling commit")
	trustedRoot := flags.String("trusted-root", "", "reviewed repository-retained Sigstore trusted-root JSON")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 ||
		*assetRoot == "" || *sourceRoot == "" || *tag == "" || *trustedRoot == "" ||
		!commitPattern.MatchString(*sourceCommit) || !commitPattern.MatchString(*toolingCommit) {
		return usageError()
	}
	report, err := verifyRevocationDirectory(
		*sourceRoot, *assetRoot, *tag, *sourceCommit, *toolingCommit, *trustedRoot,
	)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return err
	}
	if _, err := output.Write(canonical); err != nil {
		return err
	}
	_, err = output.Write([]byte{'\n'})
	return err
}

func verifyRevocationDirectory(
	sourceRoot, assetRoot, tag, sourceCommit, toolingCommit, trustedRoot string,
) (revocationDirectoryVerification, error) {
	matches := revocationTagPattern.FindStringSubmatch(tag)
	if matches == nil || strings.TrimSpace(tag) != tag {
		return revocationDirectoryVerification{}, fmt.Errorf("revocation tag must match forms/revocations/v<semver>")
	}
	version := matches[1]
	for _, identity := range []struct {
		label  string
		commit string
	}{
		{label: "source", commit: sourceCommit},
		{label: "tooling", commit: toolingCommit},
	} {
		resolved, err := git(sourceRoot, "rev-parse", "--verify", identity.commit+"^{commit}")
		if err != nil || strings.TrimSpace(resolved) != identity.commit {
			return revocationDirectoryVerification{}, fmt.Errorf(
				"%s commit %s is not an exact local repository commit", identity.label, identity.commit,
			)
		}
	}
	if _, err := git(sourceRoot, "merge-base", "--is-ancestor", sourceCommit, toolingCommit); err != nil {
		return revocationDirectoryVerification{}, fmt.Errorf("source commit is not an ancestor of tooling commit: %w", err)
	}
	status, err := git(
		sourceRoot, "status", "--porcelain=v1", "--untracked-files=all", "--",
		"forms/revocations",
	)
	if err != nil {
		return revocationDirectoryVerification{}, err
	}
	if strings.TrimSpace(status) != "" {
		return revocationDirectoryVerification{}, fmt.Errorf("revocation source is dirty")
	}
	if _, err := git(
		sourceRoot, "diff", "--quiet", sourceCommit, toolingCommit, "--", "forms/revocations",
	); err != nil {
		return revocationDirectoryVerification{}, fmt.Errorf("revocation source changed between source and tooling commits: %w", err)
	}

	snapshotRoot, err := materializeRevocationSourceAtCommit(sourceRoot, toolingCommit)
	if err != nil {
		return revocationDirectoryVerification{}, fmt.Errorf("materialize reviewed revocation source: %w", err)
	}
	defer os.RemoveAll(snapshotRoot)
	statementPath := filepath.Join(snapshotRoot, "forms", "revocations", version+".json")
	checkpointPath := filepath.Join(snapshotRoot, "forms", "revocations", "checkpoints", version+".json")
	revocation, canonicalStatement, checkpoint, canonicalCheckpoint, err := verifyRevocationSourceChain(statementPath, checkpointPath)
	if err != nil {
		return revocationDirectoryVerification{}, fmt.Errorf("verify reviewed revocation chain at tooling commit: %w", err)
	}
	if revocation.StatementVersion != version ||
		checkpoint.CheckpointVersion != version ||
		checkpoint.Sequence != revocation.Sequence {
		return revocationDirectoryVerification{}, fmt.Errorf("tag version does not match the current statement/checkpoint identity")
	}
	checkpointDigest, err := formpackage.DigestCanonicalJSON(canonicalCheckpoint)
	if err != nil {
		return revocationDirectoryVerification{}, err
	}
	trustedRootAbsolute, trustedRootDigest, err := verifyRetainedTrustedRoot(
		sourceRoot, toolingCommit, trustedRoot,
	)
	if err != nil {
		return revocationDirectoryVerification{}, err
	}
	rawAssets, reportAssets, err := readExactRevocationAssets(assetRoot, version)
	if err != nil {
		return revocationDirectoryVerification{}, err
	}
	if err := validateRevocationAssetClosure(
		rawAssets, version, tag, sourceCommit, toolingCommit,
		revocation, canonicalStatement, checkpoint, canonicalCheckpoint, checkpointDigest,
	); err != nil {
		return revocationDirectoryVerification{}, err
	}
	return revocationDirectoryVerification{
		Format: revocationVerificationFormat, SemanticStatus: "verified",
		CryptographicStatus: "external-required",
		Tag:                 tag, Version: version, SourceCommit: sourceCommit, ToolingCommit: toolingCommit,
		CheckpointSequence: checkpoint.Sequence, CheckpointDigest: checkpointDigest,
		PackageDigest: revocation.PackageDigest, FormRef: revocation.FormRef,
		TrustedRoot: revocationVerificationFile{Path: trustedRootAbsolute, SHA256: trustedRootDigest},
		Assets:      reportAssets,
	}, nil
}

func materializeRevocationSourceAtCommit(sourceRoot, toolingCommit string) (string, error) {
	listing, err := gitBytes(
		sourceRoot, "ls-tree", "-rz", "--full-tree", toolingCommit, "--", "forms/revocations",
	)
	if err != nil {
		return "", err
	}
	root, err := os.MkdirTemp("", "takoform-revocation-source-")
	if err != nil {
		return "", err
	}
	cleanup := func(materializeErr error) (string, error) {
		if removeErr := os.RemoveAll(root); removeErr != nil {
			return "", errors.Join(materializeErr, removeErr)
		}
		return "", materializeErr
	}
	for _, record := range bytes.Split(listing, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		metadata, pathRaw, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return cleanup(fmt.Errorf("malformed revocation Git tree entry"))
		}
		fields := strings.Fields(string(metadata))
		path := string(pathRaw)
		if len(fields) != 3 {
			return cleanup(fmt.Errorf("malformed revocation Git tree metadata for %q", path))
		}
		if !revocationPath.MatchString(path) {
			continue
		}
		if fields[0] != "100644" || fields[1] != "blob" ||
			!gitObjectPattern.MatchString(fields[2]) {
			return cleanup(fmt.Errorf("revocation source %q must be an ordinary non-executable Git blob", path))
		}
		raw, err := gitBytes(sourceRoot, "show", toolingCommit+":"+path)
		if err != nil {
			return cleanup(err)
		}
		destination := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return cleanup(err)
		}
		handle, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return cleanup(err)
		}
		if _, err := handle.Write(raw); err != nil {
			_ = handle.Close()
			return cleanup(err)
		}
		if err := handle.Close(); err != nil {
			return cleanup(err)
		}
	}
	return root, nil
}

func verifyRetainedTrustedRoot(sourceRoot, toolingCommit, providedPath string) (string, string, error) {
	provided, err := filepath.Abs(filepath.Clean(providedPath))
	if err != nil {
		return "", "", fmt.Errorf("resolve trusted root: %w", err)
	}
	info, err := os.Lstat(provided)
	if err != nil {
		return "", "", fmt.Errorf("inspect trusted root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("trusted root must be a regular file, not a symlink")
	}
	realPath, err := filepath.EvalSymlinks(provided)
	if err != nil {
		return "", "", fmt.Errorf("resolve real trusted root: %w", err)
	}
	if filepath.Clean(realPath) != filepath.Clean(provided) {
		return "", "", fmt.Errorf("trusted root path must not traverse symlinks")
	}
	treeEntry, err := gitBytes(sourceRoot, "ls-tree", "-z", toolingCommit, "--", trustedRootPath)
	if err != nil {
		return "", "", fmt.Errorf("inspect trusted root at tooling commit: %w", err)
	}
	record := bytes.TrimSuffix(treeEntry, []byte{0})
	metadata, entryPath, ok := bytes.Cut(record, []byte{'\t'})
	fields := strings.Fields(string(metadata))
	if !ok || string(entryPath) != trustedRootPath || len(fields) != 3 ||
		fields[0] != "100644" || fields[1] != "blob" ||
		!gitObjectPattern.MatchString(fields[2]) {
		return "", "", fmt.Errorf("trusted root at tooling commit must be one exact ordinary Git blob")
	}
	raw, err := gitBytes(sourceRoot, "show", toolingCommit+":"+trustedRootPath)
	if err != nil {
		return "", "", fmt.Errorf("read trusted root at tooling commit: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxTrustedRootBytes {
		return "", "", fmt.Errorf("trusted root at tooling commit size must be within 1..%d bytes", maxTrustedRootBytes)
	}
	if info.Size() <= 0 || info.Size() > maxTrustedRootBytes {
		return "", "", fmt.Errorf("provided trusted root size must be within 1..%d bytes", maxTrustedRootBytes)
	}
	providedRaw, err := os.ReadFile(provided)
	if err != nil {
		return "", "", fmt.Errorf("read provided trusted root: %w", err)
	}
	if int64(len(providedRaw)) != info.Size() {
		return "", "", fmt.Errorf("provided trusted root changed while it was read")
	}
	if _, err := formpackage.Canonicalize(providedRaw); err != nil {
		return "", "", fmt.Errorf("trusted root is not I-JSON: %w", err)
	}
	if _, err := sigstoreroot.NewTrustedRootFromJSON(providedRaw); err != nil {
		return "", "", fmt.Errorf("decode Sigstore trusted root: %w", err)
	}
	if !bytes.Equal(raw, providedRaw) {
		return "", "", fmt.Errorf("trusted-root bytes differ from the exact tooling commit")
	}
	sourceAbsolute, err := filepath.Abs(filepath.Clean(sourceRoot))
	if err != nil {
		return "", "", fmt.Errorf("resolve source root: %w", err)
	}
	sourceAbsolute, err = filepath.EvalSymlinks(sourceAbsolute)
	if err != nil {
		return "", "", fmt.Errorf("resolve real source root: %w", err)
	}
	logicalPath := filepath.Join(sourceAbsolute, filepath.FromSlash(trustedRootPath))
	return logicalPath, formpackage.DigestBytes(providedRaw), nil
}

func readExactRevocationAssets(
	assetRoot, version string,
) (map[string][]byte, []revocationVerificationAsset, error) {
	absolute, err := filepath.Abs(filepath.Clean(assetRoot))
	if err != nil {
		return nil, nil, fmt.Errorf("resolve revocation asset root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect revocation asset root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, nil, fmt.Errorf("revocation asset root must be a real directory, not a symlink")
	}
	realRoot, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve real revocation asset root: %w", err)
	}
	if filepath.Clean(realRoot) != filepath.Clean(absolute) {
		return nil, nil, fmt.Errorf("revocation asset root must not traverse symlinks")
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return nil, nil, fmt.Errorf("read revocation asset root: %w", err)
	}
	if len(entries) != 6 {
		return nil, nil, fmt.Errorf("revocation asset root has %d entries, want exact six-asset closure", len(entries))
	}
	base := "takoform-form-revocation_" + version
	expected := map[string]struct{}{
		"release-manifest.json":            {},
		"SHA256SUMS":                       {},
		base + "_checkpoint.json":          {},
		base + "_checkpoint.sigstore.json": {},
		base + "_provenance.intoto.json":   {},
		base + "_statement.json":           {},
	}
	result := make(map[string][]byte, len(expected))
	report := make([]revocationVerificationAsset, 0, len(expected))
	totalBytes := int64(0)
	for _, entry := range entries {
		if _, ok := expected[entry.Name()]; !ok {
			return nil, nil, fmt.Errorf("revocation asset root contains non-canonical entry %q", entry.Name())
		}
		name := filepath.Join(absolute, entry.Name())
		fileInfo, err := os.Lstat(name)
		if err != nil {
			return nil, nil, fmt.Errorf("inspect revocation asset %q: %w", entry.Name(), err)
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("revocation asset %q must be a regular file, not a symlink", entry.Name())
		}
		if fileInfo.Size() <= 0 || fileInfo.Size() > maxRevocationAssetBytes {
			return nil, nil, fmt.Errorf("revocation asset %q has an invalid size", entry.Name())
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return nil, nil, fmt.Errorf("read revocation asset %q: %w", entry.Name(), err)
		}
		if int64(len(raw)) != fileInfo.Size() {
			return nil, nil, fmt.Errorf("revocation asset %q changed while it was read", entry.Name())
		}
		totalBytes += int64(len(raw))
		if totalBytes > maxRevocationAssetBytes {
			return nil, nil, fmt.Errorf("revocation asset closure exceeds %d bytes", maxRevocationAssetBytes)
		}
		digest := formpackage.DigestBytes(raw)
		result[entry.Name()] = raw
		report = append(report, revocationVerificationAsset{
			Name: entry.Name(), SHA256: digest, Size: int64(len(raw)),
		})
	}
	sort.Slice(report, func(i, j int) bool { return report[i].Name < report[j].Name })
	return result, report, nil
}

func validateRevocationAssetClosure(
	rawAssets map[string][]byte,
	version, tag, sourceCommit, toolingCommit string,
	revocation formpackage.RevocationStatement,
	canonicalStatement []byte,
	checkpoint formpackage.RevocationCheckpoint,
	canonicalCheckpoint []byte,
	checkpointDigest string,
) error {
	base := "takoform-form-revocation_" + version
	checkpointName := base + "_checkpoint.json"
	bundleName := base + "_checkpoint.sigstore.json"
	provenanceName := base + "_provenance.intoto.json"
	statementName := base + "_statement.json"
	manifestRaw := rawAssets["release-manifest.json"]
	var manifest releaseManifest
	if err := decodeStrictJSONBytes(manifestRaw, &manifest); err != nil {
		return fmt.Errorf("decode revocation release manifest: %w", err)
	}
	wantPolicy := publisherPolicy(revokeWorkflow, "refs/tags/forms/revocations/v*", toolingCommit)
	if manifest.SchemaVersion != 1 || manifest.ReleaseType != "form-package-revocation" ||
		manifest.Tag != tag || manifest.SourceRepository != sourceRepository ||
		manifest.SourceCommit != sourceCommit || manifest.ToolingCommit != toolingCommit ||
		manifest.Workflow != revokeWorkflow || manifest.PackageVersion != version ||
		manifest.ReleaseID != "" || manifest.PackageDigest != revocation.PackageDigest ||
		!reflect.DeepEqual(manifest.FormRef, revocation.FormRef) ||
		manifest.CheckpointSequence != checkpoint.Sequence || manifest.CheckpointDigest != checkpointDigest ||
		manifest.Canonicalization != canonicalization || manifest.SignedSubject != checkpointName ||
		manifest.SignatureBundle != bundleName || manifest.SignatureMediaType != bundleMediaType ||
		manifest.PublisherPolicy != wantPolicy || !manifest.PublicationReady ||
		len(manifest.PublicationBlockers) != 0 {
		return fmt.Errorf("revocation release manifest does not bind the exact source, tag, commits, checkpoint, and current publisher")
	}
	wantMedia := map[string]string{
		checkpointName: "application/vnd.takoform.form-package-revocation-checkpoint.v1+json",
		bundleName:     bundleMediaType,
		provenanceName: "application/vnd.in-toto+json",
		statementName:  "application/vnd.takoform.form-package-revocation.v1+json",
	}
	if len(manifest.Assets) != len(wantMedia) {
		return fmt.Errorf("revocation manifest asset closure has %d entries, want exactly 4", len(manifest.Assets))
	}
	manifestAssets := make(map[string]releaseAsset, len(manifest.Assets))
	lastName := ""
	for position, asset := range manifest.Assets {
		mediaType, ok := wantMedia[asset.Name]
		raw, exists := rawAssets[asset.Name]
		if !ok || !exists || asset.MediaType != mediaType || asset.Size != int64(len(raw)) ||
			asset.Digest != formpackage.DigestBytes(raw) {
			return fmt.Errorf("revocation manifest asset %q does not bind exact name, media type, size, and digest", asset.Name)
		}
		if position > 0 && asset.Name <= lastName {
			return fmt.Errorf("revocation manifest assets are not in strict canonical name order")
		}
		if _, duplicate := manifestAssets[asset.Name]; duplicate {
			return fmt.Errorf("revocation manifest duplicates asset %q", asset.Name)
		}
		manifestAssets[asset.Name] = asset
		lastName = asset.Name
	}
	if err := validateRevocationChecksums(rawAssets["SHA256SUMS"], manifestRaw, manifestAssets); err != nil {
		return err
	}
	if !bytes.Equal(rawAssets[checkpointName], canonicalCheckpoint) {
		return fmt.Errorf("released checkpoint differs from the exact canonical repository checkpoint")
	}
	if !bytes.Equal(rawAssets[statementName], canonicalStatement) {
		return fmt.Errorf("released statement differs from the exact canonical repository statement")
	}
	expectedProvenance := createProvenance(
		tag, revokeWorkflow, sourceCommit, toolingCommit,
		[]releaseAsset{manifestAssets[checkpointName], manifestAssets[statementName]},
	)
	expectedRaw, err := json.Marshal(expectedProvenance)
	if err != nil {
		return err
	}
	expectedRaw, err = formpackage.Canonicalize(expectedRaw)
	if err != nil {
		return err
	}
	provenanceRaw := rawAssets[provenanceName]
	if canonical, err := formpackage.Canonicalize(provenanceRaw); err != nil ||
		!bytes.Equal(provenanceRaw, canonical) || !bytes.Equal(provenanceRaw, expectedRaw) {
		return fmt.Errorf("revocation provenance does not exactly bind checkpoint, statement, tag, source/tooling commits, and workflow")
	}
	if err := validateRevocationSigstoreBundle(rawAssets[bundleName], rawAssets[checkpointName]); err != nil {
		return fmt.Errorf("revocation Sigstore bundle: %w", err)
	}
	return nil
}

func validateRevocationChecksums(raw, manifestRaw []byte, assets map[string]releaseAsset) error {
	if len(raw) == 0 || !bytes.HasSuffix(raw, []byte("\n")) {
		return fmt.Errorf("revocation SHA256SUMS must be non-empty and newline terminated")
	}
	expected := map[string]string{"release-manifest.json": formpackage.DigestBytes(manifestRaw)}
	for name, asset := range assets {
		expected[name] = asset.Digest
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != len(expected) || len(lines) != 5 {
		return fmt.Errorf("revocation SHA256SUMS closure has %d lines, want exactly 5", len(lines))
	}
	lastName := ""
	seen := make(map[string]struct{}, len(lines))
	for position, line := range lines {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || len(parts[0]) != 64 || filepath.Base(parts[1]) != parts[1] {
			return fmt.Errorf("invalid revocation SHA256SUMS line %q", line)
		}
		if position > 0 && parts[1] <= lastName {
			return fmt.Errorf("revocation SHA256SUMS is not in strict canonical name order")
		}
		if want, ok := expected[parts[1]]; !ok || "sha256:"+parts[0] != want {
			return fmt.Errorf("revocation SHA256SUMS does not bind %q to the release manifest", parts[1])
		}
		if _, duplicate := seen[parts[1]]; duplicate {
			return fmt.Errorf("revocation SHA256SUMS duplicates %q", parts[1])
		}
		seen[parts[1]] = struct{}{}
		lastName = parts[1]
	}
	return nil
}

func validateRevocationSigstoreBundle(raw, subject []byte) error {
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return fmt.Errorf("bundle is not I-JSON: %w", err)
	}
	var bundle map[string]any
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return fmt.Errorf("decode bundle: %w", err)
	}
	if bundle["mediaType"] != bundleMediaType {
		return fmt.Errorf("bundle mediaType is not Sigstore v0.3")
	}
	verification, ok := bundle["verificationMaterial"].(map[string]any)
	if !ok {
		return fmt.Errorf("bundle lacks verificationMaterial")
	}
	certificate, ok := verification["certificate"].(map[string]any)
	if !ok {
		return fmt.Errorf("bundle lacks signing certificate")
	}
	if _, err := decodeRequiredBase64(certificate["rawBytes"]); err != nil {
		return fmt.Errorf("bundle signing certificate: %w", err)
	}
	tlogEntries, ok := verification["tlogEntries"].([]any)
	if !ok || len(tlogEntries) == 0 {
		return fmt.Errorf("bundle lacks transparency-log entries")
	}
	for position, value := range tlogEntries {
		entry, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("bundle transparency-log entry %d is not an object", position)
		}
		if _, err := decodeRequiredBase64(entry["canonicalizedBody"]); err != nil {
			return fmt.Errorf("bundle transparency-log entry %d body: %w", position, err)
		}
		proof, ok := entry["inclusionProof"].(map[string]any)
		if !ok || stringJSONField(proof, "logIndex") == "" || stringJSONField(proof, "treeSize") == "" {
			return fmt.Errorf("bundle transparency-log entry %d lacks inclusion proof identity", position)
		}
		rootHash, err := decodeRequiredBase64(proof["rootHash"])
		if err != nil || len(rootHash) != sha256.Size {
			return fmt.Errorf("bundle transparency-log entry %d has invalid rootHash", position)
		}
		checkpointValue, ok := proof["checkpoint"].(map[string]any)
		if !ok || stringJSONField(checkpointValue, "envelope") == "" {
			return fmt.Errorf("bundle transparency-log entry %d lacks checkpoint", position)
		}
		hashes, ok := proof["hashes"].([]any)
		if !ok {
			return fmt.Errorf("bundle transparency-log entry %d lacks inclusion hashes", position)
		}
		for hashPosition, encoded := range hashes {
			hash, err := decodeRequiredBase64(encoded)
			if err != nil || len(hash) != sha256.Size {
				return fmt.Errorf("bundle transparency-log entry %d hash %d is invalid", position, hashPosition)
			}
		}
	}
	messageSignature, ok := bundle["messageSignature"].(map[string]any)
	if !ok {
		return fmt.Errorf("bundle lacks messageSignature")
	}
	messageDigest, ok := messageSignature["messageDigest"].(map[string]any)
	if !ok || stringJSONField(messageDigest, "algorithm") != "SHA2_256" {
		return fmt.Errorf("bundle message digest algorithm is not SHA2_256")
	}
	boundDigest, err := decodeRequiredBase64(messageDigest["digest"])
	if err != nil {
		return fmt.Errorf("bundle message digest: %w", err)
	}
	expectedDigest := sha256.Sum256(subject)
	if !bytes.Equal(boundDigest, expectedDigest[:]) {
		return fmt.Errorf("bundle message digest does not bind the signed checkpoint")
	}
	if _, err := decodeRequiredBase64(messageSignature["signature"]); err != nil {
		return fmt.Errorf("bundle message signature: %w", err)
	}
	return nil
}

func decodeRequiredBase64(value any) ([]byte, error) {
	encoded, ok := value.(string)
	if !ok || encoded == "" {
		return nil, fmt.Errorf("value is not a non-empty base64 string")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) == 0 {
		return nil, fmt.Errorf("value is not valid non-empty base64")
	}
	return raw, nil
}

func stringJSONField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func decodeStrictJSONBytes(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
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
	for _, directory := range []string{filepath.Dir(statementPath), filepath.Dir(checkpointPath)} {
		info, err := os.Lstat(directory)
		if err != nil {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil,
				fmt.Errorf("revocation source directory %s must be a real directory, not a symlink", directory)
		}
	}
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
		info, err := os.Lstat(path)
		if err != nil {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil,
				fmt.Errorf("revocation statement %s must be a regular file, not a symlink", path)
		}
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
		info, err := os.Lstat(path)
		if err != nil {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return formpackage.RevocationStatement{}, nil, formpackage.RevocationCheckpoint{}, nil,
				fmt.Errorf("revocation checkpoint %s must be a regular file, not a symlink", path)
		}
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
	output, err := gitBytes(repo, arguments...)
	return string(output), err
}

func gitBytes(repo string, arguments ...string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-C", repo}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %s: %w", strings.Join(arguments, " "), strings.TrimSpace(string(output)), err)
	}
	return output, nil
}
