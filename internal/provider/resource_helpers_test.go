package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
)

func TestVersionedTypedReadsGetFenceThenObserveAndMapDrift(t *testing.T) {
	for _, kind := range typedResourceKinds() {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			form := providerCandidateForms()[kind]
			var server *httptest.Server
			requests := make([]string, 0, 3)
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/.well-known/takoform/v1alpha2":
					writeProviderDiscovery(t, w, server.URL)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/resources/"+kind+"/fixture"):
					assertProviderExactQuery(t, r, form)
					w.Header().Set("ETag", `"7"`)
					_ = json.NewEncoder(w).Encode(providerObservedResource(kind, form, "7"))
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/resources/"+kind+"/fixture/observe"):
					assertProviderExactQuery(t, r, form)
					if r.Header.Get("If-Match") != `"7"` {
						t.Errorf("If-Match = %q, want quoted generation 7", r.Header.Get("If-Match"))
					}
					w.Header().Set("ETag", `"7"`)
					resource := providerObservedResource(kind, form, "7")
					resource.Status.Observed["driftedFields"] = []any{"/name"}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"resource": resource,
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
			observed, currentSpec, err := observeResourceForRead(context.Background(), formClient, kind, "fixture", "prod", form)
			if err != nil {
				t.Fatal(err)
			}
			if currentSpec["name"] != "fixture" {
				t.Fatalf("current desired spec = %#v, want fixture identity", currentSpec)
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
				if r.URL.Path == "/.well-known/takoform/v1alpha2" {
					writeProviderDiscovery(t, w, server.URL)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"code":"resource_not_found","message":"missing","requestId":"req-missing","retryable":false}}`))
			}))
			defer server.Close()
			formClient := client.New(server.URL, "", server.Client())
			if _, err := formClient.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, _, err := observeResourceForRead(context.Background(), formClient, kind, "fixture", "prod", providerCandidateForms()[kind])
			if !errors.Is(err, client.ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
			if requests != 2 {
				t.Fatalf("requests = %d, want discovery plus exact GET only", requests)
			}
		})
	}
}

func TestVersionedTypedReadsRejectObserveAtAnotherGeneration(t *testing.T) {
	for _, kind := range typedResourceKinds() {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			form := providerCandidateForms()[kind]
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/.well-known/takoform/v1alpha2":
					writeProviderDiscovery(t, w, server.URL)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/resources/"+kind+"/fixture"):
					w.Header().Set("ETag", `"7"`)
					_ = json.NewEncoder(w).Encode(providerObservedResource(kind, form, "7"))
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/resources/"+kind+"/fixture/observe"):
					if got := r.Header.Get("If-Match"); got != `"7"` {
						t.Errorf("If-Match = %q, want quoted generation 7", got)
					}
					w.Header().Set("ETag", `"8"`)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"resource": providerObservedResource(kind, form, "8"),
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
			_, _, err := observeResourceForRead(context.Background(), formClient, kind, "fixture", "prod", form)
			if err == nil || !strings.Contains(err.Error(), "generation protected by If-Match") {
				t.Fatalf("error = %v, want generation-fence rejection", err)
			}
		})
	}
}

// typedResourceKinds exercises the read path against every declared Form, so
// a new catalogue entry is covered the moment it exists.
func typedResourceKinds() []string {
	kinds := make([]string, 0, len(currentformcatalog.Kinds))
	for _, kind := range currentformcatalog.Kinds {
		kinds = append(kinds, kind.Kind)
	}
	return kinds
}

func providerObservedResource(kind string, form client.InstalledFormReference, version string) client.Resource {
	generation, err := strconv.ParseInt(version, 10, 64)
	if err != nil {
		panic("provider test has invalid resourceVersion " + version)
	}
	status := providerPortableStatus(kind, "fixture", generation)
	declared, ok := currentformcatalog.ByKind(kind)
	if !ok {
		panic("provider test has no Form declaration for " + kind)
	}
	spec := declared.CanonicalDesired()
	spec["name"] = "fixture"
	return client.Resource{
		APIVersion: client.APIVersion, Kind: kind, Form: &form,
		Metadata: client.Metadata{Name: "fixture", Space: "prod", ResourceVersion: version},
		Spec:     spec,
		Status:   status,
	}
}

func providerPortableStatus(kindName, name string, generation int64) *client.Status {
	kind, ok := currentformcatalog.ByKind(kindName)
	if !ok {
		panic("provider test has no Form declaration for " + kindName)
	}
	observed := kind.CanonicalObserved()
	output := kind.CanonicalOutput()
	observed["id"] = kindName + "/" + name
	output["id"] = kindName + "/" + name
	output["name"] = name
	observed["generation"] = generation
	output["generation"] = generation
	return &client.Status{Observed: observed, Output: output}
}

func assertTypedDriftState(t *testing.T, kind string, observed *client.Resource, want string) {
	t.Helper()
	declared, ok := currentformcatalog.ByKind(kind)
	if !ok {
		t.Fatalf("no declared Form for %s", kind)
	}
	resource := &formResource{kind: declared, data: &providerData{defaultSpace: "prod"}}
	var response frameworkresource.SchemaResponse
	resource.Schema(context.Background(), frameworkresource.SchemaRequest{}, &response)
	state := tfsdk.State{Schema: response.Schema, Raw: tftypes.NewValue(response.Schema.Type().TerraformType(context.Background()), nil)}
	values := formValues{Fields: map[string]attr.Value{}, Artifact: nullArtifactSourceValues()}
	if diags := resource.setState(
		context.Background(),
		&state,
		observed.Metadata.Name,
		observed.Spec,
		observed,
		"prod",
		values,
		true,
	); diags.HasError() {
		t.Fatalf("status diagnostics: %v", diags)
	}
	var drift types.String
	if diags := state.GetAttribute(context.Background(), path.Root("drift_status"), &drift); diags.HasError() {
		t.Fatalf("read drift_status: %v", diags)
	}
	if drift.ValueString() != want {
		t.Fatalf("drift_status = %q, want %q", drift.ValueString(), want)
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
			"api": origin + "/apis/forms.takoform.com/v1alpha2",
		},
	})
}

func ptrForm(form client.InstalledFormReference) *client.InstalledFormReference {
	return &form
}

// writeProviderFormAvailability answers the exact-availability probe every
// versioned mutation performs before it sends desired state.
func writeProviderFormAvailability(t *testing.T, w http.ResponseWriter, forms ...client.InstalledFormReference) {
	t.Helper()
	available := make([]client.FormAvailability, 0, len(forms))
	for _, form := range forms {
		available = append(available, client.FormAvailability{
			Identity: form, DefinitionKnown: true, Installed: true, Executable: true,
			Activated: true, AvailableToPrincipal: true,
			Operations: []string{"create", "read", "update", "delete", "import", "observe", "refresh"},
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"forms": available})
}

// mustDiscoveredProviderClient returns a client that has negotiated the
// versioned API base, which every provider resource requires.
func mustDiscoveredProviderClient(t *testing.T, server *httptest.Server) *client.Client {
	t.Helper()
	c := client.New(server.URL, "", server.Client())
	if _, err := c.Discover(context.Background()); err != nil {
		t.Fatalf("discover test host: %v", err)
	}
	return c
}

func assertProviderExactQuery(t *testing.T, r *http.Request, form client.InstalledFormReference) {
	t.Helper()
	// The packageDigest is not an exact-query selector: read/lifecycle
	// identity is the FormRef plus Space (see client.exactResourceQuery).
	want := map[string]string{
		"space": "prod", "apiVersion": form.FormRef.APIVersion, "kind": form.FormRef.Kind,
		"definitionVersion": form.FormRef.DefinitionVersion, "schemaDigest": form.FormRef.SchemaDigest,
	}
	for key, value := range want {
		if r.URL.Query().Get(key) != value {
			t.Errorf("query %s = %q, want %q", key, r.URL.Query().Get(key), value)
		}
	}
	if r.URL.Query().Get("packageDigest") != "" {
		t.Errorf("query packageDigest = %q, want absent from exact query", r.URL.Query().Get("packageDigest"))
	}
}
