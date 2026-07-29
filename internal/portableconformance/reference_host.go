package portableconformance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

const (
	referencePrimaryToken         = "reference-primary-token"
	referenceAlternateToken       = "reference-alternate-token"
	referenceAlternateTenantToken = "reference-alternate-tenant-token"
)

var referenceCapabilityToken = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:-]{0,127}$`)

// referenceHost is a deliberately small, deterministic implementation used to
// prove the runner itself. It is not a reusable host and its reports are never
// publication-ready.
type referenceHost struct {
	mu sync.Mutex

	contract                        Contract
	resources                       map[string]*client.Resource
	previews                        map[string][]byte
	replays                         map[string]recordedReplay
	enforcePlanBinding              bool
	enforceIdempotentReplay         bool
	scopeReplayToTenant             bool
	scopeReplayToPrincipal          bool
	scopeReplayToSpace              bool
	reauthorizeReplayAuthentication bool
	reauthorizeReplayPermission     bool
	reauthorizeReplayPolicy         bool
	scopeResourcesToSpace           bool
	resolveConnectionsAcrossSpaces  bool
	invalidObservedProjection       bool
	mismatchedPortability           bool
	omitInterfaceForm               bool
	inventInterfaceResourceURI      bool
	acceptInterfaceWrites           bool
	ignoreInterfaceQueryRules       bool
	leakInterfacesAcrossSpaces      bool
	rejectOmittedVersion            bool
	resolveAmbiguousInstance        bool
	projectInterfaceAuthority       bool
	exposeInterfaceBeforeReady      bool
}

type recordedReplay struct {
	Fingerprint string
	Status      int
	ETag        string
	Body        []byte
}

func newReferenceHost(contract Contract) *referenceHost {
	return &referenceHost{
		contract: contract, resources: map[string]*client.Resource{},
		previews: map[string][]byte{}, replays: map[string]recordedReplay{},
		enforcePlanBinding: true, enforceIdempotentReplay: true,
		scopeReplayToTenant: true, scopeReplayToPrincipal: true, scopeReplayToSpace: true,
		reauthorizeReplayAuthentication: true,
		reauthorizeReplayPermission:     true,
		reauthorizeReplayPolicy:         true,
		scopeResourcesToSpace:           true,
	}
}

func (h *referenceHost) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	if !h.authorizeRequest(w, request) {
		return
	}
	if probe := request.Header.Get(RawJSONProbeHeader); probe != "" {
		if probe != RawJSONProbeDuplicateErrorCode {
			writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "unknown raw JSON conformance probe")
			return
		}
		writeReferenceRaw(
			w,
			http.StatusBadRequest,
			"",
			[]byte(`{"error":{"code":"invalid_argument","code":"invalid_argument","message":"duplicate error code probe","requestId":"reference-duplicate-error-code","retryable":false}}`),
		)
		return
	}
	if code := request.Header.Get(ErrorProbeHeader); code != "" {
		status, retryable, ok := stableErrorSemantics(code)
		if !ok {
			writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "unknown conformance error probe")
			return
		}
		writeReferenceError(w, status, code, retryable, "deterministic conformance error probe")
		return
	}
	if probe := request.Header.Get(PlanBindingProbeHeader); probe != "" {
		if !containsValue(h.contract.PlanBinding.InstrumentedAdapter.Inputs, probe) {
			writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "unknown conformance plan-binding probe")
			return
		}
		if request.Method != http.MethodPut ||
			!strings.HasPrefix(request.URL.Path, h.contract.APIPath+"/resources/") {
			writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "plan-binding probe is only valid for disposable apply instrumentation")
			return
		}
	}
	if request.URL.Path == h.contract.DiscoveryPath && request.Method == http.MethodGet {
		origin := "http://" + request.Host
		writeReferenceJSON(w, http.StatusOK, "", map[string]any{
			"api_versions": []string{h.contract.APIVersion},
			"features": map[string]bool{
				"service_forms": true, "exact_form_ref": true,
				"optimistic_concurrency": true, "idempotent_lifecycle": true,
				client.FeatureInterfaceDeclarations: true,
			},
			"endpoints": map[string]string{
				"api":        origin + h.contract.APIPath,
				"forms":      origin + h.contract.APIPath + "/forms",
				"interfaces": origin + h.contract.APIPath + "/interfaces",
			},
		})
		return
	}
	if request.URL.Path == h.contract.APIPath+"/forms" && request.Method == http.MethodGet {
		h.handleForms(w, request)
		return
	}
	if request.URL.Path == h.contract.APIPath+"/interfaces" ||
		strings.HasPrefix(request.URL.Path, h.contract.APIPath+"/interfaces/") {
		h.handleInterfaces(w, request)
		return
	}
	if request.URL.Path == h.contract.APIPath+"/resources/preview" && request.Method == http.MethodPost {
		h.handlePreview(w, request)
		return
	}
	resourcePrefix := h.contract.APIPath + "/resources/"
	if !strings.HasPrefix(request.URL.Path, resourcePrefix) {
		writeReferenceError(w, http.StatusNotFound, "resource_not_found", false, "unknown route")
		return
	}
	parts := strings.Split(strings.TrimPrefix(request.URL.Path, resourcePrefix), "/")
	if len(parts) < 2 || len(parts) > 3 {
		writeReferenceError(w, http.StatusNotFound, "resource_not_found", false, "unknown route")
		return
	}
	kind, name := parts[0], parts[1]
	action := ""
	if len(parts) == 3 {
		action = parts[2]
	}
	if _, ok := h.referenceIdentityForKind(kind); !ok || strings.TrimSpace(name) == "" {
		writeReferenceError(w, http.StatusNotFound, "resource_not_found", false, "unknown Resource")
		return
	}
	switch {
	case request.Method == http.MethodPut && action == "":
		h.handleApply(w, request, name)
	case request.Method == http.MethodGet && action == "":
		h.handleRead(w, request, name)
	case request.Method == http.MethodPost && action == "observe":
		h.handleObserveOrRefresh(w, request, name, "observe")
	case request.Method == http.MethodPost && action == "refresh":
		h.handleObserveOrRefresh(w, request, name, "refresh")
	case request.Method == http.MethodPost && action == "import":
		h.handleImport(w, request, name)
	case request.Method == http.MethodDelete && action == "":
		h.handleDelete(w, request, name)
	default:
		writeReferenceError(w, http.StatusNotFound, "resource_not_found", false, "unknown operation")
	}
}

func (h *referenceHost) handleForms(w http.ResponseWriter, request *http.Request) {
	identity, ok := h.referenceIdentityForExactQuery(request.URL.Query())
	if !ok {
		writeReferenceError(w, http.StatusNotFound, "form_unknown", false, "exact Form is unknown")
		return
	}
	writeReferenceJSON(w, http.StatusOK, "", map[string]any{
		"forms": []client.FormAvailability{{
			Identity:        identity,
			DefinitionKnown: true, Installed: true, Executable: true, Activated: true, AvailableToPrincipal: true,
			Operations: []string{"create", "read", "update", "delete", "import", "observe", "refresh"},
		}},
	})
}

func (h *referenceHost) handleInterfaces(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		if h.acceptInterfaceWrites {
			writeReferenceJSON(w, http.StatusOK, "", map[string]any{"accepted": true})
			return
		}
		writeReferenceError(w, http.StatusNotFound, "resource_not_found", false, "unknown Interface operation")
		return
	}
	query := request.URL.Query()
	isList := request.URL.Path == h.contract.APIPath+"/interfaces"
	allowed := map[string]bool{"space": true}
	if !isList {
		allowed["version"] = true
		allowed["resourceKind"] = true
		allowed["resourceName"] = true
	}
	if !h.ignoreInterfaceQueryRules {
		for key, values := range query {
			if !allowed[key] || len(values) != 1 {
				writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "Interface query vocabulary is closed")
				return
			}
		}
	}
	spaceValues, hasSpace := query["space"]
	if !hasSpace || len(spaceValues) != 1 {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "exactly one space is required")
		return
	}
	if err := client.ValidateSpaceID(spaceValues[0]); err != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "invalid SpaceID")
		return
	}
	resourceKind, hasResourceKind := query["resourceKind"]
	resourceName, hasResourceName := query["resourceName"]
	hasResourceSelector := hasResourceKind && hasResourceName
	if !h.ignoreInterfaceQueryRules && hasResourceKind != hasResourceName {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "resourceKind and resourceName must be supplied together")
		return
	}
	if hasResourceSelector && (strings.TrimSpace(resourceKind[0]) == "" || strings.TrimSpace(resourceName[0]) == "") {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "Resource selector values must be non-empty")
		return
	}
	visible := h.visibleInterfaces(spaceValues[0])
	if isList {
		writeReferenceJSON(w, http.StatusOK, "", map[string]any{"interfaces": visible})
		return
	}
	if h.rejectOmittedVersion && query.Get("version") == "" {
		writeReferenceError(w, http.StatusNotFound, "resource_not_found", false, "version is required")
		return
	}
	name, err := url.PathUnescape(strings.TrimPrefix(request.URL.Path, h.contract.APIPath+"/interfaces/"))
	if err != nil || strings.TrimSpace(name) == "" {
		writeReferenceError(w, http.StatusNotFound, "resource_not_found", false, "Interface not visible")
		return
	}
	matches := make([]client.DeclaredInterface, 0, len(visible))
	for _, candidate := range visible {
		if candidate.Name != name {
			continue
		}
		if version := query.Get("version"); version != "" && candidate.Version != version {
			continue
		}
		if hasResourceSelector &&
			(candidate.Resource.Kind != resourceKind[0] || candidate.Resource.Name != resourceName[0]) {
			continue
		}
		matches = append(matches, candidate)
	}
	if len(matches) == 0 {
		writeReferenceError(w, http.StatusNotFound, "resource_not_found", false, "Interface not visible")
		return
	}
	if query.Get("version") == "" {
		versions := map[string]bool{}
		for _, candidate := range matches {
			versions[candidate.Version] = true
		}
		if len(versions) > 1 {
			writeReferenceError(w, http.StatusConflict, "interface_identity_ambiguous", false, "Interface version is ambiguous")
			return
		}
	}
	if !hasResourceSelector && len(matches) > 1 && !h.resolveAmbiguousInstance {
		writeReferenceError(w, http.StatusConflict, "interface_instance_ambiguous", false, "Interface Resource instance is ambiguous")
		return
	}
	if len(matches) != 1 && !h.resolveAmbiguousInstance {
		writeReferenceError(w, http.StatusConflict, "interface_instance_ambiguous", false, "Interface Resource instance is ambiguous")
		return
	}
	writeReferenceJSON(w, http.StatusOK, "", matches[0])
}

func (h *referenceHost) handlePreview(w http.ResponseWriter, request *http.Request) {
	var resource client.Resource
	if err := decodeReferenceBody(request, &resource); err != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, err.Error())
		return
	}
	if err := h.validateRequestedResource(resource); err != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, err.Error())
		return
	}
	if !h.previewFence(
		request,
		resource.Metadata.Space,
		resource.Kind,
		resource.Metadata.Name,
		resource.Metadata.ResourceVersion,
	) {
		writeReferenceError(w, http.StatusPreconditionFailed, "resource_version_conflict", false, "preview generation fence failed")
		return
	}
	if resource.Status != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "preview request must not contain status")
		return
	}
	specDigest, err := digestJSON(resource.Spec)
	if err != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "spec is not canonicalizable I-JSON")
		return
	}
	planPayload := struct {
		SpecDigest string          `json:"specDigest"`
		Resource   client.Resource `json:"resource"`
		Plan       string          `json:"plan"`
	}{SpecDigest: specDigest, Resource: resource, Plan: "deterministic-reference-plan"}
	planDigest, err := digestJSON(planPayload)
	if err != nil {
		writeReferenceError(w, http.StatusInternalServerError, "internal_error", false, err.Error())
		return
	}
	canonicalResource, err := canonicalResourceBytes(resource)
	if err != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, err.Error())
		return
	}
	h.previews[planDigest] = canonicalResource
	writeReferenceJSON(w, http.StatusOK, "", client.PreviewResourceResult{
		Resource: resource,
		Review: client.PreviewReview{
			PlanDigest: planDigest, SpecDigest: specDigest,
		},
	})
}

func (h *referenceHost) handleApply(w http.ResponseWriter, request *http.Request, name string) {
	raw, err := io.ReadAll(io.LimitReader(request.Body, 8*1024*1024+1))
	if err != nil || len(raw) > 8*1024*1024 {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "invalid apply body")
		return
	}
	if request.Header.Get(PlanBindingProbeHeader) != "" {
		h.handlePlanBindingProbe(w, raw)
		return
	}
	var body applyRequest
	if err := decodeStrictBytes(raw, &body); err != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, err.Error())
		return
	}
	if h.tryReplay(w, request, raw, body.Resource.Metadata.Space) {
		return
	}
	if body.Resource.Metadata.Name != name {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "Resource name differs from request target")
		return
	}
	if err := h.validateRequestedResource(body.Resource); err != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, err.Error())
		return
	}
	if h.enforcePlanBinding && !h.reviewedPlanBinds(body.Review.PlanDigest, body.Resource) {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "reviewed plan does not bind this exact Resource")
		return
	}
	if !h.connectionResolutionSatisfied(body.Resource) {
		writeReferenceError(w, http.StatusNotFound, "resource_not_found", false, "portable Connection target is absent in the source Resource Space")
		return
	}
	version, status, ok := h.mutationFence(
		request,
		body.Metadata.Space,
		body.Kind,
		name,
		body.Metadata.ResourceVersion,
	)
	if !ok {
		writeReferenceError(w, http.StatusPreconditionFailed, "resource_version_conflict", false, "apply generation fence failed")
		return
	}
	resource := h.readyResource(body.Resource, version, false)
	h.resources[h.referenceResourceKey(body.Metadata.Space, body.Kind, name)] = &resource
	response := encodeReferenceJSON(resource)
	etag := `"` + strconv.Itoa(version) + `"`
	writeReferenceRaw(w, status, etag, response)
	h.recordReplay(request, raw, body.Resource.Metadata.Space, status, etag, response)
}

// handlePlanBindingProbe is disposable adapter instrumentation. Authentication
// has already run in ServeHTTP. The candidate is passed to the same exact
// comparison used by normal apply, but this branch never executes a mutation
// fence, writes Resource state, or records an idempotency replay—even when the
// binding is deliberately disabled by an adversarial test.
func (h *referenceHost) handlePlanBindingProbe(w http.ResponseWriter, raw []byte) {
	planDigest, canonicalResource, err := canonicalInstrumentedApply(raw)
	if err != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "invalid instrumented apply body")
		return
	}
	if h.enforcePlanBinding && !h.reviewedPlanDigestBinds(planDigest, canonicalResource) {
		w.Header().Set(PlanBindingProbeResultHeader, PlanBindingProbeResultRejected)
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "reviewed plan does not bind this exact Resource")
		return
	}
	w.Header().Set(PlanBindingProbeResultHeader, PlanBindingProbeResultAcceptedNoMutation)
	w.WriteHeader(http.StatusNoContent)
}

func (h *referenceHost) reviewedPlanBinds(planDigest string, resource client.Resource) bool {
	canonicalResource, err := canonicalResourceBytes(resource)
	return err == nil && h.reviewedPlanDigestBinds(planDigest, canonicalResource)
}

func (h *referenceHost) reviewedPlanDigestBinds(planDigest string, canonicalResource []byte) bool {
	return bytes.Equal(h.previews[planDigest], canonicalResource)
}

func canonicalInstrumentedApply(raw []byte) (string, []byte, error) {
	if _, err := formpackage.DigestCanonicalJSON(raw); err != nil {
		return "", nil, err
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return "", nil, err
	}
	reviewRaw, ok := document["review"]
	if !ok {
		return "", nil, errors.New("review is required")
	}
	var review client.DeploymentReview
	if err := decodeStrictBytes(reviewRaw, &review); err != nil ||
		strings.TrimSpace(review.PlanDigest) == "" {
		return "", nil, errors.New("review.planDigest is required")
	}
	delete(document, "review")
	resourceRaw, err := json.Marshal(document)
	if err != nil {
		return "", nil, err
	}
	resourceDigest, err := formpackage.DigestCanonicalJSON(resourceRaw)
	if err != nil {
		return "", nil, err
	}
	return review.PlanDigest, []byte(resourceDigest), nil
}

func (h *referenceHost) connectionResolutionSatisfied(resource client.Resource) bool {
	probe := h.contract.RunnerInput.ConnectionProbe
	if resource.Kind != probe.SourceIdentity.FormRef.Kind {
		return true
	}
	if h.resources[h.referenceResourceKey(
		resource.Metadata.Space,
		probe.TargetKind,
		probe.TargetName,
	)] != nil {
		return true
	}
	if !h.resolveConnectionsAcrossSpaces {
		return false
	}
	for _, candidate := range h.resources {
		if candidate.Kind == probe.TargetKind && candidate.Metadata.Name == probe.TargetName {
			return true
		}
	}
	return false
}

func (h *referenceHost) handleRead(w http.ResponseWriter, request *http.Request, name string) {
	resource, ok := h.exactCurrentResource(w, request, name)
	if !ok {
		return
	}
	writeReferenceRaw(w, http.StatusOK, `"`+resource.Metadata.ResourceVersion+`"`, encodeReferenceJSON(resource))
}

func (h *referenceHost) handleObserveOrRefresh(
	w http.ResponseWriter,
	request *http.Request,
	name,
	operation string,
) {
	raw, err := io.ReadAll(io.LimitReader(request.Body, 8*1024*1024+1))
	if err != nil || len(raw) != 0 {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, operation+" body must be empty")
		return
	}
	if h.tryReplay(w, request, raw, request.URL.Query().Get("space")) {
		return
	}
	resource, ok := h.exactCurrentResource(w, request, name)
	if !ok {
		return
	}
	if request.Header.Get("If-Match") != `"`+resource.Metadata.ResourceVersion+`"` {
		writeReferenceError(w, http.StatusPreconditionFailed, "resource_version_conflict", false, operation+" generation fence failed")
		return
	}
	response := encodeReferenceJSON(resourceEnvelope{Resource: resource})
	etag := `"` + resource.Metadata.ResourceVersion + `"`
	writeReferenceRaw(w, http.StatusOK, etag, response)
	h.recordReplay(request, raw, request.URL.Query().Get("space"), http.StatusOK, etag, response)
}

func (h *referenceHost) handleImport(w http.ResponseWriter, request *http.Request, name string) {
	raw, err := io.ReadAll(io.LimitReader(request.Body, 8*1024*1024+1))
	if err != nil || len(raw) > 8*1024*1024 {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "invalid import body")
		return
	}
	var body importRequest
	if err := decodeStrictBytes(raw, &body); err != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, err.Error())
		return
	}
	if h.tryReplay(w, request, raw, body.Resource.Metadata.Space) {
		return
	}
	if body.Resource.Metadata.Name != name {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "Resource name differs from request target")
		return
	}
	if strings.TrimSpace(body.NativeID) == "" {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "nativeId is required")
		return
	}
	if err := h.validateRequestedResource(body.Resource); err != nil {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, err.Error())
		return
	}
	version, status, ok := h.mutationFence(
		request,
		body.Metadata.Space,
		body.Kind,
		name,
		body.Metadata.ResourceVersion,
	)
	if !ok {
		writeReferenceError(w, http.StatusPreconditionFailed, "resource_version_conflict", false, "import generation fence failed")
		return
	}
	resource := h.readyResource(body.Resource, version, true)
	h.resources[h.referenceResourceKey(body.Metadata.Space, body.Kind, name)] = &resource
	response := encodeReferenceJSON(resourceEnvelope{Resource: resource})
	etag := `"` + strconv.Itoa(version) + `"`
	writeReferenceRaw(w, status, etag, response)
	h.recordReplay(request, raw, body.Resource.Metadata.Space, status, etag, response)
}

func (h *referenceHost) handleDelete(w http.ResponseWriter, request *http.Request, name string) {
	raw, err := io.ReadAll(io.LimitReader(request.Body, 1))
	if err != nil || len(raw) != 0 {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "delete body must be empty")
		return
	}
	if h.tryReplay(w, request, raw, request.URL.Query().Get("space")) {
		return
	}
	resource, ok := h.exactCurrentResource(w, request, name)
	if !ok {
		return
	}
	if request.Header.Get("If-Match") != `"`+resource.Metadata.ResourceVersion+`"` {
		writeReferenceError(w, http.StatusPreconditionFailed, "resource_version_conflict", false, "delete generation fence failed")
		return
	}
	delete(h.resources, h.referenceResourceKey(resource.Metadata.Space, resource.Kind, name))
	writeReferenceRaw(w, http.StatusNoContent, "", nil)
	h.recordReplay(request, raw, request.URL.Query().Get("space"), http.StatusNoContent, "", nil)
}

func (h *referenceHost) mutationFence(
	request *http.Request,
	space,
	kind,
	name,
	bodyVersion string,
) (version int, status int, ok bool) {
	resource := h.resources[h.referenceResourceKey(space, kind, name)]
	if resource == nil {
		return 1, http.StatusCreated, bodyVersion == "" && request.Header.Get("If-None-Match") == "*"
	}
	currentVersion := resource.Metadata.ResourceVersion
	if bodyVersion != currentVersion || request.Header.Get("If-Match") != `"`+currentVersion+`"` {
		return 0, 0, false
	}
	current, err := strconv.Atoi(currentVersion)
	if err != nil {
		return 0, 0, false
	}
	return current + 1, http.StatusOK, true
}

func (h *referenceHost) previewFence(request *http.Request, space, kind, name, bodyVersion string) bool {
	if bodyVersion == "" {
		return request.Header.Get("If-None-Match") == "*"
	}
	resource := h.resources[h.referenceResourceKey(space, kind, name)]
	return resource != nil && resource.Metadata.ResourceVersion == bodyVersion &&
		request.Header.Get("If-Match") == `"`+bodyVersion+`"`
}

func (h *referenceHost) exactCurrentResource(
	w http.ResponseWriter,
	request *http.Request,
	name string,
) (client.Resource, bool) {
	if !h.exactQuery(request.URL.Query()) {
		writeReferenceError(w, http.StatusConflict, "form_identity_conflict", false, "exact Form identity differs")
		return client.Resource{}, false
	}
	resource := h.resources[h.referenceResourceKey(
		request.URL.Query().Get("space"),
		request.URL.Query().Get("kind"),
		name,
	)]
	if resource == nil {
		writeReferenceError(w, http.StatusNotFound, "resource_not_found", false, "Resource is absent")
		return client.Resource{}, false
	}
	return *resource, true
}

func (h *referenceHost) exactQuery(query url.Values) bool {
	_, ok := h.referenceIdentityForExactQuery(query)
	return ok
}

func (h *referenceHost) referenceIdentityForExactQuery(
	query url.Values,
) (client.InstalledFormReference, bool) {
	space := query.Get("space")
	if space != h.contract.RunnerInput.Space && space != h.contract.RunnerInput.AlternateSpace {
		return client.InstalledFormReference{}, false
	}
	identity, ok := h.referenceIdentityForKind(query.Get("kind"))
	if !ok ||
		query.Get("apiVersion") != identity.FormRef.APIVersion ||
		query.Get("definitionVersion") != identity.FormRef.DefinitionVersion ||
		query.Get("schemaDigest") != identity.FormRef.SchemaDigest ||
		query.Get("packageDigest") != identity.PackageDigest {
		return client.InstalledFormReference{}, false
	}
	return identity, true
}

func (h *referenceHost) referenceIdentityForKind(
	kind string,
) (client.InstalledFormReference, bool) {
	for _, identity := range []InstalledFormReference{
		h.contract.RunnerInput.Identity,
		h.contract.RunnerInput.ConnectionProbe.SourceIdentity,
	} {
		if kind == identity.FormRef.Kind {
			return toClientIdentity(identity), true
		}
	}
	return client.InstalledFormReference{}, false
}

func (h *referenceHost) validateRequestedResource(resource client.Resource) error {
	identity, knownKind := h.referenceIdentityForKind(resource.Kind)
	if resource.APIVersion != h.contract.APIVersion || !knownKind ||
		resource.Form == nil || !reflect.DeepEqual(*resource.Form, identity) ||
		(resource.Metadata.Space != h.contract.RunnerInput.Space &&
			resource.Metadata.Space != h.contract.RunnerInput.AlternateSpace) {
		return fmt.Errorf("Resource or exact Form identity differs")
	}
	if err := client.ValidateResourceName(resource.Metadata.Name); err != nil {
		return fmt.Errorf("Resource metadata.name: %w", err)
	}
	if err := client.ValidateSpaceID(resource.Metadata.Space); err != nil {
		return fmt.Errorf("Resource metadata.space: %w", err)
	}
	if resource.Metadata.ResourceVersion != "" {
		value, err := strconv.ParseUint(resource.Metadata.ResourceVersion, 10, 64)
		if err != nil || resource.Metadata.ResourceVersion[0] == '0' || value > 9223372036854775807 {
			return fmt.Errorf("resourceVersion is not a positive decimal generation")
		}
	}
	if resource.Status != nil {
		return fmt.Errorf("request Resource must not contain status")
	}
	if resource.Kind == h.contract.RunnerInput.Identity.FormRef.Kind {
		return validateObjectBucketDesired(resource.Spec, resource.Metadata.Name)
	}
	probe := h.contract.RunnerInput.ConnectionProbe
	exactDesired := sameReferenceCanonicalJSON(resource.Spec, probe.Desired)
	if resource.Kind == probe.SourceIdentity.FormRef.Kind &&
		resource.Metadata.Name == probe.SourceName &&
		exactDesired {
		return nil
	}
	return fmt.Errorf(
		"Connection probe Resource differs (kind=%q name=%q exactDesired=%t)",
		resource.Kind,
		resource.Metadata.Name,
		exactDesired,
	)
}

func sameReferenceCanonicalJSON(left, right any) bool {
	leftDigest, leftErr := digestJSON(left)
	rightDigest, rightErr := digestJSON(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func validateObjectBucketDesired(spec map[string]any, name string) error {
	if spec == nil || spec["name"] != name {
		return fmt.Errorf("desired name differs")
	}
	allowed := map[string]bool{"name": true, "storageClass": true, "versioning": true, "accessProtocols": true}
	for key := range spec {
		if !allowed[key] {
			return fmt.Errorf("undeclared desired key %q", key)
		}
	}
	if value, ok := spec["storageClass"]; ok {
		token, ok := value.(string)
		if !ok || (token != "standard" && token != "infrequent_access" && token != "archive") {
			return fmt.Errorf("invalid storageClass")
		}
	}
	if value, ok := spec["versioning"]; ok {
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("versioning is not boolean")
		}
	}
	if value, ok := spec["accessProtocols"]; ok {
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("accessProtocols is not an array")
		}
		seen := map[string]bool{}
		for _, item := range items {
			token, ok := item.(string)
			if !ok || !referenceCapabilityToken.MatchString(token) || seen[token] {
				return fmt.Errorf("invalid accessProtocols")
			}
			seen[token] = true
		}
	}
	return nil
}

func (h *referenceHost) readyResource(request client.Resource, generation int, imported bool) client.Resource {
	resource := request
	resource.Metadata.ResourceVersion = strconv.Itoa(generation)
	id := resource.Kind + "/" + resource.Metadata.Name
	portability := "portable"
	resource.Status = &client.Status{
		Observed: map[string]any{
			"id": id, "generation": generation, "ready": true,
			"imported": imported, "portability": portability, "driftedFields": []any{},
		},
		Output: map[string]any{
			"id": id, "kind": resource.Kind, "name": resource.Metadata.Name,
			"generation": generation, "portability": portability,
		},
	}
	if h.invalidObservedProjection {
		resource.Status.Observed["backend"] = "reference-only-leak"
	}
	if h.mismatchedPortability {
		resource.Status.Output["portability"] = "native"
	}
	return resource
}

func (h *referenceHost) visibleInterfaces(space string) []client.DeclaredInterface {
	if h.leakInterfacesAcrossSpaces && space != h.contract.RunnerInput.Space {
		space = h.contract.RunnerInput.Space
	}
	resources := make([]*client.Resource, 0, len(h.resources))
	for _, resource := range h.resources {
		if resource.Metadata.Space == space &&
			resource.Status != nil &&
			resource.Status.Observed["ready"] == true {
			resources = append(resources, resource)
		}
	}
	sort.Slice(resources, func(left, right int) bool {
		return resources[left].Metadata.Name < resources[right].Metadata.Name
	})
	if h.exposeInterfaceBeforeReady && len(resources) == 0 && space == h.contract.RunnerInput.Space {
		exposed := client.Resource{
			APIVersion: h.contract.APIVersion,
			Kind:       h.contract.RunnerInput.Identity.FormRef.Kind,
			Metadata: client.Metadata{
				Name:  h.contract.RunnerInput.Name,
				Space: h.contract.RunnerInput.Space,
			},
			Spec: cloneMap(h.contract.RunnerInput.Desired),
		}
		ready := h.readyResource(exposed, 1, false)
		resources = append(resources, &ready)
	}
	declarations := make([]client.DeclaredInterface, 0, len(resources))
	for _, resource := range resources {
		form := toClientIdentity(h.contract.RunnerInput.Identity)
		declaration := client.DeclaredInterface{
			Name: "object.storage", Version: "1",
			Resource: client.InterfaceResourceRef{Kind: resource.Kind, Name: resource.Metadata.Name},
			Document: map[string]any{"operations": []any{"delete", "get", "list", "put"}},
			Values: map[string]any{
				"resource": resource.Kind + "/" + resource.Metadata.Name,
				"name":     resource.Metadata.Name,
			},
			Form: &form,
		}
		if h.omitInterfaceForm {
			declaration.Form = nil
		}
		if h.inventInterfaceResourceURI {
			declaration.ResourceURI = "https://invented.example/object-bucket"
		}
		if h.projectInterfaceAuthority {
			declaration.Values["authorization"] = "not-portable"
		}
		declarations = append(declarations, declaration)
	}
	return declarations
}

func (h *referenceHost) referenceResourceKey(space, kind, name string) string {
	if !h.scopeResourcesToSpace {
		return kind + "\x00" + name
	}
	return space + "\x00" + kind + "\x00" + name
}

func (h *referenceHost) tryReplay(w http.ResponseWriter, request *http.Request, raw []byte, space string) bool {
	key := request.Header.Get("Idempotency-Key")
	if key == "" {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "Idempotency-Key is required")
		return true
	}
	if !h.enforceIdempotentReplay {
		return false
	}
	recorded, ok := h.replays[h.replayKey(request, space, key)]
	if !ok {
		return false
	}
	if recorded.Fingerprint != requestFingerprint(request, raw) {
		writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "Idempotency-Key was reused for another request")
		return true
	}
	writeReferenceRaw(w, recorded.Status, recorded.ETag, recorded.Body)
	return true
}

func (h *referenceHost) recordReplay(
	request *http.Request,
	raw []byte,
	space string,
	status int,
	etag string,
	body []byte,
) {
	if !h.enforceIdempotentReplay {
		return
	}
	key := request.Header.Get("Idempotency-Key")
	h.replays[h.replayKey(request, space, key)] = recordedReplay{
		Fingerprint: requestFingerprint(request, raw),
		Status:      status, ETag: etag, Body: append([]byte(nil), body...),
	}
}

type referenceAuthContext struct {
	Tenant    string
	Principal string
}

func (h *referenceHost) authorizeRequest(w http.ResponseWriter, request *http.Request) bool {
	if probe := request.Header.Get(AuthorizationProbeHeader); probe != "" {
		switch probe {
		case AuthorizationProbeCredentialRevoked:
			if h.reauthorizeReplayAuthentication {
				writeReferenceError(w, http.StatusUnauthorized, "unauthenticated", false, "current credential is revoked")
				return false
			}
		case AuthorizationProbePermissionRevoked:
			if h.reauthorizeReplayPermission {
				writeReferenceError(w, http.StatusForbidden, "permission_denied", false, "current permission is revoked")
				return false
			}
		case AuthorizationProbePolicyRevoked:
			if h.reauthorizeReplayPolicy {
				writeReferenceError(w, http.StatusForbidden, "policy_denied", false, "current policy denies the request")
				return false
			}
		default:
			writeReferenceError(w, http.StatusBadRequest, "invalid_argument", false, "unknown authorization probe")
			return false
		}
	}
	if _, ok := referenceRequestAuth(request); !ok {
		writeReferenceError(w, http.StatusUnauthorized, "unauthenticated", false, "valid bearer authentication is required")
		return false
	}
	return true
}

func (h *referenceHost) replayKey(request *http.Request, space, key string) string {
	auth, _ := referenceRequestAuth(request)
	parts := make([]string, 0, 4)
	if h.scopeReplayToTenant {
		parts = append(parts, "tenant="+auth.Tenant)
	}
	if h.scopeReplayToPrincipal {
		parts = append(parts, "principal="+auth.Principal)
	}
	if h.scopeReplayToSpace {
		parts = append(parts, "space="+space)
	}
	parts = append(parts, "key="+key)
	return strings.Join(parts, "\x00")
}

func referenceRequestAuth(request *http.Request) (referenceAuthContext, bool) {
	const prefix = "Bearer "
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) {
		return referenceAuthContext{}, false
	}
	switch strings.TrimPrefix(authorization, prefix) {
	case referencePrimaryToken:
		return referenceAuthContext{Tenant: "reference-tenant", Principal: "reference-primary"}, true
	case referenceAlternateToken:
		return referenceAuthContext{Tenant: "reference-tenant", Principal: "reference-alternate"}, true
	case referenceAlternateTenantToken:
		return referenceAuthContext{Tenant: "reference-other-tenant", Principal: "reference-primary"}, true
	default:
		return referenceAuthContext{}, false
	}
}

func requestFingerprint(request *http.Request, raw []byte) string {
	return digestBytes([]byte(strings.Join([]string{
		request.Method,
		request.URL.RequestURI(),
		request.Header.Get("If-Match"),
		request.Header.Get("If-None-Match"),
		string(raw),
	}, "\x00")))
}

func canonicalResourceBytes(resource client.Resource) ([]byte, error) {
	raw, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		return nil, err
	}
	return []byte(digest), nil
}

func decodeReferenceBody(request *http.Request, target any) error {
	raw, err := io.ReadAll(io.LimitReader(request.Body, 8*1024*1024+1))
	if err != nil {
		return err
	}
	if len(raw) > 8*1024*1024 {
		return fmt.Errorf("request exceeded 8 MiB")
	}
	return decodeStrictBytes(raw, target)
}

func encodeReferenceJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return append(raw, '\n')
}

func writeReferenceJSON(w http.ResponseWriter, status int, etag string, value any) {
	writeReferenceRaw(w, status, etag, encodeReferenceJSON(value))
}

func writeReferenceRaw(w http.ResponseWriter, status int, etag string, raw []byte) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	w.WriteHeader(status)
	if len(raw) > 0 {
		_, _ = w.Write(raw)
	}
}

func writeReferenceError(w http.ResponseWriter, status int, code string, retryable bool, message string) {
	writeReferenceJSON(w, status, "", map[string]any{
		"error": map[string]any{
			"code": code, "message": message,
			"requestId": "reference-" + code, "retryable": retryable,
		},
	})
}
