package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/formpublication"
	"github.com/tako0614/terraform-provider-takoform/internal/standardforms"
)

func TestUsageExposesOnlyOneCurrentPublicationDownloadCommand(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStderr := os.Stderr
	os.Stderr = write
	usage()
	os.Stderr = originalStderr
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "download-current") {
		t.Fatalf("usage retains removed download-current alias: %q", text)
	}
	if strings.Count(text, "download-plan") != 1 {
		t.Fatalf("usage must expose download-plan exactly once: %q", text)
	}
}

func TestRemovedDownloadCurrentAliasIsUnknownRegardlessOfTokenSource(t *testing.T) {
	baseEnvironment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "GITHUB_TOKEN", "GH_TOKEN", "PUBLISHED_PACKAGE_SET_COMMAND_HELPER",
			"PUBLISHED_PACKAGE_SET_COMMAND_OUTPUT_ROOT":
			continue
		default:
			baseEnvironment = append(baseEnvironment, entry)
		}
	}
	tokenSources := []struct {
		name string
		env  string
	}{
		{name: "GITHUB_TOKEN", env: "GITHUB_TOKEN=github-token-must-not-select-an-alias"},
		{name: "GH_TOKEN", env: "GH_TOKEN=gh-token-must-not-select-an-alias"},
	}
	var commonOutput string
	for _, tokenSource := range tokenSources {
		t.Run(tokenSource.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestPublishedPackageSetCommandHelper$")
			command.Env = append(
				append([]string(nil), baseEnvironment...),
				"PUBLISHED_PACKAGE_SET_COMMAND_HELPER=1",
				"PUBLISHED_PACKAGE_SET_COMMAND_OUTPUT_ROOT="+filepath.Join(t.TempDir(), "unused"),
				tokenSource.env,
			)
			raw, err := command.CombinedOutput()
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) || exitError.ExitCode() != 2 {
				t.Fatalf("removed command exit = %v, output = %q", err, raw)
			}
			output := string(raw)
			if strings.Contains(output, "download-current") {
				t.Fatalf("removed alias remains public: %q", output)
			}
			if !strings.Contains(output, "download-plan") {
				t.Fatalf("replacement command missing from usage: %q", output)
			}
			if strings.Contains(output, "GITHUB_TOKEN") || strings.Contains(output, "GH_TOKEN") {
				t.Fatalf("removed alias reached token handling: %q", output)
			}
			if commonOutput == "" {
				commonOutput = output
			} else if output != commonOutput {
				t.Fatalf("removed alias depends on ambient token source:\nfirst: %q\nthis:  %q", commonOutput, output)
			}
		})
	}
}

func TestPublishedPackageSetCommandHelper(t *testing.T) {
	if os.Getenv("PUBLISHED_PACKAGE_SET_COMMAND_HELPER") != "1" {
		return
	}
	os.Args = []string{
		"published-package-set",
		"download-current",
		"--output-root",
		os.Getenv("PUBLISHED_PACKAGE_SET_COMMAND_OUTPUT_ROOT"),
	}
	main()
	t.Fatal("published-package-set main returned")
}

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

func TestDownloadPlanStagesExactCurrentPublicationClosure(t *testing.T) {
	source := newPlanSourceRepository(t)
	fake := newPlanFakeGitHub(t, source)
	server := httptest.NewServer(fake)
	defer server.Close()
	client, err := newGitHubClient(server.URL+"/", "fixture-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	outputRoot := filepath.Join(t.TempDir(), "publication")
	if err := downloadPlanWithVerifier(
		context.Background(), client, source.root, outputRoot, verifySyntheticPublication,
	); err != nil {
		t.Fatalf("download plan: %v", err)
	}

	regularFiles := 0
	err = filepath.WalkDir(outputRoot, func(name string, entry os.DirEntry, walkErr error) error {
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
	if regularFiles != expectedPlanCount*expectedAssetCount+5 {
		t.Fatalf("staged regular files = %d, want %d", regularFiles, expectedPlanCount*expectedAssetCount+5)
	}

	setRaw, err := os.ReadFile(filepath.Join(outputRoot, publicationSetPath))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(setRaw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(setRaw, canonical) {
		t.Fatal("publication set is not canonical JSON")
	}
	var set formPackagePublicationSet
	if err := json.Unmarshal(setRaw, &set); err != nil {
		t.Fatal(err)
	}
	if set.Format != publicationSetFormat || set.Generation != "portable-v1" || set.Repository != repository ||
		set.PublicationStatus != "published-immutable" || set.AdmissionStatus != "external-required" ||
		set.RevocationCheckpointStatus != "external-required" || set.GitObjectFormat != "sha1" ||
		set.ProtectedMainCommit != source.mainCommit || len(set.Entries) != expectedPlanCount {
		t.Fatalf("publication set identity/closure drifted: %+v", set)
	}
	trustedRootCopy, err := os.ReadFile(filepath.Join(
		outputRoot, filepath.FromSlash(set.VerificationPolicy.TrustedRoot.Path),
	))
	if err != nil {
		t.Fatal(err)
	}
	trustedRootSource, err := os.ReadFile(filepath.Join(
		source.root, filepath.FromSlash(trustedRootSourcePath),
	))
	if err != nil {
		t.Fatal(err)
	}
	if set.VerificationPolicy.TrustedRoot.Path != trustedRootOutputPath ||
		set.VerificationPolicy.TrustedRoot.SourcePath != trustedRootSourcePath ||
		set.VerificationPolicy.TrustedRoot.SHA256 != formpackage.DigestBytes(trustedRootSource) ||
		!bytes.Equal(trustedRootCopy, trustedRootSource) ||
		set.VerificationPolicy.CertificateIdentity != currentPackageIdentity ||
		set.VerificationPolicy.OIDCIssuer != currentPackageIssuer ||
		set.VerificationPolicy.BundleMediaType != "application/vnd.dev.sigstore.bundle.v0.3+json" {
		t.Fatalf("publication verification policy drifted: %+v", set.VerificationPolicy)
	}
	planCopy, err := os.ReadFile(filepath.Join(outputRoot, set.SourcePlan.Path))
	if err != nil {
		t.Fatal(err)
	}
	if set.SourcePlan.Path != "release-plan.json" ||
		set.SourcePlan.SourcePath != standardforms.ReleasePlanPath ||
		set.SourcePlan.SHA256 != formpackage.DigestBytes(source.planRaw) ||
		!bytes.Equal(planCopy, source.planRaw) {
		t.Fatal("publication set does not retain and bind the exact source plan")
	}
	for position, entry := range set.Entries {
		planned := source.plan.Releases[position]
		authorityPlanPath, authorityTrustedRootPath := retainedAuthorityPaths(source.mainCommit)
		if entry.Kind != planned.Kind || entry.ReleaseID != planned.ReleaseID ||
			entry.Version != planned.Version || entry.Tag != planned.Tag ||
			entry.SourcePath != planned.SourcePath || entry.FormRef != planned.FormRef ||
			entry.PackageDigest != planned.PackageDigest || entry.SourceCommit != source.sourceCommit ||
			entry.PeeledCommit != source.sourceCommit || entry.ToolingCommit != source.mainCommit ||
			entry.ReleasePlan.Path != authorityPlanPath ||
			entry.ReleasePlan.SourcePath != standardforms.ReleasePlanPath ||
			entry.ReleasePlan.SHA256 != formpackage.DigestBytes(source.planRaw) ||
			entry.TrustedRoot.Path != authorityTrustedRootPath ||
			entry.TrustedRoot.SourcePath != trustedRootSourcePath ||
			entry.TrustedRoot.SHA256 != formpackage.DigestBytes(trustedRootSource) ||
			!commitPattern.MatchString(entry.TagObjectOID) || entry.GitHubReleaseID == "" ||
			entry.PublishedAt == "" || !entry.Immutable || len(entry.Assets) != expectedAssetCount {
			t.Fatalf("publication entry %d does not bind the exact release: %+v", position, entry)
		}
		retainedPlan, err := os.ReadFile(filepath.Join(outputRoot, filepath.FromSlash(entry.ReleasePlan.Path)))
		if err != nil {
			t.Fatal(err)
		}
		retainedTrustedRoot, err := os.ReadFile(filepath.Join(outputRoot, filepath.FromSlash(entry.TrustedRoot.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(retainedPlan, source.planRaw) ||
			!bytes.Equal(retainedTrustedRoot, trustedRootSource) {
			t.Fatalf("%s retained historical authority bytes drifted", entry.Kind)
		}
		lastName := ""
		for _, asset := range entry.Assets {
			if asset.Name <= lastName || !formpackage.ValidDigest(asset.SHA256) || asset.Size < 0 {
				t.Fatalf("%s has invalid ordered asset %+v", entry.Kind, asset)
			}
			retained, err := os.ReadFile(filepath.Join(
				outputRoot, "releases", entry.ReleaseID, entry.Version, asset.Name,
			))
			if err != nil {
				t.Fatal(err)
			}
			if int64(len(retained)) != asset.Size || formpackage.DigestBytes(retained) != asset.SHA256 {
				t.Fatalf("%s retained asset %s differs from the publication set", entry.Kind, asset.Name)
			}
			lastName = asset.Name
		}
	}
	wantRequests := 1 + expectedPlanCount*(1+expectedAssetCount+2) + 1
	if fake.requestCount != wantRequests {
		t.Fatalf("GitHub request count = %d, want %d", fake.requestCount, wantRequests)
	}
}

func TestDownloadPlanRetainsHistoricalPerToolingAuthorityAfterSelectionAdvance(t *testing.T) {
	source := newPlanSourceRepository(t)
	fake := newPlanFakeGitHub(t, source)
	historicalTrustedRoot, err := os.ReadFile(filepath.Join(
		source.root, filepath.FromSlash(trustedRootSourcePath),
	))
	if err != nil {
		t.Fatal(err)
	}

	planPath := filepath.Join(source.root, filepath.FromSlash(standardforms.ReleasePlanPath))
	currentPlanRaw := append(append([]byte(nil), source.planRaw...), '\n')
	if err := os.WriteFile(planPath, currentPlanRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	var trustedRootDocument any
	if err := json.Unmarshal(historicalTrustedRoot, &trustedRootDocument); err != nil {
		t.Fatal(err)
	}
	currentTrustedRoot, err := json.MarshalIndent(trustedRootDocument, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	currentTrustedRoot = append(currentTrustedRoot, '\n')
	if bytes.Equal(historicalTrustedRoot, currentTrustedRoot) {
		t.Fatal("trusted-root advance fixture did not change bytes")
	}
	trustedRootPath := filepath.Join(source.root, filepath.FromSlash(trustedRootSourcePath))
	if err := os.WriteFile(trustedRootPath, currentTrustedRoot, 0o644); err != nil {
		t.Fatal(err)
	}
	futureSourcePath := filepath.Join(source.root, "forms", "releases", "future-source", "README")
	if err := os.MkdirAll(filepath.Dir(futureSourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(futureSourcePath, []byte("future release source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustTestGit(
		t, source.root, "add", "--",
		standardforms.ReleasePlanPath, trustedRootSourcePath, "forms/releases/future-source/README",
	)
	mustTestGit(t, source.root, "commit", "-q", "-m", "advance selection plan source and trusted root")
	protectedMain := strings.TrimSpace(mustTestGit(t, source.root, "rev-parse", "HEAD"))
	fake.refs["refs/heads/main"] = githubGitRef{
		Ref: "refs/heads/main", Object: githubGitObject{Type: "commit", SHA: protectedMain},
	}
	for _, commit := range []string{source.sourceCommit, source.mainCommit} {
		fake.comparisons[commit+"..."+protectedMain] = githubComparison{
			Status: "ahead", BaseCommit: githubGitObject{SHA: commit},
			MergeBaseCommit: githubGitObject{SHA: commit},
		}
	}

	server := httptest.NewServer(fake)
	defer server.Close()
	client, err := newGitHubClient(server.URL+"/", "fixture-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	outputRoot := filepath.Join(t.TempDir(), "publication")
	if err := downloadPlanWithVerifier(
		context.Background(), client, source.root, outputRoot, verifySyntheticPublication,
	); err != nil {
		t.Fatalf("download plan after selection advance: %v", err)
	}
	var set formPackagePublicationSet
	setRaw, err := os.ReadFile(filepath.Join(outputRoot, publicationSetPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(setRaw, &set); err != nil {
		t.Fatal(err)
	}
	if set.ProtectedMainCommit != protectedMain ||
		set.SourcePlan.SHA256 != formpackage.DigestBytes(currentPlanRaw) ||
		set.VerificationPolicy.TrustedRoot.SHA256 != formpackage.DigestBytes(currentTrustedRoot) {
		t.Fatalf("current selection snapshot drifted: %+v", set)
	}
	if len(set.Entries) != expectedPlanCount {
		t.Fatalf("publication entries = %d, want %d", len(set.Entries), expectedPlanCount)
	}
	authorityPlanPath, authorityTrustedRootPath := retainedAuthorityPaths(source.mainCommit)
	for _, entry := range set.Entries {
		if entry.ToolingCommit != source.mainCommit ||
			entry.ReleasePlan.Path != authorityPlanPath ||
			entry.ReleasePlan.SHA256 != formpackage.DigestBytes(source.planRaw) ||
			entry.TrustedRoot.Path != authorityTrustedRootPath ||
			entry.TrustedRoot.SHA256 != formpackage.DigestBytes(historicalTrustedRoot) {
			t.Fatalf("%s did not bind historical tooling authority: %+v", entry.Kind, entry)
		}
	}
	retainedPlan, err := os.ReadFile(filepath.Join(outputRoot, filepath.FromSlash(authorityPlanPath)))
	if err != nil {
		t.Fatal(err)
	}
	retainedTrustedRoot, err := os.ReadFile(filepath.Join(outputRoot, filepath.FromSlash(authorityTrustedRootPath)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retainedPlan, source.planRaw) ||
		!bytes.Equal(retainedTrustedRoot, historicalTrustedRoot) ||
		bytes.Equal(retainedTrustedRoot, currentTrustedRoot) {
		t.Fatal("retained per-tooling authority bytes do not preserve the historical release authority")
	}
}

func TestDownloadPlanRejectsSemanticSigstoreDriftAndRemovesOutput(t *testing.T) {
	source := newPlanSourceRepository(t)
	fake := newPlanFakeGitHub(t, source)
	first := source.plan.Releases[0]
	base := "takoform-form-" + first.ReleaseID + "_" + first.Version
	bundleName := base + "_package-index.sigstore.json"
	fake.mutateReleaseAsset(first.Tag, bundleName, func(raw []byte) []byte {
		var bundle map[string]any
		if err := json.Unmarshal(raw, &bundle); err != nil {
			t.Fatal(err)
		}
		signature := bundle["messageSignature"].(map[string]any)
		digest := signature["messageDigest"].(map[string]any)
		digest["digest"] = base64.StdEncoding.EncodeToString(make([]byte, sha256.Size))
		return canonicalTestJSON(t, bundle)
	})
	server := httptest.NewServer(fake)
	defer server.Close()
	client, err := newGitHubClient(server.URL+"/", "fixture-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	outputRoot := filepath.Join(t.TempDir(), "failed-publication")
	err = downloadPlan(context.Background(), client, source.root, outputRoot)
	if err == nil || !strings.Contains(err.Error(), "message digest does not bind") {
		t.Fatalf("semantic Sigstore drift error = %v", err)
	}
	if _, statErr := os.Lstat(outputRoot); !os.IsNotExist(statErr) {
		t.Fatalf("failed publication left output root: %v", statErr)
	}
}

func TestDownloadPlanProductionPathInvokesCommonVerifierAndFailsClosed(t *testing.T) {
	source := newCompletePlanSourceRepository(t)
	fake := newPlanFakeGitHub(t, source)
	server := httptest.NewServer(fake)
	defer server.Close()
	client, err := newGitHubClient(server.URL+"/", "fixture-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	outputRoot := filepath.Join(t.TempDir(), "failed-common-verification")
	err = downloadPlan(context.Background(), client, source.root, outputRoot)
	if err == nil || !strings.Contains(err.Error(), "verify staged publication set") {
		t.Fatalf("common publication verification error = %v", err)
	}
	if strings.Contains(err.Error(), "load exact Legacy portable-v1 set") {
		t.Fatalf("production path did not reach the common publication verifier: %v", err)
	}
	if _, statErr := os.Lstat(outputRoot); !os.IsNotExist(statErr) {
		t.Fatalf("failed common verification left output root: %v", statErr)
	}
}

func TestDownloadPlanRefusesExistingOutputRootWithoutNetwork(t *testing.T) {
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
	err = downloadPlan(context.Background(), client, repoRoot, outputRoot)
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

func TestDownloadPlanRefusesSymlinkedOutputParentWithoutNetwork(t *testing.T) {
	repoRoot := testRepositoryRoot(t)
	parent := t.TempDir()
	realParent := filepath.Join(parent, "real")
	if err := os.Mkdir(realParent, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(parent, "linked")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Fatal(err)
	}
	fake := newFakeGitHub(t, repoRoot)
	server := httptest.NewServer(fake)
	defer server.Close()
	client, err := newGitHubClient(server.URL+"/", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	outputRoot := filepath.Join(linkedParent, "unsafe-publication")
	err = downloadPlan(context.Background(), client, repoRoot, outputRoot)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked-parent error = %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(realParent, "unsafe-publication")); !os.IsNotExist(statErr) {
		t.Fatalf("symlinked output was created: %v", statErr)
	}
	if fake.requestCount != 0 {
		t.Fatalf("symlinked-parent refusal made %d network requests", fake.requestCount)
	}
}

func TestSelectCurrentPlannedReleaseRequiresOneExactDigestIdentity(t *testing.T) {
	t.Parallel()
	releaseID := formpackage.ReleaseIDForKind("Example")
	packageDigest := "sha256:" + strings.Repeat("a", 64)
	artifactID := "sha256-" + strings.Repeat("a", 64)
	tag := "forms/" + releaseID + "/" + artifactID
	identity := standardforms.CurrentFormReleaseIdentity{
		ProposalID: "p-example", State: "experimental", Kind: "Example",
		ReleaseID: releaseID, ArtifactID: artifactID, Tag: tag,
		SourcePath: "forms/releases/" + releaseID + "/" + artifactID,
		FormRef: formpackage.FormRef{
			APIVersion: formpackage.CurrentFormAPIVersion, Kind: "Example",
			DefinitionVersion: "0.1.0", SchemaDigest: "sha256:" + strings.Repeat("b", 64),
		},
		PackageDigest: packageDigest,
	}
	plan := standardforms.CurrentFormReleasePlan{
		Format: "takoform.current-form-release-plan@v2", Repository: repository,
		Releases: []standardforms.CurrentFormReleaseIdentity{identity},
	}

	selected, err := selectCurrentPlannedRelease(plan, identity.Kind, identity.Tag)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Kind != identity.Kind || selected.ReleaseID != releaseID ||
		selected.ArtifactID != artifactID || selected.LegacyVersion != "" ||
		selected.Tag != tag || selected.SourcePath != identity.SourcePath ||
		selected.FormRef != identity.FormRef || selected.PackageDigest != packageDigest ||
		selected.APIVersion != formpackage.CurrentPackageAPIVersion {
		t.Fatalf("selected current release = %#v", selected)
	}
	if _, err := selectCurrentPlannedRelease(plan, identity.Kind, "forms/"+releaseID+"/sha256-"+strings.Repeat("c", 64)); err == nil {
		t.Fatal("kind with a different digest tag selected a current release")
	}
}

func TestVerifyPlanDirectoryAcceptsExactSemanticClosureAndReportsTrustedRootDigest(t *testing.T) {
	source := newPlanSourceRepository(t)
	planned := source.plan.Releases[0]
	assetRoot := writeTestReleaseAssets(t, buildPlanReleaseFixture(t, source, planned))
	trustedRoot := filepath.Join(source.root, filepath.FromSlash(trustedRootSourcePath))

	result, err := verifyPlanDirectory(
		source.root, assetRoot, planned.Kind, planned.Tag,
		source.sourceCommit, source.mainCommit, trustedRoot,
	)
	if err != nil {
		t.Fatalf("verify exact local release: %v", err)
	}
	trustedRaw, err := os.ReadFile(trustedRoot)
	if err != nil {
		t.Fatal(err)
	}
	trustedAbsolute, err := filepath.Abs(trustedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if result.Format != planVerificationFormat || result.SemanticStatus != "verified" ||
		result.CryptographicStatus != "external-required" || result.Kind != planned.Kind ||
		result.ReleaseID != planned.ReleaseID || result.Version != planned.Version ||
		result.Tag != planned.Tag || result.SourceCommit != source.sourceCommit ||
		result.ToolingCommit != source.mainCommit || result.TrustedRoot.Path != trustedAbsolute ||
		result.TrustedRoot.SHA256 != formpackage.DigestBytes(trustedRaw) ||
		len(result.Assets) != expectedAssetCount {
		t.Fatalf("local verification result drifted: %+v", result)
	}
	lastName := ""
	for _, asset := range result.Assets {
		raw, err := os.ReadFile(filepath.Join(assetRoot, asset.Name))
		if err != nil {
			t.Fatal(err)
		}
		if asset.Name <= lastName || asset.Size != int64(len(raw)) ||
			asset.SHA256 != formpackage.DigestBytes(raw) {
			t.Fatalf("invalid ordered local verification asset: %+v", asset)
		}
		lastName = asset.Name
	}
	resultRaw, err := canonicalJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	if canonical, err := formpackage.Canonicalize(resultRaw); err != nil || !bytes.Equal(resultRaw, canonical) {
		t.Fatalf("verification result is not canonical JSON: %v", err)
	}
}

func TestVerifyPlanDirectoryAcceptsExactCurrentDigestLifecycleClosure(t *testing.T) {
	source, current := newCurrentPlanSourceRepository(t)
	planned := exactPlannedFormRelease{
		Kind: current.Kind, ReleaseID: current.ReleaseID, ArtifactID: current.ArtifactID,
		Tag: current.Tag, SourcePath: current.SourcePath, FormRef: current.FormRef,
		PackageDigest: current.PackageDigest, APIVersion: formpackage.CurrentPackageAPIVersion,
	}
	assetRoot := writeTestReleaseAssets(t, buildExactReleaseFixture(t, source, planned))
	trustedRoot := filepath.Join(source.root, filepath.FromSlash(trustedRootSourcePath))

	result, err := verifyPlanDirectory(
		source.root, assetRoot, current.Kind, current.Tag,
		source.sourceCommit, source.mainCommit, trustedRoot,
	)
	if err != nil {
		t.Fatalf("verify exact current lifecycle release: %v", err)
	}
	if result.Format != planVerificationFormat || result.SemanticStatus != "verified" ||
		result.CryptographicStatus != "external-required" || result.Kind != current.Kind ||
		result.ReleaseID != current.ReleaseID || result.ArtifactID != current.ArtifactID ||
		result.Version != "" || result.Tag != current.Tag ||
		result.SourceCommit != source.sourceCommit || result.ToolingCommit != source.mainCommit ||
		len(result.Assets) != expectedAssetCount {
		t.Fatalf("current lifecycle verification result drifted: %+v", result)
	}
}

func TestVerifyPlanDirectoryUsesHistoricalToolingTreeAfterPlanSourceAndRootAdvance(t *testing.T) {
	source := newPlanSourceRepository(t)
	planned := source.plan.Releases[0]
	assets := buildPlanReleaseFixture(t, source, planned)
	assetRoot := writeTestReleaseAssets(t, assets)

	currentTrustedRoot := filepath.Join(source.root, filepath.FromSlash(trustedRootSourcePath))
	historicalTrustedRootRaw, err := os.ReadFile(currentTrustedRoot)
	if err != nil {
		t.Fatal(err)
	}
	historicalTrustedRoot := filepath.Join(t.TempDir(), "trusted-root.json")
	if err := os.WriteFile(historicalTrustedRoot, historicalTrustedRootRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	planPath := filepath.Join(source.root, filepath.FromSlash(standardforms.ReleasePlanPath))
	if err := os.WriteFile(planPath, append(append([]byte(nil), source.planRaw...), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	definitionPath := filepath.Join(
		source.root, filepath.FromSlash(planned.SourcePath), "definition.json",
	)
	definitionRaw, err := os.ReadFile(definitionPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(definitionPath, append(append([]byte(nil), definitionRaw...), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	var trustedRootDocument any
	if err := json.Unmarshal(historicalTrustedRootRaw, &trustedRootDocument); err != nil {
		t.Fatal(err)
	}
	rotatedTrustedRoot, err := json.MarshalIndent(trustedRootDocument, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	rotatedTrustedRoot = append(rotatedTrustedRoot, '\n')
	if bytes.Equal(historicalTrustedRootRaw, rotatedTrustedRoot) {
		t.Fatal("trusted-root rotation fixture did not change bytes")
	}
	if err := os.WriteFile(currentTrustedRoot, rotatedTrustedRoot, 0o644); err != nil {
		t.Fatal(err)
	}
	mustTestGit(
		t, source.root, "add", "--",
		standardforms.ReleasePlanPath,
		planned.SourcePath+"/definition.json",
		trustedRootSourcePath,
	)
	mustTestGit(t, source.root, "commit", "-q", "-m", "advance plan source and trusted root")

	result, err := verifyPlanDirectory(
		source.root, assetRoot, planned.Kind, planned.Tag,
		source.sourceCommit, source.mainCommit, historicalTrustedRoot,
	)
	if err != nil {
		t.Fatalf("historical Form release rejected after later authority advance: %v", err)
	}
	logicalTrustedRootAbsolute, err := filepath.Abs(currentTrustedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if result.ToolingCommit != source.mainCommit ||
		result.TrustedRoot.Path != logicalTrustedRootAbsolute ||
		result.TrustedRoot.SHA256 != formpackage.DigestBytes(historicalTrustedRootRaw) {
		t.Fatalf("historical Form report did not bind tooling tree/root: %+v", result)
	}

	_, err = verifyPlanDirectory(
		source.root, assetRoot, planned.Kind, planned.Tag,
		source.sourceCommit, source.mainCommit, currentTrustedRoot,
	)
	if err == nil || !strings.Contains(err.Error(), "trusted-root bytes differ from the exact tooling commit") {
		t.Fatalf("later substituted trusted-root error = %v", err)
	}

	base := "takoform-form-" + planned.ReleaseID + "_" + planned.Version
	bundleName := base + "_package-index.sigstore.json"
	var bundle map[string]any
	if err := json.Unmarshal(assets[bundleName], &bundle); err != nil {
		t.Fatal(err)
	}
	messageSignature := bundle["messageSignature"].(map[string]any)
	messageDigest := messageSignature["messageDigest"].(map[string]any)
	messageDigest["digest"] = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, sha256.Size))
	assets[bundleName] = canonicalTestJSON(t, bundle)
	resealTestReleaseAssets(t, assets)
	substitutedAssetRoot := writeTestReleaseAssets(t, assets)
	_, err = verifyPlanDirectory(
		source.root, substitutedAssetRoot, planned.Kind, planned.Tag,
		source.sourceCommit, source.mainCommit, historicalTrustedRoot,
	)
	if err == nil || !strings.Contains(err.Error(), "Sigstore bundle") {
		t.Fatalf("historical resealed asset substitution error = %v", err)
	}
}

func TestVerifyPlanDirectoryRejectsResealedSemanticSigstoreDrift(t *testing.T) {
	source := newPlanSourceRepository(t)
	planned := source.plan.Releases[0]
	assets := buildPlanReleaseFixture(t, source, planned)
	base := "takoform-form-" + planned.ReleaseID + "_" + planned.Version
	bundleName := base + "_package-index.sigstore.json"
	var bundle map[string]any
	if err := json.Unmarshal(assets[bundleName], &bundle); err != nil {
		t.Fatal(err)
	}
	messageSignature := bundle["messageSignature"].(map[string]any)
	messageDigest := messageSignature["messageDigest"].(map[string]any)
	messageDigest["digest"] = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, sha256.Size))
	assets[bundleName] = canonicalTestJSON(t, bundle)
	resealTestReleaseAssets(t, assets)
	assetRoot := writeTestReleaseAssets(t, assets)

	_, err := verifyPlanDirectory(
		source.root, assetRoot, planned.Kind, planned.Tag,
		source.sourceCommit, source.mainCommit,
		filepath.Join(source.root, filepath.FromSlash(trustedRootSourcePath)),
	)
	if err == nil || !strings.Contains(err.Error(), "Sigstore bundle") {
		t.Fatalf("semantic Sigstore drift error = %v", err)
	}
}

func TestVerifyPlanDirectoryRejectsMalformedTrustedRootBeforeAssetAcceptance(t *testing.T) {
	source := newPlanSourceRepository(t)
	planned := source.plan.Releases[0]
	assetRoot := writeTestReleaseAssets(t, buildPlanReleaseFixture(t, source, planned))
	malformedRoot := filepath.Join(t.TempDir(), "trusted-root.json")
	if err := os.WriteFile(malformedRoot, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := verifyPlanDirectory(
		source.root, assetRoot, planned.Kind, planned.Tag,
		source.sourceCommit, source.mainCommit, malformedRoot,
	)
	if err == nil || !strings.Contains(err.Error(), "decode Sigstore trusted root") {
		t.Fatalf("malformed trusted-root error = %v", err)
	}
}

func TestVerifyPlanDirectoryRejectsSourcePathDriftBetweenSourceAndToolingCommits(t *testing.T) {
	source := newPlanSourceRepository(t)
	planned := source.plan.Releases[0]
	assetRoot := writeTestReleaseAssets(t, buildPlanReleaseFixture(t, source, planned))
	indexPath := filepath.Join(
		source.root, filepath.FromSlash(planned.SourcePath), formpackage.PackageIndexFilename,
	)
	original, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, append(append([]byte(nil), original...), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	mustTestGit(t, source.root, "add", "--", planned.SourcePath)
	mustTestGit(t, source.root, "commit", "-q", "-m", "drift planned source")
	driftedTooling := strings.TrimSpace(mustTestGit(t, source.root, "rev-parse", "HEAD"))
	if err := os.WriteFile(indexPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = verifyPlanDirectory(
		source.root, assetRoot, planned.Kind, planned.Tag,
		source.sourceCommit, driftedTooling,
		filepath.Join(source.root, filepath.FromSlash(trustedRootSourcePath)),
	)
	if err == nil || !strings.Contains(err.Error(), "planned source path changed") {
		t.Fatalf("source/tooling path drift error = %v", err)
	}
}

func TestVerifyPlanDirectoryRejectsValidButSubstitutedTrustedRootBytes(t *testing.T) {
	source := newPlanSourceRepository(t)
	planned := source.plan.Releases[0]
	assetRoot := writeTestReleaseAssets(t, buildPlanReleaseFixture(t, source, planned))
	trustedRoot := filepath.Join(source.root, filepath.FromSlash(trustedRootSourcePath))
	raw, err := os.ReadFile(trustedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trustedRoot, append(append([]byte(nil), raw...), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = verifyPlanDirectory(
		source.root, assetRoot, planned.Kind, planned.Tag,
		source.sourceCommit, source.mainCommit, trustedRoot,
	)
	if err == nil || !strings.Contains(err.Error(), "trusted-root bytes differ from the exact tooling commit") {
		t.Fatalf("substituted trusted-root error = %v", err)
	}
}

func TestVerifyPlanDirectoryRejectsSymlinkedAsset(t *testing.T) {
	source := newPlanSourceRepository(t)
	planned := source.plan.Releases[0]
	assetRoot := writeTestReleaseAssets(t, buildPlanReleaseFixture(t, source, planned))
	checksums := filepath.Join(assetRoot, "SHA256SUMS")
	if err := os.Remove(checksums); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("release-manifest.json", checksums); err != nil {
		t.Fatal(err)
	}

	_, err := verifyPlanDirectory(
		source.root, assetRoot, planned.Kind, planned.Tag,
		source.sourceCommit, source.mainCommit,
		filepath.Join(source.root, filepath.FromSlash(trustedRootSourcePath)),
	)
	if err == nil || !strings.Contains(err.Error(), "regular file, not a symlink") {
		t.Fatalf("symlinked local release asset error = %v", err)
	}
}

func TestResolveTagRejectsLightweightAndNestedTags(t *testing.T) {
	tag := "forms/k-j5rguzldorbhky3lmv2a/v3.0.0"
	for _, test := range []struct {
		name string
		ref  githubGitObject
		tag  *githubGitTag
	}{
		{
			name: "lightweight",
			ref:  githubGitObject{Type: "commit", SHA: strings.Repeat("1", 40)},
		},
		{
			name: "nested annotated tag",
			ref:  githubGitObject{Type: "tag", SHA: strings.Repeat("2", 40)},
			tag: &githubGitTag{
				SHA: strings.Repeat("2", 40), Tag: tag,
				Object: githubGitObject{Type: "tag", SHA: strings.Repeat("3", 40)},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeGitHub{
				t: t, releases: map[string]githubRelease{}, bodies: map[int64][]byte{},
				refs: map[string]githubGitRef{
					"refs/tags/" + tag: {Ref: "refs/tags/" + tag, Object: test.ref},
				},
				tags: map[string]githubGitTag{}, comparisons: map[string]githubComparison{},
			}
			if test.tag != nil {
				fake.tags[test.tag.SHA] = *test.tag
			}
			server := httptest.NewServer(fake)
			defer server.Close()
			client, err := newGitHubClient(server.URL+"/", "", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.resolveTag(context.Background(), tag); err == nil {
				t.Fatal("non-direct annotated tag unexpectedly accepted")
			}
		})
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

type planSourceFixture struct {
	root         string
	planRaw      []byte
	plan         standardforms.ReleasePlan
	sourceCommit string
	mainCommit   string
}

func verifySyntheticPublication(
	repositoryRoot string,
	publicationRoot string,
	_ string,
) (formpublication.Set, error) {
	raw, err := os.ReadFile(filepath.Join(
		repositoryRoot, filepath.FromSlash(standardforms.ReleasePlanPath),
	))
	if err != nil {
		return formpublication.Set{}, err
	}
	var plan standardforms.ReleasePlan
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return formpublication.Set{}, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return formpublication.Set{}, err
	}
	expected := admissionrelease.CandidateSet{
		Generation: plan.Generation,
		Entries:    make([]admissionrelease.Candidate, 0, len(plan.Releases)),
	}
	for _, planned := range plan.Releases {
		expected.Entries = append(expected.Entries, admissionrelease.Candidate{
			Kind: planned.Kind, Slug: planned.Slug, PackagePath: planned.SourcePath,
			FormRef: planned.FormRef, PackageDigest: planned.PackageDigest,
		})
	}
	return formpublication.VerifyStructure(publicationRoot, expected)
}

func newCompletePlanSourceRepository(t *testing.T) planSourceFixture {
	t.Helper()
	workingRoot := testRepositoryRoot(t)
	root := filepath.Join(t.TempDir(), "source")
	command := exec.Command("git", "clone", "--quiet", "--no-hardlinks", workingRoot, root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("clone complete source fixture: %v: %s", err, strings.TrimSpace(string(output)))
	}
	// The production command verifies the current worktree, and the portable
	// gate must remain runnable before those bytes are committed. Keep the
	// fixture's Git history for provenance checks while projecting the current
	// generated public surfaces into the temporary worktree.
	for _, relativeRoot := range []string{"docs/resources", "examples/resources"} {
		destination := filepath.Join(root, filepath.FromSlash(relativeRoot))
		if err := os.RemoveAll(destination); err != nil {
			t.Fatal(err)
		}
		copyTestTree(t,
			filepath.Join(workingRoot, filepath.FromSlash(relativeRoot)),
			destination,
		)
	}
	for _, relativeRoot := range []string{"proposals"} {
		destination := filepath.Join(root, filepath.FromSlash(relativeRoot))
		if err := os.RemoveAll(destination); err != nil {
			t.Fatal(err)
		}
		copyTestTree(t,
			filepath.Join(workingRoot, filepath.FromSlash(relativeRoot)),
			destination,
		)
	}
	copyTestFile(t,
		filepath.Join(workingRoot, "spec", "decisions", "0006-v1alpha2-restarts-form-lines.md"),
		filepath.Join(root, "spec", "decisions", "0006-v1alpha2-restarts-form-lines.md"),
	)
	copyTestFile(t,
		filepath.Join(workingRoot, "forms", "README.md"),
		filepath.Join(root, "forms", "README.md"),
	)
	for _, relativePath := range []string{
		"forms/lifecycle.json",
		"forms/lifecycle.schema.json",
		"admission/admission-identities.json",
	} {
		copyTestFile(t,
			filepath.Join(workingRoot, filepath.FromSlash(relativePath)),
			filepath.Join(root, filepath.FromSlash(relativePath)),
		)
	}
	if err := os.Remove(filepath.Join(root, "forms", "admission-candidate-set.json")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, relativePath := range []string{
		"forms/releases/k-inxw24dvorsus3ttorqw4y3f/1.0.0",
		"forms/releases/k-ivsgozkxn5zgwzls/2.0.0",
		"forms/releases/k-jvxwizlmivxgi4dpnfxhi/2.0.0",
		"forms/releases/k-k5xxe23gnrxxo/1.0.0",
		"forms/releases/k-kn2gc5dfmz2wyrlooruxi6i/2.0.0",
		"forms/releases/k-kn2gc5djmnjws5df/1.0.0",
	} {
		if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(relativePath))); err != nil {
			t.Fatal(err)
		}
	}
	if strings.TrimSpace(mustTestGit(t, root, "status", "--short")) != "" {
		mustTestGit(t, root, "config", "user.name", "Takoform test")
		mustTestGit(t, root, "config", "user.email", "takoform-test@example.invalid")
		mustTestGit(t, root, "add", "--", "docs/resources", "examples/resources", "forms", "proposals", "spec/decisions/0006-v1alpha2-restarts-form-lines.md", "admission/admission-identities.json")
		mustTestGit(t, root, "commit", "-q", "-m", "fixture current generated surfaces")
	}
	planRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(standardforms.ReleasePlanPath)))
	if err != nil {
		t.Fatal(err)
	}
	var plan standardforms.ReleasePlan
	if err := json.Unmarshal(planRaw, &plan); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(mustTestGit(t, root, "rev-parse", "HEAD"))
	return planSourceFixture{
		root: root, planRaw: planRaw, plan: plan, sourceCommit: head, mainCommit: head,
	}
}

func newPlanSourceRepository(t *testing.T) planSourceFixture {
	t.Helper()
	originalRoot := testRepositoryRoot(t)
	planRaw, err := os.ReadFile(filepath.Join(originalRoot, filepath.FromSlash(standardforms.ReleasePlanPath)))
	if err != nil {
		t.Fatal(err)
	}
	var plan standardforms.ReleasePlan
	if err := json.Unmarshal(planRaw, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Releases) != expectedPlanCount {
		t.Fatalf("source plan releases = %d, want %d", len(plan.Releases), expectedPlanCount)
	}
	root := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(root, "forms"), 0o755); err != nil {
		t.Fatal(err)
	}
	copyTestFile(t,
		filepath.Join(originalRoot, filepath.FromSlash(standardforms.ReleasePlanPath)),
		filepath.Join(root, filepath.FromSlash(standardforms.ReleasePlanPath)),
	)
	copyTestFile(t,
		filepath.Join(originalRoot, "forms", "standard-package-set.json"),
		filepath.Join(root, "forms", "standard-package-set.json"),
	)
	copyTestFile(t,
		filepath.Join(originalRoot, filepath.FromSlash(trustedRootSourcePath)),
		filepath.Join(root, filepath.FromSlash(trustedRootSourcePath)),
	)
	copied := make(map[string]struct{}, len(plan.Releases))
	for _, release := range plan.Releases {
		if _, ok := copied[release.SourcePath]; ok {
			continue
		}
		copyTestTree(t,
			filepath.Join(originalRoot, filepath.FromSlash(release.SourcePath)),
			filepath.Join(root, filepath.FromSlash(release.SourcePath)),
		)
		copied[release.SourcePath] = struct{}{}
	}
	mustTestGit(t, root, "init", "-q", "--initial-branch=main")
	mustTestGit(t, root, "config", "user.name", "Takoform test")
	mustTestGit(t, root, "config", "user.email", "takoform-test@example.invalid")
	mustTestGit(t, root, "add", "--", "forms", "admission")
	mustTestGit(t, root, "commit", "-q", "-m", "fixture release sources")
	sourceCommit := strings.TrimSpace(mustTestGit(t, root, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "protected-main-marker"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustTestGit(t, root, "add", "--", "protected-main-marker")
	mustTestGit(t, root, "commit", "-q", "-m", "fixture protected main")
	mainCommit := strings.TrimSpace(mustTestGit(t, root, "rev-parse", "HEAD"))
	if sourceCommit == mainCommit || !commitPattern.MatchString(sourceCommit) || !commitPattern.MatchString(mainCommit) {
		t.Fatal("fixture commits were not initialized")
	}
	return planSourceFixture{
		root: root, planRaw: planRaw, plan: plan, sourceCommit: sourceCommit, mainCommit: mainCommit,
	}
}

func newCurrentPlanSourceRepository(t *testing.T) (planSourceFixture, standardforms.CurrentFormReleaseIdentity) {
	t.Helper()
	originalRoot := testRepositoryRoot(t)
	root := filepath.Join(t.TempDir(), "source")
	copyTestFile(t,
		filepath.Join(originalRoot, "forms", "lifecycle.schema.json"),
		filepath.Join(root, "forms", "lifecycle.schema.json"),
	)
	copyTestFile(t,
		filepath.Join(originalRoot, filepath.FromSlash(trustedRootSourcePath)),
		filepath.Join(root, filepath.FromSlash(trustedRootSourcePath)),
	)
	for _, relativePath := range []string{
		"proposals/example.md",
		"decisions/example-proposal.md",
		"decisions/example-experimental.md",
		"evidence/fixtures.md",
		"evidence/host.md",
		"evidence/consumer.md",
		"evidence/known-limitations.md",
		"evidence/compatibility.md",
		"evidence/migration.md",
		"evidence/security-review.md",
		"evidence/documentation.md",
		"evidence/publication-plan.md",
	} {
		writeCurrentFixtureFile(t, root, relativePath, []byte("reviewed fixture evidence\n"))
	}
	report, locator := writeCurrentPackageFixture(t, root)
	lifecycle := map[string]any{
		"format":        "takoform.form-lifecycle@v2",
		"projectStatus": "experimental",
		"currentEpoch":  formpackage.CurrentFormAPIVersion,
		"states":        []string{"proposal", "experimental", "stable", "legacy"},
		"legacy": map[string]any{
			"apiVersion":               formpackage.LegacyFormAPIVersion,
			"decision":                 "spec/decisions/0004-takoform-is-an-experimental-specification.md",
			"epochDecision":            "spec/decisions/0006-v1alpha2-restarts-form-lines.md",
			"releaseSources":           "forms/releases",
			"releaseSourceInventory":   map[string]any{"format": "takoform.legacy-release-inventory@v1", "count": 1, "digest": "sha256:" + strings.Repeat("0", 64)},
			"historicalAdmissionRoots": []string{"admission/v1", "admission/v3", "admission/v4"},
			"newCreatePolicy":          "host-policy",
			"retainedCapabilities":     []string{"read", "observe", "delete", "recovery", "migration"},
		},
		"proposals": []any{map[string]any{
			"id": "p-example", "document": "proposals/example.md", "owner": "maintainer:example",
			"consumer": "consumer:example", "intendedHosts": []string{"host:example"},
			"workload": "example workload", "portableBoundary": "portable desired state only",
			"portableFields":   []string{"name"},
			"hostDecisions":    []string{"placement"},
			"lifecycleRisks":   map[string]any{"replacement": "reviewed", "dataLoss": "reviewed", "delete": "reviewed", "import": "reviewed", "drift": "reviewed"},
			"securityBoundary": map[string]any{"credentials": "external", "network": "host-owned", "artifacts": "digest-pinned", "secrets": "excluded"},
			"priorArt": []any{
				map[string]any{"name": "OCCI", "applicability": "applicable", "finding": "reviewed"},
				map[string]any{"name": "CIMI", "applicability": "applicable", "finding": "reviewed"},
				map[string]any{"name": "TOSCA", "applicability": "not-applicable", "finding": "reviewed"},
				map[string]any{"name": "Kubernetes/Crossplane", "applicability": "applicable", "finding": "reviewed"},
				map[string]any{"name": "Terraform/OpenTofu", "applicability": "applicable", "finding": "reviewed"},
			},
			"existingAbstractionGap": "existing APIs do not expose this boundary",
		}},
		"currentForms": []any{map[string]any{
			"proposalId": "p-example", "state": "experimental", "owner": "maintainer:example",
			"formRef": report.FormRef, "packageDigest": report.PackageDigest, "packagePath": locator.SourcePath,
			"history": []any{
				map[string]any{"state": "proposal", "decision": "decisions/example-proposal.md"},
				map[string]any{"state": "experimental", "decision": "decisions/example-experimental.md"},
			},
			"evidence": map[string]any{
				"definition": locator.SourcePath + "/definition.json", "fixtures": "evidence/fixtures.md",
				"hostImplementations": []any{map[string]any{"subject": "host:example", "maintainer": "maintainer:host", "evidence": "evidence/host.md"}},
				"realConsumers":       []any{map[string]any{"subject": "consumer:example", "evidence": "evidence/consumer.md"}},
				"knownLimitations":    "evidence/known-limitations.md", "compatibility": "evidence/compatibility.md",
				"migration": "evidence/migration.md", "securityReview": "evidence/security-review.md",
				"documentation": "evidence/documentation.md", "publicationPlan": "evidence/publication-plan.md",
			},
		}},
	}
	writeCurrentFixtureFile(t, root, "forms/lifecycle.json", append(canonicalTestJSON(t, lifecycle), '\n'))
	mustTestGit(t, root, "init", "-q", "--initial-branch=main")
	mustTestGit(t, root, "config", "user.name", "Takoform test")
	mustTestGit(t, root, "config", "user.email", "takoform-test@example.invalid")
	mustTestGit(t, root, "add", "--", ".")
	mustTestGit(t, root, "commit", "-q", "-m", "fixture current release source")
	sourceCommit := strings.TrimSpace(mustTestGit(t, root, "rev-parse", "HEAD"))
	writeCurrentFixtureFile(t, root, "protected-main-marker", []byte("main\n"))
	mustTestGit(t, root, "add", "--", "protected-main-marker")
	mustTestGit(t, root, "commit", "-q", "-m", "fixture protected main")
	mainCommit := strings.TrimSpace(mustTestGit(t, root, "rev-parse", "HEAD"))
	plan, err := standardforms.CurrentReleasePlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Releases) != 1 {
		t.Fatalf("current fixture release plan = %#v", plan)
	}
	return planSourceFixture{root: root, sourceCommit: sourceCommit, mainCommit: mainCommit}, plan.Releases[0]
}

func writeCurrentPackageFixture(t *testing.T, root string) (formpackage.VerificationReport, formpackage.PublicationLocator) {
	t.Helper()
	desiredRaw := canonicalTestJSON(t, map[string]any{"name": "example"})
	negativeRaw := canonicalTestJSON(t, map[string]any{})
	definitionRaw := canonicalTestJSON(t, map[string]any{
		"apiVersion": formpackage.CurrentFormAPIVersion, "kind": "Example", "definitionVersion": "0.1.0",
		"title": "Example Experimental Form",
		"desiredSchema": map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
			"additionalProperties": false, "required": []string{"name"},
			"properties": map[string]any{"name": map[string]any{"type": "string", "minLength": 1}},
		},
		"observedSchema": map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
			"additionalProperties": false, "properties": map[string]any{},
		},
		"lifecycleCapabilities": []string{"create", "read", "update", "delete", "import", "observe", "refresh", "drift"},
		"conformanceFixtures":   []any{map[string]any{"name": "basic", "desiredPath": "fixtures/desired.json"}},
		"negativeConformanceFixtures": []any{map[string]any{
			"name": "missing-name", "stage": "desired", "inputPath": "fixtures/negative-missing-name.json",
			"expectedFailure": "schema_validation_failed",
		}},
	})
	schemaDigest, err := formpackage.DigestCanonicalJSON(definitionRaw)
	if err != nil {
		t.Fatal(err)
	}
	files := []formpackage.PackageFile{
		{Path: "definition.json", MediaType: formpackage.DefinitionMediaType, Size: int64(len(definitionRaw)), Digest: formpackage.DigestBytes(definitionRaw)},
		{Path: "fixtures/desired.json", MediaType: "application/json", Size: int64(len(desiredRaw)), Digest: formpackage.DigestBytes(desiredRaw)},
		{Path: "fixtures/negative-missing-name.json", MediaType: "application/json", Size: int64(len(negativeRaw)), Digest: formpackage.DigestBytes(negativeRaw)},
	}
	indexRaw := canonicalTestJSON(t, formpackage.PackageIndex{
		APIVersion: formpackage.CurrentPackageAPIVersion, Kind: formpackage.PackageKind,
		FormRef:        formpackage.FormRef{APIVersion: formpackage.CurrentFormAPIVersion, Kind: "Example", DefinitionVersion: "0.1.0", SchemaDigest: schemaDigest},
		DefinitionPath: "definition.json", Files: files,
	})
	staging := filepath.Join(root, "package-staging")
	writeCurrentFixtureFile(t, staging, "definition.json", definitionRaw)
	writeCurrentFixtureFile(t, staging, "fixtures/desired.json", desiredRaw)
	writeCurrentFixtureFile(t, staging, "fixtures/negative-missing-name.json", negativeRaw)
	writeCurrentFixtureFile(t, staging, formpackage.PackageIndexFilename, indexRaw)
	report, err := formpackage.VerifyDirectory(staging)
	if err != nil {
		t.Fatal(err)
	}
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		t.Fatal(err)
	}
	locator, err := formpackage.PublicationLocatorFor(index, report.PackageDigest)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, filepath.FromSlash(locator.SourcePath))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(staging, destination); err != nil {
		t.Fatal(err)
	}
	return report, locator
}

func writeCurrentFixtureFile(t *testing.T, root, relativePath string, raw []byte) {
	t.Helper()
	destination := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyTestTree(t *testing.T, source, destination string) {
	t.Helper()
	if err := filepath.WalkDir(source, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture source contains symlink %s", name)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("fixture source contains non-regular file %s", name)
		}
		copyTestFile(t, name, target)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func copyTestFile(t *testing.T, source, destination string) {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustTestGit(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	raw, err := gitOutput(context.Background(), root, arguments...)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func newPlanFakeGitHub(t *testing.T, source planSourceFixture) *fakeGitHub {
	t.Helper()
	fake := &fakeGitHub{
		t: t, releases: make(map[string]githubRelease), bodies: make(map[int64][]byte),
		refs: make(map[string]githubGitRef), tags: make(map[string]githubGitTag),
		comparisons: make(map[string]githubComparison),
	}
	fake.refs["refs/heads/main"] = githubGitRef{
		Ref: "refs/heads/main", Object: githubGitObject{Type: "commit", SHA: source.mainCommit},
	}
	fake.comparisons[source.sourceCommit+"..."+source.mainCommit] = githubComparison{
		Status: "ahead", BaseCommit: githubGitObject{SHA: source.sourceCommit},
		MergeBaseCommit: githubGitObject{SHA: source.sourceCommit},
	}
	nextAssetID := int64(20_000)
	for position, planned := range source.plan.Releases {
		downloaded := buildPlanReleaseFixture(t, source, planned)
		names := make([]string, 0, len(downloaded))
		for name := range downloaded {
			names = append(names, name)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
		assets := make([]githubReleaseAsset, 0, len(names))
		for _, name := range names {
			raw := downloaded[name]
			assets = append(assets, githubReleaseAsset{
				ID: nextAssetID, Name: name, State: "uploaded", Size: int64(len(raw)), Digest: formpackage.DigestBytes(raw),
			})
			fake.bodies[nextAssetID] = raw
			nextAssetID++
		}
		fake.releases[planned.Tag] = githubRelease{
			ID: int64(2_000 + position), TagName: planned.Tag, Immutable: true,
			PublishedAt: time.Date(2026, 7, 29, 8, position, 0, 0, time.UTC), Assets: assets,
		}
		tagObjectOID := fmt.Sprintf("%040x", 50_000+position)
		fake.refs["refs/tags/"+planned.Tag] = githubGitRef{
			Ref: "refs/tags/" + planned.Tag, Object: githubGitObject{Type: "tag", SHA: tagObjectOID},
		}
		fake.tags[tagObjectOID] = githubGitTag{
			SHA: tagObjectOID, Tag: planned.Tag,
			Object: githubGitObject{Type: "commit", SHA: source.sourceCommit},
		}
	}
	return fake
}

func buildPlanReleaseFixture(
	t *testing.T,
	source planSourceFixture,
	planned standardforms.PlannedFormRelease,
) map[string][]byte {
	t.Helper()
	return buildExactReleaseFixture(t, source, exactPlannedFormRelease{
		Kind: planned.Kind, Slug: planned.Slug, ReleaseID: planned.ReleaseID,
		ArtifactID: planned.Version, LegacyVersion: planned.Version, Tag: planned.Tag,
		SourcePath: planned.SourcePath, FormRef: planned.FormRef,
		PackageDigest: planned.PackageDigest, APIVersion: formpackage.PackageAPIVersion,
	})
}

func buildExactReleaseFixture(
	t *testing.T,
	source planSourceFixture,
	planned exactPlannedFormRelease,
) map[string][]byte {
	t.Helper()
	packageRoot := filepath.Join(source.root, filepath.FromSlash(planned.SourcePath))
	report, err := formpackage.VerifyDirectory(packageRoot)
	if err != nil {
		t.Fatal(err)
	}
	indexRaw, err := os.ReadFile(filepath.Join(packageRoot, formpackage.PackageIndexFilename))
	if err != nil {
		t.Fatal(err)
	}
	indexRaw, err = formpackage.Canonicalize(indexRaw)
	if err != nil {
		t.Fatal(err)
	}
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		t.Fatal(err)
	}
	base := "takoform-form-" + planned.ReleaseID + "_" + planned.ArtifactID
	indexName := base + "_package-index.json"
	archiveName := base + ".tar.gz"
	bundleName := base + "_package-index.sigstore.json"
	sbomName := base + "_sbom.spdx.json"
	provenanceName := base + "_provenance.intoto.json"
	archiveRaw := buildTestPackageArchive(t, packageRoot, indexRaw, index)
	manifest := releaseManifest{
		SchemaVersion: 1, ReleaseType: "form-package", Tag: planned.Tag,
		SourceRepository: "github.com/" + repository, SourceCommit: source.sourceCommit,
		ToolingCommit: source.mainCommit, Workflow: currentPackageWorkflow,
		ReleaseID:     planned.ReleaseID,
		PackageDigest: report.PackageDigest, FormRef: report.FormRef,
		Canonicalization: "RFC8785", SignedSubject: indexName,
		SignatureBundle: bundleName, SignatureMediaType: "application/vnd.dev.sigstore.bundle.v0.3+json",
		PublisherPolicy: releasePublisherPolicy{
			OIDCIssuer: currentPackageIssuer, Identity: currentPackageIdentity,
			TagPattern: currentPackageTagScope, ToolingCommit: source.mainCommit,
		},
		PublicationReady: true, PublicationBlockers: []string{},
	}
	if planned.APIVersion == formpackage.PackageAPIVersion {
		manifest.PackageVersion = planned.LegacyVersion
		manifest.PublisherPolicy.TagPattern = legacyPackageTagScope
	} else {
		manifest.ArtifactID = planned.ArtifactID
	}
	indexMediaType := "application/vnd.takoform.package-index.v2+json"
	if planned.APIVersion == formpackage.PackageAPIVersion {
		indexMediaType = "application/vnd.takoform.package-index.v1+json"
	}
	sbomRaw := buildTestPackageSBOM(t, packageRoot, indexRaw, index, manifest)
	indexAsset := releaseAsset{
		Name: indexName, MediaType: indexMediaType,
		Size: int64(len(indexRaw)), Digest: formpackage.DigestBytes(indexRaw),
	}
	archiveAsset := releaseAsset{
		Name: archiveName, MediaType: "application/gzip",
		Size: int64(len(archiveRaw)), Digest: formpackage.DigestBytes(archiveRaw),
	}
	provenanceRaw := buildTestPackageProvenance(t, manifest, map[string]releaseAsset{
		indexName: indexAsset, archiveName: archiveAsset,
	})
	bundleRaw := buildTestSigstoreBundle(t, indexRaw)
	rawAssets := map[string]struct {
		mediaType string
		raw       []byte
	}{
		archiveName:    {mediaType: "application/gzip", raw: archiveRaw},
		indexName:      {mediaType: indexMediaType, raw: indexRaw},
		bundleName:     {mediaType: "application/vnd.dev.sigstore.bundle.v0.3+json", raw: bundleRaw},
		provenanceName: {mediaType: "application/vnd.in-toto+json", raw: provenanceRaw},
		sbomName:       {mediaType: "application/spdx+json", raw: sbomRaw},
	}
	manifest.Assets = make([]releaseAsset, 0, len(rawAssets))
	for name, asset := range rawAssets {
		manifest.Assets = append(manifest.Assets, releaseAsset{
			Name: name, MediaType: asset.mediaType, Size: int64(len(asset.raw)), Digest: formpackage.DigestBytes(asset.raw),
		})
	}
	sort.Slice(manifest.Assets, func(i, j int) bool { return manifest.Assets[i].Name < manifest.Assets[j].Name })
	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	downloaded := make(map[string][]byte, expectedAssetCount)
	for name, asset := range rawAssets {
		downloaded[name] = asset.raw
	}
	downloaded["release-manifest.json"] = append(manifestRaw, '\n')
	names := make([]string, 0, len(downloaded))
	for name := range downloaded {
		names = append(names, name)
	}
	sort.Strings(names)
	var checksums strings.Builder
	for _, name := range names {
		fmt.Fprintf(&checksums, "%s  %s\n", strings.TrimPrefix(formpackage.DigestBytes(downloaded[name]), "sha256:"), name)
	}
	downloaded["SHA256SUMS"] = []byte(checksums.String())
	return downloaded
}

func writeTestReleaseAssets(t *testing.T, assets map[string][]byte) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "release-assets")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, raw := range assets {
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func resealTestReleaseAssets(t *testing.T, assets map[string][]byte) {
	t.Helper()
	var manifest releaseManifest
	if err := json.Unmarshal(assets["release-manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	for position := range manifest.Assets {
		raw, ok := assets[manifest.Assets[position].Name]
		if !ok {
			t.Fatalf("fixture omits manifest asset %s", manifest.Assets[position].Name)
		}
		manifest.Assets[position].Size = int64(len(raw))
		manifest.Assets[position].Digest = formpackage.DigestBytes(raw)
	}
	sort.Slice(manifest.Assets, func(i, j int) bool { return manifest.Assets[i].Name < manifest.Assets[j].Name })
	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	assets["release-manifest.json"] = append(manifestRaw, '\n')
	names := make([]string, 0, len(assets)-1)
	for name := range assets {
		if name != "SHA256SUMS" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var checksums strings.Builder
	for _, name := range names {
		fmt.Fprintf(
			&checksums, "%s  %s\n",
			strings.TrimPrefix(formpackage.DigestBytes(assets[name]), "sha256:"), name,
		)
	}
	assets["SHA256SUMS"] = []byte(checksums.String())
}

func buildTestPackageArchive(
	t *testing.T,
	packageRoot string,
	indexRaw []byte,
	index formpackage.PackageIndex,
) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&output, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	write := func(name string, raw []byte) {
		header := &tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(raw)), ModTime: time.Unix(0, 0).UTC(),
			AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(), Format: tar.FormatPAX,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	write(formpackage.PackageIndexFilename, indexRaw)
	files := append([]formpackage.PackageFile(nil), index.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		raw, err := os.ReadFile(filepath.Join(packageRoot, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatal(err)
		}
		write(file.Path, raw)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func buildTestPackageSBOM(
	t *testing.T,
	packageRoot string,
	indexRaw []byte,
	index formpackage.PackageIndex,
	manifest releaseManifest,
) []byte {
	t.Helper()
	code, err := packageVerificationCodeFromSource(packageRoot, indexRaw, index)
	if err != nil {
		t.Fatal(err)
	}
	files := make([]spdxFile, 0, len(index.Files)+1)
	appendFile := func(name, digest string) {
		digest = strings.TrimPrefix(digest, "sha256:")
		files = append(files, spdxFile{
			FileName: "./" + name, SPDXID: "SPDXRef-File-" + releaseSPDXID(name) + "-" + digest[:12],
			Checksums:        []spdxChecksum{{Algorithm: "SHA256", ChecksumValue: digest}},
			LicenseConcluded: "NOASSERTION", LicenseInfoInFiles: []string{"NOASSERTION"}, CopyrightText: "NOASSERTION",
		})
	}
	appendFile(formpackage.PackageIndexFilename, formpackage.DigestBytes(indexRaw))
	for _, file := range index.Files {
		appendFile(file.Path, file.Digest)
	}
	relationships := []spdxRelationship{{
		SPDXElementID: "SPDXRef-DOCUMENT", RelationshipType: "DESCRIBES", RelatedSPDXElement: "SPDXRef-Package",
	}}
	for _, file := range files {
		relationships = append(relationships, spdxRelationship{
			SPDXElementID: "SPDXRef-Package", RelationshipType: "CONTAINS", RelatedSPDXElement: file.SPDXID,
		})
	}
	artifactIdentity := manifest.PackageVersion
	if artifactIdentity == "" {
		artifactIdentity = manifest.ArtifactID
	}
	return canonicalTestJSON(t, packageSBOM{
		SPDXVersion: "SPDX-2.3", DataLicense: "CC0-1.0", SPDXID: "SPDXRef-DOCUMENT",
		Name:              "Takoform Form Package " + manifest.FormRef.Kind + " " + artifactIdentity,
		DocumentNamespace: "https://forms.takoform.com/spdx/package/" + strings.TrimPrefix(manifest.PackageDigest, "sha256:"),
		CreationInfo:      spdxCreationInfo{Creators: []string{"Tool: takoform-form-package-release"}, Created: "2026-07-29T08:00:00Z"},
		Packages: []spdxPackage{{
			Name: manifest.FormRef.Kind, SPDXID: "SPDXRef-Package", VersionInfo: artifactIdentity,
			DownloadLocation: "NOASSERTION", FilesAnalyzed: true,
			PackageVerificationCode: spdxPackageVerificationCode{Value: code},
			LicenseConcluded:        "NOASSERTION", LicenseDeclared: "NOASSERTION", CopyrightText: "NOASSERTION",
		}},
		Files: files, Relationships: relationships,
	})
}

func buildTestPackageProvenance(
	t *testing.T,
	manifest releaseManifest,
	assets map[string]releaseAsset,
) []byte {
	t.Helper()
	archiveName := strings.TrimSuffix(manifest.SignedSubject, "_package-index.json") + ".tar.gz"
	subjects := make([]provenanceSubject, 0, 2)
	for _, name := range []string{manifest.SignedSubject, archiveName} {
		asset := assets[name]
		subjects = append(subjects, provenanceSubject{
			Name: name, Digest: map[string]string{"sha256": strings.TrimPrefix(asset.Digest, "sha256:")},
		})
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].Name < subjects[j].Name })
	return canonicalTestJSON(t, packageProvenance{
		Type: "https://in-toto.io/Statement/v1", Subject: subjects,
		PredicateType: "https://slsa.dev/provenance/v1",
		Predicate: provenancePredicate{
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
		},
	})
}

func buildTestSigstoreBundle(t *testing.T, subject []byte) []byte {
	t.Helper()
	digest := sha256.Sum256(subject)
	return canonicalTestJSON(t, map[string]any{
		"mediaType": "application/vnd.dev.sigstore.bundle.v0.3+json",
		"verificationMaterial": map[string]any{
			"certificate": map[string]string{
				"rawBytes": base64.StdEncoding.EncodeToString([]byte("fixture-certificate")),
			},
			"tlogEntries": []any{map[string]any{
				"canonicalizedBody": base64.StdEncoding.EncodeToString([]byte("fixture-tlog-body")),
				"inclusionProof": map[string]any{
					"logIndex": "1", "treeSize": "1",
					"rootHash": base64.StdEncoding.EncodeToString(make([]byte, sha256.Size)),
					"hashes":   []any{},
					"checkpoint": map[string]string{
						"envelope": "fixture checkpoint",
					},
				},
			}},
		},
		"messageSignature": map[string]any{
			"messageDigest": map[string]string{
				"algorithm": "SHA2_256", "digest": base64.StdEncoding.EncodeToString(digest[:]),
			},
			"signature": base64.StdEncoding.EncodeToString([]byte("fixture-signature")),
		},
	})
}

func canonicalTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

type fakeGitHub struct {
	t               *testing.T
	releases        map[string]githubRelease
	bodies          map[int64][]byte
	refs            map[string]githubGitRef
	tags            map[string]githubGitTag
	comparisons     map[string]githubComparison
	requestCount    int
	firstManifestID int64
	lastAssetID     int64
}

func newFakeGitHub(t *testing.T, repoRoot string) *fakeGitHub {
	t.Helper()
	candidatesRaw, err := os.ReadFile(filepath.Join(repoRoot, "forms", "retired-package-set.json"))
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
	fake := &fakeGitHub{
		t: t, releases: make(map[string]githubRelease), bodies: make(map[int64][]byte),
		refs: make(map[string]githubGitRef), tags: make(map[string]githubGitTag),
		comparisons: make(map[string]githubComparison),
	}
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

func (fake *fakeGitHub) mutateReleaseAsset(tag, name string, mutate func([]byte) []byte) {
	fake.t.Helper()
	release, ok := fake.releases[tag]
	if !ok {
		fake.t.Fatalf("release %s not found", tag)
	}
	var assetID int64
	for _, asset := range release.Assets {
		if asset.Name == name {
			assetID = asset.ID
			break
		}
	}
	if assetID == 0 {
		fake.t.Fatalf("release %s asset %s not found", tag, name)
	}
	fake.replaceAsset(assetID, mutate(append([]byte(nil), fake.bodies[assetID]...)))
	fake.resealRelease(tag)
}

func (fake *fakeGitHub) resealRelease(tag string) {
	fake.t.Helper()
	release := fake.releases[tag]
	byName := make(map[string]githubReleaseAsset, len(release.Assets))
	for _, asset := range release.Assets {
		byName[asset.Name] = asset
	}
	manifestAsset := byName["release-manifest.json"]
	var manifest releaseManifest
	if err := json.Unmarshal(fake.bodies[manifestAsset.ID], &manifest); err != nil {
		fake.t.Fatal(err)
	}
	for index := range manifest.Assets {
		live := byName[manifest.Assets[index].Name]
		manifest.Assets[index].Size = live.Size
		manifest.Assets[index].Digest = live.Digest
	}
	sort.Slice(manifest.Assets, func(i, j int) bool { return manifest.Assets[i].Name < manifest.Assets[j].Name })
	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fake.t.Fatal(err)
	}
	fake.replaceAsset(manifestAsset.ID, append(manifestRaw, '\n'))

	release = fake.releases[tag]
	byName = make(map[string]githubReleaseAsset, len(release.Assets))
	names := make([]string, 0, len(release.Assets)-1)
	for _, asset := range release.Assets {
		byName[asset.Name] = asset
		if asset.Name != "SHA256SUMS" {
			names = append(names, asset.Name)
		}
	}
	sort.Strings(names)
	var checksums strings.Builder
	for _, name := range names {
		fmt.Fprintf(&checksums, "%s  %s\n", strings.TrimPrefix(byName[name].Digest, "sha256:"), name)
	}
	fake.replaceAsset(byName["SHA256SUMS"].ID, []byte(checksums.String()))
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
	refPrefix := "/repos/" + repository + "/git/ref/"
	if strings.HasPrefix(request.URL.Path, refPrefix) {
		ref := "refs/" + strings.TrimPrefix(request.URL.Path, refPrefix)
		value, ok := fake.refs[ref]
		if !ok {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(value); err != nil {
			fake.t.Errorf("encode fake ref: %v", err)
		}
		return
	}
	tagPrefix := "/repos/" + repository + "/git/tags/"
	if strings.HasPrefix(request.URL.Path, tagPrefix) {
		objectID := strings.TrimPrefix(request.URL.Path, tagPrefix)
		value, ok := fake.tags[objectID]
		if !ok {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(value); err != nil {
			fake.t.Errorf("encode fake tag object: %v", err)
		}
		return
	}
	comparePrefix := "/repos/" + repository + "/compare/"
	if strings.HasPrefix(request.URL.Path, comparePrefix) {
		comparisonID := strings.TrimPrefix(request.URL.Path, comparePrefix)
		value, ok := fake.comparisons[comparisonID]
		if !ok {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(value); err != nil {
			fake.t.Errorf("encode fake comparison: %v", err)
		}
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
