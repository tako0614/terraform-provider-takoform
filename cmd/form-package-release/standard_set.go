package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1" // SPDX 2.3 package verification code requires SHA-1.
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
)

const (
	standardSetFormat       = "takoform.standard-form-package-set-release@v1"
	standardSetManifestName = "standard-form-package-set.json"
	standardSetBundleName   = "standard-form-package-set.sigstore.json"
	controllerCandidateKind = "takos.release-candidate-manifest@v1"
	controllerResultKind    = "takos.release-safety-adapter-result@v1"
	standardSetSurfaceID    = "takoform-standard-form-package-set"
)

var sha256Pattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type standardSetInventory struct {
	Format            string                      `json:"format"`
	DefinitionVersion string                      `json:"definitionVersion"`
	PackageVersion    string                      `json:"packageVersion"`
	Packages          []standardSetInventoryEntry `json:"packages"`
}

type standardSetInventoryEntry struct {
	Kind          string              `json:"kind"`
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}

type standardSetActivation struct {
	Type                         string `json:"type"`
	Tag                          string `json:"tag"`
	ReleaseURL                   string `json:"releaseUrl"`
	PartialPackageReleasesActive bool   `json:"partialPackageReleasesActive"`
}

type standardSetPackage struct {
	Kind                  string              `json:"kind"`
	ReleaseID             string              `json:"releaseId"`
	Tag                   string              `json:"tag"`
	FormRef               formpackage.FormRef `json:"formRef"`
	PackageDigest         string              `json:"packageDigest"`
	ReleaseManifestDigest string              `json:"releaseManifestDigest"`
	Assets                []releaseAsset      `json:"assets"`
}

type standardSetManifest struct {
	Format              string                `json:"format"`
	Repository          string                `json:"repository"`
	SourceCommit        string                `json:"sourceCommit"`
	ToolingCommit       string                `json:"toolingCommit"`
	Version             string                `json:"version"`
	Tag                 string                `json:"tag"`
	ActivationAuthority standardSetActivation `json:"activationAuthority"`
	PackageCount        int                   `json:"packageCount"`
	Packages            []standardSetPackage  `json:"packages"`
}

type standardSetReleaseManifest struct {
	SchemaVersion       int                     `json:"schemaVersion"`
	ReleaseType         string                  `json:"releaseType"`
	Tag                 string                  `json:"tag"`
	SourceRepository    string                  `json:"sourceRepository"`
	SourceCommit        string                  `json:"sourceCommit"`
	ToolingCommit       string                  `json:"toolingCommit"`
	Workflow            string                  `json:"workflow"`
	Version             string                  `json:"version"`
	Canonicalization    string                  `json:"canonicalization"`
	SignedSubject       string                  `json:"signedSubject"`
	SignatureBundle     string                  `json:"signatureBundle"`
	SignatureMediaType  string                  `json:"signatureMediaType"`
	PublisherPolicy     publisherPolicyEvidence `json:"publisherPolicy"`
	PackageCount        int                     `json:"packageCount"`
	Assets              []releaseAsset          `json:"assets"`
	PublicationReady    bool                    `json:"publicationReady"`
	PublicationBlockers []string                `json:"publicationBlockers"`
}

type controllerReleaseAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

type controllerCandidateManifest struct {
	Kind              string                   `json:"kind"`
	SurfaceID         string                   `json:"surfaceId"`
	Repository        string                   `json:"repository"`
	SourceCommit      string                   `json:"sourceCommit"`
	Version           string                   `json:"version"`
	Tag               string                   `json:"tag"`
	WorkflowRunID     string                   `json:"workflowRunId"`
	BuiltAt           string                   `json:"builtAt"`
	OCIImages         []any                    `json:"ociImages"`
	ReleaseAssets     []controllerReleaseAsset `json:"releaseAssets"`
	ArtifactDigests   []string                 `json:"artifactDigests"`
	SBOMDigests       []string                 `json:"sbomDigests"`
	ProvenanceDigests []string                 `json:"provenanceDigests"`
	ConfigDigest      string                   `json:"configDigest"`
	PolicyDigest      string                   `json:"policyDigest"`
	ToolchainDigest   string                   `json:"toolchainDigest"`
}

type releaseSafetyCheck struct {
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	BindingDigest *string `json:"bindingDigest"`
}

type standardSetReadback struct {
	Kind              string               `json:"kind"`
	Status            string               `json:"status"`
	SurfaceID         string               `json:"surfaceId"`
	SourceCommit      string               `json:"sourceCommit"`
	ControllerCommit  string               `json:"controllerCommit"`
	ControllerDigest  string               `json:"controllerDigest"`
	AdapterDigest     string               `json:"adapterDigest"`
	ArtifactDigests   []string             `json:"artifactDigests"`
	TargetFingerprint string               `json:"targetFingerprint"`
	AttestationDigest string               `json:"attestationDigest"`
	ReleaseTag        string               `json:"releaseTag"`
	ReleaseURL        string               `json:"releaseUrl"`
	WorkflowRunID     string               `json:"workflowRunId"`
	ReadbackAt        string               `json:"readbackAt"`
	HealthChecks      []releaseSafetyCheck `json:"healthChecks"`
}

func runBuildStandardSet(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("build-standard-set", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repo := flags.String("repo", ".", "repository root")
	tag := flags.String("tag", "", "exact standard-forms/v<semver> tag")
	packagesRoot := flags.String("packages-root", "", "directory containing ten finalized package outputs")
	outputDir := flags.String("output", "", "new root set output directory")
	toolingCommit := flags.String("tooling-commit", "", "exact protected-main release-tooling commit")
	allowUntagged := flags.Bool("allow-untagged-candidate", false, "permit a non-publishable local set candidate")
	allowDirty := flags.Bool("allow-dirty-candidate", false, "permit a non-publishable local dirty source")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *packagesRoot == "" || *outputDir == "" || !commitPattern.MatchString(*toolingCommit) {
		return usageError()
	}
	version, err := standardSetVersion(*tag)
	if err != nil {
		return err
	}
	evidence, err := inspectSource(*repo, *tag, *allowUntagged, *allowDirty)
	if err != nil {
		return err
	}
	inventory, err := readStandardSetInventory(*repo)
	if err != nil {
		return err
	}
	if inventory.PackageVersion != version || inventory.DefinitionVersion != version || len(inventory.Packages) != len(standardforms.Specs) {
		return fmt.Errorf("standard package inventory does not match set version %s", version)
	}
	packages := make([]standardSetPackage, 0, len(inventory.Packages))
	for index, spec := range standardforms.Specs {
		entry := inventory.Packages[index]
		if entry.Kind != spec.Kind {
			return fmt.Errorf("standard package inventory order changed at %d", index)
		}
		releaseID, err := releaseIDForKind(entry.Kind)
		if err != nil {
			return err
		}
		packageOutput := filepath.Join(*packagesRoot, releaseID)
		pkg, err := verifyFinalizedSetPackage(packageOutput, entry, version, evidence.commit, *toolingCommit)
		if err != nil {
			return fmt.Errorf("%s package output: %w", entry.Kind, err)
		}
		packages = append(packages, pkg)
	}
	set := standardSetManifest{
		Format: standardSetFormat, Repository: sourceRepository, SourceCommit: evidence.commit, ToolingCommit: *toolingCommit,
		Version: version, Tag: *tag, PackageCount: len(packages), Packages: packages,
		ActivationAuthority: standardSetActivation{
			Type: "immutable-root-release", Tag: *tag,
			ReleaseURL:                   "https://" + sourceRepository + "/releases/tag/" + *tag,
			PartialPackageReleasesActive: false,
		},
	}
	if err := createOutput(*outputDir); err != nil {
		return err
	}
	if err := writeCanonicalJSON(filepath.Join(*outputDir, standardSetManifestName), set); err != nil {
		return err
	}
	archiveName := "takoform-standard-forms_" + version + ".tar.gz"
	if err := writeStandardSetArchive(filepath.Join(*outputDir, archiveName), *packagesRoot, packages); err != nil {
		return err
	}
	setAsset, err := describeAsset(filepath.Join(*outputDir, standardSetManifestName), standardSetManifestName, "application/vnd.takoform.standard-form-package-set.v1+json")
	if err != nil {
		return err
	}
	archiveAsset, err := describeAsset(filepath.Join(*outputDir, archiveName), archiveName, "application/gzip")
	if err != nil {
		return err
	}
	sbomName := "takoform-standard-forms_" + version + "_sbom.spdx.json"
	if err := writeCanonicalJSON(filepath.Join(*outputDir, sbomName), createStandardSetSBOM(set, evidence.commitTime)); err != nil {
		return err
	}
	sbomAsset, err := describeAsset(filepath.Join(*outputDir, sbomName), sbomName, "application/spdx+json")
	if err != nil {
		return err
	}
	provenanceName := "takoform-standard-forms_" + version + "_provenance.intoto.json"
	provenance := createProvenance(*tag, setWorkflow, evidence.commit, *toolingCommit, []releaseAsset{setAsset, archiveAsset})
	if err := writeCanonicalJSON(filepath.Join(*outputDir, provenanceName), provenance); err != nil {
		return err
	}
	provenanceAsset, err := describeAsset(filepath.Join(*outputDir, provenanceName), provenanceName, "application/vnd.in-toto+json")
	if err != nil {
		return err
	}
	policy := publisherPolicy(setWorkflow, "refs/tags/standard-forms/v*", *toolingCommit)
	policy.Identity = "https://" + sourceRepository + "/" + setWorkflow + "@refs/tags/" + *tag
	manifest := standardSetReleaseManifest{
		SchemaVersion: 1, ReleaseType: "standard-form-package-set", Tag: *tag,
		SourceRepository: sourceRepository, SourceCommit: evidence.commit, ToolingCommit: *toolingCommit,
		Workflow: setWorkflow, Version: version, Canonicalization: canonicalization,
		SignedSubject: standardSetManifestName, SignatureBundle: standardSetBundleName, SignatureMediaType: bundleMediaType,
		PublisherPolicy: policy, PackageCount: len(packages), Assets: []releaseAsset{setAsset, archiveAsset, sbomAsset, provenanceAsset},
		PublicationReady: false, PublicationBlockers: evidence.blockers,
	}
	sort.Slice(manifest.Assets, func(i, j int) bool { return manifest.Assets[i].Name < manifest.Assets[j].Name })
	if err := writeJSON(filepath.Join(*outputDir, "release-manifest.json"), manifest); err != nil {
		return err
	}
	return writeJSONTo(output, manifest)
}

func runFinalizeStandardSet(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("finalize-standard-set", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	outputDir := flags.String("output", "", "root set output directory")
	workflowRunID := flags.String("workflow-run-id", "", "candidate GitHub Actions run id")
	builtAt := flags.String("built-at", "", "candidate build timestamp")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *outputDir == "" || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(*workflowRunID) {
		return usageError()
	}
	if _, err := time.Parse(time.RFC3339Nano, *builtAt); err != nil {
		return fmt.Errorf("built-at must be RFC3339: %w", err)
	}
	manifestPath := filepath.Join(*outputDir, "release-manifest.json")
	var manifest standardSetReleaseManifest
	if err := readStrictJSON(manifestPath, &manifest); err != nil {
		return err
	}
	if manifest.ReleaseType != "standard-form-package-set" || manifest.SignedSubject != standardSetManifestName || manifest.SignatureBundle != standardSetBundleName {
		return fmt.Errorf("root set release manifest identity is invalid")
	}
	if err := validateSigstoreBundle(filepath.Join(*outputDir, manifest.SignatureBundle)); err != nil {
		return err
	}
	bundle, err := describeAsset(filepath.Join(*outputDir, manifest.SignatureBundle), manifest.SignatureBundle, bundleMediaType)
	if err != nil {
		return err
	}
	for _, asset := range manifest.Assets {
		if asset.Name == bundle.Name {
			return fmt.Errorf("standard set signature bundle is already finalized")
		}
	}
	manifest.Assets = append(manifest.Assets, bundle)
	sort.Slice(manifest.Assets, func(i, j int) bool { return manifest.Assets[i].Name < manifest.Assets[j].Name })
	manifest.PublicationReady = len(manifest.PublicationBlockers) == 0
	if !manifest.PublicationReady {
		return fmt.Errorf("standard set candidate is not publication-ready: %s", strings.Join(manifest.PublicationBlockers, "; "))
	}
	if err := writeJSON(manifestPath, manifest); err != nil {
		return err
	}
	if err := writeChecksums(*outputDir); err != nil {
		return err
	}
	candidate, err := createControllerCandidate(*outputDir, manifest, *workflowRunID, *builtAt)
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(*outputDir, "release-candidate-manifest.json"), candidate); err != nil {
		return err
	}
	return writeJSONTo(output, candidate)
}

func runVerifyStandardSetCandidate(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("verify-standard-set-candidate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	outputDir := flags.String("output", "", "downloaded closed candidate directory")
	sourceCommit := flags.String("source-commit", "", "envelope source commit")
	workflowRunID := flags.String("workflow-run-id", "", "candidate workflow run id")
	manifestDigest := flags.String("manifest-digest", "", "candidate manifest digest")
	artifactDigestsB64 := flags.String("artifact-digests-b64", "", "base64url ordered artifact digests")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *outputDir == "" || !commitPattern.MatchString(*sourceCommit) || !sha256Pattern.MatchString(*manifestDigest) {
		return usageError()
	}
	candidatePath := filepath.Join(*outputDir, "release-candidate-manifest.json")
	if digestFile(candidatePath) != *manifestDigest {
		return fmt.Errorf("candidate manifest digest differs from the envelope")
	}
	var candidate controllerCandidateManifest
	if err := readStrictJSON(candidatePath, &candidate); err != nil {
		return err
	}
	if candidate.Kind != controllerCandidateKind || candidate.SurfaceID != standardSetSurfaceID || candidate.Repository != "https://"+sourceRepository+".git" ||
		candidate.SourceCommit != *sourceCommit || candidate.WorkflowRunID != *workflowRunID || candidate.Tag != "standard-forms/v"+candidate.Version || len(candidate.OCIImages) != 0 {
		return fmt.Errorf("candidate controller identity differs from the envelope")
	}
	requestedDigests, err := decodeStringArray(*artifactDigestsB64)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(requestedDigests, candidate.ArtifactDigests) {
		return fmt.Errorf("candidate artifact digests differ from the envelope")
	}
	expectedNames := []string{"release-candidate-manifest.json"}
	for _, asset := range candidate.ReleaseAssets {
		if digestFile(filepath.Join(*outputDir, asset.Name)) != asset.Digest {
			return fmt.Errorf("candidate asset digest drifted: %s", asset.Name)
		}
		expectedNames = append(expectedNames, asset.Name)
	}
	actualNames, err := regularFileNames(*outputDir)
	if err != nil {
		return err
	}
	sort.Strings(expectedNames)
	if !reflect.DeepEqual(expectedNames, actualNames) {
		return fmt.Errorf("candidate directory inventory is not closed")
	}
	var set standardSetManifest
	if err := readStrictJSON(filepath.Join(*outputDir, standardSetManifestName), &set); err != nil {
		return err
	}
	if err := verifyStandardSetManifest(set, candidate.Version, *sourceCommit); err != nil {
		return err
	}
	archive := filepath.Join(*outputDir, "takoform-standard-forms_"+candidate.Version+".tar.gz")
	if err := verifyStandardSetArchive(archive, set.Packages); err != nil {
		return err
	}
	return writeJSONTo(output, map[string]any{"kind": "takoform.standard-form-package-set-candidate-verification@v1", "ok": true, "packageCount": len(set.Packages)})
}

func runBuildStandardSetReadback(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("build-standard-set-readback", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	outputPath := flags.String("output", "", "new readback JSON path")
	sourceCommit := flags.String("source-commit", "", "source commit")
	controllerCommit := flags.String("controller-commit", "", "controller commit")
	controllerDigest := flags.String("controller-digest", "", "controller digest")
	adapterDigest := flags.String("adapter-digest", "", "adapter digest")
	artifactDigestsB64 := flags.String("artifact-digests-b64", "", "base64url artifact digests")
	healthChecksB64 := flags.String("health-checks-b64", "", "base64url required health checks")
	targetFingerprint := flags.String("target-fingerprint", "", "target fingerprint")
	attestationDigest := flags.String("attestation-digest", "", "root Sigstore bundle digest")
	workflowRunID := flags.String("workflow-run-id", "", "promotion workflow run id")
	readbackAt := flags.String("readback-at", "", "readback timestamp")
	version := flags.String("version", "", "set version")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *outputPath == "" || !commitPattern.MatchString(*sourceCommit) || !commitPattern.MatchString(*controllerCommit) || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(*workflowRunID) {
		return usageError()
	}
	for _, digest := range []string{*controllerDigest, *adapterDigest, *targetFingerprint, *attestationDigest} {
		if !sha256Pattern.MatchString(digest) {
			return fmt.Errorf("readback requires exact sha256 bindings")
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, *readbackAt); err != nil {
		return fmt.Errorf("readback-at must be RFC3339: %w", err)
	}
	artifacts, err := decodeStringArray(*artifactDigestsB64)
	if err != nil {
		return err
	}
	checks, err := decodeChecks(*healthChecksB64)
	if err != nil {
		return err
	}
	for index := range checks {
		if checks[index].Status != "required" || checks[index].BindingDigest == nil || !sha256Pattern.MatchString(*checks[index].BindingDigest) {
			return fmt.Errorf("health check %q is not envelope-bound and required", checks[index].Name)
		}
		checks[index].Status = "passed"
	}
	wantNames := []string{"ten Form Package immutable release readback", "active root set manifest readback", "Sigstore bundle and transparency readback"}
	gotNames := make([]string, len(checks))
	for index := range checks {
		gotNames[index] = checks[index].Name
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		return fmt.Errorf("readback health checks are not the fixed ordered set")
	}
	result := standardSetReadback{
		Kind: controllerResultKind, Status: "promoted", SurfaceID: standardSetSurfaceID,
		SourceCommit: *sourceCommit, ControllerCommit: *controllerCommit, ControllerDigest: *controllerDigest,
		AdapterDigest: *adapterDigest, ArtifactDigests: artifacts, TargetFingerprint: *targetFingerprint,
		AttestationDigest: *attestationDigest, ReleaseTag: "standard-forms/v" + *version,
		ReleaseURL:    "https://" + sourceRepository + "/releases/tag/standard-forms/v" + *version,
		WorkflowRunID: *workflowRunID, ReadbackAt: *readbackAt, HealthChecks: checks,
	}
	if _, err := os.Stat(*outputPath); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("readback output already exists")
	}
	if err := writeJSON(*outputPath, result); err != nil {
		return err
	}
	return writeJSONTo(output, result)
}

func standardSetVersion(tag string) (string, error) {
	match := regexp.MustCompile(`^standard-forms/v(` + semverPattern + `)$`).FindStringSubmatch(tag)
	if match == nil {
		return "", fmt.Errorf("standard set tag must match standard-forms/v<semver>")
	}
	return match[1], nil
}

func readStandardSetInventory(repo string) (standardSetInventory, error) {
	var inventory standardSetInventory
	raw, err := os.ReadFile(filepath.Join(repo, "forms", "standard-package-set.json"))
	if err != nil {
		return standardSetInventory{}, err
	}
	if err := json.Unmarshal(raw, &inventory); err != nil {
		return standardSetInventory{}, err
	}
	if inventory.Format != "takoform.standard-package-set@v1" {
		return standardSetInventory{}, fmt.Errorf("unexpected standard package inventory format")
	}
	return inventory, nil
}

func verifyFinalizedSetPackage(root string, entry standardSetInventoryEntry, version, sourceCommit, toolingCommit string) (standardSetPackage, error) {
	var manifest releaseManifest
	if err := readStrictJSON(filepath.Join(root, "release-manifest.json"), &manifest); err != nil {
		return standardSetPackage{}, err
	}
	releaseID, _ := releaseIDForKind(entry.Kind)
	tag := "forms/" + releaseID + "/v" + version
	expectedIdentity := "https://" + sourceRepository + "/" + setWorkflow + "@refs/tags/standard-forms/v" + version
	if manifest.ReleaseType != "form-package" || manifest.Tag != tag || manifest.SourceCommit != sourceCommit || manifest.ToolingCommit != toolingCommit ||
		manifest.Workflow != setWorkflow || manifest.PackageVersion != version || manifest.ReleaseID != releaseID || manifest.PackageDigest != entry.PackageDigest ||
		manifest.FormRef != entry.FormRef || !manifest.PublicationReady || len(manifest.PublicationBlockers) != 0 ||
		manifest.PublisherPolicy.Identity != expectedIdentity || manifest.PublisherPolicy.TagPattern != "refs/tags/standard-forms/v*" {
		return standardSetPackage{}, fmt.Errorf("finalized package release manifest is not bound to the coordinated source set")
	}
	if err := validateSigstoreBundle(filepath.Join(root, manifest.SignatureBundle)); err != nil {
		return standardSetPackage{}, err
	}
	if err := verifyChecksums(root); err != nil {
		return standardSetPackage{}, err
	}
	names, err := regularFileNames(root)
	if err != nil {
		return standardSetPackage{}, err
	}
	expected := []string{"release-manifest.json", "SHA256SUMS"}
	for _, asset := range manifest.Assets {
		actual, err := describeAsset(filepath.Join(root, asset.Name), asset.Name, asset.MediaType)
		if err != nil || actual != asset {
			return standardSetPackage{}, fmt.Errorf("asset %s differs from release manifest", asset.Name)
		}
		expected = append(expected, asset.Name)
	}
	sort.Strings(expected)
	if !reflect.DeepEqual(names, expected) {
		return standardSetPackage{}, fmt.Errorf("finalized package output inventory is not closed")
	}
	assets := make([]releaseAsset, 0, len(names))
	for _, name := range names {
		mediaType := "application/octet-stream"
		if name == "release-manifest.json" || strings.HasSuffix(name, ".json") {
			mediaType = "application/json"
		} else if name == "SHA256SUMS" {
			mediaType = "text/plain"
		} else if strings.HasSuffix(name, ".tar.gz") {
			mediaType = "application/gzip"
		}
		asset, err := describeAsset(filepath.Join(root, name), name, mediaType)
		if err != nil {
			return standardSetPackage{}, err
		}
		assets = append(assets, asset)
	}
	return standardSetPackage{
		Kind: entry.Kind, ReleaseID: releaseID, Tag: tag, FormRef: entry.FormRef, PackageDigest: entry.PackageDigest,
		ReleaseManifestDigest: digestFile(filepath.Join(root, "release-manifest.json")), Assets: assets,
	}, nil
}

func writeStandardSetArchive(path, packagesRoot string, packages []standardSetPackage) error {
	handle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	failed := true
	defer func() {
		_ = handle.Close()
		if failed {
			_ = os.Remove(path)
		}
	}()
	gzipWriter, err := gzip.NewWriterLevel(handle, gzip.BestCompression)
	if err != nil {
		return err
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	for _, pkg := range packages {
		for _, asset := range pkg.Assets {
			raw, err := os.ReadFile(filepath.Join(packagesRoot, pkg.ReleaseID, asset.Name))
			if err != nil {
				return err
			}
			if formpackage.DigestBytes(raw) != asset.Digest || int64(len(raw)) != asset.Size {
				return fmt.Errorf("package asset changed during set archive: %s/%s", pkg.ReleaseID, asset.Name)
			}
			if err := writeTarFile(tarWriter, "packages/"+pkg.ReleaseID+"/"+asset.Name, raw); err != nil {
				return err
			}
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

func verifyStandardSetArchive(path string, packages []standardSetPackage) error {
	expected := map[string]string{}
	for _, pkg := range packages {
		for _, asset := range pkg.Assets {
			expected["packages/"+pkg.ReleaseID+"/"+asset.Name] = asset.Digest
		}
	}
	handle, err := os.Open(path)
	if err != nil {
		return err
	}
	defer handle.Close()
	gzipReader, err := gzip.NewReader(handle)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	seen := map[string]struct{}{}
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		digest, ok := expected[header.Name]
		if !ok || header.Typeflag != tar.TypeReg || header.Mode != 0o644 {
			return fmt.Errorf("unexpected set archive entry %s", header.Name)
		}
		raw, err := io.ReadAll(tarReader)
		if err != nil {
			return err
		}
		if formpackage.DigestBytes(raw) != digest {
			return fmt.Errorf("set archive entry digest drifted: %s", header.Name)
		}
		if _, duplicate := seen[header.Name]; duplicate {
			return fmt.Errorf("duplicate set archive entry %s", header.Name)
		}
		seen[header.Name] = struct{}{}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("set archive is incomplete: got %d want %d", len(seen), len(expected))
	}
	return nil
}

func createStandardSetSBOM(set standardSetManifest, created time.Time) map[string]any {
	files := []map[string]any{}
	sha1Values := []string{}
	for _, pkg := range set.Packages {
		for _, asset := range pkg.Assets {
			digest := strings.TrimPrefix(asset.Digest, "sha256:")
			sha1Sum := sha1.Sum([]byte(pkg.ReleaseID + "\x00" + asset.Name + "\x00" + asset.Digest))
			sha1Values = append(sha1Values, hex.EncodeToString(sha1Sum[:]))
			files = append(files, map[string]any{
				"fileName":         "./packages/" + pkg.ReleaseID + "/" + asset.Name,
				"SPDXID":           "SPDXRef-File-" + spdxID(pkg.ReleaseID+"-"+asset.Name),
				"checksums":        []map[string]string{{"algorithm": "SHA256", "checksumValue": digest}},
				"licenseConcluded": "NOASSERTION", "licenseInfoInFiles": []string{"NOASSERTION"}, "copyrightText": "NOASSERTION",
			})
		}
	}
	sort.Strings(sha1Values)
	verification := sha1.Sum([]byte(strings.Join(sha1Values, "")))
	relationships := []map[string]string{{"spdxElementId": "SPDXRef-DOCUMENT", "relationshipType": "DESCRIBES", "relatedSpdxElement": "SPDXRef-Package"}}
	for _, file := range files {
		relationships = append(relationships, map[string]string{"spdxElementId": "SPDXRef-Package", "relationshipType": "CONTAINS", "relatedSpdxElement": file["SPDXID"].(string)})
	}
	return map[string]any{
		"spdxVersion": "SPDX-2.3", "dataLicense": "CC0-1.0", "SPDXID": "SPDXRef-DOCUMENT",
		"name":              "Takoform standard Form Package set " + set.Version,
		"documentNamespace": "https://forms.takoform.com/spdx/standard-set/" + set.Version + "/" + set.SourceCommit,
		"creationInfo":      map[string]any{"creators": []string{"Tool: takoform-form-package-release"}, "created": created.Format(time.RFC3339)},
		"packages": []map[string]any{{
			"name": "Takoform standard Form Package set", "SPDXID": "SPDXRef-Package", "versionInfo": set.Version,
			"downloadLocation": "NOASSERTION", "filesAnalyzed": true,
			"packageVerificationCode": map[string]string{"packageVerificationCodeValue": hex.EncodeToString(verification[:])},
			"licenseConcluded":        "NOASSERTION", "licenseDeclared": "NOASSERTION", "copyrightText": "NOASSERTION",
		}},
		"files": files, "relationships": relationships,
	}
}

func createControllerCandidate(root string, manifest standardSetReleaseManifest, workflowRunID, builtAt string) (controllerCandidateManifest, error) {
	names, err := regularFileNames(root)
	if err != nil {
		return controllerCandidateManifest{}, err
	}
	assets := []controllerReleaseAsset{}
	for _, name := range names {
		if name == "release-candidate-manifest.json" {
			return controllerCandidateManifest{}, fmt.Errorf("candidate manifest already exists")
		}
		assets = append(assets, controllerReleaseAsset{Name: name, Digest: digestFile(filepath.Join(root, name))})
	}
	artifactDigests := make([]string, len(assets))
	for index := range assets {
		artifactDigests[index] = assets[index].Digest
	}
	find := func(suffix string) (string, error) {
		for _, asset := range assets {
			if strings.HasSuffix(asset.Name, suffix) {
				return asset.Digest, nil
			}
		}
		return "", fmt.Errorf("candidate lacks %s", suffix)
	}
	sbom, err := find("_sbom.spdx.json")
	if err != nil {
		return controllerCandidateManifest{}, err
	}
	provenance, err := find("_provenance.intoto.json")
	if err != nil {
		return controllerCandidateManifest{}, err
	}
	policyRaw, _ := json.Marshal(manifest.PublisherPolicy)
	return controllerCandidateManifest{
		Kind: controllerCandidateKind, SurfaceID: standardSetSurfaceID,
		Repository: "https://" + sourceRepository + ".git", SourceCommit: manifest.SourceCommit,
		Version: manifest.Version, Tag: manifest.Tag, WorkflowRunID: workflowRunID, BuiltAt: builtAt,
		OCIImages: []any{}, ReleaseAssets: assets, ArtifactDigests: artifactDigests,
		SBOMDigests: []string{sbom}, ProvenanceDigests: []string{provenance},
		ConfigDigest:    digestFile(filepath.Join(root, standardSetManifestName)),
		PolicyDigest:    formpackage.DigestBytes(policyRaw),
		ToolchainDigest: formpackage.DigestBytes([]byte("go1.26.5\ncosign-v3.0.6\n")),
	}, nil
}

func verifyStandardSetManifest(set standardSetManifest, version, sourceCommit string) error {
	if set.Format != standardSetFormat || set.Repository != sourceRepository || set.SourceCommit != sourceCommit || set.Version != version ||
		set.Tag != "standard-forms/v"+version || set.PackageCount != len(standardforms.Specs) || len(set.Packages) != len(standardforms.Specs) ||
		set.ActivationAuthority.Type != "immutable-root-release" || set.ActivationAuthority.Tag != set.Tag || set.ActivationAuthority.PartialPackageReleasesActive ||
		set.ActivationAuthority.ReleaseURL != "https://"+sourceRepository+"/releases/tag/"+set.Tag {
		return fmt.Errorf("standard set manifest identity or activation authority is invalid")
	}
	for index, spec := range standardforms.Specs {
		pkg := set.Packages[index]
		if pkg.Kind != spec.Kind || pkg.FormRef.Kind != spec.Kind || pkg.FormRef.DefinitionVersion != version || pkg.Tag != "forms/"+pkg.ReleaseID+"/v"+version ||
			!sha256Pattern.MatchString(pkg.PackageDigest) || !sha256Pattern.MatchString(pkg.ReleaseManifestDigest) || len(pkg.Assets) < 6 {
			return fmt.Errorf("standard set package %d is invalid", index)
		}
	}
	return nil
}

func verifyChecksums(root string) error {
	raw, err := os.ReadFile(filepath.Join(root, "SHA256SUMS"))
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	expected := map[string]string{}
	for _, line := range lines {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(parts[0]) || filepath.Base(parts[1]) != parts[1] || parts[1] == "SHA256SUMS" {
			return fmt.Errorf("invalid SHA256SUMS entry")
		}
		if _, duplicate := expected[parts[1]]; duplicate {
			return fmt.Errorf("duplicate SHA256SUMS entry %s", parts[1])
		}
		expected[parts[1]] = "sha256:" + parts[0]
	}
	names, err := regularFileNames(root)
	if err != nil {
		return err
	}
	for _, name := range names {
		if name == "SHA256SUMS" {
			continue
		}
		if expected[name] != digestFile(filepath.Join(root, name)) {
			return fmt.Errorf("checksum closure differs for %s", name)
		}
		delete(expected, name)
	}
	if len(expected) != 0 {
		return fmt.Errorf("SHA256SUMS names absent files")
	}
	return nil
}

func regularFileNames(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			return nil, fmt.Errorf("non-regular release entry %s", entry.Name())
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func digestFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func decodeStringArray(encoded string) ([]string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode base64url string array: %w", err)
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("digest array must not be empty")
	}
	for _, value := range values {
		if !sha256Pattern.MatchString(value) {
			return nil, fmt.Errorf("digest array contains a non-SHA256 value")
		}
	}
	return values, nil
}

func decodeChecks(encoded string) ([]releaseSafetyCheck, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode base64url health checks: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var checks []releaseSafetyCheck
	if err := decoder.Decode(&checks); err != nil {
		return nil, err
	}
	if len(checks) == 0 {
		return nil, fmt.Errorf("health checks must not be empty")
	}
	return checks, nil
}
