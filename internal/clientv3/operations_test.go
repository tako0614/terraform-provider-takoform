package clientv3

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestValidateOperationWireAcceptsSchemaOptionalMembers(t *testing.T) {
	pending := map[string]any{
		"apiVersion": OperationAPIVersion,
		"kind":       OperationKind,
		"id":         "op_optional",
		"done":       false,
		"target": map[string]any{
			"uid":            "uid-1",
			"manifestDigest": "sha256:" + strings.Repeat("a", 64),
		},
		"metadata": map[string]any{"phase": "Provisioning", "progress": 25},
	}
	raw, err := json.Marshal(pending)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOperationWire(raw, false); err != nil {
		t.Fatalf("pending operation optional members rejected: %v", err)
	}
	terminal := map[string]any{
		"apiVersion": OperationAPIVersion,
		"kind":       OperationKind,
		"id":         "op_terminal",
		"done":       true,
		"result":     map[string]any{},
	}
	raw, err = json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOperationWire(raw, false); err != nil {
		t.Fatalf("terminal result operation rejected: %v", err)
	}
	terminal["result"] = nil
	terminal["error"] = map[string]any{
		"code":      "internal_error",
		"message":   "failed",
		"requestId": "req-1",
		"retryable": false,
		"hostCode":  "host.internal",
	}
	delete(terminal, "result")
	raw, err = json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOperationWire(raw, false); err != nil {
		t.Fatalf("terminal error hostCode operation rejected: %v", err)
	}
}

func TestValidateOperationWireRejectsMalformedOptionalMembers(t *testing.T) {
	base := map[string]any{
		"apiVersion": OperationAPIVersion,
		"kind":       OperationKind,
		"id":         "op_bad_optional",
		"done":       false,
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"unknown operation field", func(value map[string]any) { value["future"] = true }},
		{"null target", func(value map[string]any) { value["target"] = nil }},
		{"metadata unknown field", func(value map[string]any) { value["metadata"] = map[string]any{"future": true} }},
		{"empty error message", func(value map[string]any) {
			value["done"] = true
			value["error"] = map[string]any{"code": "internal_error", "message": "", "requestId": "req-1", "retryable": false}
		}},
		{"empty host code", func(value map[string]any) {
			value["done"] = true
			value["error"] = map[string]any{"code": "internal_error", "message": "failed", "requestId": "req-1", "retryable": false, "hostCode": ""}
		}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			value := mapsClone(base)
			test.mutate(value)
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateOperationWire(raw, false); err == nil {
				t.Fatal("malformed operation was accepted")
			}
		})
	}
}

func mapsClone(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func TestApplyResourceVia202OperationPolling(t *testing.T) {
	spec := map[string]any{"image": "example"}
	operationGets := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == APIRootPath+"/forms":
			writeJSON(t, w, http.StatusOK, wireAvailability("create"))
			return true
		case r.URL.Path == APIRootPath+"/resources/prepare":
			handlePrepare(t, w, r)
			return true
		case r.Method == http.MethodPut:
			w.Header().Set("Retry-After", "0")
			writeJSON(t, w, http.StatusAccepted, map[string]any{
				"operation": map[string]any{
					"apiVersion": OperationAPIVersion, "kind": OperationKind,
					"id": "op_apply1", "done": false,
					"metadata": map[string]any{"phase": "Provisioning"},
				},
			})
			return true
		case r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/op_apply1":
			operationGets++
			if operationGets == 1 {
				w.Header().Set("Retry-After", "0")
				writeJSON(t, w, http.StatusOK, map[string]any{
					"apiVersion": OperationAPIVersion, "kind": OperationKind,
					"id": "op_apply1", "done": false,
				})
				return true
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": "op_apply1", "done": true,
				"result": map[string]any{
					"resource": wireResource("app", "uid-1", "1", "3", spec),
				},
			})
			return true
		}
		return false
	})

	out, err := client.ApplyResource(context.Background(), testResourceRequest(spec), Fence{})
	if err != nil {
		t.Fatalf("apply via operation: %v", err)
	}
	if operationGets < 2 {
		t.Fatalf("expected at least two operation polls, saw %d", operationGets)
	}
	if out.Metadata.UID != "uid-1" || out.Metadata.Revision != "3" {
		t.Fatalf("operation result resource not extracted: %+v", out.Metadata)
	}
}

func TestAwaitOperationTerminalError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/op_fail1" {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": "op_fail1", "done": true,
				"error": map[string]any{
					"code": "policy_denied", "message": "denied by policy",
					"requestId": "req-denied", "retryable": false,
				},
			})
			return true
		}
		return false
	})
	operation, err := client.AwaitOperation(context.Background(), "op_fail1", time.Minute)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError from terminal error operation, got %v", err)
	}
	if apiErr.Code != "policy_denied" || apiErr.StatusCode != http.StatusForbidden || apiErr.Retryable {
		t.Fatalf("terminal error not mapped to the closed taxonomy: %+v", apiErr)
	}
	if operation == nil || !operation.Done || operation.Error == nil {
		t.Fatalf("terminal operation not returned alongside the error")
	}
}

func TestAwaitOperationRejectsResultAndError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/op_bad1" {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": "op_bad1", "done": true,
				"result": map[string]any{},
				"error": map[string]any{
					"code": "internal_error", "message": "both",
					"requestId": "req-both", "retryable": false,
				},
			})
			return true
		}
		return false
	})
	_, err := client.AwaitOperation(context.Background(), "op_bad1", time.Minute)
	if err == nil || !contains(err, "exactly one of result or error") {
		t.Fatalf("expected exactly-one rejection, got %v", err)
	}
}

func TestAwaitOperationOverallDeadline(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/op_slow1" {
			w.Header().Set("Retry-After", "0")
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": "op_slow1", "done": false,
			})
			return true
		}
		return false
	})
	start := time.Now()
	_, err := client.AwaitOperation(context.Background(), "op_slow1", 300*time.Millisecond)
	if err == nil || !contains(err, "not terminal before the overall deadline") {
		t.Fatalf("expected overall-deadline failure, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("deadline not enforced promptly: %v", elapsed)
	}
}

func TestAwaitOperationCallerDeadlineOutlivesOverallDeadline(t *testing.T) {
	// The caller's context carries a deadline far beyond the client's tiny
	// overall deadline. The first poll answers pending with Retry-After: 1
	// (one second, longer than the overall deadline), so polling survives past
	// the overall deadline only when the caller's context is authoritative.
	polls := 0
	client := newTestClientWithOptions(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/operations/op_ctx1" {
			polls++
			if polls == 1 {
				w.Header().Set("Retry-After", "1")
				writeJSON(t, w, http.StatusOK, map[string]any{
					"apiVersion": OperationAPIVersion, "kind": OperationKind,
					"id": "op_ctx1", "done": false,
				})
				return true
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": "op_ctx1", "done": true,
				"result": map[string]any{"resource": map[string]any{}},
			})
			return true
		}
		return false
	}, Options{OverallDeadline: 50 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	operation, err := client.AwaitOperation(ctx, "op_ctx1", 0)
	if err != nil {
		t.Fatalf("polling under the caller deadline was cut short: %v", err)
	}
	if operation == nil || !operation.Done {
		t.Fatalf("operation did not reach its terminal state: %+v", operation)
	}
	if polls < 2 {
		t.Fatalf("expected at least two polls, saw %d", polls)
	}
}

func TestWaitForPollHonorsRetryAfterOverBackoff(t *testing.T) {
	// A Retry-After hint longer than the context deadline must keep the
	// poller sleeping until the context expires: the hint, not the jittered
	// backoff, controls the wait.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := waitForPoll(ctx, 0, 5*time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline while honoring Retry-After, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("waitForPoll returned before the context deadline: %v", elapsed)
	}
}

func TestWaitForPollCapsBackoffWindow(t *testing.T) {
	// Even at a high attempt count the jittered window must stay within the
	// 15s cap; with a generous margin this stays deterministic.
	ctx := context.Background()
	start := time.Now()
	if err := waitForPoll(ctx, 1, 0); err != nil {
		t.Fatalf("waitForPoll: %v", err)
	}
	if elapsed := time.Since(start); elapsed > pollMaxDelay {
		t.Fatalf("backoff exceeded the per-interval cap: %v", elapsed)
	}
}
