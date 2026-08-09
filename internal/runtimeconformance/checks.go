package runtimeconformance

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/fakeruntime"
)

// target is the runtime under test: the deployed worker, and — when the
// operator supplied one — the adapter over its module loader.
type target struct {
	client      *http.Client
	worker      string
	loader      string
	token       string
	loaderToken string

	// runToken is this run's correlation token. Every correlated check
	// resolves its pinned template against it, so the values that reach the
	// runtime belong to this run and the observations that come back can be
	// told apart from the ones a previous run against the same deployment
	// left in the `edge.kv` namespace.
	runToken string
}

// correlate resolves one pinned template into the value this run uses. The
// value the runner sends and the observation it expects come from this one
// call, so they cannot drift apart.
func (t *target) correlate(contract Contract, check Check) string {
	return contract.RunCorrelation.Resolve(check.Payload.NonceTemplate, t.runToken)
}

// runCheck executes one check and records what was observed. A check never
// panics a run: a transport failure is a failed check with its diagnostic, so
// one broken route does not hide the state of the rest of the ABI.
func runCheck(ctx context.Context, contract Contract, target *target, check Check) CheckEvidence {
	evidence := CheckEvidence{
		Name:      check.Name,
		Operation: check.Operation,
		Procedure: check.Procedure,
		Proves:    check.Proves,
	}
	var observed string
	var err error
	switch check.Procedure {
	case ProcedureLoad:
		observed, err = runLoad(ctx, contract, target, check)
		if errors.Is(err, errLoaderAbsent) {
			evidence.Outcome = OutcomeUnmeasured
			evidence.Observed = observed
			evidence.Detail = err.Error()
			return evidence
		}
	case ProcedureRequest:
		observed, err = runRequest(ctx, target, check)
	case ProcedureEnvironment:
		observed, err = runEnvironment(ctx, contract, target, check)
	case ProcedureGlobals:
		observed, err = runGlobals(ctx, contract, target, check)
	case ProcedureKVRoundTrip:
		observed, err = runKVRoundTrip(ctx, contract, target, check)
	case ProcedureRequestStream:
		observed, err = runRequestStream(ctx, contract, target, check)
	case ProcedureResponseStream:
		observed, err = runResponseStream(ctx, contract, target, check)
	case ProcedureWaitUntil:
		observed, err = runWaitUntil(ctx, contract, target, check)
	case ProcedureScheduledObservation:
		observed, err = runScheduledObservation(ctx, contract, target, check)
	case ProcedureQueueRoundTrip:
		observed, err = runQueueRoundTrip(ctx, contract, target, check)
	case ProcedureUnmeasured:
		evidence.Outcome = OutcomeUnmeasured
		evidence.Observed = check.Unmeasured.Reason
		evidence.Detail = check.Unmeasured.ClosedBy
		return evidence
	default:
		evidence.Outcome = OutcomeFailed
		evidence.Detail = fmt.Sprintf("unknown procedure %q", check.Procedure)
		return evidence
	}
	evidence.Observed = observed
	if err != nil {
		evidence.Outcome = OutcomeFailed
		evidence.Detail = err.Error()
		return evidence
	}
	evidence.Outcome = OutcomePassed
	return evidence
}

var errLoaderAbsent = errors.New(
	"no module-loader adapter was supplied, so the load lane was not measured; " +
		"loadModule outcomes are decided before any traffic arrives and no request to a running worker can observe them")

// runLoad hands the loader a bundle's exact bytes and reads back what it made
// of them.
func runLoad(ctx context.Context, contract Contract, target *target, check Check) (string, error) {
	if target.loader == "" {
		return "not measured", errLoaderAbsent
	}
	bundle, ok := contract.Bundle(check.Bundle)
	if !ok {
		return "", fmt.Errorf("unknown bundle %q", check.Bundle)
	}
	body, err := json.Marshal(loadRequestFor(contract, bundle, check.Load.DeclaredHandlers))
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.loader, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("content-type", "application/json")
	if target.loaderToken != "" {
		request.Header.Set("authorization", "Bearer "+target.loaderToken)
	}
	response, err := target.client.Do(request)
	if err != nil {
		return "", err
	}
	defer closeBody(response)
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var decoded fakeruntime.LoadResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("loader answered %d with a body that is not a load response: %w", response.StatusCode, err)
	}
	if check.Load.ExpectError != "" {
		if decoded.Error == nil {
			return fmt.Sprintf("exportedHandlers=%v", decoded.ExportedHandlers),
				fmt.Errorf("expected %s; the loader accepted the bundle", check.Load.ExpectError)
		}
		if decoded.Error.Code != check.Load.ExpectError {
			return "error=" + decoded.Error.Code,
				fmt.Errorf("expected %s; the loader answered %s", check.Load.ExpectError, decoded.Error.Code)
		}
		return "error=" + decoded.Error.Code, nil
	}
	if decoded.Error != nil {
		return "error=" + decoded.Error.Code,
			fmt.Errorf("expected exports %v; the loader answered %s",
				check.Load.ExpectExportedHandlers, decoded.Error.Code)
	}
	if !reflect.DeepEqual(decoded.ExportedHandlers, check.Load.ExpectExportedHandlers) {
		return fmt.Sprintf("exportedHandlers=%v", decoded.ExportedHandlers),
			fmt.Errorf("expected exports %v", check.Load.ExpectExportedHandlers)
	}
	return fmt.Sprintf("exportedHandlers=%v", decoded.ExportedHandlers), nil
}

// runRequest drives one probe route and compares the whole observation.
func runRequest(ctx context.Context, target *target, check Check) (string, error) {
	response, raw, err := target.request(ctx, check.Request.Method, check.Request.Path, nil)
	if err != nil {
		return "", err
	}
	if response.StatusCode != check.Expect.Status {
		return "status=" + strconv.Itoa(response.StatusCode),
			fmt.Errorf("expected status %d", check.Expect.Status)
	}
	if check.Expect.JSON == nil {
		return "status=" + strconv.Itoa(response.StatusCode), nil
	}
	document, err := decodeJSONObject(raw)
	if err != nil {
		return "status=" + strconv.Itoa(response.StatusCode), err
	}
	if !reflect.DeepEqual(document, check.Expect.JSON) {
		return compactJSON(raw), fmt.Errorf("observation differs from the expected one")
	}
	return compactJSON(raw), nil
}

// runEnvironment reads the own enumerable property names of `env`. The
// expectation is the deployment's own declaration, never a second copy of it.
func runEnvironment(ctx context.Context, contract Contract, target *target, check Check) (string, error) {
	response, raw, err := target.request(ctx, check.Request.Method, check.Request.Path, nil)
	if err != nil {
		return "", err
	}
	if response.StatusCode != check.Expect.Status {
		return "status=" + strconv.Itoa(response.StatusCode),
			fmt.Errorf("expected status %d", check.Expect.Status)
	}
	var document struct {
		Probe         string   `json:"probe"`
		PropertyNames []string `json:"propertyNames"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return compactJSON(raw), err
	}
	if document.Probe != contract.ProbeProtocol.Protocol {
		return compactJSON(raw), fmt.Errorf("probe protocol is %q", document.Probe)
	}
	want := contract.Deployment.EnvironmentPropertyNames
	if !reflect.DeepEqual(document.PropertyNames, want) {
		return fmt.Sprintf("propertyNames=%v", document.PropertyNames),
			fmt.Errorf("env must project exactly %v and nothing else portable", want)
	}
	return fmt.Sprintf("propertyNames=%v", document.PropertyNames), nil
}

// runGlobals asks the isolate which members of the floor it actually has. The
// probe holds no list of its own: it answers about the names the runner sends,
// so a probe cannot flatter a runtime that is missing one.
func runGlobals(ctx context.Context, contract Contract, target *target, check Check) (string, error) {
	query := check.Request.Path + "?names=" + url.QueryEscape(strings.Join(contract.GlobalsFloor, ","))
	response, raw, err := target.request(ctx, check.Request.Method, query, nil)
	if err != nil {
		return "", err
	}
	if response.StatusCode != check.Expect.Status {
		return "status=" + strconv.Itoa(response.StatusCode),
			fmt.Errorf("expected status %d", check.Expect.Status)
	}
	var document struct {
		Probe   string   `json:"probe"`
		Present []string `json:"present"`
		Missing []string `json:"missing"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return compactJSON(raw), err
	}
	if len(document.Missing) > 0 {
		return fmt.Sprintf("missing=%v", document.Missing),
			fmt.Errorf("the runtime is missing %d members of the portable globals floor", len(document.Missing))
	}
	if !reflect.DeepEqual(document.Present, contract.GlobalsFloor) {
		return fmt.Sprintf("present=%v", document.Present),
			fmt.Errorf("the reported floor differs from %v", contract.GlobalsFloor)
	}
	return fmt.Sprintf("present=%d of %d", len(document.Present), len(contract.GlobalsFloor)), nil
}

// runKVRoundTrip writes the corpus's pinned bytes through the declared edge.kv
// binding and reads them back through it. edge.kv is eventually consistent and
// promises no read-your-writes, so the read polls to the deadline rather than
// asserting the first answer.
//
// The bytes are pinned and the KEY is correlated: a value only this run could
// have written lives at a key only this run could name, so a runtime whose
// `put` resolves without persisting anything reads nothing back, however many
// times the deployment has been measured before.
func runKVRoundTrip(ctx context.Context, contract Contract, target *target, check Check) (string, error) {
	key := "runtime-abi:" + target.correlate(contract, check)
	body, err := json.Marshal(map[string]string{"key": key, "valueBase64": check.Payload.Bytes})
	if err != nil {
		return "", err
	}
	response, raw, err := target.request(ctx, http.MethodPost, check.Request.Path, body)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "status=" + strconv.Itoa(response.StatusCode), errors.New("the binding refused the write")
	}
	var stored struct {
		Stored bool `json:"stored"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil || !stored.Stored {
		return compactJSON(raw), errors.New("the binding did not accept the value")
	}
	deadline := time.Now().Add(time.Duration(check.Timing.DeadlineSeconds) * time.Second)
	query := check.Request.Path + "?key=" + url.QueryEscape(key)
	for {
		_, readRaw, err := target.request(ctx, http.MethodGet, query, nil)
		if err != nil {
			return "", err
		}
		var observed struct {
			Found       bool   `json:"found"`
			ValueBase64 string `json:"valueBase64"`
		}
		if err := json.Unmarshal(readRaw, &observed); err != nil {
			return compactJSON(readRaw), err
		}
		if observed.Found {
			if observed.ValueBase64 != check.Payload.Bytes {
				return "valueBase64=" + observed.ValueBase64,
					errors.New("the bytes read back differ from the bytes written")
			}
			return fmt.Sprintf("key=%s roundTripBytes=%d", key, decodedLength(check.Payload.Bytes)), nil
		}
		if time.Now().After(deadline) {
			return "key=" + key + " found=false",
				fmt.Errorf("the value never became readable within %ds", check.Timing.DeadlineSeconds)
		}
		if err := sleepContext(ctx, time.Duration(check.Timing.PollMillis)*time.Millisecond); err != nil {
			return "", err
		}
	}
}

// requestResult is one outcome of the in-flight streamed request: either the
// response headers the worker sent, or the transport failure that ended it.
type requestResult struct {
	response *http.Response
	err      error
}

// calleeIdentity is the identity an answer must carry to count for this check.
//
// It is empty for a check that measures the worker itself, and the peer's own
// byte-carried identity for one that crosses the `worker.service` binding. The
// runner reads it from the corpus and the peer stamps it from its module bytes,
// so the only way an observation can carry it is for the peer to have produced
// it: a host that short-circuits the call into the caller's own handler runs
// bytes that do not contain the string.
func calleeIdentity(contract Contract, check Check) string {
	if check.ThroughBinding == "" || contract.Deployment.Peer == nil {
		return ""
	}
	return contract.Deployment.Peer.Identity
}

// verifyCalleeIdentity refuses an observation the callee did not produce.
func verifyCalleeIdentity(identity string, observed streamObservation, line []byte) error {
	if identity == "" || observed.Peer == identity {
		return nil
	}
	if observed.Peer == "" {
		return fmt.Errorf(
			"an observation arrived with no callee identity where the peer's %q was due: %s; "+
				"the caller's own bundle carries no identity, so this answer is one the caller produced "+
				"rather than one the worker.service binding dispatched",
			identity, line)
	}
	return fmt.Errorf(
		"an observation names callee %q where the peer's %q was due: %s; the call reached a worker that is not "+
			"the one the binding addresses",
		observed.Peer, identity, line)
}

// runRequestStream proves the request body is not buffered. The runner sends
// the first chunk, requires the worker to account for ALL of it BEFORE the
// second chunk exists, and only then sends the second. A host that waits for
// the whole body cannot produce that ordering.
//
// The body it sends declares NO length: it is written as it is produced, which
// is exactly the case `worker.service`'s nullable contentLength exists to
// spell. A host that demands an exact count before invoking the callee has only
// one way to obtain one, and it is the way this check fails — no response head
// arrives, because the body has not ended.
//
// What the check does not require is that read boundaries mirror write
// boundaries. A ReadableStream read is not a transport frame, so a conforming
// stack may deliver one 4096-byte write as several reads; the runner therefore
// accumulates the worker's incremental answers until they account for the bytes
// it has actually sent. The property proven is unchanged — every byte counted
// below was answered for while the second chunk existed nowhere but in this
// function.
func runRequestStream(ctx context.Context, contract Contract, target *target, check Check) (string, error) {
	identity := calleeIdentity(contract, check)
	sizes := check.Timing.RequestChunkSize
	deadline := time.Duration(check.Timing.DeadlineSeconds) * time.Second
	pipeReader, pipeWriter := io.Pipe()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.worker+check.Request.Path, pipeReader)
	if err != nil {
		return "", err
	}
	request.ContentLength = -1
	request.Header.Set("content-type", "application/octet-stream")
	if target.token != "" {
		request.Header.Set("authorization", "Bearer "+target.token)
	}
	results := make(chan requestResult, 1)
	go func() {
		response, err := target.client.Do(request)
		results <- requestResult{response: response, err: err}
	}()
	writeChunk := func(size int) error {
		_, err := pipeWriter.Write(bytes.Repeat([]byte{'x'}, size))
		return err
	}
	if err := writeChunk(sizes[0]); err != nil {
		_ = pipeWriter.CloseWithError(err)
		return "", err
	}
	// The headers are part of what this check measures, and the wait for them
	// is bounded by the check's own declared deadline rather than by whatever
	// the caller's HTTP client happens to allow. A host that buffers the
	// request body until EOF never sends them, and the run says so here
	// instead of stalling the matrix.
	outcome, err := awaitResponse(ctx, results, deadline)
	if err != nil {
		_ = pipeWriter.CloseWithError(err)
		return "responseHeaders=none", err
	}
	if outcome.err != nil {
		_ = pipeWriter.CloseWithError(outcome.err)
		return "", outcome.err
	}
	response := outcome.response
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_ = pipeWriter.Close()
		return "status=" + strconv.Itoa(response.StatusCode), errors.New("the worker refused the streamed request")
	}
	lines := newLineReader(response.Body)
	reads, err := awaitChunkAccounted(ctx, lines, deadline, 0, sizes[0], identity)
	if err != nil {
		_ = pipeWriter.Close()
		return fmt.Sprintf("reads=%d", reads), fmt.Errorf("the first request chunk was not answered: %w", err)
	}
	// Only now does the second chunk exist. Every answer above cannot have
	// depended on it.
	if err := sleepContext(ctx, time.Duration(check.Timing.GapMillis)*time.Millisecond); err != nil {
		_ = pipeWriter.Close()
		return "", err
	}
	if err := writeChunk(sizes[1]); err != nil {
		_ = pipeWriter.CloseWithError(err)
		return "", err
	}
	total, err := awaitChunkAccounted(ctx, lines, deadline, reads, sizes[1], identity)
	_ = pipeWriter.Close()
	if err != nil {
		return fmt.Sprintf("reads=%d", total), fmt.Errorf("the second request chunk was not answered: %w", err)
	}
	if identity != "" {
		return fmt.Sprintf("firstChunkAnsweredBeforeSecondSent=true reads=%d callee=%s", total, identity), nil
	}
	return fmt.Sprintf("firstChunkAnsweredBeforeSecondSent=true reads=%d", total), nil
}

// awaitResponse waits for the streamed request's headers under the check's own
// bound, and says what it was waiting for when the bound expires.
func awaitResponse(
	ctx context.Context, results <-chan requestResult, deadline time.Duration,
) (requestResult, error) {
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case outcome := <-results:
		return outcome, nil
	case <-timer.C:
		go discardResult(results)
		return requestResult{}, fmt.Errorf(
			"no response headers arrived within %s of the first request chunk; "+
				"a host that buffers the request body until EOF never sends them, because this request's body has not ended",
			deadline)
	case <-ctx.Done():
		go discardResult(results)
		return requestResult{}, ctx.Err()
	}
}

// discardResult reclaims the in-flight request once the check has stopped
// waiting for it, so an abandoned response body never outlives the run.
func discardResult(results <-chan requestResult) {
	outcome := <-results
	closeBody(outcome.response)
}

// awaitChunkAccounted reads incremental observations until the worker has
// accounted for `sent` further bytes, and reports the read count it reached.
//
// It accepts any split of those bytes across reads and rejects four things: an
// observation that arrives out of order, one that reports no bytes, one that
// pushes the total past what the client has actually written — an answer that
// cannot have come from the bytes this check sent — and, when the check crosses
// a binding, one that does not carry the callee's own identity.
func awaitChunkAccounted(
	ctx context.Context, lines *lineReader, deadline time.Duration, reads, sent int, identity string,
) (int, error) {
	accounted := 0
	for accounted < sent {
		line, err := lines.next(ctx, deadline)
		if err != nil {
			return reads, fmt.Errorf(
				"%w (the worker accounted for %d of %d bytes)", err, accounted, sent)
		}
		observed, err := decodeStreamRead(line)
		if err != nil {
			return reads, err
		}
		if err := verifyCalleeIdentity(identity, observed, line); err != nil {
			return reads, err
		}
		reads++
		if observed.Read != reads {
			return reads, fmt.Errorf(
				"the worker reported read %d where read %d was due: %s", observed.Read, reads, line)
		}
		if observed.Bytes <= 0 {
			return reads, fmt.Errorf("read %d reported %d bytes: %s", observed.Read, observed.Bytes, line)
		}
		accounted += observed.Bytes
		if accounted > sent {
			return reads, fmt.Errorf(
				"the worker accounted for %d bytes where only %d had been sent", accounted, sent)
		}
	}
	return reads, nil
}

// runResponseStream proves the response body is produced incrementally: two
// chunks the module separates in time must arrive separated in time.
//
// It also holds the response HEAD to what the body then does. A body generated
// as it is written has no byte count at the head, which is why
// `worker.service`'s contentLength is nullable in that direction; the two ways a
// host can refuse to say so are both refused here. One is to BUFFER the body to
// learn a count, which delivers chunks the producer separated in time all at
// once and fails the gap below. The other is to INVENT one, which the drain
// after the last chunk catches: a head that declared an exact length is held to
// delivering exactly that many bytes, and a length nobody could have known at
// the head is a length nobody may state there.
func runResponseStream(ctx context.Context, contract Contract, target *target, check Check) (string, error) {
	identity := calleeIdentity(contract, check)
	path := fmt.Sprintf("%s?chunks=%d&gapMillis=%d",
		check.Request.Path, check.Timing.Chunks, check.Timing.GapMillis)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.worker+path, nil)
	if err != nil {
		return "", err
	}
	if target.token != "" {
		request.Header.Set("authorization", "Bearer "+target.token)
	}
	response, err := target.client.Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "status=" + strconv.Itoa(response.StatusCode), errors.New("the worker refused the streamed response")
	}
	// The head's own claim about the body's size: a byte count when the host
	// declared one, and -1 when it declared none, which is what a body of
	// unknown length looks like on the wire.
	declared := response.ContentLength
	lines := newLineReader(response.Body)
	deadline := time.Duration(check.Timing.DeadlineSeconds) * time.Second
	arrivals := make([]time.Time, 0, check.Timing.Chunks)
	for chunk := 0; chunk < check.Timing.Chunks; chunk++ {
		line, err := lines.next(ctx, deadline)
		if err != nil {
			return fmt.Sprintf("chunksRead=%d declaredContentLength=%d", chunk, declared), err
		}
		observed, err := decodeStreamRead(line)
		if err != nil {
			return fmt.Sprintf("chunksRead=%d", chunk), err
		}
		if err := verifyCalleeIdentity(identity, observed, line); err != nil {
			return fmt.Sprintf("chunksRead=%d", chunk), err
		}
		arrivals = append(arrivals, time.Now())
	}
	// Half the module's own gap is the threshold: anything at or above it
	// cannot have come from one buffered write, and anything below it means
	// the host held the first chunk until the last was produced.
	threshold := time.Duration(check.Timing.GapMillis) * time.Millisecond / 2
	for index := 1; index < len(arrivals); index++ {
		gap := arrivals[index].Sub(arrivals[index-1])
		if gap < threshold {
			return fmt.Sprintf("gap=%s declaredContentLength=%d", gap, declared),
				fmt.Errorf(
					"chunk %d arrived %s after chunk %d; the body was buffered, which is what a host does "+
						"whether it is decoupling the two streams or reading the body to learn a length for a head "+
						"the contract lets it leave unknown",
					index+1, gap, index)
		}
	}
	if err := verifyDeclaredLength(ctx, lines, deadline, declared); err != nil {
		return fmt.Sprintf("declaredContentLength=%d deliveredBytes=%d", declared, lines.consumed()), err
	}
	return fmt.Sprintf("chunks=%d minimumGap=%s declaredContentLength=%d deliveredBytes=%d%s",
		len(arrivals), threshold, declared, lines.consumed(), calleeSuffix(identity)), nil
}

func calleeSuffix(identity string) string {
	if identity == "" {
		return ""
	}
	return " callee=" + identity
}

// verifyDeclaredLength reads the body to its end and holds the head to it.
//
// A head that declared no length promised nothing about the count, and the
// stream simply ends. A head that declared one promised exactly that many
// bytes: fewer is the abort `worker.service` names response_aborted, and there
// is no third outcome in which a host states a number and delivers a different
// one. A count the host could only have obtained by buffering the body is
// refused a step earlier, by the gap.
func verifyDeclaredLength(ctx context.Context, lines *lineReader, deadline time.Duration, declared int64) error {
	delivered, err := lines.awaitEnd(ctx, deadline)
	if err != nil {
		if declared < 0 {
			return fmt.Errorf("the response body never reached a clean end of stream: %w", err)
		}
		return fmt.Errorf(
			"the response head declared contentLength %d and the body errored after %d bytes (%w); a declared "+
				"count is a promise the bytes keep, and a host that cannot know the count when it answers the head "+
				"states an unknown length rather than inventing one",
			declared, delivered, err)
	}
	if declared >= 0 && delivered != declared {
		return fmt.Errorf(
			"the response head declared contentLength %d and the body delivered %d; a declared count is a promise "+
				"the bytes keep, and a host that cannot know the count when it answers the head states an unknown "+
				"length rather than inventing one",
			declared, delivered)
	}
	return nil
}

// runWaitUntil proves both halves of the operation at once: a rejected task
// leaves the already-sent response exactly as it was, and the isolate is still
// alive long enough afterwards for the second task to settle.
//
// The marker is correlated, so "not settled yet" is a statement about THIS
// run's task. Under a pinned constant the check read a marker a previous run's
// task had already written and failed a conforming runtime for running its
// deferred work inline — the mirror image of the defect it exists to catch.
func runWaitUntil(ctx context.Context, contract Contract, target *target, check Check) (string, error) {
	nonce := target.correlate(contract, check)
	body, err := json.Marshal(map[string]any{
		"nonce": nonce, "delayMillis": check.Timing.DelayMillis,
	})
	if err != nil {
		return "", err
	}
	response, raw, err := target.request(ctx, http.MethodPost, check.Request.Path, body)
	if err != nil {
		return "", err
	}
	if response.StatusCode != check.Expect.Status {
		return "status=" + strconv.Itoa(response.StatusCode),
			fmt.Errorf("expected status %d", check.Expect.Status)
	}
	if check.Expect.JSON != nil {
		document, err := decodeJSONObject(raw)
		if err != nil {
			return compactJSON(raw), err
		}
		if !reflect.DeepEqual(document, check.Expect.JSON) {
			return compactJSON(raw), errors.New("the response the handler returned is not the expected one")
		}
	}
	query := check.Request.Path + "?nonce=" + url.QueryEscape(nonce)
	_, immediateRaw, err := target.request(ctx, http.MethodGet, query, nil)
	if err != nil {
		return "", err
	}
	immediate, err := decodeWaitUntil(immediateRaw)
	if err != nil {
		return compactJSON(immediateRaw), err
	}
	if immediate.Settled {
		return "nonce=" + nonce + " settledBeforeResponseWasRead=true", fmt.Errorf(
			"the deferred task had already settled when the response was read; "+
				"either the host ran it inline or this run's round trip exceeded the declared %dms delay",
			check.Timing.DelayMillis)
	}
	deadline := time.Now().Add(time.Duration(check.Timing.DeadlineSeconds) * time.Second)
	for {
		_, pollRaw, err := target.request(ctx, http.MethodGet, query, nil)
		if err != nil {
			return "", err
		}
		observed, err := decodeWaitUntil(pollRaw)
		if err != nil {
			return compactJSON(pollRaw), err
		}
		if observed.Settled {
			if observed.Nonce != nonce {
				return "nonce=" + observed.Nonce, fmt.Errorf(
					"the settled marker names run %q, not this run's %q; the observation is not this run's",
					observed.Nonce, nonce)
			}
			if !observed.AfterRejection {
				return "afterRejection=false",
					errors.New("the surviving task did not run after the rejected one")
			}
			return "nonce=" + nonce + " responseUnchanged=true isolateHeldUntilSettled=true", nil
		}
		if time.Now().After(deadline) {
			return "nonce=" + nonce + " settled=false", fmt.Errorf(
				"the registered task never settled within %ds; the isolate was reclaimed with work outstanding",
				check.Timing.DeadlineSeconds)
		}
		if err := sleepContext(ctx, time.Duration(check.Timing.PollMillis)*time.Millisecond); err != nil {
			return "", err
		}
	}
}

// runScheduledObservation proves the host invokes the exported `scheduled`
// handler from the cron attachment, with the matched expression and instant.
// It clears the previous observation first, so a stale marker cannot pass.
func runScheduledObservation(ctx context.Context, contract Contract, target *target, check Check) (string, error) {
	_, baselineRaw, err := target.request(ctx, http.MethodGet, check.Request.Path, nil)
	if err != nil {
		return "", err
	}
	baseline, err := decodeScheduled(baselineRaw)
	if err != nil {
		return compactJSON(baselineRaw), err
	}
	if _, _, err := target.request(ctx, http.MethodDelete, check.Request.Path, nil); err != nil {
		return "", err
	}
	deadline := time.Now().Add(time.Duration(check.Timing.DeadlineSeconds) * time.Second)
	for {
		_, raw, err := target.request(ctx, http.MethodGet, check.Request.Path, nil)
		if err != nil {
			return "", err
		}
		observed, err := decodeScheduled(raw)
		if err != nil {
			return compactJSON(raw), err
		}
		if observed.Observed && observed.ScheduledTimeMillis > baseline.ScheduledTimeMillis {
			if observed.Cron != contract.Deployment.Cron {
				return "cron=" + observed.Cron,
					fmt.Errorf("the handler received %q, not the attached expression %q",
						observed.Cron, contract.Deployment.Cron)
			}
			return fmt.Sprintf("cron=%s scheduledTimeMillis=%d", observed.Cron, observed.ScheduledTimeMillis), nil
		}
		if time.Now().After(deadline) {
			return "observed=false", fmt.Errorf(
				"no scheduled invocation arrived within %ds of clearing the previous one",
				check.Timing.DeadlineSeconds)
		}
		if err := sleepContext(ctx, time.Duration(check.Timing.PollMillis)*time.Millisecond); err != nil {
			return "", err
		}
	}
}

// runQueueRoundTrip submits one message through the producer binding and reads
// back what the host delivered to the exported `queue` handler.
//
// The correlation value travels WITH the message: the producer writes it into
// the body, the host delivers the body, and the `queue` handler records it
// beside the batch. So the observation this check accepts is the delivery of
// the message this run submitted, not a marker some earlier run's delivery left
// under a constant name — which the payload, queue, attempts and message id all
// matched just as well.
func runQueueRoundTrip(ctx context.Context, contract Contract, target *target, check Check) (string, error) {
	nonce := target.correlate(contract, check)
	body, err := json.Marshal(map[string]string{
		"nonce": nonce, "payloadBase64": check.Payload.Bytes,
	})
	if err != nil {
		return "", err
	}
	response, raw, err := target.request(ctx, http.MethodPost, check.Request.Path, body)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "status=" + strconv.Itoa(response.StatusCode), errors.New("the producer binding refused the message")
	}
	var accepted struct {
		Sent bool `json:"sent"`
	}
	if err := json.Unmarshal(raw, &accepted); err != nil || !accepted.Sent {
		return compactJSON(raw), errors.New("the producer binding did not accept the message")
	}
	query := check.Request.Path + "?nonce=" + url.QueryEscape(nonce)
	deadline := time.Now().Add(time.Duration(check.Timing.DeadlineSeconds) * time.Second)
	for {
		_, pollRaw, err := target.request(ctx, http.MethodGet, query, nil)
		if err != nil {
			return "", err
		}
		var observed struct {
			Delivered     bool   `json:"delivered"`
			Queue         string `json:"queue"`
			Attempts      int    `json:"attempts"`
			MessageID     string `json:"messageId"`
			PayloadBase64 string `json:"payloadBase64"`
			Nonce         string `json:"nonce"`
		}
		if err := json.Unmarshal(pollRaw, &observed); err != nil {
			return compactJSON(pollRaw), err
		}
		if observed.Delivered {
			if observed.Nonce != nonce {
				return "nonce=" + observed.Nonce, fmt.Errorf(
					"the delivered batch carries run %q, not this run's %q; the delivery is not this run's",
					observed.Nonce, nonce)
			}
			if observed.PayloadBase64 != check.Payload.Bytes {
				return "payloadBase64=" + observed.PayloadBase64,
					errors.New("the batch carried different bytes from the ones produced")
			}
			if observed.Queue != contract.Deployment.Queue {
				return "queue=" + observed.Queue,
					fmt.Errorf("the batch named queue %q, not %q", observed.Queue, contract.Deployment.Queue)
			}
			if observed.Attempts != 1 {
				return "attempts=" + strconv.Itoa(observed.Attempts),
					errors.New("a first delivery must carry attempts 1")
			}
			if observed.MessageID == "" {
				return "messageId=", errors.New("the batch carried no stable message identity")
			}
			return fmt.Sprintf("nonce=%s queue=%s attempts=%d bytes=%d",
				nonce, observed.Queue, observed.Attempts, decodedLength(observed.PayloadBase64)), nil
		}
		if time.Now().After(deadline) {
			return "nonce=" + nonce + " delivered=false", fmt.Errorf(
				"the message was never delivered to the queue handler within %ds", check.Timing.DeadlineSeconds)
		}
		if err := sleepContext(ctx, time.Duration(check.Timing.PollMillis)*time.Millisecond); err != nil {
			return "", err
		}
	}
}

// request performs one ordinary probe request and reads its whole body.
func (t *target) request(ctx context.Context, method, path string, body []byte) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, t.worker+path, reader)
	if err != nil {
		return nil, nil, err
	}
	if body != nil {
		request.Header.Set("content-type", "application/json")
	}
	if t.token != "" {
		request.Header.Set("authorization", "Bearer "+t.token)
	}
	response, err := t.client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer closeBody(response)
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, nil, err
	}
	return response, raw, nil
}

func closeBody(response *http.Response) {
	if response != nil && response.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<16))
		_ = response.Body.Close()
	}
}

// countingReader counts the body bytes a stream actually delivered, which is
// the half of a length claim the head does not get to state.
type countingReader struct {
	inner io.Reader
	count atomic.Int64
}

func (c *countingReader) Read(buffer []byte) (int, error) {
	read, err := c.inner.Read(buffer)
	c.count.Add(int64(read))
	return read, err
}

// lineReader reads newline-delimited observations from a streaming body
// without letting a stalled runtime stall the whole run, and counts every byte
// it consumed so the body can be held to what the head declared.
type lineReader struct {
	lines  chan []byte
	closed chan struct{}
	body   *countingReader

	// terminal is how the stream ended: io.EOF for a body that finished, and
	// the transport's own error for one that did not. It is written before
	// closed is closed and read only after, so the close is the barrier.
	terminal error
}

func newLineReader(body io.Reader) *lineReader {
	counted := &countingReader{inner: body}
	reader := &lineReader{
		lines: make(chan []byte, 8), closed: make(chan struct{}), body: counted,
	}
	go func() {
		scanner := bufio.NewScanner(counted)
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			reader.lines <- line
		}
		reader.terminal = scanner.Err()
		if reader.terminal == nil {
			reader.terminal = io.EOF
		}
		close(reader.closed)
	}()
	return reader
}

// consumed is how many body bytes the stream delivered so far.
func (l *lineReader) consumed() int64 { return l.body.count.Load() }

func (l *lineReader) next(ctx context.Context, deadline time.Duration) ([]byte, error) {
	// A line already in hand beats the terminal signal. The scanner goroutine
	// reaches end of stream before a caller has drained what it queued, so a
	// plain select over both channels reports EOF at random while observations
	// are still waiting — which happens exactly when a whole body arrived at
	// once, and turns "the body was buffered" into "EOF" in the report of the
	// check that exists to say so.
	select {
	case line := <-l.lines:
		return line, nil
	default:
	}
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case line := <-l.lines:
		return line, nil
	case <-l.closed:
		select {
		case line := <-l.lines:
			return line, nil
		default:
		}
		return nil, l.terminal
	case <-timer.C:
		return nil, fmt.Errorf("no streamed observation arrived within %s", deadline)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// awaitEnd drains whatever is left and reports how many body bytes arrived in
// total. A clean end of stream is not an error here; anything else is the
// transport failure that ended the body early.
func (l *lineReader) awaitEnd(ctx context.Context, deadline time.Duration) (int64, error) {
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for {
		select {
		case <-l.lines:
		case <-l.closed:
			if errors.Is(l.terminal, io.EOF) {
				return l.consumed(), nil
			}
			return l.consumed(), l.terminal
		case <-timer.C:
			return l.consumed(), fmt.Errorf("the response body did not end within %s", deadline)
		case <-ctx.Done():
			return l.consumed(), ctx.Err()
		}
	}
}

// streamObservation is the part of a streamed line every procedure reads: what
// the worker accounted for, and WHICH worker accounted for it.
type streamObservation struct {
	Read  int    `json:"read"`
	Bytes int    `json:"bytes"`
	Peer  string `json:"peer"`
}

func decodeStreamRead(line []byte) (streamObservation, error) {
	var decoded streamObservation
	if err := json.Unmarshal(line, &decoded); err != nil {
		return streamObservation{}, fmt.Errorf("streamed observation is not decodable: %w", err)
	}
	return decoded, nil
}

type waitUntilObservation struct {
	Settled        bool   `json:"settled"`
	AfterRejection bool   `json:"afterRejection"`
	Nonce          string `json:"nonce"`
}

func decodeWaitUntil(raw []byte) (waitUntilObservation, error) {
	var decoded waitUntilObservation
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return waitUntilObservation{}, err
	}
	return decoded, nil
}

type scheduledObservation struct {
	Observed            bool   `json:"observed"`
	Cron                string `json:"cron"`
	ScheduledTimeMillis int64  `json:"scheduledTimeMillis"`
}

func decodeScheduled(raw []byte) (scheduledObservation, error) {
	var decoded scheduledObservation
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return scheduledObservation{}, err
	}
	return decoded, nil
}

// decodeJSONObject decodes with UseNumber so a contract expectation and a
// runtime answer compare as the same numbers rather than as float64 drift.
func decodeJSONObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("response is not a JSON object: %w", err)
	}
	return document, nil
}

func compactJSON(raw []byte) string {
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return buffer.String()
}

func decodedLength(encoded string) int {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return 0
	}
	return len(decoded)
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
