package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

func interfaceHost(t *testing.T, advertise bool, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/takoform/v1alpha2" {
			features := map[string]bool{
				"service_forms": true, "exact_form_ref": true,
				"optimistic_concurrency": true, "idempotent_lifecycle": true,
			}
			if advertise {
				features[FeatureInterfaceDeclarations] = true
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_versions": []string{APIVersion},
				"features":     features,
				"endpoints": map[string]string{
					"api":   server.URL + "/apis/forms.takoform.com/v1alpha2",
					"forms": server.URL + "/apis/forms.takoform.com/v1alpha2/forms",
				},
			})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func discoveredClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	c := New(server.URL, "test-token", server.Client())
	if _, err := c.Discover(context.Background()); err != nil {
		t.Fatalf("discover: %v", err)
	}
	return c
}

func TestAbsentInterfaceFeatureIsNotAConfigurationError(t *testing.T) {
	server := interfaceHost(t, false, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %q", r.URL.Path)
		http.NotFound(w, r)
	})
	c := discoveredClient(t, server)
	if c.SupportsInterfaceDeclarations() {
		t.Fatal("surface must stay disabled without the feature flag")
	}
	if _, err := c.ListInterfaces(context.Background(), "prod"); !errors.Is(err, ErrInterfaceDeclarationsUnsupported) {
		t.Fatalf("err = %v, want ErrInterfaceDeclarationsUnsupported", err)
	}
	if _, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{Name: "mcp.server", Version: "1", ResourceKind: "ObjectBucket", ResourceName: "assets"}); !errors.Is(err, ErrInterfaceDeclarationsUnsupported) {
		t.Fatalf("err = %v, want ErrInterfaceDeclarationsUnsupported", err)
	}
}

func TestGetInterfaceUsesExactRuntimeIdentity(t *testing.T) {
	var gotSpace, gotVersion, gotResourceKind, gotResourceName, authorization string
	server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/forms.takoform.com/v1alpha2/interfaces/mcp.server" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotSpace = r.URL.Query().Get("space")
		gotVersion = r.URL.Query().Get("version")
		gotResourceKind = r.URL.Query().Get("resourceKind")
		gotResourceName = r.URL.Query().Get("resourceName")
		authorization = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "mcp.server", "version": "2025-11-25",
			"resource": map[string]any{"kind": "ObjectBucket", "name": "assets"},
			"document": map[string]any{"title": "Portable MCP"},
			"values":   map[string]any{"endpoint": "https://example.test/mcp"},
			"form": map[string]any{
				"formRef": map[string]any{
					"apiVersion": APIVersion, "kind": "ObjectBucket", "definitionVersion": "1.0.0",
					"schemaDigest": "sha256:" + strings.Repeat("a", 64),
				},
				"packageDigest": "sha256:" + strings.Repeat("b", 64),
			},
		})
	})
	c := discoveredClient(t, server)
	one, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{
		Name: "mcp.server", Version: "2025-11-25", ResourceKind: "ObjectBucket", ResourceName: "assets",
	})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if gotSpace != "prod" || gotVersion != "2025-11-25" || gotResourceKind != "ObjectBucket" || gotResourceName != "assets" {
		t.Fatalf("query space=%q version=%q resource=%s/%s", gotSpace, gotVersion, gotResourceKind, gotResourceName)
	}
	if one.Document["title"] != "Portable MCP" || one.Values["endpoint"] != "https://example.test/mcp" {
		t.Fatalf("declaration = %+v", one)
	}
	if one.Form == nil || one.Form.FormRef.Kind != "ObjectBucket" {
		t.Fatalf("form = %+v", one.Form)
	}
	if authorization != "Bearer test-token" {
		t.Fatalf("authorization = %q", authorization)
	}
}

func TestGetInterfaceWithoutVersionRequiresUniqueVisibleName(t *testing.T) {
	versions := []string{"2", "1"}
	server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis/forms.takoform.com/v1alpha2/interfaces":
			items := make([]map[string]any, 0, len(versions))
			for _, version := range versions {
				items = append(items, map[string]any{
					"name": "mcp.server", "version": version,
					"resource": map[string]any{"kind": "ObjectBucket", "name": "assets"}, "document": map[string]any{},
					"values": map[string]any{},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"interfaces": items})
		case "/apis/forms.takoform.com/v1alpha2/interfaces/mcp.server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "mcp.server", "version": r.URL.Query().Get("version"),
				"resource": map[string]any{"kind": r.URL.Query().Get("resourceKind"), "name": r.URL.Query().Get("resourceName")},
				"document": map[string]any{"title": "complete exact read"},
				"values":   map[string]any{},
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	})
	c := discoveredClient(t, server)
	if _, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{Name: "mcp.server"}); !errors.Is(err, ErrInterfaceIdentityAmbiguous) || !strings.Contains(err.Error(), "1, 2") {
		t.Fatalf("err = %v, want deterministic ambiguity", err)
	}

	versions = []string{"2025-11-25"}
	one, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{Name: "mcp.server"})
	if err != nil || one.Version != "2025-11-25" || one.Document["title"] != "complete exact read" {
		t.Fatalf("unique lookup = %+v err=%v", one, err)
	}
}

func TestInterfaceReadsRejectSubstitutionAndDuplicateIdentity(t *testing.T) {
	t.Run("exact response substitution", func(t *testing.T) {
		server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "mcp.server", "version": "other",
				"resource": map[string]any{"kind": "ObjectBucket", "name": "other"},
				"document": map[string]any{},
				"values":   map[string]any{},
			})
		})
		c := discoveredClient(t, server)
		if _, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{Name: "mcp.server", Version: "1", ResourceKind: "ObjectBucket", ResourceName: "assets"}); err == nil {
			t.Fatal("substituted version must fail closed")
		}
	})

	t.Run("missing exact document", func(t *testing.T) {
		server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "mcp.server", "version": "1",
				"resource": map[string]any{"kind": "ObjectBucket", "name": "assets"},
				"values":   map[string]any{},
			})
		})
		c := discoveredClient(t, server)
		_, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{
			Name: "mcp.server", Version: "1", ResourceKind: "ObjectBucket", ResourceName: "assets",
		})
		if err == nil || !strings.Contains(err.Error(), "exact declared document") {
			t.Fatalf("err = %v, want missing document rejection", err)
		}
	})

	t.Run("missing exact values", func(t *testing.T) {
		server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "mcp.server", "version": "1",
				"resource": map[string]any{"kind": "ObjectBucket", "name": "assets"},
				"document": map[string]any{},
			})
		})
		c := discoveredClient(t, server)
		_, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{
			Name: "mcp.server", Version: "1", ResourceKind: "ObjectBucket", ResourceName: "assets",
		})
		if err == nil || !strings.Contains(err.Error(), "exact resolved values") {
			t.Fatalf("err = %v, want missing values rejection", err)
		}
	})

	t.Run("secret-shaped public values", func(t *testing.T) {
		server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "mcp.server", "version": "1",
				"resource": map[string]any{"kind": "ObjectBucket", "name": "assets"},
				"document": map[string]any{},
				"values":   map[string]any{"api_key": "must-not-enter-state"},
			})
		})
		c := discoveredClient(t, server)
		_, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{
			Name: "mcp.server", Version: "1", ResourceKind: "ObjectBucket", ResourceName: "assets",
		})
		if err == nil || !strings.Contains(err.Error(), "forbidden interface values") {
			t.Fatalf("err = %v, want secret-shaped values rejection", err)
		}
	})

	t.Run("Form kind must match Resource kind", func(t *testing.T) {
		server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "mcp.server", "version": "1",
				"resource": map[string]any{"kind": "ObjectBucket", "name": "assets"},
				"document": map[string]any{},
				"values":   map[string]any{},
				"form": map[string]any{
					"formRef": map[string]any{
						"apiVersion": APIVersion, "kind": "EdgeWorker", "definitionVersion": "1.0.0",
						"schemaDigest": "sha256:" + strings.Repeat("a", 64),
					},
					"packageDigest": "sha256:" + strings.Repeat("b", 64),
				},
			})
		})
		c := discoveredClient(t, server)
		_, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{
			Name: "mcp.server", Version: "1", ResourceKind: "ObjectBucket", ResourceName: "assets",
		})
		if err == nil || !strings.Contains(err.Error(), "invalid Form identity") {
			t.Fatalf("err = %v, want mismatched Form kind rejection", err)
		}
	})

	t.Run("duplicate list identity", func(t *testing.T) {
		server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"interfaces": []map[string]any{
				{"name": "mcp.server", "version": "1", "resource": map[string]any{"kind": "ObjectBucket", "name": "assets"}, "document": map[string]any{}, "values": map[string]any{}},
				{"name": "mcp.server", "version": "1", "resource": map[string]any{"kind": "ObjectBucket", "name": "assets"}, "document": map[string]any{}, "values": map[string]any{}},
			}})
		})
		c := discoveredClient(t, server)
		if _, err := c.ListInterfaces(context.Background(), "prod"); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("err = %v, want duplicate rejection", err)
		}
	})
}

func TestDeclaredInterfaceIdentityMatchesWireSchemaPatterns(t *testing.T) {
	if got := resourceNamePattern.String(); got != formcatalog.PatternName {
		t.Fatalf("resource name pattern = %q, want canonical PatternName %q", got, formcatalog.PatternName)
	}
	valid := DeclaredInterface{
		Name: "mcp.server", Version: "2025-11-25",
		Resource: InterfaceResourceRef{Kind: "ObjectBucket", Name: "a" + strings.Repeat("0", 62)},
		Document: map[string]any{},
		Values:   map[string]any{},
	}
	if err := ValidateResourceName(valid.Resource.Name); err != nil {
		t.Fatalf("canonical Resource name rejected: %v", err)
	}
	if err := validateDeclaredInterfaceIdentity(valid); err != nil {
		t.Fatalf("valid declaration rejected: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*DeclaredInterface)
	}{
		{name: "uppercase interface name", mutate: func(value *DeclaredInterface) { value.Name = "Mcp.server" }},
		{name: "adjacent name separators", mutate: func(value *DeclaredInterface) { value.Name = "mcp..server" }},
		{name: "long interface name", mutate: func(value *DeclaredInterface) { value.Name = strings.Repeat("a", 129) }},
		{name: "invalid version", mutate: func(value *DeclaredInterface) { value.Version = "version/1" }},
		{name: "lowercase resource kind", mutate: func(value *DeclaredInterface) { value.Resource.Kind = "objectBucket" }},
		{name: "long resource kind", mutate: func(value *DeclaredInterface) { value.Resource.Kind = "A" + strings.Repeat("a", 64) }},
		{name: "empty resource name", mutate: func(value *DeclaredInterface) { value.Resource.Name = "" }},
		{name: "uppercase resource name", mutate: func(value *DeclaredInterface) { value.Resource.Name = "Assets" }},
		{name: "numeric-leading resource name", mutate: func(value *DeclaredInterface) { value.Resource.Name = "1assets" }},
		{name: "underscored resource name", mutate: func(value *DeclaredInterface) { value.Resource.Name = "asset_name" }},
		{name: "dotted resource name", mutate: func(value *DeclaredInterface) { value.Resource.Name = "asset.name" }},
		{name: "long resource name", mutate: func(value *DeclaredInterface) { value.Resource.Name = "a" + strings.Repeat("0", 63) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			test.mutate(&value)
			if err := validateDeclaredInterfaceIdentity(value); err == nil {
				t.Fatalf("invalid declaration accepted: %+v", value)
			}
		})
	}
}

func TestInterfaceResourceURIUsesCredentialFreeHTTPSGrammar(t *testing.T) {
	t.Parallel()

	base := DeclaredInterface{
		Name: "mcp.server", Version: "1",
		Resource: InterfaceResourceRef{Kind: "ObjectBucket", Name: "assets"},
		Document: map[string]any{},
		Values:   map[string]any{},
	}
	valid := []string{
		"https://runtime.example.invalid",
		"https://runtime.example.invalid:8443/oauth/resource",
		"https://xn--r8jz45g.xn--zckzah/oauth/resource",
		"https://runtime.example.invalid/%E8%9B%B8/runtime",
		"https://runtime.example.invalid/蛸/runtime",
	}
	invalid := []string{
		"",
		"http://runtime.example.invalid/oauth/resource",
		"https://user@runtime.example.invalid/oauth/resource",
		"https://runtime.example.invalid/oauth/resource?audience=one",
		"https://runtime.example.invalid/oauth/resource#fragment",
		"https://例え.テスト/oauth/resource",
		"https://localhost/oauth/resource",
		"https://runtime.example.invalid:123456/oauth/resource",
		"https://runtime.example.invalid/has space",
		"https://runtime.example.invalid/has\u00a0space",
		"https://runtime.example.invalid/%ZZ",
		"https://runtime.example.invalid/has\x00control",
	}
	for _, resourceURI := range valid {
		resourceURI := resourceURI
		t.Run("valid "+resourceURI, func(t *testing.T) {
			t.Parallel()
			if !formcatalog.ValidCredentialFreeHTTPSURL(resourceURI) {
				t.Fatalf("shared credential-free HTTPS validator rejected %q", resourceURI)
			}
			candidate := base
			candidate.ResourceURI = resourceURI
			if err := validateDeclaredInterfaceIdentity(candidate); err != nil {
				t.Fatalf("Interface resourceUri %q rejected: %v", resourceURI, err)
			}
		})
	}
	for _, resourceURI := range invalid {
		resourceURI := resourceURI
		t.Run("invalid "+resourceURI, func(t *testing.T) {
			t.Parallel()
			if formcatalog.ValidCredentialFreeHTTPSURL(resourceURI) {
				t.Fatalf("shared credential-free HTTPS validator accepted %q", resourceURI)
			}
			candidate := base
			candidate.ResourceURI = resourceURI
			candidate.resourceURIPresent = true
			if err := validateDeclaredInterfaceIdentity(candidate); err == nil {
				t.Fatalf("Interface resourceUri %q unexpectedly accepted", resourceURI)
			}
		})
	}
}

func TestInterfaceHTTP200RequiresNonWhitespaceJSONBody(t *testing.T) {
	for _, body := range []string{"", " \n\t"} {
		body := body
		t.Run(fmt.Sprintf("list body %q", body), func(t *testing.T) {
			server := interfaceHost(t, true, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(body))
			})
			c := discoveredClient(t, server)
			if _, err := c.ListInterfaces(context.Background(), "prod"); err == nil ||
				!strings.Contains(err.Error(), "empty JSON response body") {
				t.Fatalf("ListInterfaces body %q error = %v", body, err)
			}
		})
		t.Run(fmt.Sprintf("exact body %q", body), func(t *testing.T) {
			server := interfaceHost(t, true, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(body))
			})
			c := discoveredClient(t, server)
			_, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{
				Name: "mcp.server", Version: "1",
				ResourceKind: "ObjectBucket", ResourceName: "assets",
			})
			if err == nil || !strings.Contains(err.Error(), "empty JSON response body") {
				t.Fatalf("GetInterface body %q error = %v", body, err)
			}
		})
	}
}

func TestInterfaceListHTTP200RequiresTheInterfacesArray(t *testing.T) {
	for _, test := range []struct {
		body       string
		wantDetail bool
	}{
		{body: `{}`, wantDetail: true},
		{body: `{"interfaces":null}`, wantDetail: true},
		{body: `null`},
	} {
		test := test
		t.Run(test.body, func(t *testing.T) {
			server := interfaceHost(t, true, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.body))
			})
			c := discoveredClient(t, server)
			_, err := c.ListInterfaces(context.Background(), "prod")
			if err == nil {
				t.Fatalf("ListInterfaces body %s unexpectedly succeeded", test.body)
			}
			if test.wantDetail && !strings.Contains(err.Error(), "required interfaces array") {
				t.Fatalf("ListInterfaces body %s error = %v", test.body, err)
			}
		})
	}
}

func TestGetInterfaceRequiresResourceSelectorWhenMultipleInstancesExposeThePair(t *testing.T) {
	server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis/forms.takoform.com/v1alpha2/interfaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"interfaces": []map[string]any{
				{"name": "mcp.server", "version": "1", "resource": map[string]any{"kind": "EdgeWorker", "name": "api-a"}, "document": map[string]any{}, "values": map[string]any{}},
				{"name": "mcp.server", "version": "1", "resource": map[string]any{"kind": "EdgeWorker", "name": "api-b"}, "document": map[string]any{}, "values": map[string]any{}},
			}})
		case "/apis/forms.takoform.com/v1alpha2/interfaces/mcp.server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "mcp.server", "version": "1",
				"resource": map[string]any{"kind": r.URL.Query().Get("resourceKind"), "name": r.URL.Query().Get("resourceName")},
				"document": map[string]any{},
				"values":   map[string]any{},
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	})
	c := discoveredClient(t, server)
	if _, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{Name: "mcp.server", Version: "1"}); !errors.Is(err, ErrInterfaceInstanceAmbiguous) || !strings.Contains(err.Error(), "EdgeWorker/api-a, EdgeWorker/api-b") {
		t.Fatalf("err = %v, want deterministic instance ambiguity", err)
	}
	one, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{
		Name: "mcp.server", Version: "1", ResourceKind: "EdgeWorker", ResourceName: "api-b",
	})
	if err != nil || one.Resource.Name != "api-b" {
		t.Fatalf("exact instance = %+v err=%v", one, err)
	}
}

func TestGetInterfaceRejectsPartialResourceSelector(t *testing.T) {
	server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("partial selector must not reach host: %s", r.URL.Path)
	})
	c := discoveredClient(t, server)
	if _, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{Name: "mcp.server", ResourceKind: "EdgeWorker"}); err == nil || !strings.Contains(err.Error(), "provided together") {
		t.Fatalf("err = %v, want paired selector rejection", err)
	}
}

func TestInterfaceReadsRequireSpaceBeforeHTTP(t *testing.T) {
	requests := 0
	server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
		requests++
		_ = json.NewEncoder(w).Encode(map[string]any{"interfaces": []DeclaredInterface{}})
	})
	c := discoveredClient(t, server)
	for _, space := range []string{"", " ", "\t"} {
		if _, err := c.ListInterfaces(context.Background(), space); err == nil || !strings.Contains(err.Error(), "requires a space") {
			t.Fatalf("ListInterfaces space %q error = %v", space, err)
		}
		if _, err := c.GetInterface(context.Background(), space, InterfaceSelector{Name: "mcp.server"}); err == nil || !strings.Contains(err.Error(), "requires a space") {
			t.Fatalf("GetInterface space %q error = %v", space, err)
		}
	}
	if requests != 0 {
		t.Fatalf("blank spaces reached host %d times", requests)
	}
}

func TestListInterfacesCarriesExactEffectiveSpace(t *testing.T) {
	const space = "team alpha"
	server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("space"); got != space {
			t.Fatalf("space = %q, want %q", got, space)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"interfaces": []DeclaredInterface{}})
	})
	c := discoveredClient(t, server)
	if _, err := c.ListInterfaces(context.Background(), space); err != nil {
		t.Fatal(err)
	}
}

func TestGetInterfaceMapsOnlyStableResourceNotFound(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       int
		code         string
		wantCode     string
		wantNotFound bool
	}{
		{
			name:         "stable resource missing",
			status:       http.StatusNotFound,
			code:         "resource_not_found",
			wantCode:     "resource_not_found",
			wantNotFound: true,
		},
		{name: "Form unavailable", status: http.StatusNotFound, code: "form_not_installed"},
		{name: "plain 404", status: http.StatusNotFound},
		{name: "wrong status", status: http.StatusInternalServerError, code: "resource_not_found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				if test.code == "" {
					_, _ = w.Write([]byte("missing"))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
					"code": test.code, "message": "fixture", "requestId": "req-interface", "retryable": false,
				}})
			})
			c := discoveredClient(t, server)
			_, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{
				Name: "mcp.server", Version: "1", ResourceKind: "ObjectBucket", ResourceName: "assets",
			})
			if test.wantNotFound {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("err = %v, want ErrNotFound", err)
				}
				return
			}
			if errors.Is(err, ErrNotFound) {
				t.Fatalf("err = %v, must retain API error identity", err)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) ||
				apiErr.StatusCode != test.status ||
				apiErr.Code != test.wantCode ||
				apiErr.ProtocolInvalid != (test.wantCode == "") {
				t.Fatalf("err = %#v, want APIError status=%d code=%q", err, test.status, test.wantCode)
			}
		})
	}
}

func TestAdvertisedInterfacesEndpointMustBeSameOrigin(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"api_versions": []string{APIVersion},
			"features": map[string]bool{
				"service_forms": true, "exact_form_ref": true, "optimistic_concurrency": true,
				"idempotent_lifecycle": true, FeatureInterfaceDeclarations: true,
			},
			"endpoints": map[string]string{
				"api": server.URL + "/apis/forms.takoform.com/v1alpha2", "interfaces": "https://attacker.test/interfaces",
			},
		})
	}))
	defer server.Close()
	c := New(server.URL, "test-token", server.Client())
	if _, err := c.Discover(context.Background()); err == nil {
		t.Fatal("cross-origin interfaces endpoint must be rejected")
	}
}
