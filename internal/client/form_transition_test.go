package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

var transitionOldDatabaseForm = InstalledFormReference{
	FormRef: FormRef{
		APIVersion: APIVersion, Kind: "RelationalDatabase", DefinitionVersion: "2.0.0",
		SchemaDigest: "sha256:3898f8ee507bcebd9e03e80fbc1931b67b477299b1ebe2ff395facb7acf018de",
	},
	PackageDigest: "sha256:dc131e4858ddedbb84d553fdf7808c55fc898a37f15d84839e414fe3ca57c910",
}

var transitionNewDatabaseForm = InstalledFormReference{
	FormRef: FormRef{
		APIVersion: APIVersion, Kind: "RelationalDatabase", DefinitionVersion: "3.0.0",
		SchemaDigest: "sha256:e4c7aedb5962e6b719d7afe7a8f002ceb00ae4a1c74ebfc1eff712e257bf4044",
	},
	PackageDigest: "sha256:599e60e4f3a5b735c58f8ff5029f72b5a25445be6f317816590eca12b44e5a31",
}

func TestDatabaseTransitionEvidenceDigestMatchesHostContract(t *testing.T) {
	digest, err := TransitionEvidenceDigest(
		relationalDatabaseV2ToV3TransitionEvidenceMarker,
		transitionOldDatabaseForm,
		transitionNewDatabaseForm,
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := "sha256:7106e4a5ea37f0295b9406fccc8a6f5230b2ec92cb1f629b1fc243c99aeedbe7"; digest != want {
		t.Fatalf("transition evidence digest = %q, want host-locked %q", digest, want)
	}
}

func TestDatabaseTransitionOperationFixtureMatchesHostContract(t *testing.T) {
	request := canonicalTransitionRequest(t)
	var fixture struct {
		Format             string                 `json:"format"`
		Resource           formTransitionResource `json:"resource"`
		FromForm           InstalledFormReference `json:"fromForm"`
		ToForm             InstalledFormReference `json:"toForm"`
		DesiredSpec        map[string]any         `json:"desiredSpec"`
		Expected           FormTransitionExpected `json:"expected"`
		TransitionEvidence FormTransitionEvidence `json:"transitionEvidence"`
		DesiredSpecDigest  string                 `json:"desiredSpecDigest"`
		OperationID        string                 `json:"operationId"`
		RequestDigest      string                 `json:"requestDigest"`
	}
	raw, err := os.ReadFile("testdata/form-transition-rfc8785.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Format != "takoform.form-transition-rfc8785-fixture@v1" ||
		fixture.Resource != (formTransitionResource{Space: "prod", Kind: "RelationalDatabase", Name: "app"}) ||
		fixture.FromForm != request.FromForm || fixture.ToForm != request.ToForm ||
		!reflect.DeepEqual(fixture.DesiredSpec, request.Resource.Spec) ||
		request.Expected == nil || !reflect.DeepEqual(fixture.Expected, *request.Expected) ||
		fixture.TransitionEvidence != request.TransitionEvidence {
		t.Fatalf("shared host fixture does not describe the canonical request: %#v", fixture)
	}
	if want := fixture.OperationID; request.OperationID != want {
		t.Fatalf("operationId = %q, want host-locked %q", request.OperationID, want)
	}
	if got, want := mustSpecDigest(t, request.Resource.Spec), fixture.DesiredSpecDigest; got != want {
		t.Fatalf("desiredSpecDigest = %q, want host-locked %q", got, want)
	}
	if got, want := mustTransitionRequestDigest(t, request), fixture.RequestDigest; got != want {
		t.Fatalf("requestDigest = %q, want host-locked %q", got, want)
	}
}

func TestFormTransitionSendsOneExactBoundRequestAndAcceptsExactProof(t *testing.T) {
	request := canonicalTransitionRequest(t)
	wantDigest, err := TransitionEvidenceDigest(
		relationalDatabaseV2ToV3TransitionEvidenceMarker,
		transitionOldDatabaseForm,
		transitionNewDatabaseForm,
	)
	if err != nil {
		t.Fatal(err)
	}
	var posts atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/.well-known/takoform":
			writeTransitionDiscovery(t, w, server.URL, true)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/form-transitions/"+request.OperationID):
			writeTransitionOperationAbsent(t, w)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/resources/RelationalDatabase/app/form-transitions"):
			posts.Add(1)
			if r.URL.Query().Get("space") != "prod" {
				t.Errorf("space query = %q", r.URL.Query().Get("space"))
			}
			var got FormTransitionRequest
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if r.Header.Get("Idempotency-Key") != got.OperationID || !reflect.DeepEqual(got, request) {
				t.Fatalf("transition binding header=%q request=%#v, want %#v", r.Header.Get("Idempotency-Key"), got, request)
			}
			w.Header().Set("ETag", `"8"`)
			_ = json.NewEncoder(w).Encode(committedTransitionResponse(t, request, wantDigest, "8"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	formClient := discoveredTransitionClient(t, server)
	response, err := formClient.TransitionResourceForm(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 {
		t.Fatalf("POST requests = %d, want exactly 1", posts.Load())
	}
	if response.TransitionProof.ObservedSpecDigest != mustSpecDigest(t, request.Resource.Spec) {
		t.Fatal("client accepted proof without the exact desired spec digest")
	}
}

func TestFormTransitionLostAckReconcilesWithGetAndNeverReposts(t *testing.T) {
	request := canonicalTransitionRequest(t)
	digest, err := TransitionEvidenceDigest(
		relationalDatabaseV2ToV3TransitionEvidenceMarker,
		transitionOldDatabaseForm,
		transitionNewDatabaseForm,
	)
	if err != nil {
		t.Fatal(err)
	}
	var posts atomic.Int64
	var gets atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/.well-known/takoform":
			writeTransitionDiscovery(t, w, server.URL, true)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/form-transitions/"+request.OperationID) && posts.Load() == 0:
			gets.Add(1)
			writeTransitionOperationAbsent(t, w)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/form-transitions"):
			posts.Add(1)
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("test server cannot drop transport")
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = connection.Close()
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/form-transitions/"+request.OperationID):
			gets.Add(1)
			w.Header().Set("ETag", `"8"`)
			_ = json.NewEncoder(w).Encode(committedTransitionResponse(t, request, digest, "8"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	formClient := discoveredTransitionClient(t, server)
	if _, err := formClient.TransitionResourceForm(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 || gets.Load() != 2 {
		t.Fatalf("POST=%d GET=%d, want one preflight, one mutation, then one readback", posts.Load(), gets.Load())
	}
}

func TestFormTransitionLostAckReturnsIndeterminateWithoutBlindRetry(t *testing.T) {
	request := canonicalTransitionRequest(t)
	var posts atomic.Int64
	var gets atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/.well-known/takoform":
			writeTransitionDiscovery(t, w, server.URL, true)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/form-transitions/") && posts.Load() == 0:
			gets.Add(1)
			writeTransitionOperationAbsent(t, w)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/form-transitions"):
			posts.Add(1)
			hijacker := w.(http.Hijacker)
			connection, _, _ := hijacker.Hijack()
			_ = connection.Close()
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/form-transitions/"):
			gets.Add(1)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(FormTransitionResponse{Operation: FormTransitionOperation{
				OperationID: request.OperationID, Status: "indeterminate",
				RequestDigest:     mustTransitionRequestDigest(t, request),
				ReconcilePath:     "/apis/forms.takoform.com/v1alpha1/resources/RelationalDatabase/app/form-transitions/" + request.OperationID,
				DispatchAttempted: boolPointer(true),
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	formClient := discoveredTransitionClient(t, server)
	_, err := formClient.TransitionResourceForm(context.Background(), request)
	var indeterminate *FormTransitionIndeterminateError
	if !errors.As(err, &indeterminate) || indeterminate.OperationID != request.OperationID {
		t.Fatalf("error = %v, want operation-bound indeterminate error", err)
	}
	if posts.Load() != 1 || gets.Load() != 2 {
		t.Fatalf("POST=%d GET=%d, want no blind mutation retry", posts.Load(), gets.Load())
	}
}

func TestFormTransitionDirectAcceptedResponseIsIndeterminateWithoutRetry(t *testing.T) {
	request := canonicalTransitionRequest(t)
	var posts atomic.Int64
	var gets atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/.well-known/takoform":
			writeTransitionDiscovery(t, w, server.URL, true)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/form-transitions/"):
			gets.Add(1)
			writeTransitionOperationAbsent(t, w)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/form-transitions"):
			posts.Add(1)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"operation": map[string]any{
				"operationId":       request.OperationID,
				"status":            "indeterminate",
				"requestDigest":     mustTransitionRequestDigest(t, request),
				"reconcilePath":     "/apis/forms.takoform.com/v1alpha1/resources/RelationalDatabase/app/form-transitions/" + request.OperationID,
				"dispatchAttempted": true,
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	formClient := discoveredTransitionClient(t, server)
	_, err := formClient.TransitionResourceForm(context.Background(), request)
	var indeterminate *FormTransitionIndeterminateError
	if !errors.As(err, &indeterminate) || indeterminate.OperationID != request.OperationID {
		t.Fatalf("error = %v, want operation-bound indeterminate error", err)
	}
	if posts.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("POST=%d GET=%d, direct 202 needs one preflight and no extra readback", posts.Load(), gets.Load())
	}
}

func TestFormTransitionPreflightNeverReissuesAnAttemptedOrUnprovedOperation(t *testing.T) {
	request := canonicalTransitionRequest(t)
	evidenceDigest, err := TransitionEvidenceDigest(
		relationalDatabaseV2ToV3TransitionEvidenceMarker,
		transitionOldDatabaseForm,
		transitionNewDatabaseForm,
	)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		write       func(http.ResponseWriter)
		wantSuccess bool
		wantText    string
	}{
		{
			name: "committed exact proof",
			write: func(w http.ResponseWriter) {
				w.Header().Set("ETag", `"8"`)
				_ = json.NewEncoder(w).Encode(committedTransitionResponse(t, request, evidenceDigest, "8"))
			},
			wantSuccess: true,
		},
		{
			name: "prepared after dispatch attempt",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusAccepted)
				response := unresolvedTransitionResponse(t, request, "prepared")
				response.Operation.DispatchAttempted = boolPointer(true)
				_ = json.NewEncoder(w).Encode(response)
			},
			wantText: "indeterminate",
		},
		{
			name: "indeterminate",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(unresolvedTransitionResponse(t, request, "indeterminate"))
			},
			wantText: "indeterminate",
		},
		{
			name: "prepared missing dispatch proof",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusAccepted)
				response := unresolvedTransitionResponse(t, request, "prepared")
				response.Operation.DispatchAttempted = nil
				_ = json.NewEncoder(w).Encode(response)
			},
			wantText: "indeterminate",
		},
		{
			name: "prepared with premature committed proof",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusAccepted)
				response := unresolvedTransitionResponse(t, request, "prepared")
				response.Resource = &request.Resource
				_ = json.NewEncoder(w).Encode(response)
			},
			wantText: "committed Resource or proof",
		},
		{
			name: "request digest conflict",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusAccepted)
				response := unresolvedTransitionResponse(t, request, "prepared")
				response.Operation.RequestDigest = "sha256:" + strings.Repeat("b", 64)
				_ = json.NewEncoder(w).Encode(response)
			},
			wantText: "different exact request",
		},
		{
			name: "stable failed",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
					"code": "form_identity_conflict", "message": "host rejected transition",
					"requestId": "request-form-transition-failed", "retryable": false,
					"hostCode": "database_migration_rejected",
					"details": map[string]any{
						"operationId": request.OperationID, "requestDigest": mustTransitionRequestDigest(t, request),
						"status": "failed", "failureCode": "database_migration_rejected",
					},
				}})
			},
			wantText: "failed definitively",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var posts atomic.Int64
			var gets atomic.Int64
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/.well-known/takoform":
					writeTransitionDiscovery(t, w, server.URL, true)
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/form-transitions/"):
					gets.Add(1)
					test.write(w)
				case r.Method == http.MethodPost:
					posts.Add(1)
					http.Error(w, "preflight incorrectly issued mutation", http.StatusInternalServerError)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			formClient := discoveredTransitionClient(t, server)
			response, err := formClient.TransitionResourceForm(context.Background(), request)
			if test.wantSuccess {
				if err != nil || response == nil {
					t.Fatalf("committed preflight = %#v, %v", response, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want %q", err, test.wantText)
			}
			if gets.Load() != 1 || posts.Load() != 0 {
				t.Fatalf("GET=%d POST=%d, attempted or unproved operation must be readback-only", gets.Load(), posts.Load())
			}
		})
	}
}

func TestFormTransitionPreflightResumesExactPreparedOperationBeforeDispatch(t *testing.T) {
	request := canonicalTransitionRequest(t)
	evidenceDigest, err := TransitionEvidenceDigest(
		relationalDatabaseV2ToV3TransitionEvidenceMarker,
		transitionOldDatabaseForm,
		transitionNewDatabaseForm,
	)
	if err != nil {
		t.Fatal(err)
	}
	var posts atomic.Int64
	var gets atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/.well-known/takoform":
			writeTransitionDiscovery(t, w, server.URL, true)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/form-transitions/"):
			gets.Add(1)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(unresolvedTransitionResponse(t, request, "prepared"))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/form-transitions"):
			posts.Add(1)
			if r.Header.Get("Idempotency-Key") != request.OperationID {
				t.Fatalf("Idempotency-Key = %q, want %q", r.Header.Get("Idempotency-Key"), request.OperationID)
			}
			w.Header().Set("ETag", `"8"`)
			_ = json.NewEncoder(w).Encode(committedTransitionResponse(t, request, evidenceDigest, "8"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	formClient := discoveredTransitionClient(t, server)
	response, err := formClient.TransitionResourceForm(context.Background(), request)
	if err != nil || response == nil {
		t.Fatalf("resumed prepared operation = %#v, %v", response, err)
	}
	if gets.Load() != 1 || posts.Load() != 1 {
		t.Fatalf("GET=%d POST=%d, want one read-only preflight and one CAS-protected resume", gets.Load(), posts.Load())
	}
}

func TestFormTransitionPreflightRequiresExactPostPermission(t *testing.T) {
	request := canonicalTransitionRequest(t)
	tests := []struct {
		name  string
		write func(http.ResponseWriter)
	}{
		{
			name: "retryable absence is not stable proof",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
					"code": "resource_not_found", "message": "transition lookup unavailable",
					"requestId": "request-transition-lookup-unavailable", "retryable": true,
					"hostCode": "form_transition_operation_not_found",
				}})
			},
		},
		{
			name: "wrong host code",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
					"code": "resource_not_found", "message": "resource absent",
					"requestId": "request-resource-absent", "retryable": false,
					"hostCode": "resource_not_found",
				}})
			},
		},
		{
			name: "transport unknown",
			write: func(w http.ResponseWriter) {
				connection, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Fatal(err)
				}
				_ = connection.Close()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var posts atomic.Int64
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/.well-known/takoform":
					writeTransitionDiscovery(t, w, server.URL, true)
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/form-transitions/"):
					test.write(w)
				case r.Method == http.MethodPost:
					posts.Add(1)
					http.Error(w, "unsafe POST", http.StatusInternalServerError)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			formClient := discoveredTransitionClient(t, server)
			_, err := formClient.TransitionResourceForm(context.Background(), request)
			var indeterminate *FormTransitionIndeterminateError
			if !errors.As(err, &indeterminate) {
				t.Fatalf("error = %v, want fail-closed indeterminate preflight", err)
			}
			if posts.Load() != 0 {
				t.Fatalf("POST=%d, preflight did not prove absence or unattempted prepared state", posts.Load())
			}
		})
	}
}

func TestFormTransitionRejectsMissingCapabilityBeforeHostMutation(t *testing.T) {
	request := canonicalTransitionRequest(t)
	var mutations atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/takoform" {
			writeTransitionDiscovery(t, w, server.URL, false)
			return
		}
		mutations.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	formClient := discoveredTransitionClient(t, server)
	if _, err := formClient.TransitionResourceForm(context.Background(), request); err == nil || !strings.Contains(err.Error(), FeatureResourceFormTransition) {
		t.Fatalf("error = %v, want capability diagnostic", err)
	}
	if mutations.Load() != 0 {
		t.Fatalf("host mutations = %d, want 0", mutations.Load())
	}
}

func TestFormTransitionRejectsSubstitutedProof(t *testing.T) {
	request := canonicalTransitionRequest(t)
	digest, _ := TransitionEvidenceDigest(
		relationalDatabaseV2ToV3TransitionEvidenceMarker,
		transitionOldDatabaseForm,
		transitionNewDatabaseForm,
	)
	tests := []struct {
		name   string
		mutate func(*FormTransitionResponse)
	}{
		{name: "operation", mutate: func(response *FormTransitionResponse) { response.TransitionProof.OperationID = "other" }},
		{name: "request digest", mutate: func(response *FormTransitionResponse) {
			response.Operation.RequestDigest = "sha256:" + strings.Repeat("b", 64)
		}},
		{name: "from form", mutate: func(response *FormTransitionResponse) { response.TransitionProof.FromForm = transitionNewDatabaseForm }},
		{name: "to form", mutate: func(response *FormTransitionResponse) { response.TransitionProof.ToForm = transitionOldDatabaseForm }},
		{name: "evidence digest", mutate: func(response *FormTransitionResponse) {
			response.TransitionProof.TransitionEvidenceDigest = "sha256:" + strings.Repeat("b", 64)
		}},
		{name: "spec digest", mutate: func(response *FormTransitionResponse) {
			response.TransitionProof.ObservedSpecDigest = "sha256:" + strings.Repeat("b", 64)
		}},
		{name: "resource form", mutate: func(response *FormTransitionResponse) { response.Resource.Form = &transitionOldDatabaseForm }},
		{name: "resource version", mutate: func(response *FormTransitionResponse) { response.TransitionProof.ResourceVersion = "9" }},
		{name: "resource generation jump", mutate: func(response *FormTransitionResponse) {
			response.TransitionProof.ResourceVersion = "9"
			response.Resource.Metadata.ResourceVersion = "9"
		}},
		{name: "native identity", mutate: func(response *FormTransitionResponse) { response.TransitionProof.NativeIdentity.ID = "other" }},
		{name: "not committed", mutate: func(response *FormTransitionResponse) { response.TransitionProof.Committed = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/.well-known/takoform" {
					writeTransitionDiscovery(t, w, server.URL, true)
					return
				}
				response := committedTransitionResponse(t, request, digest, "8")
				test.mutate(&response)
				w.Header().Set("ETag", `"8"`)
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			formClient := discoveredTransitionClient(t, server)
			if _, err := formClient.TransitionResourceForm(context.Background(), request); err == nil {
				t.Fatal("client accepted a substituted transition proof")
			}
		})
	}
}

func unresolvedTransitionResponse(t *testing.T, request FormTransitionRequest, status string) FormTransitionResponse {
	t.Helper()
	dispatchAttempted := status == "indeterminate"
	return FormTransitionResponse{Operation: FormTransitionOperation{
		OperationID: request.OperationID,
		Status:      status, RequestDigest: mustTransitionRequestDigest(t, request),
		ReconcilePath:     "/apis/forms.takoform.com/v1alpha1/resources/RelationalDatabase/app/form-transitions/" + request.OperationID,
		DispatchAttempted: &dispatchAttempted,
	}}
}

func boolPointer(value bool) *bool { return &value }

func canonicalTransitionRequest(t *testing.T) FormTransitionRequest {
	t.Helper()
	resource := Resource{
		APIVersion: APIVersion,
		Kind:       "RelationalDatabase",
		Form:       &transitionNewDatabaseForm,
		Metadata: Metadata{
			Name: "app", Space: "prod", ResourceVersion: "7",
		},
		Spec: map[string]any{
			"name": "app", "engine": "postgres",
			"schemaUrl":    "https://artifacts.portable-conformance.invalid/schema.json",
			"schemaSha256": strings.Repeat("a", 64),
			"schemaFormat": "takosumi.resource-migrations",
		},
	}
	evidence, err := NewFormTransitionEvidence(
		relationalDatabaseV2ToV3TransitionEvidenceMarker,
		transitionOldDatabaseForm,
		transitionNewDatabaseForm,
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := NewFormTransitionRequest(
		transitionOldDatabaseForm,
		transitionNewDatabaseForm,
		resource,
		FormTransitionExpected{
			ResourceVersion: "7",
			NativeIdentity:  &NativeResourceIdentity{Type: "cloudflare-d1-database", ID: "native-1"},
		},
		evidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func committedTransitionResponse(
	t *testing.T,
	request FormTransitionRequest,
	evidenceDigest string,
	resourceVersion string,
) FormTransitionResponse {
	t.Helper()
	resource := request.Resource
	resource.Metadata.ResourceVersion = resourceVersion
	resource.Status = &Status{
		Observed: map[string]any{
			"id": "RelationalDatabase/app", "ready": true, "generation": 8,
			"imported": true, "portability": "portable", "driftedFields": []any{},
		},
		Output: map[string]any{
			"id": "RelationalDatabase/app", "kind": "RelationalDatabase", "name": "app",
			"generation": 8, "portability": "portable", "engine": "postgres",
		},
	}
	return FormTransitionResponse{
		Operation: FormTransitionOperation{
			OperationID: request.OperationID, Status: "committed",
			RequestDigest: mustTransitionRequestDigest(t, request),
			ReconcilePath: "/apis/forms.takoform.com/v1alpha1/resources/RelationalDatabase/app/form-transitions/" + request.OperationID,
		},
		Resource: &resource,
		TransitionProof: &FormTransitionProof{
			OperationID: request.OperationID, FromForm: request.FromForm, ToForm: request.ToForm,
			TransitionEvidenceDigest: evidenceDigest,
			ObservedSpecDigest:       mustSpecDigest(t, request.Resource.Spec),
			ResourceVersion:          resourceVersion,
			NativeIdentity:           NativeResourceIdentity{Type: "cloudflare-d1-database", ID: "native-1"},
			Committed:                true,
		},
	}
}

func writeTransitionOperationAbsent(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"code": "resource_not_found", "message": "transition operation not found",
		"requestId": "request-form-transition-absent", "retryable": false,
		"hostCode": "form_transition_operation_not_found",
	}})
}

func mustTransitionRequestDigest(t *testing.T, request FormTransitionRequest) string {
	t.Helper()
	digest, err := formTransitionRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func writeTransitionDiscovery(t *testing.T, w http.ResponseWriter, origin string, capability bool) {
	t.Helper()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"api_versions": []string{APIVersion},
		"features": map[string]bool{
			"service_forms": true, "exact_form_ref": true,
			"optimistic_concurrency": true, "idempotent_lifecycle": true,
			FeatureResourceFormTransition: capability,
		},
		"endpoints": map[string]string{"api": origin + "/apis/forms.takoform.com/v1alpha1"},
	})
}

func discoveredTransitionClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client := New(server.URL, "", server.Client())
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	return client
}

func mustSpecDigest(t *testing.T, spec map[string]any) string {
	t.Helper()
	digest, err := canonicalValueDigest(spec)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
