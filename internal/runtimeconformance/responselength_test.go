package runtimeconformance

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// `worker.service` completes at the response HEAD, so a body generated as it is
// written has no byte count when the head is answered. That is why contentLength
// is nullable: a host that knows the count states it, and a host that does not
// states unknown. The two ways of refusing to say so are a fabricated count and
// a buffered body, and the response-stream procedure has to catch both — an
// unknown-length spelling nothing enforces would just be a third thing a host
// may ignore.

// runResponseCheck drives the response-stream check alone against one
// hand-written runtime.
func runResponseCheck(t *testing.T, handler http.Handler) CheckEvidence {
	t.Helper()
	contract := fastCorpus(t)
	check := checkFor(t, contract, "response-body-streams-rather-than-buffering")
	worker := httptest.NewServer(handler)
	defer worker.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := worker.Client()
	client.Timeout = 2 * time.Minute
	return runCheck(ctx, contract, &target{
		client: client, worker: worker.URL, runToken: "responselength",
	}, check)
}

// chunkLines is what a conforming module produces: one observation per chunk,
// separated in time.
func chunkLines(chunks int) [][]byte {
	lines := make([][]byte, 0, chunks)
	for chunk := 1; chunk <= chunks; chunk++ {
		line, _ := json.Marshal(map[string]any{"chunk": chunk})
		lines = append(lines, append(line, '\n'))
	}
	return lines
}

// fabricatingWorker answers the head with an exact length nobody could have
// known there, and then streams the body it was always going to stream. It is
// the host that met a required exact count by inventing one.
func fabricatingWorker(declared int, chunks int, gap time.Duration) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			http.Error(writer, "this transport cannot fabricate a head", http.StatusInternalServerError)
			return
		}
		connection, buffered, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		_, _ = buffered.WriteString(fmt.Sprintf(
			"HTTP/1.1 200 OK\r\ncontent-type: application/x-ndjson\r\ncontent-length: %d\r\n\r\n", declared))
		_ = buffered.Flush()
		writeChunks(buffered, chunks, gap)
	})
}

func writeChunks(buffered *bufio.ReadWriter, chunks int, gap time.Duration) {
	for index, line := range chunkLines(chunks) {
		if index > 0 {
			time.Sleep(gap)
		}
		_, _ = buffered.Write(line)
		_ = buffered.Flush()
	}
}

// bufferingWorker produces the whole body first so that it can state a truthful
// length, and delivers in one write what the module separated in time. It is
// the host that met a required exact count by reading the body to learn it.
func bufferingWorker(chunks int, gap time.Duration) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		body := []byte{}
		for index, line := range chunkLines(chunks) {
			if index > 0 {
				time.Sleep(gap)
			}
			body = append(body, line...)
		}
		writer.Header().Set("content-type", "application/x-ndjson")
		writer.Header().Set("content-length", fmt.Sprint(len(body)))
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(body)
	})
}

// honestWorker streams as it produces and declares no length, which is what a
// body of unknown size looks like on the wire.
func honestWorker(chunks int, gap time.Duration) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("content-type", "application/x-ndjson")
		writer.WriteHeader(http.StatusOK)
		flusher, _ := writer.(http.Flusher)
		for index, line := range chunkLines(chunks) {
			if index > 0 {
				time.Sleep(gap)
			}
			_, _ = writer.Write(line)
			if flusher != nil {
				flusher.Flush()
			}
		}
	})
}

// TestAFabricatedResponseLengthFails is the first of the two lines the length
// model is verified by. Nothing about the timing is wrong — the chunks arrive
// separated, the head arrives first — and the run still refuses it, because the
// count the head stated is not the count the body delivered.
func TestAFabricatedResponseLengthFails(t *testing.T) {
	evidence := runResponseCheck(t, fabricatingWorker(1048576, 3, 60*time.Millisecond))
	if evidence.Outcome != OutcomeFailed {
		t.Fatalf("a host that invented a length at the response head passed: %s", evidence.Observed)
	}
	if !strings.Contains(evidence.Detail, "declared contentLength 1048576") ||
		!strings.Contains(evidence.Detail, "states an unknown length rather than inventing one") {
		t.Fatalf("the failure must name the invented count and the alternative, got %q", evidence.Detail)
	}
}

// TestABufferedResponseBodyFailsWhateverItsHeadSaid is the second. This host's
// length is perfectly truthful; what it paid for the truth is the streaming,
// which is the trade the nullable count exists to make unnecessary.
func TestABufferedResponseBodyFailsWhateverItsHeadSaid(t *testing.T) {
	evidence := runResponseCheck(t, bufferingWorker(3, 60*time.Millisecond))
	if evidence.Outcome != OutcomeFailed {
		t.Fatalf("a host that buffered the body to learn its length passed: %s", evidence.Observed)
	}
	if !strings.Contains(evidence.Detail, "the body was buffered") ||
		!strings.Contains(evidence.Detail, "learn a length") {
		t.Fatalf("the failure must name buffering and why a host does it, got %q", evidence.Detail)
	}
}

// TestAnUnknownLengthResponseIsAccepted keeps the two refusals from being a
// wall. The answer the contract asks for — stream as you produce, declare no
// count you do not have — is the answer that passes.
func TestAnUnknownLengthResponseIsAccepted(t *testing.T) {
	evidence := runResponseCheck(t, honestWorker(3, 60*time.Millisecond))
	if evidence.Outcome != OutcomePassed {
		t.Fatalf("a host that streamed a body of unknown length was failed: %s (%s)",
			evidence.Detail, evidence.Observed)
	}
	if !strings.Contains(evidence.Observed, "declaredContentLength=-1") {
		t.Fatalf("the evidence must record what the head claimed, got %q", evidence.Observed)
	}
}

// TestATruthfulDeclaredLengthIsAccepted is the last corner: a host that DOES
// know its count is not being asked to hide it. The model admits an exact
// length; what it refuses is one the bytes do not keep.
func TestATruthfulDeclaredLengthIsAccepted(t *testing.T) {
	body := chunkLines(3)
	total := 0
	for _, line := range body {
		total += len(line)
	}
	evidence := runResponseCheck(t, fabricatingWorker(total, 3, 60*time.Millisecond))
	if evidence.Outcome != OutcomePassed {
		t.Fatalf("a host that declared a count it kept was failed: %s (%s)",
			evidence.Detail, evidence.Observed)
	}
	if !strings.Contains(evidence.Observed, fmt.Sprintf("declaredContentLength=%d deliveredBytes=%d", total, total)) {
		t.Fatalf("the evidence must record the count and what arrived, got %q", evidence.Observed)
	}
}
