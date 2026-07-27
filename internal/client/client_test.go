package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discoveryBody(serviceForms bool, origin string) string {
	body := map[string]any{
		"api_versions": []string{APIVersion},
		"features": map[string]bool{
			"service_forms":          serviceForms,
			"exact_form_ref":         true,
			"optimistic_concurrency": true,
			"idempotent_lifecycle":   true,
			"oidc":                   true,
		},
		"endpoints": map[string]string{
			"api":         origin + "/apis/forms.takoform.com/v1alpha1",
			"forms":       origin + "/apis/forms.takoform.com/v1alpha1/forms",
			"oidc_issuer": origin,
		},
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

func TestDiscover_Success(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/takoform" {
			t.Errorf("unexpected discovery path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, discoveryBody(true, srv.URL))
	}))
	defer srv.Close()

	c := New(srv.URL, "", srv.Client())
	disco, err := c.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !disco.SupportsServiceForms() {
		t.Fatalf("expected SupportsServiceForms true")
	}
	if !disco.HasFeature("oidc") {
		t.Fatalf("expected oidc feature present")
	}
	if disco.Endpoints.API == "" {
		t.Fatalf("expected versioned api endpoint parsed")
	}
	if len(disco.APIVersions) != 1 || disco.APIVersions[0] != APIVersion {
		t.Fatalf("unexpected api_versions: %#v", disco.APIVersions)
	}
	// Discovery is cached on the client.
	if !c.Discovery.SupportsServiceForms() {
		t.Fatalf("expected cached Discovery")
	}
}

func TestDiscover_ServiceFormsFalse(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, discoveryBody(false, srv.URL))
	}))
	defer srv.Close()

	c := New(srv.URL, "", srv.Client())
	_, err := c.Discover(context.Background())
	if err == nil || !strings.Contains(err.Error(), "features.service_forms") {
		t.Fatalf("expected service_forms negotiation error, got %v", err)
	}
}

// TestDiscoverRequiresVersionedEndpoint proves that a host advertising no
// versioned API base is rejected. There is no unversioned lane to silently
// downgrade into.
func TestDiscoverRequiresVersionedEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"api_versions":["`+APIVersion+`"],"features":{"service_forms":true},"endpoints":{}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "", srv.Client())
	_, err := c.Discover(context.Background())
	if err == nil || !strings.Contains(err.Error(), "endpoints.api") {
		t.Fatalf("expected versioned endpoint requirement, got %v", err)
	}
}

func TestErrorEnvelope(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/takoform" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, discoveryBody(true, srv.URL))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		// Nested error envelope: the "error" field is an object.
		_, _ = io.WriteString(w, `{"error":{"code":"invalid_argument","message":"interfaces must not be empty","requestId":"req-42","details":{"field":"interfaces"}}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "", srv.Client())
	if _, err := c.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	_, err := c.GetResource(context.Background(), KindObjectBucket, "assets", "prod", exactObjectBucketFixture)
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("unexpected status %d", apiErr.StatusCode)
	}
	if apiErr.Code != "invalid_argument" {
		t.Errorf("unexpected code %q", apiErr.Code)
	}
	if apiErr.Message != "interfaces must not be empty" {
		t.Errorf("unexpected message %q", apiErr.Message)
	}
	if apiErr.RequestID != "req-42" {
		t.Errorf("unexpected requestId %q", apiErr.RequestID)
	}
	if string(apiErr.Details) != `{"field":"interfaces"}` {
		t.Errorf("unexpected details %q", string(apiErr.Details))
	}
	if msg := apiErr.Error(); msg == "" {
		t.Errorf("expected non-empty error string")
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	c := New("https://takoform.example.com/", "", nil)
	if c.Endpoint() != "https://takoform.example.com" {
		t.Fatalf("expected trailing slash trimmed, got %q", c.Endpoint())
	}
}
