package main

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
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
	Tag             string              `json:"tag"`
	SourceCommit    string              `json:"sourceCommit"`
	ToolingCommit   string              `json:"toolingCommit"`
	PackageVersion  string              `json:"packageVersion"`
	ReleaseID       string              `json:"releaseId"`
	PackageDigest   string              `json:"packageDigest"`
	FormRef         formpackage.FormRef `json:"formRef"`
	SignedSubject   string              `json:"signedSubject"`
	SignatureBundle string              `json:"signatureBundle"`
	Assets          []releaseAsset      `json:"assets"`
}

type releaseAsset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
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
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

func main() {
	if len(os.Args) != 2 || os.Args[1] != "snapshot" {
		fmt.Fprintln(os.Stderr, "usage: published-package-set snapshot")
		os.Exit(2)
	}
	if err := snapshot(); err != nil {
		fmt.Fprintln(os.Stderr, "published-package-set:", err)
		os.Exit(1)
	}
	if err := standardforms.VerifyPublishedPackageSet("."); err != nil {
		fmt.Fprintln(os.Stderr, "published-package-set: verify snapshot:", err)
		os.Exit(1)
	}
	fmt.Println("published-package-set: snapshot and offline verification passed")
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
