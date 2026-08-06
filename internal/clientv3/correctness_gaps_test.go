package clientv3

// correctness_gaps_test.go covers the small fail-closed and contract-coverage
// gaps of the v1alpha3 client: operation-id substitution on cancel, exact
// schemaDigest agreement on support profiles, host Retry-After hints that the
// client used to silently shorten, and the upload-abandon endpoint.

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestCancelOperationRejectsASubstitutedOperationID(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/operations/op_wanted/cancel" {
			// A conforming host replays the terminal state of the operation that
			// was asked about. This one answers about a different operation.
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": "op_other", "done": true,
				"error": map[string]any{
					"code": "operation_cancelled", "message": "cancelled", "retryable": false,
				},
			})
			return true
		}
		return false
	})
	if _, err := client.CancelOperation(context.Background(), "op_wanted"); err == nil ||
		!contains(err, "different operation id") {
		t.Fatalf("cancel accepted a response naming another operation: %v", err)
	}
}

func TestCancelOperationAcceptsTheRequestedOperationID(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/operations/op_wanted/cancel" {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": OperationAPIVersion, "kind": OperationKind,
				"id": "op_wanted", "done": false,
			})
			return true
		}
		return false
	})
	operation, err := client.CancelOperation(context.Background(), "op_wanted")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if operation.ID != "op_wanted" {
		t.Fatalf("cancel returned %q", operation.ID)
	}
}

func TestGetFormSupportRejectsADifferentExactSchemaDigest(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet {
			profile := wireSupportProfile()
			// Same group/kind/definitionVersion line, different immutable
			// definition: the profile does not describe what was asked about.
			profile["formRef"].(map[string]any)["schemaDigest"] =
				"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
			writeJSON(t, w, http.StatusOK, profile)
			return true
		}
		return false
	})
	if _, err := client.GetFormSupport(context.Background(), testRef); err == nil ||
		!contains(err, "different exact Form schemaDigest") {
		t.Fatalf("support profile for another exact definition was accepted: %v", err)
	}
}

func TestGetFormSupportAcceptsAProfileWithoutASchemaDigest(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet {
			profile := wireSupportProfile()
			delete(profile["formRef"].(map[string]any), "schemaDigest")
			writeJSON(t, w, http.StatusOK, profile)
			return true
		}
		return false
	})
	if _, err := client.GetFormSupport(context.Background(), testRef); err != nil {
		t.Fatalf("line-level support profile without a schemaDigest was rejected: %v", err)
	}
}

// TestRetryAfterDelayHonorsTheHostHint proves the hint is no longer truncated
// to a client-side per-interval cap. Shortening it produced earlier, pointless
// traffic against a host that had already said "not yet"; the caller's own
// deadline is the only legitimate bound and the wait loops already select on
// it.
func TestRetryAfterDelayHonorsTheHostHint(t *testing.T) {
	bounded, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	for name, testCase := range map[string]struct {
		ctx        context.Context
		retryAfter time.Duration
		cap        time.Duration
		want       time.Duration
	}{
		"hint beyond the poll cap is honored in full": {
			ctx: bounded, retryAfter: 90 * time.Second, cap: pollMaxDelay, want: 90 * time.Second,
		},
		"hint beyond the retry cap is honored in full": {
			ctx: bounded, retryAfter: 10 * time.Minute, cap: retryMaxDelay, want: 10 * time.Minute,
		},
		"hint below the cap is unchanged": {
			ctx: bounded, retryAfter: time.Second, cap: pollMaxDelay, want: time.Second,
		},
		"deadline-free caller keeps the cap as its only bound": {
			ctx: context.Background(), retryAfter: 90 * time.Second, cap: pollMaxDelay, want: pollMaxDelay,
		},
		"deadline-free caller below the cap is unchanged": {
			ctx: context.Background(), retryAfter: time.Second, cap: pollMaxDelay, want: time.Second,
		},
	} {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			if got := retryAfterDelay(testCase.ctx, testCase.retryAfter, testCase.cap); got != testCase.want {
				t.Fatalf("retryAfterDelay = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestPollBackoffWindowKeepsTheCap pins the other half of the contract: the
// no-hint exponential backoff is still capped unconditionally.
func TestPollBackoffWindowKeepsTheCap(t *testing.T) {
	if got := pollBackoffWindow(0); got != pollBaseDelay {
		t.Fatalf("first backoff window = %v, want %v", got, pollBaseDelay)
	}
	if got := pollBackoffWindow(1); got != 2*pollBaseDelay {
		t.Fatalf("second backoff window = %v, want %v", got, 2*pollBaseDelay)
	}
	for _, attempt := range []int{40, 1000} {
		if got := pollBackoffWindow(attempt); got != pollMaxDelay {
			t.Fatalf("backoff window at attempt %d = %v, want the %v cap", attempt, got, pollMaxDelay)
		}
	}
}

func TestAbandonUploadReleasesTheSession(t *testing.T) {
	wantPath := APIRootPath + "/artifacts/uploads/" + url.PathEscape("up_1")
	var sawDelete bool
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodDelete && r.URL.EscapedPath() == wantPath {
			sawDelete = true
			if r.Header.Get("Idempotency-Key") == "" {
				t.Error("upload abandon must send an Idempotency-Key")
			}
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		return false
	})
	if err := client.AbandonUpload(context.Background(), "up_1"); err != nil {
		t.Fatalf("abandon upload: %v", err)
	}
	if !sawDelete {
		t.Fatal("abandon upload never reached DELETE /artifacts/uploads/{uploadId}")
	}
}

func TestAbandonUploadRejectsAMalformedUploadID(t *testing.T) {
	client := newTestClient(t, nil)
	if err := client.AbandonUpload(context.Background(), "upload-1"); err == nil ||
		!contains(err, "uploadId must match") {
		t.Fatalf("malformed uploadId was accepted: %v", err)
	}
}

func TestAbandonUploadSurfacesTheTypedHostError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodDelete {
			writeStableError(t, w, http.StatusNotFound, "artifact_missing", false)
			return true
		}
		return false
	})
	err := client.AbandonUpload(context.Background(), "up_gone")
	if err == nil || !contains(err, "artifact_missing") {
		t.Fatalf("abandon upload swallowed the typed host error: %v", err)
	}
}
