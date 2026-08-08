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
		observed, err = runKVRoundTrip(ctx, target, check)
	case ProcedureRequestStream:
		observed, err = runRequestStream(ctx, target, check)
	case ProcedureResponseStream:
		observed, err = runResponseStream(ctx, target, check)
	case ProcedureWaitUntil:
		observed, err = runWaitUntil(ctx, target, check)
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

// runKVRoundTrip writes bytes through the declared edge.kv binding and reads
// them back through it. edge.kv is eventually consistent and promises no
// read-your-writes, so the read polls to the deadline rather than asserting
// the first answer.
func runKVRoundTrip(ctx context.Context, target *target, check Check) (string, error) {
	key := "runtime-abi:" + check.Payload.Nonce
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
			return "roundTripBytes=" + strconv.Itoa(decodedLength(check.Payload.Bytes)), nil
		}
		if time.Now().After(deadline) {
			return "found=false",
				fmt.Errorf("the value never became readable within %ds", check.Timing.DeadlineSeconds)
		}
		if err := sleepContext(ctx, time.Duration(check.Timing.PollMillis)*time.Millisecond); err != nil {
			return "", err
		}
	}
}

// runRequestStream proves the request body is not buffered. The runner sends
// the first chunk, requires the worker's first answer BEFORE the second chunk
// exists, and only then sends it. A host that waits for the whole body cannot
// produce that ordering.
func runRequestStream(ctx context.Context, target *target, check Check) (string, error) {
	sizes := check.Timing.RequestChunkSize
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
	type result struct {
		response *http.Response
		err      error
	}
	results := make(chan result, 1)
	go func() {
		response, err := target.client.Do(request)
		results <- result{response: response, err: err}
	}()
	writeChunk := func(size int) error {
		_, err := pipeWriter.Write(bytes.Repeat([]byte{'x'}, size))
		return err
	}
	if err := writeChunk(sizes[0]); err != nil {
		_ = pipeWriter.CloseWithError(err)
		return "", err
	}
	outcome := <-results
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
	deadline := time.Duration(check.Timing.DeadlineSeconds) * time.Second
	first, err := lines.next(ctx, deadline)
	if err != nil {
		_ = pipeWriter.Close()
		return "", fmt.Errorf("no answer arrived for the first request chunk: %w", err)
	}
	firstRead, err := decodeStreamRead(first)
	if err != nil {
		_ = pipeWriter.Close()
		return string(first), err
	}
	if firstRead.Read != 1 || firstRead.Bytes != sizes[0] {
		_ = pipeWriter.Close()
		return string(first), fmt.Errorf("the worker reported read %d of %d bytes for the first chunk",
			firstRead.Read, firstRead.Bytes)
	}
	// Only now does the second chunk exist. The answer above cannot have
	// depended on it.
	if err := sleepContext(ctx, time.Duration(check.Timing.GapMillis)*time.Millisecond); err != nil {
		_ = pipeWriter.Close()
		return "", err
	}
	if err := writeChunk(sizes[1]); err != nil {
		_ = pipeWriter.CloseWithError(err)
		return "", err
	}
	second, err := lines.next(ctx, deadline)
	if err != nil {
		_ = pipeWriter.Close()
		return "", fmt.Errorf("no answer arrived for the second request chunk: %w", err)
	}
	_ = pipeWriter.Close()
	secondRead, err := decodeStreamRead(second)
	if err != nil {
		return string(second), err
	}
	if secondRead.Read != 2 || secondRead.Bytes != sizes[1] {
		return string(second), fmt.Errorf("the worker reported read %d of %d bytes for the second chunk",
			secondRead.Read, secondRead.Bytes)
	}
	return fmt.Sprintf("firstChunkAnsweredBeforeSecondSent=true reads=%d", secondRead.Read), nil
}

// runResponseStream proves the response body is produced incrementally: two
// chunks the module separates in time must arrive separated in time.
func runResponseStream(ctx context.Context, target *target, check Check) (string, error) {
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
	lines := newLineReader(response.Body)
	deadline := time.Duration(check.Timing.DeadlineSeconds) * time.Second
	arrivals := make([]time.Time, 0, check.Timing.Chunks)
	for chunk := 0; chunk < check.Timing.Chunks; chunk++ {
		if _, err := lines.next(ctx, deadline); err != nil {
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
			return fmt.Sprintf("gap=%s", gap),
				fmt.Errorf("chunk %d arrived %s after chunk %d; the body was buffered", index+1, gap, index)
		}
	}
	return fmt.Sprintf("chunks=%d minimumGap=%s", len(arrivals), threshold), nil
}

// runWaitUntil proves both halves of the operation at once: a rejected task
// leaves the already-sent response exactly as it was, and the isolate is still
// alive long enough afterwards for the second task to settle.
func runWaitUntil(ctx context.Context, target *target, check Check) (string, error) {
	body, err := json.Marshal(map[string]any{
		"nonce": check.Payload.Nonce, "delayMillis": check.Timing.DelayMillis,
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
	query := check.Request.Path + "?nonce=" + url.QueryEscape(check.Payload.Nonce)
	_, immediateRaw, err := target.request(ctx, http.MethodGet, query, nil)
	if err != nil {
		return "", err
	}
	immediate, err := decodeWaitUntil(immediateRaw)
	if err != nil {
		return compactJSON(immediateRaw), err
	}
	if immediate.Settled {
		return "settledBeforeResponseWasRead=true", fmt.Errorf(
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
			if !observed.AfterRejection {
				return "afterRejection=false",
					errors.New("the surviving task did not run after the rejected one")
			}
			return "responseUnchanged=true isolateHeldUntilSettled=true", nil
		}
		if time.Now().After(deadline) {
			return "settled=false", fmt.Errorf(
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
func runQueueRoundTrip(ctx context.Context, contract Contract, target *target, check Check) (string, error) {
	body, err := json.Marshal(map[string]string{
		"nonce": check.Payload.Nonce, "payloadBase64": check.Payload.Bytes,
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
	query := check.Request.Path + "?nonce=" + url.QueryEscape(check.Payload.Nonce)
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
		}
		if err := json.Unmarshal(pollRaw, &observed); err != nil {
			return compactJSON(pollRaw), err
		}
		if observed.Delivered {
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
			return fmt.Sprintf("queue=%s attempts=%d bytes=%d",
				observed.Queue, observed.Attempts, decodedLength(observed.PayloadBase64)), nil
		}
		if time.Now().After(deadline) {
			return "delivered=false", fmt.Errorf(
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

// lineReader reads newline-delimited observations from a streaming body
// without letting a stalled runtime stall the whole run.
type lineReader struct {
	lines chan []byte
	fail  chan error
}

func newLineReader(body io.Reader) *lineReader {
	reader := &lineReader{lines: make(chan []byte, 8), fail: make(chan error, 1)}
	go func() {
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			reader.lines <- line
		}
		if err := scanner.Err(); err != nil {
			reader.fail <- err
			return
		}
		reader.fail <- io.EOF
	}()
	return reader
}

func (l *lineReader) next(ctx context.Context, deadline time.Duration) ([]byte, error) {
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case line := <-l.lines:
		return line, nil
	case err := <-l.fail:
		return nil, err
	case <-timer.C:
		return nil, fmt.Errorf("no streamed observation arrived within %s", deadline)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type streamRead struct {
	Read  int `json:"read"`
	Bytes int `json:"bytes"`
}

func decodeStreamRead(line []byte) (streamRead, error) {
	var decoded streamRead
	if err := json.Unmarshal(line, &decoded); err != nil {
		return streamRead{}, fmt.Errorf("streamed observation is not decodable: %w", err)
	}
	return decoded, nil
}

type waitUntilObservation struct {
	Settled        bool `json:"settled"`
	AfterRejection bool `json:"afterRejection"`
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
