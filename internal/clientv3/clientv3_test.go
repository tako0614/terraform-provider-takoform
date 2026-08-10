package clientv3

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	testGroup        = "edge.forms.takoform.com/v1beta1"
	testKind         = "Worker"
	testSpace        = "team-a"
	testSchemaDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

var testRef = FormRef{
	APIVersion:        testGroup,
	Kind:              testKind,
	DefinitionVersion: "1.0.0",
	SchemaDigest:      testSchemaDigest,
}

// splitGroupResourcePath returns the URL path of one resource with the
// namespaced group carried as its two ordinary segments (package convention,
// spec/decisions/0018).
func splitGroupResourcePath(name, suffix string) string {
	p := APIRootPath + "/resources/" + groupPathSegments(testGroup) + "/" + testKind + "/" + name
	if suffix != "" {
		p += "/" + suffix
	}
	return p
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, body any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding test response: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func writeStableError(t *testing.T, w http.ResponseWriter, status int, code string, retryable bool) {
	t.Helper()
	writeJSON(t, w, status, map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   "test " + code,
			"requestId": "req-1",
			"retryable": retryable,
		},
	})
}

func discoveryDoc(origin string) map[string]any {
	features := map[string]bool{}
	for _, feature := range requiredFeatures {
		features[feature] = true
	}
	return map[string]any{
		"api_versions": []string{APIVersion},
		"features":     features,
		"endpoints":    map[string]string{"api": origin + APIRootPath},
	}
}

// newTestClient starts an httptest server whose non-discovery requests go to
// handle, then negotiates discovery.
func newTestClient(t *testing.T, handle func(w http.ResponseWriter, r *http.Request) bool) *Client {
	t.Helper()
	return newTestClientWithOptions(t, handle, Options{})
}

// newTestClientWithOptions is newTestClient with explicit client Options.
func newTestClientWithOptions(
	t *testing.T,
	handle func(w http.ResponseWriter, r *http.Request) bool,
	options Options,
) *Client {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			writeJSON(t, w, http.StatusOK, discoveryDoc(srv.URL))
			return
		}
		if handle != nil && handle(w, r) {
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	client := NewWithOptions(srv.URL, "test-token", srv.Client(), options)
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatalf("discovery: %v", err)
	}
	return client
}

func wireFormReference() map[string]any {
	return map[string]any{
		"formRef": map[string]any{
			"apiVersion":        testGroup,
			"kind":              testKind,
			"definitionVersion": "1.0.0",
			"schemaDigest":      testSchemaDigest,
		},
	}
}

func wireResource(name, uid, generation, revision string, spec map[string]any) map[string]any {
	return map[string]any{
		"apiVersion": testGroup,
		"kind":       testKind,
		"form":       wireFormReference(),
		"metadata": map[string]any{
			"name": name, "space": testSpace,
			"uid": uid, "generation": generation, "revision": revision,
		},
		"spec": spec,
		"status": map[string]any{
			"observedGeneration": generation,
			"conditions": []map[string]any{{
				"type":               "Ready",
				"status":             "True",
				"reason":             "Available",
				"lastTransitionTime": "2026-08-06T00:00:00Z",
			}},
		},
	}
}

func wireAvailability(operations ...string) map[string]any {
	return map[string]any{
		"forms": []map[string]any{{
			"identity":             wireFormReference(),
			"definitionKnown":      true,
			"installed":            true,
			"executable":           true,
			"activated":            true,
			"availableToPrincipal": true,
			"operations":           operations,
		}},
	}
}

func specDigestOf(t *testing.T, spec any) string {
	t.Helper()
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		t.Fatalf("digest spec: %v", err)
	}
	return digest
}

func testResourceRequest(spec map[string]any) *Resource {
	return &Resource{
		APIVersion: testGroup,
		Kind:       testKind,
		Form:       &FormReference{FormRef: testRef},
		Metadata:   Metadata{Name: "app", Space: testSpace},
		Spec:       spec,
	}
}

func TestDiscoverNegotiatesV1alpha3(t *testing.T) {
	client := newTestClient(t, nil)
	if client.apiBase == "" || !strings.HasSuffix(client.apiBase, APIRootPath) {
		t.Fatalf("apiBase not negotiated: %q", client.apiBase)
	}
	if !client.Discovery.HasFeature("artifact_upload") {
		t.Fatalf("discovery features not cached")
	}
}

func discoverWithDoc(t *testing.T, mutate func(doc map[string]any, origin string)) error {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := discoveryDoc(srv.URL)
		mutate(doc, srv.URL)
		writeJSON(t, w, http.StatusOK, doc)
	}))
	t.Cleanup(srv.Close)
	client := New(srv.URL, "test-token", srv.Client())
	_, err := client.Discover(context.Background())
	return err
}

func TestDiscoverRejectsWrongAPIVersions(t *testing.T) {
	err := discoverWithDoc(t, func(doc map[string]any, _ string) {
		doc["api_versions"] = []string{"forms.takoform.com/v1alpha2"}
	})
	if err == nil || !strings.Contains(err.Error(), "must advertise only API version") {
		t.Fatalf("expected api_versions rejection, got %v", err)
	}
}

func TestDiscoverRejectsMissingRequiredFeature(t *testing.T) {
	err := discoverWithDoc(t, func(doc map[string]any, _ string) {
		doc["features"].(map[string]bool)["operations"] = false
	})
	if err == nil || !strings.Contains(err.Error(), "features.operations") {
		t.Fatalf("expected missing-feature rejection, got %v", err)
	}
}

// TestDiscoverRejectsPercentEncodedEndpointPaths proves the client refuses a
// discovery document whose advertised endpoints are not the lane's ordinary
// path shape. The escaped form is compared, so a host advertising
// "%2Fv1beta1" cannot pass as "/v1beta1" — the exact substitution a decoded
// comparison would accept and an intermediary would then disagree about
// (spec/decisions/0018).
func TestDiscoverRejectsPercentEncodedEndpointPaths(t *testing.T) {
	encodedAPIRoot := "/apis/forms.takoform.com%2Fv1beta1"
	for _, test := range []struct {
		name     string
		mutate   func(doc map[string]any, origin string)
		wantPart string
	}{
		{
			name: "the required api endpoint",
			mutate: func(doc map[string]any, origin string) {
				doc["endpoints"].(map[string]string)["api"] = origin + encodedAPIRoot
			},
			wantPart: "invalid discovery API endpoint",
		},
		{
			name: "an optional endpoint",
			mutate: func(doc map[string]any, origin string) {
				doc["endpoints"] = map[string]string{
					"api":       origin + APIRootPath,
					"artifacts": origin + encodedAPIRoot + "/artifacts",
				}
			},
			wantPart: "invalid discovery artifacts endpoint",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := discoverWithDoc(t, test.mutate)
			if err == nil || !strings.Contains(err.Error(), test.wantPart) ||
				!strings.Contains(err.Error(), "must not percent-encode") {
				t.Fatalf("expected a percent-encoded path rejection, got %v", err)
			}
		})
	}
}

func TestDiscoverRejectsCrossOriginAPIEndpoint(t *testing.T) {
	err := discoverWithDoc(t, func(doc map[string]any, _ string) {
		doc["endpoints"].(map[string]string)["api"] = "https://other.example" + APIRootPath
	})
	if err == nil || !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("expected cross-origin rejection, got %v", err)
	}
}

func TestDiscoverRejectsCrossOriginOptionalEndpoint(t *testing.T) {
	err := discoverWithDoc(t, func(doc map[string]any, origin string) {
		doc["endpoints"] = map[string]string{
			"api":       origin + APIRootPath,
			"artifacts": "https://other.example" + APIRootPath + "/artifacts",
		}
	})
	if err == nil || !strings.Contains(err.Error(), "artifacts endpoint") {
		t.Fatalf("expected artifacts endpoint rejection, got %v", err)
	}
}
