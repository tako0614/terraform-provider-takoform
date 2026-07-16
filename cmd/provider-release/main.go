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

type descriptor struct {
	SchemaVersion      int      `json:"schemaVersion"`
	Version            string   `json:"version"`
	Tag                string   `json:"tag"`
	SourceRepository   string   `json:"sourceRepository"`
	ProviderAddress    string   `json:"providerAddress"`
	GoModule           string   `json:"goModule"`
	GoVersion          string   `json:"goVersion"`
	SigningFingerprint string   `json:"signingFingerprint"`
	PublicKeyPath      string   `json:"publicKeyPath"`
	Platforms          []string `json:"platforms"`
	PublicationStatus  string   `json:"publicationStatus"`
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
	SchemaVersion       int               `json:"schemaVersion"`
	Kind                string            `json:"kind"`
	Version             string            `json:"version"`
	Tag                 string            `json:"tag"`
	SourceRepository    string            `json:"sourceRepository"`
	SourceCommit        string            `json:"sourceCommit"`
	SourceDirty         bool              `json:"sourceDirty"`
	ProviderAddress     string            `json:"providerAddress"`
	GoModule            string            `json:"goModule"`
	GoVersion           string            `json:"goVersion"`
	PublicationStatus   string            `json:"publicationStatus"`
	PublicationReady    bool              `json:"publicationReady"`
	PublicationBlockers []string          `json:"publicationBlockers"`
	Artifacts           []artifact        `json:"artifacts"`
	Materials           map[string]string `json:"materials"`
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

func main() {
	if len(os.Args) < 2 {
		fail("usage: provider-release <verify-source|build|verify-reproducible> [options]")
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
	default:
		fail("unknown command: " + os.Args[1])
	}
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
		"Terraform Registry public-key registration and public install proof are external",
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
		ProviderAddress: desc.ProviderAddress, GoModule: desc.GoModule, GoVersion: desc.GoVersion,
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
	raw, err := json.Marshal(document)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return schema.Validate(value)
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
