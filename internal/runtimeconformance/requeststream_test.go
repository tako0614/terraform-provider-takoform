package runtimeconformance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The request-stream check proves ONE thing: the handler answered for bytes
// while the next bytes existed nowhere but in the runner. It does not prove
// that a `ReadableStream` read corresponds to a client write — nothing in the
// ABI says it does, and a conforming HTTP stack may deliver one 4096-byte write
// as several reads. The tests below pin both halves: the split is accepted, and
// the buffer is not.

func checkFor(t *testing.T, contract Contract, name string) Check {
	t.Helper()
	for _, check := range contract.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("the corpus carries no check named %q", name)
	return Check{}
}

// runStreamCheck drives the request-stream check alone against one hand-written
// runtime, which is how a test states what that check accepts and refuses
// without deploying a whole matrix.
func runStreamCheck(t *testing.T, handler http.Handler) (CheckEvidence, time.Duration) {
	t.Helper()
	contract := fastCorpus(t)
	check := checkFor(t, contract, "request-body-streams-rather-than-buffering")
	worker := httptest.NewServer(handler)
	defer worker.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// The client's own timeout is deliberately far longer than the check's
	// bound: what a run waits for must be the bound the corpus declares.
	client := worker.Client()
	client.Timeout = 2 * time.Minute
	started := time.Now()
	evidence := runCheck(ctx, contract, &target{
		client: client, worker: worker.URL, runToken: "streamtest",
	}, check)
	return evidence, time.Since(started)
}

// streamingWorker answers a line per read of at most readSize bytes, which is
// what a runtime whose transport splits a client write into several stream
// reads looks like from outside.
func streamingWorker(readSize int) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = http.NewResponseController(writer).EnableFullDuplex()
		writer.Header().Set("content-type", "application/x-ndjson")
		writer.WriteHeader(http.StatusOK)
		flusher, _ := writer.(http.Flusher)
		buffer := make([]byte, readSize)
		reads := 0
		for {
			read, err := request.Body.Read(buffer)
			if read > 0 {
				reads++
				line, _ := json.Marshal(map[string]any{"read": reads, "bytes": read})
				_, _ = writer.Write(append(line, '\n'))
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err != nil {
				return
			}
		}
	})
}

// TestASplitReadStillProvesTheRequestBodyStreamed is the acceptance half. Four
// reads of 1024 bytes account for one 4096-byte write, and every one of them
// was answered before the second write existed.
func TestASplitReadStillProvesTheRequestBodyStreamed(t *testing.T) {
	evidence, _ := runStreamCheck(t, streamingWorker(1024))
	if evidence.Outcome != OutcomePassed {
		t.Fatalf("a runtime that split each write across reads was failed: %s (%s)",
			evidence.Detail, evidence.Observed)
	}
	if !strings.Contains(evidence.Observed, "firstChunkAnsweredBeforeSecondSent=true") {
		t.Fatalf("the evidence must state what was proven, got %q", evidence.Observed)
	}
}

// TestABufferedRequestBodyStillFails is the refusal half, and the reason the
// accumulation above may not become a licence: a host that waits for the whole
// body answers nothing while the second chunk is unsent, so it never accounts
// for the first chunk at all.
func TestABufferedRequestBodyStillFails(t *testing.T) {
	buffering := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		writer.Header().Set("content-type", "application/x-ndjson")
		writer.WriteHeader(http.StatusOK)
		line, _ := json.Marshal(map[string]any{"read": 1, "bytes": len(body)})
		_, _ = writer.Write(append(line, '\n'))
	})
	evidence, elapsed := runStreamCheck(t, buffering)
	if evidence.Outcome != OutcomeFailed {
		t.Fatalf("a runtime that buffered the whole request body passed: %s", evidence.Observed)
	}
	if !strings.Contains(evidence.Detail, "no response headers arrived") {
		t.Fatalf("the failure must name what the run was waiting for, got %q", evidence.Detail)
	}
	// The check declares a bound; the run must be that bound and not the CLI
	// client's two minutes.
	if elapsed > 30*time.Second {
		t.Fatalf("the check waited %s for a bound it declares as 5s", elapsed)
	}
}

// TestAnAnswerForBytesThatWereNotSentFails keeps the accumulation from being
// satisfiable by a host that guesses: an answer accounting for more bytes than
// the client has written cannot have come from the bytes this check sent.
func TestAnAnswerForBytesThatWereNotSentFails(t *testing.T) {
	guessing := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = http.NewResponseController(writer).EnableFullDuplex()
		writer.Header().Set("content-type", "application/x-ndjson")
		writer.WriteHeader(http.StatusOK)
		line, _ := json.Marshal(map[string]any{"read": 1, "bytes": 4096 + 8192})
		_, _ = writer.Write(append(line, '\n'))
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = io.Copy(io.Discard, request.Body)
	})
	evidence, _ := runStreamCheck(t, guessing)
	if evidence.Outcome != OutcomeFailed ||
		!strings.Contains(evidence.Detail, "only 4096 had been sent") {
		t.Fatalf("an answer for unsent bytes was accepted: %+v", evidence)
	}
}
