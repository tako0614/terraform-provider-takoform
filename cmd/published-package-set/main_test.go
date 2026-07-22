package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
)

func TestDownloadSnapshotStagesExactTenBySevenClosure(t *testing.T) {
	repoRoot := testRepositoryRoot(t)
	fake := newFakeGitHub(t, repoRoot)
	server := httptest.NewServer(fake)
	defer server.Close()
	client, err := newGitHubClient(server.URL+"/", "test-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	outputRoot := filepath.Join(t.TempDir(), "snapshot")
	if err := downloadSnapshot(context.Background(), client, repoRoot, outputRoot); err != nil {
		t.Fatalf("download snapshot: %v", err)
	}

	releaseRoot := filepath.Join(outputRoot, "admission", "v1", "releases")
	regularFiles := 0
	err = filepath.WalkDir(releaseRoot, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("staged symlink %s", name)
		}
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("staged non-regular file %s", name)
			}
			regularFiles++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if regularFiles != expectedPackageCount*expectedAssetCount {
		t.Fatalf("staged regular files = %d, want %d", regularFiles, expectedPackageCount*expectedAssetCount)
	}

	setRaw, err := os.ReadFile(filepath.Join(outputRoot, filepath.FromSlash(setPath)))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(setRaw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(setRaw, canonical) {
		t.Fatal("staged published-package set is not canonical JSON")
	}
	var set admissionrelease.PublishedPackageSet
	if err := json.Unmarshal(setRaw, &set); err != nil {
		t.Fatal(err)
	}
	if set.PackageVersion != "1.0.1" || len(set.Entries) != expectedPackageCount {
		t.Fatalf("staged set version/entries = %s/%d", set.PackageVersion, len(set.Entries))
	}
	seenReleaseIDs := make(map[int64]struct{}, len(set.Entries))
	for _, entry := range set.Entries {
		if entry.GitHubReleaseID <= 0 || !entry.Immutable {
			t.Fatalf("entry %s lacks immutable release identity", entry.Kind)
		}
		if _, duplicate := seenReleaseIDs[entry.GitHubReleaseID]; duplicate {
			t.Fatalf("duplicate release id %d", entry.GitHubReleaseID)
		}
		seenReleaseIDs[entry.GitHubReleaseID] = struct{}{}
	}
	if fake.requestCount != expectedPackageCount*(1+expectedAssetCount) {
		t.Fatalf("GitHub request count = %d, want %d", fake.requestCount, expectedPackageCount*(1+expectedAssetCount))
	}
}

func TestDownloadSnapshotFailureRemovesFinalRoot(t *testing.T) {
	repoRoot := testRepositoryRoot(t)
	tests := []struct {
		name   string
		mutate func(*fakeGitHub)
		want   string
	}{
		{
			name: "asset API bytes drift",
			mutate: func(fake *fakeGitHub) {
				fake.bodies[fake.lastAssetID] = append(append([]byte(nil), fake.bodies[fake.lastAssetID]...), 0)
			},
			want: "size/digest",
		},
		{
			name: "manifest candidate drift",
			mutate: func(fake *fakeGitHub) {
				manifestID := fake.firstManifestID
				raw := fake.bodies[manifestID]
				marker := []byte(`"packageDigest": "sha256:`)
				start := bytes.Index(raw, marker)
				if start < 0 {
					t.Fatal("fixture manifest packageDigest missing")
				}
				start += len(marker)
				mutated := append([]byte(nil), raw...)
				copy(mutated[start:start+64], strings.Repeat("0", 64))
				fake.replaceAsset(manifestID, mutated)
			},
			want: "exact candidate",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeGitHub(t, repoRoot)
			test.mutate(fake)
			server := httptest.NewServer(fake)
			defer server.Close()
			client, err := newGitHubClient(server.URL+"/", "", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			outputRoot := filepath.Join(t.TempDir(), "failed-snapshot")
			err = downloadSnapshot(context.Background(), client, repoRoot, outputRoot)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("download error = %v, want %q", err, test.want)
			}
			if _, statErr := os.Lstat(outputRoot); !os.IsNotExist(statErr) {
				t.Fatalf("failed snapshot left final root: %v", statErr)
			}
		})
	}
}

func TestDownloadSnapshotRefusesExistingOutputRoot(t *testing.T) {
	repoRoot := testRepositoryRoot(t)
	outputRoot := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(outputRoot, "owned-by-maintainer")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := newFakeGitHub(t, repoRoot)
	server := httptest.NewServer(fake)
	defer server.Close()
	client, err := newGitHubClient(server.URL+"/", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	err = downloadSnapshot(context.Background(), client, repoRoot, outputRoot)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output error = %v", err)
	}
	raw, err := os.ReadFile(marker)
	if err != nil || string(raw) != "keep" {
		t.Fatalf("existing output was modified: %q, %v", raw, err)
	}
	if fake.requestCount != 0 {
		t.Fatalf("existing-root refusal made %d network requests", fake.requestCount)
	}
}

func TestDownloadSnapshotRefusesSymlinkedParentIntoSourceRepository(t *testing.T) {
	repoRoot := testRepositoryRoot(t)
	parent := t.TempDir()
	linkedParent := filepath.Join(parent, "linked-source")
	if err := os.Symlink(repoRoot, linkedParent); err != nil {
		t.Fatal(err)
	}
	fake := newFakeGitHub(t, repoRoot)
	server := httptest.NewServer(fake)
	defer server.Close()
	client, err := newGitHubClient(server.URL+"/", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	outputRoot := filepath.Join(linkedParent, "unsafe-snapshot")
	err = downloadSnapshot(context.Background(), client, repoRoot, outputRoot)
	if err == nil || !strings.Contains(err.Error(), "outside the source repository") {
		t.Fatalf("symlinked-parent error = %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(repoRoot, "unsafe-snapshot")); !os.IsNotExist(statErr) {
		t.Fatalf("symlinked output was created in source repository: %v", statErr)
	}
	if fake.requestCount != 0 {
		t.Fatalf("symlinked-parent refusal made %d network requests", fake.requestCount)
	}
}

func TestValidateLiveReleaseRejectsNonClosedIdentity(t *testing.T) {
	releaseID := "k-ivsgozkxn5zgwzls"
	version := "1.0.1"
	tag := "forms/" + releaseID + "/v" + version
	names := canonicalAssetNames(releaseID, version)
	assets := make([]githubReleaseAsset, 0, len(names))
	for name := range names {
		assets = append(assets, githubReleaseAsset{
			ID: int64(len(assets) + 1), Name: name, State: "uploaded", Size: 1,
			Digest: "sha256:" + strings.Repeat("1", 64),
		})
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
	valid := githubRelease{
		ID: 1, TagName: tag, Immutable: true, PublishedAt: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), Assets: assets,
	}
	tests := []struct {
		name   string
		mutate func(*githubRelease)
	}{
		{name: "draft", mutate: func(release *githubRelease) { release.Draft = true }},
		{name: "prerelease", mutate: func(release *githubRelease) { release.Prerelease = true }},
		{name: "mutable", mutate: func(release *githubRelease) { release.Immutable = false }},
		{name: "non-positive release id", mutate: func(release *githubRelease) { release.ID = 0 }},
		{name: "six assets", mutate: func(release *githubRelease) { release.Assets = release.Assets[:6] }},
		{name: "duplicate asset id", mutate: func(release *githubRelease) { release.Assets[1].ID = release.Assets[0].ID }},
		{name: "non-canonical name", mutate: func(release *githubRelease) { release.Assets[0].Name = "extra.json" }},
		{name: "invalid digest", mutate: func(release *githubRelease) { release.Assets[0].Digest = "sha256:no" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := valid
			got.Assets = append([]githubReleaseAsset(nil), valid.Assets...)
			test.mutate(&got)
			if _, _, err := validateLiveRelease(got, tag, releaseID, version); err == nil {
				t.Fatal("unsafe release accepted")
			}
		})
	}
}

type fakeGitHub struct {
	t               *testing.T
	releases        map[string]githubRelease
	bodies          map[int64][]byte
	requestCount    int
	firstManifestID int64
	lastAssetID     int64
}

func newFakeGitHub(t *testing.T, repoRoot string) *fakeGitHub {
	t.Helper()
	candidatesRaw, err := os.ReadFile(filepath.Join(repoRoot, "forms", "standard-package-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	var candidates candidateSet
	if err := json.Unmarshal(candidatesRaw, &candidates); err != nil {
		t.Fatal(err)
	}
	if len(candidates.Packages) != expectedPackageCount {
		t.Fatalf("fixture packages = %d", len(candidates.Packages))
	}
	fake := &fakeGitHub{t: t, releases: make(map[string]githubRelease), bodies: make(map[int64][]byte)}
	nextAssetID := int64(10_000)
	for position, candidate := range candidates.Packages {
		releaseID, err := releaseIDForKind(candidate.Kind)
		if err != nil {
			t.Fatal(err)
		}
		tag := "forms/" + releaseID + "/v" + candidates.PackageVersion
		directory := filepath.Join(repoRoot, "admission", "v1", "releases", releaseID, candidates.PackageVersion)
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != expectedAssetCount {
			t.Fatalf("%s fixture assets = %d", candidate.Kind, len(entries))
		}
		assets := make([]githubReleaseAsset, 0, len(entries))
		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				t.Fatalf("non-regular fixture %s", entry.Name())
			}
			raw, err := os.ReadFile(filepath.Join(directory, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			asset := githubReleaseAsset{
				ID: nextAssetID, Name: entry.Name(), State: "uploaded", Size: int64(len(raw)), Digest: formpackage.DigestBytes(raw),
			}
			if position == 0 && entry.Name() == "release-manifest.json" {
				fake.firstManifestID = nextAssetID
			}
			fake.bodies[nextAssetID] = raw
			assets = append(assets, asset)
			fake.lastAssetID = nextAssetID
			nextAssetID++
		}
		sort.Slice(assets, func(i, j int) bool { return assets[i].Name > assets[j].Name })
		fake.releases[tag] = githubRelease{
			ID: int64(1_000 + position), TagName: tag, Immutable: true,
			PublishedAt: time.Date(2026, 7, 22, 9, position, 0, 0, time.UTC), Assets: assets,
		}
	}
	if fake.firstManifestID == 0 || fake.lastAssetID == 0 {
		t.Fatal("fake GitHub fixture ids were not initialized")
	}
	return fake
}

func (fake *fakeGitHub) replaceAsset(assetID int64, raw []byte) {
	fake.t.Helper()
	fake.bodies[assetID] = raw
	for tag, release := range fake.releases {
		for index := range release.Assets {
			if release.Assets[index].ID == assetID {
				release.Assets[index].Size = int64(len(raw))
				release.Assets[index].Digest = formpackage.DigestBytes(raw)
				fake.releases[tag] = release
				return
			}
		}
	}
	fake.t.Fatalf("asset id %d not found", assetID)
}

func (fake *fakeGitHub) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	fake.requestCount++
	if request.Method != http.MethodGet {
		http.Error(response, "method", http.StatusMethodNotAllowed)
		return
	}
	if request.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
		http.Error(response, "API version", http.StatusBadRequest)
		return
	}
	releasePrefix := "/repos/" + repository + "/releases/tags/"
	if strings.HasPrefix(request.URL.Path, releasePrefix) {
		tag := strings.TrimPrefix(request.URL.Path, releasePrefix)
		release, ok := fake.releases[tag]
		if !ok {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(release); err != nil {
			fake.t.Errorf("encode fake release: %v", err)
		}
		return
	}
	assetPrefix := "/repos/" + repository + "/releases/assets/"
	if strings.HasPrefix(request.URL.Path, assetPrefix) {
		assetID, err := strconv.ParseInt(strings.TrimPrefix(request.URL.Path, assetPrefix), 10, 64)
		if err != nil {
			http.Error(response, "asset id", http.StatusBadRequest)
			return
		}
		raw, ok := fake.bodies[assetID]
		if !ok {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/octet-stream")
		_, _ = response.Write(raw)
		return
	}
	http.NotFound(response, request)
}

func testRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}
