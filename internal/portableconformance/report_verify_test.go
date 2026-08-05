package portableconformance

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateHostRunnerReportRejectsLegacySubjectAndIncompleteEvidence(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	report := semanticReportFixture(contract)
	if err := ValidateHostRunnerReport(contract, report); err != nil {
		t.Fatalf("valid report: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*HostRunnerReport)
		want   string
	}{
		{
			name: "legacy runner subject",
			mutate: func(report *HostRunnerReport) {
				report.RunnerSubject = "takoform.portable-host-black-box-runner@v1"
			},
			want: "identity",
		},
		{
			name: "missing check",
			mutate: func(report *HostRunnerReport) {
				report.Checks = report.Checks[:len(report.Checks)-1]
			},
			want: "checks",
		},
		{
			name: "relabelled error probe",
			mutate: func(report *HostRunnerReport) {
				report.ErrorProbes[0].Code = "permission_denied"
			},
			want: "error probe",
		},
		{
			name: "adapter plan substitute omitted",
			mutate: func(report *HostRunnerReport) {
				report.PlanBindingEvidence.InstrumentedAdapterInputs = nil
			},
			want: "plan-binding",
		},
		{
			name: "replay authorization omitted",
			mutate: func(report *HostRunnerReport) {
				report.IdempotencyEvidence.ReplayAuthorizationDenials = nil
			},
			want: "idempotency",
		},
		{
			name: "Interface exactness omitted",
			mutate: func(report *HostRunnerReport) {
				report.InterfaceEvidence.ExactReadyProjection = false
			},
			want: "Interface",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := semanticReportFixture(contract)
			test.mutate(&candidate)
			err := ValidateHostRunnerReport(contract, candidate)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateHostRunnerReport() error = %v, want %q", err, test.want)
			}
		})
	}
}

func semanticReportFixture(contract Contract) HostRunnerReport {
	errorProbes := make([]ErrorProbeEvidence, 0, len(contract.StableErrorCodes))
	for _, code := range contract.StableErrorCodes {
		status, retryable, _ := stableErrorSemantics(code)
		errorProbes = append(errorProbes, ErrorProbeEvidence{
			Code: code, HTTPStatus: status, Retryable: retryable,
		})
	}
	negativeFixtures := make([]NegativeFixtureEvidence, 0, len(contract.RunnerInput.NegativeFixtures))
	for _, fixture := range contract.RunnerInput.NegativeFixtures {
		negativeFixtures = append(negativeFixtures, NegativeFixtureEvidence{
			Name: fixture.Name, Stage: fixture.Stage, SHA256: fixture.SHA256,
		})
	}
	return HostRunnerReport{
		Format:            HostRunnerReportFormat,
		Classification:    EndpointConformanceRun,
		PublicationReady:  false,
		Status:            "passed",
		Subject:           "host:http://127.0.0.1:41234",
		RunnerSubject:     contract.RunnerEvidence.Subject,
		RunnerInputDigest: contract.RunnerEvidence.SHA256,
		Checks:            append([]string(nil), contract.RequiredRunnerChecks...),
		ErrorProbes:       errorProbes,
		NegativeFixtures:  negativeFixtures,
		GenerationTransitions: []string{
			"1", "2", "1", "2",
		},
		PlanBindingEvidence: PlanBindingRunnerEvidence{
			PureBlackBoxInputs:        append([]string(nil), contract.PlanBinding.PureBlackBoxInputs...),
			InstrumentedAdapterInputs: append([]string(nil), contract.PlanBinding.InstrumentedAdapter.Inputs...),
		},
		IdempotencyEvidence: IdempotencyRunnerEvidence{
			IsolationDimensions: []string{
				"authenticated-principal", "authenticated-tenant", "space",
			},
			ReplayAuthorizationDenials: append([]string(nil), contract.Idempotency.ReplayDenialCodes...),
			SuccessReplayPreserved:     true,
		},
		InterfaceEvidence: InterfaceRunnerEvidence{
			Checks:               append([]string(nil), contract.InterfaceDeclarations.Checks...),
			AbsentBeforeReady:    true,
			ExactReadyProjection: true,
			AbsentAfterDelete:    true,
		},
	}
}
