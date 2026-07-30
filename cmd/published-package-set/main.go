package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
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
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	sigstoreroot "github.com/sigstore/sigstore-go/pkg/root"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/formpublication"
	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
)

const (
	setPath                = "admission/v1/published-package-set.json"
	trustPath              = "admission/v1/trust/published-package-trust.json"
	publicationSetPath     = formpublication.SetFilename
	publicationSetFormat   = formpublication.SetFormat
	planVerificationFormat = "takoform.form-package-directory-verification@v1"
	repository             = "tako0614/terraform-provider-takoform"
	maxGitHubResponseBytes = 4 << 20
	maxGitHubAssetBytes    = 64 << 20
	maxPlanAssetBytes      = 512 << 20
	maxReleasePlanBytes    = 1 << 20
	maxTrustedRootBytes    = 4 << 20
	expectedPackageCount   = 10
	expectedPlanCount      = 34
	expectedAssetCount     = 7
	trustedRootSourcePath  = "admission/v4/trust/trusted-root.json"
	trustedRootOutputPath  = "trust/trusted-root.json"
	currentPackageWorkflow = ".github/workflows/form-package-release.yml"
	currentPackageIssuer   = "https://token.actions.githubusercontent.com"
	currentPackageIdentity = "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/heads/main"
	currentPackageTagScope = "refs/tags/forms/k-*/v*"
)

var (
	commitPattern    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	gitObjectPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	versionPattern   = regexp.MustCompile(`^[0-9][0-9A-Za-z.+-]*$`)
)

type candidateSet struct {
	DefinitionVersion string             `json:"definitionVersion"`
	PackageVersion    string             `json:"packageVersion"`
	Packages          []candidatePackage `json:"packages"`
}

type candidatePackage struct {
	Kind          string              `json:"kind"`
	Slug          string              `json:"slug,omitempty"`
	Path          string              `json:"path"`
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}

type releaseManifest struct {
	SchemaVersion       int                    `json:"schemaVersion"`
	ReleaseType         string                 `json:"releaseType"`
	Tag                 string                 `json:"tag"`
	SourceRepository    string                 `json:"sourceRepository"`
	SourceCommit        string                 `json:"sourceCommit"`
	ToolingCommit       string                 `json:"toolingCommit"`
	Workflow            string                 `json:"workflow"`
	PackageVersion      string                 `json:"packageVersion"`
	ReleaseID           string                 `json:"releaseId"`
	PackageDigest       string                 `json:"packageDigest"`
	FormRef             formpackage.FormRef    `json:"formRef"`
	Canonicalization    string                 `json:"canonicalization"`
	SignedSubject       string                 `json:"signedSubject"`
	SignatureBundle     string                 `json:"signatureBundle"`
	SignatureMediaType  string                 `json:"signatureMediaType"`
	PublisherPolicy     releasePublisherPolicy `json:"publisherPolicy"`
	Assets              []releaseAsset         `json:"assets"`
	PublicationReady    bool                   `json:"publicationReady"`
	PublicationBlockers []string               `json:"publicationBlockers"`
}

type releasePublisherPolicy struct {
	OIDCIssuer    string `json:"oidcIssuer"`
	Identity      string `json:"identity"`
	TagPattern    string `json:"tagPattern"`
	ToolingCommit string `json:"toolingCommit"`
}

type releaseAsset struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType,omitempty"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

type packageSBOM struct {
	SPDXVersion       string             `json:"spdxVersion"`
	DataLicense       string             `json:"dataLicense"`
	SPDXID            string             `json:"SPDXID"`
	Name              string             `json:"name"`
	DocumentNamespace string             `json:"documentNamespace"`
	CreationInfo      spdxCreationInfo   `json:"creationInfo"`
	Packages          []spdxPackage      `json:"packages"`
	Files             []spdxFile         `json:"files"`
	Relationships     []spdxRelationship `json:"relationships"`
}

type spdxCreationInfo struct {
	Creators []string `json:"creators"`
	Created  string   `json:"created"`
}

type spdxPackage struct {
	Name                    string                      `json:"name"`
	SPDXID                  string                      `json:"SPDXID"`
	VersionInfo             string                      `json:"versionInfo"`
	DownloadLocation        string                      `json:"downloadLocation"`
	FilesAnalyzed           bool                        `json:"filesAnalyzed"`
	PackageVerificationCode spdxPackageVerificationCode `json:"packageVerificationCode"`
	LicenseConcluded        string                      `json:"licenseConcluded"`
	LicenseDeclared         string                      `json:"licenseDeclared"`
	CopyrightText           string                      `json:"copyrightText"`
}

type spdxPackageVerificationCode struct {
	Value string `json:"packageVerificationCodeValue"`
}

type spdxFile struct {
	FileName           string         `json:"fileName"`
	SPDXID             string         `json:"SPDXID"`
	Checksums          []spdxChecksum `json:"checksums"`
	LicenseConcluded   string         `json:"licenseConcluded"`
	LicenseInfoInFiles []string       `json:"licenseInfoInFiles"`
	CopyrightText      string         `json:"copyrightText"`
}

type spdxChecksum struct {
	Algorithm     string `json:"algorithm"`
	ChecksumValue string `json:"checksumValue"`
}

type spdxRelationship struct {
	SPDXElementID      string `json:"spdxElementId"`
	RelationshipType   string `json:"relationshipType"`
	RelatedSPDXElement string `json:"relatedSpdxElement"`
}

type packageProvenance struct {
	Type          string              `json:"_type"`
	Subject       []provenanceSubject `json:"subject"`
	PredicateType string              `json:"predicateType"`
	Predicate     provenancePredicate `json:"predicate"`
}

type provenanceSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

type provenancePredicate struct {
	BuildDefinition provenanceBuildDefinition `json:"buildDefinition"`
	RunDetails      provenanceRunDetails      `json:"runDetails"`
}

type provenanceBuildDefinition struct {
	BuildType            string                 `json:"buildType"`
	ExternalParameters   map[string]string      `json:"externalParameters"`
	InternalParameters   map[string]string      `json:"internalParameters"`
	ResolvedDependencies []provenanceDependency `json:"resolvedDependencies"`
}

type provenanceDependency struct {
	Name   string            `json:"name"`
	URI    string            `json:"uri"`
	Digest map[string]string `json:"digest"`
}

type provenanceRunDetails struct {
	Builder provenanceBuilder `json:"builder"`
}

type provenanceBuilder struct {
	ID string `json:"id"`
}

type githubRelease struct {
	ID          int64                `json:"id"`
	TagName     string               `json:"tag_name"`
	Draft       bool                 `json:"draft"`
	Prerelease  bool                 `json:"prerelease"`
	Immutable   bool                 `json:"immutable"`
	PublishedAt time.Time            `json:"published_at"`
	Assets      []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type githubGitObject struct {
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

type githubGitRef struct {
	Ref    string          `json:"ref"`
	Object githubGitObject `json:"object"`
}

type githubGitTag struct {
	SHA    string          `json:"sha"`
	Tag    string          `json:"tag"`
	Object githubGitObject `json:"object"`
}

type githubComparison struct {
	Status          string          `json:"status"`
	BaseCommit      githubGitObject `json:"base_commit"`
	MergeBaseCommit githubGitObject `json:"merge_base_commit"`
}

type resolvedGitTag struct {
	ObjectOID    string
	PeeledCommit string
}

type githubClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	token      string
}

type formPackagePublicationSet = formpublication.Set
type publicationSetSourcePlan = formpublication.SourcePlan
type publicationSetVerification = formpublication.VerificationPolicy
type formPackagePublicationEntry = formpublication.Entry
type publicationSetAsset = formpublication.Asset

type planDirectoryVerification struct {
	Format              string                   `json:"format"`
	SemanticStatus      string                   `json:"semanticStatus"`
	CryptographicStatus string                   `json:"cryptographicStatus"`
	Kind                string                   `json:"kind"`
	ReleaseID           string                   `json:"releaseId"`
	Version             string                   `json:"version"`
	Tag                 string                   `json:"tag"`
	SourceCommit        string                   `json:"sourceCommit"`
	ToolingCommit       string                   `json:"toolingCommit"`
	TrustedRoot         planDirectoryTrustedRoot `json:"trustedRoot"`
	Assets              []publicationSetAsset    `json:"assets"`
}

type planDirectoryTrustedRoot struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type downloadedPackageRelease struct {
	live       githubRelease
	manifest   releaseManifest
	names      []string
	assets     map[string]githubReleaseAsset
	assetIDs   []int64
	downloaded map[string][]byte
	totalBytes int64
}

type planAuthorityAtCommit struct {
	root           string
	plan           standardforms.ReleasePlan
	planRaw        []byte
	trustedRootRaw []byte
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "snapshot":
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		err = snapshot()
		if err == nil {
			err = standardforms.VerifyPublishedPackageSet(".")
			if err != nil {
				err = fmt.Errorf("verify snapshot: %w", err)
			}
		}
		if err == nil {
			fmt.Println("published-package-set: snapshot and offline verification passed")
		}
	case "download":
		flags := flag.NewFlagSet("published-package-set download", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		outputRoot := flags.String("output-root", "", "new, absent directory that receives the staged admission/v1 snapshot")
		if parseErr := flags.Parse(os.Args[2:]); parseErr != nil || flags.NArg() != 0 || strings.TrimSpace(*outputRoot) == "" {
			if parseErr == nil {
				usage()
			}
			os.Exit(2)
		}
		client, clientErr := newGitHubClient("https://api.github.com/", os.Getenv("GITHUB_TOKEN"), nil)
		if clientErr != nil {
			err = clientErr
		} else {
			err = downloadSnapshot(context.Background(), client, ".", *outputRoot)
		}
		if err == nil {
			fmt.Printf("published-package-set: staged exact live snapshot at %s\n", *outputRoot)
		}
	case "download-plan":
		flags := flag.NewFlagSet("published-package-set download-plan", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		outputRoot := flags.String("output-root", "", "new, absent directory outside the repository that receives all current planned releases")
		if parseErr := flags.Parse(os.Args[2:]); parseErr != nil || flags.NArg() != 0 || strings.TrimSpace(*outputRoot) == "" {
			if parseErr == nil {
				usage()
			}
			os.Exit(2)
		}
		token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
		if token == "" {
			token = strings.TrimSpace(os.Getenv("GH_TOKEN"))
		}
		if token == "" {
			err = fmt.Errorf("download-plan requires GITHUB_TOKEN or GH_TOKEN for the complete GitHub API readback")
		} else {
			client, clientErr := newGitHubClient("https://api.github.com/", token, nil)
			if clientErr != nil {
				err = clientErr
			} else {
				err = downloadPlan(context.Background(), client, ".", *outputRoot)
			}
		}
		if err == nil {
			fmt.Printf("published-package-set: staged exact current plan publication set at %s\n", *outputRoot)
		}
	case "verify-plan-directory":
		flags := flag.NewFlagSet("published-package-set verify-plan-directory", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		assetRoot := flags.String("asset-root", "", "directory containing exactly one planned release's seven assets")
		sourceRoot := flags.String("source-root", "", "repository root containing the exact current release plan and reviewed package source")
		kind := flags.String("kind", "", "exact planned Form kind")
		tag := flags.String("tag", "", "exact planned forms/<release-id>/v<version> tag")
		sourceCommit := flags.String("source-commit", "", "exact 40-character release source commit")
		toolingCommit := flags.String("tooling-commit", "", "exact 40-character release tooling commit")
		trustedRoot := flags.String("trusted-root", "", "reviewed Sigstore trusted-root JSON used by the caller's cryptographic verifier")
		if parseErr := flags.Parse(os.Args[2:]); parseErr != nil || flags.NArg() != 0 ||
			strings.TrimSpace(*assetRoot) == "" || strings.TrimSpace(*sourceRoot) == "" ||
			strings.TrimSpace(*kind) == "" || strings.TrimSpace(*tag) == "" ||
			strings.TrimSpace(*sourceCommit) == "" || strings.TrimSpace(*toolingCommit) == "" ||
			strings.TrimSpace(*trustedRoot) == "" {
			if parseErr == nil {
				usage()
			}
			os.Exit(2)
		}
		var result planDirectoryVerification
		result, err = verifyPlanDirectory(
			*sourceRoot, *assetRoot, *kind, *tag, *sourceCommit, *toolingCommit, *trustedRoot,
		)
		if err == nil {
			var raw []byte
			raw, err = canonicalJSON(result)
			if err == nil {
				_, err = os.Stdout.Write(append(raw, '\n'))
			}
		}
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "published-package-set:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: published-package-set snapshot | published-package-set download --output-root DIRECTORY | published-package-set download-plan --output-root DIRECTORY | published-package-set verify-plan-directory --asset-root DIRECTORY --source-root REPOSITORY --kind KIND --tag TAG --source-commit SHA --tooling-commit SHA --trusted-root FILE")
}

func newGitHubClient(rawBaseURL, token string, httpClient *http.Client) (*githubClient, error) {
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub API base URL: %w", err)
	}
	if (baseURL.Scheme != "https" && baseURL.Scheme != "http") || baseURL.Host == "" || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("GitHub API base URL must be an absolute HTTP(S) URL without query or fragment")
	}
	if !strings.HasSuffix(baseURL.Path, "/") {
		baseURL.Path += "/"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &githubClient{baseURL: baseURL, httpClient: httpClient, token: strings.TrimSpace(token)}, nil
}

func downloadSnapshot(ctx context.Context, client *githubClient, sourceRoot, outputRoot string) (err error) {
	if client == nil {
		return fmt.Errorf("GitHub client is required")
	}
	// The published set is the retired generation: this command downloads and
	// verifies bytes that already exist, never the current candidate set.
	candidatesRaw, err := os.ReadFile(filepath.Join(sourceRoot, "forms", "retired-package-set.json"))
	if err != nil {
		return err
	}
	var candidates candidateSet
	if err := json.Unmarshal(candidatesRaw, &candidates); err != nil {
		return err
	}
	if len(candidates.Packages) != expectedPackageCount {
		return fmt.Errorf("candidate set contains %d packages, want exactly %d", len(candidates.Packages), expectedPackageCount)
	}
	if candidates.DefinitionVersion == "" || !versionPattern.MatchString(candidates.PackageVersion) {
		return fmt.Errorf("candidate set has an invalid definition/package version")
	}
	return downloadSnapshotForSet(ctx, client, sourceRoot, outputRoot, candidates, setPath, trustPath)
}

type publicationVerifier func(
	repositoryRoot string,
	publicationRoot string,
	trustRoot string,
) (formpublication.Set, error)

func downloadPlan(ctx context.Context, client *githubClient, sourceRoot, outputRoot string) error {
	return downloadPlanWithVerifier(ctx, client, sourceRoot, outputRoot, verifyCurrentPublication)
}

func verifyCurrentPublication(
	repositoryRoot string,
	publicationRoot string,
	trustRoot string,
) (formpublication.Set, error) {
	expected, err := standardforms.CurrentPortableCandidateSet(repositoryRoot)
	if err != nil {
		return formpublication.Set{}, fmt.Errorf("load exact current portable candidate set: %w", err)
	}
	return formpublication.Verify(repositoryRoot, publicationRoot, trustRoot, expected)
}

func downloadPlanWithVerifier(
	ctx context.Context,
	client *githubClient,
	sourceRoot string,
	outputRoot string,
	verify publicationVerifier,
) (err error) {
	if client == nil {
		return fmt.Errorf("GitHub client is required")
	}
	if verify == nil {
		return fmt.Errorf("publication verifier is required")
	}
	planPath := filepath.Join(sourceRoot, filepath.FromSlash(standardforms.ReleasePlanPath))
	planInfo, err := os.Lstat(planPath)
	if err != nil {
		return fmt.Errorf("inspect source release plan: %w", err)
	}
	if planInfo.Mode()&os.ModeSymlink != 0 || !planInfo.Mode().IsRegular() ||
		planInfo.Size() <= 0 || planInfo.Size() > maxReleasePlanBytes {
		return fmt.Errorf("source release plan must be a regular file, not a symlink")
	}
	if err := standardforms.VerifyReleasePlan(sourceRoot); err != nil {
		return fmt.Errorf("verify source release plan: %w", err)
	}
	planRaw, err := os.ReadFile(planPath)
	if err != nil {
		return fmt.Errorf("read source release plan: %w", err)
	}
	if _, err := formpackage.Canonicalize(planRaw); err != nil {
		return fmt.Errorf("source release plan is not I-JSON: %w", err)
	}
	var plan standardforms.ReleasePlan
	decoder := json.NewDecoder(bytes.NewReader(planRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return fmt.Errorf("decode source release plan: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("decode source release plan: %w", err)
	}
	if err := validateCurrentReleasePlan(plan); err != nil {
		return err
	}
	trustedRootRaw, _, trustedRootDigest, err := readTrustedRoot(
		filepath.Join(sourceRoot, filepath.FromSlash(trustedRootSourcePath)),
	)
	if err != nil {
		return fmt.Errorf("read publication trusted root: %w", err)
	}

	outputAbsolute, err := createPublicationOutputRoot(sourceRoot, outputRoot)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			if cleanupErr := os.RemoveAll(outputAbsolute); cleanupErr != nil {
				cleanupErr = fmt.Errorf("remove failed output root: %w", cleanupErr)
				if err == nil {
					err = cleanupErr
				} else {
					err = errors.Join(err, cleanupErr)
				}
			}
		}
	}()

	protectedMainCommit, err := client.fetchProtectedMainCommit(ctx)
	if err != nil {
		return fmt.Errorf("read protected main: %w", err)
	}
	gitObjectFormat, err := verifySourceRepositoryState(
		ctx, sourceRoot, protectedMainCommit, planRaw, trustedRootRaw,
	)
	if err != nil {
		return err
	}
	set := formPackagePublicationSet{
		Format: publicationSetFormat, Generation: plan.Generation, Repository: plan.Repository,
		PublicationStatus: "published-immutable", AdmissionStatus: "external-required",
		RevocationCheckpointStatus: "external-required",
		GitObjectFormat:            gitObjectFormat, ProtectedMainCommit: protectedMainCommit,
		SourcePlan: publicationSetSourcePlan{
			Path: "release-plan.json", SourcePath: standardforms.ReleasePlanPath, SHA256: formpackage.DigestBytes(planRaw),
		},
		VerificationPolicy: publicationSetVerification{
			TrustedRoot: publicationSetSourcePlan{
				Path: trustedRootOutputPath, SourcePath: trustedRootSourcePath, SHA256: trustedRootDigest,
			},
			CertificateIdentity: currentPackageIdentity,
			OIDCIssuer:          currentPackageIssuer,
			BundleMediaType:     "application/vnd.dev.sigstore.bundle.v0.3+json",
		},
		Entries: make([]formPackagePublicationEntry, 0, expectedPlanCount),
	}
	seenReleaseIDs := make(map[int64]string, expectedPlanCount)
	seenAssetIDs := make(map[int64]string, expectedPlanCount*expectedAssetCount)
	reachableCommits := map[string]struct{}{protectedMainCommit: {}}
	verifiedSources := make(map[string]struct{}, expectedPlanCount)
	authorities := make(map[string]planAuthorityAtCommit)
	defer func() {
		for _, authority := range authorities {
			if cleanupErr := os.RemoveAll(authority.root); cleanupErr != nil {
				cleanupErr = fmt.Errorf("remove tooling-commit authority snapshot: %w", cleanupErr)
				if err == nil {
					err = cleanupErr
				} else {
					err = errors.Join(err, cleanupErr)
				}
			}
		}
	}()
	totalBytes := int64(0)
	for _, selected := range plan.Releases {
		release, releaseErr := downloadPackageReleaseAssets(
			ctx, client, selected.Kind, selected.Version,
		)
		if releaseErr != nil {
			return fmt.Errorf("%s: %w", selected.Kind, releaseErr)
		}
		manifest, err := decodeReleaseManifest(release.downloaded["release-manifest.json"])
		if err != nil {
			return fmt.Errorf("%s: %w", selected.Kind, err)
		}
		if !commitPattern.MatchString(manifest.SourceCommit) ||
			!commitPattern.MatchString(manifest.ToolingCommit) {
			return fmt.Errorf("%s: release manifest commits are not exact 40-character identities", selected.Kind)
		}
		resolvedTag, err := client.resolveTag(ctx, selected.Tag)
		if err != nil {
			return fmt.Errorf("%s: resolve exact tag: %w", selected.Kind, err)
		}
		if resolvedTag.PeeledCommit != manifest.SourceCommit {
			return fmt.Errorf("%s: tag peeled commit %s differs from release-manifest sourceCommit %s",
				selected.Kind, resolvedTag.PeeledCommit, manifest.SourceCommit)
		}
		for _, commit := range []string{manifest.SourceCommit, manifest.ToolingCommit} {
			if _, verified := reachableCommits[commit]; verified {
				continue
			}
			if err := client.verifyCommitReachableFrom(ctx, commit, protectedMainCommit); err != nil {
				return fmt.Errorf("%s: commit %s is not reachable from protected main: %w", selected.Kind, commit, err)
			}
			if err := verifyLocalCommitReachable(ctx, sourceRoot, commit, protectedMainCommit); err != nil {
				return fmt.Errorf("%s: local commit %s is not reachable from protected main: %w", selected.Kind, commit, err)
			}
			reachableCommits[commit] = struct{}{}
		}
		if err := verifyLocalCommitReachable(
			ctx, sourceRoot, manifest.SourceCommit, manifest.ToolingCommit,
		); err != nil {
			return fmt.Errorf("%s: source commit is not reachable from tooling commit: %w", selected.Kind, err)
		}
		authority, ok := authorities[manifest.ToolingCommit]
		if !ok {
			authority, err = loadPlanAuthorityAtCommit(ctx, sourceRoot, manifest.ToolingCommit)
			if err != nil {
				return fmt.Errorf("%s: %w", selected.Kind, err)
			}
			authorities[manifest.ToolingCommit] = authority
		}
		planned, err := selectPlannedRelease(authority.plan, selected.Kind, selected.Tag)
		if err != nil {
			return fmt.Errorf("%s: %w", selected.Kind, err)
		}
		sourceKey := manifest.SourceCommit + "\x00" + manifest.ToolingCommit + "\x00" + planned.SourcePath
		if _, verified := verifiedSources[sourceKey]; !verified {
			if err := verifyPlannedSourceAtCommit(
				ctx, sourceRoot, manifest.SourceCommit, manifest.ToolingCommit, planned.SourcePath,
			); err != nil {
				return fmt.Errorf("%s: tagged package source differs from tooling commit: %w", selected.Kind, err)
			}
			verifiedSources[sourceKey] = struct{}{}
		}
		candidate := candidatePackage{
			Kind: planned.Kind, Slug: planned.Slug, Path: planned.SourcePath,
			FormRef: planned.FormRef, PackageDigest: planned.PackageDigest,
		}
		manifest, err = validateDownloadedPackage(
			candidate, planned.Version, planned.ReleaseID, planned.Tag,
			filepath.Join(authority.root, filepath.FromSlash(planned.SourcePath)),
			release.assets, release.downloaded,
		)
		if err != nil {
			return fmt.Errorf("%s: %w", selected.Kind, err)
		}
		release.manifest = manifest
		if err := validateCurrentPackagePublisher(manifest); err != nil {
			return fmt.Errorf("%s: %w", selected.Kind, err)
		}
		authorityPlanPath, authorityTrustedRootPath := retainedAuthorityPaths(manifest.ToolingCommit)
		totalBytes += release.totalBytes
		if totalBytes > maxPlanAssetBytes {
			return fmt.Errorf("complete release-plan asset closure exceeds %d bytes", maxPlanAssetBytes)
		}
		if previous, duplicate := seenReleaseIDs[release.live.ID]; duplicate {
			return fmt.Errorf("GitHub release id %d is shared by %s and %s", release.live.ID, previous, planned.Kind)
		}
		seenReleaseIDs[release.live.ID] = planned.Kind
		for _, assetID := range release.assetIDs {
			if previous, duplicate := seenAssetIDs[assetID]; duplicate {
				return fmt.Errorf("GitHub asset id %d is shared by %s and %s", assetID, previous, planned.Kind)
			}
			seenAssetIDs[assetID] = planned.Kind
		}
		releaseDirectory := filepath.Join(outputAbsolute, "releases", planned.ReleaseID, planned.Version)
		if err := writeDownloadedRelease(releaseDirectory, release); err != nil {
			return fmt.Errorf("%s: retain release assets: %w", planned.Kind, err)
		}
		entry := formPackagePublicationEntry{
			Kind: planned.Kind, ReleaseID: planned.ReleaseID, Version: planned.Version, Tag: planned.Tag,
			SourcePath: planned.SourcePath, FormRef: planned.FormRef, PackageDigest: planned.PackageDigest,
			TagObjectOID: resolvedTag.ObjectOID, PeeledCommit: resolvedTag.PeeledCommit,
			SourceCommit: release.manifest.SourceCommit, ToolingCommit: release.manifest.ToolingCommit,
			ReleasePlan: publicationSetSourcePlan{
				Path: authorityPlanPath, SourcePath: standardforms.ReleasePlanPath,
				SHA256: formpackage.DigestBytes(authority.planRaw),
			},
			TrustedRoot: publicationSetSourcePlan{
				Path: authorityTrustedRootPath, SourcePath: trustedRootSourcePath,
				SHA256: formpackage.DigestBytes(authority.trustedRootRaw),
			},
			GitHubReleaseID: strconv.FormatInt(release.live.ID, 10), PublishedAt: release.live.PublishedAt.UTC().Format(time.RFC3339),
			Immutable: release.live.Immutable, Assets: make([]publicationSetAsset, 0, expectedAssetCount),
		}
		for _, name := range release.names {
			asset := release.assets[name]
			entry.Assets = append(entry.Assets, publicationSetAsset{
				Name: name, SHA256: asset.Digest, Size: asset.Size,
			})
		}
		set.Entries = append(set.Entries, entry)
	}

	for toolingCommit, authority := range authorities {
		planPath, trustedRootPath := retainedAuthorityPaths(toolingCommit)
		for name, raw := range map[string][]byte{
			planPath:        authority.planRaw,
			trustedRootPath: authority.trustedRootRaw,
		} {
			destination := filepath.Join(outputAbsolute, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return err
			}
			if err := writeCreateOnly(destination, raw); err != nil {
				return err
			}
		}
	}

	setRaw, err := json.Marshal(set)
	if err != nil {
		return err
	}
	canonicalSet, err := formpackage.Canonicalize(setRaw)
	if err != nil {
		return err
	}
	if err := writeCreateOnly(filepath.Join(outputAbsolute, publicationSetPath), canonicalSet); err != nil {
		return err
	}
	if err := writeCreateOnly(filepath.Join(outputAbsolute, set.SourcePlan.Path), planRaw); err != nil {
		return err
	}
	if err := os.MkdirAll(
		filepath.Dir(filepath.Join(outputAbsolute, filepath.FromSlash(set.VerificationPolicy.TrustedRoot.Path))),
		0o755,
	); err != nil {
		return err
	}
	if err := writeCreateOnly(
		filepath.Join(outputAbsolute, filepath.FromSlash(set.VerificationPolicy.TrustedRoot.Path)),
		trustedRootRaw,
	); err != nil {
		return err
	}
	trustRoot := filepath.Join(sourceRoot, "admission", "v4")
	if _, err := verify(sourceRoot, outputAbsolute, trustRoot); err != nil {
		return fmt.Errorf("verify staged publication set: %w", err)
	}
	complete = true
	return nil
}

func validateCurrentReleasePlan(plan standardforms.ReleasePlan) error {
	if err := validateReleasePlan(plan); err != nil {
		return err
	}
	if len(plan.Releases) != expectedPlanCount {
		return fmt.Errorf("source release plan is not the exact current %d-release portable plan", expectedPlanCount)
	}
	return nil
}

func validateReleasePlan(plan standardforms.ReleasePlan) error {
	if plan.Format != "takoform.release-plan@v1" || plan.Generation != "portable-v1" ||
		plan.Repository != repository || strings.TrimSpace(plan.Note) == "" ||
		len(plan.Releases) == 0 {
		return fmt.Errorf("source release plan has an invalid portable identity or empty closure")
	}
	seenKinds := make(map[string]struct{}, len(plan.Releases))
	seenReleaseIDs := make(map[string]struct{}, len(plan.Releases))
	seenTags := make(map[string]struct{}, len(plan.Releases))
	for _, planned := range plan.Releases {
		releaseID, err := releaseIDForKind(planned.Kind)
		if err != nil {
			return fmt.Errorf("release plan kind: %w", err)
		}
		wantSourcePath := path.Join("forms", "releases", releaseID, planned.Version)
		wantTag := "forms/" + releaseID + "/v" + planned.Version
		if planned.ReleaseID != releaseID || planned.Tag != wantTag || planned.SourcePath != wantSourcePath ||
			planned.Slug == "" || path.Base(planned.Slug) != planned.Slug ||
			!versionPattern.MatchString(planned.Version) || planned.FormRef.Kind != planned.Kind ||
			planned.FormRef.DefinitionVersion != planned.Version || !formpackage.ValidDigest(planned.PackageDigest) {
			return fmt.Errorf("release plan entry for %q has an invalid exact identity", planned.Kind)
		}
		if _, duplicate := seenKinds[planned.Kind]; duplicate {
			return fmt.Errorf("source release plan duplicates kind %q", planned.Kind)
		}
		if _, duplicate := seenReleaseIDs[planned.ReleaseID]; duplicate {
			return fmt.Errorf("source release plan duplicates release id %q", planned.ReleaseID)
		}
		if _, duplicate := seenTags[planned.Tag]; duplicate {
			return fmt.Errorf("source release plan duplicates tag %q", planned.Tag)
		}
		seenKinds[planned.Kind] = struct{}{}
		seenReleaseIDs[planned.ReleaseID] = struct{}{}
		seenTags[planned.Tag] = struct{}{}
	}
	return nil
}

func validateCurrentPackagePublisher(manifest releaseManifest) error {
	if manifest.Workflow != currentPackageWorkflow ||
		manifest.PublisherPolicy.OIDCIssuer != currentPackageIssuer ||
		manifest.PublisherPolicy.Identity != currentPackageIdentity ||
		manifest.PublisherPolicy.TagPattern != currentPackageTagScope ||
		manifest.PublisherPolicy.ToolingCommit != manifest.ToolingCommit {
		return fmt.Errorf("release manifest publisher is not the protected current Form Package workflow")
	}
	return nil
}

func verifyPlanDirectory(
	sourceRoot, assetRoot, kind, tag, sourceCommit, toolingCommit, trustedRootPath string,
) (planDirectoryVerification, error) {
	if strings.TrimSpace(kind) != kind || kind == "" ||
		strings.TrimSpace(tag) != tag || tag == "" ||
		!commitPattern.MatchString(sourceCommit) || !commitPattern.MatchString(toolingCommit) {
		return planDirectoryVerification{}, fmt.Errorf("kind, tag, source commit, and tooling commit must be exact canonical identities")
	}
	for label, commit := range map[string]string{
		"source commit": sourceCommit, "tooling commit": toolingCommit,
	} {
		resolved, err := gitOutput(
			context.Background(), sourceRoot, "rev-parse", "--verify", commit+"^{commit}",
		)
		if err != nil || strings.TrimSpace(string(resolved)) != commit {
			return planDirectoryVerification{}, fmt.Errorf("%s %s is not an exact local repository commit", label, commit)
		}
	}
	ctx := context.Background()
	if err := verifyLocalCommitReachable(ctx, sourceRoot, sourceCommit, toolingCommit); err != nil {
		return planDirectoryVerification{}, fmt.Errorf("source commit is not reachable from tooling commit: %w", err)
	}

	authorityRoot, err := materializePlanAuthorityAtCommit(ctx, sourceRoot, toolingCommit)
	if err != nil {
		return planDirectoryVerification{}, fmt.Errorf("materialize release authority at tooling commit: %w", err)
	}
	defer os.RemoveAll(authorityRoot)
	plan, _, err := readReleasePlan(authorityRoot)
	if err != nil {
		return planDirectoryVerification{}, fmt.Errorf("read release plan at tooling commit: %w", err)
	}
	planned, err := selectPlannedRelease(plan, kind, tag)
	if err != nil {
		return planDirectoryVerification{}, err
	}
	if err := verifyPlannedSourceAtCommit(
		ctx, sourceRoot, sourceCommit, toolingCommit, planned.SourcePath,
	); err != nil {
		return planDirectoryVerification{}, err
	}

	trustedRootRaw, _, trustedRootDigest, err := readTrustedRoot(trustedRootPath)
	if err != nil {
		return planDirectoryVerification{}, fmt.Errorf("trusted root: %w", err)
	}
	if err := verifyPlanTrustedRootAtCommit(
		ctx, sourceRoot, toolingCommit, trustedRootRaw,
	); err != nil {
		return planDirectoryVerification{}, err
	}
	downloaded, liveAssets, assets, err := readExactReleaseAssetDirectory(
		assetRoot, planned.ReleaseID, planned.Version,
	)
	if err != nil {
		return planDirectoryVerification{}, err
	}
	trustedRootLogicalPath, err := repositoryTrustedRootPath(sourceRoot)
	if err != nil {
		return planDirectoryVerification{}, err
	}
	manifest, err := validateDownloadedPackage(
		candidatePackage{
			Kind: planned.Kind, Slug: planned.Slug, Path: planned.SourcePath,
			FormRef: planned.FormRef, PackageDigest: planned.PackageDigest,
		},
		planned.Version, planned.ReleaseID, planned.Tag,
		filepath.Join(authorityRoot, filepath.FromSlash(planned.SourcePath)),
		liveAssets, downloaded,
	)
	if err != nil {
		return planDirectoryVerification{}, fmt.Errorf("verify exact local release asset closure: %w", err)
	}
	if err := validateCurrentPackagePublisher(manifest); err != nil {
		return planDirectoryVerification{}, err
	}
	if manifest.SourceCommit != sourceCommit || manifest.ToolingCommit != toolingCommit {
		return planDirectoryVerification{}, fmt.Errorf(
			"release manifest commits %s/%s differ from requested source/tooling commits %s/%s",
			manifest.SourceCommit, manifest.ToolingCommit, sourceCommit, toolingCommit,
		)
	}
	return planDirectoryVerification{
		Format: planVerificationFormat, SemanticStatus: "verified",
		CryptographicStatus: "external-required",
		Kind:                planned.Kind, ReleaseID: planned.ReleaseID, Version: planned.Version, Tag: planned.Tag,
		SourceCommit: sourceCommit, ToolingCommit: toolingCommit,
		TrustedRoot: planDirectoryTrustedRoot{Path: trustedRootLogicalPath, SHA256: trustedRootDigest},
		Assets:      assets,
	}, nil
}

func repositoryTrustedRootPath(sourceRoot string) (string, error) {
	sourceAbsolute, err := filepath.Abs(filepath.Clean(sourceRoot))
	if err != nil {
		return "", fmt.Errorf("resolve source root: %w", err)
	}
	sourceAbsolute, err = filepath.EvalSymlinks(sourceAbsolute)
	if err != nil {
		return "", fmt.Errorf("resolve real source root: %w", err)
	}
	return filepath.Join(sourceAbsolute, filepath.FromSlash(trustedRootSourcePath)), nil
}

func selectPlannedRelease(
	plan standardforms.ReleasePlan, kind, tag string,
) (standardforms.PlannedFormRelease, error) {
	var planned standardforms.PlannedFormRelease
	found := false
	for _, candidate := range plan.Releases {
		if candidate.Kind == kind || candidate.Tag == tag {
			if found || candidate.Kind != kind || candidate.Tag != tag {
				return standardforms.PlannedFormRelease{}, fmt.Errorf(
					"kind %q and tag %q do not select one exact release-plan entry", kind, tag,
				)
			}
			planned = candidate
			found = true
		}
	}
	if !found {
		return standardforms.PlannedFormRelease{}, fmt.Errorf(
			"kind %q and tag %q are absent from the tooling-commit release plan", kind, tag,
		)
	}
	return planned, nil
}

func readReleasePlan(sourceRoot string) (standardforms.ReleasePlan, []byte, error) {
	planPath := filepath.Join(sourceRoot, filepath.FromSlash(standardforms.ReleasePlanPath))
	info, err := os.Lstat(planPath)
	if err != nil {
		return standardforms.ReleasePlan{}, nil, fmt.Errorf("inspect source release plan: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return standardforms.ReleasePlan{}, nil, fmt.Errorf("source release plan must be a regular file, not a symlink")
	}
	if info.Size() <= 0 || info.Size() > maxReleasePlanBytes {
		return standardforms.ReleasePlan{}, nil, fmt.Errorf("source release plan size must be within 1..%d bytes", maxReleasePlanBytes)
	}
	if err := standardforms.VerifyReleasePlan(sourceRoot); err != nil {
		return standardforms.ReleasePlan{}, nil, fmt.Errorf("verify source release plan: %w", err)
	}
	raw, err := os.ReadFile(planPath)
	if err != nil {
		return standardforms.ReleasePlan{}, nil, fmt.Errorf("read source release plan: %w", err)
	}
	if int64(len(raw)) != info.Size() {
		return standardforms.ReleasePlan{}, nil, fmt.Errorf("source release plan changed while it was read")
	}
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return standardforms.ReleasePlan{}, nil, fmt.Errorf("source release plan is not I-JSON: %w", err)
	}
	var plan standardforms.ReleasePlan
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return standardforms.ReleasePlan{}, nil, fmt.Errorf("decode source release plan: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return standardforms.ReleasePlan{}, nil, fmt.Errorf("decode source release plan: %w", err)
	}
	if err := validateReleasePlan(plan); err != nil {
		return standardforms.ReleasePlan{}, nil, err
	}
	return plan, raw, nil
}

func materializePlanAuthorityAtCommit(
	ctx context.Context,
	sourceRoot, toolingCommit string,
) (string, error) {
	listing, err := gitOutput(
		ctx, sourceRoot, "ls-tree", "-rz", "--full-tree", toolingCommit, "--",
		standardforms.ReleasePlanPath, "forms/standard-package-set.json", "forms/releases",
	)
	if err != nil {
		return "", err
	}
	root, err := os.MkdirTemp("", "takoform-plan-authority-")
	if err != nil {
		return "", err
	}
	cleanup := func(materializeErr error) (string, error) {
		if removeErr := os.RemoveAll(root); removeErr != nil {
			return "", errors.Join(materializeErr, removeErr)
		}
		return "", materializeErr
	}
	required := map[string]bool{
		standardforms.ReleasePlanPath:     false,
		"forms/standard-package-set.json": false,
	}
	seen := make(map[string]struct{})
	totalBytes := int64(0)
	for _, record := range bytes.Split(listing, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		metadata, pathRaw, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return cleanup(fmt.Errorf("malformed release-authority Git tree entry"))
		}
		fields := strings.Fields(string(metadata))
		sourcePath := string(pathRaw)
		allowed := sourcePath == standardforms.ReleasePlanPath ||
			sourcePath == "forms/standard-package-set.json" ||
			strings.HasPrefix(sourcePath, "forms/releases/")
		if !allowed || path.Clean(sourcePath) != sourcePath ||
			len(fields) != 3 || fields[0] != "100644" || fields[1] != "blob" ||
			!gitObjectPattern.MatchString(fields[2]) {
			return cleanup(fmt.Errorf(
				"release authority %q must be an exact ordinary non-executable Git blob", sourcePath,
			))
		}
		if _, duplicate := seen[sourcePath]; duplicate {
			return cleanup(fmt.Errorf("release authority Git tree duplicates %q", sourcePath))
		}
		seen[sourcePath] = struct{}{}
		if _, ok := required[sourcePath]; ok {
			required[sourcePath] = true
		}
		raw, err := gitOutput(ctx, sourceRoot, "show", toolingCommit+":"+sourcePath)
		if err != nil {
			return cleanup(fmt.Errorf("read release authority %q: %w", sourcePath, err))
		}
		if int64(len(raw)) > maxPlanAssetBytes-totalBytes {
			return cleanup(fmt.Errorf("release authority snapshot exceeds %d bytes", maxPlanAssetBytes))
		}
		totalBytes += int64(len(raw))
		destination := filepath.Join(root, filepath.FromSlash(sourcePath))
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
	for sourcePath, present := range required {
		if !present {
			return cleanup(fmt.Errorf("tooling commit omits release authority %q", sourcePath))
		}
	}
	return root, nil
}

func loadPlanAuthorityAtCommit(
	ctx context.Context, sourceRoot, toolingCommit string,
) (planAuthorityAtCommit, error) {
	root, err := materializePlanAuthorityAtCommit(ctx, sourceRoot, toolingCommit)
	if err != nil {
		return planAuthorityAtCommit{}, fmt.Errorf("materialize tooling-commit release authority: %w", err)
	}
	cleanup := func(loadErr error) (planAuthorityAtCommit, error) {
		if removeErr := os.RemoveAll(root); removeErr != nil {
			return planAuthorityAtCommit{}, errors.Join(loadErr, removeErr)
		}
		return planAuthorityAtCommit{}, loadErr
	}
	plan, planRaw, err := readReleasePlan(root)
	if err != nil {
		return cleanup(fmt.Errorf("read tooling-commit release plan: %w", err))
	}
	trustedRootRaw, err := readPlanTrustedRootAtCommit(ctx, sourceRoot, toolingCommit)
	if err != nil {
		return cleanup(err)
	}
	return planAuthorityAtCommit{
		root: root, plan: plan, planRaw: planRaw, trustedRootRaw: trustedRootRaw,
	}, nil
}

func retainedAuthorityPaths(toolingCommit string) (string, string) {
	root := path.Join("authority", toolingCommit)
	return path.Join(root, "release-plan.json"), path.Join(root, "trusted-root.json")
}

func verifyPlanTrustedRootAtCommit(
	ctx context.Context,
	sourceRoot, toolingCommit string, trustedRootRaw []byte,
) error {
	committed, err := readPlanTrustedRootAtCommit(ctx, sourceRoot, toolingCommit)
	if err != nil {
		return err
	}
	if !bytes.Equal(committed, trustedRootRaw) {
		return fmt.Errorf("trusted-root bytes differ from the exact tooling commit")
	}
	return nil
}

func readPlanTrustedRootAtCommit(
	ctx context.Context, sourceRoot, toolingCommit string,
) ([]byte, error) {
	treeEntry, err := gitOutput(
		ctx, sourceRoot, "ls-tree", "-z", toolingCommit, "--", trustedRootSourcePath,
	)
	if err != nil {
		return nil, fmt.Errorf("inspect trusted root at tooling commit: %w", err)
	}
	record := bytes.TrimSuffix(treeEntry, []byte{0})
	metadata, entryPath, ok := bytes.Cut(record, []byte{'\t'})
	fields := strings.Fields(string(metadata))
	if !ok || string(entryPath) != trustedRootSourcePath || len(fields) != 3 ||
		fields[0] != "100644" || fields[1] != "blob" ||
		!gitObjectPattern.MatchString(fields[2]) {
		return nil, fmt.Errorf("trusted root at tooling commit must be one exact ordinary Git blob")
	}
	committed, err := gitOutput(
		ctx, sourceRoot, "show", toolingCommit+":"+trustedRootSourcePath,
	)
	if err != nil {
		return nil, fmt.Errorf("read trusted root at tooling commit: %w", err)
	}
	if len(committed) == 0 || len(committed) > maxTrustedRootBytes {
		return nil, fmt.Errorf("trusted root at tooling commit has an invalid size")
	}
	if _, err := formpackage.Canonicalize(committed); err != nil {
		return nil, fmt.Errorf("trusted root at tooling commit is not I-JSON: %w", err)
	}
	if _, err := sigstoreroot.NewTrustedRootFromJSON(committed); err != nil {
		return nil, fmt.Errorf("decode Sigstore trusted root at tooling commit: %w", err)
	}
	return committed, nil
}

func readTrustedRoot(name string) ([]byte, string, string, error) {
	absolute, err := filepath.Abs(filepath.Clean(name))
	if err != nil {
		return nil, "", "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, "", "", fmt.Errorf("inspect file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, "", "", fmt.Errorf("file must be regular, not a symlink")
	}
	if info.Size() <= 0 || info.Size() > maxTrustedRootBytes {
		return nil, "", "", fmt.Errorf("file size must be within 1..%d bytes", maxTrustedRootBytes)
	}
	realPath, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, "", "", fmt.Errorf("resolve real path: %w", err)
	}
	if filepath.Clean(realPath) != filepath.Clean(absolute) {
		return nil, "", "", fmt.Errorf("path must not traverse symlinks")
	}
	raw, err := os.ReadFile(absolute)
	if err != nil {
		return nil, "", "", fmt.Errorf("read file: %w", err)
	}
	if int64(len(raw)) != info.Size() {
		return nil, "", "", fmt.Errorf("file changed while it was read")
	}
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return nil, "", "", fmt.Errorf("file is not I-JSON: %w", err)
	}
	if _, err := sigstoreroot.NewTrustedRootFromJSON(raw); err != nil {
		return nil, "", "", fmt.Errorf("decode Sigstore trusted root: %w", err)
	}
	return raw, absolute, formpackage.DigestBytes(raw), nil
}

func readExactReleaseAssetDirectory(
	assetRoot, releaseID, version string,
) (map[string][]byte, map[string]githubReleaseAsset, []publicationSetAsset, error) {
	absolute, err := filepath.Abs(filepath.Clean(assetRoot))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve asset root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("inspect asset root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, nil, nil, fmt.Errorf("asset root must be a real directory, not a symlink")
	}
	realRoot, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve real asset root: %w", err)
	}
	if filepath.Clean(realRoot) != filepath.Clean(absolute) {
		return nil, nil, nil, fmt.Errorf("asset root path must not traverse symlinks")
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read asset root: %w", err)
	}
	if len(entries) != expectedAssetCount {
		return nil, nil, nil, fmt.Errorf("asset root has %d entries, want exact seven-asset closure", len(entries))
	}
	wantNames := canonicalAssetNames(releaseID, version)
	downloaded := make(map[string][]byte, expectedAssetCount)
	liveAssets := make(map[string]githubReleaseAsset, expectedAssetCount)
	assets := make([]publicationSetAsset, 0, expectedAssetCount)
	totalBytes := int64(0)
	for position, entry := range entries {
		if _, ok := wantNames[entry.Name()]; !ok {
			return nil, nil, nil, fmt.Errorf("asset root contains non-canonical entry %q", entry.Name())
		}
		name := filepath.Join(absolute, entry.Name())
		fileInfo, err := os.Lstat(name)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("inspect asset %q: %w", entry.Name(), err)
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
			return nil, nil, nil, fmt.Errorf("asset %q must be a regular file, not a symlink", entry.Name())
		}
		if fileInfo.Size() < 0 || fileInfo.Size() > maxGitHubAssetBytes {
			return nil, nil, nil, fmt.Errorf("asset %q exceeds its size bound", entry.Name())
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read asset %q: %w", entry.Name(), err)
		}
		if int64(len(raw)) != fileInfo.Size() {
			return nil, nil, nil, fmt.Errorf("asset %q changed while it was read", entry.Name())
		}
		totalBytes += int64(len(raw))
		if totalBytes > maxGitHubAssetBytes {
			return nil, nil, nil, fmt.Errorf("local release asset closure exceeds %d bytes", maxGitHubAssetBytes)
		}
		digest := formpackage.DigestBytes(raw)
		downloaded[entry.Name()] = raw
		liveAssets[entry.Name()] = githubReleaseAsset{
			ID: int64(position + 1), Name: entry.Name(), State: "uploaded",
			Size: int64(len(raw)), Digest: digest,
		}
		assets = append(assets, publicationSetAsset{Name: entry.Name(), SHA256: digest, Size: int64(len(raw))})
	}
	for name := range wantNames {
		if _, ok := downloaded[name]; !ok {
			return nil, nil, nil, fmt.Errorf("asset root omits canonical asset %q", name)
		}
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
	return downloaded, liveAssets, assets, nil
}

func canonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return formpackage.Canonicalize(raw)
}

func createPublicationOutputRoot(sourceRoot, outputRoot string) (string, error) {
	cleanOutput := filepath.Clean(outputRoot)
	if cleanOutput == "." || cleanOutput == string(filepath.Separator) {
		return "", fmt.Errorf("output root must be a new dedicated directory")
	}
	sourceAbsolute, err := filepath.Abs(sourceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve source root: %w", err)
	}
	sourceAbsolute, err = filepath.EvalSymlinks(sourceAbsolute)
	if err != nil {
		return "", fmt.Errorf("resolve real source root: %w", err)
	}
	outputAbsolute, err := filepath.Abs(cleanOutput)
	if err != nil {
		return "", fmt.Errorf("resolve output root: %w", err)
	}
	outputParent := filepath.Dir(outputAbsolute)
	parentInfo, err := os.Lstat(outputParent)
	if err != nil {
		return "", fmt.Errorf("inspect output parent: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return "", fmt.Errorf("output parent must be a real directory, not a symlink")
	}
	realParent, err := filepath.EvalSymlinks(outputParent)
	if err != nil {
		return "", fmt.Errorf("resolve real output parent: %w", err)
	}
	if filepath.Clean(realParent) != filepath.Clean(outputParent) {
		return "", fmt.Errorf("output path must not traverse symlinks")
	}
	outputAbsolute = filepath.Join(realParent, filepath.Base(outputAbsolute))
	if outputAbsolute == sourceAbsolute || strings.HasPrefix(outputAbsolute, sourceAbsolute+string(filepath.Separator)) {
		return "", fmt.Errorf("output root must be outside the source repository")
	}
	if _, err := os.Lstat(outputAbsolute); err == nil {
		return "", fmt.Errorf("output root %q already exists", outputRoot)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect output root %q: %w", outputRoot, err)
	}
	if err := os.Mkdir(outputAbsolute, 0o755); err != nil {
		return "", fmt.Errorf("create output root: %w", err)
	}
	return outputAbsolute, nil
}

func verifySourceRepositoryState(
	ctx context.Context,
	sourceRoot string,
	protectedMainCommit string,
	planRaw []byte,
	trustedRootRaw []byte,
) (string, error) {
	objectFormatRaw, err := gitOutput(ctx, sourceRoot, "rev-parse", "--show-object-format")
	if err != nil {
		return "", err
	}
	objectFormat := strings.TrimSpace(string(objectFormatRaw))
	if objectFormat != "sha1" {
		return "", fmt.Errorf("source repository object format is %q, want sha1", objectFormat)
	}
	headRaw, err := gitOutput(ctx, sourceRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(headRaw)) != protectedMainCommit {
		return "", fmt.Errorf("local HEAD does not equal current protected main commit %s", protectedMainCommit)
	}
	shallowRaw, err := gitOutput(ctx, sourceRoot, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(shallowRaw)) != "false" {
		return "", fmt.Errorf("source repository must be non-shallow")
	}
	statusRaw, err := gitOutput(ctx, sourceRoot, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	if len(bytes.TrimSpace(statusRaw)) != 0 {
		return "", fmt.Errorf("source repository must be clean")
	}
	committedPlan, err := gitOutput(ctx, sourceRoot, "show", protectedMainCommit+":"+standardforms.ReleasePlanPath)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(committedPlan, planRaw) {
		return "", fmt.Errorf("source release plan bytes do not equal the plan at protected main")
	}
	committedTrustedRoot, err := gitOutput(
		ctx, sourceRoot, "show", protectedMainCommit+":"+trustedRootSourcePath,
	)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(committedTrustedRoot, trustedRootRaw) {
		return "", fmt.Errorf("trusted-root bytes do not equal the reviewed root at protected main")
	}
	return objectFormat, nil
}

func verifyPlannedSourceAtCommit(
	ctx context.Context,
	sourceRoot string,
	sourceCommit string,
	protectedMainCommit string,
	sourcePath string,
) error {
	if !commitPattern.MatchString(sourceCommit) || !commitPattern.MatchString(protectedMainCommit) {
		return fmt.Errorf("source/main commits must be 40-character SHA-1 values")
	}
	if sourcePath == "" || path.Clean(sourcePath) != sourcePath || strings.HasPrefix(sourcePath, "../") {
		return fmt.Errorf("planned source path is invalid")
	}
	if _, err := gitOutput(ctx, sourceRoot, "cat-file", "-e", sourceCommit+"^{commit}"); err != nil {
		return fmt.Errorf("tagged source commit is unavailable locally: %w", err)
	}
	command := exec.CommandContext(
		ctx, "git", "-C", sourceRoot, "diff", "--quiet", "--no-ext-diff", "--no-textconv",
		sourceCommit, protectedMainCommit, "--", sourcePath,
	)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("planned source path changed between tagged source and protected main: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func verifyLocalCommitReachable(ctx context.Context, sourceRoot, commit, protectedMainCommit string) error {
	if commit == protectedMainCommit {
		return nil
	}
	command := exec.CommandContext(ctx, "git", "-C", sourceRoot, "merge-base", "--is-ancestor", commit, protectedMainCommit)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge-base did not prove ancestry: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func gitOutput(ctx context.Context, sourceRoot string, arguments ...string) ([]byte, error) {
	args := append([]string{"-C", sourceRoot}, arguments...)
	command := exec.CommandContext(ctx, "git", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func downloadSnapshotForSet(ctx context.Context, client *githubClient, sourceRoot, outputRoot string, candidates candidateSet, outputSetPath, outputTrustPath string) (err error) {
	if client == nil {
		return fmt.Errorf("GitHub client is required")
	}
	if len(candidates.Packages) != expectedPackageCount {
		return fmt.Errorf("candidate set contains %d packages, want exactly %d", len(candidates.Packages), expectedPackageCount)
	}
	if candidates.DefinitionVersion == "" || !versionPattern.MatchString(candidates.PackageVersion) {
		return fmt.Errorf("candidate set has an invalid definition/package version")
	}
	trustRaw, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(outputTrustPath)))
	if err != nil {
		return err
	}

	cleanOutput := filepath.Clean(outputRoot)
	if cleanOutput == "." || cleanOutput == string(filepath.Separator) {
		return fmt.Errorf("output root must be a new dedicated directory")
	}
	sourceAbsolute, err := filepath.Abs(sourceRoot)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	sourceAbsolute, err = filepath.EvalSymlinks(sourceAbsolute)
	if err != nil {
		return fmt.Errorf("resolve real source root: %w", err)
	}
	outputAbsolute, err := filepath.Abs(cleanOutput)
	if err != nil {
		return fmt.Errorf("resolve output root: %w", err)
	}
	outputParent, err := filepath.EvalSymlinks(filepath.Dir(outputAbsolute))
	if err != nil {
		return fmt.Errorf("resolve real output parent: %w", err)
	}
	outputAbsolute = filepath.Join(outputParent, filepath.Base(outputAbsolute))
	if outputAbsolute == sourceAbsolute || strings.HasPrefix(outputAbsolute, sourceAbsolute+string(filepath.Separator)) {
		return fmt.Errorf("output root must be outside the source repository")
	}
	if _, err := os.Lstat(cleanOutput); err == nil {
		return fmt.Errorf("output root %q already exists", cleanOutput)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect output root %q: %w", cleanOutput, err)
	}
	parentInfo, err := os.Stat(filepath.Dir(cleanOutput))
	if err != nil {
		return fmt.Errorf("inspect output parent: %w", err)
	}
	if !parentInfo.IsDir() {
		return fmt.Errorf("output parent is not a directory")
	}
	if err := os.Mkdir(cleanOutput, 0o755); err != nil {
		return fmt.Errorf("create output root: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			if cleanupErr := os.RemoveAll(cleanOutput); err == nil && cleanupErr != nil {
				err = fmt.Errorf("remove failed output root: %w", cleanupErr)
			}
		}
	}()

	set := admissionrelease.PublishedPackageSet{
		Format:                     "takoform.published-package-set@v1",
		Repository:                 repository,
		DefinitionVersion:          candidates.DefinitionVersion,
		PackageVersion:             candidates.PackageVersion,
		PublicationStatus:          "published-immutable",
		AdmissionStatus:            "external-required",
		RevocationCheckpointStatus: "external-required",
		Trust: admissionrelease.PublishedPackageTrustRef{
			Path: "trust/published-package-trust.json", Digest: formpackage.DigestBytes(trustRaw),
		},
		Entries: make([]admissionrelease.PublishedPackageEntry, 0, expectedPackageCount),
	}
	seenKinds := make(map[string]struct{}, expectedPackageCount)
	seenReleaseIDs := make(map[int64]string, expectedPackageCount)
	seenAssetIDs := make(map[int64]string, expectedPackageCount*expectedAssetCount)
	for _, candidate := range candidates.Packages {
		if candidate.Kind == "" || candidate.FormRef.Kind != candidate.Kind || !formpackage.ValidDigest(candidate.PackageDigest) {
			return fmt.Errorf("candidate %q has an invalid exact identity", candidate.Kind)
		}
		if _, duplicate := seenKinds[candidate.Kind]; duplicate {
			return fmt.Errorf("candidate set duplicates kind %q", candidate.Kind)
		}
		seenKinds[candidate.Kind] = struct{}{}
		packageVersion := candidates.PackageVersion
		retainedRoot := "admission/v1"
		entry, liveID, assetIDs, packageErr := downloadPackage(ctx, client, sourceRoot, cleanOutput, retainedRoot, candidate, packageVersion)
		if packageErr != nil {
			return fmt.Errorf("%s: %w", candidate.Kind, packageErr)
		}
		if previous, duplicate := seenReleaseIDs[liveID]; duplicate {
			return fmt.Errorf("GitHub release id %d is shared by %s and %s", liveID, previous, candidate.Kind)
		}
		seenReleaseIDs[liveID] = candidate.Kind
		for _, assetID := range assetIDs {
			if previous, duplicate := seenAssetIDs[assetID]; duplicate {
				return fmt.Errorf("GitHub asset id %d is shared by %s and %s", assetID, previous, candidate.Kind)
			}
			seenAssetIDs[assetID] = candidate.Kind
		}
		set.Entries = append(set.Entries, entry)
	}

	setRaw, err := json.Marshal(set)
	if err != nil {
		return err
	}
	canonicalSet, err := formpackage.Canonicalize(setRaw)
	if err != nil {
		return err
	}
	stagedSetPath := filepath.Join(cleanOutput, filepath.FromSlash(outputSetPath))
	if err := os.MkdirAll(filepath.Dir(stagedSetPath), 0o755); err != nil {
		return err
	}
	if err := writeCreateOnly(stagedSetPath, canonicalSet); err != nil {
		return err
	}
	complete = true
	return nil
}

func downloadPackage(
	ctx context.Context,
	client *githubClient,
	sourceRoot string,
	outputRoot string,
	retainedRoot string,
	candidate candidatePackage,
	packageVersion string,
) (admissionrelease.PublishedPackageEntry, int64, []int64, error) {
	releaseID, err := releaseIDForKind(candidate.Kind)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, 0, nil, err
	}
	downloadedRelease, err := downloadPackageRelease(
		ctx, client, candidate, packageVersion,
		filepath.Join(sourceRoot, filepath.FromSlash(candidate.Path)),
	)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, 0, nil, err
	}
	if err := writeDownloadedRelease(
		filepath.Join(outputRoot, filepath.FromSlash(retainedRoot), "releases", releaseID, packageVersion),
		downloadedRelease,
	); err != nil {
		return admissionrelease.PublishedPackageEntry{}, 0, nil, err
	}

	releaseDirectory := path.Join("releases", releaseID, packageVersion)
	manifest := downloadedRelease.manifest
	manifestRaw := downloadedRelease.downloaded["release-manifest.json"]
	checksumsRaw := downloadedRelease.downloaded["SHA256SUMS"]
	entry := admissionrelease.PublishedPackageEntry{
		Kind: candidate.Kind, Slug: candidateSlug(candidate), FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest,
		ReleaseTag: manifest.Tag, ReleaseCommit: manifest.SourceCommit, ReleaseToolingCommit: manifest.ToolingCommit,
		GitHubReleaseID: downloadedRelease.live.ID, PublishedAt: downloadedRelease.live.PublishedAt.UTC().Format(time.RFC3339), Immutable: downloadedRelease.live.Immutable,
		PackageReleaseManifestPath: releaseDirectory + "/release-manifest.json", PackageReleaseManifestDigest: formpackage.DigestBytes(manifestRaw),
		ChecksumsPath: releaseDirectory + "/SHA256SUMS", ChecksumsDigest: formpackage.DigestBytes(checksumsRaw),
		PackageIndexPath: releaseDirectory + "/" + manifest.SignedSubject, PackageIndexSigstoreBundle: releaseDirectory + "/" + manifest.SignatureBundle,
	}
	return entry, downloadedRelease.live.ID, downloadedRelease.assetIDs, nil
}

func downloadPackageRelease(
	ctx context.Context,
	client *githubClient,
	candidate candidatePackage,
	packageVersion string,
	packageRoot string,
) (downloadedPackageRelease, error) {
	releaseID, err := releaseIDForKind(candidate.Kind)
	if err != nil {
		return downloadedPackageRelease{}, err
	}
	release, err := downloadPackageReleaseAssets(ctx, client, candidate.Kind, packageVersion)
	if err != nil {
		return downloadedPackageRelease{}, err
	}
	manifest, err := validateDownloadedPackage(
		candidate, packageVersion, releaseID, release.live.TagName,
		packageRoot, release.assets, release.downloaded,
	)
	if err != nil {
		return downloadedPackageRelease{}, err
	}
	release.manifest = manifest
	return release, nil
}

func downloadPackageReleaseAssets(
	ctx context.Context,
	client *githubClient,
	kind, packageVersion string,
) (downloadedPackageRelease, error) {
	releaseID, err := releaseIDForKind(kind)
	if err != nil {
		return downloadedPackageRelease{}, err
	}
	tag := "forms/" + releaseID + "/v" + packageVersion
	live, err := client.fetchRelease(ctx, tag)
	if err != nil {
		return downloadedPackageRelease{}, err
	}
	assets, assetIDs, err := validateLiveRelease(live, tag, releaseID, packageVersion)
	if err != nil {
		return downloadedPackageRelease{}, err
	}

	names := make([]string, 0, len(assets))
	for name := range assets {
		names = append(names, name)
	}
	sort.Strings(names)
	downloaded := make(map[string][]byte, len(names))
	totalBytes := int64(0)
	for _, name := range names {
		raw, err := client.fetchAsset(ctx, assets[name])
		if err != nil {
			return downloadedPackageRelease{}, fmt.Errorf("download %q: %w", name, err)
		}
		totalBytes += int64(len(raw))
		if totalBytes > maxGitHubAssetBytes {
			return downloadedPackageRelease{}, fmt.Errorf("release asset closure exceeds %d bytes", maxGitHubAssetBytes)
		}
		downloaded[name] = raw
	}
	return downloadedPackageRelease{
		live: live, names: names, assets: assets, assetIDs: assetIDs, downloaded: downloaded,
		totalBytes: totalBytes,
	}, nil
}

func writeDownloadedRelease(directory string, release downloadedPackageRelease) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	for _, name := range release.names {
		if err := writeCreateOnly(filepath.Join(directory, name), release.downloaded[name]); err != nil {
			return err
		}
	}
	return nil
}

func candidateSlug(candidate candidatePackage) string {
	if candidate.Slug != "" {
		return candidate.Slug
	}
	return path.Base(candidate.Path)
}

func validateLiveRelease(live githubRelease, tag, releaseID, packageVersion string) (map[string]githubReleaseAsset, []int64, error) {
	if live.TagName != tag || live.Draft || live.Prerelease || !live.Immutable || live.ID <= 0 || live.PublishedAt.IsZero() {
		return nil, nil, fmt.Errorf("GitHub release is not the exact published immutable release %q", tag)
	}
	if len(live.Assets) != expectedAssetCount {
		return nil, nil, fmt.Errorf("live release asset closure has %d entries, want exactly %d", len(live.Assets), expectedAssetCount)
	}
	wantNames := canonicalAssetNames(releaseID, packageVersion)
	assets := make(map[string]githubReleaseAsset, expectedAssetCount)
	seenIDs := make(map[int64]struct{}, expectedAssetCount)
	ids := make([]int64, 0, expectedAssetCount)
	for _, asset := range live.Assets {
		if _, ok := wantNames[asset.Name]; !ok {
			return nil, nil, fmt.Errorf("live release contains non-canonical asset %q", asset.Name)
		}
		if _, duplicate := assets[asset.Name]; duplicate {
			return nil, nil, fmt.Errorf("live release duplicates asset name %q", asset.Name)
		}
		if asset.ID <= 0 || asset.State != "uploaded" || asset.Size < 0 || asset.Size > maxGitHubAssetBytes || !formpackage.ValidDigest(asset.Digest) {
			return nil, nil, fmt.Errorf("live asset %q has invalid API identity, state, size, or digest", asset.Name)
		}
		if _, duplicate := seenIDs[asset.ID]; duplicate {
			return nil, nil, fmt.Errorf("live release duplicates asset id %d", asset.ID)
		}
		seenIDs[asset.ID] = struct{}{}
		assets[asset.Name] = asset
		ids = append(ids, asset.ID)
	}
	if len(assets) != len(wantNames) {
		return nil, nil, fmt.Errorf("live release does not contain the exact canonical seven-asset closure")
	}
	for name := range wantNames {
		if _, ok := assets[name]; !ok {
			return nil, nil, fmt.Errorf("live release omits canonical asset %q", name)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return assets, ids, nil
}

func canonicalAssetNames(releaseID, packageVersion string) map[string]struct{} {
	base := "takoform-form-" + releaseID + "_" + packageVersion
	return map[string]struct{}{
		"release-manifest.json":               {},
		"SHA256SUMS":                          {},
		base + ".tar.gz":                      {},
		base + "_package-index.json":          {},
		base + "_package-index.sigstore.json": {},
		base + "_provenance.intoto.json":      {},
		base + "_sbom.spdx.json":              {},
	}
}

func validateDownloadedPackage(
	candidate candidatePackage,
	packageVersion, releaseID, tag string,
	packageRoot string,
	liveAssets map[string]githubReleaseAsset,
	downloaded map[string][]byte,
) (releaseManifest, error) {
	manifestRaw, ok := downloaded["release-manifest.json"]
	if !ok {
		return releaseManifest{}, fmt.Errorf("downloaded release omits release-manifest.json")
	}
	manifest, err := decodeReleaseManifest(manifestRaw)
	if err != nil {
		return releaseManifest{}, err
	}
	base := "takoform-form-" + releaseID + "_" + packageVersion
	if manifest.SchemaVersion != 1 || manifest.ReleaseType != "form-package" || manifest.Tag != tag ||
		manifest.SourceRepository != "github.com/"+repository || !commitPattern.MatchString(manifest.SourceCommit) ||
		!commitPattern.MatchString(manifest.ToolingCommit) || manifest.Workflow == "" || manifest.PackageVersion != packageVersion ||
		manifest.ReleaseID != releaseID || manifest.FormRef != candidate.FormRef || manifest.PackageDigest != candidate.PackageDigest ||
		manifest.Canonicalization != "RFC8785" || manifest.SignedSubject != base+"_package-index.json" ||
		manifest.SignatureBundle != base+"_package-index.sigstore.json" ||
		manifest.SignatureMediaType != "application/vnd.dev.sigstore.bundle.v0.3+json" ||
		manifest.PublisherPolicy.OIDCIssuer == "" || manifest.PublisherPolicy.Identity == "" || manifest.PublisherPolicy.TagPattern == "" ||
		manifest.PublisherPolicy.ToolingCommit != manifest.ToolingCommit || !manifest.PublicationReady || len(manifest.PublicationBlockers) != 0 {
		return releaseManifest{}, fmt.Errorf("release manifest does not bind the exact candidate and canonical release identity")
	}
	if len(manifest.Assets) != 5 {
		return releaseManifest{}, fmt.Errorf("release manifest asset closure has %d entries, want exactly 5", len(manifest.Assets))
	}
	wantMediaTypes := map[string]string{
		base + ".tar.gz":                      "application/gzip",
		base + "_package-index.json":          "application/vnd.takoform.package-index.v1+json",
		base + "_package-index.sigstore.json": "application/vnd.dev.sigstore.bundle.v0.3+json",
		base + "_provenance.intoto.json":      "application/vnd.in-toto+json",
		base + "_sbom.spdx.json":              "application/spdx+json",
	}
	manifestAssets := make(map[string]releaseAsset, len(manifest.Assets))
	lastManifestAsset := ""
	for position, asset := range manifest.Assets {
		wantMediaType, ok := wantMediaTypes[asset.Name]
		if !ok || asset.MediaType != wantMediaType || asset.Size < 0 || asset.Size > maxGitHubAssetBytes || !formpackage.ValidDigest(asset.Digest) {
			return releaseManifest{}, fmt.Errorf("release manifest contains invalid asset %q", asset.Name)
		}
		if position > 0 && asset.Name <= lastManifestAsset {
			return releaseManifest{}, fmt.Errorf("release manifest assets are not in strict canonical name order")
		}
		if _, duplicate := manifestAssets[asset.Name]; duplicate {
			return releaseManifest{}, fmt.Errorf("release manifest duplicates asset %q", asset.Name)
		}
		manifestAssets[asset.Name] = asset
		lastManifestAsset = asset.Name
	}
	if len(manifestAssets) != len(wantMediaTypes) {
		return releaseManifest{}, fmt.Errorf("release manifest does not contain the exact canonical five-asset closure")
	}

	for name, live := range liveAssets {
		raw, ok := downloaded[name]
		if !ok || int64(len(raw)) != live.Size || formpackage.DigestBytes(raw) != live.Digest {
			return releaseManifest{}, fmt.Errorf("asset API bytes for %q do not match its name, size, and digest", name)
		}
		if manifestAsset, ok := manifestAssets[name]; ok &&
			(manifestAsset.Size != live.Size || manifestAsset.Digest != live.Digest) {
			return releaseManifest{}, fmt.Errorf("release manifest asset %q differs from the live API identity", name)
		}
	}
	if err := validateChecksums(downloaded["SHA256SUMS"], manifestRaw, manifestAssets); err != nil {
		return releaseManifest{}, err
	}
	indexRaw, index, err := validateDownloadedPackageIndex(packageRoot, candidate, packageVersion, manifest, downloaded)
	if err != nil {
		return releaseManifest{}, err
	}
	if err := validateDownloadedPackageArchive(downloaded[base+".tar.gz"], indexRaw); err != nil {
		return releaseManifest{}, fmt.Errorf("package archive: %w", err)
	}
	if err := validateDownloadedPackageSBOM(downloaded[base+"_sbom.spdx.json"], packageRoot, indexRaw, index, manifest); err != nil {
		return releaseManifest{}, fmt.Errorf("package SBOM: %w", err)
	}
	if err := validateDownloadedPackageProvenance(downloaded[base+"_provenance.intoto.json"], manifest, manifestAssets); err != nil {
		return releaseManifest{}, fmt.Errorf("package provenance: %w", err)
	}
	if err := validateDownloadedSigstoreBundle(downloaded[manifest.SignatureBundle], indexRaw); err != nil {
		return releaseManifest{}, fmt.Errorf("package Sigstore bundle: %w", err)
	}
	return manifest, nil
}

func decodeReleaseManifest(raw []byte) (releaseManifest, error) {
	var manifest releaseManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return releaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return releaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	return manifest, nil
}

func validateChecksums(raw, manifestRaw []byte, manifestAssets map[string]releaseAsset) error {
	if len(raw) == 0 || !bytes.HasSuffix(raw, []byte("\n")) {
		return fmt.Errorf("SHA256SUMS must be non-empty and newline terminated")
	}
	expected := map[string]string{"release-manifest.json": formpackage.DigestBytes(manifestRaw)}
	for name, asset := range manifestAssets {
		expected[name] = asset.Digest
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 6 || len(lines) != len(expected) {
		return fmt.Errorf("SHA256SUMS closure has %d lines, want exactly 6", len(lines))
	}
	seen := make(map[string]struct{}, len(lines))
	lastName := ""
	for position, line := range lines {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || len(parts[0]) != 64 || path.Base(parts[1]) != parts[1] {
			return fmt.Errorf("invalid SHA256SUMS line %q", line)
		}
		if position > 0 && parts[1] <= lastName {
			return fmt.Errorf("SHA256SUMS is not in strict canonical name order")
		}
		want, ok := expected[parts[1]]
		if !ok || "sha256:"+parts[0] != want {
			return fmt.Errorf("SHA256SUMS does not bind %q to the release manifest", parts[1])
		}
		if _, duplicate := seen[parts[1]]; duplicate {
			return fmt.Errorf("SHA256SUMS duplicates %q", parts[1])
		}
		seen[parts[1]] = struct{}{}
		lastName = parts[1]
	}
	return nil
}

func validateDownloadedPackageIndex(
	packageRoot string,
	candidate candidatePackage,
	packageVersion string,
	manifest releaseManifest,
	downloaded map[string][]byte,
) ([]byte, formpackage.PackageIndex, error) {
	indexRaw, ok := downloaded[manifest.SignedSubject]
	if !ok {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("downloaded release omits the signed package index")
	}
	canonical, err := formpackage.Canonicalize(indexRaw)
	if err != nil {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("signed package index is not I-JSON: %w", err)
	}
	if !bytes.Equal(indexRaw, canonical) {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("signed package index bytes are not RFC 8785 canonical")
	}
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("validate signed package index: %w", err)
	}
	if index.FormRef != candidate.FormRef || index.PackageVersion != packageVersion ||
		formpackage.DigestBytes(indexRaw) != candidate.PackageDigest ||
		manifest.FormRef != index.FormRef || manifest.PackageDigest != formpackage.DigestBytes(indexRaw) {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("signed package index does not bind the exact planned package")
	}
	rootInfo, err := os.Lstat(packageRoot)
	if err != nil {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("inspect planned package source: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("planned package source must be a real directory, not a symlink")
	}
	report, err := formpackage.VerifyDirectory(packageRoot)
	if err != nil {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("verify planned package source: %w", err)
	}
	if report.FormRef != candidate.FormRef || report.PackageDigest != candidate.PackageDigest {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("planned package source identity differs from the release plan")
	}
	localIndexPath := filepath.Join(packageRoot, formpackage.PackageIndexFilename)
	localInfo, err := os.Lstat(localIndexPath)
	if err != nil {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("inspect planned package index: %w", err)
	}
	if localInfo.Mode()&os.ModeSymlink != 0 || !localInfo.Mode().IsRegular() {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("planned package index must be a regular file, not a symlink")
	}
	localIndex, err := os.ReadFile(localIndexPath)
	if err != nil {
		return nil, formpackage.PackageIndex{}, err
	}
	localCanonical, err := formpackage.Canonicalize(localIndex)
	if err != nil || !bytes.Equal(indexRaw, localCanonical) {
		return nil, formpackage.PackageIndex{}, fmt.Errorf("signed package index bytes differ from the reviewed release source")
	}
	return indexRaw, index, nil
}

func validateDownloadedPackageArchive(archiveRaw, canonicalIndex []byte) error {
	index, err := formpackage.ValidatePackageIndex(canonicalIndex)
	if err != nil {
		return err
	}
	compressed := bytes.NewReader(archiveRaw)
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		return err
	}
	defer reader.Close()
	if !reader.ModTime.IsZero() || reader.OS != 255 || reader.Name != "" || reader.Comment != "" || len(reader.Extra) != 0 {
		return fmt.Errorf("gzip header is not deterministic")
	}
	reader.Multistream(false)
	type expectedFile struct {
		name   string
		size   int64
		digest string
		raw    []byte
	}
	expected := []expectedFile{{
		name: formpackage.PackageIndexFilename, size: int64(len(canonicalIndex)),
		digest: formpackage.DigestBytes(canonicalIndex), raw: canonicalIndex,
	}}
	files := append([]formpackage.PackageFile(nil), index.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		expected = append(expected, expectedFile{name: file.Path, size: file.Size, digest: file.Digest})
	}
	tarReader := tar.NewReader(reader)
	for position, want := range expected {
		header, err := tarReader.Next()
		if err != nil {
			return fmt.Errorf("entry %d %q: %w", position, want.name, err)
		}
		epoch := time.Unix(0, 0).UTC()
		expectedPAX := map[string]string{"atime": "0", "ctime": "0"}
		if header.Name != want.name || header.Typeflag != tar.TypeReg || header.Mode != 0o644 || header.Size != want.size ||
			!header.ModTime.Equal(epoch) || !header.AccessTime.Equal(epoch) || !header.ChangeTime.Equal(epoch) ||
			header.Uid != 0 || header.Gid != 0 || header.Uname != "" || header.Gname != "" || header.Linkname != "" ||
			header.Devmajor != 0 || header.Devminor != 0 || header.Format != tar.FormatPAX ||
			!reflect.DeepEqual(header.PAXRecords, expectedPAX) || len(header.Xattrs) != 0 {
			return fmt.Errorf("entry %d is not the deterministic regular file %q", position, want.name)
		}
		payload, err := io.ReadAll(io.LimitReader(tarReader, want.size+1))
		if err != nil {
			return err
		}
		if int64(len(payload)) != want.size || formpackage.DigestBytes(payload) != want.digest ||
			(want.raw != nil && !bytes.Equal(payload, want.raw)) {
			return fmt.Errorf("entry %q payload does not match the signed package index", want.name)
		}
	}
	if header, err := tarReader.Next(); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return fmt.Errorf("archive contains unlisted entry %q", header.Name)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("finish gzip member: %w", err)
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("close gzip member: %w", err)
	}
	if compressed.Len() != 0 {
		return fmt.Errorf("archive contains %d trailing bytes or an additional gzip member", compressed.Len())
	}
	return nil
}

func validateDownloadedPackageSBOM(
	raw []byte,
	packageRoot string,
	canonicalIndex []byte,
	index formpackage.PackageIndex,
	manifest releaseManifest,
) error {
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return fmt.Errorf("invalid RFC 8785 I-JSON: %w", err)
	}
	if !bytes.Equal(raw, canonical) {
		return fmt.Errorf("bytes are not RFC 8785 canonical")
	}
	var document packageSBOM
	if err := decodeStrictJSON(raw, &document); err != nil {
		return fmt.Errorf("strict SPDX document: %w", err)
	}
	if document.SPDXVersion != "SPDX-2.3" || document.DataLicense != "CC0-1.0" || document.SPDXID != "SPDXRef-DOCUMENT" ||
		document.Name != "Takoform Form Package "+manifest.FormRef.Kind+" "+manifest.PackageVersion ||
		document.DocumentNamespace != "https://forms.takoform.com/spdx/package/"+strings.TrimPrefix(manifest.PackageDigest, "sha256:") ||
		!reflect.DeepEqual(document.CreationInfo.Creators, []string{"Tool: takoform-form-package-release"}) {
		return fmt.Errorf("document identity does not bind the exact FormRef and package digest")
	}
	if _, err := time.Parse(time.RFC3339, document.CreationInfo.Created); err != nil {
		return fmt.Errorf("creationInfo.created is not RFC 3339: %w", err)
	}
	if len(document.Packages) != 1 {
		return fmt.Errorf("package closure has %d entries, want exactly 1", len(document.Packages))
	}
	verificationCode, err := packageVerificationCodeFromSource(packageRoot, canonicalIndex, index)
	if err != nil {
		return err
	}
	wantPackage := spdxPackage{
		Name: manifest.FormRef.Kind, SPDXID: "SPDXRef-Package", VersionInfo: manifest.PackageVersion,
		DownloadLocation: "NOASSERTION", FilesAnalyzed: true,
		PackageVerificationCode: spdxPackageVerificationCode{Value: verificationCode},
		LicenseConcluded:        "NOASSERTION", LicenseDeclared: "NOASSERTION", CopyrightText: "NOASSERTION",
	}
	if document.Packages[0] != wantPackage {
		return fmt.Errorf("SPDX package does not bind the exact package identity and file verification code")
	}
	expectedFiles := make([]struct {
		path   string
		digest string
	}, 0, len(index.Files)+1)
	expectedFiles = append(expectedFiles, struct {
		path   string
		digest string
	}{path: formpackage.PackageIndexFilename, digest: formpackage.DigestBytes(canonicalIndex)})
	for _, file := range index.Files {
		expectedFiles = append(expectedFiles, struct {
			path   string
			digest string
		}{path: file.Path, digest: file.Digest})
	}
	if len(document.Files) != len(expectedFiles) {
		return fmt.Errorf("file closure has %d entries, want %d", len(document.Files), len(expectedFiles))
	}
	seenIDs := make(map[string]struct{}, len(document.Files))
	wantRelationships := make([]spdxRelationship, 0, len(document.Files)+1)
	wantRelationships = append(wantRelationships, spdxRelationship{
		SPDXElementID: "SPDXRef-DOCUMENT", RelationshipType: "DESCRIBES", RelatedSPDXElement: "SPDXRef-Package",
	})
	for position, expected := range expectedFiles {
		file := document.Files[position]
		digest := strings.TrimPrefix(expected.digest, "sha256:")
		wantID := "SPDXRef-File-" + releaseSPDXID(expected.path) + "-" + digest[:12]
		if file.FileName != "./"+expected.path || file.SPDXID != wantID ||
			!reflect.DeepEqual(file.Checksums, []spdxChecksum{{Algorithm: "SHA256", ChecksumValue: digest}}) ||
			file.LicenseConcluded != "NOASSERTION" || !reflect.DeepEqual(file.LicenseInfoInFiles, []string{"NOASSERTION"}) ||
			file.CopyrightText != "NOASSERTION" {
			return fmt.Errorf("file entry %d does not bind %q and its exact SHA-256", position, expected.path)
		}
		if _, duplicate := seenIDs[file.SPDXID]; duplicate {
			return fmt.Errorf("duplicate SPDX file id %q", file.SPDXID)
		}
		seenIDs[file.SPDXID] = struct{}{}
		wantRelationships = append(wantRelationships, spdxRelationship{
			SPDXElementID: "SPDXRef-Package", RelationshipType: "CONTAINS", RelatedSPDXElement: wantID,
		})
	}
	if !reflect.DeepEqual(document.Relationships, wantRelationships) {
		return fmt.Errorf("document relationship closure does not exactly DESCRIBE the package and CONTAIN every file in order")
	}
	return nil
}

func packageVerificationCodeFromSource(packageRoot string, canonicalIndex []byte, index formpackage.PackageIndex) (string, error) {
	digests := make([]string, 0, len(index.Files)+1)
	appendDigest := func(raw []byte) {
		digest := sha1.Sum(raw)
		digests = append(digests, hex.EncodeToString(digest[:]))
	}
	appendDigest(canonicalIndex)
	for _, file := range index.Files {
		name := filepath.Join(packageRoot, filepath.FromSlash(file.Path))
		info, err := os.Lstat(name)
		if err != nil {
			return "", fmt.Errorf("inspect package file %q for SPDX verification: %w", file.Path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", fmt.Errorf("package file %q must be regular, not a symlink", file.Path)
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return "", fmt.Errorf("read package file %q for SPDX verification: %w", file.Path, err)
		}
		if int64(len(raw)) != file.Size || formpackage.DigestBytes(raw) != file.Digest {
			return "", fmt.Errorf("package file %q drifted from the signed index", file.Path)
		}
		appendDigest(raw)
	}
	sort.Strings(digests)
	code := sha1.Sum([]byte(strings.Join(digests, "")))
	return hex.EncodeToString(code[:]), nil
}

func releaseSPDXID(value string) string {
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

func validateDownloadedPackageProvenance(raw []byte, manifest releaseManifest, assets map[string]releaseAsset) error {
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return fmt.Errorf("invalid RFC 8785 I-JSON: %w", err)
	}
	if !bytes.Equal(raw, canonical) {
		return fmt.Errorf("bytes are not RFC 8785 canonical")
	}
	var statement packageProvenance
	if err := decodeStrictJSON(raw, &statement); err != nil {
		return fmt.Errorf("strict in-toto statement: %w", err)
	}
	archiveName := strings.TrimSuffix(manifest.SignedSubject, "_package-index.json") + ".tar.gz"
	expectedSubjects := make([]provenanceSubject, 0, 2)
	for _, name := range []string{manifest.SignedSubject, archiveName} {
		asset, ok := assets[name]
		if !ok {
			return fmt.Errorf("required provenance subject %q is absent from the release", name)
		}
		expectedSubjects = append(expectedSubjects, provenanceSubject{
			Name: name, Digest: map[string]string{"sha256": strings.TrimPrefix(asset.Digest, "sha256:")},
		})
	}
	sort.Slice(expectedSubjects, func(i, j int) bool { return expectedSubjects[i].Name < expectedSubjects[j].Name })
	wantPredicate := provenancePredicate{
		BuildDefinition: provenanceBuildDefinition{
			BuildType:          "https://forms.takoform.com/buildtypes/data-release/v1",
			ExternalParameters: map[string]string{"tag": manifest.Tag},
			InternalParameters: map[string]string{"canonicalization": "RFC8785"},
			ResolvedDependencies: []provenanceDependency{{
				Name: "tagged-release-source", URI: "git+https://" + manifest.SourceRepository,
				Digest: map[string]string{"gitCommit": manifest.SourceCommit},
			}, {
				Name: "protected-main-release-tooling", URI: "git+https://" + manifest.SourceRepository,
				Digest: map[string]string{"gitCommit": manifest.ToolingCommit},
			}},
		},
		RunDetails: provenanceRunDetails{Builder: provenanceBuilder{
			ID: "https://" + manifest.SourceRepository + "/" + manifest.Workflow + "@" + manifest.ToolingCommit,
		}},
	}
	if statement.Type != "https://in-toto.io/Statement/v1" || statement.PredicateType != "https://slsa.dev/provenance/v1" ||
		!reflect.DeepEqual(statement.Subject, expectedSubjects) || !reflect.DeepEqual(statement.Predicate, wantPredicate) {
		return fmt.Errorf("statement does not bind the exact index/archive, source, tag, commit, workflow, and canonicalization")
	}
	return nil
}

func validateDownloadedSigstoreBundle(raw, subject []byte) error {
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return fmt.Errorf("bundle is not I-JSON: %w", err)
	}
	var bundle map[string]any
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return fmt.Errorf("decode bundle: %w", err)
	}
	if bundle["mediaType"] != "application/vnd.dev.sigstore.bundle.v0.3+json" {
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
	if _, err := decodeNonEmptyBase64(certificate["rawBytes"]); err != nil {
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
		if _, err := decodeNonEmptyBase64(entry["canonicalizedBody"]); err != nil {
			return fmt.Errorf("bundle transparency-log entry %d body: %w", position, err)
		}
		proof, ok := entry["inclusionProof"].(map[string]any)
		if !ok {
			return fmt.Errorf("bundle transparency-log entry %d lacks inclusionProof", position)
		}
		rootHash, err := decodeNonEmptyBase64(proof["rootHash"])
		if err != nil || len(rootHash) != sha256.Size {
			return fmt.Errorf("bundle transparency-log entry %d has invalid rootHash", position)
		}
		if stringField(proof, "logIndex") == "" || stringField(proof, "treeSize") == "" {
			return fmt.Errorf("bundle transparency-log entry %d lacks logIndex/treeSize", position)
		}
		checkpoint, ok := proof["checkpoint"].(map[string]any)
		if !ok || stringField(checkpoint, "envelope") == "" {
			return fmt.Errorf("bundle transparency-log entry %d lacks checkpoint", position)
		}
		hashes, ok := proof["hashes"].([]any)
		if !ok {
			return fmt.Errorf("bundle transparency-log entry %d lacks inclusion hashes", position)
		}
		for hashPosition, encoded := range hashes {
			hash, err := decodeNonEmptyBase64(encoded)
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
	if !ok || stringField(messageDigest, "algorithm") != "SHA2_256" {
		return fmt.Errorf("bundle message digest algorithm is not SHA2_256")
	}
	boundDigest, err := decodeNonEmptyBase64(messageDigest["digest"])
	if err != nil {
		return fmt.Errorf("bundle message digest: %w", err)
	}
	expectedDigest := sha256.Sum256(subject)
	if !bytes.Equal(boundDigest, expectedDigest[:]) {
		return fmt.Errorf("bundle message digest does not bind the signed package index")
	}
	if _, err := decodeNonEmptyBase64(messageSignature["signature"]); err != nil {
		return fmt.Errorf("bundle message signature: %w", err)
	}
	return nil
}

func decodeNonEmptyBase64(value any) ([]byte, error) {
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

func stringField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func decodeStrictJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("contains trailing JSON value")
		}
		return err
	}
	return nil
}

func writeCreateOnly(name string, raw []byte) error {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(name)
		}
	}()
	if _, err := file.Write(raw); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", name, err)
	}
	remove = false
	return nil
}

func (client *githubClient) fetchRelease(ctx context.Context, tag string) (githubRelease, error) {
	relative := "repos/" + repository + "/releases/tags/" + url.PathEscape(tag)
	raw, err := client.get(ctx, relative, "application/vnd.github+json", maxGitHubResponseBytes)
	if err != nil {
		return githubRelease{}, err
	}
	var release githubRelease
	if err := decodeGitHubJSON(raw, &release); err != nil {
		return githubRelease{}, err
	}
	return release, nil
}

func (client *githubClient) fetchAsset(ctx context.Context, asset githubReleaseAsset) ([]byte, error) {
	relative := fmt.Sprintf("repos/%s/releases/assets/%d", repository, asset.ID)
	raw, err := client.get(ctx, relative, "application/octet-stream", maxGitHubAssetBytes)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) != asset.Size || formpackage.DigestBytes(raw) != asset.Digest {
		return nil, fmt.Errorf("asset API response for %q does not match size/digest", asset.Name)
	}
	return raw, nil
}

func (client *githubClient) fetchProtectedMainCommit(ctx context.Context) (string, error) {
	ref, err := client.fetchGitRef(ctx, "refs/heads/main")
	if err != nil {
		return "", err
	}
	if ref.Object.Type != "commit" || !commitPattern.MatchString(ref.Object.SHA) {
		return "", fmt.Errorf("protected main ref does not point to an exact repository commit")
	}
	return ref.Object.SHA, nil
}

func (client *githubClient) resolveTag(ctx context.Context, tag string) (resolvedGitTag, error) {
	ref, err := client.fetchGitRef(ctx, "refs/tags/"+tag)
	if err != nil {
		return resolvedGitTag{}, err
	}
	if ref.Object.Type != "tag" || !commitPattern.MatchString(ref.Object.SHA) {
		return resolvedGitTag{}, fmt.Errorf("tag ref must point to one exact annotated SHA-1 tag object")
	}
	tagObject, err := client.fetchGitTag(ctx, ref.Object.SHA)
	if err != nil {
		return resolvedGitTag{}, err
	}
	if tagObject.SHA != ref.Object.SHA || tagObject.Tag != tag ||
		tagObject.Object.Type != "commit" || !commitPattern.MatchString(tagObject.Object.SHA) {
		return resolvedGitTag{}, fmt.Errorf("annotated tag object does not directly bind %q to one commit", tag)
	}
	return resolvedGitTag{ObjectOID: ref.Object.SHA, PeeledCommit: tagObject.Object.SHA}, nil
}

func (client *githubClient) verifyCommitReachableFrom(ctx context.Context, commit, protectedMainCommit string) error {
	if !commitPattern.MatchString(commit) || !commitPattern.MatchString(protectedMainCommit) {
		return fmt.Errorf("commit identities must be 40-character SHA-1 values")
	}
	if commit == protectedMainCommit {
		return nil
	}
	relative := fmt.Sprintf("repos/%s/compare/%s...%s", repository, commit, protectedMainCommit)
	raw, err := client.get(ctx, relative, "application/vnd.github+json", maxGitHubResponseBytes)
	if err != nil {
		return err
	}
	var comparison githubComparison
	if err := decodeGitHubJSON(raw, &comparison); err != nil {
		return fmt.Errorf("decode GitHub comparison: %w", err)
	}
	if comparison.BaseCommit.SHA != commit || comparison.MergeBaseCommit.SHA != commit ||
		(comparison.Status != "ahead" && comparison.Status != "identical") {
		return fmt.Errorf("GitHub comparison status %q does not prove commit ancestry", comparison.Status)
	}
	return nil
}

func (client *githubClient) fetchGitRef(ctx context.Context, ref string) (githubGitRef, error) {
	if !strings.HasPrefix(ref, "refs/") || strings.TrimPrefix(ref, "refs/") == "" {
		return githubGitRef{}, fmt.Errorf("Git ref must begin with refs/")
	}
	relative := "repos/" + repository + "/git/ref/" + url.PathEscape(strings.TrimPrefix(ref, "refs/"))
	raw, err := client.get(ctx, relative, "application/vnd.github+json", maxGitHubResponseBytes)
	if err != nil {
		return githubGitRef{}, err
	}
	var result githubGitRef
	if err := decodeGitHubJSON(raw, &result); err != nil {
		return githubGitRef{}, fmt.Errorf("decode GitHub ref: %w", err)
	}
	if result.Ref != ref || !commitPattern.MatchString(result.Object.SHA) {
		return githubGitRef{}, fmt.Errorf("GitHub ref response does not bind %q", ref)
	}
	return result, nil
}

func (client *githubClient) fetchGitTag(ctx context.Context, objectID string) (githubGitTag, error) {
	if !commitPattern.MatchString(objectID) {
		return githubGitTag{}, fmt.Errorf("Git tag object id is not a 40-character SHA-1")
	}
	relative := "repos/" + repository + "/git/tags/" + objectID
	raw, err := client.get(ctx, relative, "application/vnd.github+json", maxGitHubResponseBytes)
	if err != nil {
		return githubGitTag{}, err
	}
	var result githubGitTag
	if err := decodeGitHubJSON(raw, &result); err != nil {
		return githubGitTag{}, fmt.Errorf("decode GitHub tag object: %w", err)
	}
	return result, nil
}

func decodeGitHubJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(value); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func (client *githubClient) get(ctx context.Context, relative, accept string, limit int64) ([]byte, error) {
	requestURL, err := client.baseURL.Parse(relative)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "takoform-published-package-set")
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("GitHub response exceeds %d bytes", limit)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub readback returned %s", response.Status)
	}
	return raw, nil
}

func snapshot() error {
	candidatesRaw, err := os.ReadFile("forms/standard-package-set.json")
	if err != nil {
		return err
	}
	var candidates candidateSet
	if err := json.Unmarshal(candidatesRaw, &candidates); err != nil {
		return err
	}
	trustRaw, err := os.ReadFile(trustPath)
	if err != nil {
		return err
	}
	set := admissionrelease.PublishedPackageSet{
		Format:                     "takoform.published-package-set@v1",
		Repository:                 repository,
		DefinitionVersion:          candidates.DefinitionVersion,
		PackageVersion:             candidates.PackageVersion,
		PublicationStatus:          "published-immutable",
		AdmissionStatus:            "external-required",
		RevocationCheckpointStatus: "external-required",
		Trust: admissionrelease.PublishedPackageTrustRef{
			Path: "trust/published-package-trust.json", Digest: formpackage.DigestBytes(trustRaw),
		},
		Entries: make([]admissionrelease.PublishedPackageEntry, 0, len(candidates.Packages)),
	}
	for _, candidate := range candidates.Packages {
		entry, err := snapshotPackage(candidate, candidates.PackageVersion)
		if err != nil {
			return fmt.Errorf("%s: %w", candidate.Kind, err)
		}
		set.Entries = append(set.Entries, entry)
	}
	raw, err := json.Marshal(set)
	if err != nil {
		return err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return err
	}
	temporary := setPath + ".tmp"
	if err := os.WriteFile(temporary, canonical, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, setPath)
}

func snapshotPackage(candidate candidatePackage, packageVersion string) (admissionrelease.PublishedPackageEntry, error) {
	releaseID, err := releaseIDForKind(candidate.Kind)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, err
	}
	releaseDirectory := path.Join("releases", releaseID, packageVersion)
	retainedDirectory := filepath.Join("admission", "v1", filepath.FromSlash(releaseDirectory))
	manifestPath := filepath.Join(retainedDirectory, "release-manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, err
	}
	var manifest releaseManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return admissionrelease.PublishedPackageEntry{}, err
	}
	if manifest.ReleaseID != releaseID || manifest.PackageVersion != packageVersion || manifest.FormRef != candidate.FormRef ||
		manifest.PackageDigest != candidate.PackageDigest {
		return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("release manifest does not bind the candidate")
	}
	live, err := fetchGitHubRelease(manifest.Tag)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, err
	}
	if live.TagName != manifest.Tag || live.Draft || live.Prerelease || !live.Immutable || live.ID <= 0 || live.PublishedAt.IsZero() {
		return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("GitHub release is not a published immutable release")
	}
	expectedAssets := map[string]releaseAsset{
		"release-manifest.json": {Name: "release-manifest.json", Size: int64(len(manifestRaw)), Digest: formpackage.DigestBytes(manifestRaw)},
	}
	checksumsRaw, err := os.ReadFile(filepath.Join(retainedDirectory, "SHA256SUMS"))
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, err
	}
	expectedAssets["SHA256SUMS"] = releaseAsset{Name: "SHA256SUMS", Size: int64(len(checksumsRaw)), Digest: formpackage.DigestBytes(checksumsRaw)}
	for _, asset := range manifest.Assets {
		if _, duplicate := expectedAssets[asset.Name]; duplicate {
			return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("release manifest duplicates asset %q", asset.Name)
		}
		expectedAssets[asset.Name] = asset
	}
	if len(expectedAssets) != 7 || len(live.Assets) != len(expectedAssets) {
		return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("live release asset closure has %d entries, want exactly 7", len(live.Assets))
	}
	seen := make(map[string]struct{}, len(live.Assets))
	for _, asset := range live.Assets {
		want, ok := expectedAssets[asset.Name]
		if !ok || asset.Size != want.Size || asset.Digest != want.Digest {
			return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("live asset %q does not match retained bytes", asset.Name)
		}
		localRaw, err := os.ReadFile(filepath.Join(retainedDirectory, asset.Name))
		if err != nil {
			return admissionrelease.PublishedPackageEntry{}, err
		}
		if int64(len(localRaw)) != asset.Size || formpackage.DigestBytes(localRaw) != asset.Digest {
			return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("retained asset %q does not match GitHub digest", asset.Name)
		}
		seen[asset.Name] = struct{}{}
	}
	localEntries, err := os.ReadDir(retainedDirectory)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, err
	}
	localNames := make([]string, 0, len(localEntries))
	for _, entry := range localEntries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("retained release contains non-regular entry %q", entry.Name())
		}
		localNames = append(localNames, entry.Name())
	}
	sort.Strings(localNames)
	if len(localNames) != len(seen) {
		return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("retained release inventory is not the exact live seven-asset closure")
	}
	for _, name := range localNames {
		if _, ok := seen[name]; !ok {
			return admissionrelease.PublishedPackageEntry{}, fmt.Errorf("retained release has unlisted asset %q", name)
		}
	}
	return admissionrelease.PublishedPackageEntry{
		Kind: candidate.Kind, Slug: path.Base(candidate.Path), FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest,
		ReleaseTag: manifest.Tag, ReleaseCommit: manifest.SourceCommit, ReleaseToolingCommit: manifest.ToolingCommit,
		GitHubReleaseID: live.ID, PublishedAt: live.PublishedAt.UTC().Format(time.RFC3339), Immutable: live.Immutable,
		PackageReleaseManifestPath:   releaseDirectory + "/release-manifest.json",
		PackageReleaseManifestDigest: formpackage.DigestBytes(manifestRaw),
		ChecksumsPath:                releaseDirectory + "/SHA256SUMS", ChecksumsDigest: formpackage.DigestBytes(checksumsRaw),
		PackageIndexPath:           releaseDirectory + "/" + manifest.SignedSubject,
		PackageIndexSigstoreBundle: releaseDirectory + "/" + manifest.SignatureBundle,
	}, nil
}

func fetchGitHubRelease(tag string) (githubRelease, error) {
	requestURL := "https://api.github.com/repos/" + repository + "/releases/tags/" + url.PathEscape(tag)
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return githubRelease{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "takoform-published-package-set")
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return githubRelease{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxGitHubResponseBytes+1))
	if err != nil {
		return githubRelease{}, err
	}
	if len(raw) > maxGitHubResponseBytes {
		return githubRelease{}, fmt.Errorf("GitHub release readback exceeds %d bytes", maxGitHubResponseBytes)
	}
	if response.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("GitHub release readback returned %s", response.Status)
	}
	var release githubRelease
	if err := json.Unmarshal(raw, &release); err != nil {
		return githubRelease{}, err
	}
	return release, nil
}

func releaseIDForKind(kind string) (string, error) {
	if kind == "" {
		return "", fmt.Errorf("kind is required")
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(kind))
	return "k-" + strings.ToLower(encoded), nil
}
