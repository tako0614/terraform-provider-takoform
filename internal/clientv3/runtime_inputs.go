package clientv3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	runtimeInputPreparationFormat           = "takoserver.worker-runtime-input-preparation@v2"
	runtimeInputPreparationRoot             = "/v1/takoform/worker-runtime-input-preparations"
	runtimeInputPublicApplyCommitmentLabel  = "takoserver.worker-runtime-input-public-apply@v1"
	runtimeInputPublicApplyMaximumPathBytes = 8 << 10
	runtimeInputPublicApplyMaximumBodyBytes = 1 << 20
	runtimeInputPreparationMaximumBindings  = 64
	runtimeInputMaximumValueBytes           = 32 << 10
)

var runtimeInputBindingNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// RuntimeInputPublicApplyCommitment is the cross-implementation commitment to
// the exact ordinary public apply request. Each field in
// [label, method, path, ifNoneMatch, body] is UTF-8 preceded by its unsigned
// 64-bit big-endian byte length; SHA-256 is rendered as lowercase hex.
func RuntimeInputPublicApplyCommitment(method, path, ifNoneMatch string, body []byte) (string, error) {
	if method != http.MethodPut {
		return "", errors.New("takoform: runtime input public apply method must be PUT")
	}
	if len(path) < 1 || len(path) > runtimeInputPublicApplyMaximumPathBytes || path[0] != '/' || !utf8.ValidString(path) {
		return "", errors.New("takoform: runtime input public apply path is invalid or overlong")
	}
	if ifNoneMatch != "*" {
		return "", errors.New("takoform: runtime input public apply fence must be If-None-Match: *")
	}
	if len(body) < 1 || len(body) > runtimeInputPublicApplyMaximumBodyBytes || !utf8.Valid(body) {
		return "", errors.New("takoform: runtime input public apply body is invalid UTF-8 or overlong")
	}
	hash := sha256.New()
	fields := [][]byte{
		[]byte(runtimeInputPublicApplyCommitmentLabel),
		[]byte(method),
		[]byte(path),
		[]byte(ifNoneMatch),
		body,
	}
	var prefix [8]byte
	for _, field := range fields {
		binary.BigEndian.PutUint64(prefix[:], uint64(len(field)))
		_, _ = hash.Write(prefix[:])
		_, _ = hash.Write(field)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

type runtimeInputMaterial struct {
	CanonicalPublicOrigin string
	Bindings              map[string][]byte
}

// runtimeInputExpectation is the value-free part retained after the one
// private request has consumed and cleared every binding value. Public apply
// recovery needs only this identity to validate the private GET response.
type runtimeInputExpectation struct {
	CanonicalPublicOrigin string
	BindingNames          []string
	ApplyCommitment       string
}

type runtimeInputPreparation struct {
	Format                 string   `json:"format"`
	Status                 string   `json:"status"`
	OperationKey           string   `json:"operationKey"`
	ApplyCommitment        string   `json:"applyCommitment"`
	CanonicalPublicOrigin  string   `json:"canonicalPublicOrigin"`
	BindingNames           []string `json:"bindingNames"`
	HostOperationID        string   `json:"hostOperationId,omitempty"`
	recoveredAfterPutError bool
}

// RuntimeInputApplyIndeterminateError means the private preparation exists but
// has no ordinary Host operation to poll after the public PUT acknowledgement
// was lost. The provider must not replay the PUT.
type RuntimeInputApplyIndeterminateError struct {
	OperationKey string
}

func (err *RuntimeInputApplyIndeterminateError) Error() string {
	return "takoform: public WorkerVersion apply acknowledgement was lost while the runtime input preparation remains only prepared; the provider will not replay the public PUT"
}

// RuntimeInputApplyError is a closed, value-free failure from the sensitive
// runtime-input path. Code is selected by this client, never copied from a
// Host response. In particular, Host messages, details, host codes, request
// IDs, response bodies, and transport errors are not retained in this error.
type RuntimeInputApplyError struct {
	Code string
}

func (err *RuntimeInputApplyError) Error() string {
	return "takoform: runtime input apply failed [" + err.Code + "]"
}

const (
	runtimeInputErrorPrerequisiteUnavailable    = "prerequisite_unavailable"
	runtimeInputErrorPreparationLookupFailed    = "preparation_lookup_failed"
	runtimeInputErrorPreparationRejected        = "preparation_rejected"
	runtimeInputErrorPreparationAckUnavailable  = "preparation_acknowledgement_unavailable"
	runtimeInputErrorPreparationResponseInvalid = "preparation_response_invalid"
	runtimeInputErrorApplyCommitmentMismatch    = "apply_commitment_mismatch"
	runtimeInputErrorPublicApplyRejected        = "public_apply_rejected"
	runtimeInputErrorPublicApplyRecoveryFailed  = "public_apply_recovery_failed"
	runtimeInputErrorOperationPollFailed        = "operation_poll_failed"
	runtimeInputErrorOperationResultInvalid     = "operation_result_invalid"
)

var errRuntimeInputApplyCommitmentMismatch = errors.New("runtime input apply commitment mismatch")

func runtimeInputApplyError(code string) error {
	return &RuntimeInputApplyError{Code: code}
}

func runtimeInputPreparationPath(operationKey string) string {
	return runtimeInputPreparationRoot + "/" + url.PathEscape(operationKey)
}

func (c *Client) prepareRuntimeInputs(
	ctx context.Context,
	operationKey string,
	material *runtimeInputMaterial,
	expectation *runtimeInputExpectation,
	publicPath string,
	publicBody []byte,
) (*runtimeInputPreparation, error) {
	defer clearRuntimeInputMaterial(material)
	existing, err := c.getRuntimeInputPreparation(ctx, operationKey, expectation)
	if err == nil {
		return existing, nil
	}
	if !runtimeInputPreparationAbsent(err) {
		return nil, sanitizeRuntimeInputPreparationReadError(err, runtimeInputErrorPreparationLookupFailed)
	}
	// The exact not-found classification is all that survives into the private
	// PUT. Drop the Host error envelope before any plaintext values are sent.
	err = nil
	result, acknowledgementUncertain, err := c.putRuntimeInputPreparation(
		ctx, operationKey, material, expectation, publicPath, publicBody,
	)
	if err == nil {
		return result, nil
	}
	if !acknowledgementUncertain {
		return nil, err
	}
	var putAPIError *APIError
	putWasRejection := errors.As(err, &putAPIError)
	putCommitmentMismatch := errors.Is(err, errRuntimeInputApplyCommitmentMismatch)
	// Do not retain a Host-controlled error (which may reflect a submitted
	// value) while the value-free recovery request runs. Only these two local
	// classifications survive beyond this point.
	putAPIError = nil
	err = nil
	// A private PUT whose acknowledgement is lost or malformed is never sent
	// again: its body contains the only plaintext values in this protocol. The
	// PUT helper has returned, so its encoded body and binding map are already
	// cleared before this value-free readback begins.
	recoveryCtx, cancel := c.runtimeInputRecoveryContext()
	defer cancel()
	result, readbackErr := c.getRuntimeInputPreparation(recoveryCtx, operationKey, expectation)
	if readbackErr != nil {
		if runtimeInputPreparationAbsent(readbackErr) {
			if putWasRejection {
				return nil, runtimeInputApplyError(runtimeInputErrorPreparationRejected)
			}
			return nil, runtimeInputApplyError(runtimeInputErrorPreparationAckUnavailable)
		}
		return nil, sanitizeRuntimeInputPreparationReadError(readbackErr, runtimeInputErrorPreparationAckUnavailable)
	}
	if putCommitmentMismatch {
		// The PUT acknowledgement itself was bound to a different public apply.
		// The required GET still establishes the value-free recovery state, but
		// it cannot make that mismatched acknowledgement safe to act on during
		// this run. Stop before public mutation or operation polling.
		return nil, runtimeInputApplyError(runtimeInputErrorApplyCommitmentMismatch)
	}
	result.recoveredAfterPutError = true
	return result, nil
}

func (c *Client) runtimeInputRecoveryContext() (context.Context, context.CancelFunc) {
	deadline := c.overallDeadline
	if deadline <= 0 {
		deadline = defaultOverallDeadline
	}
	return context.WithTimeout(context.Background(), deadline)
}

func runtimeInputPreparationAbsent(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound &&
		apiErr.Code == "operation_not_found" && !apiErr.ProtocolInvalid
}

func (c *Client) putRuntimeInputPreparation(
	ctx context.Context,
	operationKey string,
	material *runtimeInputMaterial,
	expectation *runtimeInputExpectation,
	publicPath string,
	publicBody []byte,
) (*runtimeInputPreparation, bool, error) {
	defer clearRuntimeInputMaterial(material)
	raw, err := encodeRuntimeInputPrepareRequest(material, publicPath, publicBody)
	if err != nil {
		return nil, false, runtimeInputApplyError(runtimeInputErrorPreparationResponseInvalid)
	}
	defer clearRuntimeInputBytes(raw)
	path := runtimeInputPreparationPath(operationKey)
	headers := map[string]string{"Idempotency-Key": operationKey}
	_, _, data, err := c.do(
		ctx,
		http.MethodPut,
		c.endpoint+path,
		headers,
		raw,
		false,
		http.StatusOK,
	)
	defer clearRuntimeInputBytes(data)
	if err != nil {
		// Every private PUT failure is acknowledgement-uncertain until a fresh,
		// bounded, value-free GET says otherwise. This includes structured Host
		// rejections: the response is untrusted and may reflect submitted values.
		return nil, true, err
	}
	result, err := c.decodeRuntimeInputPreparation(data, c.endpoint+path, operationKey, expectation)
	if err != nil {
		return nil, true, err
	}
	return result, false, nil
}

// encodeRuntimeInputPrepareRequest avoids putting plaintext binding values in
// an immutable string-valued SDK request struct. The returned mutable buffer is
// wiped promptly by the caller after the transport returns. Go, net/http, TLS,
// and the compiler may still make copies outside this buffer; this is
// best-effort lifetime reduction, not a memory-erasure guarantee.
func encodeRuntimeInputPrepareRequest(
	material *runtimeInputMaterial,
	publicPath string,
	publicBody []byte,
) ([]byte, error) {
	if material == nil || !utf8.ValidString(material.CanonicalPublicOrigin) ||
		!utf8.ValidString(publicPath) || !utf8.Valid(publicBody) {
		return nil, errors.New("runtime input preparation has invalid UTF-8")
	}
	names := sortedRuntimeInputBindingNames(material.Bindings)
	capacity := len(`{"format":`) + runtimeInputJSONStringSize([]byte(runtimeInputPreparationFormat)) +
		len(`,"canonicalPublicOrigin":`) + runtimeInputJSONStringSize([]byte(material.CanonicalPublicOrigin)) +
		len(`,"publicApply":{"method":"PUT","path":`) + runtimeInputJSONStringSize([]byte(publicPath)) +
		len(`,"fences":{"ifNoneMatch":"*"},"body":`) + runtimeInputJSONStringSize(publicBody) +
		len(`},"bindings":{`) + len(`}}`)
	for index, name := range names {
		if index > 0 {
			capacity++
		}
		capacity += runtimeInputJSONStringSize([]byte(name)) + 1 +
			runtimeInputJSONStringSize(material.Bindings[name])
	}
	// Exact preallocation prevents intermediate grow-and-copy buffers from
	// retaining additional plaintext binding copies until garbage collection.
	raw := make([]byte, 0, capacity)
	raw = append(raw, `{"format":`...)
	raw = appendRuntimeInputJSONString(raw, []byte(runtimeInputPreparationFormat))
	raw = append(raw, `,"canonicalPublicOrigin":`...)
	raw = appendRuntimeInputJSONString(raw, []byte(material.CanonicalPublicOrigin))
	raw = append(raw, `,"publicApply":{"method":"PUT","path":`...)
	raw = appendRuntimeInputJSONString(raw, []byte(publicPath))
	raw = append(raw, `,"fences":{"ifNoneMatch":"*"},"body":`...)
	raw = appendRuntimeInputJSONString(raw, publicBody)
	raw = append(raw, `},"bindings":{`...)
	for index, name := range names {
		if index > 0 {
			raw = append(raw, ',')
		}
		raw = appendRuntimeInputJSONString(raw, []byte(name))
		raw = append(raw, ':')
		raw = appendRuntimeInputJSONString(raw, material.Bindings[name])
	}
	raw = append(raw, '}', '}')
	return raw, nil
}

func runtimeInputJSONStringSize(value []byte) int {
	size := 2 // surrounding quotes
	for _, current := range value {
		switch current {
		case '"', '\\', '\b', '\f', '\n', '\r', '\t':
			size += 2
		default:
			if current < 0x20 {
				size += 6
			} else {
				size++
			}
		}
	}
	return size
}

func appendRuntimeInputJSONString(dst, value []byte) []byte {
	const hexDigits = "0123456789abcdef"
	dst = append(dst, '"')
	for _, current := range value {
		switch current {
		case '"', '\\':
			dst = append(dst, '\\', current)
		case '\b':
			dst = append(dst, `\b`...)
		case '\f':
			dst = append(dst, `\f`...)
		case '\n':
			dst = append(dst, `\n`...)
		case '\r':
			dst = append(dst, `\r`...)
		case '\t':
			dst = append(dst, `\t`...)
		default:
			if current < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[current>>4], hexDigits[current&0xf])
			} else {
				dst = append(dst, current)
			}
		}
	}
	return append(dst, '"')
}

func (c *Client) getRuntimeInputPreparation(
	ctx context.Context,
	operationKey string,
	expectation *runtimeInputExpectation,
) (*runtimeInputPreparation, error) {
	path := runtimeInputPreparationPath(operationKey)
	_, _, data, err := c.do(
		ctx,
		http.MethodGet,
		c.endpoint+path,
		map[string]string{"Idempotency-Key": operationKey},
		nil,
		false,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}
	defer clearRuntimeInputBytes(data)
	return c.decodeRuntimeInputPreparation(data, c.endpoint+path, operationKey, expectation)
}

func (c *Client) decodeRuntimeInputPreparation(
	data []byte,
	fullURL, operationKey string,
	expectation *runtimeInputExpectation,
) (*runtimeInputPreparation, error) {
	var result runtimeInputPreparation
	if err := decodeBody(data, fullURL, &result); err != nil {
		return nil, err
	}
	if result.Format != runtimeInputPreparationFormat {
		return nil, errors.New("takoform: runtime input preparation response has the wrong format")
	}
	if !slices.Contains([]string{"prepared", "accepted", "dispatched", "consumed"}, result.Status) {
		return nil, errors.New("takoform: runtime input preparation response has an invalid status")
	}
	if result.OperationKey != operationKey {
		return nil, errors.New("takoform: runtime input preparation response changed the operation key")
	}
	if expectation == nil || result.CanonicalPublicOrigin != expectation.CanonicalPublicOrigin {
		return nil, errors.New("takoform: runtime input preparation response changed the canonical public origin")
	}
	if result.ApplyCommitment != expectation.ApplyCommitment {
		return nil, errRuntimeInputApplyCommitmentMismatch
	}
	if !slices.Equal(result.BindingNames, expectation.BindingNames) {
		return nil, errors.New("takoform: runtime input preparation response changed the binding-name set")
	}
	if result.Status == "prepared" && result.HostOperationID != "" {
		return nil, errors.New("takoform: prepared runtime inputs unexpectedly name a Host operation")
	}
	if result.Status != "prepared" && !operationIDPattern.MatchString(result.HostOperationID) {
		return nil, errors.New("takoform: accepted runtime inputs do not name a valid Host operation")
	}
	return &result, nil
}

func validateRuntimeInputMaterial(origin, operationKey string, material *runtimeInputMaterial) error {
	if material == nil || material.CanonicalPublicOrigin != origin {
		return errors.New("takoform: runtime input origin must exactly match the configured Host origin")
	}
	parsed, err := url.Parse(material.CanonicalPublicOrigin)
	if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("takoform: runtime input origin must be an absolute HTTPS origin")
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || parsed.Host != strings.ToLower(parsed.Host) ||
		parsed.Port() == "443" || parsed.String() != material.CanonicalPublicOrigin {
		return errors.New("takoform: runtime input origin must be in exact canonical HTTPS origin form")
	}
	if err := ValidateIdempotencyKey(operationKey); err != nil {
		return err
	}
	if len(material.Bindings) < 1 || len(material.Bindings) > runtimeInputPreparationMaximumBindings {
		return errors.New("takoform: runtime input binding count must be in 1..64")
	}
	for name, value := range material.Bindings {
		if !runtimeInputBindingNamePattern.MatchString(name) {
			return errors.New("takoform: runtime input binding name is invalid")
		}
		if !utf8.Valid(value) || bytes.IndexByte(value, 0) >= 0 {
			return errors.New("takoform: runtime input value must be UTF-8 text without NUL")
		}
		if len(value) < 1 || len(value) > runtimeInputMaximumValueBytes {
			return errors.New("takoform: runtime input value size must be in 1..32768 bytes")
		}
	}
	return nil
}

func runtimeInputExpectationFor(
	origin, operationKey string,
	material *runtimeInputMaterial,
) (*runtimeInputExpectation, error) {
	if err := validateRuntimeInputMaterial(origin, operationKey, material); err != nil {
		return nil, err
	}
	return &runtimeInputExpectation{
		CanonicalPublicOrigin: material.CanonicalPublicOrigin,
		BindingNames:          sortedRuntimeInputBindingNames(material.Bindings),
	}, nil
}

func clearRuntimeInputMaterial(material *runtimeInputMaterial) {
	if material == nil {
		return
	}
	for name := range material.Bindings {
		clearRuntimeInputBytes(material.Bindings[name])
		material.Bindings[name] = nil
		delete(material.Bindings, name)
	}
	material.Bindings = nil
}

func sortedRuntimeInputBindingNames(bindings map[string][]byte) []string {
	names := make([]string, 0, len(bindings))
	for name := range bindings {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func clearRuntimeInputBytes(raw []byte) {
	for index := range raw {
		raw[index] = 0
	}
}

func sanitizeRuntimeInputPreparationReadError(err error, fallbackCode string) error {
	if errors.Is(err, errRuntimeInputApplyCommitmentMismatch) {
		return runtimeInputApplyError(runtimeInputErrorApplyCommitmentMismatch)
	}
	return runtimeInputApplyError(fallbackCode)
}

func (c *Client) recoverRuntimeInputPublicApply(
	operationKey string,
	expectation *runtimeInputExpectation,
	ref FormRef,
	name, space string,
	publicRejected bool,
) (*Resource, error) {
	recoveryCtx, cancel := c.runtimeInputRecoveryContext()
	defer cancel()
	preparation, err := c.getRuntimeInputPreparation(recoveryCtx, operationKey, expectation)
	if err != nil {
		return nil, sanitizeRuntimeInputPreparationReadError(err, runtimeInputErrorPublicApplyRecoveryFailed)
	}
	if preparation.HostOperationID == "" {
		if publicRejected {
			return nil, runtimeInputApplyError(runtimeInputErrorPublicApplyRejected)
		}
		return nil, &RuntimeInputApplyIndeterminateError{OperationKey: operationKey}
	}
	return c.finishRuntimeInputOperation(recoveryCtx, preparation, ref, name, space)
}

func (c *Client) finishRuntimeInputOperation(
	ctx context.Context,
	preparation *runtimeInputPreparation,
	ref FormRef,
	name, space string,
) (*Resource, error) {
	operation, err := c.AwaitOperation(ctx, preparation.HostOperationID, 0)
	if err != nil {
		uid := operationTargetUID(operation)
		releaseRuntimeInputOperation(operation)
		err = nil
		return nil, acceptedMutation(
			preparation.HostOperationID,
			uid,
			runtimeInputApplyError(runtimeInputErrorOperationPollFailed),
		)
	}
	resource, err := OperationResultResource(operation, ref, name, space)
	if err != nil {
		uid := operationTargetUID(operation)
		releaseRuntimeInputOperation(operation)
		err = nil
		return nil, acceptedMutation(
			preparation.HostOperationID,
			uid,
			runtimeInputApplyError(runtimeInputErrorOperationResultInvalid),
		)
	}
	releaseRuntimeInputOperation(operation)
	return resource, nil
}

func releaseRuntimeInputOperation(operation *Operation) {
	if operation == nil {
		return
	}
	clearRuntimeInputBytes(operation.Result)
	operation.Result = nil
	if operation.Error != nil {
		operation.Error.Code = ""
		operation.Error.Message = ""
		operation.Error.RequestID = ""
		operation.Error.HostCode = ""
		operation.Error = nil
	}
}
