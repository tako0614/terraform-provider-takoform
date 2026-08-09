package runtimeconformance

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/fakeruntime"
)

// selfTestWithoutLoader runs the matrix against the stand-in's worker with no
// module-loader adapter, which is the state an operator is in before they
// expose one.
func selfTestWithoutLoader(ctx context.Context, contract Contract) (Report, error) {
	deployment, err := contract.WorkerDeployment()
	if err != nil {
		return Report{}, err
	}
	runtime, err := fakeruntime.New(deployment, fakeruntime.Options{})
	if err != nil {
		return Report{}, err
	}
	defer runtime.Close()
	worker := httptest.NewServer(runtime.WorkerHandler())
	defer worker.Close()
	return RunEndpoint(ctx, contract, EndpointOptions{
		Endpoint:       worker.URL,
		HTTPClient:     worker.Client(),
		Classification: FakeRuntimeSelfTest,
		Subject:        "in-process-fake-runtime",
	})
}

func corpusRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "conformance", "runtime-abi-v1"))
	if err != nil {
		t.Fatalf("corpus root: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("corpus root: %v", err)
	}
	return root
}

func verifiedContract(t *testing.T) Contract {
	t.Helper()
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	return contract
}

// TestSelfTestExecutesTheWholeMatrix runs the corpus against the in-process
// stand-in over real HTTP and pins what the report may claim.
func TestSelfTestExecutesTheWholeMatrix(t *testing.T) {
	contract := verifiedContract(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := SelfTest(ctx, contract)
	if err != nil {
		t.Fatalf("self-test: %v", err)
	}
	if report.Format != ReportFormat || report.Status != OutcomePassed {
		t.Fatalf("report identity drifted: %+v", report)
	}
	if report.PublicationReady {
		t.Fatalf("a runtime ABI report must never be publication-ready")
	}
	if report.Classification != FakeRuntimeSelfTest {
		t.Fatalf("classification = %q, want %q", report.Classification, FakeRuntimeSelfTest)
	}
	if !strings.Contains(report.Proves, "proves nothing about any runtime") {
		t.Fatalf("a self-test report must say it proves no runtime: %q", report.Proves)
	}
	if !strings.Contains(report.BlockerEvidence, "V3-008") ||
		!strings.Contains(report.BlockerEvidence, "never closes it") {
		t.Fatalf("the report must name the evidence that closes V3-008: %q", report.BlockerEvidence)
	}
	if len(report.Checks) != len(contract.RequiredChecks) {
		t.Fatalf("executed %d checks, want %d", len(report.Checks), len(contract.RequiredChecks))
	}
	for index, evidence := range report.Checks {
		if evidence.Name != contract.RequiredChecks[index] {
			t.Fatalf("check %d is %q, want %q", index, evidence.Name, contract.RequiredChecks[index])
		}
		if evidence.Outcome == OutcomeFailed {
			t.Fatalf("check %q failed: %s (%s)", evidence.Name, evidence.Detail, evidence.Observed)
		}
	}
	if report.Failed != 0 || report.Measured != len(contract.RequiredChecks) || report.Unmeasured != 0 {
		t.Fatalf("measured/unmeasured accounting drifted: %+v", report)
	}
	// Every check in the corpus is now measurable, so a self-test with the
	// loader adapter present is COMPLETE. It was partial while `tail` sat in the
	// matrix as an entry nothing could reach.
	if report.Completeness != "complete" {
		t.Fatalf("a run that measured every check must report complete, got %q", report.Completeness)
	}
	if report.ContractDigest != contract.Digest() || report.Interface != contract.Interface {
		t.Fatalf("the report must bind the corpus digest and the measured Interface: %+v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if !strings.Contains(string(encoded), `"publicationReady":false`) {
		t.Fatalf("publicationReady must serialize as false")
	}
}

// TestEveryDeclaredHandlerIsMeasured is the property the ABI has now that
// `tail` is gone, read off the committed corpus rather than asserted in prose.
//
// The corpus used to carry `tail` as an explicitly unmeasured entry, which was
// the honest thing to say about a handler nothing could invoke. It is now true
// that every declared handler is exercised by a check that can fail, and this
// reads that off the corpus the same way a reviewer would.
func TestEveryDeclaredHandlerIsMeasured(t *testing.T) {
	contract := verifiedContract(t)
	measured := map[string][]string{}
	for _, check := range contract.Checks {
		if check.Procedure == ProcedureUnmeasured {
			continue
		}
		measured[check.Operation] = append(measured[check.Operation], check.Name)
	}
	for _, handler := range contract.HandlerVocabulary {
		if len(measured[handler]) == 0 {
			t.Fatalf("handler %q is declared by the ABI and measured by no check", handler)
		}
	}
	for _, check := range contract.Checks {
		if check.Unmeasured != nil {
			t.Fatalf("check %q states an unmeasured surface; the corpus has none left", check.Name)
		}
	}
}

// TestTheCorpusRefusesADeclaredHandlerNoCheckMeasures is the tooth under it.
//
// The property above is only worth stating if the corpus ENFORCES it, so this
// widens the vocabulary by one handler — the shape a future ABI revision takes
// — and requires the loader to refuse the corpus rather than run a matrix with
// a hole in it. A bundle exporting the new handler comes with it, so what the
// loader objects to is the missing CHECK and not an incoherent bundle.
func TestTheCorpusRefusesADeclaredHandlerNoCheckMeasures(t *testing.T) {
	contract := verifiedContract(t)
	contract.HandlerVocabulary = append(append([]string(nil), contract.HandlerVocabulary...), "alarm")
	err := validateEveryDeclaredHandlerIsMeasured(contract)
	if err == nil {
		t.Fatal("the corpus accepted a declared handler no check measures")
	}
	if !strings.Contains(err.Error(), "no check measures it") {
		t.Fatalf("refusal %q does not say what is missing", err)
	}
}

// TestAnUnmeasuredEntryCannotDischargeAHandler is the same tooth from the other
// side: recording a handler as `unmeasured` is exactly what the corpus used to
// do for `tail`, and it no longer counts as measuring one.
func TestAnUnmeasuredEntryCannotDischargeAHandler(t *testing.T) {
	contract := verifiedContract(t)
	contract.HandlerVocabulary = append(append([]string(nil), contract.HandlerVocabulary...), "alarm")
	contract.Checks = append(append([]Check(nil), contract.Checks...), Check{
		Name:      "alarm-is-unmeasured",
		Operation: "alarm",
		Procedure: ProcedureUnmeasured,
		Proves:    "nothing.",
		Unmeasured: &UnmeasuredCase{
			Reason: "nothing activates it", WouldUse: "/abi/alarm",
			ClosedBy: "an attachment", BlockerID: "V3-999",
		},
	})
	if err := validateEveryDeclaredHandlerIsMeasured(contract); err == nil {
		t.Fatal("an unmeasured entry discharged a declared handler")
	}
}

// TestRunEndpointRefusesAnEndpointItMustNotMeasure pins the transport contract
// of a deployed run.
func TestRunEndpointRefusesAnEndpointItMustNotMeasure(t *testing.T) {
	contract := verifiedContract(t)
	for _, testCase := range []struct {
		name    string
		options EndpointOptions
	}{
		{"empty", EndpointOptions{}},
		{"plaintext remote", EndpointOptions{Endpoint: "http://worker.example"}},
		{"userinfo", EndpointOptions{Endpoint: "https://token@worker.example"}},
		{"query", EndpointOptions{Endpoint: "https://worker.example?probe=1"}},
		{"scheme", EndpointOptions{Endpoint: "ftp://worker.example"}},
		{"classification", EndpointOptions{
			Endpoint: "https://worker.example", Classification: "verified-by-vendor",
		}},
	} {
		if _, err := RunEndpoint(context.Background(), contract, testCase.options); err == nil {
			t.Fatalf("%s: expected refusal", testCase.name)
		}
	}
}

// TestALoadLaneWithoutAnAdapterIsUnmeasuredNotPassed proves the run reports an
// unmeasurable half honestly instead of counting it as evidence.
func TestALoadLaneWithoutAnAdapterIsUnmeasuredNotPassed(t *testing.T) {
	contract := verifiedContract(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := selfTestWithoutLoader(ctx, contract)
	if err != nil {
		t.Fatalf("self-test: %v", err)
	}
	loadChecks := 0
	for _, evidence := range report.Checks {
		if evidence.Procedure != ProcedureLoad {
			continue
		}
		loadChecks++
		if evidence.Outcome != OutcomeUnmeasured {
			t.Fatalf("check %q outcome = %q, want unmeasured", evidence.Name, evidence.Outcome)
		}
	}
	if loadChecks == 0 {
		t.Fatalf("the corpus must carry load checks")
	}
	if report.Completeness != "partial" || report.Status != OutcomePassed {
		t.Fatalf("an unmeasured load lane is partial, not failed: %+v", report)
	}
}
