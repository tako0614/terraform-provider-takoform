package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

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
	if len(dataSourceNames) != 0 {
		t.Fatalf("data sources = %v, want none; the takoform_interface data source was withdrawn with the v1alpha2 lane", dataSourceNames)
	}
}

func TestProviderStateExcludesBackendCredentialAndPriceAuthority(t *testing.T) {
	for _, factory := range (&takoformProvider{}).Resources(context.Background()) {
		candidate := factory()
		var metadata frameworkresource.MetadataResponse
		candidate.Metadata(context.Background(), frameworkresource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		var schemaResponse frameworkresource.SchemaResponse
		candidate.Schema(context.Background(), frameworkresource.SchemaRequest{}, &schemaResponse)
		// `target` is a legitimate portable relationship field for Forms such as
		// Schedule and TopicSubscription. Host placement/implementation
		// authority is represented by the terms below, never by that generic
		// relationship spelling.
		for _, forbidden := range []string{"selected_implementation", "locked", "credential", "secret", "price", "quote", "billing", "backend"} {
			if _, ok := schemaResponse.Schema.Attributes[forbidden]; ok {
				t.Errorf("%s exposes forbidden provider-state attribute %s", metadata.TypeName, forbidden)
			}
		}
		// The Host API v1beta1 lane splits the single resource_version into
		// the generation/revision fence pair; every resource carries the full
		// identity triple.
		for _, fence := range []string{"uid", "generation", "revision"} {
			if _, ok := schemaResponse.Schema.Attributes[fence]; !ok {
				t.Errorf("%s omits the v1beta1 %s identity attribute", metadata.TypeName, fence)
			}
		}
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
			// Configuration authority is decided before any lane negotiates, so
			// the endpoint only has to exist; a 404 leaves the lane error
			// recorded rather than fatal.
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				http.NotFound(w, r)
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
				"endpoint":            tftypes.String,
				"space":               tftypes.String,
				"token":               tftypes.String,
				"runtime_input_nonce": tftypes.String,
				"runtime_inputs":      tftypes.Map{ElementType: tftypes.String},
			}}
			request := frameworkprovider.ConfigureRequest{
				Config: tfsdk.Config{
					Schema: schemaResponse.Schema,
					Raw: tftypes.NewValue(configType, map[string]tftypes.Value{
						"endpoint":            tftypes.NewValue(tftypes.String, server.URL),
						"space":               tftypes.NewValue(tftypes.String, test.space),
						"token":               tftypes.NewValue(tftypes.String, test.token),
						"runtime_input_nonce": tftypes.NewValue(tftypes.String, nil),
						"runtime_inputs": tftypes.NewValue(
							tftypes.Map{ElementType: tftypes.String},
							nil,
						),
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
			if got := requests.Load(); got != 0 {
				t.Errorf("unknown provider %s made %d discovery requests", test.name, got)
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

func TestResourceAPIHTTPClientBoundsStalledHostResponseHeaders(t *testing.T) {
	client := newResourceAPIHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Resource API transport = %T, want *http.Transport", client.Transport)
	}
	if transport.ResponseHeaderTimeout <= 0 || transport.ResponseHeaderTimeout > time.Minute {
		t.Fatalf(
			"Resource API response header timeout must fail a stalled host within one minute, got %s",
			transport.ResponseHeaderTimeout,
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

	_, err := configureClientV3(context.Background(), redirector.URL, "must-not-forward", newResourceAPIHTTPClient())
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
	// Every registered resource type has exactly one example directory, with no
	// exceptions: every provider resource is derived from a Form, so every one
	// has an example rendered from that Form (spec/decisions/0021).
	want := append([]string(nil), currentProviderResourceTypeNames()...)
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
	assembly := mustPublisherProviderSnapshotAssembly()
	forms := currentPublisherProviderForms()
	names := make([]string, 0, len(forms))
	for _, form := range forms {
		ref, err := assembly.registry.DefaultCreate(v3GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind})
		if err != nil {
			panic(err)
		}
		resourceType, ok := assembly.resourceTypes.Lookup(ref.ExactKey())
		if !ok {
			panic("publisher-selected provider resource type mapping missing for " + ref.ExactKey().String())
		}
		names = append(names, resourceType)
	}
	sort.Strings(names)
	return names
}

// v3ProviderResourceTypeNames is retained Provider 3 aggregate history. The
// current tako0614/takoform surface is currentProviderResourceTypeNames above.
func v3ProviderResourceTypeNames() []string {
	forms := providerV3CurrentForms()
	names := make([]string, 0, len(forms))
	for _, form := range forms {
		ref, err := mustProviderV3SnapshotAssembly().registry.DefaultCreate(v3GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind})
		if err != nil {
			panic(err)
		}
		resourceType, ok := v3TerraformResourceTypes().Lookup(ref.ExactKey())
		if !ok {
			panic("provider resource type mapping missing for " + ref.ExactKey().String())
		}
		names = append(names, resourceType)
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
