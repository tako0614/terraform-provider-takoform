package clientv3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Operation wire constants (spec/schemas/operation-v1alpha1.schema.json).
const (
	OperationAPIVersion = "operations.takoform.com/v1alpha1"
	OperationKind       = "Operation"
)

// Operation polling policy: honor Retry-After when present, otherwise
// exponential backoff base 250ms doubling with full jitter, capped at 15s
// per interval, under an overall deadline (default 10 minutes).
const (
	pollBaseDelay          = 250 * time.Millisecond
	pollMaxDelay           = 15 * time.Second
	defaultOverallDeadline = 10 * time.Minute
)

var operationIDPattern = regexp.MustCompile(`^op_[A-Za-z0-9][A-Za-z0-9._-]{0,124}$`)

// OperationTarget names what a long-running operation acts on.
type OperationTarget struct {
	UID            string `json:"uid,omitempty"`
	ManifestDigest string `json:"manifestDigest,omitempty"`
}

// OperationMeta carries optional host progress reporting.
type OperationMeta struct {
	Phase    string `json:"phase,omitempty"`
	Progress *int   `json:"progress,omitempty"`
}

// OperationError is the terminal failure of an operation.
//
// RequestID is REQUIRED by the operation schema this lane selects: a failure a
// caller cannot correlate with a host-side record is a failure they cannot
// report. Decoding uses DisallowUnknownFields, so a field absent from this
// struct is not merely ignored — a conforming host's terminal error would be
// rejected as protocol-invalid.
type OperationError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
	Retryable bool   `json:"retryable"`
	HostCode  string `json:"hostCode,omitempty"`
}

// Operation is one resumable long-running operation. A terminal operation
// (Done true) carries exactly one of Result or Error.
type Operation struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	ID         string           `json:"id"`
	Done       bool             `json:"done"`
	Target     *OperationTarget `json:"target,omitempty"`
	Metadata   *OperationMeta   `json:"metadata,omitempty"`
	Result     json.RawMessage  `json:"result,omitempty"`
	Error      *OperationError  `json:"error,omitempty"`
}

func validateOperation(operation *Operation) error {
	if operation == nil {
		return errors.New("takoform: host omitted the operation")
	}
	if operation.APIVersion != OperationAPIVersion || operation.Kind != OperationKind {
		return errors.New("takoform: host operation identity is not operations.takoform.com/v1alpha1 Operation")
	}
	if !operationIDPattern.MatchString(operation.ID) {
		return errors.New("takoform: host operation id must match ^op_[A-Za-z0-9][A-Za-z0-9._-]{0,124}$")
	}
	hasResult := len(operation.Result) > 0
	hasError := operation.Error != nil
	if !operation.Done && (hasResult || hasError) {
		return errors.New("takoform: host operation carries a terminal payload before done")
	}
	if operation.Done && hasResult == hasError {
		return errors.New("takoform: terminal operation must carry exactly one of result or error")
	}
	if hasError {
		if _, closed := stableErrorHTTPStatusByCode[operation.Error.Code]; !closed {
			return fmt.Errorf("takoform: terminal operation error code %q is outside this lane's closed taxonomy", operation.Error.Code)
		}
		if strings.TrimSpace(operation.Error.RequestID) == "" {
			return errors.New("takoform: terminal operation error carries no requestId")
		}
		if operation.Error.Retryable {
			// The envelope's own rule: a terminal operation has stopped, so
			// there is nothing left to retry. Retrying is a NEW request, and a
			// true here would tell a client to poll a record that will never
			// move again.
			return errors.New("takoform: terminal operation error declares itself retryable")
		}
	}
	return nil
}

func (c *Client) decodeOperationEnvelope(data []byte, fullURL string) (*Operation, error) {
	var envelope struct {
		Operation *Operation `json:"operation"`
	}
	if err := decodeBody(data, fullURL, &envelope); err != nil {
		return nil, err
	}
	if err := validateOperation(envelope.Operation); err != nil {
		return nil, err
	}
	return envelope.Operation, nil
}

func (c *Client) operationURL(id, suffix string) string {
	u := c.apiBase + "/operations/" + id
	if suffix != "" {
		u += "/" + suffix
	}
	return u
}

// GetOperation reads one operation by its stable, resumable ID.
func (c *Client) GetOperation(ctx context.Context, id string) (*Operation, error) {
	operation, _, err := c.getOperation(ctx, id)
	return operation, err
}

func (c *Client) getOperation(ctx context.Context, id string) (*Operation, time.Duration, error) {
	if err := c.requireReady(); err != nil {
		return nil, 0, err
	}
	if !operationIDPattern.MatchString(id) {
		return nil, 0, errors.New("takoform: operation id must match ^op_[A-Za-z0-9][A-Za-z0-9._-]{0,124}$")
	}
	fullURL := c.operationURL(id, "")
	_, headers, data, err := c.do(ctx, http.MethodGet, fullURL, nil, nil, false, http.StatusOK)
	if err != nil {
		return nil, 0, err
	}
	var operation Operation
	if err := decodeBody(data, fullURL, &operation); err != nil {
		return nil, 0, err
	}
	if err := validateOperation(&operation); err != nil {
		return nil, 0, err
	}
	if operation.ID != id {
		return nil, 0, errors.New("takoform: host returned a different operation id")
	}
	return &operation, parseRetryAfter(headers.Get("Retry-After")), nil
}

// IsOperationNotFound reports the closed `operation_not_found` outcome: the
// host no longer holds a record for that id. The lane permits an operation
// record to expire, so a client resuming a persisted id must be able to tell
// "the host forgot this operation" apart from every other failure and fall
// back to reading the resource itself.
func IsOperationNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) &&
		apiErr.StatusCode == http.StatusNotFound &&
		apiErr.Code == "operation_not_found"
}

// OperationResultResource decodes and VERIFIES the resource a terminal,
// successful operation carries. The identity contract is the same one every
// direct response is held to: a result naming another FormRef, name, or space,
// or one missing host-owned identity, is refused rather than adopted. A caller
// resuming a persisted operation id has no other way to know that the result it
// is about to write into state describes the resource it asked about.
func OperationResultResource(operation *Operation, ref FormRef, name, space string) (*Resource, error) {
	if operation == nil || !operation.Done {
		return nil, errors.New("takoform: operation has no terminal result")
	}
	if operation.Error != nil {
		return nil, fmt.Errorf("takoform: operation %s terminated with %s", operation.ID, operation.Error.Code)
	}
	if len(operation.Result) == 0 {
		return nil, errors.New("takoform: terminal operation carries no result")
	}
	var envelope struct {
		Resource Resource `json:"resource"`
	}
	if err := decodeBody(operation.Result, "operation "+operation.ID+" result", &envelope); err != nil {
		return nil, fmt.Errorf("takoform: terminal operation result is not a resource envelope: %w", err)
	}
	out := envelope.Resource
	if err := verifyResourceResponse(ref, name, space, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelOperation requests cancellation. Cancel is honored only for safely
// stoppable operations; an already-terminal operation replays its terminal
// state.
func (c *Client) CancelOperation(ctx context.Context, id string) (*Operation, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if !operationIDPattern.MatchString(id) {
		return nil, errors.New("takoform: operation id must match ^op_[A-Za-z0-9][A-Za-z0-9._-]{0,124}$")
	}
	headers := map[string]string{
		"Idempotency-Key": mutationKey("operation-cancel", id),
	}
	fullURL := c.operationURL(id, "cancel")
	_, _, data, err := c.do(ctx, http.MethodPost, fullURL, headers, nil, true, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var operation Operation
	if err := decodeBody(data, fullURL, &operation); err != nil {
		return nil, err
	}
	if err := validateOperation(&operation); err != nil {
		return nil, err
	}
	// A cancel response naming a different operation is a substitution: the
	// caller would otherwise read another operation's terminal state as the
	// answer for the one it asked to stop.
	if operation.ID != id {
		return nil, errors.New("takoform: host returned a different operation id")
	}
	return &operation, nil
}

// AwaitOperation polls an operation ID to its terminal state. A positive
// deadline bounds this call explicitly. A non-positive deadline defers to the
// caller's context when ctx already carries a deadline; only a deadline-free
// context falls back to the client's overall deadline (default 10 minutes),
// so per-operation timeouts chosen by the caller are never silently cut
// short. A terminal error operation returns the operation together with a
// typed *APIError carrying the closed code semantics.
func (c *Client) AwaitOperation(ctx context.Context, id string, deadline time.Duration) (*Operation, error) {
	operation, retryAfter, err := c.getOperation(ctx, id)
	if err != nil {
		return nil, err
	}
	return c.awaitOperation(ctx, operation, retryAfter, deadline)
}

func (c *Client) awaitOperation(
	ctx context.Context,
	operation *Operation,
	retryAfter time.Duration,
	deadline time.Duration,
) (*Operation, error) {
	pollCtx := ctx
	cancel := func() {}
	if deadline > 0 {
		pollCtx, cancel = context.WithTimeout(ctx, deadline)
	} else if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		// The caller's context is authoritative: the client-wide overall
		// deadline is a fallback for deadline-free contexts only.
		pollCtx, cancel = context.WithTimeout(ctx, c.overallDeadline)
	}
	defer cancel()
	id := operation.ID
	for attempt := 0; ; attempt++ {
		if operation.Done {
			return finalizeOperation(operation)
		}
		if err := waitForPoll(pollCtx, attempt, retryAfter); err != nil {
			return nil, fmt.Errorf("takoform: operation %s not terminal before the overall deadline: %w", id, err)
		}
		var err error
		operation, retryAfter, err = c.getOperation(pollCtx, id)
		if err != nil {
			if pollCtx.Err() != nil {
				return nil, fmt.Errorf("takoform: operation %s not terminal before the overall deadline: %w", id, pollCtx.Err())
			}
			return nil, err
		}
	}
}

// finalizeOperation maps a terminal error into *APIError. The HTTP status is
// derived from the closed taxonomy when the code is portable; retryable is
// honored only for the automatically-retryable set.
func finalizeOperation(operation *Operation) (*Operation, error) {
	if operation.Error == nil {
		return operation, nil
	}
	_, allowedRetry := automaticallyRetryableErrorCodes[operation.Error.Code]
	return operation, &APIError{
		StatusCode: stableErrorHTTPStatusByCode[operation.Error.Code],
		Code:       operation.Error.Code,
		Message:    operation.Error.Message,
		HostCode:   operation.Error.HostCode,
		Retryable:  operation.Error.Retryable && allowedRetry,
	}
}

// pollBackoffWindow is the no-hint exponential window for one attempt,
// base*2^attempt capped at pollMaxDelay. The cap is unconditional here: with no
// host hint the client is guessing, and a guess must not grow without bound.
func pollBackoffWindow(attempt int) time.Duration {
	window := pollBaseDelay
	for i := 0; i < attempt && window < pollMaxDelay; i++ {
		window *= 2
	}
	if window > pollMaxDelay {
		window = pollMaxDelay
	}
	return window
}

// waitForPoll sleeps before the next poll. A host Retry-After hint wins and is
// honored in full, bounded only by the caller's deadline (retryAfterDelay);
// otherwise the backoff window base*2^attempt (capped at pollMaxDelay) is
// applied with full jitter.
func waitForPoll(ctx context.Context, attempt int, retryAfter time.Duration) error {
	var delay time.Duration
	if retryAfter > 0 {
		delay = retryAfterDelay(ctx, retryAfter, pollMaxDelay)
	} else {
		delay = time.Duration(rand.Int63n(int64(pollBackoffWindow(attempt))))
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
