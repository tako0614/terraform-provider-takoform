package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

func discoveryHandler(t *testing.T, serviceForms bool) http.HandlerFunc {
	t.Helper()
	return versionedDiscoveryHandler(t, "forms.takoform.com/v1alpha1", serviceForms)
}

// versionedDiscoveryHandler serves the only discovery document a conforming
// host may serve: a versioned same-origin API base plus the required feature
// set. There is no unversioned capability document to fall back to.
func versionedDiscoveryHandler(t *testing.T, discoveryVersion string, serviceForms bool) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/takoform" {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		origin := "http://" + r.Host
		body := map[string]any{
			"api_versions": []string{discoveryVersion},
			"features": map[string]bool{
				"service_forms":          serviceForms,
				"exact_form_ref":         true,
				"optimistic_concurrency": true,
				"idempotent_lifecycle":   true,
			},
			"endpoints": map[string]string{
				"api":   origin + "/apis/forms.takoform.com/v1alpha1",
				"forms": origin + "/apis/forms.takoform.com/v1alpha1/forms",
			},
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

// TestProviderExposesNoHostAuthorityResources keeps the provider on the
// portable side of the boundary. A Form describes what a caller wants; who
// runs it, on whose capacity, with whose credentials, and at what price stay
// with the host, so no resource may name those concerns.
func TestProviderExposesNoHostAuthorityResources(t *testing.T) {
	forbidden := []string{
		"target",
		"target_pool",
		"provider_connection",
		"credential",
		"secret",
		"provider_binding",
		"operator_policy",
		"adapter",
		"billing",
		"invoice",
		"price",
		"quota",
		"account",
		"backend",
		"manager",
	}
	for _, name := range providerResourceTypeNames(t) {
		normalized := strings.ToLower(name)
		for _, term := range forbidden {
			if strings.Contains(normalized, term) {
				t.Fatalf("host authority is outside the typed Takoform provider: %s contains %q", name, term)
			}
		}
	}
	p := &takoformProvider{}
	var dataSourceNames []string
	for _, factory := range p.DataSources(context.Background()) {
		var metadata frameworkdatasource.MetadataResponse
		factory().Metadata(context.Background(), frameworkdatasource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		dataSourceNames = append(dataSourceNames, metadata.TypeName)
		for _, term := range forbidden {
			if strings.Contains(strings.ToLower(metadata.TypeName), term) {
				t.Fatalf("Takosumi host administration is outside the typed Takoform provider: %s contains %q", metadata.TypeName, term)
			}
		}
	}
	if !reflect.DeepEqual(dataSourceNames, []string{"takoform_interface"}) {
		t.Fatalf("data sources = %v, want only the read-only declaration surface", dataSourceNames)
	}
}

func TestInterfaceDataSourceIsReadOnlyAndVersionExact(t *testing.T) {
	var schemaResponse frameworkdatasource.SchemaResponse
	NewInterfaceDataSource().Schema(context.Background(), frameworkdatasource.SchemaRequest{}, &schemaResponse)
	for name, attribute := range schemaResponse.Schema.Attributes {
		switch name {
		case "name", "space":
			if !attribute.IsRequired() && !attribute.IsOptional() {
				t.Errorf("%s must be a selector", name)
			}
		case "version", "resource_kind", "resource_name":
			if !attribute.IsOptional() || !attribute.IsComputed() {
				t.Errorf("%s must be an optional exact selector and a computed identity result", name)
			}
		default:
			if !attribute.IsComputed() || attribute.IsRequired() || attribute.IsOptional() {
				t.Errorf("%s must be computed-only", name)
			}
		}
	}
	for _, forbidden := range []string{
		"binding", "permission", "grant", "token", "credential", "secret",
		"policy", "target", "price", "billing", "quota",
	} {
		if _, ok := schemaResponse.Schema.Attributes[forbidden]; ok {
			t.Errorf("declaration read exposes forbidden attribute %s", forbidden)
		}
	}
}

func TestInterfaceResourceIsGenericDataOnlyDeclaration(t *testing.T) {
	var schemaResponse frameworkresource.SchemaResponse
	NewInterfaceResource().Schema(context.Background(), frameworkresource.SchemaRequest{}, &schemaResponse)
	for _, required := range []string{
		"name", "version", "resource_kind", "resource_name", "document_json",
	} {
		if attribute := schemaResponse.Schema.Attributes[required]; attribute == nil || !attribute.IsRequired() {
			t.Errorf("%s must be required", required)
		}
	}
	for _, computed := range []string{"id", "values_json", "resource_uri", "resource_version"} {
		if attribute := schemaResponse.Schema.Attributes[computed]; attribute == nil || !attribute.IsComputed() {
			t.Errorf("%s must be computed", computed)
		}
	}
	for _, forbidden := range []string{
		"mcp", "http", "ui", "s3", "binding", "permission", "grant", "token",
		"credential", "secret", "target", "price", "billing", "quota",
	} {
		if _, exists := schemaResponse.Schema.Attributes[forbidden]; exists {
			t.Errorf("generic declaration exposes protocol or host authority field %s", forbidden)
		}
	}
}

func TestInterfaceResourceImportsItsPortableCompoundIdentity(t *testing.T) {
	ctx := context.Background()
	resource := NewInterfaceResource().(*interfaceResource)
	var schemaResponse frameworkresource.SchemaResponse
	resource.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	empty := tfsdk.State{
		Schema: schemaResponse.Schema,
		Raw: tftypes.NewValue(
			schemaResponse.Schema.Type().TerraformType(ctx),
			nil,
		),
	}
	id := `["prod","EdgeWorker","api","example.runtime","1"]`
	response := frameworkresource.ImportStateResponse{State: empty}
	resource.ImportState(
		ctx,
		frameworkresource.ImportStateRequest{ID: id},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("import: %v", response.Diagnostics)
	}
	for attribute, want := range map[string]string{
		"id":            id,
		"space":         "prod",
		"resource_kind": "EdgeWorker",
		"resource_name": "api",
		"name":          "example.runtime",
		"version":       "1",
	} {
		var got types.String
		response.State.GetAttribute(ctx, path.Root(attribute), &got)
		if got.ValueString() != want {
			t.Errorf("%s = %q, want %q", attribute, got.ValueString(), want)
		}
	}

	invalid := frameworkresource.ImportStateResponse{State: empty}
	resource.ImportState(
		ctx,
		frameworkresource.ImportStateRequest{ID: "prod/api"},
		&invalid,
	)
	if !invalid.Diagnostics.HasError() {
		t.Fatal("invalid compound identity was accepted")
	}
}

func TestInterfaceDataSourceRejectsUnknownIdentityAndScopeBeforeRead(t *testing.T) {
	base := interfaceDataSourceModel{
		Name: types.StringValue("mcp.server"), Space: types.StringNull(), Version: types.StringNull(),
		ResourceKind: types.StringNull(), ResourceName: types.StringNull(),
	}
	tests := []struct {
		name   string
		mutate func(*interfaceDataSourceModel)
	}{
		{name: "name", mutate: func(value *interfaceDataSourceModel) { value.Name = types.StringUnknown() }},
		{name: "space", mutate: func(value *interfaceDataSourceModel) { value.Space = types.StringUnknown() }},
		{name: "version", mutate: func(value *interfaceDataSourceModel) { value.Version = types.StringUnknown() }},
		{name: "resource_kind", mutate: func(value *interfaceDataSourceModel) { value.ResourceKind = types.StringUnknown() }},
		{name: "resource_name", mutate: func(value *interfaceDataSourceModel) { value.ResourceName = types.StringUnknown() }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			if got := unknownInterfaceReadField(value); got != test.name {
				t.Fatalf("unknown field = %q, want %q", got, test.name)
			}
		})
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

func TestConfigureClientUsesOnlyTheAdvertisedVersionedEndpoint(t *testing.T) {
	var server *httptest.Server
	unversionedRequests := 0
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/takoform":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_versions": []string{"forms.takoform.com/v1alpha1"},
				"features":     map[string]bool{"service_forms": true, "exact_form_ref": true, "optimistic_concurrency": true, "idempotent_lifecycle": true},
				"endpoints": map[string]string{
					"api":   server.URL + "/apis/forms.takoform.com/v1alpha1",
					"forms": server.URL + "/apis/forms.takoform.com/v1alpha1/forms",
				},
			})
		default:
			if strings.HasPrefix(r.URL.Path, "/v1/") {
				unversionedRequests++
			}
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	if _, err := configureClient(context.Background(), server.URL, "token", server.Client()); err != nil {
		t.Fatal(err)
	}
	if unversionedRequests != 0 {
		t.Fatalf("provider configuration touched %d unversioned endpoints", unversionedRequests)
	}
}

// TestConfigureClientRejectsHostsWithoutAVersionedEndpoint proves the provider
// fails closed instead of downgrading to an unversioned Resource API.
func TestConfigureClientRejectsHostsWithoutAVersionedEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/takoform" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"api_versions": []string{"forms.takoform.com/v1alpha1"},
			"features":     map[string]bool{"service_forms": true},
			"endpoints":    map[string]string{},
		})
	}))
	defer server.Close()
	if _, err := configureClient(context.Background(), server.URL, "token", server.Client()); err == nil {
		t.Fatal("expected a fail-closed configuration error")
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
		fullAddress  = `registry.terraform.io/tako0614/takoform`
		shortAddress = `source = "tako0614/takoform"`
	)
	fullAddressPattern := regexp.MustCompile(`source\s*=\s*"registry\.terraform\.io/tako0614/takoform"`)
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
		if !fullAddressPattern.MatchString(contents) {
			t.Errorf("%s must use the exact provider address %q", filename, fullAddress)
		}
	}
}

func currentProviderResourceTypeNames() []string {
	names := make([]string, 0, len(formcatalog.Kinds)+1)
	for _, kind := range formcatalog.Kinds {
		names = append(names, kind.ResourceType)
	}
	names = append(names, "takoform_interface")
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

func TestConfigureClient_AcceptsServiceForms(t *testing.T) {
	srv := httptest.NewServer(discoveryHandler(t, true))
	defer srv.Close()

	c, err := configureClient(context.Background(), srv.URL, "tok", srv.Client())
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

	_, err := configureClient(context.Background(), srv.URL, "", srv.Client())
	if err == nil {
		t.Fatalf("expected configuration to fail when service_forms is false")
	}
	if !strings.Contains(err.Error(), "features.service_forms") {
		t.Fatalf("expected a clear Service Form API diagnostic, got: %v", err)
	}
}

func TestConfigureClient_RejectsUnsupportedDiscoveryVersion(t *testing.T) {
	srv := httptest.NewServer(versionedDiscoveryHandler(t, "forms.takoform.com/v0", true))
	defer srv.Close()

	_, err := configureClient(context.Background(), srv.URL, "", srv.Client())
	if err == nil {
		t.Fatalf("expected configuration to fail on unsupported discovery api version")
	}
	if !strings.Contains(err.Error(), "does not advertise API version") {
		t.Fatalf("expected api version diagnostic, got: %v", err)
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
