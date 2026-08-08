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
	if report.Failed != 0 || report.Measured != len(contract.RequiredChecks)-1 || report.Unmeasured != 1 {
		t.Fatalf("measured/unmeasured accounting drifted: %+v", report)
	}
	if report.Completeness != "partial" {
		t.Fatalf("a run carrying an unmeasured check must report partial completeness, got %q", report.Completeness)
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

// TestTailIsRecordedAsUnmeasuredRatherThanOmitted pins the decision that a
// handler nothing can invoke is stated, not quietly skipped.
func TestTailIsRecordedAsUnmeasuredRatherThanOmitted(t *testing.T) {
	contract := verifiedContract(t)
	for _, check := range contract.Checks {
		if check.Operation != "tail" {
			continue
		}
		if check.Procedure != ProcedureUnmeasured || check.Unmeasured == nil {
			t.Fatalf("the tail check must be explicitly unmeasured: %+v", check)
		}
		if !strings.Contains(check.Unmeasured.Reason, "activates one") ||
			check.Unmeasured.BlockerID != "V3-011" {
			t.Fatalf("the tail entry must say why it is unreachable and name its blocker: %+v", check.Unmeasured)
		}
		return
	}
	t.Fatalf("the corpus must account for every handler the ABI declares, including tail")
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
