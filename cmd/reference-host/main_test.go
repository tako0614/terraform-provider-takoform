package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/portableconformancev3"
)

// syncBuffer collects the command's banner while it is still serving.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// The README tells a reader to start this command and point a provider at it.
// A documented walk that nobody executes is a claim, so this test performs the
// first step of it: bind a port, ask the discovery document the provider asks
// for, and require the answer that makes the rest of the walk possible.
func TestReferenceHostServesTheCurrentLaneDiscovery(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	// Port 0 lets the kernel choose, and run() announces what it chose, so the
	// test drives the same banner a reader copies from.
	announced := &syncBuffer{}
	done := make(chan error, 1)
	go func() {
		done <- run([]string{"--addr", "127.0.0.1:0", "--repo-root", root}, announced)
	}()

	var origin string
	for attempt := 0; attempt < 200 && origin == ""; attempt++ {
		select {
		case err := <-done:
			t.Fatalf("reference host exited before serving: %v", err)
		default:
		}
		for _, line := range strings.Split(announced.String(), "\n") {
			if _, address, found := strings.Cut(line, "listening on "); found {
				origin = strings.TrimSpace(address)
			}
		}
		if origin == "" {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if origin == "" {
		t.Fatal("reference host never announced an origin")
	}

	request, err := http.NewRequest(http.MethodGet, origin+"/.well-known/takoform/v1beta1", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+portableconformancev3.ReferencePrimaryToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("discovery request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("discovery responded %d: %s", response.StatusCode, body)
	}

	var discovery struct {
		APIVersions []string          `json:"api_versions"`
		Endpoints   map[string]string `json:"endpoints"`
		Features    map[string]bool   `json:"features"`
	}
	if err := json.NewDecoder(response.Body).Decode(&discovery); err != nil {
		t.Fatal(err)
	}
	if len(discovery.APIVersions) != 1 ||
		discovery.APIVersions[0] != "forms.takoform.com/v1beta1" {
		t.Fatalf("api_versions = %v, want exactly the current lane", discovery.APIVersions)
	}
	if !strings.HasSuffix(discovery.Endpoints["api"], "/apis/forms.takoform.com/v1beta1") {
		t.Fatalf("endpoints.api = %q", discovery.Endpoints["api"])
	}
	for _, feature := range []string{
		"artifact_upload", "exact_form_ref", "idempotent_lifecycle",
		"operations", "optimistic_concurrency", "service_forms", "support_profiles",
	} {
		if !discovery.Features[feature] {
			t.Fatalf("feature %q is not advertised", feature)
		}
	}
}

// The command's default contract has to be the corpus the rest of the
// repository measures, or the host a reader starts is not the host
// cmd/worker-authoring-conformance drives.
func TestReferenceHostDefaultsToTheMeasuredCorpus(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	harness, err := os.ReadFile(filepath.Join(
		"..", "..", "internal", "workerauthoring", "harness.go",
	))
	if err != nil {
		t.Fatal(err)
	}
	const corpus = `"conformance", "portable-host-v1beta1"`
	if !strings.Contains(string(source), corpus) {
		t.Fatalf("cmd/reference-host does not default to %s", corpus)
	}
	if !strings.Contains(string(harness), corpus) {
		t.Fatalf("internal/workerauthoring no longer verifies %s", corpus)
	}
}

// The loopback boundary is enforced, not stated: a non-loopback bind is
// refused before serving, whatever --addr said.
func TestReferenceHostRefusesNonLoopbackAddresses(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	runErr := run([]string{"--addr", "0.0.0.0:0", "--repo-root", root}, io.Discard)
	if runErr == nil || !strings.Contains(runErr.Error(), "loopback") {
		t.Fatalf("non-loopback bind was not refused: %v", runErr)
	}
}
