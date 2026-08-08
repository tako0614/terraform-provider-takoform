package runtimeconformance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/fakeruntime"
)

// A deployed runtime outlives the run that measures it. Its `edge.kv` namespace
// is still there the next morning, so the second measurement of one deployment
// starts with every observation the first one caused already in place. SelfTest
// constructs a fresh stand-in per run and therefore never sees that state; the
// tests below keep one stand-in alive across runs, which is the only way this
// repository can observe what an operator measuring the same worker twice does.

// measuredDeployment is one stand-in kept alive across runs, with two host
// defects a test can switch on. Neither defect is corpus knowledge: `put`
// resolving without storing anything, and a producer accepting a message the
// host never delivers, are things runtimes do.
type measuredDeployment struct {
	worker         *httptest.Server
	client         *http.Client
	dropKVWrites   atomic.Bool
	dropQueueSends atomic.Bool
}

func newMeasuredDeployment(t *testing.T, contract Contract) *measuredDeployment {
	t.Helper()
	deployment, err := contract.WorkerDeployment()
	if err != nil {
		t.Fatalf("deployment: %v", err)
	}
	runtime, err := fakeruntime.New(deployment, fakeruntime.Options{})
	if err != nil {
		t.Fatalf("stand-in: %v", err)
	}
	host := &measuredDeployment{}
	inner := runtime.WorkerHandler()
	host.worker = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			switch {
			case request.URL.Path == "/abi/kv" && host.dropKVWrites.Load():
				answer(writer, map[string]any{"probe": fakeruntime.ProbeProtocol, "stored": true})
				return
			case request.URL.Path == "/abi/queue" && host.dropQueueSends.Load():
				answer(writer, map[string]any{"probe": fakeruntime.ProbeProtocol, "sent": true})
				return
			}
		}
		inner.ServeHTTP(writer, request)
	}))
	host.client = host.worker.Client()
	t.Cleanup(func() {
		host.worker.Close()
		runtime.Close()
	})
	return host
}

func answer(writer http.ResponseWriter, body map[string]any) {
	encoded, _ := json.Marshal(body)
	writer.Header().Set("content-type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(encoded)
}

// measure runs the whole matrix against this deployment with the correlation
// token supplied, which is how a test states the difference between a corpus
// that pins one value forever and a runner that mints one per run.
func (m *measuredDeployment) measure(t *testing.T, contract Contract, runToken string) Report {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := runEndpoint(ctx, contract, EndpointOptions{
		Endpoint:       m.worker.URL,
		HTTPClient:     m.client,
		Classification: FakeRuntimeSelfTest,
		Subject:        "in-process-fake-runtime",
	}, runToken)
	if report.Format != ReportFormat {
		t.Fatalf("the run produced no report: %v", err)
	}
	return report
}

func outcomeOf(t *testing.T, report Report, name string) CheckEvidence {
	t.Helper()
	for _, evidence := range report.Checks {
		if evidence.Name == name {
			return evidence
		}
	}
	t.Fatalf("the report accounts for no check named %q", name)
	return CheckEvidence{}
}

// fastCorpus is the committed corpus with its TIMING shortened and nothing
// else: the same checks, the same templates, the same module bytes. Every one
// of those bounds is a maximum a conforming runtime stays inside, and these
// tests measure an in-process stand-in, so shrinking them changes how long a
// test takes and nothing it proves — a failing round trip is what costs a full
// deadline, and these tests deliberately produce several.
func fastCorpus(t *testing.T) Contract {
	t.Helper()
	root := copyCorpus(t)
	mutateCorpus(t, root, func(document map[string]any) {
		retime(document, "kv-binding-round-trips-bytes-through-env", map[string]any{
			"deadlineSeconds": 1, "pollMillis": 25,
		})
		retime(document, "queue-batch-delivered-to-the-queue-handler", map[string]any{
			"deadlineSeconds": 1, "pollMillis": 25,
		})
		retime(document, "wait-until-holds-the-isolate-and-a-rejection-leaves-the-response-alone",
			map[string]any{"deadlineSeconds": 5, "pollMillis": 25, "delayMillis": 100})
		retime(document, "scheduled-invoked-by-the-host-cron-attachment", map[string]any{
			"deadlineSeconds": 5, "pollMillis": 25,
		})
		retime(document, "response-body-streams-rather-than-buffering", map[string]any{
			"deadlineSeconds": 5, "gapMillis": 50, "chunks": 3,
		})
		retime(document, "request-body-streams-rather-than-buffering", map[string]any{
			"deadlineSeconds": 5, "gapMillis": 25,
			"requestChunkSize": []any{4096, 8192},
		})
	})
	contract, err := Verify(root)
	if err != nil {
		t.Fatalf("verify the retimed corpus: %v", err)
	}
	return contract
}

func retime(document map[string]any, check string, timing map[string]any) {
	checkNamed(document, check)["timing"] = timing
}

// TestAPinnedCorrelationValuePassesARuntimeThatStoresNothing is the first of
// the two lines this fix is verified by. One conforming run leaves its
// observations in the deployment's `edge.kv` namespace; the runtime then stops
// storing and stops delivering. Measured again with the value a byte-pinned
// constant would have supplied, it passes both round trips on the previous
// run's leftovers. Measured with a token minted for the run, it fails both.
func TestAPinnedCorrelationValuePassesARuntimeThatStoresNothing(t *testing.T) {
	contract := fastCorpus(t)
	host := newMeasuredDeployment(t, contract)
	const pinned = "pinnedconstant"
	const kvCheck = "kv-binding-round-trips-bytes-through-env"
	const queueCheck = "queue-batch-delivered-to-the-queue-handler"

	first := host.measure(t, contract, pinned)
	for _, name := range []string{kvCheck, queueCheck} {
		if evidence := outcomeOf(t, first, name); evidence.Outcome != OutcomePassed {
			t.Fatalf("the conforming first run must pass %q: %s", name, evidence.Detail)
		}
	}

	// The runtime is now one no conformance run may pass: its binding accepts a
	// write it does not persist, and its producer accepts a message the host
	// never delivers.
	host.dropKVWrites.Store(true)
	host.dropQueueSends.Store(true)

	stale := host.measure(t, contract, pinned)
	for _, name := range []string{kvCheck, queueCheck} {
		if evidence := outcomeOf(t, stale, name); evidence.Outcome != OutcomePassed {
			t.Fatalf(
				"this test asserts the DEFECT a pinned correlation value has: %q was expected to pass "+
					"on the previous run's observation, and did not (%s)", name, evidence.Detail)
		}
	}

	fresh := host.measure(t, contract, mintToken(t, contract))
	for _, name := range []string{kvCheck, queueCheck} {
		evidence := outcomeOf(t, fresh, name)
		if evidence.Outcome != OutcomeFailed {
			t.Fatalf("%q passed a runtime that stored nothing and delivered nothing: %s",
				name, evidence.Observed)
		}
	}
	for _, name := range []string{kvCheck, queueCheck} {
		if !strings.Contains(outcomeOf(t, fresh, name).Observed, fresh.RunToken) {
			t.Fatalf("a failed %q must record the correlation value it looked for", name)
		}
	}
}

// TestAPinnedCorrelationValueFailsAConformingRuntimeOnItsSecondRun is the same
// defect wearing its other face. Under a pinned value the `waitUntil` check
// reads the marker the PREVIOUS run's deferred task wrote, concludes the task
// it just registered had already settled, and fails a runtime that did
// everything the ABI asks. Under a run token there is nothing to read but this
// run's own task.
func TestAPinnedCorrelationValueFailsAConformingRuntimeOnItsSecondRun(t *testing.T) {
	contract := fastCorpus(t)
	host := newMeasuredDeployment(t, contract)
	const pinned = "pinnedconstant"
	const waitUntil = "wait-until-holds-the-isolate-and-a-rejection-leaves-the-response-alone"

	if evidence := outcomeOf(t, host.measure(t, contract, pinned), waitUntil); evidence.Outcome != OutcomePassed {
		t.Fatalf("the first run must pass %q: %s", waitUntil, evidence.Detail)
	}
	repeated := outcomeOf(t, host.measure(t, contract, pinned), waitUntil)
	if repeated.Outcome != OutcomeFailed ||
		!strings.Contains(repeated.Observed, "settledBeforeResponseWasRead") {
		t.Fatalf(
			"this test asserts the DEFECT a pinned correlation value has: a conforming runtime was "+
				"expected to be failed by its own previous run's marker, and was not (%+v)", repeated)
	}
	corrected := outcomeOf(t, host.measure(t, contract, mintToken(t, contract)), waitUntil)
	if corrected.Outcome != OutcomePassed {
		t.Fatalf("%q failed a conforming runtime: %s (%s)", waitUntil, corrected.Detail, corrected.Observed)
	}
}

// TestADeploymentMeasuredTenTimesBehavesLikeAFreshOne is the property the run
// token buys: repeat measurement of one deployment is measurement, not
// archaeology. Every run mints its own token, every run passes, and no two runs
// used the same one.
func TestADeploymentMeasuredTenTimesBehavesLikeAFreshOne(t *testing.T) {
	contract := fastCorpus(t)
	host := newMeasuredDeployment(t, contract)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	tokens := map[string]int{}
	for run := 1; run <= 10; run++ {
		report, err := RunEndpoint(ctx, contract, EndpointOptions{
			Endpoint:       host.worker.URL,
			HTTPClient:     host.client,
			Classification: FakeRuntimeSelfTest,
			Subject:        "in-process-fake-runtime",
		})
		if err != nil {
			t.Fatalf("run %d of a deployment already measured %d times: %v", run, run-1, err)
		}
		if report.Failed != 0 || report.Status != OutcomePassed {
			t.Fatalf("run %d failed %d checks: %+v", run, report.Failed, report.Checks)
		}
		if earlier, repeated := tokens[report.RunToken]; repeated {
			t.Fatalf("runs %d and %d minted the same correlation token %q", earlier, run, report.RunToken)
		}
		tokens[report.RunToken] = run
	}
}

// TestACorrelatedCheckCannotPinAConstant keeps the mechanism honest the way
// this corpus keeps everything else honest: the loader refuses a corpus that
// states a correlation value no run could make its own, rather than leaving a
// check that a stale observation answers.
func TestACorrelatedCheckCannotPinAConstant(t *testing.T) {
	for name, mutate := range map[string]func(document map[string]any){
		"constant nonce": func(document map[string]any) {
			payload := checkNamed(document, "kv-binding-round-trips-bytes-through-env")["payload"].(map[string]any)
			payload["nonceTemplate"] = "kv-round-trip"
		},
		"placeholder twice": func(document map[string]any) {
			payload := checkNamed(document, "queue-batch-delivered-to-the-queue-handler")["payload"].(map[string]any)
			payload["nonceTemplate"] = "queue-{run}-{run}"
		},
	} {
		root := copyCorpus(t)
		mutateCorpus(t, root, mutate)
		if _, err := Verify(root); err == nil ||
			!strings.Contains(err.Error(), "run-correlation placeholder") {
			t.Fatalf("%s: expected the corpus to be refused, got %v", name, err)
		}
	}
}

// TestTwoChecksCannotShareOneCorrelationTemplate refuses the shape where two
// checks read one observation: whichever ran second would be answered by the
// first, which is the pinned-constant defect inside a single run.
func TestTwoChecksCannotShareOneCorrelationTemplate(t *testing.T) {
	root := copyCorpus(t)
	mutateCorpus(t, root, func(document map[string]any) {
		payload := checkNamed(document, "queue-batch-delivered-to-the-queue-handler")["payload"].(map[string]any)
		payload["nonceTemplate"] = "kv-round-trip-{run}"
	})
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "are one check") {
		t.Fatalf("expected the corpus to be refused, got %v", err)
	}
}

// TestTheRunTokenIsStatedInTheReport keeps a failed deployed run diagnosable
// after the fact: the correlation values a run used are derivable from the
// report and the corpus, and from nothing else.
func TestTheRunTokenIsStatedInTheReport(t *testing.T) {
	contract := verifiedContract(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := SelfTest(ctx, contract)
	if err != nil {
		t.Fatalf("self-test: %v", err)
	}
	if len(report.RunToken) != contract.RunCorrelation.TokenBytes*2 {
		t.Fatalf("the report states the run token %q, which is not a %d-byte token",
			report.RunToken, contract.RunCorrelation.TokenBytes)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if !strings.Contains(string(encoded), `"runToken":"`+report.RunToken+`"`) {
		t.Fatalf("the run token must survive into the report document")
	}
	for _, evidence := range report.Checks {
		if evidence.Procedure != ProcedureKVRoundTrip && evidence.Procedure != ProcedureQueueRoundTrip &&
			evidence.Procedure != ProcedureWaitUntil {
			continue
		}
		if !strings.Contains(evidence.Observed, report.RunToken) {
			t.Fatalf("check %q observed %q, which names no run", evidence.Name, evidence.Observed)
		}
	}
}

func mintToken(t *testing.T, contract Contract) string {
	t.Helper()
	token, err := mintRunToken(contract.RunCorrelation)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return token
}
