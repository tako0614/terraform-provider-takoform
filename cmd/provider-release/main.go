// Command provider-release builds deterministic, non-publishing Takoform
// provider candidate evidence. It intentionally has no upload or signing
// capability: those require external maintainer trust and custody decisions.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	slsav1 "github.com/in-toto/attestation/go/predicates/provenance/v1"
	intotov1 "github.com/in-toto/attestation/go/v1"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"google.golang.org/protobuf/encoding/protojson"
)

const descriptorPath = "release/version.json"

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$`)

type cliCompatibility struct {
	Product         string `json:"product"`
	Version         string `json:"version"`
	ProviderAddress string `json:"providerAddress"`
}

type descriptor struct {
	SchemaVersion      int                `json:"schemaVersion"`
	Version            string             `json:"version"`
	Tag                string             `json:"tag"`
	SourceRepository   string             `json:"sourceRepository"`
	ProviderAddress    string             `json:"providerAddress"`
	CLIMatrix          []cliCompatibility `json:"cliMatrix"`
	GoModule           string             `json:"goModule"`
	GoVersion          string             `json:"goVersion"`
	SigningFingerprint string             `json:"signingFingerprint"`
	PublicKeyPath      string             `json:"publicKeyPath"`
	Platforms          []string           `json:"platforms"`
	PublicationStatus  string             `json:"publicationStatus"`
}

type sourceEvidence struct {
	Commit               string   `json:"commit"`
	Dirty                bool     `json:"dirty"`
	TagPresent           bool     `json:"tagPresent"`
	TagSigned            bool     `json:"tagSigned"`
	TagSignerFingerprint string   `json:"tagSignerFingerprint,omitempty"`
	GoVersion            string   `json:"goVersion"`
	Blockers             []string `json:"blockers"`
	PublicationReady     bool     `json:"publicationReady"`
}

type artifact struct {
	Platform              string `json:"platform"`
	Archive               string `json:"archive"`
	ArchiveSHA256         string `json:"archiveSha256"`
	ArchiveSize           int64  `json:"archiveSize"`
	Binary                string `json:"binary"`
	BinarySHA256          string `json:"binarySha256"`
	BinarySize            int64  `json:"binarySize"`
	EmbeddedVersionLDFlag string `json:"embeddedVersionLdflag"`
}

type releaseManifest struct {
	SchemaVersion       int                `json:"schemaVersion"`
	Kind                string             `json:"kind"`
	Version             string             `json:"version"`
	Tag                 string             `json:"tag"`
	SourceRepository    string             `json:"sourceRepository"`
	SourceCommit        string             `json:"sourceCommit"`
	SourceDirty         bool               `json:"sourceDirty"`
	ProviderAddress     string             `json:"providerAddress"`
	CLIMatrix           []cliCompatibility `json:"cliMatrix"`
	GoModule            string             `json:"goModule"`
	GoVersion           string             `json:"goVersion"`
	PublicationStatus   string             `json:"publicationStatus"`
	PublicationReady    bool               `json:"publicationReady"`
	PublicationBlockers []string           `json:"publicationBlockers"`
	Artifacts           []artifact         `json:"artifacts"`
	Materials           map[string]string  `json:"materials"`
}

type module struct {
	Path    string  `json:"Path"`
	Version string  `json:"Version"`
	Sum     string  `json:"Sum"`
	Replace *module `json:"Replace"`
}

type spdxDocument struct {
	SPDXVersion       string        `json:"spdxVersion"`
	DataLicense       string        `json:"dataLicense"`
	SPDXID            string        `json:"SPDXID"`
	Name              string        `json:"name"`
	DocumentNamespace string        `json:"documentNamespace"`
	CreationInfo      spdxCreation  `json:"creationInfo"`
	Packages          []spdxPackage `json:"packages"`
}

type spdxCreation struct {
	Creators []string `json:"creators"`
	Created  string   `json:"created"`
}

type spdxPackage struct {
	Name             string `json:"name"`
	SPDXID           string `json:"SPDXID"`
	VersionInfo      string `json:"versionInfo,omitempty"`
	DownloadLocation string `json:"downloadLocation"`
	FilesAnalyzed    bool   `json:"filesAnalyzed"`
}

type statement struct {
	Type          string             `json:"_type"`
	Subject       []statementSubject `json:"subject"`
	PredicateType string             `json:"predicateType"`
	Predicate     statementPredicate `json:"predicate"`
}

type statementSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

type statementPredicate struct {
	BuildDefinition map[string]any `json:"buildDefinition"`
	RunDetails      map[string]any `json:"runDetails"`
}

type signedTagArtifactMetadata struct {
	Format            string `json:"format"`
	Repository        string `json:"repository"`
	WorkflowPath      string `json:"workflowPath"`
	WorkflowRef       string `json:"workflowRef"`
	RunID             string `json:"runId"`
	RunAttempt        string `json:"runAttempt"`
	SourceRef         string `json:"sourceRef"`
	SourceCommit      string `json:"sourceCommit"`
	ReleaseTag        string `json:"releaseTag"`
	ObjectFormat      string `json:"objectFormat"`
	TagObjectOID      string `json:"tagObjectOid"`
	TagObjectSHA256   string `json:"tagObjectSha256"`
	PreflightSHA256   string `json:"preflightSha256"`
	SignerFingerprint string `json:"signerFingerprint"`
}

type signedTagArtifactEvidence struct {
	Kind                 string `json:"kind"`
	ReleaseTag           string `json:"releaseTag"`
	SourceCommit         string `json:"sourceCommit"`
	WorkflowRun          string `json:"workflowRun"`
	PreflightSHA256      string `json:"preflightSha256"`
	TagObjectOID         string `json:"tagObjectOid"`
	TagObjectSHA256      string `json:"tagObjectSha256"`
	SignerFingerprint    string `json:"signerFingerprint"`
	LocalRefMaterialized bool   `json:"localRefMaterialized"`
	Verified             bool   `json:"verified"`
}

func main() {
	if len(os.Args) < 2 {
		fail("usage: provider-release <verify-source|build|verify-reproducible|verify-sbom|registry-checksum-targets|verify-tag-artifact> [options]")
	}
	repo, err := repositoryRoot()
	check(err)
	desc, err := loadDescriptor(repo)
	check(err)
	switch os.Args[1] {
	case "verify-source":
		fs := flag.NewFlagSet("verify-source", flag.ExitOnError)
		allowDirty := fs.Bool("allow-dirty-candidate", false, "allow dirty local evidence; never publication-ready")
		allowUntagged := fs.Bool("allow-untagged-candidate", false, "allow untagged local evidence; never publication-ready")
		expectedTriggerTag := fs.String("expected-trigger-tag", "", "when set, must exactly match the release descriptor tag")
		check(fs.Parse(os.Args[2:]))
		check(verifyExpectedTriggerTag(*expectedTriggerTag, desc.Tag))
		evidence, err := inspectSource(repo, desc, *allowDirty, *allowUntagged)
		check(err)
		writeJSON(os.Stdout, map[string]any{
			"kind":       "takoform.provider-release-source@v1",
			"descriptor": desc,
			"source":     evidence,
		})
	case "build":
		fs := flag.NewFlagSet("build", flag.ExitOnError)
		output := fs.String("output", "", "new output directory")
		allowDirty := fs.Bool("allow-dirty-candidate", false, "allow dirty local evidence; never publication-ready")
		allowUntagged := fs.Bool("allow-untagged-candidate", false, "allow untagged local evidence; never publication-ready")
		check(fs.Parse(os.Args[2:]))
		if strings.TrimSpace(*output) == "" {
			fail("--output is required")
		}
		manifest, err := build(repo, *output, desc, *allowDirty, *allowUntagged)
		check(err)
		writeJSON(os.Stdout, manifest)
	case "verify-reproducible":
		fs := flag.NewFlagSet("verify-reproducible", flag.ExitOnError)
		allowDirty := fs.Bool("allow-dirty-candidate", false, "allow dirty local evidence; never publication-ready")
		allowUntagged := fs.Bool("allow-untagged-candidate", false, "allow untagged local evidence; never publication-ready")
		check(fs.Parse(os.Args[2:]))
		check(verifyReproducible(repo, desc, *allowDirty, *allowUntagged))
		writeJSON(os.Stdout, map[string]any{
			"kind":         "takoform.provider-release-reproducibility@v1",
			"version":      desc.Version,
			"platforms":    desc.Platforms,
			"reproducible": true,
		})
	case "verify-sbom":
		fs := flag.NewFlagSet("verify-sbom", flag.ExitOnError)
		check(fs.Parse(os.Args[2:]))
		verified, err := verifySPDXFiles(repo, fs.Args())
		check(err)
		writeJSON(os.Stdout, map[string]any{
			"kind":     "takoform.provider-release-sbom-verification@v1",
			"schema":   "SPDX-2.3",
			"verified": verified,
		})
	case "registry-checksum-targets":
		fs := flag.NewFlagSet("registry-checksum-targets", flag.ExitOnError)
		product := fs.String("product", "", "exact CLI product: OpenTofu or Terraform")
		check(fs.Parse(os.Args[2:]))
		targets, err := registryChecksumTargets(desc, *product)
		check(err)
		for _, target := range targets {
			fmt.Fprintln(os.Stdout, target)
		}
	case "verify-tag-artifact":
		fs := flag.NewFlagSet("verify-tag-artifact", flag.ExitOnError)
		artifactPath := fs.String("artifact", "", "downloaded provider-signed-tag artifact directory")
		preflightPath := fs.String("preflight-artifact", "", "downloaded provider-tag-preflight artifact directory")
		expectedRunID := fs.String("expected-run-id", "", "exact approved GitHub Actions run id")
		expectedRunAttempt := fs.String("expected-run-attempt", "", "exact approved GitHub Actions run attempt")
		expectedCommit := fs.String("expected-commit", "", "exact protected-main source commit")
		materializeRef := fs.Bool("materialize-ref", false, "atomically create the local descriptor tag ref after verification")
		check(fs.Parse(os.Args[2:]))
		evidence, err := verifySignedTagArtifact(repo, desc, *artifactPath, *preflightPath, *expectedRunID, *expectedRunAttempt, *expectedCommit, *materializeRef)
		check(err)
		writeJSON(os.Stdout, evidence)
	default:
		fail("unknown command: " + os.Args[1])
	}
}

// registryChecksumTargets returns the only files that may be named by the
// provider Registry checksum manifest. GitHub Release evidence such as SPDX
// SBOMs, the Registry metadata manifest, and workflow provenance remains in
// the separately attested release inventory and must never be projected as a
// provider package by either Registry.
func registryChecksumTargets(desc descriptor, product string) ([]string, error) {
	if err := validateCLIMatrix(desc.CLIMatrix); err != nil {
		return nil, err
	}
	found := false
	for _, entry := range desc.CLIMatrix {
		if entry.Product == product {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("unsupported Registry checksum product %q", product)
	}
	targets := make([]string, 0, len(desc.Platforms))
	for _, platform := range desc.Platforms {
		if !regexp.MustCompile(`^[a-z0-9]+_[a-z0-9]+$`).MatchString(platform) {
			return nil, fmt.Errorf("invalid Registry checksum platform %q", platform)
		}
		targets = append(targets, fmt.Sprintf("terraform-provider-takoform_%s_%s.zip", desc.Version, platform))
	}
	sort.Strings(targets)
	return targets, nil
}

func verifySignedTagArtifact(repo string, desc descriptor, artifactPath, preflightPath, expectedRunID, expectedRunAttempt, expectedCommit string, materializeRef bool) (signedTagArtifactEvidence, error) {
	if strings.TrimSpace(artifactPath) == "" || strings.TrimSpace(preflightPath) == "" {
		return signedTagArtifactEvidence{}, errors.New("--artifact and --preflight-artifact are required")
	}
	if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(expectedRunID) || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(expectedRunAttempt) {
		return signedTagArtifactEvidence{}, errors.New("expected run id and attempt must be positive decimal strings")
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(expectedCommit) {
		return signedTagArtifactEvidence{}, errors.New("expected commit must be an exact lowercase 40-character object id")
	}
	if err := verifyClosedChecksums(preflightPath,
		[]string{"SHA256SUMS", "preflight.json", "provider-candidate-manifest.json", "provider-lifecycle-matrix.json", "provider-provenance.json", "provider-sbom.spdx.json"},
		[]string{"preflight.json", "provider-candidate-manifest.json", "provider-lifecycle-matrix.json", "provider-provenance.json", "provider-sbom.spdx.json"}); err != nil {
		return signedTagArtifactEvidence{}, fmt.Errorf("verify preflight artifact: %w", err)
	}
	if err := verifyClosedChecksums(artifactPath, []string{"SHA256SUMS", "metadata.json", "tag-object"}, []string{"metadata.json", "tag-object"}); err != nil {
		return signedTagArtifactEvidence{}, fmt.Errorf("verify signed tag artifact: %w", err)
	}
	var metadata signedTagArtifactMetadata
	if err := readStrictJSONFile(filepath.Join(artifactPath, "metadata.json"), &metadata); err != nil {
		return signedTagArtifactEvidence{}, fmt.Errorf("read signed tag metadata: %w", err)
	}
	expectedWorkflowPath := ".github/workflows/provider-release-tag.yml"
	expectedWorkflowRef := "tako0614/terraform-provider-takoform/" + expectedWorkflowPath + "@refs/heads/main"
	expectedWorkflowRun := "https://github.com/tako0614/terraform-provider-takoform/actions/runs/" + expectedRunID + "/attempts/" + expectedRunAttempt
	if metadata.Format != "takoform.provider-signed-tag-artifact@v1" ||
		metadata.Repository != "tako0614/terraform-provider-takoform" ||
		metadata.WorkflowPath != expectedWorkflowPath || metadata.WorkflowRef != expectedWorkflowRef ||
		metadata.RunID != expectedRunID || metadata.RunAttempt != expectedRunAttempt ||
		metadata.SourceRef != "refs/heads/main" || metadata.SourceCommit != expectedCommit ||
		metadata.ReleaseTag != desc.Tag || metadata.SignerFingerprint != desc.SigningFingerprint {
		return signedTagArtifactEvidence{}, errors.New("signed tag metadata does not match the exact release, source, workflow run, or signer")
	}
	objectFormat, err := command(repo, nil, "git", "rev-parse", "--show-object-format")
	if err != nil {
		return signedTagArtifactEvidence{}, err
	}
	objectFormat = strings.TrimSpace(objectFormat)
	oidLength := 40
	if objectFormat == "sha256" {
		oidLength = 64
	} else if objectFormat != "sha1" {
		return signedTagArtifactEvidence{}, fmt.Errorf("unsupported Git object format %q", objectFormat)
	}
	if metadata.ObjectFormat != objectFormat || !regexp.MustCompile(fmt.Sprintf(`^[0-9a-f]{%d}$`, oidLength)).MatchString(metadata.TagObjectOID) {
		return signedTagArtifactEvidence{}, errors.New("tag object id or object format does not match the local repository")
	}
	sha256Pattern := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	if !sha256Pattern.MatchString(metadata.TagObjectSHA256) || !sha256Pattern.MatchString(metadata.PreflightSHA256) {
		return signedTagArtifactEvidence{}, errors.New("artifact metadata contains an invalid SHA256 digest")
	}
	preflightDigest, _, err := fileDigest(filepath.Join(preflightPath, "SHA256SUMS"))
	if err != nil {
		return signedTagArtifactEvidence{}, err
	}
	if metadata.PreflightSHA256 != "sha256:"+preflightDigest {
		return signedTagArtifactEvidence{}, errors.New("signed preflight digest does not match the downloaded closed preflight inventory")
	}
	tagObjectPath := filepath.Join(artifactPath, "tag-object")
	tagObject, err := os.ReadFile(tagObjectPath)
	if err != nil {
		return signedTagArtifactEvidence{}, err
	}
	tagObjectDigest, _, err := fileDigest(tagObjectPath)
	if err != nil {
		return signedTagArtifactEvidence{}, err
	}
	if metadata.TagObjectSHA256 != "sha256:"+tagObjectDigest {
		return signedTagArtifactEvidence{}, errors.New("tag object digest does not match metadata")
	}
	if err := verifyTagObjectBindings(tagObject, desc.Tag, expectedCommit, metadata.PreflightSHA256, expectedWorkflowRun); err != nil {
		return signedTagArtifactEvidence{}, err
	}
	if _, err := command(repo, nil, "git", "cat-file", "-e", expectedCommit+"^{commit}"); err != nil {
		return signedTagArtifactEvidence{}, fmt.Errorf("expected source commit is unavailable locally: %w", err)
	}
	tagObjectOID, err := commandInput(repo, nil, tagObject, "git", "mktag")
	if err != nil {
		return signedTagArtifactEvidence{}, fmt.Errorf("reconstruct signed tag object: %w", err)
	}
	tagObjectOID = strings.TrimSpace(tagObjectOID)
	if tagObjectOID != metadata.TagObjectOID {
		return signedTagArtifactEvidence{}, fmt.Errorf("reconstructed tag object id %s does not match expected %s", tagObjectOID, metadata.TagObjectOID)
	}
	peeled, err := command(repo, nil, "git", "rev-parse", tagObjectOID+"^{}")
	if err != nil || strings.TrimSpace(peeled) != expectedCommit {
		return signedTagArtifactEvidence{}, errors.New("reconstructed tag object does not peel to the exact protected-main commit")
	}
	gnupgHome, err := os.MkdirTemp("", "takoform-provider-tag-verify-")
	if err != nil {
		return signedTagArtifactEvidence{}, err
	}
	defer os.RemoveAll(gnupgHome)
	if err := os.Chmod(gnupgHome, 0o700); err != nil {
		return signedTagArtifactEvidence{}, err
	}
	verifyEnv := append(os.Environ(), "GNUPGHOME="+gnupgHome)
	if _, err := command(repo, verifyEnv, "gpg", "--batch", "--import", filepath.Join(repo, desc.PublicKeyPath)); err != nil {
		return signedTagArtifactEvidence{}, fmt.Errorf("import pinned provider public key: %w", err)
	}
	var verifyOutput bytes.Buffer
	verify := exec.Command("git", "verify-tag", "--raw", tagObjectOID)
	verify.Dir, verify.Env, verify.Stdout, verify.Stderr = repo, verifyEnv, &verifyOutput, &verifyOutput
	verifyErr := verify.Run()
	signer, err := verifyPinnedTagSigner(verifyOutput.String(), verifyErr, desc.SigningFingerprint)
	if err != nil {
		return signedTagArtifactEvidence{}, err
	}
	if materializeRef {
		zeroOID := strings.Repeat("0", oidLength)
		if _, err := command(repo, nil, "git", "update-ref", "refs/tags/"+desc.Tag, tagObjectOID, zeroOID); err != nil {
			return signedTagArtifactEvidence{}, fmt.Errorf("materialize local release tag ref: %w", err)
		}
	}
	return signedTagArtifactEvidence{
		Kind: "takoform.provider-signed-tag-verification@v1", ReleaseTag: desc.Tag, SourceCommit: expectedCommit,
		WorkflowRun: expectedWorkflowRun, PreflightSHA256: metadata.PreflightSHA256,
		TagObjectOID: tagObjectOID, TagObjectSHA256: metadata.TagObjectSHA256,
		SignerFingerprint: signer, LocalRefMaterialized: materializeRef, Verified: true,
	}, nil
}

func verifyClosedChecksums(root string, expectedFiles, checksumTargets []string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	actualFiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact entry %q is not a regular file", entry.Name())
		}
		actualFiles = append(actualFiles, entry.Name())
	}
	sort.Strings(actualFiles)
	expected := append([]string(nil), expectedFiles...)
	sort.Strings(expected)
	if strings.Join(actualFiles, "\n") != strings.Join(expected, "\n") {
		return fmt.Errorf("closed artifact inventory mismatch: got %v, want %v", actualFiles, expected)
	}
	raw, err := os.ReadFile(filepath.Join(root, "SHA256SUMS"))
	if err != nil {
		return err
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return errors.New("SHA256SUMS must be non-empty and newline terminated")
	}
	wantTargets := append([]string(nil), checksumTargets...)
	sort.Strings(wantTargets)
	actualTargets := make([]string, 0, len(wantTargets))
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if len(line) < 67 || line[64:66] != "  " || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(line[:64]) {
			return fmt.Errorf("invalid checksum line %q", line)
		}
		name := line[66:]
		if name == "" || filepath.Base(name) != name {
			return fmt.Errorf("unsafe checksum target %q", name)
		}
		digest, _, err := fileDigest(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if digest != line[:64] {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
		actualTargets = append(actualTargets, name)
	}
	sort.Strings(actualTargets)
	if strings.Join(actualTargets, "\n") != strings.Join(wantTargets, "\n") {
		return fmt.Errorf("checksum target inventory mismatch: got %v, want %v", actualTargets, wantTargets)
	}
	return nil
}

func verifyTagObjectBindings(raw []byte, releaseTag, sourceCommit, preflightSHA256, workflowRun string) error {
	headerEnd := bytes.Index(raw, []byte("\n\n"))
	signatureStart := bytes.Index(raw, []byte("-----BEGIN PGP SIGNATURE-----"))
	if headerEnd < 0 || signatureStart <= headerEnd+2 {
		return errors.New("tag object does not contain canonical headers, message, and PGP signature")
	}
	headers := strings.Split(string(raw[:headerEnd]), "\n")
	if len(headers) != 4 || headers[0] != "object "+sourceCommit || headers[1] != "type commit" || headers[2] != "tag "+releaseTag ||
		!strings.HasPrefix(headers[3], "tagger Takoform Provider Release <release@takoform.invalid> ") {
		return errors.New("tag object headers do not bind the exact source commit, tag, type, and release identity")
	}
	expectedMessage := fmt.Sprintf("Takoform provider %s\n\nsource-commit: %s\npreflight-sha256: %s\nworkflow-run: %s\n", releaseTag, sourceCommit, preflightSHA256, workflowRun)
	if string(raw[headerEnd+2:signatureStart]) != expectedMessage {
		return errors.New("signed tag message does not bind the exact source, preflight inventory, and workflow run")
	}
	return nil
}

func readStrictJSONFile(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("JSON document contains trailing data")
	}
	return nil
}

func repositoryRoot() (string, error) {
	root, err := command("", nil, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(root), nil
}

func loadDescriptor(repo string) (descriptor, error) {
	var value descriptor
	raw, err := os.ReadFile(filepath.Join(repo, descriptorPath))
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, err
	}
	if value.SchemaVersion != 1 || !semverPattern.MatchString(value.Version) {
		return value, errors.New("release descriptor has invalid schemaVersion or semver")
	}
	if value.Tag != "v"+value.Version {
		return value, errors.New("release tag must equal v<version> for Terraform Registry discovery")
	}
	if value.SourceRepository != "github.com/tako0614/terraform-provider-takoform" ||
		value.ProviderAddress != "registry.terraform.io/tako0614/takoform" ||
		value.GoModule != "github.com/tako0614/terraform-provider-takoform" {
		return value, errors.New("release descriptor public identity mismatch")
	}
	if err := validateCLIMatrix(value.CLIMatrix); err != nil {
		return value, err
	}
	if value.SigningFingerprint != "3510E75E05BBCC303B92D77934FC18AC897FB709" ||
		value.PublicKeyPath != "release/keys/provider-signing-key.asc" {
		return value, errors.New("release descriptor signing identity mismatch")
	}
	pinnedFingerprint, err := publicKeyFingerprint(filepath.Join(repo, value.PublicKeyPath))
	if err != nil {
		return value, fmt.Errorf("verify pinned release public key: %w", err)
	}
	if pinnedFingerprint != value.SigningFingerprint {
		return value, fmt.Errorf("pinned public key fingerprint %s does not match descriptor %s", pinnedFingerprint, value.SigningFingerprint)
	}
	if value.PublicationStatus != "candidate-only" || len(value.Platforms) == 0 {
		return value, errors.New("release descriptor must remain candidate-only with platforms")
	}
	seen := map[string]bool{}
	for _, platform := range value.Platforms {
		parts := strings.Split(platform, "_")
		if len(parts) != 2 || seen[platform] {
			return value, fmt.Errorf("invalid or duplicate platform %q", platform)
		}
		seen[platform] = true
	}
	return value, nil
}

func validateCLIMatrix(matrix []cliCompatibility) error {
	expected := map[string]string{
		"OpenTofu":  "registry.opentofu.org/tako0614/takoform",
		"Terraform": "registry.terraform.io/tako0614/takoform",
	}
	if len(matrix) != len(expected) {
		return errors.New("release descriptor must pin exactly OpenTofu and Terraform CLI/FQN entries")
	}
	seen := map[string]bool{}
	for _, entry := range matrix {
		address, ok := expected[entry.Product]
		if !ok || seen[entry.Product] || entry.Version == "" || entry.ProviderAddress != address {
			return fmt.Errorf("invalid release CLI/FQN matrix entry for %q", entry.Product)
		}
		seen[entry.Product] = true
	}
	return nil
}

func inspectSource(repo string, desc descriptor, allowDirty, allowUntagged bool) (sourceEvidence, error) {
	commit, err := command(repo, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return sourceEvidence{}, err
	}
	status, err := command(repo, nil, "git", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return sourceEvidence{}, err
	}
	dirty := strings.TrimSpace(status) != ""
	tags, err := command(repo, nil, "git", "tag", "--points-at", "HEAD")
	if err != nil {
		return sourceEvidence{}, err
	}
	tagPresent := linePresent(tags, desc.Tag)
	tagSigned := false
	tagSignerFingerprint := ""
	if tagPresent {
		var verifyOutput bytes.Buffer
		verify := exec.Command("git", "verify-tag", "--raw", desc.Tag)
		verify.Dir = repo
		verify.Stdout = &verifyOutput
		verify.Stderr = &verifyOutput
		verifyErr := verify.Run()
		tagSignerFingerprint, verifyErr = verifyPinnedTagSigner(
			verifyOutput.String(),
			verifyErr,
			desc.SigningFingerprint,
		)
		if verifyErr != nil {
			return sourceEvidence{}, fmt.Errorf(
				"release tag %s is not signed by pinned signer %s: %w",
				desc.Tag,
				desc.SigningFingerprint,
				verifyErr,
			)
		}
		tagSigned = true
	}
	blockers := []string{}
	if dirty {
		blockers = append(blockers, "source tree is dirty")
		if !allowDirty {
			return sourceEvidence{}, errors.New("source tree is dirty; commit first or use --allow-dirty-candidate for non-publishable evidence")
		}
	}
	if !tagPresent {
		blockers = append(blockers, "exact release tag is not attached to HEAD")
		if !allowUntagged {
			return sourceEvidence{}, fmt.Errorf("tag %s is not attached to HEAD; use --allow-untagged-candidate only for non-publishable evidence", desc.Tag)
		}
	} else if !tagSigned {
		return sourceEvidence{}, fmt.Errorf("release tag %s is not signed by pinned signer %s", desc.Tag, desc.SigningFingerprint)
	}
	if runtime.Version() != desc.GoVersion {
		return sourceEvidence{}, fmt.Errorf("Go toolchain mismatch: got %s, want %s", runtime.Version(), desc.GoVersion)
	}
	blockers = append(blockers,
		"candidate artifacts are unsigned; checksum signing occurs only in the environment-gated signed-tag workflow",
		fmt.Sprintf("direct Registry install/readback for provider %s is post-publication evidence", desc.Version),
		"release attestation publication occurs only in the environment-gated signed-tag workflow",
	)
	return sourceEvidence{
		Commit: strings.TrimSpace(commit), Dirty: dirty, TagPresent: tagPresent,
		TagSigned: tagSigned, TagSignerFingerprint: tagSignerFingerprint,
		GoVersion: runtime.Version(), Blockers: blockers,
		PublicationReady: false,
	}, nil
}

func build(repo, output string, desc descriptor, allowDirty, allowUntagged bool) (releaseManifest, error) {
	if _, err := os.Stat(output); err == nil {
		return releaseManifest{}, fmt.Errorf("output path already exists: %s", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return releaseManifest{}, err
	}
	evidence, err := inspectSource(repo, desc, allowDirty, allowUntagged)
	if err != nil {
		return releaseManifest{}, err
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return releaseManifest{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(output)
		}
	}()
	work, err := os.MkdirTemp("", "takoform-provider-build-")
	if err != nil {
		return releaseManifest{}, err
	}
	defer os.RemoveAll(work)

	artifacts := make([]artifact, 0, len(desc.Platforms))
	for _, platform := range desc.Platforms {
		parts := strings.Split(platform, "_")
		goos, goarch := parts[0], parts[1]
		binaryName := "terraform-provider-takoform_v" + desc.Version
		if goos == "windows" {
			binaryName += ".exe"
		}
		binaryPath := filepath.Join(work, platform, binaryName)
		if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
			return releaseManifest{}, err
		}
		ldflag := "-buildid= -X main.version=" + desc.Version
		env := append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
		if _, err := command(repo, env, "go", "build", "-trimpath", "-buildvcs=false", "-ldflags", ldflag, "-o", binaryPath, "."); err != nil {
			return releaseManifest{}, err
		}
		binaryBytes, err := os.ReadFile(binaryPath)
		if err != nil {
			return releaseManifest{}, err
		}
		if !bytes.Contains(binaryBytes, []byte(desc.Version)) {
			return releaseManifest{}, fmt.Errorf("binary %s does not contain version ldflag", platform)
		}
		embedded := "-X main.version=" + desc.Version
		if goos == runtime.GOOS && goarch == runtime.GOARCH {
			reported, err := command(repo, nil, binaryPath, "-version")
			if err != nil {
				return releaseManifest{}, err
			}
			if strings.TrimSpace(reported) != desc.Version {
				return releaseManifest{}, fmt.Errorf("binary %s reports version %q, want %q", platform, strings.TrimSpace(reported), desc.Version)
			}
		}
		binaryDigest, binarySize, err := fileDigest(binaryPath)
		if err != nil {
			return releaseManifest{}, err
		}
		archiveName := fmt.Sprintf("terraform-provider-takoform_%s_%s.zip", desc.Version, platform)
		archivePath := filepath.Join(output, archiveName)
		if err := deterministicZip(archivePath, []zipInput{
			{Name: binaryName, Path: binaryPath, Mode: 0o755},
			{Name: "LICENSE", Path: filepath.Join(repo, "LICENSE"), Mode: 0o644},
		}); err != nil {
			return releaseManifest{}, err
		}
		archiveDigest, archiveSize, err := fileDigest(archivePath)
		if err != nil {
			return releaseManifest{}, err
		}
		artifacts = append(artifacts, artifact{
			Platform: platform, Archive: archiveName, ArchiveSHA256: archiveDigest,
			ArchiveSize: archiveSize, Binary: binaryName, BinarySHA256: binaryDigest,
			BinarySize: binarySize, EmbeddedVersionLDFlag: embedded,
		})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Platform < artifacts[j].Platform })
	checksums := new(bytes.Buffer)
	for _, item := range artifacts {
		fmt.Fprintf(checksums, "%s  %s\n", item.ArchiveSHA256, item.Archive)
	}
	if err := os.WriteFile(filepath.Join(output, "SHA256SUMS"), checksums.Bytes(), 0o644); err != nil {
		return releaseManifest{}, err
	}
	sbom, err := createSBOM(repo, desc, evidence.Commit)
	if err != nil {
		return releaseManifest{}, err
	}
	if err := validateSPDX(repo, sbom); err != nil {
		return releaseManifest{}, fmt.Errorf("validate SPDX 2.3 candidate SBOM: %w", err)
	}
	if err := writeJSONFile(filepath.Join(output, "sbom.spdx.json"), sbom); err != nil {
		return releaseManifest{}, err
	}
	sbomDigest, _, err := fileDigest(filepath.Join(output, "sbom.spdx.json"))
	if err != nil {
		return releaseManifest{}, err
	}
	checksumDigest, _, err := fileDigest(filepath.Join(output, "SHA256SUMS"))
	if err != nil {
		return releaseManifest{}, err
	}
	manifest := releaseManifest{
		SchemaVersion: 1, Kind: "takoform.provider-release-candidate@v1",
		Version: desc.Version, Tag: desc.Tag, SourceRepository: desc.SourceRepository,
		SourceCommit: evidence.Commit, SourceDirty: evidence.Dirty,
		ProviderAddress: desc.ProviderAddress, CLIMatrix: desc.CLIMatrix, GoModule: desc.GoModule, GoVersion: desc.GoVersion,
		PublicationStatus: desc.PublicationStatus, PublicationReady: false,
		PublicationBlockers: evidence.Blockers, Artifacts: artifacts,
		Materials: map[string]string{"SHA256SUMS": checksumDigest, "sbom.spdx.json": sbomDigest},
	}
	if err := writeJSONFile(filepath.Join(output, "manifest.json"), manifest); err != nil {
		return releaseManifest{}, err
	}
	provenance := createProvenance(desc, evidence, artifacts)
	if err := validateSLSAProvenance(provenance); err != nil {
		return releaseManifest{}, fmt.Errorf("validate in-toto/SLSA v1 candidate provenance: %w", err)
	}
	if err := writeJSONFile(filepath.Join(output, "provenance.json"), provenance); err != nil {
		return releaseManifest{}, err
	}
	cleanup = false
	return manifest, nil
}

func verifyExpectedTriggerTag(expected, descriptorTag string) error {
	if expected == "" {
		return nil
	}
	if expected != descriptorTag {
		return fmt.Errorf("release trigger tag %q does not match descriptor tag %q", expected, descriptorTag)
	}
	return nil
}

func verifyReproducible(repo string, desc descriptor, allowDirty, allowUntagged bool) error {
	base, err := os.MkdirTemp("", "takoform-provider-repro-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(base)
	first, second := filepath.Join(base, "first"), filepath.Join(base, "second")
	if _, err := build(repo, first, desc, allowDirty, allowUntagged); err != nil {
		return err
	}
	if _, err := build(repo, second, desc, allowDirty, allowUntagged); err != nil {
		return err
	}
	left, err := treeDigests(first)
	if err != nil {
		return err
	}
	right, err := treeDigests(second)
	if err != nil {
		return err
	}
	if len(left) != len(right) {
		return errors.New("candidate file count differs between builds")
	}
	for name, digest := range left {
		if right[name] != digest {
			return fmt.Errorf("candidate is not reproducible: %s differs", name)
		}
	}
	return nil
}

type zipInput struct {
	Name, Path string
	Mode       os.FileMode
}

func deterministicZip(target string, inputs []zipInput) error {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(target)
		}
	}()
	zw := zip.NewWriter(file)
	fixed := time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, input := range inputs {
		raw, err := os.ReadFile(input.Path)
		if err != nil {
			_ = zw.Close()
			return err
		}
		header := &zip.FileHeader{Name: input.Name, Method: zip.Deflate}
		header.SetMode(input.Mode)
		header.SetModTime(fixed)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if _, err := writer.Write(raw); err != nil {
			_ = zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func createSBOM(repo string, desc descriptor, commit string) (spdxDocument, error) {
	commitTimeText, err := command(repo, nil, "git", "show", "-s", "--format=%cI", commit)
	if err != nil {
		return spdxDocument{}, err
	}
	commitTime, err := time.Parse(time.RFC3339, strings.TrimSpace(commitTimeText))
	if err != nil {
		return spdxDocument{}, fmt.Errorf("parse source commit timestamp: %w", err)
	}
	out, err := command(repo, nil, "go", "list", "-m", "-json", "all")
	if err != nil {
		return spdxDocument{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(out))
	packages := []spdxPackage{}
	for {
		var mod module
		if err := decoder.Decode(&mod); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return spdxDocument{}, err
		}
		effective := mod
		if mod.Replace != nil {
			effective = *mod.Replace
		}
		pkg := spdxPackage{
			Name: mod.Path, SPDXID: "SPDXRef-Package-" + sanitizeSPDXID(mod.Path),
			VersionInfo: effective.Version, DownloadLocation: "NOASSERTION", FilesAnalyzed: false,
		}
		packages = append(packages, pkg)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })
	return spdxDocument{
		SPDXVersion: "SPDX-2.3", DataLicense: "CC0-1.0", SPDXID: "SPDXRef-DOCUMENT",
		Name:              "terraform-provider-takoform-" + desc.Version,
		DocumentNamespace: "https://takoform.com/spdx/provider/" + desc.Version + "/" + commit,
		CreationInfo: spdxCreation{
			Creators: []string{"Tool: takoform-provider-release"},
			Created:  commitTime.UTC().Format(time.RFC3339),
		},
		Packages: packages,
	}, nil
}

func createProvenance(desc descriptor, evidence sourceEvidence, artifacts []artifact) statement {
	subjects := make([]statementSubject, 0, len(artifacts))
	for _, item := range artifacts {
		subjects = append(subjects, statementSubject{Name: item.Archive, Digest: map[string]string{"sha256": item.ArchiveSHA256}})
	}
	return statement{
		Type: "https://in-toto.io/Statement/v1", Subject: subjects,
		PredicateType: "https://slsa.dev/provenance/v1",
		Predicate: statementPredicate{
			BuildDefinition: map[string]any{
				"buildType": "https://takoform.com/buildtypes/provider-candidate/v1",
				"externalParameters": map[string]any{
					"version": desc.Version,
					"tag":     desc.Tag,
				},
				"internalParameters": map[string]any{
					"goVersion": desc.GoVersion,
					"platforms": desc.Platforms,
					"cliMatrix": desc.CLIMatrix,
				},
				"resolvedDependencies": []map[string]any{{"uri": "git+https://" + desc.SourceRepository, "digest": map[string]string{"gitCommit": evidence.Commit}}},
			},
			RunDetails: map[string]any{
				"builder": map[string]string{"id": "https://github.com/tako0614/terraform-provider-takoform/tree/main/cmd/provider-release"},
			},
		},
	}
}

func validateSPDX(repo string, document spdxDocument) error {
	created, err := time.Parse(time.RFC3339, document.CreationInfo.Created)
	if err != nil || document.CreationInfo.Created != created.UTC().Format(time.RFC3339) {
		return fmt.Errorf("creationInfo.created must be a non-empty UTC RFC 3339 timestamp")
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return validateSPDXValue(repo, value)
}

func validateSPDXFile(repo, path string) error {
	var document any
	if err := readStrictJSONFile(path, &document); err != nil {
		return fmt.Errorf("read SPDX document %s: %w", filepath.Base(path), err)
	}
	if err := validateSPDXValue(repo, document); err != nil {
		return fmt.Errorf("validate SPDX document %s: %w", filepath.Base(path), err)
	}
	return nil
}

func verifySPDXFiles(repo string, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, errors.New("verify-sbom requires one or more SPDX JSON paths")
	}
	verified := make([]string, 0, len(requested))
	for _, item := range requested {
		path := item
		if !filepath.IsAbs(path) {
			path = filepath.Join(repo, path)
		}
		if err := validateSPDXFile(repo, path); err != nil {
			return nil, err
		}
		verified = append(verified, filepath.Base(path))
	}
	sort.Strings(verified)
	return verified, nil
}

func validateSPDXValue(repo string, document any) error {
	schemaPath := filepath.Join(repo, "release", "schemas", "spdx-2.3.schema.json")
	schemaFile, err := os.Open(schemaPath)
	if err != nil {
		return err
	}
	defer schemaFile.Close()
	var schemaDocument any
	if err := json.NewDecoder(schemaFile).Decode(&schemaDocument); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const schemaURL = "https://raw.githubusercontent.com/spdx/spdx-spec/refs/tags/v2.3/schemas/spdx-schema.json"
	if err := compiler.AddResource(schemaURL, schemaDocument); err != nil {
		return err
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return err
	}
	return schema.Validate(document)
}

func validateSLSAProvenance(document statement) error {
	raw, err := json.Marshal(document)
	if err != nil {
		return err
	}
	var envelope intotov1.Statement
	if err := protojson.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	if envelope.PredicateType != "https://slsa.dev/provenance/v1" {
		return fmt.Errorf("unexpected predicate type %q", envelope.PredicateType)
	}
	predicateRaw, err := protojson.Marshal(envelope.Predicate)
	if err != nil {
		return err
	}
	var predicate slsav1.Provenance
	if err := protojson.Unmarshal(predicateRaw, &predicate); err != nil {
		return err
	}
	if err := predicate.Validate(); err != nil {
		return err
	}
	if predicate.GetBuildDefinition().GetInternalParameters() == nil {
		return errors.New("buildDefinition.internalParameters must be explicitly recorded")
	}
	return nil
}

func treeDigests(root string) (map[string]string, error) {
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest, _, err := fileDigest(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(rel)] = digest
		return nil
	})
	return result, err
}

func fileDigest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	h := sha256.New()
	size, err := io.Copy(h, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func command(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func commandInput(dir string, env []string, input []byte, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func linePresent(text, want string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func validSignatureFingerprint(status string) string {
	for _, line := range strings.Split(status, "\n") {
		fields := strings.Fields(line)
		for index, field := range fields {
			if field == "VALIDSIG" && index+1 < len(fields) {
				fingerprint := strings.ToUpper(fields[index+1])
				if regexp.MustCompile(`^[0-9A-F]{40}$`).MatchString(fingerprint) {
					return fingerprint
				}
			}
		}
	}
	return ""
}

func verifyPinnedTagSigner(status string, verifyErr error, expected string) (string, error) {
	fingerprint := validSignatureFingerprint(status)
	if verifyErr != nil {
		return fingerprint, fmt.Errorf("git tag signature verification failed: %w", verifyErr)
	}
	if fingerprint == "" {
		return "", errors.New("git tag signature verification returned no VALIDSIG fingerprint")
	}
	if fingerprint != expected {
		return fingerprint, fmt.Errorf("release tag signer %s does not match pinned signer %s", fingerprint, expected)
	}
	return fingerprint, nil
}

func publicKeyFingerprint(path string) (string, error) {
	out, err := command("", nil, "gpg", "--batch", "--show-keys", "--with-colons", path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) > 9 && fields[0] == "fpr" && regexp.MustCompile(`^[0-9A-Fa-f]{40}$`).MatchString(fields[9]) {
			return strings.ToUpper(fields[9]), nil
		}
	}
	return "", errors.New("public key has no primary fingerprint")
}

func sanitizeSPDXID(value string) string {
	return regexp.MustCompile(`[^A-Za-z0-9.-]+`).ReplaceAllString(value, "-")
}

func writeJSONFile(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeJSON(writer io.Writer, value any) {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	check(encoder.Encode(value))
}

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}
func fail(message string) { fmt.Fprintln(os.Stderr, "provider-release:", message); os.Exit(1) }
