package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/characterization"
)

func TestCompatibilityCandidateDiscoveryParity(t *testing.T) {
	t.Parallel()
	fixture := mustLoadDiscoveryFixture(t)
	var host Discovery
	var capabilities ProductCapabilities
	mustUnmarshal(t, fixture.Host, &host)
	mustUnmarshal(t, fixture.Capabilities, &capabilities)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/takoform":
			candidate := host
			candidate.Endpoints.Capabilities = server.URL + "/v1/capabilities"
			writeJSON(t, w, candidate)
		case "/v1/capabilities":
			writeJSON(t, w, capabilities)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	client := New(server.URL, "fixture-token", server.Client())
	discovered, err := client.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !discovered.SupportsServiceForms() || len(discovered.APIVersions) != 1 || discovered.APIVersions[0] != APIVersion {
		t.Fatalf("discovery drifted: %#v", discovered)
	}
	gotCapabilities, err := client.GetCapabilities(context.Background())
	if err != nil {
		t.Fatalf("GetCapabilities: %v", err)
	}
	if len(gotCapabilities.Resources) != len(characterization.ExpectedKinds) {
		t.Fatalf("capabilities advertise %d resources, want %d", len(gotCapabilities.Resources), len(characterization.ExpectedKinds))
	}
	for _, identity := range characterization.ExpectedKinds {
		if !gotCapabilities.SupportsResource(identity.Kind) {
			t.Errorf("capabilities do not advertise %s", identity.Kind)
		}
	}
}

func TestCompatibilityCandidateDeployWireParity(t *testing.T) {
	t.Parallel()
	desired := mustLoadResourceFixtures(t, "desired")
	observed := mustLoadResourceFixtures(t, "observed")

	for _, identity := range characterization.ExpectedKinds {
		identity := identity
		t.Run(identity.Kind, func(t *testing.T) {
			want := desired[identity.Kind]
			response := observed[identity.Kind]
			previewSeen := false
			applySeen := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				switch {
				case request.Method == http.MethodPost && request.URL.Path == "/v1/resources/preview":
					previewSeen = true
					var body Resource
					mustDecodeRequest(t, request, &body)
					assertSameCandidateJSON(t, body, want)
					writeJSON(t, w, PreviewResourceResult{Resource: body, PlanDigest: "fixture-plan-digest"})
				case request.Method == http.MethodPut && request.URL.Path == resourcePath(identity.Kind, want.Metadata.Name):
					applySeen = true
					var body struct {
						Resource
						Review DeploymentReview `json:"review"`
					}
					mustDecodeRequest(t, request, &body)
					assertSameCandidateJSON(t, body.Resource, want)
					if body.Review.PlanDigest != "fixture-plan-digest" || body.Review.QuoteID != "" || body.Review.QuoteDigest != "" {
						t.Errorf("review evidence drifted: %#v", body.Review)
					}
					writeJSON(t, w, response)
				default:
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := New(server.URL, "fixture-token", server.Client())
			got, err := client.PutResource(context.Background(), identity.Kind, want.Metadata.Name, &want)
			if err != nil {
				t.Fatalf("PutResource: %v", err)
			}
			if !previewSeen || !applySeen {
				t.Fatalf("reviewed deploy lifecycle incomplete: preview=%v apply=%v", previewSeen, applySeen)
			}
			assertSameCandidateJSON(t, *got, response)
		})
	}
}

func TestCompatibilityCandidateObserveWireParity(t *testing.T) {
	t.Parallel()
	observed := mustLoadResourceFixtures(t, "observed")
	for _, identity := range characterization.ExpectedKinds {
		identity := identity
		t.Run(identity.Kind, func(t *testing.T) {
			want := observed[identity.Kind]
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != resourcePath(identity.Kind, want.Metadata.Name)+"/observe" || request.URL.Query().Get("space") != want.Metadata.Space {
					http.Error(w, "unexpected request", http.StatusNotFound)
					return
				}
				writeJSON(t, w, want)
			}))
			defer server.Close()

			client := New(server.URL, "fixture-token", server.Client())
			got, err := client.ObserveResource(context.Background(), identity.Kind, want.Metadata.Name, want.Metadata.Space)
			if err != nil {
				t.Fatalf("ObserveResource: %v", err)
			}
			assertSameCandidateJSON(t, *got, want)
		})
	}
}

func TestCompatibilityCandidateAPIErrorParity(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", "conformance", "compatibility-candidate-v1")
	document, err := characterization.LoadCases[characterization.ErrorCase](root, "error")
	if err != nil {
		t.Fatalf("load error fixtures: %v", err)
	}
	for _, fixture := range document.Cases {
		fixture := fixture
		t.Run(fixture.Kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(fixture.API.Status)
				if _, err := w.Write(fixture.API.Body); err != nil {
					t.Errorf("write error fixture: %v", err)
				}
			}))
			defer server.Close()

			client := New(server.URL, "fixture-token", server.Client())
			_, err := client.GetResource(context.Background(), fixture.Kind, "fixture", "fixture-space")
			var apiError *APIError
			if !errors.As(err, &apiError) {
				t.Fatalf("GetResource error = %T %v, want *APIError", err, err)
			}
			if apiError.StatusCode != fixture.API.Status || apiError.Code != fixture.API.Code || apiError.Message != fixture.API.Message || apiError.RequestID != fixture.API.RequestID {
				t.Fatalf("API error drifted: %#v", apiError)
			}
		})
	}
}

func mustLoadDiscoveryFixture(t *testing.T) characterization.DiscoveryFixture {
	t.Helper()
	root := filepath.Join("..", "..", "conformance", "compatibility-candidate-v1")
	fixture, err := characterization.LoadDiscovery(root)
	if err != nil {
		t.Fatalf("load discovery fixture: %v", err)
	}
	return fixture
}

func mustLoadResourceFixtures(t *testing.T, category string) map[string]Resource {
	t.Helper()
	root := filepath.Join("..", "..", "conformance", "compatibility-candidate-v1")
	document, err := characterization.LoadCases[characterization.ResourceCase](root, category)
	if err != nil {
		t.Fatalf("load %s fixtures: %v", category, err)
	}
	result := make(map[string]Resource, len(document.Cases))
	for _, fixture := range document.Cases {
		var resource Resource
		mustUnmarshal(t, fixture.Resource, &resource)
		result[fixture.Kind] = resource
	}
	return result
}

func resourcePath(kind, name string) string {
	return "/v1/resources/" + url.PathEscape(kind) + "/" + url.PathEscape(name)
}

func mustUnmarshal(t *testing.T, raw []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
}

func mustDecodeRequest(t *testing.T, request *http.Request, target any) {
	t.Helper()
	defer request.Body.Close()
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(target); err != nil {
		t.Errorf("decode request: %v", err)
	}
}

func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func assertSameCandidateJSON(t *testing.T, got, want any) {
	t.Helper()
	gotDigest, err := characterization.DigestJSONValue(got)
	if err != nil {
		t.Fatalf("digest got JSON: %v", err)
	}
	wantDigest, err := characterization.DigestJSONValue(want)
	if err != nil {
		t.Fatalf("digest wanted JSON: %v", err)
	}
	if gotDigest != wantDigest {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("candidate JSON drifted\nwant: %s\n got: %s", wantJSON, gotJSON)
	}
}

func TestCompatibilityCandidateAPIVersionConstant(t *testing.T) {
	if APIVersion != characterization.APIVersion {
		t.Fatalf("client APIVersion = %q, candidate fixture = %q", APIVersion, characterization.APIVersion)
	}
	if ManagedByOpenTofu != "opentofu" {
		t.Fatalf("managedBy = %q, want opentofu", ManagedByOpenTofu)
	}
	for _, identity := range characterization.ExpectedKinds {
		if identity.Kind == "" || identity.ResourceType == "" {
			t.Fatal(fmt.Sprintf("invalid characterized identity: %#v", identity))
		}
	}
}
