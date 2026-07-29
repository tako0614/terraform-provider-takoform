package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

var exactObjectBucketFixture = InstalledFormReference{
	FormRef: FormRef{
		APIVersion: APIVersion, Kind: testObjectBucketKind,
		DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:      "sha256:ee32286a40681296fc6f3db9ece79c2d651821aa2e947d1fa1cd6e28e8be8391",
	},
	PackageDigest: "sha256:0c43dfbf565c959ad627a6cd8d19aa77bf56d9e3655f44f71bb207fb79b264f2",
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestVersionedClientUsesDiscoveryExactIdentityAndMutationFences(t *testing.T) {
	t.Parallel()
	planDigest := "sha256:" + strings.Repeat("a", 64)
	var server *httptest.Server
	var mu sync.Mutex
	requests := []struct {
		method, path, ifMatch, ifNone, idempotency string
	}{}
	resource := Resource{
		APIVersion: APIVersion, Kind: testObjectBucketKind, Form: &exactObjectBucketFixture,
		Metadata: Metadata{Name: "assets", Space: "prod", ResourceVersion: "1"},
		Spec:     map[string]any{"name": "assets", "storageClass": "standard"},
		Status: &Status{
			Observed: map[string]any{"name": "assets", "storageClass": "standard"},
			Output:   map[string]any{"reference": "fixture-output"},
		},
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, struct {
			method, path, ifMatch, ifNone, idempotency string
		}{r.Method, r.URL.Path, r.Header.Get("If-Match"), r.Header.Get("If-None-Match"), r.Header.Get("Idempotency-Key")})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/.well-known/takoform":
			writeVersionedDiscovery(t, w, server.URL)
		case r.Method == http.MethodGet && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/forms":
			assertExactQuery(t, r, exactObjectBucketFixture)
			_ = json.NewEncoder(w).Encode(map[string]any{"forms": []FormAvailability{{
				Identity: exactObjectBucketFixture, DefinitionKnown: true, Installed: true,
				Executable: true, Activated: true, AvailableToPrincipal: true,
				Operations: []string{"create", "read", "update", "delete", "import", "refresh"},
			}}})
		case r.Method == http.MethodPost && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/preview":
			var desired Resource
			if err := json.NewDecoder(r.Body).Decode(&desired); err != nil {
				t.Fatal(err)
			}
			if !sameForm(desired.Form, &exactObjectBucketFixture) {
				t.Errorf("preview changed FormRef: %#v", desired)
			}
			specDigest, err := canonicalValueDigest(desired.Spec)
			if err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(PreviewResourceResult{
				Resource: desired, Review: PreviewReview{PlanDigest: planDigest, SpecDigest: specDigest},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/ObjectBucket/assets":
			var apply applyResourceBody
			if err := json.NewDecoder(r.Body).Decode(&apply); err != nil {
				t.Fatal(err)
			}
			if apply.Review.PlanDigest != planDigest || !sameForm(apply.Form, &exactObjectBucketFixture) {
				t.Errorf("invalid reviewed apply: %#v", apply)
			}
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(resource)
		case r.Method == http.MethodPost && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/ObjectBucket/assets/import":
			var imported importResourceBody
			if err := json.NewDecoder(r.Body).Decode(&imported); err != nil {
				t.Fatal(err)
			}
			if imported.NativeID != "native-assets" || !sameForm(imported.Form, &exactObjectBucketFixture) {
				t.Errorf("invalid import request: %#v", imported)
			}
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource})
		case r.Method == http.MethodGet && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/ObjectBucket/assets":
			assertExactQuery(t, r, exactObjectBucketFixture)
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(resource)
		case r.Method == http.MethodPost && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/ObjectBucket/assets/observe":
			assertExactQuery(t, r, exactObjectBucketFixture)
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource})
		case r.Method == http.MethodPost && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/ObjectBucket/assets/refresh":
			assertExactQuery(t, r, exactObjectBucketFixture)
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource})
		case r.Method == http.MethodDelete && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/ObjectBucket/assets":
			assertExactQuery(t, r, exactObjectBucketFixture)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected route", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	desired := &Resource{
		APIVersion: APIVersion, Kind: testObjectBucketKind, Form: &exactObjectBucketFixture,
		Metadata: Metadata{Name: "assets", Space: "prod"},
		Spec:     map[string]any{"name": "assets", "storageClass": "standard"},
	}
	if _, err := client.ImportResource(context.Background(), testObjectBucketKind, "assets", "native-assets", desired); err != nil {
		t.Fatal(err)
	}
	applied, err := client.PutResource(context.Background(), testObjectBucketKind, "assets", desired)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Metadata.ResourceVersion != "1" {
		t.Fatalf("resourceVersion = %q", applied.Metadata.ResourceVersion)
	}
	if _, err := client.GetResource(context.Background(), testObjectBucketKind, "assets", "prod", exactObjectBucketFixture); err != nil {
		t.Fatal(err)
	}
	fence := MutationFence{ResourceVersion: "1", Form: exactObjectBucketFixture}
	if _, err := client.ObserveResource(context.Background(), testObjectBucketKind, "assets", "prod", fence); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RefreshResource(context.Background(), testObjectBucketKind, "assets", "prod", fence); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteResource(context.Background(), testObjectBucketKind, "assets", "prod", fence); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	var sawPreview, sawApply, sawImport, sawObserve, sawRefresh, sawDelete bool
	for _, request := range requests {
		switch {
		case request.method == http.MethodPost && strings.HasSuffix(request.path, "/preview"):
			sawPreview = request.ifNone == "*" && request.idempotency == ""
		case request.method == http.MethodPut:
			sawApply = request.ifNone == "*" && strings.HasPrefix(request.idempotency, "takoform-")
		case request.method == http.MethodPost && strings.HasSuffix(request.path, "/import"):
			sawImport = request.ifNone == "*" && strings.HasPrefix(request.idempotency, "takoform-")
		case request.method == http.MethodPost && strings.HasSuffix(request.path, "/observe"):
			sawObserve = request.ifMatch == `"1"` && strings.HasPrefix(request.idempotency, "takoform-")
		case request.method == http.MethodPost && strings.HasSuffix(request.path, "/refresh"):
			sawRefresh = request.ifMatch == `"1"` && strings.HasPrefix(request.idempotency, "takoform-")
		case request.method == http.MethodDelete:
			sawDelete = request.ifMatch == `"1"` && strings.HasPrefix(request.idempotency, "takoform-")
		}
	}
	if !sawPreview || !sawApply || !sawImport || !sawObserve || !sawRefresh || !sawDelete {
		t.Fatalf("missing versioned precondition/idempotency evidence: %#v", requests)
	}
}

func TestVersionedClientRetriesOnlyStableRetryableErrors(t *testing.T) {
	t.Parallel()
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"backend_unavailable","message":"retry","requestId":"req-1","retryable":true}}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := NewWithOptions(server.URL, "", server.Client(), Options{RetryAttempts: 2})
	client.apiBase = server.URL + "/apis/forms.takoform.com/v1alpha1"
	fence := MutationFence{ResourceVersion: "1", Form: exactObjectBucketFixture}
	if err := client.DeleteResource(context.Background(), testObjectBucketKind, "assets", "prod", fence); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}

	attempts = 0
	client.retryAttempts = 3
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"error":{"code":"resource_version_conflict","message":"stale","requestId":"req-2","retryable":false}}`))
	})
	err := client.DeleteResource(context.Background(), testObjectBucketKind, "assets", "prod", fence)
	if err == nil || attempts != 1 {
		t.Fatalf("conflict err=%v attempts=%d", err, attempts)
	}
}

func TestDeleteResourceRejectsNonEmptyHTTPNoContentBody(t *testing.T) {
	t.Parallel()
	const responseBody = `{"unexpected":"state"}`
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusNoContent,
			Status:        "204 No Content",
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader(responseBody)),
			ContentLength: int64(len(responseBody)),
			Request:       request,
		}, nil
	})}
	client := New("https://forms.example.test", "", httpClient)
	client.apiBase = "https://forms.example.test/apis/forms.takoform.com/v1alpha1"
	fence := MutationFence{ResourceVersion: "1", Form: exactObjectBucketFixture}

	err := client.DeleteResource(
		context.Background(),
		testObjectBucketKind,
		"assets",
		"prod",
		fence,
	)
	if err == nil ||
		!strings.Contains(err.Error(), "HTTP 204") ||
		!strings.Contains(err.Error(), "response body") {
		t.Fatalf("DeleteResource() error = %v, want non-empty HTTP 204 response body rejection", err)
	}
}

func TestPortableRetryableRequiresExactStableTuple(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		status    int
		code      string
		retryable bool
		want      bool
	}{
		{
			name:      "resource busy",
			status:    http.StatusConflict,
			code:      "resource_busy",
			retryable: true,
			want:      true,
		},
		{
			name:      "backend unavailable",
			status:    http.StatusServiceUnavailable,
			code:      "backend_unavailable",
			retryable: true,
			want:      true,
		},
		{
			name:      "resource busy flag false",
			status:    http.StatusConflict,
			code:      "resource_busy",
			retryable: false,
		},
		{
			name:      "resource busy wrong status",
			status:    http.StatusServiceUnavailable,
			code:      "resource_busy",
			retryable: true,
		},
		{
			name:      "backend unavailable flag false",
			status:    http.StatusServiceUnavailable,
			code:      "backend_unavailable",
			retryable: false,
		},
		{
			name:      "backend unavailable wrong status",
			status:    http.StatusConflict,
			code:      "backend_unavailable",
			retryable: true,
		},
		{
			name:      "resource version conflict",
			status:    http.StatusPreconditionFailed,
			code:      "resource_version_conflict",
			retryable: true,
		},
		{
			name:      "internal error",
			status:    http.StatusInternalServerError,
			code:      "internal_error",
			retryable: true,
		},
		{
			name:      "missing stable code",
			status:    http.StatusServiceUnavailable,
			retryable: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := isPortableRetryable(&APIError{
				StatusCode: test.status,
				Code:       test.code,
				Retryable:  test.retryable,
			})
			if got != test.want {
				t.Fatalf("isPortableRetryable() = %t, want %t", got, test.want)
			}
		})
	}
	if isPortableRetryable(nil) {
		t.Fatal("nil error unexpectedly acquired retry semantics")
	}
}

func TestMutationTransportFailuresAreNeverRetried(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		method string
		body   any
	}{
		{name: "apply", method: http.MethodPut, body: map[string]any{"resource": "fixture"}},
		{name: "import", method: http.MethodPost, body: map[string]any{"nativeId": "fixture"}},
		{name: "observe", method: http.MethodPost},
		{name: "refresh", method: http.MethodPost},
		{name: "delete", method: http.MethodDelete},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempts := 0
			httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				attempts++
				if request.Body == nil {
					t.Error("mutation request body must be one-shot, including an empty body")
				}
				if request.GetBody != nil {
					t.Error("mutation request must not be replayable by net/http")
				}
				return nil, errors.New("ambiguous transport failure")
			})}
			formClient := NewWithOptions(
				"https://forms.example.test",
				"",
				httpClient,
				Options{RetryAttempts: 5},
			)
			_, err := formClient.doJSONWithHeaders(
				context.Background(),
				test.method,
				"https://forms.example.test/mutation",
				map[string]string{"Idempotency-Key": "takoform-fixture"},
				test.body,
				nil,
				true,
				http.StatusNoContent,
			)
			if err == nil || !strings.Contains(err.Error(), "ambiguous transport failure") {
				t.Fatalf("error = %v, want original ambiguous transport failure", err)
			}
			if attempts != 1 {
				t.Fatalf("attempts = %d, want exactly one", attempts)
			}
		})
	}
}

func TestResourceLifecycleMapsOnlyStableResourceNotFound(t *testing.T) {
	fence := MutationFence{ResourceVersion: "1", Form: exactObjectBucketFixture}
	operations := []struct {
		name   string
		delete bool
		run    func(*Client) error
	}{
		{name: "get", run: func(c *Client) error {
			_, err := c.GetResource(context.Background(), testObjectBucketKind, "assets", "prod", exactObjectBucketFixture)
			return err
		}},
		{name: "observe", run: func(c *Client) error {
			_, err := c.ObserveResource(context.Background(), testObjectBucketKind, "assets", "prod", fence)
			return err
		}},
		{name: "refresh", run: func(c *Client) error {
			_, err := c.RefreshResource(context.Background(), testObjectBucketKind, "assets", "prod", fence)
			return err
		}},
		{name: "delete", delete: true, run: func(c *Client) error {
			return c.DeleteResource(context.Background(), testObjectBucketKind, "assets", "prod", fence)
		}},
	}
	responses := []struct {
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
	}

	for _, operation := range operations {
		for _, response := range responses {
			t.Run(operation.name+"/"+response.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(response.status)
					if response.code == "" {
						_, _ = w.Write([]byte("missing"))
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
						"code": response.code, "message": "fixture", "requestId": "req-resource", "retryable": false,
					}})
				}))
				defer server.Close()

				c := New(server.URL, "", server.Client())
				c.apiBase = server.URL
				err := operation.run(c)
				if response.wantNotFound {
					if operation.delete {
						if err != nil {
							t.Fatalf("delete err = %v, want already deleted", err)
						}
					} else if !errors.Is(err, ErrNotFound) {
						t.Fatalf("err = %v, want ErrNotFound", err)
					}
					return
				}
				if errors.Is(err, ErrNotFound) || err == nil {
					t.Fatalf("err = %v, must retain API error identity", err)
				}
				var apiErr *APIError
				if !errors.As(err, &apiErr) ||
					apiErr.StatusCode != response.status ||
					apiErr.Code != response.wantCode ||
					apiErr.ProtocolInvalid != (response.wantCode == "") {
					t.Fatalf("err = %#v, want APIError status=%d code=%q", err, response.status, response.wantCode)
				}
			})
		}
	}
}

func TestDiscoveryRejectsCrossOriginEndpointsBeforeSendingBearer(t *testing.T) {
	t.Parallel()
	evilRequests := 0
	evil := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		evilRequests++
	}))
	defer evil.Close()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/takoform" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"api_versions": []string{APIVersion},
			"features": map[string]bool{
				"service_forms": true, "exact_form_ref": true,
				"optimistic_concurrency": true, "idempotent_lifecycle": true,
			},
			"endpoints": map[string]string{"api": evil.URL + "/apis/forms.takoform.com/v1alpha1"},
		})
	}))
	defer host.Close()

	client := New(host.URL, "must-not-leak", host.Client())
	if _, err := client.Discover(context.Background()); err == nil || !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("expected cross-origin discovery rejection, got %v", err)
	}
	if evilRequests != 0 {
		t.Fatalf("cross-origin endpoint received %d requests", evilRequests)
	}
}

func TestDiscoveryRequiresConfiguredOrigin(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{
		"forms.example.com", "ftp://forms.example.com", "https://user@forms.example.com",
		"https://forms.example.com/base", "https://forms.example.com?api=1", "https://forms.example.com#fragment",
		"http://forms.example.com",
	} {
		client := New(endpoint, "", nil)
		if _, err := client.Discover(context.Background()); err == nil {
			t.Fatalf("Discover(%q) unexpectedly succeeded", endpoint)
		}
	}
	for _, endpoint := range []string{"http://localhost:8090", "http://127.0.0.2:8090", "http://[::1]:8090"} {
		client := New(endpoint, "", nil)
		if _, err := client.configuredOrigin(); err != nil {
			t.Fatalf("configuredOrigin(%q) rejected loopback development origin: %v", endpoint, err)
		}
	}
}

func TestClientRejectsOversizedResponses(t *testing.T) {
	t.Parallel()
	exact := []byte(`{"padding":"` + strings.Repeat("x", maxResponseBodyBytes-len(`{"padding":""}`)) + `"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/exact" {
			_, _ = w.Write(exact)
			return
		}
		_, _ = w.Write(append(exact, '\n'))
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	var accepted map[string]string
	if err := client.doJSON(context.Background(), http.MethodGet, server.URL+"/exact", nil, &accepted); err != nil {
		t.Fatalf("response at exact bound was rejected: %v", err)
	}
	if len(accepted["padding"]) == 0 {
		t.Fatal("response at exact bound was not decoded")
	}
	if _, err := client.Discover(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected bounded response error, got %v", err)
	}
}

func TestCaptureResourceVersionRejectsMissingInvalidAndConflictingFences(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, bodyVersion, etag string
		wantError               bool
	}{
		{name: "missing ETag", bodyVersion: "2", wantError: true},
		{name: "missing body", etag: `"2"`, wantError: true},
		{name: "matching", bodyVersion: "2", etag: `"2"`},
		{name: "missing", wantError: true},
		{name: "invalid body", bodyVersion: "rv-2", wantError: true},
		{name: "overflowing body", bodyVersion: "9223372036854775808", etag: `"9223372036854775808"`, wantError: true},
		{name: "unquoted ETag", bodyVersion: "2", etag: "2", wantError: true},
		{name: "weak ETag", bodyVersion: "2", etag: `W/"2"`, wantError: true},
		{name: "conflict", bodyVersion: "2", etag: `"3"`, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			resource := Resource{Metadata: Metadata{ResourceVersion: test.bodyVersion}}
			headers := http.Header{}
			if test.etag != "" {
				headers.Set("ETag", test.etag)
			}
			err := captureResourceVersion(&resource, headers)
			if (err != nil) != test.wantError {
				t.Fatalf("error=%v wantError=%v", err, test.wantError)
			}
			if err == nil && resource.Metadata.ResourceVersion != "2" {
				t.Fatalf("resourceVersion=%q", resource.Metadata.ResourceVersion)
			}
		})
	}
}

func TestResourceVersionUsesCanonicalPositiveInt64Range(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"1",
		"9007199254740993",
		"9223372036854775806",
		"9223372036854775807",
	} {
		if !validResourceVersion(value) {
			t.Errorf("valid resourceVersion %q was rejected", value)
		}
	}
	for _, value := range []string{
		"",
		"0",
		"00",
		"01",
		"-1",
		"+1",
		"1.0",
		"9223372036854775808",
		"10000000000000000000",
	} {
		if validResourceVersion(value) {
			t.Errorf("invalid resourceVersion %q was accepted", value)
		}
	}
}

func TestStrictWireDecoderPreservesGenerationBeyondIEEE754(t *testing.T) {
	t.Parallel()

	var decoded struct {
		Status map[string]any `json:"status"`
	}
	if err := decodeStrictJSON(
		[]byte(`{"status":{"generation":9007199254740993}}`),
		&decoded,
	); err != nil {
		t.Fatal(err)
	}
	generation, ok := decoded.Status["generation"].(json.Number)
	if !ok || generation.String() != "9007199254740993" {
		t.Fatalf("decoded generation = %T(%v), want exact json.Number", decoded.Status["generation"], decoded.Status["generation"])
	}
}

func TestObserveAndRefreshSuccessMustEchoIfMatchGeneration(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		operation string
		fence     string
		returned  string
		wantError bool
	}{
		{name: "observe exact", operation: "observe", fence: "7", returned: "7"},
		{name: "observe advanced", operation: "observe", fence: "7", returned: "8", wantError: true},
		{name: "refresh exact maximum", operation: "refresh", fence: "9223372036854775807", returned: "9223372036854775807"},
		{name: "refresh stale response", operation: "refresh", fence: "7", returned: "6", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("If-Match"); got != quoteResourceVersion(test.fence) {
					t.Errorf("If-Match = %q, want %q", got, quoteResourceVersion(test.fence))
				}
				resource := Resource{
					APIVersion: APIVersion,
					Kind:       testObjectBucketKind,
					Form:       &exactObjectBucketFixture,
					Metadata: Metadata{
						Name: "assets", Space: "prod", ResourceVersion: test.returned,
					},
					Spec: map[string]any{"name": "assets"},
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("ETag", quoteResourceVersion(test.returned))
				_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource})
			}))
			defer server.Close()

			formClient := New(server.URL, "", server.Client())
			formClient.apiBase = server.URL
			fence := MutationFence{ResourceVersion: test.fence, Form: exactObjectBucketFixture}
			var err error
			switch test.operation {
			case "observe":
				_, err = formClient.ObserveResource(context.Background(), testObjectBucketKind, "assets", "prod", fence)
			case "refresh":
				_, err = formClient.RefreshResource(context.Background(), testObjectBucketKind, "assets", "prod", fence)
			default:
				t.Fatalf("unknown operation %q", test.operation)
			}
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError = %v", err, test.wantError)
			}
			if err != nil && !strings.Contains(err.Error(), "generation protected by If-Match") {
				t.Fatalf("error = %v, want exact generation-fence rejection", err)
			}
		})
	}
}

func TestExactInstalledFormReferenceValidationFailsClosed(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"1.0.0", "0.0.0-legacy.1", "1.2.3-rc.1+build.5"} {
		form := exactObjectBucketFixture
		form.FormRef.DefinitionVersion = version
		if err := validateInstalledFormReference(testObjectBucketKind, form); err != nil {
			t.Fatalf("valid SemVer %q rejected: %v", version, err)
		}
	}

	for _, test := range []struct {
		name   string
		kind   string
		mutate func(*InstalledFormReference)
	}{
		{name: "API version", mutate: func(form *InstalledFormReference) { form.FormRef.APIVersion = "forms.takoform.com/v0" }},
		{name: "kind", mutate: func(form *InstalledFormReference) { form.FormRef.Kind = testQueueKind }},
		{name: "lowercase kind", kind: "objectBucket", mutate: func(form *InstalledFormReference) {
			form.FormRef.Kind = "objectBucket"
		}},
		{name: "empty definition version", mutate: func(form *InstalledFormReference) { form.FormRef.DefinitionVersion = "" }},
		{name: "short definition version", mutate: func(form *InstalledFormReference) { form.FormRef.DefinitionVersion = "1" }},
		{name: "prefixed definition version", mutate: func(form *InstalledFormReference) { form.FormRef.DefinitionVersion = "v1.2.3" }},
		{name: "leading zero definition version", mutate: func(form *InstalledFormReference) { form.FormRef.DefinitionVersion = "01.2.3" }},
		{name: "numeric prerelease leading zero", mutate: func(form *InstalledFormReference) { form.FormRef.DefinitionVersion = "1.2.3-01" }},
		{name: "trailing prerelease separator", mutate: func(form *InstalledFormReference) { form.FormRef.DefinitionVersion = "1.2.3-" }},
		{name: "trailing build separator", mutate: func(form *InstalledFormReference) { form.FormRef.DefinitionVersion = "1.2.3+" }},
		{name: "definition version whitespace", mutate: func(form *InstalledFormReference) { form.FormRef.DefinitionVersion = " 1.2.3" }},
		{name: "invalid schema digest", mutate: func(form *InstalledFormReference) { form.FormRef.SchemaDigest = "sha256:not-a-digest" }},
		{name: "uppercase schema digest", mutate: func(form *InstalledFormReference) {
			form.FormRef.SchemaDigest = "sha256:" + strings.Repeat("A", 64)
		}},
		{name: "uppercase package digest", mutate: func(form *InstalledFormReference) {
			form.PackageDigest = "sha256:" + strings.Repeat("B", 64)
		}},
		{name: "empty package digest", mutate: func(form *InstalledFormReference) { form.PackageDigest = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			form := exactObjectBucketFixture
			test.mutate(&form)
			kind := test.kind
			if kind == "" {
				kind = testObjectBucketKind
			}
			if err := validateInstalledFormReference(kind, form); err == nil {
				t.Fatalf("invalid FormRef unexpectedly passed: %#v", form)
			}
		})
	}
}

func TestVersionedResourceResponseRejectsNonCanonicalFormRefJSON(t *testing.T) {
	t.Parallel()
	validRaw, err := json.Marshal(exactObjectBucketFixture.FormRef)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, raw, wantError string
	}{
		{
			name:      "unknown field",
			raw:       strings.TrimSuffix(string(validRaw), "}") + `,"extension":"must-not-be-ignored"}`,
			wantError: "extension",
		},
		{
			name:      "duplicate field",
			raw:       strings.Replace(string(validRaw), "{", `{"kind":"objectBucket",`, 1),
			wantError: "duplicate object name",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("ETag", `"1"`)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"apiVersion": APIVersion,
					"kind":       testObjectBucketKind,
					"form": map[string]any{
						"formRef":       json.RawMessage(test.raw),
						"packageDigest": exactObjectBucketFixture.PackageDigest,
					},
					"metadata": map[string]any{
						"name":            "assets",
						"space":           "prod",
						"resourceVersion": "1",
					},
					"spec": map[string]any{"name": "assets"},
				})
			}))
			defer server.Close()

			formClient := New(server.URL, "", server.Client())
			formClient.apiBase = server.URL
			_, err := formClient.GetResource(
				context.Background(),
				testObjectBucketKind,
				"assets",
				"prod",
				exactObjectBucketFixture,
			)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("non-canonical nested FormRef error = %v", err)
			}
		})
	}
}

func TestVersionedLifecycleRejectsResponseNameAndSpaceSubstitution(t *testing.T) {
	operations := []struct {
		name string
		run  func(*Client, *Resource) error
	}{
		{name: "put", run: func(c *Client, desired *Resource) error {
			_, err := c.PutResource(context.Background(), testObjectBucketKind, "assets", desired)
			return err
		}},
		{name: "import", run: func(c *Client, desired *Resource) error {
			_, err := c.ImportResource(context.Background(), testObjectBucketKind, "assets", "native-assets", desired)
			return err
		}},
		{name: "get", run: func(c *Client, _ *Resource) error {
			_, err := c.GetResource(context.Background(), testObjectBucketKind, "assets", "prod", exactObjectBucketFixture)
			return err
		}},
		{name: "observe", run: func(c *Client, _ *Resource) error {
			_, err := c.ObserveResource(context.Background(), testObjectBucketKind, "assets", "prod", MutationFence{ResourceVersion: "1", Form: exactObjectBucketFixture})
			return err
		}},
		{name: "refresh", run: func(c *Client, _ *Resource) error {
			_, err := c.RefreshResource(context.Background(), testObjectBucketKind, "assets", "prod", MutationFence{ResourceVersion: "1", Form: exactObjectBucketFixture})
			return err
		}},
	}
	mutations := []struct {
		name   string
		mutate func(*Resource)
	}{
		{name: "name", mutate: func(resource *Resource) { resource.Metadata.Name = "substituted" }},
		{name: "space", mutate: func(resource *Resource) { resource.Metadata.Space = "other-space" }},
	}

	for _, operation := range operations {
		for _, mutation := range mutations {
			t.Run(operation.name+"/"+mutation.name, func(t *testing.T) {
				var server *httptest.Server
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch {
					case r.URL.Path == "/.well-known/takoform":
						writeVersionedDiscovery(t, w, server.URL)
					case r.URL.Path == "/apis/forms.takoform.com/v1alpha1/forms":
						_ = json.NewEncoder(w).Encode(map[string]any{"forms": []FormAvailability{{
							Identity: exactObjectBucketFixture, DefinitionKnown: true, Installed: true,
							Executable: true, Activated: true, AvailableToPrincipal: true,
							Operations: []string{"create", "import"},
						}}})
					case r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/preview":
						var desired Resource
						if err := json.NewDecoder(r.Body).Decode(&desired); err != nil {
							t.Fatal(err)
						}
						specDigest, err := canonicalValueDigest(desired.Spec)
						if err != nil {
							t.Fatal(err)
						}
						_ = json.NewEncoder(w).Encode(PreviewResourceResult{
							Resource: desired,
							Review: PreviewReview{
								PlanDigest: "sha256:" + strings.Repeat("a", 64),
								SpecDigest: specDigest,
							},
						})
					default:
						resource := Resource{
							APIVersion: APIVersion, Kind: testObjectBucketKind, Form: &exactObjectBucketFixture,
							Metadata: Metadata{Name: "assets", Space: "prod", ResourceVersion: "1"},
						}
						mutation.mutate(&resource)
						w.Header().Set("ETag", `"1"`)
						switch operation.name {
						case "put", "get":
							_ = json.NewEncoder(w).Encode(resource)
						case "import":
							_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource})
						case "observe", "refresh":
							_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource})
						}
					}
				}))
				defer server.Close()

				formClient := New(server.URL, "", server.Client())
				if _, err := formClient.Discover(context.Background()); err != nil {
					t.Fatal(err)
				}
				desired := &Resource{
					APIVersion: APIVersion, Kind: testObjectBucketKind, Form: &exactObjectBucketFixture,
					Metadata: Metadata{Name: "assets", Space: "prod"}, Spec: map[string]any{"name": "assets"},
				}
				err := operation.run(formClient, desired)
				if err == nil || !strings.Contains(err.Error(), "name or space") {
					t.Fatalf("error = %v, want fail-closed requested identity rejection", err)
				}
			})
		}
	}
}

func writeVersionedDiscovery(t *testing.T, w http.ResponseWriter, origin string) {
	t.Helper()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"api_versions": []string{APIVersion},
		"features":     map[string]bool{"service_forms": true, "exact_form_ref": true, "optimistic_concurrency": true, "idempotent_lifecycle": true},
		"endpoints": map[string]string{
			"api":   origin + "/apis/forms.takoform.com/v1alpha1",
			"forms": origin + "/apis/forms.takoform.com/v1alpha1/forms",
		},
	})
}

func assertExactQuery(t *testing.T, r *http.Request, form InstalledFormReference) {
	t.Helper()
	query := r.URL.Query()
	want := map[string]string{
		"space": "prod", "apiVersion": form.FormRef.APIVersion, "kind": form.FormRef.Kind,
		"definitionVersion": form.FormRef.DefinitionVersion, "schemaDigest": form.FormRef.SchemaDigest,
		"packageDigest": form.PackageDigest,
	}
	for key, value := range want {
		if query.Get(key) != value {
			t.Errorf("query %s=%q, want %q", key, query.Get(key), value)
		}
	}
}
