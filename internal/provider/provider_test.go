package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
)

func discoveryHandler(t *testing.T, serviceForms bool) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		switch r.URL.Path {
		case "/.well-known/takoform":
			body = map[string]any{
				"api_versions": []string{"forms.takoform.com/v1alpha1"},
				"features": map[string]bool{
					"service_forms": serviceForms,
				},
				"endpoints": map[string]string{},
			}
		case "/v1/capabilities":
			body = map[string]any{
				"apiVersion": "forms.takoform.com/v1alpha1",
				"resources": map[string]bool{
					"EdgeWorker":             serviceForms,
					"ObjectBucket":           serviceForms,
					"KVStore":                serviceForms,
					"Queue":                  serviceForms,
					"SQLDatabase":            serviceForms,
					"ContainerService":       serviceForms,
					"VectorIndex":            serviceForms,
					"DurableWorkflow":        serviceForms,
					"StatefulActorNamespace": serviceForms,
					"Schedule":               serviceForms,
				},
			}
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		raw, _ := json.Marshal(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}
}

func versionedDiscoveryHandler(t *testing.T, discoveryVersion string, capabilityVersion string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		switch r.URL.Path {
		case "/.well-known/takoform":
			body = map[string]any{
				"api_versions": []string{discoveryVersion},
				"features": map[string]bool{
					"service_forms": true,
				},
				"endpoints": map[string]string{},
			}
		case "/v1/capabilities":
			body = map[string]any{
				"apiVersion": capabilityVersion,
				"resources": map[string]bool{
					"EdgeWorker":       false,
					"ContainerService": false,
				},
			}
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		raw, _ := json.Marshal(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}
}

func TestProviderResourcesIncludeCurrentServiceForms(t *testing.T) {
	got := providerResourceTypeNames(t)
	want := currentProviderResourceTypeNames()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected provider resource set:\ngot  %v\nwant %v", got, want)
	}
}

func TestProviderDoesNotExposePushNotificationResources(t *testing.T) {
	for _, name := range providerResourceTypeNames(t) {
		normalized := strings.ToLower(name)
		if strings.Contains(normalized, "push") || strings.Contains(normalized, "notification") {
			t.Fatalf("push notification delivery is product-local, not a Takoform provider resource: %s", name)
		}
	}
}

func TestProviderSplitDoesNotExposeTakosumiAdminResources(t *testing.T) {
	forbidden := []string{
		"target",
		"target_pool",
		"provider_connection",
		"credential",
		"provider_binding",
		"policy",
		"adapter",
		"billing",
		"quota",
		"account",
	}
	for _, name := range providerResourceTypeNames(t) {
		normalized := strings.ToLower(name)
		for _, term := range forbidden {
			if strings.Contains(normalized, term) {
				t.Fatalf("Takosumi host administration is outside the typed Takoform provider: %s contains %q", name, term)
			}
		}
	}
	p := &takoformProvider{}
	if dataSources := p.DataSources(context.Background()); len(dataSources) != 0 {
		t.Fatalf("typed Takoform provider must not expose host-admin data sources: %d", len(dataSources))
	}
}

func TestProviderStateExcludesBackendCredentialAndPriceAuthority(t *testing.T) {
	for _, factory := range (&takoformProvider{}).Resources(context.Background()) {
		candidate := factory()
		var metadata frameworkresource.MetadataResponse
		candidate.Metadata(context.Background(), frameworkresource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		var schemaResponse frameworkresource.SchemaResponse
		candidate.Schema(context.Background(), frameworkresource.SchemaRequest{}, &schemaResponse)
		for _, forbidden := range []string{"selected_implementation", "target", "locked", "credential", "secret", "price", "quote", "billing", "backend"} {
			if _, ok := schemaResponse.Schema.Attributes[forbidden]; ok {
				t.Errorf("%s exposes forbidden provider-state attribute %s", metadata.TypeName, forbidden)
			}
		}
		if _, ok := schemaResponse.Schema.Attributes["resource_version"]; !ok {
			t.Errorf("%s omits the optimistic-concurrency fence", metadata.TypeName)
		}
	}
}

func TestConfigureClientUsesAdvertisedVersionedEndpointWithoutV1Capabilities(t *testing.T) {
	var server *httptest.Server
	legacyRequests := 0
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/takoform":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_versions": []string{"forms.takoform.com/v1alpha1"},
				"features":     map[string]bool{"service_forms": true, "exact_form_ref": true, "optimistic_concurrency": true, "idempotent_lifecycle": true},
				"endpoints": map[string]string{
					"api":               server.URL + "/apis/forms.takoform.com/v1alpha1",
					"forms":             server.URL + "/apis/forms.takoform.com/v1alpha1/forms",
					"compatibility_api": server.URL + "/v1",
				},
			})
		case "/v1/capabilities":
			legacyRequests++
			http.Error(w, "legacy endpoint must not be called", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configured, err := configureClient(context.Background(), server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if configured.UsesCompatibilityFallback() || legacyRequests != 0 {
		t.Fatalf("unexpected compatibility fallback=%v legacyRequests=%d", configured.UsesCompatibilityFallback(), legacyRequests)
	}
}

func TestResourceAPIHTTPClientWaitsForServerSideOpenTofuRuns(t *testing.T) {
	client := newResourceAPIHTTPClient()
	if client.Timeout < 11*time.Minute {
		t.Fatalf(
			"Resource API timeout must cover server-side OpenTofu apply waits, got %s",
			client.Timeout,
		)
	}
}

func TestResourceAPIHTTPClientDoesNotForwardBearerThroughRedirect(t *testing.T) {
	redirectTargetRequests := 0
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		redirectTargetRequests++
	}))
	defer redirectTarget.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", redirectTarget.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	_, err := configureClient(context.Background(), redirector.URL, "must-not-forward", newResourceAPIHTTPClient())
	if err == nil {
		t.Fatal("redirected discovery unexpectedly configured the provider")
	}
	if redirectTargetRequests != 0 {
		t.Fatalf("redirect target received %d requests", redirectTargetRequests)
	}
}

func TestProviderExampleResourcesMatchCurrentResources(t *testing.T) {
	entries, err := os.ReadDir(filepath.Clean("../../examples/resources"))
	if err != nil {
		t.Fatalf("read examples/resources: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			got = append(got, entry.Name())
		}
	}
	sort.Strings(got)
	want := currentProviderResourceTypeNames()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("example resource directories must match provider resources:\ngot  %v\nwant %v", got, want)
	}
}

func TestPublishedHCLUsesFullyQualifiedProviderAddress(t *testing.T) {
	t.Helper()
	const (
		fullAddress  = `source = "registry.terraform.io/tako0614/takoform"`
		shortAddress = `source = "tako0614/takoform"`
	)
	paths := []string{
		filepath.Clean("../../README.md"),
		filepath.Clean("../../docs/index.md"),
	}
	entries, err := os.ReadDir(filepath.Clean("../../examples/resources"))
	if err != nil {
		t.Fatalf("read examples/resources: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			paths = append(paths, filepath.Join("../../examples/resources", entry.Name(), "resource.tf"))
		}
	}

	for _, filename := range paths {
		raw, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		contents := string(raw)
		if strings.Contains(contents, shortAddress) {
			t.Errorf("%s uses the two-segment provider shorthand, which OpenTofu resolves under the wrong registry", filename)
		}
		if !strings.Contains(contents, fullAddress) {
			t.Errorf("%s must use the exact provider address %q", filename, fullAddress)
		}
	}
}

func currentProviderResourceTypeNames() []string {
	names := []string{
		"takoform_edge_worker",
		"takoform_object_bucket",
		"takoform_kv_store",
		"takoform_queue",
		"takoform_sql_database",
		"takoform_container_service",
		"takoform_vector_index",
		"takoform_durable_workflow",
		"takoform_stateful_actor_namespace",
		"takoform_schedule",
	}
	sort.Strings(names)
	return names
}

func providerResourceTypeNames(t *testing.T) []string {
	t.Helper()
	p := &takoformProvider{}
	got := make([]string, 0, len(p.Resources(context.Background())))
	for _, factory := range p.Resources(context.Background()) {
		res := factory()
		var resp frameworkresource.MetadataResponse
		res.Metadata(context.Background(), frameworkresource.MetadataRequest{
			ProviderTypeName: "takoform",
		}, &resp)
		got = append(got, resp.TypeName)
	}
	sort.Strings(got)
	return got
}

func TestConfigureClient_AcceptsContainerServiceOnlyCapabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		switch r.URL.Path {
		case "/.well-known/takoform":
			body = map[string]any{
				"api_versions": []string{"forms.takoform.com/v1alpha1"},
				"features": map[string]bool{
					"service_forms": true,
				},
				"endpoints": map[string]string{},
			}
		case "/v1/capabilities":
			body = map[string]any{
				"apiVersion": "forms.takoform.com/v1alpha1",
				"resources": map[string]bool{
					"EdgeWorker":       false,
					"ContainerService": true,
				},
			}
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		raw, _ := json.Marshal(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	c, err := configureClient(context.Background(), srv.URL, "tok", srv.Client(), true)
	if err != nil {
		t.Fatalf("configureClient: %v", err)
	}
	if !c.Capabilities.SupportsResource("ContainerService") {
		t.Fatalf("expected ContainerService capability cached")
	}
}

func TestConfigureClient_AcceptsServiceFormAPIWithNoEnabledForms(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		switch r.URL.Path {
		case "/.well-known/takoform":
			body = map[string]any{
				"api_versions": []string{"forms.takoform.com/v1alpha1"},
				"features": map[string]bool{
					"service_forms": true,
				},
				"endpoints": map[string]string{},
			}
		case "/v1/capabilities":
			body = map[string]any{
				"apiVersion": "forms.takoform.com/v1alpha1",
				"resources":  map[string]bool{},
			}
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		raw, _ := json.Marshal(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	c, err := configureClient(context.Background(), srv.URL, "tok", srv.Client(), true)
	if err != nil {
		t.Fatalf("configureClient: %v", err)
	}
	if c == nil {
		t.Fatalf("expected a client")
	}
}

func TestConfigureClient_AcceptsServiceForms(t *testing.T) {
	srv := httptest.NewServer(discoveryHandler(t, true))
	defer srv.Close()

	c, err := configureClient(context.Background(), srv.URL, "tok", srv.Client(), true)
	if err != nil {
		t.Fatalf("configureClient: %v", err)
	}
	if c == nil {
		t.Fatalf("expected a client")
	}
}

func TestConfigureClient_RejectsWhenServiceFormsFalse(t *testing.T) {
	srv := httptest.NewServer(discoveryHandler(t, false))
	defer srv.Close()

	_, err := configureClient(context.Background(), srv.URL, "", srv.Client(), true)
	if err == nil {
		t.Fatalf("expected configuration to fail when service_forms is false")
	}
	if !strings.Contains(err.Error(), "features.service_forms") {
		t.Fatalf("expected a clear Service Form API diagnostic, got: %v", err)
	}
}

func TestConfigureClient_RejectsUnsupportedDiscoveryVersion(t *testing.T) {
	srv := httptest.NewServer(versionedDiscoveryHandler(t, "forms.takoform.com/v0", "forms.takoform.com/v1alpha1"))
	defer srv.Close()

	_, err := configureClient(context.Background(), srv.URL, "", srv.Client(), true)
	if err == nil {
		t.Fatalf("expected configuration to fail on unsupported discovery api version")
	}
	if !strings.Contains(err.Error(), "does not advertise API version") {
		t.Fatalf("expected api version diagnostic, got: %v", err)
	}
}

func TestConfigureClient_RejectsUnsupportedCapabilitiesVersion(t *testing.T) {
	srv := httptest.NewServer(versionedDiscoveryHandler(t, "forms.takoform.com/v1alpha1", "forms.takoform.com/v0"))
	defer srv.Close()

	_, err := configureClient(context.Background(), srv.URL, "", srv.Client(), true)
	if err == nil {
		t.Fatalf("expected configuration to fail on unsupported capabilities api version")
	}
	if !strings.Contains(err.Error(), "unsupported capabilities apiVersion") {
		t.Fatalf("expected capabilities apiVersion diagnostic, got: %v", err)
	}
}

func TestConfigureClient_DiscoveryError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"code":"boom","message":"down"}}`)
	}))
	defer srv.Close()

	_, err := configureClient(context.Background(), srv.URL, "", srv.Client())
	if err == nil {
		t.Fatalf("expected discovery error")
	}
	if !strings.Contains(err.Error(), "discovering Takoform endpoint") {
		t.Fatalf("expected discovery-wrapped error, got: %v", err)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "x"); got != "x" {
		t.Fatalf("expected x, got %q", got)
	}
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Fatalf("expected a, got %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestParseExplicitBool(t *testing.T) {
	for _, value := range []string{"", "0", "false", "FALSE", "no"} {
		got, err := parseExplicitBool(value)
		if err != nil || got {
			t.Fatalf("parseExplicitBool(%q) = %v, %v", value, got, err)
		}
	}
	for _, value := range []string{"1", "true", "TRUE", "yes"} {
		got, err := parseExplicitBool(value)
		if err != nil || !got {
			t.Fatalf("parseExplicitBool(%q) = %v, %v", value, got, err)
		}
	}
	if _, err := parseExplicitBool("fallback-please"); err == nil {
		t.Fatal("expected invalid compatibility fallback value to fail closed")
	}
}
