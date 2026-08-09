package clientv3

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

const testPrepareDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// handlePrepare echoes the requested identity and spec with a fixed
// prepareDigest and the canonical specDigest, the way a conforming host
// binds a prepare.
func handlePrepare(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	raw, _ := io.ReadAll(r.Body)
	var request struct {
		APIVersion string         `json:"apiVersion"`
		Kind       string         `json:"kind"`
		Form       map[string]any `json:"form"`
		Metadata   map[string]any `json:"metadata"`
		Spec       map[string]any `json:"spec"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Errorf("prepare request not JSON: %v", err)
	}
	writeJSON(t, w, http.StatusOK, map[string]any{
		"resource": map[string]any{
			"apiVersion": request.APIVersion,
			"kind":       request.Kind,
			"form":       request.Form,
			"metadata":   request.Metadata,
			"spec":       request.Spec,
		},
		"review": map[string]any{
			"prepareDigest": testPrepareDigest,
			"specDigest":    specDigestOf(t, request.Spec),
		},
	})
}

func TestApplyResourceCreateHappyPath(t *testing.T) {
	spec := map[string]any{"image": "example"}
	var sawPut bool
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/forms":
			q := r.URL.Query()
			if q.Get("group") != testGroup || q.Get("kind") != testKind ||
				q.Get("definitionVersion") != "1.0.0" || q.Get("schemaDigest") != testSchemaDigest ||
				q.Get("space") != testSpace {
				t.Errorf("forms query is not the exact FormRef query: %v", q)
			}
			if q.Get("apiVersion") != "" {
				t.Errorf("forms query must use group, not apiVersion")
			}
			writeJSON(t, w, http.StatusOK, wireAvailability("create", "update", "delete"))
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare":
			if got := r.Header.Get(expectedGenerationHeader); got != "" {
				t.Errorf("create prepare must not send a generation fence, got %q", got)
			}
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodPut && r.URL.EscapedPath() == splitGroupResourcePath("app", ""):
			sawPut = true
			if r.Header.Get("If-None-Match") != "*" {
				t.Errorf("create apply must send If-None-Match: *, got %q", r.Header.Get("If-None-Match"))
			}
			if r.Header.Get(expectedGenerationHeader) != "" {
				t.Errorf("create apply must not send a generation fence")
			}
			if r.Header.Get("Idempotency-Key") == "" {
				t.Errorf("apply must send an Idempotency-Key")
			}
			raw, _ := io.ReadAll(r.Body)
			var body struct {
				Review struct {
					PrepareDigest string `json:"prepareDigest"`
				} `json:"review"`
				ExpectedUID        string `json:"expectedUid"`
				ExpectedGeneration string `json:"expectedGeneration"`
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("apply body not JSON: %v", err)
			}
			if body.Review.PrepareDigest != testPrepareDigest {
				t.Errorf("apply must carry the prepareDigest, got %q", body.Review.PrepareDigest)
			}
			if body.ExpectedUID != "" || body.ExpectedGeneration != "" {
				t.Errorf("create apply must not carry fences in the body")
			}
			w.Header().Set("ETag", `"7"`)
			writeJSON(t, w, http.StatusCreated, wireResource("app", "uid-1", "1", "7", spec))
			return true
		}
		return false
	})

	out, err := client.ApplyResource(context.Background(), testResourceRequest(spec), Fence{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !sawPut {
		t.Fatalf("apply never reached PUT")
	}
	if out.Metadata.UID != "uid-1" || out.Metadata.Generation != "1" || out.Metadata.Revision != "7" {
		t.Fatalf("apply did not capture host identity: %+v", out.Metadata)
	}
	if !ResourceReady(out) {
		t.Fatalf("Ready condition not surfaced")
	}
}

func TestApplyResourceIdentitySubstitutionFailsClosed(t *testing.T) {
	spec := map[string]any{"image": "example"}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodPut:
			substituted := wireResource("other", "uid-1", "1", "7", spec)
			w.Header().Set("ETag", `"7"`)
			writeJSON(t, w, http.StatusCreated, substituted)
			return true
		}
		return false
	})
	_, err := client.ApplyResource(context.Background(), testResourceRequest(spec), Fence{})
	if err == nil || !contains(err, "changed the requested resource name") {
		t.Fatalf("expected identity substitution rejection, got %v", err)
	}
}

func TestApplyResourceMissingETagFailsClosed(t *testing.T) {
	spec := map[string]any{"image": "example"}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodPut:
			writeJSON(t, w, http.StatusCreated, wireResource("app", "uid-1", "1", "7", spec))
			return true
		}
		return false
	})
	_, err := client.ApplyResource(context.Background(), testResourceRequest(spec), Fence{})
	if err == nil || !contains(err, "exactly one ETag") {
		t.Fatalf("expected ETag requirement, got %v", err)
	}
}

func TestApplyResourceStaleGenerationConflict(t *testing.T) {
	spec := map[string]any{"image": "example"}
	putCalls := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create", "update"))
			return true
		case r.URL.Path == APIRootPath+"/resources/prepare":
			if got := r.Header.Get(expectedGenerationHeader); got != "3" {
				t.Errorf("update prepare must send %s: 3, got %q", expectedGenerationHeader, got)
			}
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodPut:
			putCalls++
			if r.Header.Get(expectedGenerationHeader) != "3" {
				t.Errorf("update apply must fence on Takoform-Expected-Generation, got %q",
					r.Header.Get(expectedGenerationHeader))
			}
			if r.Header.Get("If-None-Match") != "" {
				t.Errorf("update apply must not send If-None-Match")
			}
			writeStableError(t, w, http.StatusPreconditionFailed, "generation_conflict", false)
			return true
		}
		return false
	})

	_, err := client.ApplyResource(context.Background(), testResourceRequest(spec), Fence{ExpectedGeneration: "3"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusPreconditionFailed || apiErr.Code != "generation_conflict" {
		t.Fatalf("expected 412 generation_conflict, got %+v", apiErr)
	}
	if apiErr.Retryable || apiErr.ProtocolInvalid {
		t.Fatalf("generation_conflict must be a stable non-retryable error: %+v", apiErr)
	}
	if putCalls != 1 {
		t.Fatalf("non-retryable stable error must not be retried, saw %d attempts", putCalls)
	}
}

func TestGetResourceCapturesRevisionETag(t *testing.T) {
	spec := map[string]any{"image": "example"}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.EscapedPath() == splitGroupResourcePath("app", "") {
			if r.URL.Query().Get("group") != testGroup {
				t.Errorf("read query must carry group, got %v", r.URL.Query())
			}
			w.Header().Set("ETag", `"12"`)
			writeJSON(t, w, http.StatusOK, wireResource("app", "uid-1", "2", "12", spec))
			return true
		}
		return false
	})
	out, err := client.GetResource(context.Background(), testSpace, testRef, "app")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out.Metadata.Revision != "12" {
		t.Fatalf("revision not captured: %+v", out.Metadata)
	}
}

func TestGetResourceNotFound(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		writeStableError(t, w, http.StatusNotFound, "resource_not_found", false)
		return true
	})
	_, err := client.GetResource(context.Background(), testSpace, testRef, "app")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestObserveResourceFenceMismatchRejected(t *testing.T) {
	spec := map[string]any{"image": "example"}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.EscapedPath() == splitGroupResourcePath("app", "observe") {
			if r.Header.Get(expectedGenerationHeader) != "4" {
				t.Errorf("observe must fence on Takoform-Expected-Generation, got %q",
					r.Header.Get(expectedGenerationHeader))
			}
			if r.Header.Get("Idempotency-Key") == "" {
				t.Errorf("observe must send an Idempotency-Key")
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"resource": wireResource("app", "uid-1", "5", "20", spec),
			})
			return true
		}
		return false
	})
	_, err := client.ObserveResource(context.Background(), testSpace, testRef, "app", "4")
	if err == nil || !contains(err, "changed the generation protected by the fence") {
		t.Fatalf("expected fence mismatch rejection, got %v", err)
	}
}

func TestObserveResourceHonorsFence(t *testing.T) {
	spec := map[string]any{"image": "example"}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.EscapedPath() == splitGroupResourcePath("app", "observe") {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"resource": wireResource("app", "uid-1", "4", "21", spec),
			})
			return true
		}
		return false
	})
	out, err := client.ObserveResource(context.Background(), testSpace, testRef, "app", "4")
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if out.Metadata.Generation != "4" || out.Metadata.Revision != "21" {
		t.Fatalf("observe result identity wrong: %+v", out.Metadata)
	}
}

func TestDeleteResourceGenerationFence(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodDelete && r.URL.EscapedPath() == splitGroupResourcePath("app", "") {
			if r.Header.Get(expectedGenerationHeader) != "9" {
				t.Errorf("delete must fence on the expected generation, got %q", r.Header.Get(expectedGenerationHeader))
			}
			// And never on the representation: a teardown moves revisions it
			// does not own, so a delete that carried If-Match would refuse
			// itself (spec/decisions/0011).
			if got := r.Header.Get("If-Match"); got != "" {
				t.Errorf("delete must not fence on the representation revision, got If-Match %q", got)
			}
			if r.Header.Get("Idempotency-Key") == "" {
				t.Errorf("delete must send an Idempotency-Key")
			}
			if r.URL.Query().Get("group") != testGroup {
				t.Errorf("delete query must carry the exact FormRef, got %v", r.URL.Query())
			}
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		return false
	})
	if err := client.DeleteResource(context.Background(), testSpace, testRef, "app", "9"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestDeleteResourceOperationNotFoundMapsToErrNotFound(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodDelete:
			writeJSON(t, w, http.StatusAccepted, map[string]any{
				"operation": map[string]any{
					"apiVersion": OperationAPIVersion, "kind": OperationKind,
					"id": "op_del1", "done": false,
				},
			})
			return true
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/op_del1":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": "op_del1", "done": true,
				"error": map[string]any{
					"code": "resource_not_found", "message": "gone", "retryable": false,
				},
			})
			return true
		}
		return false
	})
	err := client.DeleteResource(context.Background(), testSpace, testRef, "app", "9")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound from terminal operation, got %v", err)
	}
}

func TestDeleteRetriesOnStableBackendUnavailable(t *testing.T) {
	calls := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodDelete {
			calls++
			if calls == 1 {
				writeStableError(t, w, http.StatusServiceUnavailable, "backend_unavailable", true)
				return true
			}
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		return false
	})
	if err := client.DeleteResource(context.Background(), testSpace, testRef, "app", "9"); err != nil {
		t.Fatalf("delete after retry: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected exactly one retry, saw %d attempts", calls)
	}
}

func TestDeleteDoesNotRetryProtocolInvalidBody(t *testing.T) {
	calls := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodDelete {
			calls++
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("upstream melted"))
			return true
		}
		return false
	})
	err := client.DeleteResource(context.Background(), testSpace, testRef, "app", "9")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !apiErr.ProtocolInvalid {
		t.Fatalf("expected protocol-invalid error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("protocol-invalid errors must not be retried, saw %d attempts", calls)
	}
}

func TestValidateResourceNeverRequiresDigest(t *testing.T) {
	spec := map[string]any{"image": "example"}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/validate" {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"valid": false,
				"diagnostics": []map[string]any{{
					"severity": "error", "field": "/image", "message": "unknown registry",
				}},
			})
			return true
		}
		return false
	})
	result, err := client.ValidateResource(context.Background(), testResourceRequest(spec))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if result.Valid || len(result.Diagnostics) != 1 || result.Diagnostics[0].Field != "/image" {
		t.Fatalf("diagnostics not surfaced: %+v", result)
	}
}

func TestPrepareResourceSendsUpdateFence(t *testing.T) {
	spec := map[string]any{"image": "example"}
	sawPrepare := false
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare" {
			sawPrepare = true
			if got := r.Header.Get(expectedGenerationHeader); got != "5" {
				t.Errorf("update prepare must send %s: 5, got %q", expectedGenerationHeader, got)
			}
			handlePrepare(t, w, r)
			return true
		}
		return false
	})
	if _, err := client.PrepareResource(context.Background(), testResourceRequest(spec), Fence{ExpectedGeneration: "5"}); err != nil {
		t.Fatalf("prepare with fence: %v", err)
	}
	if !sawPrepare {
		t.Fatal("prepare never reached the host")
	}
	if _, err := client.PrepareResource(context.Background(), testResourceRequest(spec), Fence{ExpectedGeneration: "05"}); err == nil ||
		!contains(err, "canonical decimal") {
		t.Fatalf("expected non-canonical fence rejection, got %v", err)
	}
}

func TestPrepareResourceRejectsSpecSubstitution(t *testing.T) {
	spec := map[string]any{"image": "example"}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/resources/prepare" {
			substituted := map[string]any{"image": "evil"}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"resource": map[string]any{
					"apiVersion": testGroup, "kind": testKind, "form": wireFormReference(),
					"metadata": map[string]any{"name": "app", "space": testSpace},
					"spec":     substituted,
				},
				"review": map[string]any{
					"prepareDigest": testPrepareDigest,
					"specDigest":    specDigestOf(t, substituted),
				},
			})
			return true
		}
		return false
	})
	_, err := client.PrepareResource(context.Background(), testResourceRequest(spec), Fence{})
	if err == nil || !contains(err, "changed the requested spec") {
		t.Fatalf("expected spec substitution rejection, got %v", err)
	}
}

func contains(err error, want string) bool {
	return err != nil && strings.Contains(err.Error(), want)
}
