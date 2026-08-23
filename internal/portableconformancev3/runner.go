package portableconformancev3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	// HostRunnerReportFormat identifies the Host API v1beta1 runner report.
	HostRunnerReportFormat = "takoform.portable-host-runner-report@v3"
	// ReferenceHostSelfTest classifies a local reference-host run: it proves
	// the executable runner, never an external host.
	ReferenceHostSelfTest = "deterministic-reference-host-self-test"
	// EndpointConformanceRun classifies a disposable external endpoint run.
	EndpointConformanceRun = "disposable-endpoint-conformance-run"
)

// EndpointOptions configures one runner invocation.
type EndpointOptions struct {
	Endpoint             string
	Token                string
	AlternateToken       string
	AlternateTenantToken string
	HTTPClient           *http.Client
	Classification       string
	Subject              string
	// Survey continues past failing steps and reports the whole gap surface
	// as one failure, instead of stopping at the first. It exists for
	// MEASURING a real host mid-implementation; it never yields a passed
	// report, so nothing downstream can mistake a survey for evidence.
	Survey bool
}

// ErrorProbeEvidence records one probed stable error outcome.
type ErrorProbeEvidence struct {
	Code       string `json:"code"`
	HTTPStatus int    `json:"httpStatus"`
	Retryable  bool   `json:"retryable"`
}

// NegativeFixtureEvidence records one executed byte-pinned negative fixture.
type NegativeFixtureEvidence struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Stage  string `json:"stage"`
	SHA256 string `json:"sha256"`
}

// HostRunnerReport is the data-only result of one run. PublicationReady is
// always false: this report proves conformance behavior only and grants no
// publication, admission, support, or maturity state.
type HostRunnerReport struct {
	Format                 string                    `json:"format"`
	Classification         string                    `json:"classification"`
	PublicationReady       bool                      `json:"publicationReady"`
	Status                 string                    `json:"status"`
	Subject                string                    `json:"subject"`
	RunnerSubject          string                    `json:"runnerSubject"`
	Checks                 []string                  `json:"checks"`
	ErrorProbes            []ErrorProbeEvidence      `json:"errorProbes"`
	NegativeFixtures       []NegativeFixtureEvidence `json:"negativeFixtures"`
	UIDTransitions         []string                  `json:"uidTransitions"`
	GenerationTransitions  []string                  `json:"generationTransitions"`
	RevisionTransitions    []string                  `json:"revisionTransitions"`
	ArtifactManifestDigest string                    `json:"artifactManifestDigest"`
	AsyncOperationID       string                    `json:"asyncOperationId"`
	CancelledOperationID   string                    `json:"cancelledOperationId"`
}

// SelfTest starts the deterministic reference host over the real Edge Family
// candidate catalog and drives the complete runner matrix through real HTTP.
// It is local implementation evidence only.
func SelfTest(ctx context.Context, contract Contract) (HostRunnerReport, error) {
	repoRoot, err := filepath.Abs(filepath.Join(contract.Root(), "..", ".."))
	if err != nil {
		return HostRunnerReport{}, err
	}
	catalog, err := LoadCatalog(repoRoot, contract)
	if err != nil {
		return HostRunnerReport{}, err
	}
	host := NewReferenceHost(contract, catalog)
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

// RunEndpoint executes the complete black-box v1beta1 matrix against a
// disposable endpoint. The endpoint must implement the runner-only probe
// header (error, async, touch-status values) through a disposable adapter;
// production endpoints must not implement it.
func RunEndpoint(ctx context.Context, contract Contract, options EndpointOptions) (HostRunnerReport, error) {
	if err := validateContract(contract); err != nil {
		return HostRunnerReport{}, fmt.Errorf("portable host v3 contract: %w", err)
	}
	endpoint := strings.TrimRight(strings.TrimSpace(options.Endpoint), "/")
	if endpoint == "" {
		return HostRunnerReport{}, errors.New("portable host v3 runner endpoint is required")
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	transportClient := *httpClient
	transportClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	classification := options.Classification
	if classification == "" {
		classification = EndpointConformanceRun
	}
	token := strings.TrimSpace(options.Token)
	alternateToken := strings.TrimSpace(options.AlternateToken)
	alternateTenantToken := strings.TrimSpace(options.AlternateTenantToken)
	if token == "" || alternateToken == "" || alternateTenantToken == "" {
		return HostRunnerReport{}, errors.New(
			"portable host v3 runner requires primary, same-tenant alternate-principal, and alternate-tenant credentials",
		)
	}
	if token == alternateToken || token == alternateTenantToken || alternateToken == alternateTenantToken {
		return HostRunnerReport{}, errors.New("portable host v3 runner requires three distinct bearer credentials")
	}
	subject := strings.TrimSpace(options.Subject)
	if subject == "" {
		subject = "host:" + endpoint
	}
	runner := &v3Runner{
		ctx:                  ctx,
		contract:             contract,
		endpoint:             endpoint,
		token:                token,
		alternateToken:       alternateToken,
		alternateTenantToken: alternateTenantToken,
		httpClient:           &transportClient,
		apiBase:              endpoint + contract.APIPath,
		completed:            map[string]bool{},
		survey:               options.Survey,
	}
	if err := runner.run(); err != nil {
		if options.Survey {
			// The survey's product is the gap list, stated with the completed
			// count so a measurement is comparable run to run. It is a FAILURE:
			// a survey never produces a passed report.
			completedCount := 0
			missing := make([]string, 0)
			for _, check := range contract.RequiredRunnerChecks {
				if runner.completed[check] {
					completedCount++
				} else {
					missing = append(missing, check)
				}
			}
			return HostRunnerReport{}, fmt.Errorf(
				"survey: %d/%d required checks completed; missing: %s\n%w",
				completedCount, len(contract.RequiredRunnerChecks),
				strings.Join(missing, ", "), err)
		}
		return HostRunnerReport{}, err
	}
	if options.Survey {
		// The guarantee is absolute: a survey is a measurement, never
		// evidence, even when it measures zero gaps. A conforming endpoint is
		// re-run without --survey to produce the report.
		return HostRunnerReport{}, errors.New(
			"survey completed with every check passing; a survey never produces a passed report - run without --survey for evidence")
	}
	checks := make([]string, 0, len(contract.RequiredRunnerChecks))
	for _, check := range contract.RequiredRunnerChecks {
		if !runner.completed[check] {
			return HostRunnerReport{}, fmt.Errorf("portable host v3 runner did not execute required check %q", check)
		}
		checks = append(checks, check)
	}
	return HostRunnerReport{
		Format:                 HostRunnerReportFormat,
		Classification:         classification,
		PublicationReady:       false,
		Status:                 "passed",
		Subject:                subject,
		RunnerSubject:          "takoform.portable-host-conformance-runner@v3",
		Checks:                 checks,
		ErrorProbes:            runner.errorProbes,
		NegativeFixtures:       runner.negativeFixtures,
		UIDTransitions:         runner.uidTransitions,
		GenerationTransitions:  runner.generationTransitions,
		RevisionTransitions:    runner.revisionTransitions,
		ArtifactManifestDigest: runner.artifactManifestDigest,
		AsyncOperationID:       runner.asyncOperationID,
		CancelledOperationID:   runner.cancelledOperationID,
	}, nil
}

type v3Runner struct {
	ctx                  context.Context
	contract             Contract
	endpoint             string
	token                string
	alternateToken       string
	alternateTenantToken string
	httpClient           *http.Client
	apiBase              string

	// desiredSchemas is the CORPUS-PINNED desiredSchema of every Form the lane
	// drives a probe against, keyed by the whole exact FormRef.
	//
	// Pinned, because materializing against the served schema would make the
	// host under test the author of the specs it is then measured on: it would
	// agree with its own defaults and bounds for the length of a run, and the
	// divergence would only appear at a real client. Keyed by the exact ref
	// rather than by group and kind, because one Form line holds more than one
	// contract in this lane (decision 0022) and a kind-keyed map would answer
	// for whichever arrived first.
	desiredSchemas map[FormRef]map[string]any

	completed              map[string]bool
	survey                 bool
	surveyFailures         []string
	errorProbes            []ErrorProbeEvidence
	negativeFixtures       []NegativeFixtureEvidence
	uidTransitions         []string
	generationTransitions  []string
	revisionTransitions    []string
	artifactManifestDigest string
	asyncOperationID       string
	cancelledOperationID   string
}

func (r *v3Runner) complete(check string) { r.completed[check] = true }

type wireResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

type wireCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	HostReason         string `json:"hostReason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime"`
}

type wireStatus struct {
	ObservedGeneration string          `json:"observedGeneration"`
	Conditions         []wireCondition `json:"conditions"`
	Observed           map[string]any  `json:"observed,omitempty"`
	Outputs            map[string]any  `json:"outputs,omitempty"`
}

type wireResource struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Form       struct {
		FormRef       FormRef `json:"formRef"`
		PackageDigest string  `json:"packageDigest,omitempty"`
	} `json:"form"`
	Metadata struct {
		Name       string `json:"name"`
		Space      string `json:"space"`
		UID        string `json:"uid"`
		Generation string `json:"generation"`
		Revision   string `json:"revision"`
	} `json:"metadata"`
	Spec   map[string]any `json:"spec"`
	Status *wireStatus    `json:"status"`
}

// probeTarget is one exact resource the runner addresses.
type probeTarget struct {
	Ref           FormRef
	PackageDigest string
	Name          string
	Space         string
	Spec          map[string]any
	// Lifecycle is the exact capability set the corpus pins for this Form.
	Lifecycle []string
	// ReadyOptional marks a probe whose Ready condition legitimately depends on
	// other resources, so a generic lifecycle assertion must not demand
	// Ready=True. A Module Worker is the one such probe: its readiness is a
	// claim about SERVICE, and nothing serves until its deployment does
	// (spec/decisions/0016). Every check that cares asserts the exact condition
	// itself rather than leaning on the generic one.
	ReadyOptional bool
}

// target builds one probe target with its desired spec already materialized
// against the Form Definition the host serves. The runner does this for the
// same reason every client must: the host materializes declared defaults at
// its entry point, so a client that compares digests or echoes against an
// unmaterialized spec is comparing against a document that no longer exists.
func (r *v3Runner) target(probe ResourceProbe) probeTarget {
	return probeTarget{
		Ref:           probe.Identity.FormRef,
		PackageDigest: probe.Identity.PackageDigest,
		Name:          probe.Name,
		Space:         r.contract.RunnerInput.Space,
		Spec:          r.materialize(probe.Identity.FormRef, probe.Desired),
		Lifecycle:     append([]string(nil), probe.LifecycleCapabilities...),
		// A Module Worker's readiness follows its deployment, and a class
		// holder's follows the same deployment for the same reason: both
		// legitimately answer Ready=False until something serves them
		// (spec/decisions/0016).
		ReadyOptional: probe.Identity.FormRef.Kind == moduleWorkerKind ||
			classHolderKinds[probe.Identity.FormRef.Kind],
	}
}

// materialize renders one probe's desired spec the way the Form Definition at
// that exact ref declares it, from the corpus pin.
//
// A ref with no pin returns the spec unchanged, and that case is not a fallback
// to the host: contract validation requires a pin for every probe, and the only
// other targets this lane builds — the synthetic second group and the synthetic
// second definition version — carry a literal empty spec the runner writes
// itself and never materializes.
func (r *v3Runner) materialize(ref FormRef, spec map[string]any) map[string]any {
	schema, pinned := r.desiredSchemas[ref]
	if !pinned {
		return spec
	}
	return currentformmodel.MaterializeDefaults(schema, spec)
}

// pinDesiredSchemas loads the runner's declared defaults out of the CORPUS.
//
// It replaces a load that read them from the host under test. The corpus pins
// what to send AND what an omitted property means; the host is asked what it
// serves separately, by `form-definition-exact`, which compares the two. A
// runner that took the answer from the subject had no comparison to make.
func (r *v3Runner) pinDesiredSchemas() {
	input := r.contract.RunnerInput
	r.desiredSchemas = map[FormRef]map[string]any{}
	for _, entry := range declaredProbes(&input) {
		r.desiredSchemas[entry.Probe.Identity.FormRef] = entry.Probe.DesiredSchema.Schema
	}
}

// wireFormDefinition is the served Form Definition surface the runner reads.
type wireFormDefinition struct {
	Identity struct {
		FormRef       FormRef `json:"formRef"`
		PackageDigest string  `json:"packageDigest,omitempty"`
	} `json:"identity"`
	DisplayName   string         `json:"displayName,omitempty"`
	Description   string         `json:"description,omitempty"`
	DesiredSchema map[string]any `json:"desiredSchema"`
	// Constraints is the served constraint list. A client reads the rules it
	// must obey from the same document that states the shape.
	Constraints []formpackage.FormConstraint `json:"constraints,omitempty"`
}

func (r *v3Runner) formDefinition(ref FormRef) (wireFormDefinition, error) {
	fullURL := fmt.Sprintf(
		"%s/form-definitions/%s/%s?%s",
		r.apiBase, groupSegments(ref.APIVersion), url.PathEscape(ref.Kind),
		r.exactQuery(r.contract.RunnerInput.Space, ref).Encode(),
	)
	response, err := r.request(http.MethodGet, fullURL, nil, nil)
	if err != nil {
		return wireFormDefinition{}, err
	}
	if response.Status != http.StatusOK {
		return wireFormDefinition{}, fmt.Errorf(
			"form-definition HTTP %d; body=%s", response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	var definition wireFormDefinition
	if err := decodeStrictResponse(response, &definition); err != nil {
		return wireFormDefinition{}, err
	}
	if definition.Identity.FormRef != ref {
		return wireFormDefinition{}, errors.New("form-definition identity is not the requested exact FormRef")
	}
	if len(definition.DesiredSchema) == 0 {
		return wireFormDefinition{}, errors.New("form-definition omitted the desiredSchema")
	}
	return definition, nil
}

// ---- transport helpers ----

func (r *v3Runner) requestWithToken(
	token, method, target string,
	headers map[string]string,
	body []byte,
) (wireResponse, error) {
	return r.requestWithAuthorization("Bearer "+token, method, target, headers, body)
}

// requestWithAuthorization is the transport with the credential stated
// literally. An EMPTY authorization omits the header entirely, which is the one
// request shape every other helper in this file makes unsendable — and the shape
// that decides whether a host reads its tenant from the credential or from a
// default it falls back to.
func (r *v3Runner) requestWithAuthorization(
	authorization, method, target string,
	headers map[string]string,
	body []byte,
) (wireResponse, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(r.ctx, method, target, reader)
	if err != nil {
		return wireResponse{}, err
	}
	request.Header.Set("Accept", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := r.httpClient.Do(request)
	if err != nil {
		return wireResponse{}, err
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	if err != nil {
		return wireResponse{}, err
	}
	return wireResponse{Status: response.StatusCode, Header: response.Header.Clone(), Body: data}, nil
}

func (r *v3Runner) request(method, target string, headers map[string]string, body []byte) (wireResponse, error) {
	return r.requestWithToken(r.token, method, target, headers, body)
}

func encodeRunnerJSON(value any) ([]byte, error) { return json.Marshal(value) }

// groupSegments renders a namespaced Form group as the two ordinary path
// segments the lane's URL templates declare, {formGroup}/{formVersion}. No
// path segment the runner builds ever percent-encodes a slash
// (spec/decisions/0018).
func groupSegments(group string) string {
	name, version, split := strings.Cut(group, "/")
	if !split {
		return url.PathEscape(group)
	}
	return url.PathEscape(name) + "/" + url.PathEscape(version)
}

func (r *v3Runner) resourceURL(ref FormRef, name, action string, query url.Values) string {
	target := fmt.Sprintf(
		"%s/resources/%s/%s/%s",
		r.apiBase, groupSegments(ref.APIVersion), url.PathEscape(ref.Kind), url.PathEscape(name),
	)
	if action != "" {
		target += "/" + action
	}
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	return target
}

func (r *v3Runner) exactQuery(space string, ref FormRef) url.Values {
	query := url.Values{}
	query.Set("space", space)
	// `group` here is the WHOLE apiVersion. This is the exact-Form query of a
	// resource route, whose vocabulary is the four FormRef members, and it is
	// not the /forms filter vocabulary — that one splits group from version
	// and takes six keys. The two are separate grammars on separate routes;
	// formsExactQuery builds the other one.
	query.Set("group", ref.APIVersion)
	query.Set("kind", ref.Kind)
	query.Set("definitionVersion", ref.DefinitionVersion)
	query.Set("schemaDigest", ref.SchemaDigest)
	return query
}

// ---- decode/assert helpers ----

// canonicalJSON renders one decoded document as RFC 8785 bytes, so two
// documents that say the same thing compare equal however each was spelled and
// whichever loader decoded it.
func canonicalJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}

func decodeStrictResponse(response wireResponse, target any) error {
	if len(bytes.TrimSpace(response.Body)) == 0 {
		return errors.New("empty JSON response body")
	}
	return formpackage.DecodeStrictIJSON(response.Body, target)
}

func (r *v3Runner) expectStableError(response wireResponse, code string) error {
	wantStatus, known := r.contract.lane.ErrorHTTPStatus[code]
	if !known {
		return fmt.Errorf("unknown stable error code %q", code)
	}
	if response.Status != wantStatus {
		return fmt.Errorf(
			"HTTP %d, want %d for %s; body=%s",
			response.Status, wantStatus, code, strings.TrimSpace(string(response.Body)),
		)
	}
	var envelope struct {
		Error struct {
			Code      string          `json:"code"`
			Message   string          `json:"message"`
			RequestID string          `json:"requestId"`
			Retryable *bool           `json:"retryable"`
			HostCode  string          `json:"hostCode,omitempty"`
			Details   json.RawMessage `json:"details,omitempty"`
		} `json:"error"`
	}
	if err := decodeStrictResponse(response, &envelope); err != nil {
		return fmt.Errorf("decode %s error envelope: %w", code, err)
	}
	if envelope.Error.Code != code ||
		strings.TrimSpace(envelope.Error.Message) == "" ||
		strings.TrimSpace(envelope.Error.RequestID) == "" ||
		envelope.Error.Retryable == nil ||
		*envelope.Error.Retryable != r.contract.lane.isAutomaticallyRetryable(code) {
		return fmt.Errorf("invalid %s error envelope: %s", code, strings.TrimSpace(string(response.Body)))
	}
	return nil
}

func decodeResource(response wireResponse, wantStatuses ...int) (wireResource, error) {
	if len(wantStatuses) > 0 {
		matched := false
		for _, status := range wantStatuses {
			if response.Status == status {
				matched = true
			}
		}
		if !matched {
			return wireResource{}, fmt.Errorf(
				"HTTP %d, want %v; body=%s",
				response.Status, wantStatuses, strings.TrimSpace(string(response.Body)),
			)
		}
	}
	var resource wireResource
	if err := decodeStrictResponse(response, &resource); err != nil {
		return wireResource{}, err
	}
	return resource, nil
}

func decodeResourceEnvelope(response wireResponse) (wireResource, error) {
	if response.Status != http.StatusOK {
		return wireResource{}, fmt.Errorf(
			"HTTP %d, want 200; body=%s", response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	var envelope struct {
		Resource wireResource `json:"resource"`
	}
	if err := decodeStrictResponse(response, &envelope); err != nil {
		return wireResource{}, err
	}
	return envelope.Resource, nil
}

func (l lane) verifyResourceIdentity(got wireResource, target probeTarget) error {
	if got.APIVersion != target.Ref.APIVersion || got.Kind != target.Ref.Kind ||
		got.Form.FormRef != target.Ref {
		return errors.New("host response changed the exact FormRef identity")
	}
	if got.Metadata.Name != target.Name || got.Metadata.Space != target.Space {
		return errors.New("host response changed the requested name or space")
	}
	if !uidPattern.MatchString(got.Metadata.UID) {
		return errors.New("host response omitted a valid metadata.uid")
	}
	if got.Metadata.Generation == "" || got.Metadata.Revision == "" {
		return errors.New("host response omitted generation or revision")
	}
	if got.Status == nil || got.Status.ObservedGeneration != got.Metadata.Generation {
		return errors.New("host response status.observedGeneration must reflect metadata.generation")
	}
	if err := l.verifyClosedConditionReasons(got); err != nil {
		return err
	}
	ready := false
	for _, condition := range got.Status.Conditions {
		if condition.Type != "Ready" {
			continue
		}
		complete := condition.Reason != "" && condition.LastTransitionTime != ""
		// A probe whose readiness depends on other resources may legitimately
		// answer Ready=False; the condition must still be COMPLETE, and the
		// check that owns that state asserts its exact reason.
		if condition.Status == "True" || (target.ReadyOptional && condition.Status == "False") {
			ready = complete
		}
	}
	if !ready {
		return errors.New("host response omitted a complete Ready condition")
	}
	return nil
}

// verifyClosedConditionReasons holds every returned condition to the closed
// portable reason vocabulary. Two conforming hosts must name one state with
// one reason, so an unrecognized reason is a portability defect even when it
// is otherwise well formed; host-specific detail belongs in hostReason.
func (l lane) verifyClosedConditionReasons(got wireResource) error {
	if got.Status == nil {
		return nil
	}
	for _, condition := range got.Status.Conditions {
		if !l.isPortableConditionReason(condition.Reason) {
			return fmt.Errorf(
				"condition %s carries reason %q outside the closed portable vocabulary %v; "+
					"host-specific detail belongs in hostReason",
				condition.Type, condition.Reason, l.ConditionReasons,
			)
		}
	}
	return nil
}

func verifyRevisionETag(response wireResponse, revision string) error {
	values := response.Header.Values("ETag")
	if len(values) != 1 || values[0] != `"`+revision+`"` {
		return fmt.Errorf("ETag = %v, want exactly %q", values, `"`+revision+`"`)
	}
	return nil
}

// ---- lifecycle request builders ----

type prepareOutcome struct {
	PrepareDigest string
	SpecDigest    string
}

func (r *v3Runner) resourceBody(target probeTarget) map[string]any {
	form := map[string]any{"formRef": refJSON(target.Ref)}
	if target.PackageDigest != "" {
		form["packageDigest"] = target.PackageDigest
	}
	spec := target.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	return map[string]any{
		"apiVersion": target.Ref.APIVersion,
		"kind":       target.Ref.Kind,
		"form":       form,
		"metadata":   map[string]any{"name": target.Name, "space": target.Space},
		"spec":       spec,
	}
}

// prepareRequest posts one raw prepare; extraHeaders may carry the update
// generation fence.
func (r *v3Runner) prepareRequest(target probeTarget, extraHeaders map[string]string) (wireResponse, error) {
	return r.prepareRequestAs(r.token, target, extraHeaders)
}

// prepareRequestAs posts one raw prepare under a named credential.
func (r *v3Runner) prepareRequestAs(
	token string,
	target probeTarget,
	extraHeaders map[string]string,
) (wireResponse, error) {
	body, err := encodeRunnerJSON(r.resourceBody(target))
	if err != nil {
		return wireResponse{}, err
	}
	return r.requestWithToken(token, http.MethodPost, r.apiBase+"/resources/prepare", extraHeaders, body)
}

// prepare binds a create-intent prepare (no update fence).
func (r *v3Runner) prepare(target probeTarget) (prepareOutcome, error) {
	return r.prepareWithFence(target, "")
}

// prepareWithFence binds a prepare under the Takoform-Expected-Generation
// update fence when generation is non-empty ("generation-fence-when-
// updating").
func (r *v3Runner) prepareWithFence(target probeTarget, generation string) (prepareOutcome, error) {
	return r.prepareWithFenceAs(r.token, target, generation)
}

// prepareWithFenceAs is prepareWithFence under a named credential, so a check
// that drives a mutation as another caller mints that caller's OWN review
// instead of borrowing the primary's.
func (r *v3Runner) prepareWithFenceAs(token string, target probeTarget, generation string) (prepareOutcome, error) {
	var headers map[string]string
	if generation != "" {
		headers = map[string]string{expectedGenerationHeader: generation}
	}
	response, err := r.prepareRequestAs(token, target, headers)
	if err != nil {
		return prepareOutcome{}, err
	}
	if response.Status != http.StatusOK {
		return prepareOutcome{}, fmt.Errorf(
			"prepare HTTP %d; body=%s", response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	var result struct {
		Resource wireResource `json:"resource"`
		Review   struct {
			PrepareDigest string `json:"prepareDigest"`
			SpecDigest    string `json:"specDigest"`
		} `json:"review"`
	}
	if err := decodeStrictResponse(response, &result); err != nil {
		return prepareOutcome{}, err
	}
	if result.Resource.Form.FormRef != target.Ref ||
		result.Resource.Metadata.Name != target.Name ||
		result.Resource.Metadata.Space != target.Space {
		return prepareOutcome{}, errors.New("prepare substituted the requested identity")
	}
	// The digest is of the MATERIALIZED spec, which is what the echo carries.
	// Comparing against the spec as SENT would be wrong for the one request
	// that omits a defaulted property — and that is not an exception, it is
	// the rule: a host materializes at the entry point, so the effective spec
	// is what came back, and a client's own desired state is the echo.
	echoedSpecDigest, err := specCanonicalDigest(result.Resource.Spec)
	if err != nil {
		return prepareOutcome{}, err
	}
	if result.Review.SpecDigest != echoedSpecDigest {
		return prepareOutcome{}, errors.New(
			"prepare bound a specDigest that is not the digest of the spec it echoed",
		)
	}
	// The echo must be the caller's own desired state MATERIALIZED and nothing
	// else: every property the caller wrote survives unchanged, every declared
	// default is filled, and nothing the caller never wrote appears. Comparing
	// only the written keys would let a host add a property of its own and
	// echo it consistently forever. Canonical JSON, so a number that decoded
	// to another Go type is not read as a different value.
	wantEcho, err := canonicalJSON(r.materialize(target.Ref, target.Spec))
	if err != nil {
		return prepareOutcome{}, err
	}
	gotEcho, err := canonicalJSON(result.Resource.Spec)
	if err != nil {
		return prepareOutcome{}, err
	}
	if gotEcho != wantEcho {
		return prepareOutcome{}, fmt.Errorf(
			"prepare echoed %s, want the materialized %s; materialization fills absent properties, "+
				"never rewrites a written one, and never adds one the caller did not write",
			gotEcho, wantEcho,
		)
	}
	if !formpackage.ValidDigest(result.Review.PrepareDigest) {
		return prepareOutcome{}, errors.New("prepare returned an invalid prepareDigest")
	}
	return prepareOutcome{
		PrepareDigest: result.Review.PrepareDigest,
		SpecDigest:    result.Review.SpecDigest,
	}, nil
}

type applyOptions struct {
	// SpecDigestEcho sends the optional review.specDigest. Empty omits it,
	// which is the ordinary case: the echo is a client's own belt-and-braces
	// check that the spec it is applying is the spec it prepared.
	SpecDigestEcho string

	Create bool
	// BodyGeneration sends the fence through the BODY transport instead of the
	// header. The lane offers a client both and calls them equal, so a runner
	// that only ever sent the header would leave half of a published transport
	// unproven — and a host serving only the header would pass.
	BodyGeneration string
	// DisagreeingBodyGeneration sends BOTH transports with different values,
	// which the lane refuses before either is evaluated.
	DisagreeingBodyGeneration string
	// CreateWithGenerationFence sends If-None-Match together with a generation
	// fence: two contradictory beliefs about whether the resource exists.
	CreateWithGenerationFence string
	ExpectedGeneration        string
	ExpectedUID               string
	IdempotencyKey            string
	OmitIdempotencyKey        bool
	OmitPrecondition          bool
	ExtraHeaders              map[string]string
	PrepareDigest             string
}

func (r *v3Runner) applyRequestParts(target probeTarget, options applyOptions) (string, map[string]string, []byte, error) {
	document := r.resourceBody(target)
	review := map[string]any{"prepareDigest": options.PrepareDigest}
	if options.SpecDigestEcho != "" {
		review["specDigest"] = options.SpecDigestEcho
	}
	document["review"] = review
	if options.ExpectedUID != "" {
		document["expectedUid"] = options.ExpectedUID
	}
	if options.BodyGeneration != "" {
		document["expectedGeneration"] = options.BodyGeneration
	}
	if options.DisagreeingBodyGeneration != "" {
		document["expectedGeneration"] = options.DisagreeingBodyGeneration
	}
	body, err := encodeRunnerJSON(document)
	if err != nil {
		return "", nil, nil, err
	}
	headers := map[string]string{}
	if !options.OmitIdempotencyKey {
		headers["Idempotency-Key"] = options.IdempotencyKey
	}
	if !options.OmitPrecondition {
		switch {
		case options.Create:
			headers["If-None-Match"] = "*"
			if options.CreateWithGenerationFence != "" {
				headers[expectedGenerationHeader] = options.CreateWithGenerationFence
			}
		case options.BodyGeneration != "":
			// Body transport only: the header is deliberately absent, which is
			// what makes this a test of the transport rather than of both.
		default:
			headers[expectedGenerationHeader] = options.ExpectedGeneration
		}
	}
	for key, value := range options.ExtraHeaders {
		headers[key] = value
	}
	return r.resourceURL(target.Ref, target.Name, "", nil), headers, body, nil
}

func (r *v3Runner) apply(target probeTarget, options applyOptions) (wireResponse, error) {
	return r.applyAs(r.token, target, options)
}

// applyAs drives one apply under a named credential, minting the prepare
// binding under that same credential.
func (r *v3Runner) applyAs(token string, target probeTarget, options applyOptions) (wireResponse, error) {
	if options.PrepareDigest == "" {
		fence := ""
		if !options.Create {
			fence = options.ExpectedGeneration
		}
		prepared, err := r.prepareWithFenceAs(token, target, fence)
		if err != nil {
			return wireResponse{}, err
		}
		options.PrepareDigest = prepared.PrepareDigest
	}
	fullURL, headers, body, err := r.applyRequestParts(target, options)
	if err != nil {
		return wireResponse{}, err
	}
	return r.requestWithToken(token, http.MethodPut, fullURL, headers, body)
}

func (r *v3Runner) applyResource(target probeTarget, options applyOptions, wantStatus int) (wireResource, wireResponse, error) {
	response, err := r.apply(target, options)
	if err != nil {
		return wireResource{}, wireResponse{}, err
	}
	resource, err := decodeResource(response, wantStatus)
	if err != nil {
		return wireResource{}, wireResponse{}, err
	}
	if err := r.contract.lane.verifyResourceIdentity(resource, target); err != nil {
		return wireResource{}, wireResponse{}, err
	}
	if err := verifyRevisionETag(response, resource.Metadata.Revision); err != nil {
		return wireResource{}, wireResponse{}, err
	}
	return resource, response, nil
}

// importOptions configures one adoption of a pre-existing native resource.
type importOptions struct {
	NativeID           string
	IdempotencyKey     string
	Create             bool
	ExpectedGeneration string
}

func (r *v3Runner) importResource(target probeTarget, options importOptions) (wireResponse, error) {
	return r.importResourceAs(r.token, target, options)
}

// importResourceAs is importResource under a named credential: adoption is a
// mutation, so a check that proves what a foreign caller may not adopt has to
// speak as that caller.
func (r *v3Runner) importResourceAs(
	token string,
	target probeTarget,
	options importOptions,
) (wireResponse, error) {
	document := r.resourceBody(target)
	document["nativeId"] = options.NativeID
	headers := map[string]string{"Idempotency-Key": options.IdempotencyKey}
	if options.Create {
		headers["If-None-Match"] = "*"
	} else {
		headers[expectedGenerationHeader] = options.ExpectedGeneration
	}
	body, err := encodeRunnerJSON(document)
	if err != nil {
		return wireResponse{}, err
	}
	return r.requestWithToken(
		token, http.MethodPost, r.resourceURL(target.Ref, target.Name, "import", nil), headers, body,
	)
}

func (r *v3Runner) read(target probeTarget) (wireResource, wireResponse, error) {
	response, err := r.request(
		http.MethodGet,
		r.resourceURL(target.Ref, target.Name, "", r.exactQuery(target.Space, target.Ref)),
		nil, nil,
	)
	if err != nil {
		return wireResource{}, wireResponse{}, err
	}
	resource, err := decodeResource(response, http.StatusOK)
	if err != nil {
		return wireResource{}, wireResponse{}, err
	}
	if err := r.contract.lane.verifyResourceIdentity(resource, target); err != nil {
		return wireResource{}, wireResponse{}, err
	}
	if err := verifyRevisionETag(response, resource.Metadata.Revision); err != nil {
		return wireResource{}, wireResponse{}, err
	}
	return resource, response, nil
}

// readRaw reads one resource without requiring a Ready=True condition. A
// source whose relation target moved is legitimately NOT ready, so the
// relation checks need a read that reports the condition instead of rejecting
// it.
func (r *v3Runner) readRaw(target probeTarget) (wireResource, error) {
	resource, _, err := r.readRawResponse(target)
	return resource, err
}

// readRawResponse is readRaw with the transport response, so a caller can hold
// the host to the ETag that accompanied the exact representation it just read.
func (r *v3Runner) readRawResponse(target probeTarget) (wireResource, wireResponse, error) {
	response, err := r.request(
		http.MethodGet,
		r.resourceURL(target.Ref, target.Name, "", r.exactQuery(target.Space, target.Ref)),
		nil, nil,
	)
	if err != nil {
		return wireResource{}, wireResponse{}, err
	}
	resource, err := decodeResource(response, http.StatusOK)
	if err != nil {
		return wireResource{}, wireResponse{}, err
	}
	if err := r.contract.lane.verifyClosedConditionReasons(resource); err != nil {
		return wireResource{}, wireResponse{}, err
	}
	return resource, response, nil
}

// requireRevisionAdvanced holds one representation change to the identity rule
// of decision 0011: the revision moves FORWARD, the generation does not move at
// all, and the strong ETag served with the new representation is the new
// revision. Anything less makes the ETag a validator that says "unchanged"
// about a change.
func requireRevisionAdvanced(before, after wireResource, response wireResponse, subject string) error {
	if after.Metadata.Generation != before.Metadata.Generation {
		return fmt.Errorf(
			"%s moved generation %s -> %s; no desired spec changed",
			subject, before.Metadata.Generation, after.Metadata.Generation,
		)
	}
	beforeRevision, err := parseRevision(before.Metadata.Revision)
	if err != nil {
		return fmt.Errorf("%s: %w", subject, err)
	}
	afterRevision, err := parseRevision(after.Metadata.Revision)
	if err != nil {
		return fmt.Errorf("%s: %w", subject, err)
	}
	if afterRevision <= beforeRevision {
		return fmt.Errorf(
			"%s left revision at %s while the representation changed; want a revision greater than %s",
			subject, after.Metadata.Revision, before.Metadata.Revision,
		)
	}
	if err := verifyRevisionETag(response, after.Metadata.Revision); err != nil {
		return fmt.Errorf("%s: %w", subject, err)
	}
	return nil
}

// parseRevision reads one canonical decimal revision. The wire type is a
// string, but the ordering the identity contract states is numeric.
func parseRevision(revision string) (int64, error) {
	value, err := strconv.ParseInt(revision, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("revision %q is not a canonical decimal string", revision)
	}
	return value, nil
}

// readyCondition returns the Ready condition, or the zero value when absent.
func readyCondition(got wireResource) wireCondition {
	if got.Status == nil {
		return wireCondition{}
	}
	for _, condition := range got.Status.Conditions {
		if condition.Type == "Ready" {
			return condition
		}
	}
	return wireCondition{}
}

// requireNotReady holds one resource to a Ready=False condition carrying an
// exact portable reason and a non-empty hostReason.
func requireNotReady(got wireResource, reason string) error {
	condition := readyCondition(got)
	if condition.Type != "Ready" {
		return errors.New("response carries no Ready condition")
	}
	if condition.Status != "False" || condition.Reason != reason {
		return fmt.Errorf(
			"Ready = %s/%s, want False/%s",
			condition.Status, condition.Reason, reason,
		)
	}
	if condition.HostReason == "" {
		return fmt.Errorf("Ready=False/%s carries no hostReason naming what changed", reason)
	}
	return nil
}

func (r *v3Runner) expectResourceAbsent(target probeTarget) error {
	response, err := r.request(
		http.MethodGet,
		r.resourceURL(target.Ref, target.Name, "", r.exactQuery(target.Space, target.Ref)),
		nil, nil,
	)
	if err != nil {
		return err
	}
	return r.expectStableError(response, "resource_not_found")
}

// deleteResource deletes under the delete fence this lane requires: the
// expected DESIRED GENERATION. The representation revision is deliberately not
// sent — a client that fenced a teardown on it would be refused by a revision
// its own teardown moved, because removing a dependent re-renders what is left
// (spec/decisions/0011, 0016 rule 9). A check that means to exercise the
// optional representation fence passes `If-Match` in extraHeaders.
func (r *v3Runner) deleteResource(
	target probeTarget,
	generation, key string,
	extraHeaders map[string]string,
) (wireResponse, error) {
	headers := map[string]string{
		expectedGenerationHeader: generation,
		"Idempotency-Key":        key,
	}
	for headerKey, value := range extraHeaders {
		headers[headerKey] = value
	}
	return r.request(
		http.MethodDelete,
		r.resourceURL(target.Ref, target.Name, "", r.exactQuery(target.Space, target.Ref)),
		headers, nil,
	)
}

func (r *v3Runner) statusAction(
	target probeTarget,
	action, generation, key string,
	extraHeaders map[string]string,
) (wireResponse, error) {
	return r.statusActionAs(r.token, target, action, generation, key, extraHeaders)
}

// statusActionAs is statusAction under a named credential, so a check can drive
// the fenced read-only re-observation as a caller that does not hold the
// resource it names.
func (r *v3Runner) statusActionAs(
	token string,
	target probeTarget,
	action, generation, key string,
	extraHeaders map[string]string,
) (wireResponse, error) {
	headers := map[string]string{
		expectedGenerationHeader: generation,
		"Idempotency-Key":        key,
	}
	for headerKey, value := range extraHeaders {
		headers[headerKey] = value
	}
	return r.requestWithToken(
		token, http.MethodPost,
		r.resourceURL(target.Ref, target.Name, action, r.exactQuery(target.Space, target.Ref)),
		headers, nil,
	)
}
