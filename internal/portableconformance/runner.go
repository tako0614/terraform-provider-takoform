package portableconformance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

const (
	HostRunnerReportFormat = "takoform.portable-host-runner-report@v1"
	ReferenceHostSelfTest  = "deterministic-reference-host-self-test"
	EndpointConformanceRun = "disposable-endpoint-conformance-run"

	// ErrorProbeHeader is a runner-harness header, not part of the portable host
	// API. A disposable conformance endpoint or adapter uses it to make every
	// stable error deterministic. Production endpoints should not implement it.
	ErrorProbeHeader = "Takoform-Conformance-Probe-Error"

	// AuthorizationProbeHeader is also runner-harness-only. Its closed values
	// ask the disposable auth adapter to perform one exact current
	// authentication, permission, or policy denial before any idempotency
	// replay lookup.
	AuthorizationProbeHeader            = "Takoform-Conformance-Probe-Authorization"
	AuthorizationProbeCredentialRevoked = "credential-revoked"
	AuthorizationProbePermissionRevoked = "permission-revoked"
	AuthorizationProbePolicyRevoked     = "policy-revoked"

	// RawJSONProbeHeader asks a disposable adapter for an exact malformed raw
	// response that a neutral runner cannot otherwise induce. It is not a host
	// API header and production endpoints must not implement it.
	RawJSONProbeHeader             = "Takoform-Conformance-Probe-Raw-JSON"
	RawJSONProbeDuplicateErrorCode = "duplicate-error-code"

	// PlanBindingProbeHeader and PlanBindingProbeResultHeader are disposable
	// conformance-adapter instrumentation, not portable API headers. They route
	// otherwise host-invalid one-field substitutions directly through the same
	// canonical plan-binding function used by apply. Production endpoints must
	// not implement them.
	PlanBindingProbeHeader                   = "Takoform-Conformance-Probe-Plan-Binding"
	PlanBindingProbeResultHeader             = "Takoform-Conformance-Probe-Plan-Binding-Result"
	PlanBindingProbeResultRejected           = "rejected"
	PlanBindingProbeResultAcceptedNoMutation = "accepted-no-mutation"
)

type EndpointOptions struct {
	Endpoint             string
	Token                string
	AlternateToken       string
	AlternateTenantToken string
	HTTPClient           *http.Client
	Classification       string
	Subject              string
}

type ErrorProbeEvidence struct {
	Code       string `json:"code"`
	HTTPStatus int    `json:"httpStatus"`
	Retryable  bool   `json:"retryable"`
}

type InterfaceRunnerEvidence struct {
	Checks               []string `json:"checks"`
	AbsentBeforeReady    bool     `json:"absentBeforeReady"`
	ExactReadyProjection bool     `json:"exactReadyProjection"`
	AbsentAfterDelete    bool     `json:"absentAfterDelete"`
}

type PlanBindingRunnerEvidence struct {
	PureBlackBoxInputs        []string `json:"pureBlackBoxInputs"`
	InstrumentedAdapterInputs []string `json:"instrumentedAdapterInputs"`
}

type IdempotencyRunnerEvidence struct {
	IsolationDimensions        []string `json:"isolationDimensions"`
	ReplayAuthorizationDenials []string `json:"replayAuthorizationDenials"`
	SuccessReplayPreserved     bool     `json:"successReplayPreservedAfterDenials"`
}

type NegativeFixtureEvidence struct {
	Name   string `json:"name"`
	Stage  string `json:"stage"`
	SHA256 string `json:"sha256"`
}

type HostRunnerReport struct {
	Format                string                    `json:"format"`
	Classification        string                    `json:"classification"`
	PublicationReady      bool                      `json:"publicationReady"`
	Status                string                    `json:"status"`
	Subject               string                    `json:"subject"`
	RunnerSubject         string                    `json:"runnerSubject"`
	RunnerInputDigest     string                    `json:"runnerInputDigest"`
	Checks                []string                  `json:"checks"`
	ErrorProbes           []ErrorProbeEvidence      `json:"errorProbes"`
	NegativeFixtures      []NegativeFixtureEvidence `json:"negativeFixtures"`
	GenerationTransitions []string                  `json:"generationTransitions"`
	PlanBindingEvidence   PlanBindingRunnerEvidence `json:"planBindingEvidence"`
	IdempotencyEvidence   IdempotencyRunnerEvidence `json:"idempotencyEvidence"`
	InterfaceEvidence     InterfaceRunnerEvidence   `json:"interfaceEvidence"`
}

// SelfTest proves that the checked-in runner executes the portable matrix over
// real HTTP requests. It is local implementation evidence only: it is neither
// a signed external host report nor publication/admission evidence.
func SelfTest(ctx context.Context, contract Contract) (HostRunnerReport, error) {
	host := newReferenceHost(contract)
	server := httptest.NewServer(host)
	defer server.Close()
	return RunEndpoint(ctx, contract, EndpointOptions{
		Endpoint:             server.URL,
		Token:                referencePrimaryToken,
		AlternateToken:       referenceAlternateToken,
		AlternateTenantToken: referenceAlternateTenantToken,
		HTTPClient:           server.Client(),
		Classification:       ReferenceHostSelfTest,
		Subject:              "reference-host",
	})
}

// RunEndpoint executes the complete black-box matrix against a disposable
// endpoint. The endpoint must support ErrorProbeHeader through a test adapter;
// this non-production hook lets the runner exercise every stable error without
// standardizing host-specific ways to cause permission, policy, or backend
// failures.
func RunEndpoint(ctx context.Context, contract Contract, options EndpointOptions) (HostRunnerReport, error) {
	if err := validate(contract); err != nil {
		return HostRunnerReport{}, fmt.Errorf("portable host contract: %w", err)
	}
	endpoint := strings.TrimRight(strings.TrimSpace(options.Endpoint), "/")
	if endpoint == "" {
		return HostRunnerReport{}, errors.New("portable host runner endpoint is required")
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	transportClient := *httpClient
	transportClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	httpClient = &transportClient
	classification := options.Classification
	if classification == "" {
		classification = EndpointConformanceRun
	}
	token := strings.TrimSpace(options.Token)
	alternateToken := strings.TrimSpace(options.AlternateToken)
	alternateTenantToken := strings.TrimSpace(options.AlternateTenantToken)
	if classification == ReferenceHostSelfTest {
		if token == "" {
			token = referencePrimaryToken
		}
		if alternateToken == "" {
			alternateToken = referenceAlternateToken
		}
		if alternateTenantToken == "" {
			alternateTenantToken = referenceAlternateTenantToken
		}
	}
	if token == "" || alternateToken == "" || alternateTenantToken == "" {
		return HostRunnerReport{}, errors.New(
			"portable host runner requires primary, same-tenant alternate-principal, and alternate-tenant credentials",
		)
	}
	if token == alternateToken || token == alternateTenantToken || alternateToken == alternateTenantToken {
		return HostRunnerReport{}, errors.New("portable host runner authority probes require three distinct bearer credentials")
	}
	subject := strings.TrimSpace(options.Subject)
	if subject == "" {
		subject = "host:" + endpoint
	}
	runner := endpointRunner{
		ctx: ctx, contract: contract, endpoint: endpoint, token: token, alternateToken: alternateToken,
		alternateTenantToken: alternateTenantToken,
		httpClient:           httpClient, completed: map[string]bool{},
	}
	if err := runner.run(); err != nil {
		return HostRunnerReport{}, err
	}
	checks := make([]string, 0, len(contract.RequiredRunnerChecks))
	for _, check := range contract.RequiredRunnerChecks {
		if !runner.completed[check] {
			return HostRunnerReport{}, fmt.Errorf("portable host runner did not execute required check %q", check)
		}
		checks = append(checks, check)
	}
	interfaceChecks := make([]string, 0, len(contract.InterfaceDeclarations.Checks))
	for _, check := range contract.InterfaceDeclarations.Checks {
		if !runner.completed[check] {
			return HostRunnerReport{}, fmt.Errorf("portable host runner did not execute declared Interface check %q", check)
		}
		interfaceChecks = append(interfaceChecks, check)
	}
	runner.interfaceEvidence.Checks = interfaceChecks
	if !reflect.DeepEqual(
		runner.planBindingEvidence.PureBlackBoxInputs,
		contract.PlanBinding.PureBlackBoxInputs,
	) {
		return HostRunnerReport{}, fmt.Errorf(
			"portable host runner pure black-box plan inputs = %v, want %v",
			runner.planBindingEvidence.PureBlackBoxInputs,
			contract.PlanBinding.PureBlackBoxInputs,
		)
	}
	if !reflect.DeepEqual(
		runner.planBindingEvidence.InstrumentedAdapterInputs,
		contract.PlanBinding.InstrumentedAdapter.Inputs,
	) {
		return HostRunnerReport{}, fmt.Errorf(
			"portable host runner instrumented plan inputs = %v, want %v",
			runner.planBindingEvidence.InstrumentedAdapterInputs,
			contract.PlanBinding.InstrumentedAdapter.Inputs,
		)
	}
	wantIsolationEvidence := []string{"authenticated-principal", "authenticated-tenant", "space"}
	if !reflect.DeepEqual(
		runner.idempotencyEvidence.IsolationDimensions,
		wantIsolationEvidence,
	) {
		return HostRunnerReport{}, fmt.Errorf(
			"portable host runner idempotency isolation evidence = %v, want %v",
			runner.idempotencyEvidence.IsolationDimensions,
			wantIsolationEvidence,
		)
	}
	if !reflect.DeepEqual(
		runner.idempotencyEvidence.ReplayAuthorizationDenials,
		contract.Idempotency.ReplayDenialCodes,
	) || !runner.idempotencyEvidence.SuccessReplayPreserved {
		return HostRunnerReport{}, fmt.Errorf(
			"portable host runner replay authorization evidence is incomplete: %#v",
			runner.idempotencyEvidence,
		)
	}
	return HostRunnerReport{
		Format: HostRunnerReportFormat, Classification: classification,
		PublicationReady: false, Status: "passed", Subject: subject,
		RunnerSubject: contract.RunnerEvidence.Subject, RunnerInputDigest: contract.RunnerEvidence.SHA256,
		Checks: checks, ErrorProbes: runner.errorProbes, NegativeFixtures: runner.negativeFixtures,
		GenerationTransitions: runner.generations, PlanBindingEvidence: runner.planBindingEvidence,
		IdempotencyEvidence: runner.idempotencyEvidence,
		InterfaceEvidence:   runner.interfaceEvidence,
	}, nil
}

type endpointRunner struct {
	ctx                  context.Context
	contract             Contract
	endpoint             string
	token                string
	alternateToken       string
	alternateTenantToken string
	httpClient           *http.Client

	apiBase             string
	formsURL            string
	interfacesURL       string
	completed           map[string]bool
	errorProbes         []ErrorProbeEvidence
	negativeFixtures    []NegativeFixtureEvidence
	generations         []string
	planBindingEvidence PlanBindingRunnerEvidence
	idempotencyEvidence IdempotencyRunnerEvidence
	interfaceEvidence   InterfaceRunnerEvidence
}

type wireResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

type applyRequest struct {
	client.Resource
	Review client.DeploymentReview `json:"review"`
}

type importRequest struct {
	client.Resource
	NativeID string `json:"nativeId"`
}

type resourceEnvelope struct {
	Resource client.Resource `json:"resource"`
}

func (r *endpointRunner) run() error {
	exact := toClientIdentity(r.contract.RunnerInput.Identity)
	resource := client.Resource{
		APIVersion: client.APIVersion,
		Kind:       exact.FormRef.Kind,
		Form:       &exact,
		Metadata: client.Metadata{
			Name:  r.contract.RunnerInput.Name,
			Space: r.contract.RunnerInput.Space,
		},
		Spec: cloneMap(r.contract.RunnerInput.Desired),
	}

	formClient := client.NewWithOptions(r.endpoint, r.token, r.httpClient, client.Options{RetryAttempts: 1})
	discovery, err := formClient.Discover(r.ctx)
	if err != nil {
		return fmt.Errorf("discovery: %w", err)
	}
	r.apiBase = strings.TrimRight(discovery.Endpoints.API, "/")
	r.formsURL = r.apiBase + "/forms"
	if discovery.Endpoints.Forms != "" {
		r.formsURL = strings.TrimRight(discovery.Endpoints.Forms, "/")
	}
	r.interfacesURL = r.apiBase + "/interfaces"
	if discovery.Endpoints.Interfaces != "" {
		r.interfacesURL = strings.TrimRight(discovery.Endpoints.Interfaces, "/")
	}
	r.complete("discovery")

	for _, operation := range []string{"create", "read", "update", "delete", "import", "observe", "refresh"} {
		if err := r.ensureExactAvailability(operation); err != nil {
			return fmt.Errorf("exact availability for %s: %w", operation, err)
		}
	}
	r.complete("exact-availability")

	if !discovery.HasFeature(client.FeatureInterfaceDeclarations) {
		return errors.New("selected runner Form has a required Interface but the conformance endpoint does not advertise interface_declarations")
	}
	r.complete("interface-required-feature-advertised")
	if err := r.verifyInterfaceEndpointOrigin(discovery); err != nil {
		return err
	}
	r.complete("interface-endpoint-same-origin")
	if err := r.verifyInterfaceSpaceRequired(); err != nil {
		return err
	}
	r.complete("interface-space-required")
	if err := r.verifyInterfaceQueryVocabulary(); err != nil {
		return err
	}
	r.complete("interface-query-vocabulary-closed")
	listed, err := formClient.ListInterfaces(r.ctx, r.contract.RunnerInput.Space)
	if err != nil {
		return fmt.Errorf("Interface list before Ready: %w", err)
	}
	if len(listed) != 0 {
		return fmt.Errorf("Interface readiness: %d declarations visible before a Ready Resource", len(listed))
	}
	r.interfaceEvidence.AbsentBeforeReady = true
	r.complete("interface-absent-before-ready")

	preview, err := formClient.PreviewResource(r.ctx, &resource)
	if err != nil {
		return fmt.Errorf("preview: %w", err)
	}
	r.complete("preview")
	if err := r.verifyRawJSONBoundary(resource); err != nil {
		return err
	}
	for _, fixture := range r.contract.RunnerInput.NegativeFixtures {
		negative := resource
		negative.Spec = cloneMap(fixture.Input)
		response, err := r.request(http.MethodPost, r.apiBase+"/resources/preview", map[string]string{
			"If-None-Match": "*",
		}, negative)
		if err != nil {
			return err
		}
		if err := expectStableError(response, "invalid_argument"); err != nil {
			return fmt.Errorf("desired negative fixture %s: %w", fixture.Name, err)
		}
		r.negativeFixtures = append(r.negativeFixtures, NegativeFixtureEvidence{
			Name: fixture.Name, Stage: fixture.Stage, SHA256: fixture.SHA256,
		})
	}
	negativeReadback, err := r.request(http.MethodGet, r.resourceURLWithExactQuery(""), nil, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(negativeReadback, "resource_not_found"); err != nil {
		return fmt.Errorf("desired negative fixtures mutated Resource state: %w", err)
	}
	r.complete("desired-negative-fixtures")
	for _, invalidVersion := range []string{
		"0",
		"01",
		"-1",
		"+1",
		"1.0",
		"1e0",
		" 1",
		"1 ",
		"not-decimal",
		"9223372036854775808",
	} {
		invalid := resource
		invalid.Metadata.ResourceVersion = invalidVersion
		response, err := r.request(http.MethodPost, r.apiBase+"/resources/preview", map[string]string{
			"If-Match": `"` + invalidVersion + `"`,
		}, invalid)
		if err != nil {
			return err
		}
		if err := expectStableError(response, "invalid_argument"); err != nil {
			return fmt.Errorf("resourceVersion bound %q: %w", invalidVersion, err)
		}
	}
	maxVersion := "9223372036854775807"
	maxBound := resource
	maxBound.Metadata.ResourceVersion = maxVersion
	maxBoundResponse, err := r.request(http.MethodPost, r.apiBase+"/resources/preview", map[string]string{
		"If-Match": `"` + maxVersion + `"`,
	}, maxBound)
	if err != nil {
		return err
	}
	// The reference resource is absent here, so an in-range generation reaches
	// the fence and fails as stale rather than failing argument validation.
	if err := expectStableError(maxBoundResponse, "resource_version_conflict"); err != nil {
		return fmt.Errorf("resourceVersion inclusive maximum %q: %w", maxVersion, err)
	}
	r.complete("resource-version-bounds")

	altered := resource
	altered.Spec = cloneMap(resource.Spec)
	altered.Spec["storageClass"] = "archive"
	if err := r.expectPurePlanSubstitutionRejected(
		"spec",
		"portable-plan-substitution",
		altered,
		preview.Review.PlanDigest,
	); err != nil {
		return fmt.Errorf("preview plan/spec binding: %w", err)
	}
	r.planBindingEvidence.PureBlackBoxInputs = append(
		r.planBindingEvidence.PureBlackBoxInputs,
		"specDigest",
	)
	r.complete("preview-plan-spec-binding")

	for _, substitution := range []struct {
		name   string
		input  string
		mutate func(*client.Resource)
	}{
		{
			name:  "api-version",
			input: "resource.apiVersion",
			mutate: func(candidate *client.Resource) {
				candidate.APIVersion = "forms.takoform.com/v1alpha2"
			},
		},
		{
			name:  "kind",
			input: "resource.kind",
			mutate: func(candidate *client.Resource) {
				candidate.Kind = "OtherBucket"
			},
		},
		{
			name:  "name",
			input: "resource.metadata.name",
			mutate: func(candidate *client.Resource) {
				candidate.Metadata.Name = "other-object-bucket"
			},
		},
	} {
		candidate := cloneRunnerResource(resource)
		substitution.mutate(&candidate)
		if err := r.expectInstrumentedPlanSubstitutionRejected(
			"resource-identity-"+substitution.name,
			substitution.input,
			candidate,
			preview.Review.PlanDigest,
		); err != nil {
			return fmt.Errorf("preview plan Resource identity binding: %w", err)
		}
		r.planBindingEvidence.InstrumentedAdapterInputs = append(
			r.planBindingEvidence.InstrumentedAdapterInputs,
			substitution.input,
		)
	}

	spaceSubstitution := cloneRunnerResource(resource)
	spaceSubstitution.Metadata.Space = r.contract.RunnerInput.AlternateSpace
	if err := r.expectPurePlanSubstitutionRejected(
		"space",
		"portable-plan-substitution-space",
		spaceSubstitution,
		preview.Review.PlanDigest,
	); err != nil {
		return fmt.Errorf("preview plan Space binding: %w", err)
	}
	r.planBindingEvidence.PureBlackBoxInputs = append(
		r.planBindingEvidence.PureBlackBoxInputs,
		"resource.metadata.space",
	)
	r.complete("preview-plan-space-binding")

	for _, substitution := range []struct {
		name   string
		input  string
		mutate func(*client.FormRef)
	}{
		{
			name:  "api-version",
			input: "resource.form.formRef.apiVersion",
			mutate: func(ref *client.FormRef) {
				ref.APIVersion = "forms.takoform.com/v1alpha2"
			},
		},
		{
			name:  "kind",
			input: "resource.form.formRef.kind",
			mutate: func(ref *client.FormRef) {
				ref.Kind = "OtherBucket"
			},
		},
		{
			name:  "definition-version",
			input: "resource.form.formRef.definitionVersion",
			mutate: func(ref *client.FormRef) {
				ref.DefinitionVersion = "999.0.0"
			},
		},
		{
			name:  "schema-digest",
			input: "resource.form.formRef.schemaDigest",
			mutate: func(ref *client.FormRef) {
				ref.SchemaDigest = digestBytes([]byte("substituted-schema"))
			},
		},
	} {
		candidate := cloneRunnerResource(resource)
		substitution.mutate(&candidate.Form.FormRef)
		if err := r.expectInstrumentedPlanSubstitutionRejected(
			"form-ref-"+substitution.name,
			substitution.input,
			candidate,
			preview.Review.PlanDigest,
		); err != nil {
			return fmt.Errorf("preview plan exact FormRef binding: %w", err)
		}
		r.planBindingEvidence.InstrumentedAdapterInputs = append(
			r.planBindingEvidence.InstrumentedAdapterInputs,
			substitution.input,
		)
	}
	r.complete("preview-plan-form-ref-binding")

	packageSubstitution := cloneRunnerResource(resource)
	packageSubstitution.Form.PackageDigest = digestBytes([]byte("substituted-package"))
	if err := r.expectInstrumentedPlanSubstitutionRejected(
		"package-digest",
		"resource.form.packageDigest",
		packageSubstitution,
		preview.Review.PlanDigest,
	); err != nil {
		return fmt.Errorf("preview plan package digest binding: %w", err)
	}
	r.planBindingEvidence.InstrumentedAdapterInputs = append(
		r.planBindingEvidence.InstrumentedAdapterInputs,
		"resource.form.packageDigest",
	)
	r.complete("preview-plan-package-digest-binding")
	r.complete("invalid-argument-normalization")

	createRequest := applyRequest{
		Resource: resource,
		Review:   client.DeploymentReview{PlanDigest: preview.Review.PlanDigest},
	}
	applyWithoutKey, err := r.request(http.MethodPut, r.resourceURL(), map[string]string{
		"If-None-Match": "*",
	}, createRequest)
	if err != nil {
		return err
	}
	if err := expectStableError(applyWithoutKey, "invalid_argument"); err != nil {
		return fmt.Errorf("apply missing Idempotency-Key: %w", err)
	}
	applyWithoutFence, err := r.request(http.MethodPut, r.resourceURL(), map[string]string{
		"Idempotency-Key": "portable-apply-without-fence",
	}, createRequest)
	if err != nil {
		return err
	}
	if err := expectStableError(applyWithoutFence, "resource_version_conflict"); err != nil {
		return fmt.Errorf("apply missing generation fence: %w", err)
	}
	r.complete("apply-headers-required")
	createHeaders := map[string]string{
		"If-None-Match": "*", "Idempotency-Key": "portable-apply-create",
	}
	created, err := r.request(http.MethodPut, r.resourceURL(), createHeaders, createRequest)
	if err != nil {
		return err
	}
	if created.Status != http.StatusCreated && created.Status != http.StatusOK {
		return fmt.Errorf("apply returned HTTP %d", created.Status)
	}
	createdResource, err := decodeResourceResponse(created)
	if err != nil {
		return fmt.Errorf("apply response: %w", err)
	}
	if err := verifyRunnerResource(createdResource, resource, "1", r.contract.RunnerInput.Definition); err != nil {
		return fmt.Errorf("apply response: %w", err)
	}
	r.generations = append(r.generations, "1")
	r.complete("apply")

	createReplay, err := r.request(http.MethodPut, r.resourceURL(), createHeaders, createRequest)
	if err != nil {
		return err
	}
	if !sameWireResponse(created, createReplay) {
		return errors.New("apply idempotent replay changed status, ETag, or response body")
	}
	r.complete("apply-idempotency")
	crossPrincipal, err := r.requestWithToken(
		http.MethodPut,
		r.resourceURL(),
		createHeaders,
		createRequest,
		r.alternateToken,
	)
	if err != nil {
		return err
	}
	if err := expectStableError(crossPrincipal, "resource_version_conflict"); err != nil {
		return fmt.Errorf("cross-principal Idempotency-Key isolation: %w", err)
	}
	r.complete("idempotency-cross-principal-isolated")
	r.idempotencyEvidence.IsolationDimensions = append(
		r.idempotencyEvidence.IsolationDimensions,
		"authenticated-principal",
	)
	crossTenant, err := r.requestWithToken(
		http.MethodPut,
		r.resourceURL(),
		createHeaders,
		createRequest,
		r.alternateTenantToken,
	)
	if err != nil {
		return err
	}
	if err := expectStableError(crossTenant, "resource_version_conflict"); err != nil {
		return fmt.Errorf("cross-tenant Idempotency-Key isolation: %w", err)
	}
	r.complete("idempotency-cross-tenant-isolated")
	r.idempotencyEvidence.IsolationDimensions = append(
		r.idempotencyEvidence.IsolationDimensions,
		"authenticated-tenant",
	)

	for _, denial := range []struct {
		probe string
		code  string
		check string
		label string
	}{
		{
			probe: AuthorizationProbeCredentialRevoked,
			code:  "unauthenticated",
			check: "idempotency-replay-authentication-before-cache",
			label: "credential revocation",
		},
		{
			probe: AuthorizationProbePermissionRevoked,
			code:  "permission_denied",
			check: "idempotency-replay-permission-before-cache",
			label: "permission revocation",
		},
		{
			probe: AuthorizationProbePolicyRevoked,
			code:  "policy_denied",
			check: "idempotency-replay-policy-before-cache",
			label: "policy revocation",
		},
	} {
		deniedHeaders := cloneStringMap(createHeaders)
		deniedHeaders[AuthorizationProbeHeader] = denial.probe
		deniedReplay, err := r.request(
			http.MethodPut,
			r.resourceURL(),
			deniedHeaders,
			createRequest,
		)
		if err != nil {
			return err
		}
		if err := expectStableError(deniedReplay, denial.code); err != nil {
			return fmt.Errorf("%s before replay lookup: %w", denial.label, err)
		}
		r.complete(denial.check)
		r.idempotencyEvidence.ReplayAuthorizationDenials = append(
			r.idempotencyEvidence.ReplayAuthorizationDenials,
			denial.code,
		)

		unprobedReplay, err := r.request(
			http.MethodPut,
			r.resourceURL(),
			createHeaders,
			createRequest,
		)
		if err != nil {
			return err
		}
		if !sameWireResponse(created, unprobedReplay) {
			return fmt.Errorf("%s changed or poisoned the cached success", denial.label)
		}
	}
	r.idempotencyEvidence.SuccessReplayPreserved = true
	if err := r.verifyResourceSpaceIsolation(formClient, resource); err != nil {
		return err
	}
	if err := r.verifyConnectionSpaceIsolation(formClient); err != nil {
		return err
	}
	reusedKeyRequest := createRequest
	reusedKeyRequest.Spec = cloneMap(createRequest.Spec)
	reusedKeyRequest.Spec["storageClass"] = "archive"
	reusedKey, err := r.request(http.MethodPut, r.resourceURL(), createHeaders, reusedKeyRequest)
	if err != nil {
		return err
	}
	if err := expectStableError(reusedKey, "invalid_argument"); err != nil {
		return fmt.Errorf("Idempotency-Key request binding: %w", err)
	}
	r.complete("idempotency-key-reuse-rejected")

	createCollision, err := r.request(http.MethodPut, r.resourceURL(), map[string]string{
		"If-None-Match": "*", "Idempotency-Key": "portable-apply-collision",
	}, createRequest)
	if err != nil {
		return err
	}
	if err := expectStableError(createCollision, "resource_version_conflict"); err != nil {
		return fmt.Errorf("create precondition conflict: %w", err)
	}
	r.complete("create-precondition-conflict")

	if err := r.verifyReadyInterface(formClient, exact); err != nil {
		return err
	}
	if err := r.verifyInterfaceReadOnly(formClient); err != nil {
		return err
	}
	r.complete("portable-interface-routes-reject-writes")
	if err := r.verifyInterfaceSpaceIsolation(formClient, resource, exact); err != nil {
		return err
	}
	r.complete("interface-space-isolation")
	if err := r.verifyMultiResourceInstanceAmbiguity(formClient, resource, exact); err != nil {
		return err
	}
	r.complete("interface-multi-resource-instance-fails-closed")
	r.interfaceEvidence.ExactReadyProjection = true
	r.complete("interface-ready-projection")
	r.complete("interface-required-ready-projection-present")

	update := resource
	update.Metadata.ResourceVersion = "1"
	update.Spec = cloneMap(resource.Spec)
	update.Spec["storageClass"] = "infrequent_access"
	updatePreview, err := formClient.PreviewResource(r.ctx, &update)
	if err != nil {
		return fmt.Errorf("update preview: %w", err)
	}
	updateRequest := applyRequest{
		Resource: update,
		Review:   client.DeploymentReview{PlanDigest: updatePreview.Review.PlanDigest},
	}
	updateWithoutKey, err := r.request(http.MethodPut, r.resourceURL(), map[string]string{
		"If-Match": `"1"`,
	}, updateRequest)
	if err != nil {
		return err
	}
	if err := expectStableError(updateWithoutKey, "invalid_argument"); err != nil {
		return fmt.Errorf("update missing Idempotency-Key: %w", err)
	}
	updateWithoutFence, err := r.request(http.MethodPut, r.resourceURL(), map[string]string{
		"Idempotency-Key": "portable-update-without-fence",
	}, updateRequest)
	if err != nil {
		return err
	}
	if err := expectStableError(updateWithoutFence, "resource_version_conflict"); err != nil {
		return fmt.Errorf("update missing generation fence: %w", err)
	}
	r.complete("update-headers-required")
	updateHeaders := map[string]string{
		"If-Match": `"1"`, "Idempotency-Key": "portable-apply-update",
	}
	updated, err := r.request(http.MethodPut, r.resourceURL(), updateHeaders, updateRequest)
	if err != nil {
		return err
	}
	if updated.Status != http.StatusOK {
		return fmt.Errorf("update returned HTTP %d", updated.Status)
	}
	updatedResource, err := decodeResourceResponse(updated)
	if err != nil {
		return fmt.Errorf("update response: %w", err)
	}
	if err := verifyRunnerResource(updatedResource, update, "2", r.contract.RunnerInput.Definition); err != nil {
		return fmt.Errorf("update response: %w", err)
	}
	r.generations = append(r.generations, "2")
	r.complete("update")

	// P1 was reviewed against generation 1. After that exact update advances
	// the Resource to generation 2, the same desired body with generation 2
	// and If-Match "2" is an otherwise valid update. Only the old plan's
	// resourceVersion binding may reject it.
	generationSubstitution := cloneRunnerResource(update)
	generationSubstitution.Metadata.ResourceVersion = "2"
	if err := r.expectPureUpdatePlanSubstitutionRejected(
		"resource-identity-resource-version",
		generationSubstitution,
		updatePreview.Review.PlanDigest,
	); err != nil {
		return fmt.Errorf("preview plan Resource identity binding: %w", err)
	}
	r.planBindingEvidence.PureBlackBoxInputs = append(
		r.planBindingEvidence.PureBlackBoxInputs,
		"resource.metadata.resourceVersion",
	)
	r.complete("preview-plan-resource-identity-binding")

	updateReplay, err := r.request(http.MethodPut, r.resourceURL(), updateHeaders, updateRequest)
	if err != nil {
		return err
	}
	if !sameWireResponse(updated, updateReplay) {
		return errors.New("update idempotent replay changed status, ETag, or response body")
	}
	r.complete("update-idempotency")
	if err := r.verifyReadyInterface(formClient, exact); err != nil {
		return fmt.Errorf("Interface readiness after update: %w", err)
	}
	r.complete("interface-ready-after-update")

	staleUpdate, err := r.request(http.MethodPut, r.resourceURL(), map[string]string{
		"If-Match": `"1"`, "Idempotency-Key": "portable-stale-update",
	}, updateRequest)
	if err != nil {
		return err
	}
	if err := expectStableError(staleUpdate, "resource_version_conflict"); err != nil {
		return fmt.Errorf("stale update: %w", err)
	}
	r.complete("stale-update-rejected")

	read, err := formClient.GetResource(r.ctx, resource.Kind, resource.Metadata.Name, resource.Metadata.Space, exact)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if err := verifyRunnerResource(*read, update, "2", r.contract.RunnerInput.Definition); err != nil {
		return fmt.Errorf("canonical read parity: %w", err)
	}
	r.complete("read")
	r.complete("canonical-resource-parity")

	substitution, err := r.request(http.MethodGet, r.resourceURLWithExactQuery(
		"sha256:"+strings.Repeat("f", 64),
	), nil, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(substitution, "form_identity_conflict"); err != nil {
		return fmt.Errorf("exact digest substitution: %w", err)
	}
	r.complete("exact-digest-substitution-rejected")

	observeHeaders := map[string]string{
		"If-Match": `"2"`, "Idempotency-Key": "portable-observe",
	}
	observed, err := r.request(http.MethodPost, r.actionURL("observe"), observeHeaders, nil)
	if err != nil {
		return err
	}
	observedResource, err := decodeResourceEnvelope(observed, http.StatusOK)
	if err != nil {
		return fmt.Errorf("observe: %w", err)
	}
	if err := verifyRunnerResource(observedResource, update, "2", r.contract.RunnerInput.Definition); err != nil {
		return fmt.Errorf("observe response: %w", err)
	}
	r.complete("observe")
	observedReplay, err := r.request(http.MethodPost, r.actionURL("observe"), observeHeaders, nil)
	if err != nil {
		return err
	}
	if !sameWireResponse(observed, observedReplay) {
		return errors.New("observe idempotent replay changed status, ETag, or response body")
	}
	r.complete("observe-idempotency")
	observeWithoutKey, err := r.request(http.MethodPost, r.actionURL("observe"), map[string]string{
		"If-Match": `"2"`,
	}, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(observeWithoutKey, "invalid_argument"); err != nil {
		return fmt.Errorf("observe missing Idempotency-Key: %w", err)
	}
	observeWithoutFence, err := r.request(http.MethodPost, r.actionURL("observe"), map[string]string{
		"Idempotency-Key": "portable-observe-without-fence",
	}, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(observeWithoutFence, "resource_version_conflict"); err != nil {
		return fmt.Errorf("observe missing generation fence: %w", err)
	}
	r.complete("observe-headers-required")
	staleObserve, err := r.request(http.MethodPost, r.actionURL("observe"), map[string]string{
		"If-Match": `"1"`, "Idempotency-Key": "portable-stale-observe",
	}, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(staleObserve, "resource_version_conflict"); err != nil {
		return fmt.Errorf("observe generation fence: %w", err)
	}
	r.complete("observe-generation-fence")

	refreshHeaders := map[string]string{
		"If-Match": `"2"`, "Idempotency-Key": "portable-refresh",
	}
	refreshed, err := r.request(http.MethodPost, r.actionURL("refresh"), refreshHeaders, nil)
	if err != nil {
		return err
	}
	refreshedResource, err := decodeResourceEnvelope(refreshed, http.StatusOK)
	if err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	if err := verifyRunnerResource(refreshedResource, update, "2", r.contract.RunnerInput.Definition); err != nil {
		return fmt.Errorf("refresh response: %w", err)
	}
	r.complete("refresh")
	refreshedReplay, err := r.request(http.MethodPost, r.actionURL("refresh"), refreshHeaders, nil)
	if err != nil {
		return err
	}
	if !sameWireResponse(refreshed, refreshedReplay) {
		return errors.New("refresh idempotent replay changed status, ETag, or response body")
	}
	r.complete("refresh-idempotency")
	refreshWithoutKey, err := r.request(http.MethodPost, r.actionURL("refresh"), map[string]string{
		"If-Match": `"2"`,
	}, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(refreshWithoutKey, "invalid_argument"); err != nil {
		return fmt.Errorf("refresh missing Idempotency-Key: %w", err)
	}
	refreshWithoutFence, err := r.request(http.MethodPost, r.actionURL("refresh"), map[string]string{
		"Idempotency-Key": "portable-refresh-without-fence",
	}, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(refreshWithoutFence, "resource_version_conflict"); err != nil {
		return fmt.Errorf("refresh missing generation fence: %w", err)
	}
	r.complete("refresh-headers-required")
	staleRefresh, err := r.request(http.MethodPost, r.actionURL("refresh"), map[string]string{
		"If-Match": `"1"`, "Idempotency-Key": "portable-stale-refresh",
	}, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(staleRefresh, "resource_version_conflict"); err != nil {
		return fmt.Errorf("refresh generation fence: %w", err)
	}
	r.complete("refresh-generation-fence")

	staleDelete, err := r.request(http.MethodDelete, r.resourceURLWithExactQuery(""), map[string]string{
		"If-Match": `"1"`, "Idempotency-Key": "portable-stale-delete",
	}, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(staleDelete, "resource_version_conflict"); err != nil {
		return fmt.Errorf("stale delete: %w", err)
	}
	r.complete("stale-delete-rejected")

	if err := r.probeStableErrors(resource); err != nil {
		return err
	}
	r.complete("stable-error-http-status-mapping")
	r.complete("retryable-code-semantics")

	deleteHeaders := map[string]string{
		"If-Match": `"2"`, "Idempotency-Key": "portable-delete",
	}
	deleteWithoutKey, err := r.request(http.MethodDelete, r.resourceURLWithExactQuery(""), map[string]string{
		"If-Match": `"2"`,
	}, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(deleteWithoutKey, "invalid_argument"); err != nil {
		return fmt.Errorf("delete missing Idempotency-Key: %w", err)
	}
	deleteWithoutFence, err := r.request(http.MethodDelete, r.resourceURLWithExactQuery(""), map[string]string{
		"Idempotency-Key": "portable-delete-without-fence",
	}, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(deleteWithoutFence, "resource_version_conflict"); err != nil {
		return fmt.Errorf("delete missing generation fence: %w", err)
	}
	r.complete("delete-headers-required")
	deleted, err := r.request(http.MethodDelete, r.resourceURLWithExactQuery(""), deleteHeaders, nil)
	if err != nil {
		return err
	}
	if deleted.Status != http.StatusNoContent || len(bytes.TrimSpace(deleted.Body)) != 0 {
		return fmt.Errorf("delete returned HTTP %d with %d response bytes", deleted.Status, len(deleted.Body))
	}
	r.complete("delete")
	deleteReplay, err := r.request(http.MethodDelete, r.resourceURLWithExactQuery(""), deleteHeaders, nil)
	if err != nil {
		return err
	}
	if !sameWireResponse(deleted, deleteReplay) {
		return errors.New("delete idempotent replay changed status, ETag, or response body")
	}
	r.complete("delete-idempotency")

	if err := r.expectNoInterfaces(formClient, "after delete"); err != nil {
		return err
	}
	r.interfaceEvidence.AbsentAfterDelete = true
	r.complete("interface-absent-after-delete")

	importBody := resource
	importRequestBody := importRequest{
		Resource: importBody,
		NativeID: r.contract.RunnerInput.ImportNativeID,
	}
	importHeaders := map[string]string{
		"If-None-Match": "*", "Idempotency-Key": "portable-import",
	}
	importWithoutKey, err := r.request(http.MethodPost, r.resourceURL()+"/import", map[string]string{
		"If-None-Match": "*",
	}, importRequestBody)
	if err != nil {
		return err
	}
	if err := expectStableError(importWithoutKey, "invalid_argument"); err != nil {
		return fmt.Errorf("import missing Idempotency-Key: %w", err)
	}
	importWithoutFence, err := r.request(http.MethodPost, r.resourceURL()+"/import", map[string]string{
		"Idempotency-Key": "portable-import-without-fence",
	}, importRequestBody)
	if err != nil {
		return err
	}
	if err := expectStableError(importWithoutFence, "resource_version_conflict"); err != nil {
		return fmt.Errorf("import missing generation fence: %w", err)
	}
	r.complete("import-headers-required")
	imported, err := r.request(http.MethodPost, r.resourceURL()+"/import", importHeaders, importRequestBody)
	if err != nil {
		return err
	}
	importedResource, err := decodeResourceEnvelope(imported, http.StatusOK, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	if err := verifyRunnerResource(importedResource, importBody, "1", r.contract.RunnerInput.Definition); err != nil {
		return fmt.Errorf("import response: %w", err)
	}
	r.generations = append(r.generations, "1")
	r.complete("import")
	importReplay, err := r.request(http.MethodPost, r.resourceURL()+"/import", importHeaders, importRequestBody)
	if err != nil {
		return err
	}
	if !sameWireResponse(imported, importReplay) {
		return errors.New("import idempotent replay changed status, ETag, or response body")
	}
	r.complete("import-idempotency")
	if err := r.verifyReadyInterface(formClient, exact); err != nil {
		return fmt.Errorf("Interface readiness after import: %w", err)
	}
	r.complete("interface-ready-after-import")

	importUpdateResource := resource
	importUpdateResource.Metadata.ResourceVersion = "1"
	importUpdateResource.Spec = cloneMap(resource.Spec)
	importUpdateResource.Spec["storageClass"] = "archive"
	importUpdateBody := importRequest{
		Resource: importUpdateResource,
		NativeID: r.contract.RunnerInput.ImportNativeID,
	}
	importUpdateHeaders := map[string]string{
		"If-Match": `"1"`, "Idempotency-Key": "portable-import-update",
	}
	importUpdateWithoutKey, err := r.request(http.MethodPost, r.resourceURL()+"/import", map[string]string{
		"If-Match": `"1"`,
	}, importUpdateBody)
	if err != nil {
		return err
	}
	if err := expectStableError(importUpdateWithoutKey, "invalid_argument"); err != nil {
		return fmt.Errorf("existing-resource import missing Idempotency-Key: %w", err)
	}
	importUpdateWithoutFence, err := r.request(http.MethodPost, r.resourceURL()+"/import", map[string]string{
		"Idempotency-Key": "portable-import-update-without-fence",
	}, importUpdateBody)
	if err != nil {
		return err
	}
	if err := expectStableError(importUpdateWithoutFence, "resource_version_conflict"); err != nil {
		return fmt.Errorf("existing-resource import missing generation fence: %w", err)
	}
	r.complete("import-update-headers-required")
	importUpdateStale, err := r.request(http.MethodPost, r.resourceURL()+"/import", map[string]string{
		"If-Match": `"999"`, "Idempotency-Key": "portable-import-update-stale",
	}, importUpdateBody)
	if err != nil {
		return err
	}
	if err := expectStableError(importUpdateStale, "resource_version_conflict"); err != nil {
		return fmt.Errorf("existing-resource import stale generation fence: %w", err)
	}
	r.complete("import-update-stale-rejected")
	importUpdated, err := r.request(http.MethodPost, r.resourceURL()+"/import", importUpdateHeaders, importUpdateBody)
	if err != nil {
		return err
	}
	importUpdatedResource, err := decodeResourceEnvelope(importUpdated, http.StatusOK, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("existing-resource import: %w", err)
	}
	if err := verifyRunnerResource(importUpdatedResource, importUpdateResource, "2", r.contract.RunnerInput.Definition); err != nil {
		return fmt.Errorf("existing-resource import response: %w", err)
	}
	r.generations = append(r.generations, "2")
	importUpdateReplay, err := r.request(http.MethodPost, r.resourceURL()+"/import", importUpdateHeaders, importUpdateBody)
	if err != nil {
		return err
	}
	if !sameWireResponse(importUpdated, importUpdateReplay) {
		return errors.New("existing-resource import idempotent replay changed status, ETag, or response body")
	}
	r.complete("import-update")
	if err := r.verifyReadyInterface(formClient, exact); err != nil {
		return fmt.Errorf("Interface readiness after existing-resource import: %w", err)
	}
	r.complete("interface-ready-after-import-update")

	finalDeleteHeaders := map[string]string{
		"If-Match": `"2"`, "Idempotency-Key": "portable-import-delete",
	}
	finalDelete, err := r.request(http.MethodDelete, r.resourceURLWithExactQuery(""), finalDeleteHeaders, nil)
	if err != nil {
		return err
	}
	if finalDelete.Status != http.StatusNoContent || len(bytes.TrimSpace(finalDelete.Body)) != 0 {
		return fmt.Errorf(
			"post-import delete returned HTTP %d with %d body bytes",
			finalDelete.Status,
			len(finalDelete.Body),
		)
	}
	finalDeleteReplay, err := r.request(http.MethodDelete, r.resourceURLWithExactQuery(""), finalDeleteHeaders, nil)
	if err != nil {
		return err
	}
	if !sameWireResponse(finalDelete, finalDeleteReplay) {
		return errors.New("post-import delete idempotent replay changed response")
	}
	finalReadback, err := r.request(http.MethodGet, r.resourceURLWithExactQuery(""), nil, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(finalReadback, "resource_not_found"); err != nil {
		return fmt.Errorf("post-import delete readback: %w", err)
	}
	if err := r.expectNoInterfaces(formClient, "after post-import delete"); err != nil {
		return err
	}
	r.complete("post-import-delete-readback")
	return nil
}

func (r *endpointRunner) ensureExactAvailability(operation string) error {
	query := r.exactQuery("")
	response, err := r.request(http.MethodGet, r.formsURL+"?"+query.Encode(), nil, nil)
	if err != nil {
		return err
	}
	if response.Status != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", response.Status, strings.TrimSpace(string(response.Body)))
	}
	var body struct {
		Forms []client.FormAvailability `json:"forms"`
	}
	if err := decodeStrictBytes(response.Body, &body); err != nil {
		return err
	}
	if len(body.Forms) != 1 || !reflect.DeepEqual(body.Forms[0].Identity, toClientIdentity(r.contract.RunnerInput.Identity)) {
		return errors.New("host did not return the exact installed Form identity")
	}
	form := body.Forms[0]
	if !form.DefinitionKnown || !form.Installed || !form.Executable || !form.Activated || !form.AvailableToPrincipal ||
		!containsValue(form.Operations, operation) {
		return fmt.Errorf("exact Form is not available for %s", operation)
	}
	return nil
}

func (r *endpointRunner) verifyReadyInterface(formClient *client.Client, exact client.InstalledFormReference) error {
	listed, err := formClient.ListInterfaces(r.ctx, r.contract.RunnerInput.Space)
	if err != nil {
		return fmt.Errorf("Interface list for Ready Resource: %w", err)
	}
	if len(listed) != 1 {
		return fmt.Errorf("Interface readiness: got %d declarations for one Ready ObjectBucket", len(listed))
	}
	got := listed[0]
	descriptor, err := r.runnerInterfaceDescriptor()
	if err != nil {
		return err
	}
	ready, err := formClient.GetResource(
		r.ctx,
		r.contract.RunnerInput.Identity.FormRef.Kind,
		r.contract.RunnerInput.Name,
		r.contract.RunnerInput.Space,
		exact,
	)
	if err != nil {
		return fmt.Errorf("read Interface-exposing Resource: %w", err)
	}
	want, err := expectedRunnerInterface(descriptor, *ready, exact)
	if err != nil {
		return err
	}
	if !sameRunnerInterface(got, want) {
		return fmt.Errorf("Interface projection differs from the exact Ready Form descriptor: %#v", got)
	}
	r.complete("interface-ready-projection-exactly-matches-form-descriptors")
	r.complete("interface-document-exact-copy")
	if err := validateRunnerSchema(descriptor.DocumentSchema, got.Document, "Interface document"); err != nil {
		return err
	}
	r.complete("interface-document-schema-valid")
	if err := formpackage.ValidatePortableData(got.Document); err != nil {
		return fmt.Errorf("Interface document projects authority data: %w", err)
	}
	if err := formpackage.ValidatePortableData(got.Values); err != nil {
		return fmt.Errorf("Interface values project authority data: %w", err)
	}
	r.complete("interface-projection-contains-no-authority-fields")

	exactRead, err := formClient.GetInterface(r.ctx, r.contract.RunnerInput.Space, client.InterfaceSelector{
		Name: got.Name, Version: got.Version,
		ResourceKind: got.Resource.Kind, ResourceName: got.Resource.Name,
	})
	if err != nil {
		return fmt.Errorf("exact Interface read: %w", err)
	}
	if !reflect.DeepEqual(exactRead, got) {
		return errors.New("exact Interface read differs from the list projection")
	}
	r.complete("interface-exact-resource-instance-read")

	pairQuery := url.Values{}
	pairQuery.Set("space", r.contract.RunnerInput.Space)
	pairQuery.Set("version", got.Version)
	pairRead, err := r.readInterface(got.Name, pairQuery)
	if err != nil {
		return fmt.Errorf("Interface exact pair read: %w", err)
	}
	if !reflect.DeepEqual(pairRead, got) {
		return errors.New("Interface exact pair read differs from the list projection")
	}
	r.complete("interface-exact-pair-read")

	uniqueVersionQuery := url.Values{}
	uniqueVersionQuery.Set("space", r.contract.RunnerInput.Space)
	uniqueVersionQuery.Set("resourceKind", got.Resource.Kind)
	uniqueVersionQuery.Set("resourceName", got.Resource.Name)
	uniqueVersionRead, err := r.readInterface(got.Name, uniqueVersionQuery)
	if err != nil {
		return fmt.Errorf("Interface omitted-version unique read: %w", err)
	}
	if !reflect.DeepEqual(uniqueVersionRead, got) {
		return errors.New("Interface omitted-version unique read differs from the list projection")
	}
	r.complete("interface-omitted-version-unique-resolves")
	return nil
}

func (r *endpointRunner) verifyInterfaceEndpointOrigin(discovery client.Discovery) error {
	apiEndpoint, err := url.Parse(discovery.Endpoints.API)
	if err != nil {
		return fmt.Errorf("Interface endpoint origin: invalid API endpoint: %w", err)
	}
	interfaceEndpoint := r.interfacesURL
	if discovery.Endpoints.Interfaces != "" {
		interfaceEndpoint = discovery.Endpoints.Interfaces
	}
	parsedInterface, err := url.Parse(interfaceEndpoint)
	if err != nil {
		return fmt.Errorf("Interface endpoint origin: invalid Interface endpoint: %w", err)
	}
	if !sameRunnerOrigin(apiEndpoint, parsedInterface) {
		return fmt.Errorf(
			"Interface endpoint origin %s differs from API origin %s",
			parsedInterface.Scheme+"://"+parsedInterface.Host,
			apiEndpoint.Scheme+"://"+apiEndpoint.Host,
		)
	}
	return nil
}

func sameRunnerOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectiveRunnerPort(left) == effectiveRunnerPort(right)
}

func effectiveRunnerPort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func (r *endpointRunner) verifyInterfaceReadOnly(formClient *client.Client) error {
	descriptor, err := r.runnerInterfaceDescriptor()
	if err != nil {
		return err
	}
	before, err := formClient.ListInterfaces(r.ctx, r.contract.RunnerInput.Space)
	if err != nil {
		return fmt.Errorf("read-only Interface baseline: %w", err)
	}
	if len(before) == 0 {
		return errors.New("read-only Interface probe requires a Ready projection")
	}
	targets := []string{
		r.interfacesURL + "?" + url.Values{"space": []string{r.contract.RunnerInput.Space}}.Encode(),
		r.interfacesURL + "/" + url.PathEscape(descriptor.Name) + "?" +
			url.Values{"space": []string{r.contract.RunnerInput.Space}}.Encode(),
	}
	for _, target := range targets {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			response, err := r.request(method, target, nil, nil)
			if err != nil {
				return err
			}
			if response.Status >= http.StatusOK && response.Status < http.StatusMultipleChoices {
				return fmt.Errorf("read-only Interface endpoint accepted %s %s with HTTP %d", method, target, response.Status)
			}
			if err := expectAnyStableError(response); err != nil {
				return fmt.Errorf("read-only Interface endpoint %s %s: %w", method, target, err)
			}
		}
	}
	after, err := formClient.ListInterfaces(r.ctx, r.contract.RunnerInput.Space)
	if err != nil {
		return fmt.Errorf("read-only Interface readback: %w", err)
	}
	if !reflect.DeepEqual(after, before) {
		return errors.New("rejected Interface writes changed the read-only projection")
	}
	return nil
}

func (r *endpointRunner) verifyInterfaceQueryVocabulary() error {
	descriptor, err := r.runnerInterfaceDescriptor()
	if err != nil {
		return err
	}
	cases := []string{
		r.interfacesURL + "?space=" + url.QueryEscape(r.contract.RunnerInput.Space) + "&unknown=1",
		r.interfacesURL + "?space=" + url.QueryEscape(r.contract.RunnerInput.Space) + "&version=1",
		r.interfacesURL + "?space=" + url.QueryEscape(r.contract.RunnerInput.Space) +
			"&space=" + url.QueryEscape(r.contract.RunnerInput.Space),
		r.interfacesURL + "/" + url.PathEscape(descriptor.Name) +
			"?space=" + url.QueryEscape(r.contract.RunnerInput.Space) + "&unknown=1",
		r.interfacesURL + "/" + url.PathEscape(descriptor.Name) +
			"?space=" + url.QueryEscape(r.contract.RunnerInput.Space) + "&resourceKind=" +
			url.QueryEscape(r.contract.RunnerInput.Identity.FormRef.Kind),
		r.interfacesURL + "/" + url.PathEscape(descriptor.Name) +
			"?space=" + url.QueryEscape(r.contract.RunnerInput.Space) + "&resourceName=" +
			url.QueryEscape(r.contract.RunnerInput.Name),
	}
	for _, target := range cases {
		response, err := r.request(http.MethodGet, target, nil, nil)
		if err != nil {
			return err
		}
		if err := expectStableError(response, "invalid_argument"); err != nil {
			return fmt.Errorf("closed Interface query vocabulary for %s: %w", target, err)
		}
	}
	return nil
}

func (r *endpointRunner) verifyConnectionSpaceIsolation(formClient *client.Client) error {
	probe := r.contract.RunnerInput.ConnectionProbe
	sourceIdentity := toClientIdentity(probe.SourceIdentity)
	availability, err := r.request(
		http.MethodGet,
		r.formsURL+"?"+r.exactQueryForIdentity(
			r.contract.RunnerInput.Space,
			probe.SourceIdentity,
			"",
		).Encode(),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if availability.Status != http.StatusOK {
		return fmt.Errorf(
			"Connection probe exact Form availability returned HTTP %d: %s",
			availability.Status,
			strings.TrimSpace(string(availability.Body)),
		)
	}

	targetTemplate := client.Resource{
		APIVersion: r.contract.APIVersion,
		Kind:       probe.TargetKind,
		Form:       pointerToRunnerIdentity(toClientIdentity(r.contract.RunnerInput.Identity)),
		Metadata:   client.Metadata{Name: probe.TargetName},
		Spec:       cloneMap(r.contract.RunnerInput.Desired),
	}
	targetTemplate.Spec["name"] = probe.TargetName
	createTarget := func(space, key string) (client.Resource, error) {
		target := cloneRunnerResource(targetTemplate)
		target.Metadata.Space = space
		preview, err := formClient.PreviewResource(r.ctx, &target)
		if err != nil {
			return client.Resource{}, fmt.Errorf("Connection target preview in Space %q: %w", space, err)
		}
		response, err := r.request(
			http.MethodPut,
			r.resourceURLForIdentity(target.Kind, target.Metadata.Name),
			map[string]string{
				"If-None-Match":   "*",
				"Idempotency-Key": key,
			},
			applyRequest{
				Resource: target,
				Review:   client.DeploymentReview{PlanDigest: preview.Review.PlanDigest},
			},
		)
		if err != nil {
			return client.Resource{}, err
		}
		created, err := decodeResourceResponse(response)
		if err != nil {
			return client.Resource{}, fmt.Errorf("Connection target apply in Space %q: %w", space, err)
		}
		return created, nil
	}
	deleteTarget := func(target client.Resource, key string) error {
		response, err := r.request(
			http.MethodDelete,
			r.resourceURLForIdentity(target.Kind, target.Metadata.Name)+"?"+
				r.exactQueryForIdentity(
					target.Metadata.Space,
					r.contract.RunnerInput.Identity,
					"",
				).Encode(),
			map[string]string{
				"If-Match":        `"` + target.Metadata.ResourceVersion + `"`,
				"Idempotency-Key": key,
			},
			nil,
		)
		if err != nil {
			return err
		}
		if response.Status != http.StatusNoContent {
			return fmt.Errorf(
				"Connection target delete in Space %q returned HTTP %d: %s",
				target.Metadata.Space,
				response.Status,
				strings.TrimSpace(string(response.Body)),
			)
		}
		return nil
	}

	primaryTarget, err := createTarget(
		r.contract.RunnerInput.Space,
		"portable-connection-primary-target",
	)
	if err != nil {
		return err
	}
	alternateTarget, err := createTarget(
		r.contract.RunnerInput.AlternateSpace,
		"portable-connection-alternate-target",
	)
	if err != nil {
		return err
	}

	source := client.Resource{
		APIVersion: r.contract.APIVersion,
		Kind:       probe.SourceIdentity.FormRef.Kind,
		Form:       pointerToRunnerIdentity(sourceIdentity),
		Metadata: client.Metadata{
			Name:  probe.SourceName,
			Space: r.contract.RunnerInput.Space,
		},
		Spec: cloneMap(probe.Desired),
	}
	sourcePreview, err := formClient.PreviewResource(r.ctx, &source)
	if err != nil {
		return fmt.Errorf("Connection source preview: %w", err)
	}
	if err := deleteTarget(primaryTarget, "portable-connection-primary-target-delete"); err != nil {
		return err
	}
	sourceApply, err := r.request(
		http.MethodPut,
		r.resourceURLForIdentity(source.Kind, source.Metadata.Name),
		map[string]string{
			"If-None-Match":   "*",
			"Idempotency-Key": "portable-connection-source-space",
		},
		applyRequest{
			Resource: source,
			Review:   client.DeploymentReview{PlanDigest: sourcePreview.Review.PlanDigest},
		},
	)
	if err != nil {
		return err
	}
	if err := expectStableError(sourceApply, "resource_not_found"); err != nil {
		return fmt.Errorf("Connection source-Space isolation: %w", err)
	}
	r.complete("connection-cross-space-target-not-found")
	sourceRead, err := r.request(
		http.MethodGet,
		r.resourceURLForIdentity(source.Kind, source.Metadata.Name)+"?"+
			r.exactQueryForIdentity(source.Metadata.Space, probe.SourceIdentity, "").Encode(),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if err := expectStableError(sourceRead, "resource_not_found"); err != nil {
		return fmt.Errorf("Connection source rejection mutated Resource state: %w", err)
	}
	r.complete("connection-missing-target-no-source-mutation")

	alternateRead, err := r.request(
		http.MethodGet,
		r.resourceURLForIdentity(alternateTarget.Kind, alternateTarget.Metadata.Name)+"?"+
			r.exactQueryForIdentity(
				alternateTarget.Metadata.Space,
				r.contract.RunnerInput.Identity,
				"",
			).Encode(),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	unchangedAlternate, err := decodeResourceResponse(alternateRead)
	if err != nil || !reflect.DeepEqual(unchangedAlternate, alternateTarget) {
		return fmt.Errorf("Connection failure changed the alternate-Space target: response=%#v err=%v", unchangedAlternate, err)
	}

	primaryTarget, err = createTarget(
		r.contract.RunnerInput.Space,
		"portable-connection-primary-target-recreate",
	)
	if err != nil {
		return err
	}
	sourcePreview, err = formClient.PreviewResource(r.ctx, &source)
	if err != nil {
		return fmt.Errorf("Connection same-Space source preview: %w", err)
	}
	sourceApply, err = r.request(
		http.MethodPut,
		r.resourceURLForIdentity(source.Kind, source.Metadata.Name),
		map[string]string{
			"If-None-Match":   "*",
			"Idempotency-Key": "portable-connection-source-same-space",
		},
		applyRequest{
			Resource: source,
			Review:   client.DeploymentReview{PlanDigest: sourcePreview.Review.PlanDigest},
		},
	)
	if err != nil {
		return err
	}
	sourceCreated, err := decodeResourceResponse(sourceApply)
	if err != nil {
		return fmt.Errorf("Connection same-Space source apply: %w", err)
	}
	if sourceCreated.APIVersion != source.APIVersion ||
		sourceCreated.Kind != source.Kind ||
		!reflect.DeepEqual(sourceCreated.Form, source.Form) ||
		sourceCreated.Metadata.Name != source.Metadata.Name ||
		sourceCreated.Metadata.Space != source.Metadata.Space ||
		sourceCreated.Metadata.ResourceVersion != "1" ||
		!sameCanonicalRunnerJSON(sourceCreated.Spec, source.Spec) ||
		sourceCreated.Status == nil {
		return fmt.Errorf("Connection same-Space source response substituted the Resource: %#v", sourceCreated)
	}
	r.complete("connection-same-space-resolves")

	sourceDelete, err := r.request(
		http.MethodDelete,
		r.resourceURLForIdentity(source.Kind, source.Metadata.Name)+"?"+
			r.exactQueryForIdentity(source.Metadata.Space, probe.SourceIdentity, "").Encode(),
		map[string]string{
			"If-Match":        `"1"`,
			"Idempotency-Key": "portable-connection-source-delete",
		},
		nil,
	)
	if err != nil {
		return err
	}
	if sourceDelete.Status != http.StatusNoContent {
		return fmt.Errorf("Connection source cleanup returned HTTP %d", sourceDelete.Status)
	}
	if err := deleteTarget(primaryTarget, "portable-connection-primary-target-cleanup"); err != nil {
		return err
	}
	if err := deleteTarget(alternateTarget, "portable-connection-alternate-target-cleanup"); err != nil {
		return err
	}
	return nil
}

func (r *endpointRunner) verifyResourceSpaceIsolation(
	formClient *client.Client,
	primary client.Resource,
) error {
	alternateSpace := r.contract.RunnerInput.AlternateSpace
	alternateURL := r.resourceURLWithExactQueryForSpace(
		primary.Metadata.Name,
		alternateSpace,
		"",
	)
	beforeCreate, err := r.request(http.MethodGet, alternateURL, nil, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(beforeCreate, "resource_not_found"); err != nil {
		return fmt.Errorf("Resource alternate-Space read isolation: %w", err)
	}
	r.complete("resource-cross-space-read-isolated")

	alternate := cloneRunnerResource(primary)
	alternate.Metadata.Space = alternateSpace
	alternate.Metadata.ResourceVersion = ""
	alternate.Status = nil
	alternate.Spec["storageClass"] = "archive"
	previewResponse, err := r.request(
		http.MethodPost,
		r.apiBase+"/resources/preview",
		map[string]string{"If-None-Match": "*"},
		alternate,
	)
	if err != nil {
		return err
	}
	if previewResponse.Status != http.StatusOK {
		return fmt.Errorf(
			"Resource alternate-Space preview returned HTTP %d: %s",
			previewResponse.Status,
			strings.TrimSpace(string(previewResponse.Body)),
		)
	}
	var preview client.PreviewResourceResult
	if err := decodeStrictBytes(previewResponse.Body, &preview); err != nil {
		return fmt.Errorf("Resource alternate-Space preview: %w", err)
	}
	specDigest, err := digestJSON(alternate.Spec)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(preview.Resource, alternate) ||
		preview.Review.SpecDigest != specDigest ||
		strings.TrimSpace(preview.Review.PlanDigest) == "" {
		return fmt.Errorf("Resource alternate-Space preview substituted the reviewed Resource: %#v", preview)
	}

	alternateCreate, err := r.request(
		http.MethodPut,
		r.resourceURL(),
		map[string]string{
			"If-None-Match": "*",
			// Reusing the primary Space's key is intentional: Space is one replay
			// namespace dimension, so this is an independent mutation.
			"Idempotency-Key": "portable-apply-create",
		},
		applyRequest{
			Resource: alternate,
			Review: client.DeploymentReview{
				PlanDigest: preview.Review.PlanDigest,
			},
		},
	)
	if err != nil {
		return err
	}
	alternateCreated, err := decodeResourceResponse(alternateCreate)
	if err != nil {
		return fmt.Errorf("Resource alternate-Space apply: %w", err)
	}
	if err := verifyRunnerResource(
		alternateCreated,
		alternate,
		"1",
		r.contract.RunnerInput.Definition,
	); err != nil {
		return fmt.Errorf("Resource alternate-Space apply: %w", err)
	}
	r.complete("idempotency-cross-space-isolated")
	r.idempotencyEvidence.IsolationDimensions = append(
		r.idempotencyEvidence.IsolationDimensions,
		"space",
	)

	exact := toClientIdentity(r.contract.RunnerInput.Identity)
	primaryRead, err := formClient.GetResource(
		r.ctx,
		primary.Kind,
		primary.Metadata.Name,
		primary.Metadata.Space,
		exact,
	)
	if err != nil {
		return fmt.Errorf("primary Resource after alternate-Space apply: %w", err)
	}
	if err := verifyRunnerResource(
		*primaryRead,
		primary,
		"1",
		r.contract.RunnerInput.Definition,
	); err != nil {
		return fmt.Errorf("alternate-Space apply mutated primary Resource: %w", err)
	}
	alternateRead, err := formClient.GetResource(
		r.ctx,
		alternate.Kind,
		alternate.Metadata.Name,
		alternate.Metadata.Space,
		exact,
	)
	if err != nil {
		return fmt.Errorf("Resource alternate-Space readback: %w", err)
	}
	if err := verifyRunnerResource(
		*alternateRead,
		alternate,
		"1",
		r.contract.RunnerInput.Definition,
	); err != nil {
		return fmt.Errorf("Resource alternate-Space readback: %w", err)
	}

	deleted, err := r.request(
		http.MethodDelete,
		alternateURL,
		map[string]string{
			"If-Match":        `"1"`,
			"Idempotency-Key": "portable-resource-alternate-space-delete",
		},
		nil,
	)
	if err != nil {
		return err
	}
	if deleted.Status != http.StatusNoContent || len(bytes.TrimSpace(deleted.Body)) != 0 {
		return fmt.Errorf(
			"Resource alternate-Space delete returned HTTP %d with %d bytes",
			deleted.Status,
			len(deleted.Body),
		)
	}
	afterDelete, err := r.request(http.MethodGet, alternateURL, nil, nil)
	if err != nil {
		return err
	}
	if err := expectStableError(afterDelete, "resource_not_found"); err != nil {
		return fmt.Errorf("Resource alternate-Space delete readback: %w", err)
	}
	primaryRead, err = formClient.GetResource(
		r.ctx,
		primary.Kind,
		primary.Metadata.Name,
		primary.Metadata.Space,
		exact,
	)
	if err != nil {
		return fmt.Errorf("primary Resource after alternate-Space cleanup: %w", err)
	}
	if err := verifyRunnerResource(
		*primaryRead,
		primary,
		"1",
		r.contract.RunnerInput.Definition,
	); err != nil {
		return fmt.Errorf("alternate-Space cleanup mutated primary Resource: %w", err)
	}
	r.complete("resource-cross-space-mutation-isolated")
	return nil
}

func (r *endpointRunner) verifyInterfaceSpaceRequired() error {
	descriptor, err := r.runnerInterfaceDescriptor()
	if err != nil {
		return err
	}
	type probe struct {
		label string
		space *string
	}
	empty := ""
	leadingWhitespace := " " + r.contract.RunnerInput.Space
	trailingWhitespace := r.contract.RunnerInput.Space + "\ufeff"
	slash := r.contract.RunnerInput.Space + "/child"
	control := r.contract.RunnerInput.Space + "\u0085child"
	tooLong := strings.Repeat("界", client.SpaceIDMaxLength+1)
	probes := []probe{
		{label: "missing"},
		{label: "empty", space: &empty},
		{label: "leading-whitespace", space: &leadingWhitespace},
		{label: "trailing-whitespace", space: &trailingWhitespace},
		{label: "slash", space: &slash},
		{label: "control", space: &control},
		{label: "too-long", space: &tooLong},
	}
	for _, current := range probes {
		query := url.Values{}
		if current.space != nil {
			query.Set("space", *current.space)
		}
		suffix := ""
		if current.space != nil {
			suffix = "?" + query.Encode()
		}
		for _, target := range []string{
			r.interfacesURL + suffix,
			r.interfacesURL + "/" + url.PathEscape(descriptor.Name) + suffix,
		} {
			response, err := r.request(http.MethodGet, target, nil, nil)
			if err != nil {
				return err
			}
			if err := expectStableError(response, "invalid_argument"); err != nil {
				return fmt.Errorf(
					"Interface Space requirement %s for %s: %w",
					current.label,
					target,
					err,
				)
			}
		}
	}
	return nil
}

func (r *endpointRunner) verifyInterfaceSpaceIsolation(
	formClient *client.Client,
	primary client.Resource,
	exact client.InstalledFormReference,
) error {
	alternateSpace := r.contract.RunnerInput.AlternateSpace
	alternateName := r.contract.RunnerInput.Name + "-space"
	if _, err := r.createReadyProbeResource(
		primary,
		alternateName,
		alternateSpace,
		"portable-interface-alternate-space-create",
	); err != nil {
		return fmt.Errorf("Interface alternate-Space Resource: %w", err)
	}

	primaryVisible, err := formClient.ListInterfaces(r.ctx, r.contract.RunnerInput.Space)
	if err != nil {
		return fmt.Errorf("Interface primary-Space list: %w", err)
	}
	if len(primaryVisible) != 1 ||
		primaryVisible[0].Resource.Name != r.contract.RunnerInput.Name {
		return fmt.Errorf("Interface Space isolation leaked or substituted in primary Space: %#v", primaryVisible)
	}
	alternateVisible, err := formClient.ListInterfaces(r.ctx, alternateSpace)
	if err != nil {
		return fmt.Errorf("Interface alternate-Space list: %w", err)
	}
	if len(alternateVisible) != 1 ||
		alternateVisible[0].Resource.Name != alternateName {
		return fmt.Errorf("Interface Space isolation leaked or substituted in alternate Space: %#v", alternateVisible)
	}

	crossPrimaryQuery := url.Values{}
	crossPrimaryQuery.Set("space", alternateSpace)
	crossPrimaryQuery.Set("version", primaryVisible[0].Version)
	crossPrimaryQuery.Set("resourceKind", exact.FormRef.Kind)
	crossPrimaryQuery.Set("resourceName", r.contract.RunnerInput.Name)
	crossPrimary, err := r.request(
		http.MethodGet,
		r.interfacesURL+"/"+url.PathEscape(primaryVisible[0].Name)+"?"+crossPrimaryQuery.Encode(),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if err := expectStableError(crossPrimary, "resource_not_found"); err != nil {
		return fmt.Errorf("Interface primary Resource leaked into alternate Space: %w", err)
	}

	crossAlternateQuery := url.Values{}
	crossAlternateQuery.Set("space", r.contract.RunnerInput.Space)
	crossAlternateQuery.Set("version", alternateVisible[0].Version)
	crossAlternateQuery.Set("resourceKind", exact.FormRef.Kind)
	crossAlternateQuery.Set("resourceName", alternateName)
	crossAlternate, err := r.request(
		http.MethodGet,
		r.interfacesURL+"/"+url.PathEscape(alternateVisible[0].Name)+"?"+crossAlternateQuery.Encode(),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if err := expectStableError(crossAlternate, "resource_not_found"); err != nil {
		return fmt.Errorf("Interface alternate Resource leaked into primary Space: %w", err)
	}

	if err := r.deleteReadyProbeResource(
		alternateName,
		alternateSpace,
		"portable-interface-alternate-space-delete",
	); err != nil {
		return err
	}
	alternateVisible, err = formClient.ListInterfaces(r.ctx, alternateSpace)
	if err != nil {
		return fmt.Errorf("Interface alternate-Space cleanup list: %w", err)
	}
	if len(alternateVisible) != 0 {
		return fmt.Errorf("Interface alternate-Space cleanup left projections: %#v", alternateVisible)
	}
	return nil
}

func (r *endpointRunner) verifyMultiResourceInstanceAmbiguity(
	formClient *client.Client,
	primary client.Resource,
	exact client.InstalledFormReference,
) error {
	descriptor, err := r.runnerInterfaceDescriptor()
	if err != nil {
		return err
	}
	secondaryName := r.contract.RunnerInput.Name + "-ambiguity"
	if _, err := r.createReadyProbeResource(
		primary,
		secondaryName,
		r.contract.RunnerInput.Space,
		"portable-interface-secondary-create",
	); err != nil {
		return fmt.Errorf("secondary Interface Resource: %w", err)
	}

	listed, err := formClient.ListInterfaces(r.ctx, r.contract.RunnerInput.Space)
	if err != nil {
		return fmt.Errorf("Interface multi-Resource list: %w", err)
	}
	if len(listed) != 2 {
		return fmt.Errorf("Interface multi-Resource list has %d declarations, want 2", len(listed))
	}
	query := url.Values{}
	query.Set("space", r.contract.RunnerInput.Space)
	query.Set("version", descriptor.Version)
	ambiguous, err := r.request(
		http.MethodGet,
		r.interfacesURL+"/"+url.PathEscape(descriptor.Name)+"?"+query.Encode(),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if err := expectStableError(ambiguous, "interface_instance_ambiguous"); err != nil {
		return fmt.Errorf("Interface multi-Resource ambiguity: %w", err)
	}
	exactPrimary, err := formClient.GetInterface(r.ctx, r.contract.RunnerInput.Space, client.InterfaceSelector{
		Name: descriptor.Name, Version: descriptor.Version,
		ResourceKind: exact.FormRef.Kind, ResourceName: r.contract.RunnerInput.Name,
	})
	if err != nil {
		return fmt.Errorf("Interface exact Resource read while ambiguous: %w", err)
	}
	if exactPrimary.Resource.Name != r.contract.RunnerInput.Name {
		return errors.New("Interface exact Resource read selected another instance")
	}

	if err := r.deleteReadyProbeResource(
		secondaryName,
		r.contract.RunnerInput.Space,
		"portable-interface-secondary-delete",
	); err != nil {
		return err
	}
	listed, err = formClient.ListInterfaces(r.ctx, r.contract.RunnerInput.Space)
	if err != nil {
		return fmt.Errorf("Interface list after secondary cleanup: %w", err)
	}
	if len(listed) != 1 || listed[0].Resource.Name != r.contract.RunnerInput.Name {
		return fmt.Errorf("Interface secondary cleanup left unexpected declarations: %#v", listed)
	}
	return nil
}

func (r *endpointRunner) createReadyProbeResource(
	base client.Resource,
	name,
	space,
	idempotencyKey string,
) (client.Resource, error) {
	resource := cloneRunnerResource(base)
	resource.Metadata.Name = name
	resource.Metadata.Space = space
	resource.Metadata.ResourceVersion = ""
	resource.Spec["name"] = name
	resource.Status = nil
	previewResponse, err := r.request(
		http.MethodPost,
		r.apiBase+"/resources/preview",
		map[string]string{"If-None-Match": "*"},
		resource,
	)
	if err != nil {
		return client.Resource{}, err
	}
	if previewResponse.Status != http.StatusOK {
		return client.Resource{}, fmt.Errorf(
			"probe Resource preview returned HTTP %d: %s",
			previewResponse.Status,
			strings.TrimSpace(string(previewResponse.Body)),
		)
	}
	var preview client.PreviewResourceResult
	if err := decodeStrictBytes(previewResponse.Body, &preview); err != nil {
		return client.Resource{}, fmt.Errorf("probe Resource preview: %w", err)
	}
	specDigest, err := digestJSON(resource.Spec)
	if err != nil {
		return client.Resource{}, err
	}
	if !reflect.DeepEqual(preview.Resource, resource) ||
		preview.Review.SpecDigest != specDigest ||
		strings.TrimSpace(preview.Review.PlanDigest) == "" {
		return client.Resource{}, fmt.Errorf("probe Resource preview substituted the request: %#v", preview)
	}
	created, err := r.request(
		http.MethodPut,
		r.resourceURLForName(name),
		map[string]string{
			"If-None-Match":   "*",
			"Idempotency-Key": idempotencyKey,
		},
		applyRequest{
			Resource: resource,
			Review:   client.DeploymentReview{PlanDigest: preview.Review.PlanDigest},
		},
	)
	if err != nil {
		return client.Resource{}, err
	}
	createdResource, err := decodeResourceResponse(created)
	if err != nil {
		return client.Resource{}, fmt.Errorf("probe Resource apply: %w", err)
	}
	if err := verifyRunnerResource(
		createdResource,
		resource,
		"1",
		r.contract.RunnerInput.Definition,
	); err != nil {
		return client.Resource{}, fmt.Errorf("probe Resource apply: %w", err)
	}
	return createdResource, nil
}

func (r *endpointRunner) deleteReadyProbeResource(name, space, idempotencyKey string) error {
	deleted, err := r.request(
		http.MethodDelete,
		r.resourceURLWithExactQueryForSpace(name, space, ""),
		map[string]string{
			"If-Match":        `"1"`,
			"Idempotency-Key": idempotencyKey,
		},
		nil,
	)
	if err != nil {
		return err
	}
	if deleted.Status != http.StatusNoContent || len(bytes.TrimSpace(deleted.Body)) != 0 {
		return fmt.Errorf(
			"probe Resource delete returned HTTP %d with %d bytes",
			deleted.Status,
			len(deleted.Body),
		)
	}
	return nil
}

func (r *endpointRunner) runnerInterfaceDescriptor() (formpackage.InterfaceDescriptor, error) {
	descriptors := r.contract.RunnerInput.Definition.Interfaces
	if len(descriptors) != 1 || !descriptors[0].Required {
		return formpackage.InterfaceDescriptor{}, errors.New(
			"portable host runner requires one exact required Interface descriptor",
		)
	}
	return descriptors[0], nil
}

func expectedRunnerInterface(
	descriptor formpackage.InterfaceDescriptor,
	resource client.Resource,
	exact client.InstalledFormReference,
) (client.DeclaredInterface, error) {
	if resource.Status == nil || resource.Status.Output == nil {
		return client.DeclaredInterface{}, errors.New("Interface-exposing Resource has no output document")
	}
	values := make(map[string]any, len(descriptor.Inputs))
	for _, input := range descriptor.Inputs {
		switch input.Source {
		case formpackage.InterfaceInputSourceOutput:
			value, ok := resolveRunnerJSONPointer(resource.Status.Output, input.Pointer)
			if !ok {
				return client.DeclaredInterface{}, fmt.Errorf(
					"Interface input %q does not resolve output pointer %q",
					input.Name,
					input.Pointer,
				)
			}
			values[input.Name] = value
		case formpackage.InterfaceInputSourceLiteral:
			var value any
			if len(input.Value) == 0 || json.Unmarshal(input.Value, &value) != nil {
				return client.DeclaredInterface{}, fmt.Errorf("Interface input %q has an invalid literal", input.Name)
			}
			values[input.Name] = value
		default:
			return client.DeclaredInterface{}, fmt.Errorf(
				"runner cannot derive Interface input %q from source %q",
				input.Name,
				input.Source,
			)
		}
	}
	return client.DeclaredInterface{
		Name: descriptor.Name, Version: descriptor.Version,
		Resource: client.InterfaceResourceRef{Kind: resource.Kind, Name: resource.Metadata.Name},
		Document: cloneMap(descriptor.Document), Values: values, Form: &exact,
	}, nil
}

func sameRunnerInterface(got, want client.DeclaredInterface) bool {
	return got.Name == want.Name &&
		got.Version == want.Version &&
		got.Resource == want.Resource &&
		reflect.DeepEqual(got.Document, want.Document) &&
		reflect.DeepEqual(got.Values, want.Values) &&
		got.ResourceURI == want.ResourceURI &&
		(got.Form == nil || (want.Form != nil && reflect.DeepEqual(*got.Form, *want.Form)))
}

func resolveRunnerJSONPointer(document any, pointer string) (any, bool) {
	if pointer == "" {
		return document, true
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	current := document
	for _, raw := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		token := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		switch typed := current.(type) {
		case map[string]any:
			value, ok := typed[token]
			if !ok {
				return nil, false
			}
			current = value
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func (r *endpointRunner) readInterface(name string, query url.Values) (client.DeclaredInterface, error) {
	response, err := r.request(
		http.MethodGet,
		r.interfacesURL+"/"+url.PathEscape(name)+"?"+query.Encode(),
		nil,
		nil,
	)
	if err != nil {
		return client.DeclaredInterface{}, err
	}
	if response.Status != http.StatusOK {
		return client.DeclaredInterface{}, fmt.Errorf(
			"Interface read returned HTTP %d: %s",
			response.Status,
			strings.TrimSpace(string(response.Body)),
		)
	}
	var declared client.DeclaredInterface
	if err := decodeStrictBytes(response.Body, &declared); err != nil {
		return client.DeclaredInterface{}, err
	}
	return declared, nil
}

func (r *endpointRunner) complete(check string) {
	r.completed[check] = true
}

func (r *endpointRunner) expectNoInterfaces(formClient *client.Client, stage string) error {
	listed, err := formClient.ListInterfaces(r.ctx, r.contract.RunnerInput.Space)
	if err != nil {
		return fmt.Errorf("Interface list %s: %w", stage, err)
	}
	if len(listed) != 0 {
		return fmt.Errorf("Interface readiness: %d declarations visible %s", len(listed), stage)
	}
	return nil
}

func (r *endpointRunner) probeStableErrors(resource client.Resource) error {
	for _, code := range r.contract.StableErrorCodes {
		response, err := r.request(http.MethodPost, r.apiBase+"/resources/preview", map[string]string{
			"If-None-Match": "*", ErrorProbeHeader: code,
		}, resource)
		if err != nil {
			return err
		}
		if err := expectStableError(response, code); err != nil {
			return fmt.Errorf("stable error probe %s: %w", code, err)
		}
		status, retryable, _ := stableErrorSemantics(code)
		r.errorProbes = append(r.errorProbes, ErrorProbeEvidence{
			Code: code, HTTPStatus: status, Retryable: retryable,
		})
	}
	return nil
}

func (r *endpointRunner) verifyRawJSONBoundary(base client.Resource) error {
	type rawRequestProbe struct {
		name   string
		check  string
		mutate func([]byte, client.Resource) ([]byte, error)
	}
	for _, probe := range []rawRequestProbe{
		{
			name:  "raw-unknown-top-level",
			check: "unknown-top-level-request-field-rejected-before-mutation",
			mutate: func(raw []byte, _ client.Resource) ([]byte, error) {
				return appendObjectMember(
					raw,
					"managerAuthority",
					map[string]any{"backend": "attacker-controlled"},
				)
			},
		},
		{
			name:  "raw-unknown-metadata",
			check: "unknown-metadata-field-rejected-before-mutation",
			mutate: func(raw []byte, _ client.Resource) ([]byte, error) {
				return prependObjectMember(
					raw,
					`"metadata":{`,
					"managerAuthority",
					"attacker-controlled",
				)
			},
		},
		{
			name:  "raw-unknown-desired-authority",
			check: "unknown-desired-authority-field-rejected-before-mutation",
			mutate: func(raw []byte, _ client.Resource) ([]byte, error) {
				return prependObjectMember(
					raw,
					`"spec":{`,
					"managerAuthority",
					map[string]any{"backend": "attacker-controlled"},
				)
			},
		},
		{
			name:  "raw-duplicate-space",
			check: "duplicate-metadata-space-rejected-before-typed-decode",
			mutate: func(raw []byte, resource client.Resource) ([]byte, error) {
				return prependObjectMember(raw, `"metadata":{`, "space", resource.Metadata.Space)
			},
		},
		{
			name:  "raw-duplicate-spec",
			check: "duplicate-spec-rejected-before-typed-decode",
			mutate: func(raw []byte, resource client.Resource) ([]byte, error) {
				return appendObjectMember(raw, "spec", resource.Spec)
			},
		},
		{
			name:  "raw-non-utf8",
			check: "non-utf8-request-rejected-before-typed-decode",
			mutate: func(raw []byte, _ client.Resource) ([]byte, error) {
				if len(raw) == 0 || raw[len(raw)-1] != '}' {
					return nil, errors.New("raw apply document is not an object")
				}
				mutated := append([]byte(nil), raw[:len(raw)-1]...)
				mutated = append(mutated, []byte(`,"invalidUtf8":"`)...)
				mutated = append(mutated, 0xff)
				mutated = append(mutated, []byte(`"}`)...)
				return mutated, nil
			},
		},
	} {
		resource := rawProbeResource(base, probe.name)
		if err := r.verifyRawApplyRejectedBeforeMutation(resource, probe.check, probe.mutate); err != nil {
			return err
		}
	}
	if err := r.verifyDuplicateResourceVersionRejected(base); err != nil {
		return err
	}
	if err := r.verifyDuplicateErrorCodeResponseRejected(base); err != nil {
		return err
	}
	return nil
}

func (r *endpointRunner) verifyRawApplyRejectedBeforeMutation(
	resource client.Resource,
	check string,
	mutate func([]byte, client.Resource) ([]byte, error),
) error {
	if err := r.expectResourceAbsent(resource.Metadata.Name); err != nil {
		return fmt.Errorf("%s precondition: %w", check, err)
	}
	raw, err := r.rawCreateApply(resource)
	if err != nil {
		return fmt.Errorf("%s prepare: %w", check, err)
	}
	raw, err = mutate(raw, resource)
	if err != nil {
		return fmt.Errorf("%s encode: %w", check, err)
	}
	response, err := r.requestRaw(
		http.MethodPut,
		r.resourceURLForName(resource.Metadata.Name),
		map[string]string{
			"If-None-Match":   "*",
			"Idempotency-Key": "portable-" + check,
		},
		raw,
	)
	if err != nil {
		return err
	}
	if err := expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("%s: %w", check, err)
	}
	if err := r.expectResourceAbsent(resource.Metadata.Name); err != nil {
		return fmt.Errorf("%s mutation check: %w", check, err)
	}
	r.complete(check)
	return nil
}

func (r *endpointRunner) rawCreateApply(resource client.Resource) ([]byte, error) {
	previewResponse, err := r.request(
		http.MethodPost,
		r.apiBase+"/resources/preview",
		map[string]string{"If-None-Match": "*"},
		resource,
	)
	if err != nil {
		return nil, err
	}
	if previewResponse.Status != http.StatusOK {
		return nil, fmt.Errorf(
			"clean preview returned HTTP %d: %s",
			previewResponse.Status,
			strings.TrimSpace(string(previewResponse.Body)),
		)
	}
	var preview client.PreviewResourceResult
	if err := decodeStrictBytes(previewResponse.Body, &preview); err != nil {
		return nil, err
	}
	return json.Marshal(applyRequest{
		Resource: resource,
		Review:   client.DeploymentReview{PlanDigest: preview.Review.PlanDigest},
	})
}

func (r *endpointRunner) verifyDuplicateResourceVersionRejected(base client.Resource) error {
	const check = "duplicate-resource-version-rejected-before-typed-decode"
	resource := rawProbeResource(base, "raw-duplicate-resource-version")
	if err := r.expectResourceAbsent(resource.Metadata.Name); err != nil {
		return fmt.Errorf("%s precondition: %w", check, err)
	}
	createRaw, err := r.rawCreateApply(resource)
	if err != nil {
		return fmt.Errorf("%s create prepare: %w", check, err)
	}
	createdResponse, err := r.requestRaw(
		http.MethodPut,
		r.resourceURLForName(resource.Metadata.Name),
		map[string]string{
			"If-None-Match":   "*",
			"Idempotency-Key": "portable-duplicate-resource-version-create",
		},
		createRaw,
	)
	if err != nil {
		return err
	}
	created, err := decodeResourceResponse(createdResponse)
	if err != nil {
		return fmt.Errorf("%s clean create: %w", check, err)
	}

	update := cloneRunnerResource(resource)
	update.Metadata.ResourceVersion = created.Metadata.ResourceVersion
	update.Spec["storageClass"] = "archive"
	previewResponse, err := r.request(
		http.MethodPost,
		r.apiBase+"/resources/preview",
		map[string]string{"If-Match": `"` + created.Metadata.ResourceVersion + `"`},
		update,
	)
	if err != nil {
		return err
	}
	if previewResponse.Status != http.StatusOK {
		return fmt.Errorf(
			"%s update preview returned HTTP %d: %s",
			check,
			previewResponse.Status,
			strings.TrimSpace(string(previewResponse.Body)),
		)
	}
	var preview client.PreviewResourceResult
	if err := decodeStrictBytes(previewResponse.Body, &preview); err != nil {
		return fmt.Errorf("%s update preview: %w", check, err)
	}
	updateRaw, err := json.Marshal(applyRequest{
		Resource: update,
		Review:   client.DeploymentReview{PlanDigest: preview.Review.PlanDigest},
	})
	if err != nil {
		return err
	}
	updateRaw, err = prependObjectMember(
		updateRaw,
		`"metadata":{`,
		"resourceVersion",
		created.Metadata.ResourceVersion,
	)
	if err != nil {
		return err
	}
	rejected, err := r.requestRaw(
		http.MethodPut,
		r.resourceURLForName(resource.Metadata.Name),
		map[string]string{
			"If-Match":        `"` + created.Metadata.ResourceVersion + `"`,
			"Idempotency-Key": "portable-duplicate-resource-version-update",
		},
		updateRaw,
	)
	if err != nil {
		return err
	}
	if err := expectStableError(rejected, "invalid_argument"); err != nil {
		return fmt.Errorf("%s: %w", check, err)
	}
	readbackResponse, err := r.request(
		http.MethodGet,
		r.resourceURLWithExactQueryForName(resource.Metadata.Name, ""),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	readback, err := decodeResourceResponse(readbackResponse)
	if err != nil {
		return fmt.Errorf("%s readback: %w", check, err)
	}
	if readback.Metadata.ResourceVersion != created.Metadata.ResourceVersion ||
		!sameCanonicalRunnerJSON(readback.Spec, resource.Spec) {
		return errors.New("duplicate resourceVersion request mutated Resource state")
	}
	deleted, err := r.request(
		http.MethodDelete,
		r.resourceURLWithExactQueryForName(resource.Metadata.Name, ""),
		map[string]string{
			"If-Match":        `"` + created.Metadata.ResourceVersion + `"`,
			"Idempotency-Key": "portable-duplicate-resource-version-delete",
		},
		nil,
	)
	if err != nil {
		return err
	}
	if deleted.Status != http.StatusNoContent {
		return fmt.Errorf("%s cleanup returned HTTP %d", check, deleted.Status)
	}
	if err := r.expectResourceAbsent(resource.Metadata.Name); err != nil {
		return fmt.Errorf("%s cleanup: %w", check, err)
	}
	r.complete(check)
	return nil
}

func (r *endpointRunner) verifyDuplicateErrorCodeResponseRejected(base client.Resource) error {
	const check = "duplicate-error-code-response-rejected-before-typed-decode"
	response, err := r.request(
		http.MethodPost,
		r.apiBase+"/resources/preview",
		map[string]string{
			"If-None-Match":    "*",
			RawJSONProbeHeader: RawJSONProbeDuplicateErrorCode,
		},
		rawProbeResource(base, "raw-duplicate-error-code"),
	)
	if err != nil {
		return err
	}
	if response.Status != http.StatusBadRequest {
		return fmt.Errorf("%s returned HTTP %d, want 400", check, response.Status)
	}
	var envelope map[string]any
	decodeErr := decodeStrictBytes(response.Body, &envelope)
	if decodeErr == nil || !strings.Contains(strings.ToLower(decodeErr.Error()), "duplicate") {
		return fmt.Errorf("%s accepted malformed response or failed for the wrong reason: %v", check, decodeErr)
	}
	r.complete(check)
	return nil
}

func (r *endpointRunner) expectResourceAbsent(name string) error {
	response, err := r.request(
		http.MethodGet,
		r.resourceURLWithExactQueryForName(name, ""),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	return expectStableError(response, "resource_not_found")
}

func rawProbeResource(base client.Resource, suffix string) client.Resource {
	resource := cloneRunnerResource(base)
	resource.Metadata.Name = suffix
	resource.Metadata.ResourceVersion = ""
	resource.Status = nil
	resource.Spec["name"] = suffix
	return resource
}

func prependObjectMember(raw []byte, marker, name string, value any) ([]byte, error) {
	encodedName, err := json.Marshal(name)
	if err != nil {
		return nil, err
	}
	encodedValue, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	index := bytes.Index(raw, []byte(marker))
	if index < 0 {
		return nil, fmt.Errorf("JSON object marker %q is absent", marker)
	}
	index += len(marker)
	insertion := append(encodedName, ':')
	insertion = append(insertion, encodedValue...)
	insertion = append(insertion, ',')
	mutated := make([]byte, 0, len(raw)+len(insertion))
	mutated = append(mutated, raw[:index]...)
	mutated = append(mutated, insertion...)
	mutated = append(mutated, raw[index:]...)
	return mutated, nil
}

func appendObjectMember(raw []byte, name string, value any) ([]byte, error) {
	if len(raw) == 0 || raw[len(raw)-1] != '}' {
		return nil, errors.New("JSON document is not an object")
	}
	encodedName, err := json.Marshal(name)
	if err != nil {
		return nil, err
	}
	encodedValue, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	mutated := append([]byte(nil), raw[:len(raw)-1]...)
	mutated = append(mutated, ',')
	mutated = append(mutated, encodedName...)
	mutated = append(mutated, ':')
	mutated = append(mutated, encodedValue...)
	mutated = append(mutated, '}')
	return mutated, nil
}

func (r *endpointRunner) request(method, target string, headers map[string]string, body any) (wireResponse, error) {
	return r.requestWithToken(method, target, headers, body, r.token)
}

func (r *endpointRunner) expectPurePlanSubstitutionRejected(
	name string,
	idempotencyKey string,
	resource client.Resource,
	planDigest string,
) error {
	response, err := r.request(
		http.MethodPut,
		r.resourceURLForName(resource.Metadata.Name),
		map[string]string{
			"If-None-Match":   "*",
			"Idempotency-Key": idempotencyKey,
		},
		applyRequest{
			Resource: resource,
			Review:   client.DeploymentReview{PlanDigest: planDigest},
		},
	)
	if err != nil {
		return err
	}
	if err := expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("%s substitution: %w", name, err)
	}
	return nil
}

func (r *endpointRunner) expectPureUpdatePlanSubstitutionRejected(
	name string,
	resource client.Resource,
	planDigest string,
) error {
	response, err := r.request(
		http.MethodPut,
		r.resourceURLForName(resource.Metadata.Name),
		map[string]string{
			"If-Match":        `"` + resource.Metadata.ResourceVersion + `"`,
			"Idempotency-Key": "portable-plan-substitution-" + name,
		},
		applyRequest{
			Resource: resource,
			Review:   client.DeploymentReview{PlanDigest: planDigest},
		},
	)
	if err != nil {
		return err
	}
	if err := expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("%s substitution: %w", name, err)
	}
	return nil
}

func (r *endpointRunner) expectInstrumentedPlanSubstitutionRejected(
	name,
	input string,
	resource client.Resource,
	planDigest string,
) error {
	response, err := r.request(
		http.MethodPut,
		r.resourceURLForName(resource.Metadata.Name),
		map[string]string{
			"If-None-Match":        "*",
			"Idempotency-Key":      "portable-plan-substitution-" + name,
			PlanBindingProbeHeader: input,
		},
		applyRequest{
			Resource: resource,
			Review:   client.DeploymentReview{PlanDigest: planDigest},
		},
	)
	if err != nil {
		return err
	}
	if err := expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("%s instrumented substitution: %w", name, err)
	}
	if got := response.Header.Get(PlanBindingProbeResultHeader); got != PlanBindingProbeResultRejected {
		return fmt.Errorf(
			"%s instrumented substitution did not prove the plan-binding seam: %s=%q",
			name,
			PlanBindingProbeResultHeader,
			got,
		)
	}
	return nil
}

func (r *endpointRunner) requestWithToken(
	method,
	target string,
	headers map[string]string,
	body any,
	token string,
) (wireResponse, error) {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return wireResponse{}, err
		}
	}
	return r.requestRawWithToken(method, target, headers, raw, body != nil, token)
}

func (r *endpointRunner) requestRaw(
	method,
	target string,
	headers map[string]string,
	raw []byte,
) (wireResponse, error) {
	return r.requestRawWithToken(method, target, headers, raw, true, r.token)
}

func (r *endpointRunner) requestRawWithToken(
	method,
	target string,
	headers map[string]string,
	raw []byte,
	hasBody bool,
	token string,
) (wireResponse, error) {
	request, err := http.NewRequestWithContext(r.ctx, method, target, bytes.NewReader(raw))
	if err != nil {
		return wireResponse{}, err
	}
	request.Header.Set("Accept", "application/json")
	if hasBody {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := r.httpClient.Do(request)
	if err != nil {
		return wireResponse{}, fmt.Errorf("%s %s: %w", method, target, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil {
		return wireResponse{}, err
	}
	if len(data) > 8*1024*1024 {
		return wireResponse{}, errors.New("portable host response exceeded 8 MiB")
	}
	return wireResponse{Status: response.StatusCode, Header: response.Header.Clone(), Body: data}, nil
}

func cloneStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func (r *endpointRunner) exactQuery(packageDigestOverride string) url.Values {
	return r.exactQueryForSpace(r.contract.RunnerInput.Space, packageDigestOverride)
}

func (r *endpointRunner) exactQueryForSpace(space, packageDigestOverride string) url.Values {
	return r.exactQueryForIdentity(space, r.contract.RunnerInput.Identity, packageDigestOverride)
}

func (r *endpointRunner) exactQueryForIdentity(
	space string,
	identity InstalledFormReference,
	packageDigestOverride string,
) url.Values {
	packageDigest := identity.PackageDigest
	if packageDigestOverride != "" {
		packageDigest = packageDigestOverride
	}
	query := url.Values{}
	query.Set("space", space)
	query.Set("apiVersion", identity.FormRef.APIVersion)
	query.Set("kind", identity.FormRef.Kind)
	query.Set("definitionVersion", identity.FormRef.DefinitionVersion)
	query.Set("schemaDigest", identity.FormRef.SchemaDigest)
	query.Set("packageDigest", packageDigest)
	return query
}

func (r *endpointRunner) resourceURL() string {
	return r.resourceURLForName(r.contract.RunnerInput.Name)
}

func (r *endpointRunner) resourceURLForName(name string) string {
	return r.resourceURLForIdentity(r.contract.RunnerInput.Identity.FormRef.Kind, name)
}

func (r *endpointRunner) resourceURLForIdentity(kind, name string) string {
	return fmt.Sprintf("%s/resources/%s/%s", r.apiBase,
		url.PathEscape(kind),
		url.PathEscape(name))
}

func (r *endpointRunner) resourceURLWithExactQuery(packageDigestOverride string) string {
	return r.resourceURLWithExactQueryForName(r.contract.RunnerInput.Name, packageDigestOverride)
}

func (r *endpointRunner) resourceURLWithExactQueryForName(name, packageDigestOverride string) string {
	return r.resourceURLForName(name) + "?" + r.exactQuery(packageDigestOverride).Encode()
}

func (r *endpointRunner) resourceURLWithExactQueryForSpace(
	name,
	space,
	packageDigestOverride string,
) string {
	return r.resourceURLForName(name) + "?" + r.exactQueryForSpace(space, packageDigestOverride).Encode()
}

func (r *endpointRunner) actionURL(action string) string {
	return r.resourceURL() + "/" + action + "?" + r.exactQuery("").Encode()
}

func toClientIdentity(identity InstalledFormReference) client.InstalledFormReference {
	return client.InstalledFormReference{
		FormRef: client.FormRef{
			APIVersion: identity.FormRef.APIVersion, Kind: identity.FormRef.Kind,
			DefinitionVersion: identity.FormRef.DefinitionVersion, SchemaDigest: identity.FormRef.SchemaDigest,
		},
		PackageDigest: identity.PackageDigest,
	}
}

func pointerToRunnerIdentity(identity client.InstalledFormReference) *client.InstalledFormReference {
	return &identity
}

func cloneMap(input map[string]any) map[string]any {
	raw, _ := json.Marshal(input)
	var output map[string]any
	_ = json.Unmarshal(raw, &output)
	return output
}

func cloneRunnerResource(input client.Resource) client.Resource {
	raw, _ := json.Marshal(input)
	var output client.Resource
	_ = json.Unmarshal(raw, &output)
	return output
}

func sameCanonicalRunnerJSON(left, right any) bool {
	leftDigest, leftErr := digestJSON(left)
	rightDigest, rightErr := digestJSON(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func decodeResourceResponse(response wireResponse) (client.Resource, error) {
	if response.Status != http.StatusOK && response.Status != http.StatusCreated {
		return client.Resource{}, fmt.Errorf("HTTP %d: %s", response.Status, strings.TrimSpace(string(response.Body)))
	}
	var resource client.Resource
	if err := decodeStrictBytes(response.Body, &resource); err != nil {
		return client.Resource{}, err
	}
	if err := verifyETag(response, resource.Metadata.ResourceVersion); err != nil {
		return client.Resource{}, err
	}
	return resource, nil
}

func decodeResourceEnvelope(response wireResponse, successStatuses ...int) (client.Resource, error) {
	if !containsStatus(successStatuses, response.Status) {
		return client.Resource{}, fmt.Errorf("HTTP %d: %s", response.Status, strings.TrimSpace(string(response.Body)))
	}
	var envelope resourceEnvelope
	if err := decodeStrictBytes(response.Body, &envelope); err != nil {
		return client.Resource{}, err
	}
	if err := verifyETag(response, envelope.Resource.Metadata.ResourceVersion); err != nil {
		return client.Resource{}, err
	}
	return envelope.Resource, nil
}

func containsStatus(statuses []int, want int) bool {
	for _, status := range statuses {
		if status == want {
			return true
		}
	}
	return false
}

func verifyRunnerResource(actual, requested client.Resource, version string, definition formpackage.FormDefinition) error {
	if actual.APIVersion != requested.APIVersion || actual.Kind != requested.Kind ||
		actual.Form == nil || requested.Form == nil || !reflect.DeepEqual(*actual.Form, *requested.Form) ||
		actual.Metadata.Name != requested.Metadata.Name || actual.Metadata.Space != requested.Metadata.Space ||
		actual.Metadata.ResourceVersion != version || !reflect.DeepEqual(actual.Spec, requested.Spec) ||
		actual.Status == nil || actual.Status.Observed == nil || actual.Status.Output == nil {
		return fmt.Errorf("Resource identity/spec/status mismatch: %#v", actual)
	}
	if err := validateRunnerSchema(definition.DesiredSchema, actual.Spec, "desired"); err != nil {
		return err
	}
	if err := validateRunnerSchema(definition.ObservedSchema, actual.Status.Observed, "observed"); err != nil {
		return err
	}
	if len(definition.OutputSchema) == 0 {
		if actual.Status.Output != nil {
			return errors.New("Resource returned output for a Form without outputSchema")
		}
	} else if err := validateRunnerSchema(definition.OutputSchema, actual.Status.Output, "output"); err != nil {
		return err
	}
	generation, err := strconv.Atoi(version)
	if err != nil {
		return err
	}
	wantID := actual.Kind + "/" + actual.Metadata.Name
	if actual.Status.Observed["id"] != wantID || !exactGeneration(actual.Status.Observed["generation"], generation) ||
		actual.Status.Observed["ready"] != true ||
		actual.Status.Output["id"] != wantID || actual.Status.Output["kind"] != actual.Kind ||
		actual.Status.Output["name"] != actual.Metadata.Name ||
		!exactGeneration(actual.Status.Output["generation"], generation) {
		return fmt.Errorf("Resource observed/output identity mismatch: %#v", actual.Status)
	}
	observedPortability, observedHasPortability := actual.Status.Observed["portability"]
	outputPortability, outputHasPortability := actual.Status.Output["portability"]
	if observedHasPortability != outputHasPortability ||
		(observedHasPortability && !reflect.DeepEqual(observedPortability, outputPortability)) {
		return errors.New("Resource observed/output portability values differ")
	}
	return nil
}

func validateRunnerSchema(schema map[string]any, value any, label string) error {
	if len(schema) == 0 {
		return fmt.Errorf("exact Form has no %s schema", label)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	id := "https://forms.takoform.com/portable-host-runner/" + label + ".schema.json"
	if err := compiler.AddResource(id, schema); err != nil {
		return fmt.Errorf("compile exact %s schema: %w", label, err)
	}
	compiled, err := compiler.Compile(id)
	if err != nil {
		return fmt.Errorf("compile exact %s schema: %w", label, err)
	}
	if err := compiled.Validate(value); err != nil {
		return fmt.Errorf("Resource %s document violates the exact Form schema: %w", label, err)
	}
	return nil
}

func exactGeneration(value any, want int) bool {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		return err == nil && parsed == int64(want)
	case int:
		return typed == want
	case int64:
		return typed == int64(want)
	case float64:
		return typed == float64(want)
	default:
		return false
	}
}

func verifyETag(response wireResponse, version string) error {
	values := response.Header.Values("ETag")
	if len(values) != 1 || values[0] != `"`+version+`"` {
		return fmt.Errorf("ETag = %v, want exactly %q", values, `"`+version+`"`)
	}
	return nil
}

func sameWireResponse(left, right wireResponse) bool {
	return left.Status == right.Status &&
		reflect.DeepEqual(left.Header.Values("ETag"), right.Header.Values("ETag")) &&
		bytes.Equal(left.Body, right.Body)
}

func expectStableError(response wireResponse, code string) error {
	status, retryable, ok := stableErrorSemantics(code)
	if !ok {
		return fmt.Errorf("unknown stable error code %q", code)
	}
	if response.Status != status {
		return fmt.Errorf("HTTP %d, want %d; body=%s", response.Status, status, strings.TrimSpace(string(response.Body)))
	}
	var envelope struct {
		Error struct {
			Code      string          `json:"code"`
			Message   string          `json:"message"`
			RequestID string          `json:"requestId"`
			Retryable *bool           `json:"retryable"`
			HostCode  *string         `json:"hostCode,omitempty"`
			Details   json.RawMessage `json:"details,omitempty"`
		} `json:"error"`
	}
	if err := decodeStrictBytes(response.Body, &envelope); err != nil {
		return err
	}
	if envelope.Error.Code != code || strings.TrimSpace(envelope.Error.Message) == "" ||
		strings.TrimSpace(envelope.Error.RequestID) == "" || envelope.Error.Retryable == nil ||
		*envelope.Error.Retryable != retryable {
		return fmt.Errorf("invalid %s error envelope: %#v", code, envelope.Error)
	}
	if envelope.Error.HostCode != nil && strings.TrimSpace(*envelope.Error.HostCode) == "" {
		return fmt.Errorf("invalid %s error envelope: hostCode is empty", code)
	}
	return nil
}

func expectAnyStableError(response wireResponse) error {
	var envelope struct {
		Error struct {
			Code      string          `json:"code"`
			Message   string          `json:"message"`
			RequestID string          `json:"requestId"`
			Retryable *bool           `json:"retryable"`
			HostCode  *string         `json:"hostCode,omitempty"`
			Details   json.RawMessage `json:"details,omitempty"`
		} `json:"error"`
	}
	if err := decodeStrictBytes(response.Body, &envelope); err != nil {
		return err
	}
	if envelope.Error.Code == "" {
		return errors.New("error envelope omitted code")
	}
	return expectStableError(response, envelope.Error.Code)
}

func stableErrorSemantics(code string) (status int, retryable bool, ok bool) {
	statuses := map[string]int{
		"invalid_argument":             http.StatusBadRequest,
		"unauthenticated":              http.StatusUnauthorized,
		"permission_denied":            http.StatusForbidden,
		"form_unknown":                 http.StatusNotFound,
		"form_not_installed":           http.StatusConflict,
		"form_unavailable":             http.StatusServiceUnavailable,
		"form_identity_conflict":       http.StatusConflict,
		"resource_not_found":           http.StatusNotFound,
		"resource_version_conflict":    http.StatusPreconditionFailed,
		"resource_busy":                http.StatusConflict,
		"import_conflict":              http.StatusConflict,
		"policy_denied":                http.StatusForbidden,
		"backend_unavailable":          http.StatusServiceUnavailable,
		"interface_identity_ambiguous": http.StatusConflict,
		"interface_instance_ambiguous": http.StatusConflict,
		"internal_error":               http.StatusInternalServerError,
	}
	status, ok = statuses[code]
	return status, code == "resource_busy" || code == "backend_unavailable", ok
}

func decodeStrictBytes(raw []byte, target any) error {
	return formpackage.DecodeStrictIJSON(raw, target)
}

func digestJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	canonical, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		return "", err
	}
	return canonical, nil
}

func digestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func containsValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
