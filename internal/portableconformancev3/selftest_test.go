package portableconformancev3

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// TestSelfTestExecutesCompleteMatrix runs the reference host and the full
// runner over real HTTP: the report must be passed, never publication-ready,
// and must carry every required check exactly once in contract order.
func TestSelfTestExecutesCompleteMatrix(t *testing.T) {
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := SelfTest(ctx, contract)
	if err != nil {
		t.Fatalf("self-test: %v", err)
	}
	if report.Format != HostRunnerReportFormat || report.Status != "passed" ||
		report.Classification != ReferenceHostSelfTest {
		t.Fatalf("report identity drifted: %+v", report)
	}
	if report.PublicationReady {
		t.Fatalf("a local self-test report must never be publication-ready")
	}
	if !reflect.DeepEqual(report.Checks, contract.RequiredRunnerChecks) {
		t.Fatalf("executed checks = %v, want the exact contract list", report.Checks)
	}
	if len(report.ErrorProbes) != len(contract.ErrorEnvelope.Codes) {
		t.Fatalf("error probes = %d, want %d", len(report.ErrorProbes), len(contract.ErrorEnvelope.Codes))
	}
	if len(report.NegativeFixtures) != len(contract.RunnerInput.NegativeFixtures) {
		t.Fatalf(
			"negative fixture evidence = %d, want %d",
			len(report.NegativeFixtures), len(contract.RunnerInput.NegativeFixtures),
		)
	}
	if len(report.UIDTransitions) < 2 || report.UIDTransitions[0] == report.UIDTransitions[1] {
		t.Fatalf("uid transition evidence is incomplete: %v", report.UIDTransitions)
	}
	if report.ArtifactManifestDigest == "" || report.AsyncOperationID == "" || report.CancelledOperationID == "" {
		t.Fatalf("operation/artifact evidence is incomplete: %+v", report)
	}
}

// TestRunEndpointRequiresThreeDistinctCredentials pins the credential
// contract of the black-box runner.
func TestRunEndpointRequiresThreeDistinctCredentials(t *testing.T) {
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint: "http://127.0.0.1:0",
		Token:    "same", AlternateToken: "same", AlternateTenantToken: "other",
	})
	if err == nil {
		t.Fatalf("expected distinct-credential rejection")
	}
}
