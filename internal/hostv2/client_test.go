package hostv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type testNamedString string

func fixtureDiscovery(baseURL string, offerings bool) Discovery {
	return Discovery{
		API:           APIVersion,
		BaseURL:       baseURL,
		Documentation: "https://host.example/docs",
		Authentication: Authentication{
			Schemes:       []string{"Bearer"},
			Documentation: "https://host.example/auth",
		},
		Capabilities: Capabilities{Offerings: offerings},
		Limits:       Limits{MaxRequestBytes: 65536, MaxPageSize: 50, ReplayWindowSeconds: 3600},
	}
}

func serveDiscovery(t *testing.T, w http.ResponseWriter, baseURL string, offerings bool) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(fixtureDiscovery(baseURL, offerings)); err != nil {
		t.Fatal(err)
	}
}

func TestClientDiscoversV2AndUsesTheAdvertisedBaseURL(t *testing.T) {
	var seenSupport atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case DiscoveryPath:
			serveDiscovery(t, w, server.URL+"/custom/api", false)
		case "/custom/api/support":
			seenSupport.Add(1)
			if r.URL.Query().Get("form") != "https://forms.example/Widget/1.0.0/" {
				t.Errorf("unexpected support query: %q", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"form":"https://forms.example/Widget/1.0.0/","supported":true,"operations":["create","read","update","delete"],"privateInputs":false}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "test-token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	support, err := client.GetSupport(context.Background(), "https://forms.example/Widget/1.0.0/")
	if err != nil {
		t.Fatalf("GetSupport() error = %v", err)
	}
	if !support.Supported || seenSupport.Load() != 1 {
		t.Fatalf("support = %#v, calls = %d", support, seenSupport.Load())
	}
}

func TestGetSupportRejectsEmptyFormQueryAndFragmentDelimitersBeforeHTTP(t *testing.T) {
	for _, form := range []string{
		"https://forms.example/Widget/1.0.0/?",
		"https://forms.example/Widget/1.0.0/#",
	} {
		t.Run(form[len("https://forms.example/Widget/1.0.0/"):], func(t *testing.T) {
			var apiRequests atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					serveDiscovery(t, w, server.URL+"/api", false)
					return
				}
				apiRequests.Add(1)
				http.NotFound(w, r)
			}))
			defer server.Close()

			client := New(server.URL, "token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatalf("Discover() error = %v", err)
			}
			if _, err := client.GetSupport(context.Background(), form); err == nil {
				t.Fatal("GetSupport() accepted a Form URL with an empty query or fragment delimiter")
			}
			if got := apiRequests.Load(); got != 0 {
				t.Fatalf("API requests for invalid Form URL = %d, want 0", got)
			}
		})
	}
}

func TestDiscoveryRejectsForeignOriginAndV1Documents(t *testing.T) {
	for name, body := range map[string]func(string) Discovery{
		"foreign origin": func(string) Discovery { return fixtureDiscovery("https://elsewhere.example/api", false) },
		"v1 only": func(origin string) Discovery {
			d := fixtureDiscovery(origin+"/apis/forms.takoform.com/v1", false)
			d.API = "forms.takoform.com/v1"
			return d
		},
	} {
		t.Run(name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				d := body(server.URL)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(d)
			}))
			defer server.Close()
			client := New(server.URL, "secret-value", server.Client())
			if _, err := client.Discover(context.Background()); err == nil {
				t.Fatal("Discover() unexpectedly accepted invalid discovery")
			}
		})
	}
}

func TestDiscoveryRejectsEmptyQueryAndFragmentDelimitersInBaseURL(t *testing.T) {
	for _, suffix := range []string{"?", "#"} {
		t.Run("delimiter_"+suffix, func(t *testing.T) {
			var apiCalls atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					serveDiscovery(t, w, server.URL+"/api"+suffix, false)
					return
				}
				apiCalls.Add(1)
			}))
			defer server.Close()
			client := New(server.URL, "token", server.Client())
			if _, err := client.Discover(context.Background()); err == nil {
				t.Fatal("Discover() accepted a baseUrl ending in an empty query or fragment delimiter")
			}
			if apiCalls.Load() != 0 {
				t.Fatalf("authenticated API calls after invalid discovery = %d", apiCalls.Load())
			}
		})
	}
}

func TestDiscoveryRejectsNullRequiredCapability(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			_, _ = io.WriteString(w, `{"api":"forms.takoform.com/v2","baseUrl":"`+server.URL+`/api","documentation":"https://host.example/docs","authentication":{"schemes":["Bearer"],"documentation":"https://host.example/auth"},"capabilities":{"offerings":null,"previews":false,"privateInputs":false},"limits":{"maxRequestBytes":65536,"maxPageSize":50,"replayWindowSeconds":3600}}`)
			return
		}
		t.Errorf("authenticated API request sent after malformed discovery: %s", r.URL)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err == nil {
		t.Fatal("Discover() accepted null for required capabilities.offerings")
	}
}

func TestAuthenticatedCallsRequireAdvertisedBearerScheme(t *testing.T) {
	var apiCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			discovery := fixtureDiscovery(server.URL+"/api", false)
			discovery.Authentication.Schemes = []string{"Basic"}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(discovery)
			return
		}
		apiCalls.Add(1)
	}))
	defer server.Close()
	client := New(server.URL, "configured-token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetSupport(context.Background(), "https://forms.example/Widget/1.0.0/"); err == nil {
		t.Fatal("GetSupport() sent an authenticated request without an advertised Bearer scheme")
	}
	if apiCalls.Load() != 0 {
		t.Fatalf("authenticated API calls = %d, want 0", apiCalls.Load())
	}
}

func TestEmptyBearerCredentialCannotUseBasicOrNoneDiscovery(t *testing.T) {
	for _, scheme := range []string{"Basic", "None"} {
		t.Run(scheme, func(t *testing.T) {
			var apiCalls atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					discovery := fixtureDiscovery(server.URL+"/api", false)
					discovery.Authentication.Schemes = []string{scheme}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(discovery)
					return
				}
				apiCalls.Add(1)
			}))
			defer server.Close()
			client := New(server.URL, "", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, err := client.CreateResource(context.Background(), "intent-key-00000001", CreateRequest{
				Form: "https://forms.example/Widget/1.0.0/", Space: "team-a", Name: "widget", Spec: map[string]any{},
			})
			if err == nil || apiCalls.Load() != 0 {
				t.Fatalf("CreateResource() error = %v, API calls = %d", err, apiCalls.Load())
			}
		})
	}
}

func TestOfferingSelectionIsExplicitAndGenerationFenceIsSent(t *testing.T) {
	var server *httptest.Server
	var createCalls atomic.Int32
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case DiscoveryPath:
			serveDiscovery(t, w, server.URL+"/api", true)
		case "/api/offerings":
			if r.URL.Query().Get("form") != "https://forms.example/Widget/1.0.0/" || r.URL.Query().Get("space") != "team-a" {
				t.Errorf("unexpected offering query: %q", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"items":[{"id":"small","revision":"rev-7","form":"https://forms.example/Widget/1.0.0/","label":"Small","description":"Small host"}],"nextCursor":null}`)
		case "/api/resources":
			createCalls.Add(1)
			if r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") != "intent-key-00000001" {
				t.Errorf("unexpected create method/header: %s %q", r.Method, r.Header.Get("Idempotency-Key"))
			}
			var body CreateRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Offering == nil || body.Offering.ID != "small" || body.Offering.Revision != "rev-7" {
				t.Errorf("offering not explicitly selected: %#v", body.Offering)
			}
			w.Header().Set("Location", server.URL+"/api/operations/op-1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"op-1","resourceUid":"r-1","action":"create","generation":1,"status":"succeeded","effect":"complete","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`)
		case "/api/resources/r-1":
			if r.Method != http.MethodPut || r.Header.Get("Idempotency-Key") != "intent-key-00000002" || r.Header.Get("Takoform-Expected-Generation") != "4" {
				t.Errorf("unexpected update method/headers: %s %v", r.Method, r.Header)
			}
			var body UpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Spec["size"] != "medium" {
				t.Errorf("update did not carry the caller's replacement spec: %#v, %v", body, err)
			}
			w.Header().Set("Location", server.URL+"/api/operations/op-2")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"op-2","resourceUid":"r-1","action":"update","generation":5,"status":"succeeded","effect":"complete","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	page, err := client.ListOfferings(context.Background(), "https://forms.example/Widget/1.0.0/", "team-a", PageOptions{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListOfferings() = %#v, %v", page, err)
	}
	_, err = client.CreateResource(context.Background(), "intent-key-00000001", CreateRequest{
		Form: "https://forms.example/Widget/1.0.0/", Space: "team-a", Name: "widget", Spec: map[string]any{"size": "small"},
		Offering: &OfferingSelection{ID: "small", Revision: "rev-7"},
	})
	if err != nil {
		t.Fatalf("CreateResource() error = %v", err)
	}
	_, err = client.UpdateResource(context.Background(), "r-1", "intent-key-00000002", 4, UpdateRequest{Spec: map[string]any{"size": "medium"}})
	if err != nil {
		t.Fatalf("UpdateResource() error = %v", err)
	}
	if createCalls.Load() != 1 {
		t.Fatalf("create calls = %d", createCalls.Load())
	}
}

func TestMutationRetryAfterTransportParsingPreservesMaxInt64AndExcludesJSON(t *testing.T) {
	const retryAfter = "9223372036854775807"
	var mutationCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		mutationCalls.Add(1)
		if r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") != "intent-key-00000001" {
			t.Errorf("unexpected mutation request: %s %v", r.Method, r.Header)
		}
		var body CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name != "widget" {
			t.Errorf("unexpected create body: %#v, %v", body, err)
		}
		w.Header().Set("Location", server.URL+"/api/operations/op-1")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", retryAfter)
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"id":"op-1","resourceUid":"r-1","action":"create","generation":1,"status":"queued","effect":"none","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}

	operation, err := client.CreateResource(context.Background(), "intent-key-00000001", CreateRequest{
		Form: "https://forms.example/Widget/1.0.0/", Space: "team-a", Name: "widget", Spec: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CreateResource() error = %v", err)
	}
	if operation.RetryAfterSeconds != 9223372036854775807 {
		t.Fatalf("RetryAfterSeconds = %d, want %s", operation.RetryAfterSeconds, retryAfter)
	}
	if mutationCalls.Load() != 1 {
		t.Fatalf("mutation calls = %d, want exactly one", mutationCalls.Load())
	}

	encoded, err := json.Marshal(operation)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if _, exists := wire["retryAfterSeconds"]; exists || len(wire) != 9 {
		t.Fatalf("serialized Operation contains internal metadata or changed shape: %s", encoded)
	}
}

func TestMutationRetryAfterValidationAndZeroForSettledResponse(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		retryAfter string
		want       int64
		wantError  bool
	}{
		{name: "accepted positive", status: http.StatusAccepted, retryAfter: "17", want: 17},
		{name: "settled default", status: http.StatusOK},
		{name: "accepted missing", status: http.StatusAccepted, wantError: true},
		{name: "accepted zero", status: http.StatusAccepted, retryAfter: "0", wantError: true},
		{name: "accepted negative", status: http.StatusAccepted, retryAfter: "-1", wantError: true},
		{name: "accepted decimal", status: http.StatusAccepted, retryAfter: "1.5", wantError: true},
		{name: "accepted overflow", status: http.StatusAccepted, retryAfter: "9223372036854775808", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var mutationCalls atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					serveDiscovery(t, w, server.URL+"/api", false)
					return
				}
				mutationCalls.Add(1)
				w.Header().Set("Location", server.URL+"/api/operations/op-1")
				w.Header().Set("Content-Type", "application/json")
				if test.retryAfter != "" {
					w.Header().Set("Retry-After", test.retryAfter)
				}
				w.WriteHeader(test.status)
				operationBody := `{"id":"op-1","resourceUid":"r-1","action":"create","generation":1,"status":"queued","effect":"none","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`
				if test.status == http.StatusOK {
					operationBody = `{"id":"op-1","resourceUid":"r-1","action":"create","generation":1,"status":"succeeded","effect":"complete","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`
				}
				_, _ = io.WriteString(w, operationBody)
			}))
			defer server.Close()
			client := New(server.URL, "token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			operation, err := client.CreateResource(context.Background(), "intent-key-00000001", CreateRequest{
				Form: "https://forms.example/Widget/1.0.0/", Space: "team-a", Name: "widget", Spec: map[string]any{},
			})
			if test.wantError {
				var indeterminate *IndeterminateError
				if !errors.As(err, &indeterminate) {
					t.Fatalf("CreateResource() error = %v, want *IndeterminateError", err)
				}
			} else if err != nil || operation.RetryAfterSeconds != test.want {
				t.Fatalf("CreateResource() = %#v, %v; want RetryAfterSeconds %d", operation, err, test.want)
			}
			if mutationCalls.Load() != 1 {
				t.Fatalf("mutation calls = %d, want exactly one", mutationCalls.Load())
			}
		})
	}
}

func TestCreateReplayDoesNotRejectOriginalBodyAfterOfferingCapabilityDrift(t *testing.T) {
	var mutations atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", true)
			return
		}
		mutations.Add(1)
		if r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") != "intent-key-00000001" {
			t.Errorf("unexpected replay method/key: %s %q", r.Method, r.Header.Get("Idempotency-Key"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != `{"form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","spec":{}}` {
			t.Errorf("unexpected replay body: %s (%v)", body, err)
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"about:blank","title":"Rejected","status":400,"code":"invalid_request"}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := client.CreateResource(context.Background(), "intent-key-00000001", CreateRequest{
		Form: "https://forms.example/Widget/1.0.0/", Space: "team-a", Name: "widget", Spec: map[string]any{},
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || mutations.Load() != 1 {
		t.Fatalf("CreateResource() error = %v, mutation calls = %d", err, mutations.Load())
	}
}

func TestSupportRejectsNullRequiredBoolean(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		_, _ = io.WriteString(w, `{"form":"https://forms.example/Widget/1.0.0/","supported":null,"operations":[],"privateInputs":false}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetSupport(context.Background(), "https://forms.example/Widget/1.0.0/"); err == nil {
		t.Fatal("GetSupport() accepted null for required supported flag")
	}
}

func TestSupportRejectsNullPrivateInputsBoolean(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		_, _ = io.WriteString(w, `{"form":"https://forms.example/Widget/1.0.0/","supported":false,"operations":[],"privateInputs":null}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetSupport(context.Background(), "https://forms.example/Widget/1.0.0/"); err == nil {
		t.Fatal("GetSupport() accepted null for required privateInputs flag")
	}
}

func TestOfferingPageRejectsNullRequiredLabel(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", true)
			return
		}
		_, _ = io.WriteString(w, `{"items":[{"id":"small","revision":"rev-1","form":"https://forms.example/Widget/1.0.0/","label":null,"description":"Small"}],"nextCursor":null}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListOfferings(context.Background(), "https://forms.example/Widget/1.0.0/", "team-a", PageOptions{}); err == nil {
		t.Fatal("ListOfferings() accepted null for required label")
	}
}

func TestOfferingPageRejectsNullRequiredDescription(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", true)
			return
		}
		_, _ = io.WriteString(w, `{"items":[{"id":"small","revision":"rev-1","form":"https://forms.example/Widget/1.0.0/","label":"Small","description":null}],"nextCursor":null}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListOfferings(context.Background(), "https://forms.example/Widget/1.0.0/", "team-a", PageOptions{}); err == nil {
		t.Fatal("ListOfferings() accepted null for required description")
	}
}

func TestMutationRedirectDoesNotForwardCredentials(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationCalls.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Errorf("authorization forwarded to redirect target")
		}
	}))
	defer destination.Close()
	var source *httptest.Server
	source = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, source.URL+"/api", false)
			return
		}
		http.Redirect(w, r, destination.URL+"/stolen", http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client := New(source.URL, "sensitive-token", source.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := client.CreateResource(context.Background(), "intent-key-00000001", CreateRequest{
		Form: "https://forms.example/Widget/1.0.0/", Space: "team-a", Name: "widget", Spec: map[string]any{},
	})
	if err == nil || destinationCalls.Load() != 0 {
		t.Fatalf("CreateResource() error = %v, redirect target calls = %d", err, destinationCalls.Load())
	}
	if strings.Contains(err.Error(), "sensitive-token") {
		t.Fatalf("error leaked credential: %v", err)
	}
}

func TestLostMutationResponseIsIndeterminateAndNeverRetried(t *testing.T) {
	var mutationCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		mutationCalls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer does not support connection hijacking")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := client.CreateResource(context.Background(), "intent-key-00000001", CreateRequest{
		Form: "https://forms.example/Widget/1.0.0/", Space: "team-a", Name: "widget", Spec: map[string]any{},
	})
	var indeterminate *IndeterminateError
	if !errors.As(err, &indeterminate) {
		t.Fatalf("CreateResource() error = %T %v, want *IndeterminateError", err, err)
	}
	if mutationCalls.Load() != 1 {
		t.Fatalf("mutation calls = %d, want exactly one", mutationCalls.Load())
	}
	if strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("error contains sensitive/raw transport detail: %v", err)
	}
}

func TestMalformedDiscoveryDoesNotSendAuthenticatedAPIRequests(t *testing.T) {
	var authenticatedCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			_, _ = io.WriteString(w, `{"api":"forms.takoform.com/v2","api":"forms.takoform.com/v1"}`)
			return
		}
		authenticatedCalls.Add(1)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err == nil {
		t.Fatal("Discover() accepted duplicate/malformed JSON")
	}
	if authenticatedCalls.Load() != 0 {
		t.Fatalf("authenticated API calls = %d", authenticatedCalls.Load())
	}
}

func TestResourceAndOperationReadsAndDeleteUseV2Routes(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case DiscoveryPath:
			serveDiscovery(t, w, server.URL+"/api", false)
		case "/api/resources/r-1":
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"uid":"r-1","form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","generation":3,"observedGeneration":0,"observedAt":null,"phase":"pending","spec":{"size":"large"},"observed":{},"output":{},"lastOperation":"op-3"}`)
				return
			}
			if r.Method != http.MethodDelete || r.Header.Get("Takoform-Expected-Generation") != "3" || r.Header.Get("Idempotency-Key") != "intent-key-00000003" {
				t.Errorf("unexpected delete request: %s %v", r.Method, r.Header)
			}
			w.Header().Set("Location", server.URL+"/api/operations/op-4")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"op-4","resourceUid":"r-1","action":"delete","generation":4,"status":"succeeded","effect":"complete","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`)
		case "/api/operations/op-3":
			if r.Method != http.MethodGet {
				t.Errorf("operation route method = %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"op-3","resourceUid":"r-1","action":"update","generation":3,"status":"reconciling","effect":"unknown","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	resource, err := client.ReadResource(context.Background(), "r-1")
	if err != nil || resource.Generation != 3 || resource.ObservedGeneration != 0 || resource.ObservedAt != nil {
		t.Fatalf("ReadResource() = %#v, %v", resource, err)
	}
	operation, err := client.ReadOperation(context.Background(), "op-3")
	if err != nil || operation.Status != "reconciling" || operation.Effect != "unknown" {
		t.Fatalf("ReadOperation() = %#v, %v", operation, err)
	}
	operation, err = client.DeleteResource(context.Background(), "r-1", "intent-key-00000003", 3)
	if err != nil || operation.Generation != 4 {
		t.Fatalf("DeleteResource() = %#v, %v", operation, err)
	}
}

func TestResourceRejectsObservationGenerationWithoutObservationTime(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		_, _ = io.WriteString(w, `{"uid":"r-1","form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","generation":3,"observedGeneration":2,"observedAt":null,"phase":"pending","spec":{},"observed":{},"output":{},"lastOperation":"op-3"}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadResource(context.Background(), "r-1"); err == nil {
		t.Fatal("ReadResource() accepted a verified generation without observedAt")
	}
}

func TestWaitingInputOperationRejectsNullNameElements(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		_, _ = io.WriteString(w, `{"id":"op-1","resourceUid":"r-1","action":"create","generation":1,"status":"waiting_input","effect":"none","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z","inputRequired":{"names":[null],"reason":"expired"}}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadOperation(context.Background(), "op-1"); err == nil {
		t.Fatal("ReadOperation() accepted null instead of a JSON string name")
	}
}

func TestWaitingInputOperationAcceptsFormDefinedStringNamesWithoutCommonGrammar(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		_, _ = io.WriteString(w, `{"id":"op-1","resourceUid":"r-1","action":"create","generation":1,"status":"waiting_input","effect":"none","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z","inputRequired":{"names":["Form-specific key name"],"reason":"unavailable"}}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	operation, err := client.ReadOperation(context.Background(), "op-1")
	if err != nil || operation.InputRequired == nil || len(operation.InputRequired.Names) != 1 || operation.InputRequired.Names[0] != "Form-specific key name" {
		t.Fatalf("ReadOperation() = %#v, %v", operation, err)
	}
}

func TestUpdateOperationMustBelongToRequestedResource(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		w.Header().Set("Location", server.URL+"/api/operations/op-2")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"op-2","resourceUid":"other-resource","action":"update","generation":2,"status":"succeeded","effect":"complete","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := client.UpdateResource(context.Background(), "r-1", "intent-key-00000002", 1, UpdateRequest{Spec: map[string]any{"size": "large"}})
	var indeterminate *IndeterminateError
	if !errors.As(err, &indeterminate) {
		t.Fatalf("UpdateResource() error = %T %v, want *IndeterminateError", err, err)
	}
}

func TestMutationRequiresValidProblemResponseBeforeClassifyingRejection(t *testing.T) {
	tests := []struct {
		name         string
		contentType  string
		body         string
		wantAPIError bool
		wantCode     string
	}{
		{name: "proxy HTML", contentType: "text/html", body: `<html>not found</html>`},
		{name: "malformed problem", contentType: "application/problem+json", body: `{"type":"about:blank","status":404,"code":`},
		{name: "null problem title", contentType: "application/problem+json", body: `{"type":"about:blank","title":null,"status":404,"code":"not_found"}`},
		{name: "valid problem", contentType: "application/problem+json", body: `{"type":"about:blank","title":"Not Found","status":404,"code":"not_found"}`, wantAPIError: true, wantCode: "not_found"},
		{name: "untrusted problem code", contentType: "application/problem+json", body: `{"type":"about:blank","title":"Not Found","status":404,"code":"private-input-secret"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var mutations atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					serveDiscovery(t, w, server.URL+"/api", false)
					return
				}
				mutations.Add(1)
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			client := New(server.URL, "token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, err := client.CreateResource(context.Background(), "intent-key-00000001", CreateRequest{
				Form: "https://forms.example/Widget/1.0.0/", Space: "team-a", Name: "widget", Spec: map[string]any{},
			})
			if mutations.Load() != 1 {
				t.Fatalf("mutation calls = %d, want 1", mutations.Load())
			}
			if test.wantAPIError {
				var apiError *APIError
				if !errors.As(err, &apiError) || apiError.Code != test.wantCode {
					t.Fatalf("CreateResource() error = %#v, want APIError code %q", err, test.wantCode)
				}
				if strings.Contains(err.Error(), apiError.Code) {
					t.Fatalf("CreateResource() diagnostic included host code: %v", err)
				}
			} else {
				var indeterminate *IndeterminateError
				if !errors.As(err, &indeterminate) {
					t.Fatalf("CreateResource() error = %T %v, want *IndeterminateError", err, err)
				}
				if strings.Contains(err.Error(), "private-input-secret") {
					t.Fatalf("CreateResource() error leaked response content: %v", err)
				}
			}
		})
	}
}

func TestContextCancellationStopsRequest(t *testing.T) {
	started := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := client.Discover(ctx); done <- err }()
	<-started
	cancel()
	if err := <-done; err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Discover() error = %v, want context cancellation", err)
	}
}

func TestFailedRediscoveryInvalidatesPreviouslyAcceptedAPIBase(t *testing.T) {
	var discoveryCalls atomic.Int32
	var authenticatedCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			if discoveryCalls.Add(1) == 1 {
				serveDiscovery(t, w, server.URL+"/api", false)
				return
			}
			serveDiscovery(t, w, "https://elsewhere.example/api", false)
			return
		}
		authenticatedCalls.Add(1)
		_, _ = io.WriteString(w, `{"form":"https://forms.example/Widget/1.0.0/","supported":true,"operations":["read"],"privateInputs":false}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Discover(context.Background()); err == nil {
		t.Fatal("Discover() unexpectedly accepted a foreign API base")
	}
	if _, err := client.GetSupport(context.Background(), "https://forms.example/Widget/1.0.0/"); err == nil {
		t.Fatal("GetSupport() used a stale API base after failed rediscovery")
	}
	if authenticatedCalls.Load() != 0 {
		t.Fatalf("authenticated API calls after failed rediscovery = %d, want 0", authenticatedCalls.Load())
	}
}

func TestResponseGenerationsMustBeSafeIntegers(t *testing.T) {
	const unsafe = `9007199254740992`
	for name, routeAndBody := range map[string]struct{ path, body string }{
		"resource":  {"/api/resources/r-1", `{"uid":"r-1","form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","generation":` + unsafe + `,"observedGeneration":0,"observedAt":null,"phase":"pending","spec":{},"observed":{},"output":{},"lastOperation":"op-1"}`},
		"operation": {"/api/operations/op-1", `{"id":"op-1","resourceUid":"r-1","action":"create","generation":` + unsafe + `,"status":"reconciling","effect":"unknown","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`},
	} {
		t.Run(name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					serveDiscovery(t, w, server.URL+"/api", false)
					return
				}
				if r.URL.Path != routeAndBody.path {
					t.Errorf("unexpected request path %q", r.URL.Path)
				}
				_, _ = io.WriteString(w, routeAndBody.body)
			}))
			defer server.Close()
			client := New(server.URL, "token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			var err error
			if name == "resource" {
				_, err = client.ReadResource(context.Background(), "r-1")
			} else {
				_, err = client.ReadOperation(context.Background(), "op-1")
			}
			if err == nil {
				t.Fatalf("accepted generation above the safe integer limit %s", unsafe)
			}
		})
	}
}

func TestResourceReadsPreserveRetainedOfferingAcrossCapabilityDrift(t *testing.T) {
	resource := `{"uid":"r-1","form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","offering":{"id":"small","revision":"rev-1"},"generation":3,"observedGeneration":0,"observedAt":null,"phase":"pending","spec":{},"observed":{},"output":{},"lastOperation":"op-1"}`
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		if r.URL.Path == "/api/resources" {
			_, _ = io.WriteString(w, `{"items":[`+resource+`],"nextCursor":null}`)
			return
		}
		_, _ = io.WriteString(w, resource)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := client.ReadResource(context.Background(), "r-1")
	if err != nil || got.Offering == nil || got.Offering.ID != "small" {
		t.Fatalf("ReadResource() = %#v, %v; retained Offering was lost", got, err)
	}
	page, err := client.ListResources(context.Background(), ResourceFilters{}, PageOptions{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Offering == nil || page.Items[0].Offering.ID != "small" {
		t.Fatalf("ListResources() = %#v, %v; retained Offering was lost", page, err)
	}
}

func TestResourceArbitraryMapsPreserveJSONNumberPrecision(t *testing.T) {
	const precise = `0.12345678901234567890123456789`
	resource := `{"uid":"r-1","form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","generation":3,"observedGeneration":0,"observedAt":null,"phase":"pending","spec":{"precise":` + precise + `},"observed":{"precise":` + precise + `},"output":{"precise":` + precise + `},"lastOperation":"op-1"}`
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		if r.URL.Path == "/api/resources" {
			_, _ = io.WriteString(w, `{"items":[`+resource+`],"nextCursor":null}`)
			return
		}
		_, _ = io.WriteString(w, resource)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, maps ...map[string]any) {
		t.Helper()
		for _, values := range maps {
			if got, ok := values["precise"].(json.Number); !ok || got.String() != precise {
				t.Fatalf("precise value = %#v (%T), want exact json.Number %s", values["precise"], values["precise"], precise)
			}
		}
	}
	item, err := client.ReadResource(context.Background(), "r-1")
	if err != nil {
		t.Fatal(err)
	}
	check(t, item.Spec, item.Observed, item.Output)
	page, err := client.ListResources(context.Background(), ResourceFilters{}, PageOptions{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListResources() = %#v, %v", page, err)
	}
	check(t, page.Items[0].Spec, page.Items[0].Observed, page.Items[0].Output)
}

func TestResourceAllowsTimestampForNegativeObservation(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		_, _ = io.WriteString(w, `{"uid":"r-1","form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","generation":3,"observedGeneration":0,"observedAt":"2026-10-12T12:00:00Z","phase":"idle","spec":{},"observed":{},"output":{},"lastOperation":"op-1"}`)
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	resource, err := client.ReadResource(context.Background(), "r-1")
	if err != nil || resource.ObservedGeneration != 0 || resource.ObservedAt == nil {
		t.Fatalf("ReadResource() = %#v, %v; negative observation timestamp was rejected", resource, err)
	}
}

func TestMutationsReplayExactCallerIntentAcrossCapabilityAndLimitDrift(t *testing.T) {
	const form = "https://forms.example/Widget/1.0.0/"
	resource := `{"uid":"r-1","form":"` + form + `","space":"team-a","name":"widget","generation":3,"observedGeneration":0,"observedAt":null,"phase":"idle","spec":{},"observed":{},"output":{},"lastOperation":"op-1"}`
	const createBody = `{"form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","offering":{"id":"small","revision":"rev-7"},"spec":{"size":"small"},"privateInputs":{"secret":"private-value"}}`
	const updateBody = `{"spec":{"size":"medium"},"privateInputs":{"secret":"private-value"}}`
	var posts, puts, resourceGets, supportGets atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case DiscoveryPath:
			discovery := fixtureDiscovery(server.URL+"/api", false)
			discovery.Capabilities.PrivateInputs = false
			discovery.Limits.MaxRequestBytes = 1
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(discovery)
		case "/api/support":
			supportGets.Add(1)
			_, _ = io.WriteString(w, `{"form":"https://forms.example/Widget/1.0.0/","supported":true,"operations":["create","update"],"privateInputs":false}`)
		case "/api/resources":
			posts.Add(1)
			body, err := io.ReadAll(r.Body)
			if err != nil || r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") != "intent-key-00000001" || string(body) != createBody {
				t.Errorf("create replay changed method/key/body: method=%s key=%q body=%s err=%v", r.Method, r.Header.Get("Idempotency-Key"), body, err)
			}
			writeMutationOperation(w, server.URL, "op-create", "r-1", "create", 1)
		case "/api/resources/r-1":
			if r.Method == http.MethodGet {
				resourceGets.Add(1)
				_, _ = io.WriteString(w, resource)
				return
			}
			puts.Add(1)
			body, err := io.ReadAll(r.Body)
			if r.Method != http.MethodPut || r.Header.Get("Idempotency-Key") != "intent-key-00000002" || r.Header.Get("Takoform-Expected-Generation") != "3" || string(body) != updateBody || err != nil {
				t.Errorf("update replay changed method/key/generation/body: method=%s key=%q generation=%q body=%s err=%v", r.Method, r.Header.Get("Idempotency-Key"), r.Header.Get("Takoform-Expected-Generation"), body, err)
			}
			writeMutationOperation(w, server.URL, "op-update", "r-1", "update", 4)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	secret := map[string]string{"secret": "private-value"}
	if _, err := client.CreateResource(context.Background(), "intent-key-00000001", CreateRequest{
		Form: form, Space: "team-a", Name: "widget", Offering: &OfferingSelection{ID: "small", Revision: "rev-7"},
		Spec: map[string]any{"size": "small"}, PrivateInputs: &secret,
	}); err != nil {
		t.Fatalf("CreateResource() replay error = %v", err)
	}
	if _, err := client.UpdateResource(context.Background(), "r-1", "intent-key-00000002", 3, UpdateRequest{
		Spec: map[string]any{"size": "medium"}, PrivateInputs: &secret,
	}); err != nil {
		t.Fatalf("UpdateResource() replay error = %v", err)
	}
	if posts.Load() != 1 || puts.Load() != 1 || resourceGets.Load() != 0 || supportGets.Load() != 0 {
		t.Fatalf("mutation/preflight counts: POST=%d PUT=%d resource GET=%d support GET=%d", posts.Load(), puts.Load(), resourceGets.Load(), supportGets.Load())
	}
}

func TestContinuePrivateInputsRepeatsEntireOriginalMapOnSameOperation(t *testing.T) {
	const operationID = "op-1"
	const requestBody = `{"privateInputs":{"credential":"same-secret","other":"second-secret"}}`
	var discoveries, supportCalls, continuationCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case DiscoveryPath:
			discoveries.Add(1)
			discovery := fixtureDiscovery(server.URL+"/api", false)
			discovery.Capabilities.PrivateInputs = false
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(discovery)
		case "/api/support":
			supportCalls.Add(1)
			t.Errorf("continuation must not perform fresh Form support lookup")
			http.Error(w, "unexpected support lookup", http.StatusInternalServerError)
		case "/api/operations/" + operationID + "/private-inputs":
			call := continuationCalls.Add(1)
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != requestBody || r.Method != http.MethodPut {
				t.Errorf("continuation changed method or original map: method=%s bodyMatches=%t err=%v", r.Method, string(body) == requestBody, err)
			}
			if r.Header.Get("Idempotency-Key") != "" || r.Header.Get("Takoform-Expected-Generation") != "" {
				t.Errorf("continuation introduced new intent headers: %v", r.Header)
			}
			if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Bearer token" {
				t.Errorf("continuation headers = %v", r.Header)
			}
			if call == 1 {
				writeContinuationOperation(t, w, Operation{
					ID: operationID, ResourceUID: "r-1", Action: "create", Generation: 1,
					Status: "waiting_input", Effect: "none", CreatedAt: "2026-10-12T12:00:00Z",
					UpdatedAt: "2026-10-12T12:00:01Z", RetainUntil: "2026-10-13T12:00:01Z",
					InputRequired: &InputRequired{Names: []string{"credential"}, Reason: "expired"},
				})
				return
			}
			writeContinuationOperation(t, w, Operation{
				ID: operationID, ResourceUID: "r-1", Action: "create", Generation: 1,
				Status: "running", Effect: "none", CreatedAt: "2026-10-12T12:00:00Z",
				UpdatedAt: "2026-10-12T12:00:02Z", RetainUntil: "2026-10-13T12:00:01Z",
			})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	original := Operation{
		ID: operationID, ResourceUID: "r-1", Action: "create", Generation: 1,
		Status: "waiting_input", Effect: "none", CreatedAt: "2026-10-12T12:00:00Z",
		UpdatedAt: "2026-10-12T12:00:01Z", RetainUntil: "2026-10-13T12:00:01Z",
		InputRequired: &InputRequired{Names: []string{"credential"}, Reason: "expired"},
	}
	inputs := map[string]string{"credential": "same-secret", "other": "second-secret"}
	for _, wantStatus := range []string{"waiting_input", "running"} {
		got, err := client.ContinuePrivateInputs(context.Background(), original, inputs)
		if err != nil || got.Status != wantStatus || got.ID != original.ID || got.ResourceUID != original.ResourceUID || got.Action != original.Action || got.Generation != original.Generation {
			t.Fatalf("ContinuePrivateInputs() = %#v, %v; want same identity in %s", got, err, wantStatus)
		}
		encoded, err := json.Marshal(got)
		if err != nil || strings.Contains(string(encoded), "same-secret") || strings.Contains(string(encoded), "second-secret") {
			t.Fatalf("returned Operation retained private input: %s (%v)", encoded, err)
		}
	}
	if discoveries.Load() != 1 || supportCalls.Load() != 0 || continuationCalls.Load() != 2 {
		t.Fatalf("continuation calls: discovery=%d support=%d PUT=%d", discoveries.Load(), supportCalls.Load(), continuationCalls.Load())
	}
}

func writeContinuationOperation(t *testing.T, w http.ResponseWriter, operation Operation) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(operation); err != nil {
		t.Errorf("encode continuation Operation: %v", err)
	}
}

func TestContinuePrivateInputsKeepsUnverifiableRefusalAndValidatedOperationID(t *testing.T) {
	for name, test := range map[string]struct {
		body            string
		wantOperationID string
	}{
		"host supplied known id": {body: `{"type":"about:blank","title":"Conflict","status":409,"code":"private_inputs_unverifiable","operationId":"op-1","detail":"same-secret"}`, wantOperationID: "op-1"},
		"host omitted id":        {body: `{"type":"about:blank","title":"Conflict","status":409,"code":"private_inputs_unverifiable","detail":"same-secret"}`},
	} {
		t.Run(name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					serveDiscovery(t, w, server.URL+"/api", false)
					return
				}
				if r.Method != http.MethodPut || r.URL.Path != "/api/operations/op-1/private-inputs" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			client := New(server.URL, "token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			original := continuationOriginal()
			_, err := client.ContinuePrivateInputs(context.Background(), original, map[string]string{"credential": "same-secret"})
			var apiError *APIError
			if !errors.As(err, &apiError) || apiError.Code != "private_inputs_unverifiable" || apiError.OperationID != test.wantOperationID {
				t.Fatalf("ContinuePrivateInputs() error = %#v; want unverifiable APIError OperationID %q", err, test.wantOperationID)
			}
			if strings.Contains(err.Error(), "same-secret") || strings.Contains(err.Error(), "Conflict") {
				t.Fatalf("continuation error exposed private input or Host detail: %v", err)
			}
		})
	}
}

func TestContinuePrivateInputsRejectsWrongOperationIDInProblem(t *testing.T) {
	for name, operationID := range map[string]string{
		"foreign valid id": `"foreign-operation"`,
		"invalid id":       `"../op-1"`,
		"wrong JSON type":  `7`,
		"null":             `null`,
	} {
		t.Run(name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					serveDiscovery(t, w, server.URL+"/api", false)
					return
				}
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"type":"about:blank","title":"Conflict","status":409,"code":"private_inputs_unverifiable","operationId":`+operationID+`}`)
			}))
			defer server.Close()
			client := New(server.URL, "token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, err := client.ContinuePrivateInputs(context.Background(), continuationOriginal(), map[string]string{"credential": "same-secret"})
			var apiError *APIError
			var indeterminate *IndeterminateError
			if errors.As(err, &apiError) || !errors.As(err, &indeterminate) {
				t.Fatalf("ContinuePrivateInputs() error = %#v; malformed/foreign Operation ID must not be trusted", err)
			}
		})
	}
}

func TestContinuePrivateInputsDoesNotAcceptTerminalOrMalformedAcknowledgments(t *testing.T) {
	validOperation := `{"id":"op-1","resourceUid":"r-1","action":"create","generation":1,"status":"succeeded","effect":"complete","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`
	tests := map[string]string{
		"terminal operation": validOperation,
		"unknown status":     strings.Replace(validOperation, `"succeeded"`, `"accepted"`, 1),
		"foreign operation":  strings.Replace(validOperation, `"op-1"`, `"op-2"`, 1),
		"foreign resource":   strings.Replace(validOperation, `"r-1"`, `"other-resource"`, 1),
		"wrong action":       strings.Replace(validOperation, `"create"`, `"update"`, 1),
		"wrong generation":   strings.Replace(validOperation, `"generation":1`, `"generation":2`, 1),
		"malformed JSON":     `{"id":"op-1","resourceUid":`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					serveDiscovery(t, w, server.URL+"/api", false)
					return
				}
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			client := New(server.URL, "token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			got, err := client.ContinuePrivateInputs(context.Background(), continuationOriginal(), map[string]string{"credential": "same-secret"})
			var indeterminate *IndeterminateError
			if err == nil || !errors.As(err, &indeterminate) || got.ID != "" || requests.Load() != 1 {
				t.Fatalf("ContinuePrivateInputs() = %#v, %v; requests=%d; want non-success for one accepted request", got, err, requests.Load())
			}
			if strings.Contains(err.Error(), "same-secret") {
				t.Fatalf("continuation error exposed private input: %v", err)
			}
		})
	}
}

func TestContinuePrivateInputsRefusesForeignRedirect(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationCalls.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Errorf("authorization reached foreign redirect target")
		}
	}))
	defer destination.Close()
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		http.Redirect(w, r, destination.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client := New(server.URL, "sensitive-token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := client.ContinuePrivateInputs(context.Background(), continuationOriginal(), map[string]string{"credential": "same-secret"})
	var indeterminate *IndeterminateError
	if !errors.As(err, &indeterminate) || destinationCalls.Load() != 0 {
		t.Fatalf("ContinuePrivateInputs() error=%v foreign destination calls=%d", err, destinationCalls.Load())
	}
	if strings.Contains(err.Error(), "sensitive-token") || strings.Contains(err.Error(), "same-secret") {
		t.Fatalf("continuation error exposed sensitive data: %v", err)
	}
}

func TestContinuePrivateInputsRejectsInvalidLocalIntentBeforeHTTP(t *testing.T) {
	invalid := string([]byte{0xff})
	invalidInputs := map[string]string{"credential": invalid}
	tests := map[string]struct {
		original Operation
		inputs   map[string]string
	}{
		"nil entire map":      {original: continuationOriginal()},
		"invalid UTF-8 value": {original: continuationOriginal(), inputs: invalidInputs},
		"terminal operation":  {original: Operation{ID: "op-1", ResourceUID: "r-1", Action: "create", Generation: 1, Status: "succeeded"}, inputs: map[string]string{"credential": "same-secret"}},
		"unknown operation":   {original: Operation{ID: "op-1", ResourceUID: "r-1", Action: "create", Generation: 1, Status: "unknown"}, inputs: map[string]string{"credential": "same-secret"}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var apiCalls atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					serveDiscovery(t, w, server.URL+"/api", false)
					return
				}
				apiCalls.Add(1)
			}))
			defer server.Close()
			client := New(server.URL, "token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, err := client.ContinuePrivateInputs(context.Background(), test.original, test.inputs)
			if err == nil || apiCalls.Load() != 0 {
				t.Fatalf("ContinuePrivateInputs() error=%v API calls=%d; want local refusal", err, apiCalls.Load())
			}
			if strings.Contains(err.Error(), invalid) || strings.Contains(err.Error(), "same-secret") {
				t.Fatalf("local error exposed private input: %v", err)
			}
		})
	}
}

func TestContinuePrivateInputsUsesCallerCancellationWithoutRetry(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DiscoveryPath {
			serveDiscovery(t, w, server.URL+"/api", false)
			return
		}
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
	}))
	defer server.Close()
	client := New(server.URL, "token", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.ContinuePrivateInputs(ctx, continuationOriginal(), map[string]string{"credential": "same-secret"})
		done <- err
	}()
	<-started
	cancel()
	err := <-done
	close(release)
	var indeterminate *IndeterminateError
	if !errors.As(err, &indeterminate) || calls.Load() != 1 {
		t.Fatalf("ContinuePrivateInputs() error=%v calls=%d; want one indeterminate canceled request", err, calls.Load())
	}
}

func continuationOriginal() Operation {
	return Operation{
		ID: "op-1", ResourceUID: "r-1", Action: "create", Generation: 1,
		Status: "waiting_input", Effect: "none", CreatedAt: "2026-10-12T12:00:00Z",
		UpdatedAt: "2026-10-12T12:00:01Z", RetainUntil: "2026-10-13T12:00:01Z",
		InputRequired: &InputRequired{Names: []string{"credential"}, Reason: "expired"},
	}
}

func writeMutationOperation(w http.ResponseWriter, origin, id, uid, action string, generation int64) {
	w.Header().Set("Location", origin+"/api/operations/"+id)
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"id":%q,"resourceUid":%q,"action":%q,"generation":%d,"status":"succeeded","effect":"complete","createdAt":"2026-10-12T12:00:00Z","updatedAt":"2026-10-12T12:00:01Z","retainUntil":"2026-10-13T12:00:01Z"}`, id, uid, action, generation)
}

func TestMutationRejectsInvalidUTF8InputBeforeSending(t *testing.T) {
	invalid := string([]byte{0xff})
	invalidPrivateInputs := map[string]string{"secret": invalid}
	for name, input := range map[string]struct {
		spec          map[string]any
		privateInputs *map[string]string
	}{
		"nested string": {spec: map[string]any{"nested": map[string]any{"value": invalid}}},
		"map key":       {spec: map[string]any{invalid: "value"}},
		"named string":  {spec: map[string]any{"value": testNamedString(invalid)}},
		"raw JSON":      {spec: map[string]any{"value": json.RawMessage([]byte{'"', 0xff, '"'})}},
		"private input": {spec: map[string]any{}, privateInputs: &invalidPrivateInputs},
	} {
		t.Run(name, func(t *testing.T) {
			var mutations atomic.Int32
			var supportCalls atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == DiscoveryPath {
					discovery := fixtureDiscovery(server.URL+"/api", false)
					discovery.Capabilities.PrivateInputs = true
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(discovery)
					return
				}
				if r.Method == http.MethodPost || r.Method == http.MethodPut {
					mutations.Add(1)
				}
				if r.URL.Path == "/api/support" {
					supportCalls.Add(1)
				}
				_, _ = io.WriteString(w, `{"form":"https://forms.example/Widget/1.0.0/","supported":true,"operations":["create","update"],"privateInputs":true}`)
			}))
			defer server.Close()
			client := New(server.URL, "token", server.Client())
			if _, err := client.Discover(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, err := client.CreateResource(context.Background(), "intent-key-00000001", CreateRequest{
				Form: "https://forms.example/Widget/1.0.0/", Space: "team-a", Name: "widget", Spec: input.spec, PrivateInputs: input.privateInputs,
			})
			if err == nil || mutations.Load() != 0 || supportCalls.Load() != 0 {
				t.Fatalf("CreateResource() error=%v, support calls=%d, mutation calls=%d", err, supportCalls.Load(), mutations.Load())
			}
			if strings.Contains(err.Error(), invalid) {
				t.Fatalf("error echoed invalid caller input: %v", err)
			}
		})
	}
}

var _ = strings.Contains
