package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

var exactObjectBucketFixture = InstalledFormReference{
	FormRef: FormRef{
		APIVersion: APIVersion, Kind: KindObjectBucket,
		DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:      "sha256:ee32286a40681296fc6f3db9ece79c2d651821aa2e947d1fa1cd6e28e8be8391",
	},
	PackageDigest: "sha256:0c43dfbf565c959ad627a6cd8d19aa77bf56d9e3655f44f71bb207fb79b264f2",
}

func TestVersionedClientUsesDiscoveryExactIdentityAndMutationFences(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	var mu sync.Mutex
	requests := []struct {
		method, path, ifMatch, ifNone, idempotency string
	}{}
	resource := Resource{
		APIVersion: APIVersion, Kind: KindObjectBucket, Form: &exactObjectBucketFixture,
		Metadata: Metadata{Name: "assets", Space: "prod", ResourceVersion: "1"},
		Spec:     map[string]any{"name": "assets", "storageClass": "standard"},
		Status:   &Status{Phase: "Ready", ObservedGeneration: 1, Portability: "portable"},
		ID:       "tkrn:prod:ObjectBucket:assets",
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
			if desired.Metadata.ManagedBy != "" || !sameForm(desired.Form, &exactObjectBucketFixture) {
				t.Errorf("preview leaked manager or changed FormRef: %#v", desired)
			}
			_ = json.NewEncoder(w).Encode(PreviewResourceResult{
				Resource: desired, Review: PreviewReview{PlanDigest: "sha256:plan", SpecDigest: "sha256:spec"},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/ObjectBucket/assets":
			var apply applyResourceBody
			if err := json.NewDecoder(r.Body).Decode(&apply); err != nil {
				t.Fatal(err)
			}
			if apply.Review.PlanDigest != "sha256:plan" || !sameForm(apply.Form, &exactObjectBucketFixture) {
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
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource, "import": map[string]any{"summary": "imported"}})
		case r.Method == http.MethodGet && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/ObjectBucket/assets":
			assertExactQuery(t, r, exactObjectBucketFixture)
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(resource)
		case r.Method == http.MethodPost && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/ObjectBucket/assets/observe":
			assertExactQuery(t, r, exactObjectBucketFixture)
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource, "observation": map[string]any{"status": "current", "summary": "current"}})
		case r.Method == http.MethodPost && r.URL.Path == "/apis/forms.takoform.com/v1alpha1/resources/ObjectBucket/assets/refresh":
			assertExactQuery(t, r, exactObjectBucketFixture)
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource, "refresh": map[string]any{"summary": "refreshed"}})
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
		APIVersion: APIVersion, Kind: KindObjectBucket, Form: &exactObjectBucketFixture,
		Metadata: Metadata{Name: "assets", Space: "prod"},
		Spec:     map[string]any{"name": "assets", "storageClass": "standard"},
	}
	if _, err := client.ImportResource(context.Background(), KindObjectBucket, "assets", "native-assets", desired); err != nil {
		t.Fatal(err)
	}
	applied, err := client.PutResource(context.Background(), KindObjectBucket, "assets", desired)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Metadata.ResourceVersion != "1" {
		t.Fatalf("resourceVersion = %q", applied.Metadata.ResourceVersion)
	}
	if _, err := client.GetResource(context.Background(), KindObjectBucket, "assets", "prod", exactObjectBucketFixture); err != nil {
		t.Fatal(err)
	}
	fence := MutationFence{ResourceVersion: "1", Form: exactObjectBucketFixture}
	if _, err := client.ObserveResource(context.Background(), KindObjectBucket, "assets", "prod", fence); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RefreshResource(context.Background(), KindObjectBucket, "assets", "prod", fence); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteResource(context.Background(), KindObjectBucket, "assets", "prod", fence); err != nil {
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
	if err := client.DeleteResource(context.Background(), KindObjectBucket, "assets", "prod", fence); err != nil {
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
	err := client.DeleteResource(context.Background(), KindObjectBucket, "assets", "prod", fence)
	if err == nil || attempts != 1 {
		t.Fatalf("conflict err=%v attempts=%d", err, attempts)
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
	} {
		client := New(endpoint, "", nil)
		if _, err := client.Discover(context.Background()); err == nil {
			t.Fatalf("Discover(%q) unexpectedly succeeded", endpoint)
		}
	}
}

func TestCaptureResourceVersionRejectsMissingInvalidAndConflictingFences(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, bodyVersion, etag string
		wantError               bool
	}{
		{name: "body", bodyVersion: "2"},
		{name: "etag", etag: `"2"`},
		{name: "matching", bodyVersion: "2", etag: `"2"`},
		{name: "missing", wantError: true},
		{name: "invalid body", bodyVersion: "rv-2", wantError: true},
		{name: "unquoted etag", etag: "2", wantError: true},
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

func TestExactInstalledFormReferenceValidationFailsClosed(t *testing.T) {
	t.Parallel()
	for _, mutate := range []func(*InstalledFormReference){
		func(form *InstalledFormReference) { form.FormRef.APIVersion = "forms.takoform.com/v0" },
		func(form *InstalledFormReference) { form.FormRef.Kind = KindQueue },
		func(form *InstalledFormReference) { form.FormRef.DefinitionVersion = "" },
		func(form *InstalledFormReference) { form.FormRef.SchemaDigest = "sha256:not-a-digest" },
		func(form *InstalledFormReference) { form.PackageDigest = "" },
	} {
		form := exactObjectBucketFixture
		mutate(&form)
		if err := validateInstalledFormReference(KindObjectBucket, form); err == nil {
			t.Fatalf("invalid FormRef unexpectedly passed: %#v", form)
		}
	}
}

func writeVersionedDiscovery(t *testing.T, w http.ResponseWriter, origin string) {
	t.Helper()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"api_versions": []string{APIVersion},
		"features":     map[string]bool{"service_forms": true, "exact_form_ref": true, "optimistic_concurrency": true, "idempotent_lifecycle": true},
		"endpoints": map[string]string{
			"api":               origin + "/apis/forms.takoform.com/v1alpha1",
			"forms":             origin + "/apis/forms.takoform.com/v1alpha1/forms",
			"compatibility_api": origin + "/v1",
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
