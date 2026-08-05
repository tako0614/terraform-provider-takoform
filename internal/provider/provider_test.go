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
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
)

func discoveryHandler(t *testing.T, serviceForms bool) http.HandlerFunc {
	t.Helper()
	return versionedDiscoveryHandler(t, client.APIVersion, serviceForms)
}

// versionedDiscoveryHandler serves the only discovery document a conforming
// host may serve: a versioned same-origin API base plus the required feature
// set. There is no unversioned capability document to fall back to.
func versionedDiscoveryHandler(t *testing.T, discoveryVersion string, serviceForms bool) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/takoform/v1alpha2" {
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
				"api":   origin + "/apis/forms.takoform.com/v1alpha2",
				"forms": origin + "/apis/forms.takoform.com/v1alpha2/forms",
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
		if schemaResponse.Schema.Version != 1 {
			t.Errorf("%s schema version = %d, want fail-closed provider-v1 state version 1", metadata.TypeName, schemaResponse.Schema.Version)
		}
		stateGuard, ok := candidate.(frameworkresource.ResourceWithUpgradeState)
		if !ok {
			t.Errorf("%s omits the diagnostic-only version-zero state guard", metadata.TypeName)
		} else if upgraders := stateGuard.UpgradeState(context.Background()); len(upgraders) != 1 {
			t.Errorf("%s version-zero state guard count = %d, want 1", metadata.TypeName, len(upgraders))
		} else if _, ok := upgraders[0]; !ok {
			t.Errorf("%s omits the version-zero state guard", metadata.TypeName)
		}
		for _, name := range []string{
			"form_api_version",
			"form_kind",
			"form_definition_version",
			"form_schema_digest",
			"form_package_digest",
		} {
			attribute, ok := schemaResponse.Schema.Attributes[name]
			if !ok {
				t.Errorf("%s omits exact state identity attribute %s", metadata.TypeName, name)
				continue
			}
			if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
				t.Errorf("%s attribute %s must be computed-only", metadata.TypeName, name)
			}
		}
	}
}

func TestProtocolRejectsVersionZeroFormStateWithoutRunningAnUpgrader(t *testing.T) {
	server := providerserver.NewProtocol6(New("1.0.0")())()
	response, err := server.UpgradeResourceState(context.Background(), &tfprotov6.UpgradeResourceStateRequest{
		TypeName: "takoform_object_bucket",
		Version:  0,
		RawState: &tfprotov6.RawState{JSON: []byte(`{
			"name":"legacy",
			"space":"prod",
			"storage_class":"standard"
		}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.UpgradedState != nil {
		t.Fatal("provider fabricated upgraded v0.2.1 state")
	}
	for _, diagnostic := range response.Diagnostics {
		if diagnostic.Summary == "Provider v2 requires explicit Form migration" &&
			strings.Contains(diagnostic.Detail, "State was not modified and no Resource lifecycle request was made") &&
			strings.Contains(diagnostic.Detail, "Pin the exact historical provider") &&
			strings.Contains(diagnostic.Detail, "create/import") {
			return
		}
	}
	t.Fatalf("protocol did not reject schema-version-0 state: %#v", response.Diagnostics)
}

func TestConfigureClientUsesOnlyTheAdvertisedVersionedEndpoint(t *testing.T) {
	var server *httptest.Server
	unversionedRequests := 0
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/takoform/v1alpha2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_versions": []string{client.APIVersion},
				"features":     map[string]bool{"service_forms": true, "exact_form_ref": true, "optimistic_concurrency": true, "idempotent_lifecycle": true},
				"endpoints": map[string]string{
					"api":   server.URL + "/apis/forms.takoform.com/v1alpha2",
					"forms": server.URL + "/apis/forms.takoform.com/v1alpha2/forms",
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
		if r.URL.Path != "/.well-known/takoform/v1alpha2" {
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

func TestProviderConfigureRejectsUnknownAuthorityConfigurationBeforeEnvironmentFallback(t *testing.T) {
	t.Setenv(envToken, "ambient-token")
	t.Setenv(envSpace, "ambient-space")

	tests := []struct {
		name        string
		token       any
		space       any
		wantSummary string
	}{
		{
			name:        "token",
			token:       tftypes.UnknownValue,
			space:       "configured-space",
			wantSummary: "Unknown Takoform token",
		},
		{
			name:        "space",
			token:       "configured-token",
			space:       tftypes.UnknownValue,
			wantSummary: "Unknown Takoform space",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			handler := discoveryHandler(t, true)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				handler(w, r)
			}))
			defer server.Close()

			candidate := &takoformProvider{}
			var schemaResponse frameworkprovider.SchemaResponse
			candidate.Schema(
				context.Background(),
				frameworkprovider.SchemaRequest{},
				&schemaResponse,
			)
			configType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
				"endpoint": tftypes.String,
				"space":    tftypes.String,
				"token":    tftypes.String,
			}}
			request := frameworkprovider.ConfigureRequest{
				Config: tfsdk.Config{
					Schema: schemaResponse.Schema,
					Raw: tftypes.NewValue(configType, map[string]tftypes.Value{
						"endpoint": tftypes.NewValue(tftypes.String, server.URL),
						"space":    tftypes.NewValue(tftypes.String, test.space),
						"token":    tftypes.NewValue(tftypes.String, test.token),
					}),
				},
			}
			var response frameworkprovider.ConfigureResponse
			candidate.Configure(context.Background(), request, &response)

			found := false
			for _, diagnostic := range response.Diagnostics {
				if diagnostic.Summary() == test.wantSummary {
					found = true
				}
			}
			if !found {
				t.Errorf("diagnostics = %#v, want %q", response.Diagnostics, test.wantSummary)
			}
			if requests != 0 {
				t.Errorf("unknown provider %s made %d discovery requests", test.name, requests)
			}
			if response.ResourceData != nil || response.DataSourceData != nil {
				t.Error("unknown provider authority configuration produced configured provider data")
			}
		})
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
	names := make([]string, 0, len(currentformcatalog.Kinds))
	for _, kind := range currentformcatalog.Kinds {
		names = append(names, kind.ResourceType)
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
	if !strings.Contains(err.Error(), "must advertise only API version") {
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
