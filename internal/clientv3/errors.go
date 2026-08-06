package clientv3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is returned only for the stable resource_not_found error on
// HTTP 404 (or the same code on a terminal delete Operation). Callers map
// this exact condition to "remove from state".
var ErrNotFound = errors.New("takoform: resource not found")

// AcceptedError reports that the host ACCEPTED a mutation and the call failed
// only afterwards: the PUT/POST returned a success status (200, 201, or a 202
// Operation) and something after that point — the operation never reaching a
// terminal state before the deadline, a terminal operation error, or an
// unreadable/unverifiable representation — produced this error.
//
// The distinction matters to every caller that persists state. An error that
// arrives BEFORE acceptance means nothing was mutated and retrying is safe. An
// AcceptedError means the resource may already exist, or may still be being
// created, even though no verified representation came back. A caller that
// simply drops the error on the floor and writes no state orphans the
// resource: the host owns it and the next plan proposes creating it again.
// Such a caller MUST record the identity it knows (name, space, exact FormRef)
// together with OperationID so the next plan can reconcile instead of
// duplicating.
type AcceptedError struct {
	// OperationID is the host-issued resumable operation id when the mutation
	// was accepted as a 202 Operation, and empty otherwise (a direct 200/201
	// acceptance, or a 202 whose operation envelope could not be read).
	OperationID string
	// UID is the host-issued resource identity when the accepted mutation
	// disclosed one through the operation target. It is empty otherwise.
	UID string
	// Err is the underlying failure.
	Err error
}

func (e *AcceptedError) Error() string {
	var b strings.Builder
	b.WriteString("takoform: the host accepted this mutation but it did not complete verifiably")
	if e.OperationID != "" {
		fmt.Fprintf(&b, " (operation %s)", e.OperationID)
	}
	if e.UID != "" {
		fmt.Fprintf(&b, " (uid %s)", e.UID)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

func (e *AcceptedError) Unwrap() error { return e.Err }

// acceptedMutation wraps err as an accepted-but-unverified mutation. A nil err
// stays nil so callers can wrap unconditionally.
func acceptedMutation(operationID, uid string, err error) error {
	if err == nil {
		return nil
	}
	return &AcceptedError{OperationID: operationID, UID: uid, Err: err}
}

// APIError is the typed form of the v1alpha3 error envelope for non-2xx
// responses and for terminal Operation errors. The wire envelope is nested:
//
//	{ "error": { "code": "<code>", "message": "<msg>", "requestId": "<id>", "retryable": false } }
type APIError struct {
	// StatusCode is the HTTP status; it is not part of the wire body. It is
	// zero for an error surfaced through a terminal Operation.
	StatusCode int
	Code       string
	Message    string
	RequestID  string
	Retryable  bool
	HostCode   string
	// RetryAfter is the parsed Retry-After header of the error response
	// (seconds or HTTP-date), or zero when absent or unparseable. When set,
	// it takes priority over the exponential backoff for the next attempt.
	RetryAfter time.Duration
	// ProtocolInvalid means the response did not match the closed stable
	// error taxonomy and therefore carries no portable code semantics.
	ProtocolInvalid bool
	// Details is the optional, free-form details payload, kept raw.
	Details json.RawMessage
}

// stableErrorHTTPStatusByCode is the closed v1alpha3 taxonomy, exactly
// spec/host-api/operations-v1alpha3.json errorEnvelope.httpStatusByCode.
var stableErrorHTTPStatusByCode = map[string]int{
	"invalid_argument":       http.StatusBadRequest,
	"unauthenticated":        http.StatusUnauthorized,
	"permission_denied":      http.StatusForbidden,
	"form_unknown":           http.StatusNotFound,
	"form_not_installed":     http.StatusConflict,
	"form_unavailable":       http.StatusConflict,
	"form_identity_conflict": http.StatusConflict,
	"resource_not_found":     http.StatusNotFound,
	"resource_busy":          http.StatusConflict,
	"import_conflict":        http.StatusConflict,
	"policy_denied":          http.StatusForbidden,
	"backend_unavailable":    http.StatusServiceUnavailable,
	"internal_error":         http.StatusInternalServerError,
	"rate_limited":           http.StatusTooManyRequests,
	"deadline_exceeded":      http.StatusGatewayTimeout,
	"operation_cancelled":    http.StatusConflict,
	"operation_not_found":    http.StatusNotFound,
	"dependency_in_use":      http.StatusConflict,
	"deletion_protected":     http.StatusConflict,
	"artifact_missing":       http.StatusNotFound,
	"artifact_invalid":       http.StatusBadRequest,
	"unsupported_capability": http.StatusUnprocessableEntity,
	"migration_required":     http.StatusConflict,
	"uid_mismatch":           http.StatusConflict,
	"revision_conflict":      http.StatusPreconditionFailed,
	"generation_conflict":    http.StatusPreconditionFailed,
}

// automaticallyRetryableErrorCodes are the only codes a host may mark
// retryable: true.
var automaticallyRetryableErrorCodes = map[string]struct{}{
	"resource_busy":       {},
	"backend_unavailable": {},
	"rate_limited":        {},
	"deadline_exceeded":   {},
}

// errorEnvelope decodes the nested wire shape of an error response.
type errorEnvelope struct {
	Error struct {
		Code      string          `json:"code"`
		Message   string          `json:"message"`
		RequestID string          `json:"requestId"`
		Retryable *bool           `json:"retryable"`
		HostCode  string          `json:"hostCode,omitempty"`
		Details   json.RawMessage `json:"details,omitempty"`
	} `json:"error"`
}

func (e *APIError) Error() string {
	var b strings.Builder
	if e.ProtocolInvalid {
		b.WriteString("takoform protocol-invalid error response")
	} else {
		b.WriteString("takoform api error")
	}
	if e.StatusCode != 0 {
		fmt.Fprintf(&b, " (http %d)", e.StatusCode)
	}
	if e.Code != "" {
		fmt.Fprintf(&b, " [%s]", e.Code)
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (requestId=%s)", e.RequestID)
	}
	return b.String()
}

func isResourceNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) &&
		apiErr.StatusCode == http.StatusNotFound &&
		apiErr.Code == "resource_not_found"
}

// parseAPIError decodes the nested error envelope, falling back to a
// protocol-invalid error when the response is not the closed stable shape.
func parseAPIError(status int, data []byte, retryAfterHeader string) *APIError {
	apiErr := &APIError{
		StatusCode: status, ProtocolInvalid: true,
		RetryAfter: parseRetryAfter(retryAfterHeader),
	}
	if len(bytes.TrimSpace(data)) > 0 {
		var env errorEnvelope
		if err := decodeStrictJSON(data, &env); err == nil &&
			strings.TrimSpace(env.Error.Code) != "" &&
			strings.TrimSpace(env.Error.Message) != "" &&
			strings.TrimSpace(env.Error.RequestID) != "" &&
			env.Error.Retryable != nil &&
			isStableErrorEnvelope(status, env.Error.Code, *env.Error.Retryable) {
			apiErr.Code = env.Error.Code
			apiErr.Message = env.Error.Message
			apiErr.RequestID = env.Error.RequestID
			apiErr.Retryable = *env.Error.Retryable
			apiErr.HostCode = env.Error.HostCode
			apiErr.Details = env.Error.Details
			apiErr.ProtocolInvalid = false
		}
	}
	if apiErr.Message == "" {
		if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
			apiErr.Message = trimmed
		} else {
			apiErr.Message = http.StatusText(status)
		}
	}
	return apiErr
}

// parseRetryAfter reads the Retry-After header as delta-seconds or an
// HTTP-date, returning the suggested wait. Zero means "no usable hint".
func parseRetryAfter(value string) time.Duration {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(trimmed); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(trimmed); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

// retryAfterDelay returns how long to wait when the host supplied an explicit
// Retry-After hint. The hint is the host's own statement about when the next
// attempt can succeed, so it is honored in full: shortening it to a client-side
// per-interval cap only produces earlier, pointless traffic against a host that
// already said "not yet". The single legitimate bound is the caller's own
// deadline, and the callers' select on ctx.Done() already enforces that, so no
// second clamp is applied here.
//
// fallbackCap applies only to a context that carries no deadline at all, where
// nothing else would bound the wait; the no-hint exponential-backoff path keeps
// the cap unconditionally.
func retryAfterDelay(ctx context.Context, retryAfter, fallbackCap time.Duration) time.Duration {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return retryAfter
	}
	if retryAfter > fallbackCap {
		return fallbackCap
	}
	return retryAfter
}

func isStableErrorEnvelope(status int, code string, retryable bool) bool {
	expectedStatus, known := stableErrorHTTPStatusByCode[code]
	if !known || status != expectedStatus {
		return false
	}
	if retryable {
		_, allowed := automaticallyRetryableErrorCodes[code]
		return allowed
	}
	return true
}

// isPortableRetryable permits an automatic retry only for a complete stable
// envelope whose code is in the closed automatically-retryable set. A
// protocol-invalid body never earns a retry.
func isPortableRetryable(apiErr *APIError) bool {
	if apiErr == nil || apiErr.ProtocolInvalid || !apiErr.Retryable {
		return false
	}
	return isStableErrorEnvelope(apiErr.StatusCode, apiErr.Code, true)
}
