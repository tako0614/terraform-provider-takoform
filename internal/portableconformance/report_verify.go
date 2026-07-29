package portableconformance

import (
	"fmt"
	"reflect"
	"regexp"
)

var disposableHostSubjectPattern = regexp.MustCompile(`^host:http://127[.]0[.]0[.]1:[1-9][0-9]*$`)

// ValidateHostRunnerReport verifies the semantic content of a strictly decoded,
// signed disposable-endpoint report against the exact retained runner contract.
// A valid Sigstore bundle authenticates bytes; this function proves those bytes
// actually contain the complete current portable-host result.
func ValidateHostRunnerReport(contract Contract, report HostRunnerReport) error {
	if err := validate(contract); err != nil {
		return fmt.Errorf("portable host contract: %w", err)
	}
	if report.Format != HostRunnerReportFormat ||
		report.Classification != EndpointConformanceRun ||
		report.PublicationReady ||
		report.Status != "passed" ||
		!disposableHostSubjectPattern.MatchString(report.Subject) ||
		report.RunnerSubject != contract.RunnerEvidence.Subject ||
		report.RunnerInputDigest != contract.RunnerEvidence.SHA256 {
		return fmt.Errorf("portable host runner report identity is invalid")
	}
	if !reflect.DeepEqual(report.Checks, contract.RequiredRunnerChecks) {
		return fmt.Errorf("portable host runner checks do not equal the exact contract")
	}

	if len(report.ErrorProbes) != len(contract.StableErrorCodes) {
		return fmt.Errorf("portable host runner error probe count is invalid")
	}
	for index, code := range contract.StableErrorCodes {
		status, retryable, ok := stableErrorSemantics(code)
		if !ok {
			return fmt.Errorf("portable host contract has unknown stable error %q", code)
		}
		if report.ErrorProbes[index] != (ErrorProbeEvidence{
			Code: code, HTTPStatus: status, Retryable: retryable,
		}) {
			return fmt.Errorf("portable host runner error probe %q is invalid", code)
		}
	}

	if len(report.NegativeFixtures) != len(contract.RunnerInput.NegativeFixtures) {
		return fmt.Errorf("portable host runner negative fixture count is invalid")
	}
	for index, fixture := range contract.RunnerInput.NegativeFixtures {
		if report.NegativeFixtures[index] != (NegativeFixtureEvidence{
			Name: fixture.Name, Stage: fixture.Stage, SHA256: fixture.SHA256,
		}) {
			return fmt.Errorf("portable host runner negative fixture %q is invalid", fixture.Name)
		}
	}

	if !reflect.DeepEqual(report.GenerationTransitions, []string{"1", "2", "1", "2"}) ||
		!reflect.DeepEqual(report.PlanBindingEvidence.PureBlackBoxInputs, contract.PlanBinding.PureBlackBoxInputs) ||
		!reflect.DeepEqual(report.PlanBindingEvidence.InstrumentedAdapterInputs, contract.PlanBinding.InstrumentedAdapter.Inputs) {
		return fmt.Errorf("portable host runner lifecycle or plan-binding evidence is invalid")
	}
	if !reflect.DeepEqual(
		report.IdempotencyEvidence.IsolationDimensions,
		[]string{"authenticated-principal", "authenticated-tenant", "space"},
	) ||
		!reflect.DeepEqual(
			report.IdempotencyEvidence.ReplayAuthorizationDenials,
			contract.Idempotency.ReplayDenialCodes,
		) ||
		!report.IdempotencyEvidence.SuccessReplayPreserved {
		return fmt.Errorf("portable host runner idempotency evidence is invalid")
	}
	if !reflect.DeepEqual(report.InterfaceEvidence.Checks, contract.InterfaceDeclarations.Checks) ||
		!report.InterfaceEvidence.AbsentBeforeReady ||
		!report.InterfaceEvidence.ExactReadyProjection ||
		!report.InterfaceEvidence.AbsentAfterDelete {
		return fmt.Errorf("portable host runner Interface evidence is invalid")
	}
	return nil
}
