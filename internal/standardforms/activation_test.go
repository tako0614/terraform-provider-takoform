package standardforms

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestLiveActivationRequiresExactImmutableReleaseAndCompletedControllerRun(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	setRaw, err := os.ReadFile(filepath.Join(root, "admission", "v1", "standard-admission-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	matrixRaw, err := os.ReadFile(filepath.Join(root, "admission", "v1", "registry", "provider-lifecycle-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	var set struct {
		AdmissionReleaseTag string `json:"admissionReleaseTag"`
	}
	if err := json.Unmarshal(setRaw, &set); err != nil {
		t.Fatal(err)
	}
	version := strings.TrimPrefix(set.AdmissionReleaseTag, "forms/admissions/v")
	sourceCommit := testGitOutput(t, root, "rev-parse", "HEAD")
	releaseURL := admissionRepositoryURL + "/releases/tag/" + set.AdmissionReleaseTag

	assets := map[string][]byte{
		"fresh-provider-lifecycle-matrix.json":                               matrixRaw,
		"standard-admission-set.json":                                        setRaw,
		"standard-admission-set.sigstore.json":                               []byte("signed-bundle\n"),
		"takoform-standard-admission-v1.tar.gz":                              testActivationArchive(t, setRaw, matrixRaw),
		"takoform-standard-admission_" + version + "_provenance.intoto.json": []byte("{\"predicateType\":\"https://slsa.dev/provenance/v1\"}\n"),
		"takoform-standard-admission_" + version + "_sbom.spdx.json":         []byte("{\"spdxVersion\":\"SPDX-2.3\"}\n"),
	}
	artifactNames := admissionCandidateAssetNames(version)
	checksumNames := append([]string(nil), artifactNames...)
	checksumNames = checksumNames[1:]
	var checksums strings.Builder
	for _, name := range checksumNames {
		fmt.Fprintf(&checksums, "%s  %s\n", strings.TrimPrefix(activationDigest(assets[name]), "sha256:"), name)
	}
	assets["SHA256SUMS"] = []byte(checksums.String())

	targetJSON := `{"ociImages":[],"release":{"tag":` + fmt.Sprintf("%q", set.AdmissionReleaseTag) + `,"url":` + fmt.Sprintf("%q", releaseURL) + `}}`
	readback := activationReadback{
		Kind:              "takos.release-safety-adapter-result@v1",
		Status:            "promoted",
		SurfaceID:         admissionSurfaceID,
		SourceCommit:      sourceCommit,
		ControllerCommit:  strings.Repeat("a", 40),
		ControllerDigest:  "sha256:" + strings.Repeat("b", 64),
		AdapterDigest:     "sha256:" + strings.Repeat("c", 64),
		TargetFingerprint: activationDigest([]byte(targetJSON)),
		AttestationDigest: "sha256:" + strings.Repeat("d", 64),
		ReleaseTag:        set.AdmissionReleaseTag,
		ReleaseURL:        releaseURL,
		WorkflowRunID:     "12345",
		ReadbackAt:        time.Now().UTC().Format(time.RFC3339),
	}
	for _, name := range artifactNames {
		readback.ArtifactDigests = append(readback.ArtifactDigests, activationDigest(assets[name]))
	}
	for _, name := range []string{
		"published Form Package closure readback",
		"OpenTofu and Terraform Registry admission readback",
		"signed portable-standard admission closure readback",
		"immutable admission release asset readback",
	} {
		readback.HealthChecks = append(readback.HealthChecks, activationHealthCheck{Name: name, Status: "passed", BindingDigest: "sha256:" + strings.Repeat("e", 64)})
	}
	readbackRaw, err := json.Marshal(readback)
	if err != nil {
		t.Fatal(err)
	}
	assets["release-safety-readback.json"] = append(readbackRaw, '\n')

	assetNames := make([]string, 0, len(assets))
	for name := range assets {
		assetNames = append(assetNames, name)
	}
	sort.Strings(assetNames)
	assetByID := map[string]string{}
	immutable := true
	workflowConclusion := "success"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(response, "missing scoped token", http.StatusUnauthorized)
			return
		}
		switch {
		case strings.Contains(request.URL.Path, "/releases/tags/"):
			listed := make([]activationAsset, 0, len(assetNames))
			for index, name := range assetNames {
				id := fmt.Sprintf("%d", index+1)
				assetByID[id] = name
				listed = append(listed, activationAsset{Name: name, URL: server.URL + "/repos/" + admissionRepository + "/releases/assets/" + id, Size: int64(len(assets[name]))})
			}
			_ = json.NewEncoder(response).Encode(activationRelease{TagName: set.AdmissionReleaseTag, Immutable: immutable, Assets: listed})
		case strings.Contains(request.URL.Path, "/releases/assets/"):
			id := filepath.Base(request.URL.Path)
			name, ok := assetByID[id]
			if !ok {
				http.NotFound(response, request)
				return
			}
			_, _ = response.Write(assets[name])
		case strings.HasSuffix(request.URL.Path, "/actions/runs/12345"):
			_ = json.NewEncoder(response).Encode(activationWorkflowRun{HeadSHA: sourceCommit, Event: "workflow_dispatch", Status: "completed", Conclusion: workflowConclusion, RunAttempt: 1, Path: admissionWorkflowPath, DisplayTitle: admissionRunNamePrefix + "test"})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	client, err := newActivationGitHubClient(server.URL+"/", server.Client(), "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyLiveActivation(context.Background(), root, client); err != nil {
		t.Fatal(err)
	}
	immutable = false
	if err := verifyLiveActivation(context.Background(), root, client); err == nil || !strings.Contains(err.Error(), "stable immutable activation") {
		t.Fatalf("mutable release error = %v", err)
	}
	immutable = true
	workflowConclusion = "failure"
	if err := verifyLiveActivation(context.Background(), root, client); err == nil || !strings.Contains(err.Error(), "controller workflow did not complete") {
		t.Fatalf("failed controller run error = %v", err)
	}
	workflowConclusion = "success"

	badReadback := readback
	badReadback.ArtifactDigests = append([]string(nil), readback.ArtifactDigests...)
	badReadback.ArtifactDigests[0] = "sha256:" + strings.Repeat("f", 64)
	badRaw, err := json.Marshal(badReadback)
	if err != nil {
		t.Fatal(err)
	}
	assets["release-safety-readback.json"] = append(badRaw, '\n')
	if err := verifyLiveActivation(context.Background(), root, client); err == nil || !strings.Contains(err.Error(), "artifact digest mismatch") {
		t.Fatalf("substituted controller readback error = %v", err)
	}
}

func testActivationArchive(t *testing.T, setRaw, matrixRaw []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range []struct {
		name string
		raw  []byte
	}{
		{"v1/standard-admission-set.json", setRaw},
		{"v1/registry/provider-lifecycle-matrix.json", matrixRaw},
	} {
		if err := tarWriter.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o644, Size: int64(len(entry.raw)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(entry.raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func testGitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
