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
	"github.com/tako0614/terraform-provider-takoform/internal/formregistry"
)

func TestCompatibilityCandidateDiscoveryParity(t *testing.T) {
	t.Parallel()
	fixture := mustLoadDiscoveryFixture(t)
	var host Discovery
	mustUnmarshal(t, fixture.Host, &host)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/.well-known/takoform" {
			http.NotFound(w, request)
			return
		}
		candidate := host
		candidate.Endpoints.API = server.URL + "/apis/" + APIVersion
		candidate.Endpoints.Forms = candidate.Endpoints.API + "/forms"
		writeJSON(t, w, candidate)
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
	for _, feature := range []string{"exact_form_ref", "optimistic_concurrency", "idempotent_lifecycle"} {
		if !discovered.HasFeature(feature) {
			t.Errorf("characterized discovery does not advertise features.%s", feature)
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
			form := mustCandidateForm(t, identity.Kind)
			want.Form = &form
			response := observed[identity.Kind]
			response.Form = &form
			previewSeen := false
			applySeen := false
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				base := "/apis/" + APIVersion
				switch {
				case request.Method == http.MethodGet && request.URL.Path == "/.well-known/takoform":
					writeVersionedDiscovery(t, w, server.URL)
				case request.Method == http.MethodGet && request.URL.Path == base+"/forms":
					writeJSON(t, w, map[string]any{"forms": []FormAvailability{{
						Identity: form, DefinitionKnown: true, Installed: true, Executable: true,
						Activated: true, AvailableToPrincipal: true,
						Operations: []string{"create", "read", "update", "delete", "import", "observe", "refresh"},
					}}})
				case request.Method == http.MethodPost && request.URL.Path == base+"/resources/preview":
					previewSeen = true
					var body Resource
					mustDecodeRequest(t, request, &body)
					assertSameCandidateJSON(t, body, want)
					writeJSON(t, w, PreviewResourceResult{Resource: body, Review: PreviewReview{PlanDigest: "fixture-plan-digest"}})
				case request.Method == http.MethodPut && request.URL.Path == resourcePath(identity.Kind, want.Metadata.Name):
					applySeen = true
					var body struct {
						Resource
						Review DeploymentReview `json:"review"`
					}
					mustDecodeRequest(t, request, &body)
					assertSameCandidateJSON(t, body.Resource, want)
					if body.Review.PlanDigest != "fixture-plan-digest" {
						t.Errorf("review evidence drifted: %#v", body.Review)
					}
					w.Header().Set("ETag", `"1"`)
					writeJSON(t, w, response)
				default:
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := New(server.URL, "fixture-token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatalf("Discover: %v", err)
			}
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
			form := mustCandidateForm(t, identity.Kind)
			want.Form = &form
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				switch {
				case request.Method == http.MethodGet && request.URL.Path == "/.well-known/takoform":
					writeVersionedDiscovery(t, w, server.URL)
				case request.Method == http.MethodPost && request.URL.Path == resourcePath(identity.Kind, want.Metadata.Name)+"/observe":
					if request.URL.Query().Get("space") != want.Metadata.Space {
						http.Error(w, "unexpected space", http.StatusNotFound)
						return
					}
					if request.Header.Get("If-Match") != `"1"` {
						t.Errorf("observe is not generation fenced: If-Match=%q", request.Header.Get("If-Match"))
					}
					w.Header().Set("ETag", `"1"`)
					writeJSON(t, w, map[string]any{"resource": want, "observation": map[string]any{"status": "current"}})
				default:
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := New(server.URL, "fixture-token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatalf("Discover: %v", err)
			}
			got, err := client.ObserveResource(context.Background(), identity.Kind, want.Metadata.Name, want.Metadata.Space,
				MutationFence{ResourceVersion: "1", Form: form})
			if err != nil {
				t.Fatalf("ObserveResource: %v", err)
			}
			got.Status.DriftStatus = ""
			got.Metadata.ResourceVersion = want.Metadata.ResourceVersion
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
			form := mustCandidateForm(t, fixture.Kind)
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/.well-known/takoform" {
					writeVersionedDiscovery(t, w, server.URL)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(fixture.API.Status)
				if _, err := w.Write(fixture.API.Body); err != nil {
					t.Errorf("write error fixture: %v", err)
				}
			}))
			defer server.Close()

			client := New(server.URL, "fixture-token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatalf("Discover: %v", err)
			}
			_, err := client.GetResource(context.Background(), fixture.Kind, "fixture", "fixture-space", form)
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

func mustCandidateForm(t *testing.T, kind string) InstalledFormReference {
	t.Helper()
	ref, err := formregistry.ForKind(kind)
	if err != nil {
		t.Fatalf("candidate FormRef for %s: %v", kind, err)
	}
	return InstalledFormReference{
		FormRef: FormRef{
			APIVersion: ref.APIVersion, Kind: ref.Kind,
			DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
		},
		PackageDigest: ref.PackageDigest,
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
	return "/apis/" + APIVersion + "/resources/" + url.PathEscape(kind) + "/" + url.PathEscape(name)
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
	for _, identity := range characterization.ExpectedKinds {
		if identity.Kind == "" || identity.ResourceType == "" {
			t.Fatal(fmt.Sprintf("invalid characterized identity: %#v", identity))
		}
	}
}
