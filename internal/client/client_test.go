package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	testObjectBucketKind = "ObjectBucket"
	testQueueKind        = "Queue"
)

func TestPortableWireModelContainsOnlyPortableFields(t *testing.T) {
	for _, test := range []struct {
		name string
		typ  reflect.Type
		want string
	}{
		{name: "discovery", typ: reflect.TypeOf(Discovery{}), want: "api_versions,features,endpoints"},
		{name: "metadata", typ: reflect.TypeOf(Metadata{}), want: "name,space,resourceVersion,revision"},
		{name: "status", typ: reflect.TypeOf(Status{}), want: "observed,output"},
		{name: "resource", typ: reflect.TypeOf(Resource{}), want: "apiVersion,kind,form,metadata,spec,status"},
		{name: "preview", typ: reflect.TypeOf(PreviewResourceResult{}), want: "resource,review"},
		{name: "preview review", typ: reflect.TypeOf(PreviewReview{}), want: "planDigest,specDigest"},
		{name: "interface projection", typ: reflect.TypeOf(DeclaredInterface{}), want: "name,version,resource,document,values,resourceUri,form"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fields := make([]string, 0, test.typ.NumField())
			for index := 0; index < test.typ.NumField(); index++ {
				name := strings.Split(test.typ.Field(index).Tag.Get("json"), ",")[0]
				if name != "" && name != "-" {
					fields = append(fields, name)
				}
			}
			if got := strings.Join(fields, ","); got != test.want {
				t.Fatalf("JSON fields = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPortableWireDecoderRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	for _, raw := range []string{
		`{"name":"assets","space":"prod","selectedTarget":"private-host"}`,
		`{"name":"assets","space":"prod"} {"name":"other","space":"prod"}`,
	} {
		var metadata Metadata
		if err := decodeStrictJSON([]byte(raw), &metadata); err == nil {
			t.Fatalf("decodeStrictJSON(%s) unexpectedly accepted a non-closed envelope", raw)
		}
	}
}

func TestPortableWireRejectsUndeclaredSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	var response map[string]any
	err := New(server.URL, "", server.Client()).doJSON(
		context.Background(),
		http.MethodGet,
		server.URL,
		nil,
		&response,
	)
	if err == nil || !strings.Contains(err.Error(), "unexpected success status 201") {
		t.Fatalf("unexpected status error = %v", err)
	}
}

func TestErrorEnvelopeAcquiresStableSemanticsOnlyForExactProtocolTuple(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		raw       string
		wantCode  string
		wantRetry bool
	}{
		{
			name:     "resource not found",
			status:   http.StatusNotFound,
			raw:      `{"error":{"code":"resource_not_found","message":"missing","requestId":"req-1","retryable":false}}`,
			wantCode: "resource_not_found",
		},
		{
			name:      "backend unavailable",
			status:    http.StatusServiceUnavailable,
			raw:       `{"error":{"code":"backend_unavailable","message":"retry","requestId":"req-2","retryable":true}}`,
			wantCode:  "backend_unavailable",
			wantRetry: true,
		},
		{
			name:   "unknown field",
			status: http.StatusNotFound,
			raw:    `{"error":{"code":"resource_not_found","message":"missing","requestId":"req-1","retryable":false,"selectedTarget":"private"}}`,
		},
		{
			name:   "duplicate error code",
			status: http.StatusNotFound,
			raw:    `{"error":{"code":"resource_not_found","code":"resource_not_found","message":"missing","requestId":"req-1","retryable":false}}`,
		},
		{
			name:   "missing request ID",
			status: http.StatusNotFound,
			raw:    `{"error":{"code":"resource_not_found","message":"missing","retryable":false}}`,
		},
		{
			name:   "missing retryable",
			status: http.StatusNotFound,
			raw:    `{"error":{"code":"resource_not_found","message":"missing","requestId":"req-1"}}`,
		},
		{
			name:   "unknown code",
			status: http.StatusNotImplemented,
			raw:    `{"error":{"code":"not_implemented","message":"unknown","requestId":"req-3","retryable":false}}`,
		},
		{
			name:   "stable code with wrong status",
			status: http.StatusInternalServerError,
			raw:    `{"error":{"code":"resource_not_found","message":"missing","requestId":"req-4","retryable":false}}`,
		},
		{
			name:   "non-retryable code claims retryable",
			status: http.StatusBadRequest,
			raw:    `{"error":{"code":"invalid_argument","message":"bad","requestId":"req-5","retryable":true}}`,
		},
		{
			name:   "retryable code with wrong status",
			status: http.StatusConflict,
			raw:    `{"error":{"code":"backend_unavailable","message":"retry","requestId":"req-6","retryable":true}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			apiErr := parseAPIError(test.status, []byte(test.raw), "")
			if apiErr.Code != test.wantCode ||
				apiErr.Retryable != test.wantRetry ||
				apiErr.ProtocolInvalid != (test.wantCode == "") {
				t.Fatalf("parseAPIError() = %#v", apiErr)
			}
			if test.wantCode == "" && (isResourceNotFound(apiErr) || isPortableRetryable(apiErr)) {
				t.Fatalf("protocol-invalid envelope acquired stable semantics: %#v", apiErr)
			}
		})
	}
}

func TestDiscoveryOriginAndOIDCIssuerNormalization(t *testing.T) {
	parse := func(raw string) *url.URL {
		t.Helper()
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	if !sameOrigin(parse("https://example.test"), parse("https://EXAMPLE.test:443")) {
		t.Fatal("default HTTPS port should identify the same origin")
	}
	if !sameOrigin(parse("http://localhost"), parse("http://LOCALHOST:80")) {
		t.Fatal("default HTTP port should identify the same origin")
	}
	if sameOrigin(parse("https://example.test"), parse("https://example.test:8443")) {
		t.Fatal("different effective ports must identify different origins")
	}

	for _, raw := range []string{
		"http://issuer.example.test",
		"https://user@issuer.example.test",
		"https://issuer.example.test?tenant=prod",
		"https://issuer.example.test#fragment",
	} {
		if err := validateOIDCIssuer(raw); err == nil {
			t.Fatalf("validateOIDCIssuer(%q) unexpectedly passed", raw)
		}
	}
	if err := validateOIDCIssuer("https://issuer.example.test/takoform"); err != nil {
		t.Fatalf("valid OIDC issuer: %v", err)
	}
}

func TestPreviewResultBindsExactRequestedSpecAndFence(t *testing.T) {
	request := Resource{
		APIVersion: APIVersion,
		Kind:       testObjectBucketKind,
		Form:       &exactObjectBucketFixture,
		Metadata: Metadata{
			Name:            "assets",
			Space:           "prod",
			ResourceVersion: "7",
		},
		Spec: map[string]any{"name": "assets", "storageClass": "standard"},
	}
	specDigest, err := canonicalValueDigest(request.Spec)
	if err != nil {
		t.Fatal(err)
	}
	valid := PreviewResourceResult{
		Resource: request,
		Review: PreviewReview{
			PlanDigest: "sha256:" + strings.Repeat("a", 64),
			SpecDigest: specDigest,
		},
	}
	if err := validatePreviewResult(&request, &valid); err != nil {
		t.Fatalf("valid preview: %v", err)
	}

	for name, mutate := range map[string]func(*PreviewResourceResult){
		"generation": func(result *PreviewResourceResult) {
			result.Resource.Metadata.ResourceVersion = "8"
		},
		"status": func(result *PreviewResourceResult) {
			result.Resource.Status = &Status{Observed: map[string]any{}}
		},
		"spec": func(result *PreviewResourceResult) {
			result.Resource.Spec = map[string]any{"name": "other", "storageClass": "standard"}
		},
		"spec digest": func(result *PreviewResourceResult) {
			result.Review.SpecDigest = "sha256:" + strings.Repeat("b", 64)
		},
		"plan digest": func(result *PreviewResourceResult) {
			result.Review.PlanDigest = "not-a-digest"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := validatePreviewResult(&request, &candidate); err == nil {
				t.Fatal("tampered preview unexpectedly passed")
			}
		})
	}
}

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
			"api":         origin + "/apis/forms.takoform.com/v1alpha2",
			"forms":       origin + "/apis/forms.takoform.com/v1alpha2/forms",
			"oidc_issuer": "https://issuer.example.test/takoform",
		},
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

func TestDiscover_Success(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/takoform/v1alpha2" {
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

func TestDiscoverRejectsMixedEpochsAndLegacyAPIBase(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"mixed versions":  `{"api_versions":["` + APIVersion + `","forms.takoform.com/v1alpha1"],"features":{"service_forms":true,"exact_form_ref":true,"optimistic_concurrency":true,"idempotent_lifecycle":true},"endpoints":{"api":"ORIGIN` + APIRootPath + `"}}`,
		"legacy API base": `{"api_versions":["` + APIVersion + `"],"features":{"service_forms":true,"exact_form_ref":true,"optimistic_concurrency":true,"idempotent_lifecycle":true},"endpoints":{"api":"ORIGIN/apis/forms.takoform.com/v1alpha1"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, strings.ReplaceAll(body, "ORIGIN", server.URL))
			}))
			defer server.Close()

			_, err := New(server.URL, "", server.Client()).Discover(context.Background())
			if err == nil {
				t.Fatal("ambiguous current discovery unexpectedly passed")
			}
		})
	}
}

func TestErrorEnvelope(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/takoform/v1alpha2" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, discoveryBody(true, srv.URL))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		// Nested error envelope: the "error" field is an object.
		_, _ = io.WriteString(w, `{"error":{"code":"invalid_argument","message":"interfaces must not be empty","requestId":"req-42","retryable":false,"details":{"field":"interfaces"}}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "", srv.Client())
	if _, err := c.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	_, err := c.GetResource(context.Background(), testObjectBucketKind, "assets", "prod", exactObjectBucketFixture)
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

func TestAdvertisedEndpointsCompareEffectiveOrigins(t *testing.T) {
	for _, test := range []struct {
		name       string
		configured string
		advertised string
		wantError  bool
	}{
		{
			name:       "implicit and explicit HTTPS default port",
			configured: "https://forms.example.test",
			advertised: "https://forms.example.test:443/apis/forms.takoform.com/v1alpha2",
		},
		{
			name:       "explicit and implicit HTTPS default port",
			configured: "https://forms.example.test:443",
			advertised: "https://forms.example.test/apis/forms.takoform.com/v1alpha2",
		},
		{
			name:       "implicit and explicit loopback HTTP default port",
			configured: "http://localhost",
			advertised: "http://localhost:80/apis/forms.takoform.com/v1alpha2",
		},
		{
			name:       "changed port",
			configured: "https://forms.example.test",
			advertised: "https://forms.example.test:444/apis/forms.takoform.com/v1alpha2",
			wantError:  true,
		},
		{
			name:       "changed host",
			configured: "https://forms.example.test",
			advertised: "https://other.example.test/apis/forms.takoform.com/v1alpha2",
			wantError:  true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := New(test.configured, "", nil)
			_, err := c.validAdvertisedEndpoint(test.advertised, APIRootPath)
			if (err != nil) != test.wantError {
				t.Fatalf("validAdvertisedEndpoint() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}

func TestDiscoveryValidatesOptionalOIDCIssuer(t *testing.T) {
	base := Discovery{
		APIVersions: []string{APIVersion},
		Features: map[string]bool{
			"service_forms": true, "exact_form_ref": true,
			"optimistic_concurrency": true, "idempotent_lifecycle": true,
		},
		Endpoints: Endpoints{API: "https://forms.example.test/apis/forms.takoform.com/v1alpha2"},
	}
	for _, test := range []struct {
		name      string
		issuer    string
		wantError bool
	}{
		{name: "omitted"},
		{name: "cross-origin HTTPS path", issuer: "https://accounts.example.test/realms/takoform"},
		{name: "HTTPS non-default port", issuer: "https://accounts.example.test:8443/issuer"},
		{name: "relative", issuer: "/issuer", wantError: true},
		{name: "HTTP", issuer: "http://accounts.example.test/issuer", wantError: true},
		{name: "userinfo", issuer: "https://user@accounts.example.test/issuer", wantError: true},
		{name: "query", issuer: "https://accounts.example.test/issuer?tenant=one", wantError: true},
		{name: "fragment", issuer: "https://accounts.example.test/issuer#metadata", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			discovery := base
			discovery.Endpoints.OIDCIssuer = test.issuer
			c := New("https://forms.example.test", "", nil)
			err := c.negotiateEndpoints(discovery)
			if (err != nil) != test.wantError {
				t.Fatalf("negotiateEndpoints() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "empty", value: ""},
		{name: "delta seconds", value: "5", want: 5 * time.Second},
		{name: "zero seconds", value: "0"},
		{name: "negative seconds", value: "-1"},
		{name: "HTTP date", value: time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat), want: 3 * time.Second},
		{name: "garbage", value: "soon"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := parseRetryAfter(test.value)
			if test.want == 0 {
				if got != 0 {
					t.Fatalf("parseRetryAfter(%q) = %v, want 0", test.value, got)
				}
				return
			}
			// HTTP-date parsing is subject to clock skew; allow a small window.
			if got < test.want-2*time.Second || got > test.want+2*time.Second {
				t.Fatalf("parseRetryAfter(%q) = %v, want ~%v", test.value, got, test.want)
			}
		})
	}
}

func TestParseAPIErrorCarriesRetryAfter(t *testing.T) {
	apiErr := parseAPIError(
		http.StatusServiceUnavailable,
		[]byte(`{"error":{"code":"backend_unavailable","message":"retry","requestId":"req-7","retryable":true}}`),
		"4",
	)
	if apiErr.RetryAfter != 4*time.Second {
		t.Fatalf("RetryAfter = %v, want 4s", apiErr.RetryAfter)
	}
	if apiErr.Code != "backend_unavailable" || !apiErr.Retryable || apiErr.ProtocolInvalid {
		t.Fatalf("unexpected error envelope: %#v", apiErr)
	}
}
