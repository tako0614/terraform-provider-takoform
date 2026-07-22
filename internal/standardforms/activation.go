package standardforms

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
)

const (
	admissionRepository        = "tako0614/terraform-provider-takoform"
	admissionRepositoryURL     = "https://github.com/tako0614/terraform-provider-takoform"
	admissionSurfaceID         = "takoform-standard-form-admission"
	admissionWorkflowPath      = ".github/workflows/standard-admission-release.yml"
	admissionRunNamePrefix     = "release-safety:standard-form-admission:"
	githubAPIBaseURL           = "https://api.github.com/"
	maxActivationMetadataBytes = 4 << 20
	maxActivationAssetBytes    = 128 << 20
	maxActivationTotalBytes    = 256 << 20
)

var (
	activationDigestPattern  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	activationCommitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	activationVersionPattern = regexp.MustCompile(
		`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`,
	)
)

type activationGitHubClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	token      string
}

type activationRelease struct {
	TagName    string            `json:"tag_name"`
	Draft      bool              `json:"draft"`
	Prerelease bool              `json:"prerelease"`
	Immutable  bool              `json:"immutable"`
	Assets     []activationAsset `json:"assets"`
}

type activationAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type activationWorkflowRun struct {
	HeadSHA      string `json:"head_sha"`
	Event        string `json:"event"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	RunAttempt   int    `json:"run_attempt"`
	Path         string `json:"path"`
	DisplayTitle string `json:"display_title"`
}

type activationReadback struct {
	Kind              string                  `json:"kind"`
	Status            string                  `json:"status"`
	SurfaceID         string                  `json:"surfaceId"`
	SourceCommit      string                  `json:"sourceCommit"`
	ControllerCommit  string                  `json:"controllerCommit"`
	ControllerDigest  string                  `json:"controllerDigest"`
	AdapterDigest     string                  `json:"adapterDigest"`
	ArtifactDigests   []string                `json:"artifactDigests"`
	TargetFingerprint string                  `json:"targetFingerprint"`
	AttestationDigest string                  `json:"attestationDigest"`
	ReleaseTag        string                  `json:"releaseTag"`
	ReleaseURL        string                  `json:"releaseUrl"`
	WorkflowRunID     string                  `json:"workflowRunId"`
	ReadbackAt        string                  `json:"readbackAt"`
	HealthChecks      []activationHealthCheck `json:"healthChecks"`
}

type activationHealthCheck struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	BindingDigest string `json:"bindingDigest"`
}

// VerifyReleaseReady is the public Form-admission activation gate. The exact
// offline closure must pass first, then the matching controller promotion run,
// repository-enforced immutable GitHub Release, exact eight-asset inventory,
// and retained release-safety readback are verified live. A candidate Actions
// artifact cannot satisfy this gate.
func VerifyReleaseReady(ctx context.Context, root string, httpClient *http.Client, token string) error {
	if err := VerifyAdmissionClosure(root); err != nil {
		return err
	}
	client, err := newActivationGitHubClient(githubAPIBaseURL, httpClient, token)
	if err != nil {
		return err
	}
	return verifyLiveActivation(ctx, root, client)
}

func newActivationGitHubClient(rawBaseURL string, httpClient *http.Client, token string) (*activationGitHubClient, error) {
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub API base URL: %w", err)
	}
	if baseURL.Host == "" || baseURL.RawQuery != "" || baseURL.Fragment != "" || (baseURL.Scheme != "https" && baseURL.Scheme != "http") {
		return nil, fmt.Errorf("GitHub API base URL must be an absolute HTTP(S) URL without query or fragment")
	}
	if baseURL.Scheme == "http" && baseURL.Hostname() != "127.0.0.1" && baseURL.Hostname() != "localhost" && baseURL.Hostname() != "::1" {
		return nil, fmt.Errorf("plain HTTP GitHub API base URL is allowed only for loopback tests")
	}
	if !strings.HasSuffix(baseURL.Path, "/") {
		baseURL.Path += "/"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &activationGitHubClient{baseURL: baseURL, httpClient: httpClient, token: strings.TrimSpace(token)}, nil
}

func verifyLiveActivation(ctx context.Context, root string, client *activationGitHubClient) error {
	if client == nil {
		return fmt.Errorf("GitHub activation client is required")
	}
	var set admissionrelease.Set
	if err := readJSON(filepath.Join(root, "admission", "v1", "standard-admission-set.json"), &set); err != nil {
		return err
	}
	version := strings.TrimPrefix(set.AdmissionReleaseTag, "forms/admissions/v")
	if !activationVersionPattern.MatchString(version) || set.AdmissionReleaseTag != "forms/admissions/v"+version {
		return fmt.Errorf("admission release tag is not an exact stable version")
	}
	sourceCommit, err := activationGitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolve activation source commit: %w", err)
	}
	if !activationCommitPattern.MatchString(sourceCommit) {
		return fmt.Errorf("resolve activation source commit returned invalid commit %q", sourceCommit)
	}

	var release activationRelease
	endpoint := "repos/" + admissionRepository + "/releases/tags/" + url.PathEscape(set.AdmissionReleaseTag)
	if err := client.getJSON(ctx, endpoint, &release); err != nil {
		return fmt.Errorf("Form admission activation is blocked: immutable GitHub Release readback: %w", err)
	}
	if release.TagName != set.AdmissionReleaseTag || release.Draft || release.Prerelease || !release.Immutable {
		return fmt.Errorf("Form admission activation is blocked: GitHub Release is not the exact stable immutable activation")
	}

	artifactNames := admissionCandidateAssetNames(version)
	expectedNames := append(append([]string(nil), artifactNames...), "release-safety-readback.json")
	sort.Strings(expectedNames)
	assets := make(map[string]activationAsset, len(release.Assets))
	for _, asset := range release.Assets {
		if asset.Name == "" || asset.URL == "" || asset.Size < 0 || asset.Size > maxActivationAssetBytes {
			return fmt.Errorf("activation release contains an invalid asset")
		}
		if _, duplicate := assets[asset.Name]; duplicate {
			return fmt.Errorf("activation release duplicates asset %q", asset.Name)
		}
		assets[asset.Name] = asset
	}
	actualNames := make([]string, 0, len(assets))
	for name := range assets {
		actualNames = append(actualNames, name)
	}
	sort.Strings(actualNames)
	if strings.Join(actualNames, "\x00") != strings.Join(expectedNames, "\x00") {
		return fmt.Errorf("activation release is not the exact eight-asset inventory")
	}

	downloaded := make(map[string][]byte, len(assets))
	var total int64
	for _, name := range expectedNames {
		asset := assets[name]
		raw, err := client.getAsset(ctx, asset)
		if err != nil {
			return fmt.Errorf("read activation asset %s: %w", name, err)
		}
		total += int64(len(raw))
		if total > maxActivationTotalBytes {
			return fmt.Errorf("activation release assets exceed the bounded total size")
		}
		downloaded[name] = raw
	}

	var readback activationReadback
	if err := decodeStrictActivationJSON(downloaded["release-safety-readback.json"], &readback); err != nil {
		return fmt.Errorf("decode activation controller readback: %w", err)
	}
	releaseURL := admissionRepositoryURL + "/releases/tag/" + set.AdmissionReleaseTag
	if err := validateActivationReadback(readback, set.AdmissionReleaseTag, releaseURL, sourceCommit, artifactNames, downloaded); err != nil {
		return fmt.Errorf("Form admission activation is blocked: controller readback: %w", err)
	}

	var workflowRun activationWorkflowRun
	if err := client.getJSON(ctx, "repos/"+admissionRepository+"/actions/runs/"+readback.WorkflowRunID, &workflowRun); err != nil {
		return fmt.Errorf("Form admission activation is blocked: controller workflow readback: %w", err)
	}
	workflowPathMatches := workflowRun.Path == admissionWorkflowPath || strings.HasSuffix(workflowRun.Path, "/"+admissionWorkflowPath)
	if workflowRun.HeadSHA != sourceCommit || workflowRun.Event != "workflow_dispatch" || workflowRun.Status != "completed" || workflowRun.Conclusion != "success" || workflowRun.RunAttempt != 1 || !workflowPathMatches || !strings.HasPrefix(workflowRun.DisplayTitle, admissionRunNamePrefix) {
		return fmt.Errorf("Form admission activation is blocked: controller workflow did not complete the exact protected promotion")
	}

	localSet, err := os.ReadFile(filepath.Join(root, "admission", "v1", "standard-admission-set.json"))
	if err != nil {
		return err
	}
	localMatrix, err := os.ReadFile(filepath.Join(root, "admission", "v1", "registry", "provider-lifecycle-matrix.json"))
	if err != nil {
		return err
	}
	if !bytes.Equal(downloaded["standard-admission-set.json"], localSet) || !bytes.Equal(downloaded["fresh-provider-lifecycle-matrix.json"], localMatrix) {
		return fmt.Errorf("activation release does not contain the exact reviewed set and Registry matrix bytes")
	}
	if err := verifyActivationChecksums(downloaded, artifactNames); err != nil {
		return err
	}
	if err := verifyActivationArchive(downloaded["takoform-standard-admission-v1.tar.gz"], localSet, localMatrix); err != nil {
		return err
	}
	return nil
}

func admissionCandidateAssetNames(version string) []string {
	names := []string{
		"SHA256SUMS",
		"fresh-provider-lifecycle-matrix.json",
		"standard-admission-set.json",
		"standard-admission-set.sigstore.json",
		"takoform-standard-admission-v1.tar.gz",
		"takoform-standard-admission_" + version + "_provenance.intoto.json",
		"takoform-standard-admission_" + version + "_sbom.spdx.json",
	}
	sort.Strings(names)
	return names
}

func validateActivationReadback(readback activationReadback, tag, releaseURL, sourceCommit string, artifactNames []string, downloaded map[string][]byte) error {
	if readback.Kind != "takos.release-safety-adapter-result@v1" || readback.Status != "promoted" || readback.SurfaceID != admissionSurfaceID || readback.SourceCommit != sourceCommit || readback.ReleaseTag != tag || readback.ReleaseURL != releaseURL || !activationCommitPattern.MatchString(readback.ControllerCommit) || !activationDigestPattern.MatchString(readback.ControllerDigest) || !activationDigestPattern.MatchString(readback.AdapterDigest) || !activationDigestPattern.MatchString(readback.TargetFingerprint) || !activationDigestPattern.MatchString(readback.AttestationDigest) {
		return fmt.Errorf("identity, authority, or digest binding is incomplete")
	}
	if _, err := strconv.ParseUint(readback.WorkflowRunID, 10, 64); err != nil || readback.WorkflowRunID == "0" {
		return fmt.Errorf("workflow run id is not a positive decimal integer")
	}
	if _, err := time.Parse(time.RFC3339, readback.ReadbackAt); err != nil {
		return fmt.Errorf("readback time is invalid")
	}
	targetJSON := `{"ociImages":[],"release":{"tag":` + strconv.Quote(tag) + `,"url":` + strconv.Quote(releaseURL) + `}}`
	targetDigest := sha256.Sum256([]byte(targetJSON))
	if readback.TargetFingerprint != "sha256:"+hex.EncodeToString(targetDigest[:]) {
		return fmt.Errorf("target fingerprint does not bind the immutable release")
	}
	if len(readback.ArtifactDigests) != len(artifactNames) {
		return fmt.Errorf("artifact digest closure is not exact")
	}
	for i, name := range artifactNames {
		actual := activationDigest(downloaded[name])
		if readback.ArtifactDigests[i] != actual {
			return fmt.Errorf("artifact digest mismatch for %s", name)
		}
	}
	expectedChecks := []string{
		"published Form Package closure readback",
		"OpenTofu and Terraform Registry admission readback",
		"signed portable-standard admission closure readback",
		"immutable admission release asset readback",
	}
	if len(readback.HealthChecks) != len(expectedChecks) {
		return fmt.Errorf("health-check closure is not exact")
	}
	for i, check := range readback.HealthChecks {
		if check.Name != expectedChecks[i] || check.Status != "passed" || !activationDigestPattern.MatchString(check.BindingDigest) {
			return fmt.Errorf("health-check %d is not the exact passed controller check", i)
		}
	}
	return nil
}

func verifyActivationChecksums(downloaded map[string][]byte, artifactNames []string) error {
	lines := strings.Split(strings.TrimSuffix(string(downloaded["SHA256SUMS"]), "\n"), "\n")
	expected := make([]string, 0, len(artifactNames)-1)
	for _, name := range artifactNames {
		if name != "SHA256SUMS" {
			expected = append(expected, name)
		}
	}
	if len(lines) != len(expected) {
		return fmt.Errorf("activation SHA256SUMS inventory is not exact")
	}
	for i, line := range lines {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || parts[1] != expected[i] || len(parts[0]) != 64 {
			return fmt.Errorf("activation SHA256SUMS line %d is invalid", i+1)
		}
		actual := strings.TrimPrefix(activationDigest(downloaded[expected[i]]), "sha256:")
		if parts[0] != actual {
			return fmt.Errorf("activation SHA256SUMS digest mismatch for %s", expected[i])
		}
	}
	return nil
}

func verifyActivationArchive(raw, expectedSet, expectedMatrix []byte) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("open activation archive: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	wanted := map[string][]byte{
		"v1/standard-admission-set.json":             expectedSet,
		"v1/registry/provider-lifecycle-matrix.json": expectedMatrix,
	}
	seen := map[string]bool{}
	entries := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read activation archive: %w", err)
		}
		entries++
		if entries > 4096 {
			return fmt.Errorf("activation archive contains too many entries")
		}
		expected, ok := wanted[path.Clean(header.Name)]
		if !ok {
			continue
		}
		if seen[path.Clean(header.Name)] || header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > maxActivationMetadataBytes {
			return fmt.Errorf("activation archive entry %q is invalid", header.Name)
		}
		entry, err := io.ReadAll(io.LimitReader(tarReader, maxActivationMetadataBytes+1))
		if err != nil || len(entry) > maxActivationMetadataBytes || !bytes.Equal(entry, expected) {
			return fmt.Errorf("activation archive entry %q differs from reviewed source", header.Name)
		}
		seen[path.Clean(header.Name)] = true
	}
	for name := range wanted {
		if !seen[name] {
			return fmt.Errorf("activation archive omits %q", name)
		}
	}
	return nil
}

func (client *activationGitHubClient) getJSON(ctx context.Context, relative string, target any) error {
	requestURL, err := client.baseURL.Parse(relative)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return err
	}
	client.setHeaders(request, "application/vnd.github+json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxActivationMetadataBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxActivationMetadataBytes {
		return fmt.Errorf("GitHub response exceeds %d bytes", maxActivationMetadataBytes)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub returned %s", response.Status)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	return nil
}

func (client *activationGitHubClient) getAsset(ctx context.Context, asset activationAsset) ([]byte, error) {
	assetURL, err := url.Parse(asset.URL)
	if err != nil {
		return nil, err
	}
	prefix := path.Join(client.baseURL.Path, "repos", admissionRepository, "releases", "assets") + "/"
	if assetURL.Scheme != client.baseURL.Scheme || assetURL.Host != client.baseURL.Host || !strings.HasPrefix(assetURL.Path, prefix) || assetURL.RawQuery != "" || assetURL.Fragment != "" {
		return nil, fmt.Errorf("asset API URL escapes the expected repository authority")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL.String(), nil)
	if err != nil {
		return nil, err
	}
	client.setHeaders(request, "application/octet-stream")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset download returned %s", response.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxActivationAssetBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxActivationAssetBytes || int64(len(raw)) != asset.Size {
		return nil, fmt.Errorf("asset size differs from immutable release metadata")
	}
	return raw, nil
}

func (client *activationGitHubClient) setHeaders(request *http.Request, accept string) {
	request.Header.Set("Accept", accept)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "takoform-standard-admission-release-check")
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}
}

func decodeStrictActivationJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("JSON contains trailing data")
	}
	return nil
}

func activationDigest(raw []byte) string {
	digest := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func activationGitOutput(ctx context.Context, root string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, arguments...)...)
	output, err := command.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
