package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func interfaceHost(t *testing.T, advertise bool, handler http.HandlerFunc) *httptest.Server {
	return interfaceHostWithWrites(t, advertise, false, handler)
}

func interfaceHostWithWrites(t *testing.T, advertise, writes bool, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/takoform" {
			features := map[string]bool{
				"service_forms": true, "exact_form_ref": true,
				"optimistic_concurrency": true, "idempotent_lifecycle": true,
			}
			if advertise {
				features[FeatureInterfaceDeclarations] = true
			}
			if writes {
				features[FeatureInterfaceDeclarationWrites] = true
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_versions": []string{APIVersion},
				"features":     features,
				"endpoints": map[string]string{
					"api":   server.URL + "/apis/forms.takoform.com/v1alpha1",
					"forms": server.URL + "/apis/forms.takoform.com/v1alpha1/forms",
				},
			})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestInterfaceWriteUsesPortableIdentityAndFences(t *testing.T) {
	var methods []string
	var preconditions []string
	var keys []string
	server := interfaceHostWithWrites(t, true, true, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		preconditions = append(preconditions, r.Header.Get("If-None-Match")+r.Header.Get("If-Match"))
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if r.URL.Path != "/apis/forms.takoform.com/v1alpha1/interfaces/example.runtime" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("space") != "prod" ||
			r.URL.Query().Get("version") != "1" ||
			r.URL.Query().Get("resourceKind") != "EdgeWorker" ||
			r.URL.Query().Get("resourceName") != "api" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		switch r.Method {
		case http.MethodPut:
			var body DeclaredInterface
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Document["title"] != "Example" || body.ResourceURIInput != "resource_uri" {
				t.Fatalf("body = %+v", body)
			}
			body.Values = map[string]any{
				"endpoint":     "https://api.example.test",
				"resource_uri": "https://api.example.test",
			}
			body.ResourceURI = "https://api.example.test"
			body.ResourceVersion = "7"
			w.Header().Set("ETag", `"7"`)
			_ = json.NewEncoder(w).Encode(body)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("method = %s", r.Method)
		}
	})
	c := discoveredClient(t, server)
	desired := DeclaredInterface{
		Name: "example.runtime", Version: "1",
		Resource: InterfaceResourceRef{Kind: "EdgeWorker", Name: "api"},
		Document: map[string]any{"title": "Example"},
		Inputs: []formpackage.InterfaceInputDeclaration{
			{Name: "endpoint", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/url"},
			{Name: "resource_uri", Source: formpackage.InterfaceInputSourceResourceURI},
		},
		ResourceURIInput: "resource_uri",
	}
	created, err := c.PutInterface(context.Background(), "prod", desired)
	if err != nil {
		t.Fatal(err)
	}
	if created.ResourceVersion != "7" || created.ResourceURI != "https://api.example.test" {
		t.Fatalf("created = %+v", created)
	}
	if err := c.DeleteInterface(context.Background(), "prod", InterfaceSelector{
		Name: "example.runtime", Version: "1", ResourceKind: "EdgeWorker", ResourceName: "api",
	}, created.ResourceVersion); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "PUT,DELETE" ||
		preconditions[0] != "*" || preconditions[1] != `"7"` ||
		keys[0] == "" || keys[1] == "" {
		t.Fatalf("methods=%v preconditions=%v keys=%v", methods, preconditions, keys)
	}
}

func TestInterfaceWriteRequiresSeparateCapability(t *testing.T) {
	server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unsupported write reached host: %s", r.URL.Path)
	})
	c := discoveredClient(t, server)
	_, err := c.PutInterface(context.Background(), "prod", DeclaredInterface{
		Name: "example.runtime", Version: "1",
		Resource: InterfaceResourceRef{Kind: "EdgeWorker", Name: "api"},
		Document: map[string]any{},
	})
	if !errors.Is(err, ErrInterfaceDeclarationWritesUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestInterfaceWriteRejectsHostAuthoredFieldSubstitution(t *testing.T) {
	server := interfaceHostWithWrites(t, true, true, func(w http.ResponseWriter, r *http.Request) {
		var body DeclaredInterface
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		body.Inputs = []formpackage.InterfaceInputDeclaration{
			{Name: "endpoint", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/substituted"},
		}
		body.ResourceVersion = "1"
		w.Header().Set("ETag", `"1"`)
		_ = json.NewEncoder(w).Encode(body)
	})
	c := discoveredClient(t, server)
	_, err := c.PutInterface(context.Background(), "prod", DeclaredInterface{
		Name: "example.runtime", Version: "1",
		Resource: InterfaceResourceRef{Kind: "EdgeWorker", Name: "api"},
		Document: map[string]any{"title": "Example"},
		Inputs: []formpackage.InterfaceInputDeclaration{
			{Name: "endpoint", Source: formpackage.InterfaceInputSourceOutput, Pointer: "/url"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "altered the authored interface declaration") {
		t.Fatalf("err = %v, want authored declaration substitution rejection", err)
	}
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
		if r.URL.Path != "/apis/forms.takoform.com/v1alpha1/interfaces/mcp.server" {
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
		case "/apis/forms.takoform.com/v1alpha1/interfaces":
			items := make([]map[string]any, 0, len(versions))
			for _, version := range versions {
				items = append(items, map[string]any{
					"name": "mcp.server", "version": version,
					"resource": map[string]any{"kind": "ObjectBucket", "name": "assets"}, "document": map[string]any{},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"interfaces": items})
		case "/apis/forms.takoform.com/v1alpha1/interfaces/mcp.server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "mcp.server", "version": r.URL.Query().Get("version"),
				"resource": map[string]any{"kind": r.URL.Query().Get("resourceKind"), "name": r.URL.Query().Get("resourceName")},
				"document": map[string]any{"title": "complete exact read"},
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

	t.Run("duplicate list identity", func(t *testing.T) {
		server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"interfaces": []map[string]any{
				{"name": "mcp.server", "version": "1", "resource": map[string]any{"kind": "ObjectBucket", "name": "assets"}, "document": map[string]any{}},
				{"name": "mcp.server", "version": "1", "resource": map[string]any{"kind": "ObjectBucket", "name": "assets"}, "document": map[string]any{}},
			}})
		})
		c := discoveredClient(t, server)
		if _, err := c.ListInterfaces(context.Background(), "prod"); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("err = %v, want duplicate rejection", err)
		}
	})
}

func TestGetInterfaceRequiresResourceSelectorWhenMultipleInstancesExposeThePair(t *testing.T) {
	server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis/forms.takoform.com/v1alpha1/interfaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"interfaces": []map[string]any{
				{"name": "mcp.server", "version": "1", "resource": map[string]any{"kind": "EdgeWorker", "name": "api-a"}, "document": map[string]any{}},
				{"name": "mcp.server", "version": "1", "resource": map[string]any{"kind": "EdgeWorker", "name": "api-b"}, "document": map[string]any{}},
			}})
		case "/apis/forms.takoform.com/v1alpha1/interfaces/mcp.server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "mcp.server", "version": "1",
				"resource": map[string]any{"kind": r.URL.Query().Get("resourceKind"), "name": r.URL.Query().Get("resourceName")},
				"document": map[string]any{},
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

func TestGetInterfaceMapsMissingToErrNotFound(t *testing.T) {
	server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	c := discoveredClient(t, server)
	if _, err := c.GetInterface(context.Background(), "prod", InterfaceSelector{Name: "mcp.server", Version: "1", ResourceKind: "ObjectBucket", ResourceName: "assets"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
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
				"api": server.URL + "/apis/forms.takoform.com/v1alpha1", "interfaces": "https://attacker.test/interfaces",
			},
		})
	}))
	defer server.Close()
	c := New(server.URL, "test-token", server.Client())
	if _, err := c.Discover(context.Background()); err == nil {
		t.Fatal("cross-origin interfaces endpoint must be rejected")
	}
}
