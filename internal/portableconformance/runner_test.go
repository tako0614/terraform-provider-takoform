package portableconformance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReferenceHostBlackBoxRunnerExecutesEveryRequiredCheck(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}

	report, err := SelfTest(context.Background(), contract)
	if err != nil {
		t.Fatal(err)
	}
	if report.Format != HostRunnerReportFormat || report.Classification != ReferenceHostSelfTest ||
		report.PublicationReady || report.Status != "passed" {
		t.Fatalf("unexpected self-test report identity: %#v", report)
	}
	if !reflect.DeepEqual(report.Checks, contract.RequiredRunnerChecks) {
		t.Fatalf("executed checks = %v, want %v", report.Checks, contract.RequiredRunnerChecks)
	}
	if len(report.ErrorProbes) != len(contract.StableErrorCodes) {
		t.Fatalf("error probes = %d, want %d", len(report.ErrorProbes), len(contract.StableErrorCodes))
	}
	if len(report.NegativeFixtures) != len(contract.RunnerInput.NegativeFixtures) {
		t.Fatalf("desired negative fixture evidence = %d, want %d", len(report.NegativeFixtures), len(contract.RunnerInput.NegativeFixtures))
	}
	for _, fixture := range report.NegativeFixtures {
		if fixture.Stage != "desired" {
			t.Fatalf("black-box host report claimed non-request fixture stage: %#v", fixture)
		}
	}
	if got, want := report.GenerationTransitions, []string{"1", "2", "1", "2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("generation transitions = %v, want %v", got, want)
	}
	if !report.InterfaceEvidence.AbsentBeforeReady ||
		!report.InterfaceEvidence.ExactReadyProjection ||
		!report.InterfaceEvidence.AbsentAfterDelete {
		t.Fatalf("incomplete Interface readiness evidence: %#v", report.InterfaceEvidence)
	}
	if !reflect.DeepEqual(report.InterfaceEvidence.Checks, contract.InterfaceDeclarations.Checks) {
		t.Fatalf(
			"executed Interface checks = %v, want exact declared set %v",
			report.InterfaceEvidence.Checks,
			contract.InterfaceDeclarations.Checks,
		)
	}
	if !reflect.DeepEqual(
		report.PlanBindingEvidence.PureBlackBoxInputs,
		contract.PlanBinding.PureBlackBoxInputs,
	) {
		t.Fatalf(
			"pure black-box plan inputs = %v, want %v",
			report.PlanBindingEvidence.PureBlackBoxInputs,
			contract.PlanBinding.PureBlackBoxInputs,
		)
	}
	if !reflect.DeepEqual(
		report.PlanBindingEvidence.InstrumentedAdapterInputs,
		contract.PlanBinding.InstrumentedAdapter.Inputs,
	) {
		t.Fatalf(
			"instrumented plan inputs = %v, want %v",
			report.PlanBindingEvidence.InstrumentedAdapterInputs,
			contract.PlanBinding.InstrumentedAdapter.Inputs,
		)
	}
	for _, check := range contract.InterfaceDeclarations.Checks {
		if !containsValue(report.Checks, check) {
			t.Fatalf("Interface check %q is not required runner evidence", check)
		}
	}
}

func TestBlackBoxRunnerRejectsHostWithoutPlanSpecBinding(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.enforcePlanBinding = false
	server := httptest.NewServer(host)
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-without-plan-binding",
	})
	if err == nil || !strings.Contains(err.Error(), "plan/spec binding") {
		t.Fatalf("runner error = %v, want plan/spec binding rejection", err)
	}
}

func TestBlackBoxRunnerRejectsHostWithoutMutationReplay(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.enforceIdempotentReplay = false
	server := httptest.NewServer(host)
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-without-idempotency",
	})
	if err == nil || !strings.Contains(err.Error(), "apply idempotent replay") {
		t.Fatalf("runner error = %v, want apply idempotency rejection", err)
	}
}

func TestBlackBoxRunnerRejectsObservedDocumentOutsideExactFormSchema(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.invalidObservedProjection = true
	server := httptest.NewServer(host)
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-with-invalid-observed-document",
	})
	if err == nil || !strings.Contains(err.Error(), "exact Form schema") {
		t.Fatalf("runner error = %v, want exact observed schema rejection", err)
	}
}

func TestBlackBoxRunnerRejectsMismatchedPortabilityEvidence(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.mismatchedPortability = true
	server := httptest.NewServer(host)
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-with-mismatched-portability",
	})
	if err == nil || !strings.Contains(err.Error(), "portability values differ") {
		t.Fatalf("runner error = %v, want portability parity rejection", err)
	}
}

func TestBlackBoxRunnerAcceptsOptionalInterfaceFormOmission(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.omitInterfaceForm = true
	server := httptest.NewServer(host)
	defer server.Close()

	if _, err := RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-without-optional-interface-form",
	}); err != nil {
		t.Fatalf("runner rejected optional Interface form omission: %v", err)
	}
}

func TestBlackBoxRunnerRejectsInventedInterfaceResourceURI(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	host := newReferenceHost(contract)
	host.inventInterfaceResourceURI = true
	server := httptest.NewServer(host)
	defer server.Close()

	_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Classification: ReferenceHostSelfTest,
		Subject:        "reference-host-with-invented-interface-resource-uri",
	})
	if err == nil || !strings.Contains(err.Error(), "Interface projection differs") {
		t.Fatalf("runner error = %v, want invented Interface resourceUri rejection", err)
	}
}

func TestBlackBoxRunnerRejectsUnprovenInterfaceSemantics(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*referenceHost)
		wantErr string
	}{
		{
			name: "write route accepted",
			mutate: func(host *referenceHost) {
				host.acceptInterfaceWrites = true
			},
			wantErr: "read-only Interface endpoint accepted",
		},
		{
			name: "query vocabulary ignored",
			mutate: func(host *referenceHost) {
				host.ignoreInterfaceQueryRules = true
			},
			wantErr: "closed Interface query vocabulary",
		},
		{
			name: "Space substituted",
			mutate: func(host *referenceHost) {
				host.leakInterfacesAcrossSpaces = true
			},
			wantErr: "Space isolation leaked",
		},
		{
			name: "omitted unique version rejected",
			mutate: func(host *referenceHost) {
				host.rejectOmittedVersion = true
			},
			wantErr: "omitted-version unique read",
		},
		{
			name: "ambiguous Resource silently selected",
			mutate: func(host *referenceHost) {
				host.resolveAmbiguousInstance = true
			},
			wantErr: "multi-Resource ambiguity",
		},
		{
			name: "authority projected",
			mutate: func(host *referenceHost) {
				host.projectInterfaceAuthority = true
			},
			wantErr: "forbidden interface values",
		},
		{
			name: "required Interface visible before Ready",
			mutate: func(host *referenceHost) {
				host.exposeInterfaceBeforeReady = true
			},
			wantErr: "declarations visible before a Ready Resource",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
			if err != nil {
				t.Fatal(err)
			}
			host := newReferenceHost(contract)
			test.mutate(host)
			server := httptest.NewServer(host)
			defer server.Close()

			_, err = RunEndpoint(context.Background(), contract, EndpointOptions{
				Endpoint:       server.URL,
				HTTPClient:     server.Client(),
				Classification: ReferenceHostSelfTest,
				Subject:        "adversarial-interface-reference-host",
			})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("runner error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestStableErrorEvidenceRequiresRetryableMember(t *testing.T) {
	response := wireResponse{
		Status: http.StatusBadRequest,
		Body:   []byte(`{"error":{"code":"invalid_argument","message":"bad","requestId":"request-1"}}`),
	}
	if err := expectStableError(response, "invalid_argument"); err == nil {
		t.Fatal("stable error evidence accepted an envelope without retryable")
	}
}
