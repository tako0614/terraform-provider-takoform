package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

func TestVersionedTypedReadsGetFenceThenObserveAndMapDrift(t *testing.T) {
	for _, kind := range typedResourceKinds() {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			form := providerReleaseForms()[kind]
			var server *httptest.Server
			requests := make([]string, 0, 3)
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/.well-known/takoform":
					writeProviderDiscovery(t, w, server.URL)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/resources/"+kind+"/fixture"):
					assertProviderExactQuery(t, r, form)
					w.Header().Set("ETag", `"7"`)
					_ = json.NewEncoder(w).Encode(providerObservedResource(kind, form, ""))
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/resources/"+kind+"/fixture/observe"):
					assertProviderExactQuery(t, r, form)
					if r.Header.Get("If-Match") != `"7"` {
						t.Errorf("If-Match = %q, want quoted generation 7", r.Header.Get("If-Match"))
					}
					w.Header().Set("ETag", `"7"`)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"resource":    providerObservedResource(kind, form, "7"),
						"observation": map[string]any{"status": "drifted", "summary": "native object drifted"},
					})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			formClient := client.New(server.URL, "", server.Client())
			if _, err := formClient.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			observed, err := observeResourceForRead(context.Background(), formClient, kind, "fixture", "prod", form)
			if err != nil {
				t.Fatal(err)
			}
			assertTypedDriftState(t, kind, observed, "drifted")
			if len(requests) != 3 || !strings.Contains(requests[1], "/fixture") || !strings.HasSuffix(requests[2], "/fixture/observe") {
				t.Fatalf("request sequence = %#v, want discovery, exact GET, observe", requests)
			}
		})
	}
}

func TestVersionedTypedReadsStopOnExactGet404(t *testing.T) {
	for _, kind := range typedResourceKinds() {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			var server *httptest.Server
			requests := 0
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path == "/.well-known/takoform" {
					writeProviderDiscovery(t, w, server.URL)
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			formClient := client.New(server.URL, "", server.Client())
			if _, err := formClient.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, err := observeResourceForRead(context.Background(), formClient, kind, "fixture", "prod", providerReleaseForms()[kind])
			if !errors.Is(err, client.ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
			if requests != 2 {
				t.Fatalf("requests = %d, want discovery plus exact GET only", requests)
			}
		})
	}
}

func TestCompatibilityTypedReadsUseObserveOnly(t *testing.T) {
	for _, kind := range typedResourceKinds() {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/resources/"+kind+"/fixture/observe") {
					t.Errorf("compatibility read = %s %s, want observe only", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(client.Resource{
					APIVersion: client.APIVersion, Kind: kind,
					Metadata: client.Metadata{Name: "fixture", Space: "prod"},
					Status:   &client.Status{Conditions: []client.Condition{{Type: "Drifted", Status: "False"}}},
				})
			}))
			defer server.Close()
			observed, err := observeResourceForRead(context.Background(), client.NewCompatibilityFallback(server.URL, "", server.Client()), kind, "fixture", "prod", providerReleaseForms()[kind])
			if err != nil {
				t.Fatal(err)
			}
			assertTypedDriftState(t, kind, observed, "current")
			if requests != 1 {
				t.Fatalf("requests = %d, want one observe", requests)
			}
		})
	}
}

func typedResourceKinds() []string {
	return []string{
		client.KindEdgeWorker, client.KindObjectBucket, client.KindKVStore, client.KindQueue,
		client.KindSQLDatabase, client.KindContainerService, client.KindVectorIndex,
		client.KindDurableWorkflow, client.KindStatefulActorNamespace, client.KindSchedule,
	}
}

func providerObservedResource(kind string, form client.InstalledFormReference, version string) client.Resource {
	return client.Resource{
		APIVersion: client.APIVersion, Kind: kind, Form: &form,
		Metadata: client.Metadata{Name: "fixture", Space: "prod", ResourceVersion: version},
		Spec:     map[string]any{"name": "fixture"},
		Status:   &client.Status{Portability: "portable", Outputs: map[string]any{"reference": "fixture-output"}},
	}
}

func assertTypedDriftState(t *testing.T, kind string, observed *client.Resource, want string) {
	t.Helper()
	if kind == client.KindEdgeWorker {
		model := edgeWorkerModel{}
		if diags := applyEdgeWorkerStatus(context.Background(), observed, "prod", &model); diags.HasError() {
			t.Fatalf("status diagnostics: %v", diags)
		}
		if model.DriftStatus.ValueString() != want {
			t.Fatalf("drift_status = %q, want %q", model.DriftStatus.ValueString(), want)
		}
		return
	}
	model := serviceShapeModel{}
	if diags := applyServiceShapeStatus(context.Background(), observed, kind, "prod", &model); diags.HasError() {
		t.Fatalf("status diagnostics: %v", diags)
	}
	if model.DriftStatus.ValueString() != want {
		t.Fatalf("drift_status = %q, want %q", model.DriftStatus.ValueString(), want)
	}
}

func writeProviderDiscovery(t *testing.T, w http.ResponseWriter, origin string) {
	t.Helper()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"api_versions": []string{client.APIVersion},
		"features": map[string]bool{
			"service_forms": true, "exact_form_ref": true,
			"optimistic_concurrency": true, "idempotent_lifecycle": true,
		},
		"endpoints": map[string]string{
			"api": origin + "/apis/forms.takoform.com/v1alpha1",
		},
	})
}

func assertProviderExactQuery(t *testing.T, r *http.Request, form client.InstalledFormReference) {
	t.Helper()
	want := map[string]string{
		"space": "prod", "apiVersion": form.FormRef.APIVersion, "kind": form.FormRef.Kind,
		"definitionVersion": form.FormRef.DefinitionVersion, "schemaDigest": form.FormRef.SchemaDigest,
		"packageDigest": form.PackageDigest,
	}
	for key, value := range want {
		if r.URL.Query().Get(key) != value {
			t.Errorf("query %s = %q, want %q", key, r.URL.Query().Get(key), value)
		}
	}
}
