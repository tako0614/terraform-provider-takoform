package clientv3

// accepted_mutation_test.go proves the acceptance boundary: a failure that
// arrives BEFORE the host accepted a mutation is an ordinary error, while a
// failure after acceptance carries *AcceptedError so a state-persisting caller
// can record what the host now owns instead of orphaning it.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// serve202ThenPending answers the apply PUT with a 202 Operation that never
// reaches a terminal state, modelling a host that accepted the mutation and is
// still working when the caller's deadline expires.
func serve202ThenPending(t *testing.T, operationID string, target map[string]any) func(http.ResponseWriter, *http.Request) bool {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodPut,
			r.Method == http.MethodPost && r.URL.EscapedPath() == splitGroupResourcePath("app", "import"):
			body := map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": operationID, "done": false,
			}
			if target != nil {
				body["target"] = target
			}
			w.Header().Set("Retry-After", "0")
			writeJSON(t, w, http.StatusAccepted, map[string]any{"operation": body})
			return true
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/"+operationID:
			body := map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": operationID, "done": false,
			}
			if target != nil {
				body["target"] = target
			}
			w.Header().Set("Retry-After", "0")
			writeJSON(t, w, http.StatusOK, body)
			return true
		}
		return false
	}
}

func TestApplyResourceReportsAcceptanceWhenTheOperationOutlivesTheDeadline(t *testing.T) {
	client := newTestClient(t, serve202ThenPending(t, "op_apply_pending", map[string]any{"uid": "uid-7"}))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	out, err := client.ApplyResource(ctx, testResourceRequest(map[string]any{"image": "example"}), Fence{})
	if out != nil {
		t.Fatalf("unfinished apply returned a resource: %+v", out)
	}
	var accepted *AcceptedError
	if !errors.As(err, &accepted) {
		t.Fatalf("apply that the host accepted did not report acceptance: %v", err)
	}
	if accepted.OperationID != "op_apply_pending" {
		t.Fatalf("accepted error operation id = %q, want op_apply_pending", accepted.OperationID)
	}
	if accepted.UID != "uid-7" {
		t.Fatalf("accepted error uid = %q, want the operation target uid-7", accepted.UID)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("accepted error dropped the underlying cause: %v", err)
	}
}

func TestImportResourceReportsAcceptanceWhenTheOperationOutlivesTheDeadline(t *testing.T) {
	client := newTestClient(t, serve202ThenPending(t, "op_import_pending", nil))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := client.ImportResource(ctx, testResourceRequest(map[string]any{"image": "example"}), "native-1")
	var accepted *AcceptedError
	if !errors.As(err, &accepted) {
		t.Fatalf("import that the host accepted did not report acceptance: %v", err)
	}
	if accepted.OperationID != "op_import_pending" {
		t.Fatalf("accepted error operation id = %q, want op_import_pending", accepted.OperationID)
	}
	if accepted.UID != "" {
		t.Fatalf("accepted error invented a uid the host never disclosed: %q", accepted.UID)
	}
}

// TestApplyResourceFailuresBeforeAcceptanceAreNotAccepted keeps the boundary
// honest: nothing that fails before the host answers the mutation may claim the
// mutation happened, or every failed plan would leave phantom state behind.
func TestApplyResourceFailuresBeforeAcceptanceAreNotAccepted(t *testing.T) {
	t.Run("form unavailable", func(t *testing.T) {
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == APIRootPath+"/forms" {
				writeJSON(t, w, http.StatusOK, map[string]any{"forms": []any{}})
				return true
			}
			return false
		})
		_, err := client.ApplyResource(context.Background(), testResourceRequest(nil), Fence{})
		assertNotAccepted(t, err)
	})
	t.Run("mutation rejected", func(t *testing.T) {
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
			switch {
			case r.URL.Path == APIRootPath+"/forms":
				writeJSON(t, w, http.StatusOK, wireAvailability("create"))
				return true
			case r.URL.Path == APIRootPath+"/resources/prepare":
				handlePrepare(t, w, r)
				return true
			case r.Method == http.MethodPut:
				writeStableError(t, w, http.StatusForbidden, "policy_denied", false)
				return true
			}
			return false
		})
		_, err := client.ApplyResource(context.Background(), testResourceRequest(nil), Fence{})
		assertNotAccepted(t, err)
	})
}

func assertNotAccepted(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a pre-acceptance failure")
	}
	var accepted *AcceptedError
	if errors.As(err, &accepted) {
		t.Fatalf("a failure before host acceptance claimed the mutation was accepted: %v", err)
	}
}

// TestAcceptedErrorSurfacesTheUnderlyingCause protects the callers that match
// on the wrapped message rather than the type.
func TestAcceptedErrorSurfacesTheUnderlyingCause(t *testing.T) {
	err := acceptedMutation("op_x", "uid-1", errors.New("underlying cause"))
	if !contains(err, "underlying cause") || !contains(err, "op_x") || !contains(err, "uid-1") {
		t.Fatalf("accepted error message hides its evidence: %v", err)
	}
	if acceptedMutation("op_x", "", nil) != nil {
		t.Fatal("acceptedMutation invented an error from nil")
	}
}
