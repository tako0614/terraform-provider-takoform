package main

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
)

const (
	setPath                = "admission/v1/published-package-set.json"
	trustPath              = "admission/v1/trust/published-package-trust.json"
	repository             = "tako0614/terraform-provider-takoform"
	maxGitHubResponseBytes = 4 << 20
	maxGitHubAssetBytes    = 64 << 20
	expectedPackageCount   = 10
	expectedAssetCount     = 7
)

var (
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	versionPattern = regexp.MustCompile(`^[0-9][0-9A-Za-z.+-]*$`)
)

type candidateSet struct {
	DefinitionVersion string             `json:"definitionVersion"`
	PackageVersion    string             `json:"packageVersion"`
	Packages          []candidatePackage `json:"packages"`
}

type candidatePackage struct {
	Kind          string              `json:"kind"`
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

type githubClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	token      string
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
	fmt.Fprintln(os.Stderr, "usage: published-package-set snapshot | published-package-set download --output-root DIRECTORY")
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
	candidatesRaw, err := os.ReadFile(filepath.Join(sourceRoot, "forms", "standard-package-set.json"))
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
	trustRaw, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(trustPath)))
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
		entry, liveID, assetIDs, packageErr := downloadPackage(ctx, client, cleanOutput, candidate, candidates.PackageVersion)
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
	stagedSetPath := filepath.Join(cleanOutput, filepath.FromSlash(setPath))
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
	outputRoot string,
	candidate candidatePackage,
	packageVersion string,
) (admissionrelease.PublishedPackageEntry, int64, []int64, error) {
	releaseID, err := releaseIDForKind(candidate.Kind)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, 0, nil, err
	}
	tag := "forms/" + releaseID + "/v" + packageVersion
	live, err := client.fetchRelease(ctx, tag)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, 0, nil, err
	}
	assets, assetIDs, err := validateLiveRelease(live, tag, releaseID, packageVersion)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, 0, nil, err
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
			return admissionrelease.PublishedPackageEntry{}, 0, nil, fmt.Errorf("download %q: %w", name, err)
		}
		totalBytes += int64(len(raw))
		if totalBytes > maxGitHubAssetBytes {
			return admissionrelease.PublishedPackageEntry{}, 0, nil, fmt.Errorf("release asset closure exceeds %d bytes", maxGitHubAssetBytes)
		}
		downloaded[name] = raw
	}
	manifest, err := validateDownloadedPackage(candidate, packageVersion, releaseID, tag, assets, downloaded)
	if err != nil {
		return admissionrelease.PublishedPackageEntry{}, 0, nil, err
	}

	releaseDirectory := path.Join("releases", releaseID, packageVersion)
	stagedDirectory := filepath.Join(outputRoot, "admission", "v1", filepath.FromSlash(releaseDirectory))
	if err := os.MkdirAll(stagedDirectory, 0o755); err != nil {
		return admissionrelease.PublishedPackageEntry{}, 0, nil, err
	}
	for _, name := range names {
		if err := writeCreateOnly(filepath.Join(stagedDirectory, name), downloaded[name]); err != nil {
			return admissionrelease.PublishedPackageEntry{}, 0, nil, err
		}
	}
	manifestRaw := downloaded["release-manifest.json"]
	checksumsRaw := downloaded["SHA256SUMS"]
	entry := admissionrelease.PublishedPackageEntry{
		Kind: candidate.Kind, Slug: path.Base(candidate.Path), FormRef: candidate.FormRef, PackageDigest: candidate.PackageDigest,
		ReleaseTag: manifest.Tag, ReleaseCommit: manifest.SourceCommit, ReleaseToolingCommit: manifest.ToolingCommit,
		GitHubReleaseID: live.ID, PublishedAt: live.PublishedAt.UTC().Format(time.RFC3339), Immutable: live.Immutable,
		PackageReleaseManifestPath: releaseDirectory + "/release-manifest.json", PackageReleaseManifestDigest: formpackage.DigestBytes(manifestRaw),
		ChecksumsPath: releaseDirectory + "/SHA256SUMS", ChecksumsDigest: formpackage.DigestBytes(checksumsRaw),
		PackageIndexPath: releaseDirectory + "/" + manifest.SignedSubject, PackageIndexSigstoreBundle: releaseDirectory + "/" + manifest.SignatureBundle,
	}
	return entry, live.ID, assetIDs, nil
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
	liveAssets map[string]githubReleaseAsset,
	downloaded map[string][]byte,
) (releaseManifest, error) {
	manifestRaw, ok := downloaded["release-manifest.json"]
	if !ok {
		return releaseManifest{}, fmt.Errorf("downloaded release omits release-manifest.json")
	}
	var manifest releaseManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return releaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return releaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
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
	for _, asset := range manifest.Assets {
		wantMediaType, ok := wantMediaTypes[asset.Name]
		if !ok || asset.MediaType != wantMediaType || asset.Size < 0 || asset.Size > maxGitHubAssetBytes || !formpackage.ValidDigest(asset.Digest) {
			return releaseManifest{}, fmt.Errorf("release manifest contains invalid asset %q", asset.Name)
		}
		if _, duplicate := manifestAssets[asset.Name]; duplicate {
			return releaseManifest{}, fmt.Errorf("release manifest duplicates asset %q", asset.Name)
		}
		manifestAssets[asset.Name] = asset
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
	return manifest, nil
}

func validateChecksums(raw, manifestRaw []byte, manifestAssets map[string]releaseAsset) error {
	expected := map[string]string{"release-manifest.json": formpackage.DigestBytes(manifestRaw)}
	for name, asset := range manifestAssets {
		expected[name] = asset.Digest
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 6 || len(lines) != len(expected) {
		return fmt.Errorf("SHA256SUMS closure has %d lines, want exactly 6", len(lines))
	}
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || len(parts[0]) != 64 || path.Base(parts[1]) != parts[1] {
			return fmt.Errorf("invalid SHA256SUMS line %q", line)
		}
		want, ok := expected[parts[1]]
		if !ok || "sha256:"+parts[0] != want {
			return fmt.Errorf("SHA256SUMS does not bind %q to the release manifest", parts[1])
		}
		if _, duplicate := seen[parts[1]]; duplicate {
			return fmt.Errorf("SHA256SUMS duplicates %q", parts[1])
		}
		seen[parts[1]] = struct{}{}
	}
	return nil
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
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&release); err != nil {
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
