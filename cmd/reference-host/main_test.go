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

	// Both banner fields are waited for, not just the first. The origin and the
	// discovery URL are written on separate lines, so a loop that stopped at
	// "listening on" could snapshot the buffer between the two and read an
	// empty discovery URL — a flake that would look like the defect this test
	// exists to catch. Give a concurrently executing complete gate up to 30
	// seconds to schedule and start the host; each poll still checks an early
	// process exit immediately.
	var origin, announcedDiscovery string
	for attempt := 0; attempt < 1200 && (origin == "" || announcedDiscovery == ""); attempt++ {
		select {
		case err := <-done:
			t.Fatalf("reference host exited before serving: %v", err)
		default:
		}
		for _, line := range strings.Split(announced.String(), "\n") {
			if _, address, found := strings.Cut(line, "listening on "); found {
				origin = strings.TrimSpace(address)
			}
			if _, address, found := strings.Cut(line, "discovery "); found {
				announcedDiscovery = strings.TrimSpace(address)
			}
		}
		if origin == "" || announcedDiscovery == "" {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if origin == "" {
		t.Fatal("reference host never announced an origin")
	}
	if announcedDiscovery == "" {
		t.Fatal("reference host never announced a discovery URL")
	}

	// The banner is the operator-facing surface: it is what a reader copies to
	// point a provider at this host. It named the RETAINED lane's address while
	// the host served the current one, so the URL it printed answered 404 —
	// which is why the address is driven from the banner here rather than
	// composed from a constant the banner does not use.
	if announcedDiscovery != origin+"/.well-known/takoform/v1" {
		t.Fatalf(
			"the banner announces %q, but this host serves its discovery at %q; an operator "+
				"following the banner reaches a lane that does not answer",
			announcedDiscovery, origin+"/.well-known/takoform/v1",
		)
	}

	request, err := http.NewRequest(http.MethodGet, announcedDiscovery, nil)
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
		discovery.APIVersions[0] != "forms.takoform.com/v1" {
		t.Fatalf("api_versions = %v, want exactly the current lane", discovery.APIVersions)
	}
	if !strings.HasSuffix(discovery.Endpoints["api"], "/apis/forms.takoform.com/v1") {
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

// The command's default contract is the stable generic Host corpus. Retained
// beta and worker-authoring runs name their own corpus explicitly and do not
// decide what a reader gets by default.
func TestReferenceHostDefaultsToTheMeasuredCorpus(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	const corpus = `"conformance", "takoform-v1", "family-host", "edge", "portable-host"`
	if !strings.Contains(string(source), corpus) {
		t.Fatalf("cmd/reference-host does not default to %s", corpus)
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
