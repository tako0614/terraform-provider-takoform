package portableconformancev3

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	referencePrimaryToken         = "reference-primary-token"
	referenceAlternateToken       = "reference-alternate-token"
	referenceAlternateTenantToken = "reference-alternate-tenant-token"

	// ErrorProbeHeader is runner-harness instrumentation, not part of the
	// portable host API: a disposable conformance endpoint uses it to make
	// every stable error deterministic. Production endpoints must not
	// implement it.
	ErrorProbeHeader = "Takoform-Conformance-Probe"
	// ProbeAsync asks the disposable host to take the 202 Operation path for
	// one apply or delete.
	ProbeAsync = "async"
	// ProbeTouchStatus asks the disposable host to perform one host-side
	// status touch during observe, so the runner can prove that revision
	// advances while generation does not.
	ProbeTouchStatus = "touch-status"
	// ProbeErrorPrefix carries one stable error code to return verbatim,
	// e.g. "error:permission_denied".
	ProbeErrorPrefix = "error:"

	expectedGenerationHeader = "Takoform-Expected-Generation"
	operationAPIVersion      = "operations.takoform.com/v1alpha1"
	supportAPIVersion        = "support.takoform.com/v1alpha1"
	artifactAPIVersion       = "artifacts.takoform.com/v1alpha1"
	fixedTransitionTime      = "2026-08-06T00:00:00Z"
	maxBodyBytes             = 8 * 1024 * 1024

	// asyncOperationPolls is how many operation reads a 202 mutation stays
	// pending for before the reference host completes it.
	asyncOperationPolls = 2

	// edgeFormsGroup is the one namespaced group whose cross-resource
	// semantics this reference host enforces.
	edgeFormsGroup = "edge.forms.takoform.com/v1alpha1"
	// workerDeploymentWeightTotal is the exact basis-point sum a
	// WorkerDeployment versions[] must carry. A desired-state schema cannot
	// add numbers, so the sum is host-validated semantics.
	workerDeploymentWeightTotal = 10000
	// maximumBundleBytes is the published module-size ceiling of the
	// WorkerVersion support profile. The artifact manifest validator enforces
	// exactly the limit the support surface advertises.
	maximumBundleBytes = 10485760
	// sourceMapMediaType names a module that is source-map evidence for
	// another declared module rather than executable code.
	sourceMapMediaType = "application/source-map+json"
	// sourceMapSuffix is the portable naming rule that binds a source map to
	// its target module: "<module>.map" describes "<module>".
	sourceMapSuffix = ".map"
)

var artifactPathPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*(?:/[A-Za-z0-9_][A-Za-z0-9._-]*)*$`)

var artifactModuleMediaTypes = map[string]bool{
	"application/javascript+module": true,
	"application/wasm":              true,
	"text/plain":                    true,
	"application/octet-stream":      true,
	"application/source-map+json":   true,
}

// hostError is one closed stable error outcome.
type hostError struct {
	Code    string
	Message string
}

func stableError(code, message string) *hostError { return &hostError{Code: code, Message: message} }

type storedResource struct {
	Group, Kind, Name, Space string
	UID                      string
	Generation               int64
	Revision                 int64
	Spec                     map[string]any
	SpecDigest               string
	StatusTouches            int64
	Imported                 bool
	PackageDigest            string
}

type recordedReplay struct {
	Fingerprint string
	Status      int
	ETag        string
	Body        []byte
}

type artifactUpload struct {
	ID             string
	ManifestRaw    []byte
	Manifest       artifactManifest
	ManifestDigest string
}

type artifactManifest struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	MainModule string           `json:"mainModule,omitempty"`
	Modules    []artifactModule `json:"modules,omitempty"`
	Files      []artifactFile   `json:"files,omitempty"`
}

type artifactModule struct {
	Name      string      `json:"name"`
	MediaType string      `json:"mediaType"`
	Size      json.Number `json:"size"`
	Digest    string      `json:"digest"`
}

type artifactFile struct {
	Path      string      `json:"path"`
	MediaType string      `json:"mediaType"`
	Size      json.Number `json:"size"`
	Digest    string      `json:"digest"`
}

type hostOperation struct {
	ID             string
	Done           bool
	PollsRemaining int
	commit         func() (map[string]any, *hostError)
	terminalBody   []byte
}

// ReferenceHost is a deliberately small, deterministic v1alpha3 host used to
// prove the runner over real HTTP. It is not a reusable host and its reports
// are never publication-ready.
type ReferenceHost struct {
	mu sync.Mutex

	contract Contract
	catalog  *Catalog

	resources      map[string]*storedResource
	prepares       map[string][]byte
	replays        map[string]recordedReplay
	uploads        map[string]*artifactUpload
	blobs          map[string][]byte
	manifests      map[string][]byte
	operations     map[string]*hostOperation
	uidCounter     int
	opCounter      int
	uploadCounter  int
	requestCounter int
}

// NewReferenceHost constructs the deterministic reference host over the
// verified contract and an installed catalog.
func NewReferenceHost(contract Contract, catalog *Catalog) *ReferenceHost {
	return &ReferenceHost{
		contract:   contract,
		catalog:    catalog,
		resources:  map[string]*storedResource{},
		prepares:   map[string][]byte{},
		replays:    map[string]recordedReplay{},
		uploads:    map[string]*artifactUpload{},
		blobs:      map[string][]byte{},
		manifests:  map[string][]byte{},
		operations: map[string]*hostOperation{},
	}
}

func resourceKey(space, group, kind, name string) string {
	return strings.Join([]string{space, group, kind, name}, "\x00")
}

type hostAuthContext struct {
	Tenant    string
	Principal string
}

func hostRequestAuth(request *http.Request) (hostAuthContext, bool) {
	const prefix = "Bearer "
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) {
		return hostAuthContext{}, false
	}
	switch strings.TrimPrefix(authorization, prefix) {
	case referencePrimaryToken:
		return hostAuthContext{Tenant: "reference-tenant", Principal: "reference-primary"}, true
	case referenceAlternateToken:
		return hostAuthContext{Tenant: "reference-tenant", Principal: "reference-alternate"}, true
	case referenceAlternateTenantToken:
		return hostAuthContext{Tenant: "reference-other-tenant", Principal: "reference-primary"}, true
	default:
		return hostAuthContext{}, false
	}
}

func (h *ReferenceHost) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	if _, ok := hostRequestAuth(request); !ok {
		h.writeError(w, "unauthenticated", "valid bearer authentication is required")
		return
	}
	probe := request.Header.Get(ErrorProbeHeader)
	if strings.HasPrefix(probe, ProbeErrorPrefix) {
		code := strings.TrimPrefix(probe, ProbeErrorPrefix)
		if _, known := stableErrorHTTPStatusByCode[code]; !known {
			h.writeError(w, "invalid_argument", "unknown conformance error probe")
			return
		}
		h.writeError(w, code, "deterministic conformance error probe")
		return
	}
	if probe != "" && probe != ProbeAsync && probe != ProbeTouchStatus {
		h.writeError(w, "invalid_argument", "unknown conformance probe")
		return
	}

	path := request.URL.EscapedPath()
	if path == h.contract.DiscoveryPath && request.Method == http.MethodGet {
		h.handleDiscovery(w, request)
		return
	}
	prefix := h.contract.APIPath + "/"
	if !strings.HasPrefix(path, prefix) {
		h.writeError(w, "resource_not_found", "unknown route")
		return
	}
	rawParts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	parts := make([]string, len(rawParts))
	for index, rawPart := range rawParts {
		part, err := url.PathUnescape(rawPart)
		if err != nil {
			h.writeError(w, "invalid_argument", "invalid percent-encoding in request path")
			return
		}
		parts[index] = part
	}
	switch {
	case parts[0] == "forms" && len(parts) == 1 && request.Method == http.MethodGet:
		h.handleForms(w, request)
	case parts[0] == "form-definitions" && len(parts) == 3 && request.Method == http.MethodGet:
		h.handleFormDefinition(w, request, parts[1], parts[2])
	case parts[0] == "resources" && len(parts) == 2 && parts[1] == "validate" && request.Method == http.MethodPost:
		h.handleValidate(w, request)
	case parts[0] == "resources" && len(parts) == 2 && parts[1] == "prepare" && request.Method == http.MethodPost:
		h.handlePrepare(w, request)
	case parts[0] == "resources" && len(parts) == 4:
		group, kind, name := parts[1], parts[2], parts[3]
		switch request.Method {
		case http.MethodPut:
			h.handleApply(w, request, group, kind, name)
		case http.MethodGet:
			h.handleRead(w, request, group, kind, name)
		case http.MethodDelete:
			h.handleDelete(w, request, group, kind, name)
		default:
			h.writeError(w, "resource_not_found", "unknown operation")
		}
	case parts[0] == "resources" && len(parts) == 5 && request.Method == http.MethodPost:
		group, kind, name, action := parts[1], parts[2], parts[3], parts[4]
		switch action {
		// There is no refresh action in the v1alpha3 lane: observe is the one
		// fenced read-only re-observation. A request for /refresh is an unknown
		// route, exactly like any other operation the lane does not define.
		case "observe":
			h.handleObserve(w, request, group, kind, name)
		case "import":
			h.handleImport(w, request, group, kind, name)
		default:
			h.writeError(w, "resource_not_found", "unknown operation")
		}
	case parts[0] == "operations" && len(parts) == 2 && request.Method == http.MethodGet:
		h.handleOperationGet(w, parts[1])
	case parts[0] == "operations" && len(parts) == 3 && parts[2] == "cancel" && request.Method == http.MethodPost:
		h.handleOperationCancel(w, request, parts[1])
	case parts[0] == "artifacts":
		h.routeArtifacts(w, request, parts)
	case parts[0] == "support":
		h.routeSupport(w, request, parts)
	default:
		h.writeError(w, "resource_not_found", "unknown route")
	}
}

func (h *ReferenceHost) handleDiscovery(w http.ResponseWriter, request *http.Request) {
	origin := "http://" + request.Host
	h.writeJSON(w, http.StatusOK, "", map[string]any{
		"api_versions": []string{h.contract.APIVersion},
		"features": map[string]bool{
			"service_forms":          true,
			"exact_form_ref":         true,
			"optimistic_concurrency": true,
			"idempotent_lifecycle":   true,
			"operations":             true,
			"artifact_upload":        true,
			"support_profiles":       true,
		},
		"endpoints": map[string]string{
			"api":        origin + h.contract.APIPath,
			"artifacts":  origin + h.contract.APIPath + "/artifacts",
			"operations": origin + h.contract.APIPath + "/operations",
			"support":    origin + h.contract.APIPath + "/support",
		},
	})
}

// exactQueryForm resolves the closed exact-FormRef query vocabulary. The
// packageDigest deliberately has no query key: audit evidence never enters
// identity, so an unknown key (including packageDigest) fails closed.
func (h *ReferenceHost) exactQueryForm(query url.Values) (*installedForm, string, *hostError) {
	allowed := map[string]bool{
		"space": true, "group": true, "kind": true,
		"definitionVersion": true, "schemaDigest": true,
	}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 {
			return nil, "", stableError("invalid_argument", "exact Form query vocabulary is closed")
		}
	}
	space := query.Get("space")
	if !validSpaceID(space) {
		return nil, "", stableError("invalid_argument", "exactly one valid space is required")
	}
	form := h.catalog.form(query.Get("group"), query.Get("kind"))
	if form == nil ||
		query.Get("definitionVersion") != form.Ref.DefinitionVersion ||
		query.Get("schemaDigest") != form.Ref.SchemaDigest {
		return nil, "", stableError("form_unknown", "exact Form is unknown")
	}
	return form, space, nil
}

func (h *ReferenceHost) handleForms(w http.ResponseWriter, request *http.Request) {
	form, _, hostErr := h.exactQueryForm(request.URL.Query())
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	h.writeJSON(w, http.StatusOK, "", map[string]any{
		"forms": []map[string]any{{
			"identity":             h.identityJSON(form),
			"definitionKnown":      true,
			"installed":            true,
			"executable":           true,
			"activated":            true,
			"availableToPrincipal": true,
			"operations":           form.operations(),
		}},
	})
}

func (h *ReferenceHost) handleFormDefinition(w http.ResponseWriter, request *http.Request, group, kind string) {
	form, _, hostErr := h.exactQueryForm(request.URL.Query())
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if form.Ref.APIVersion != group || form.Ref.Kind != kind {
		h.writeError(w, "invalid_argument", "path and exact query identities differ")
		return
	}
	response := map[string]any{
		"identity":      h.identityJSON(form),
		"displayName":   form.Title,
		"desiredSchema": form.DesiredSchema,
	}
	if form.Description != "" {
		response["description"] = form.Description
	}
	h.writeJSON(w, http.StatusOK, "", response)
}

func (h *ReferenceHost) identityJSON(form *installedForm) map[string]any {
	identity := map[string]any{"formRef": refJSON(form.Ref)}
	if form.PackageDigest != "" {
		identity["packageDigest"] = form.PackageDigest
	}
	return identity
}

func refJSON(ref FormRef) map[string]any {
	return map[string]any{
		"apiVersion":        ref.APIVersion,
		"kind":              ref.Kind,
		"definitionVersion": ref.DefinitionVersion,
		"schemaDigest":      ref.SchemaDigest,
	}
}

// Wire request shapes. Strict I-JSON decoding rejects duplicate members and
// unknown envelope fields before any mutation.
type wireFormReference struct {
	FormRef       FormRef `json:"formRef"`
	PackageDigest string  `json:"packageDigest,omitempty"`
}

type wireMetadata struct {
	Name       string `json:"name"`
	Space      string `json:"space"`
	UID        string `json:"uid,omitempty"`
	Generation string `json:"generation,omitempty"`
	Revision   string `json:"revision,omitempty"`
}

type resourceWire struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Form       *wireFormReference `json:"form"`
	Metadata   wireMetadata       `json:"metadata"`
	Spec       map[string]any     `json:"spec"`
}

type reviewWire struct {
	PrepareDigest string `json:"prepareDigest"`
}

type applyWire struct {
	resourceWire
	Review             *reviewWire `json:"review"`
	ExpectedUID        string      `json:"expectedUid,omitempty"`
	ExpectedGeneration string      `json:"expectedGeneration,omitempty"`
}

type importWire struct {
	resourceWire
	NativeID string `json:"nativeId"`
}

func (h *ReferenceHost) readBody(request *http.Request) ([]byte, *hostError) {
	raw, err := io.ReadAll(io.LimitReader(request.Body, maxBodyBytes+1))
	if err != nil || len(raw) > maxBodyBytes {
		return nil, stableError("invalid_argument", "invalid request body")
	}
	return raw, nil
}

// resolveResourceWire enforces the request-side identity contract shared by
// validate, prepare, apply, and import bodies.
func (h *ReferenceHost) resolveResourceWire(body *resourceWire) (*installedForm, *hostError) {
	if body.Form == nil {
		return nil, stableError("invalid_argument", "an exact FormRef is required")
	}
	form := h.catalog.form(body.Form.FormRef.APIVersion, body.Form.FormRef.Kind)
	if form == nil || form.Ref != body.Form.FormRef {
		return nil, stableError("form_unknown", "exact Form is unknown")
	}
	if body.APIVersion != form.Ref.APIVersion || body.Kind != form.Ref.Kind {
		return nil, stableError("invalid_argument", "resource apiVersion/kind and exact FormRef identities differ")
	}
	// The packageDigest is audit evidence only: it must be well-formed when
	// present, but it never has to match the installed digest and never
	// changes which resource is addressed.
	if body.Form.PackageDigest != "" && !formpackage.ValidDigest(body.Form.PackageDigest) {
		return nil, stableError("invalid_argument", "form.packageDigest must be a lowercase sha256:<hex> digest")
	}
	if !resourceNamePattern.MatchString(body.Metadata.Name) {
		return nil, stableError("invalid_argument", "resource metadata.name is invalid")
	}
	if !validSpaceID(body.Metadata.Space) {
		return nil, stableError("invalid_argument", "resource metadata.space is invalid")
	}
	return form, nil
}

// validSpaceID ports $defs/spaceId of
// spec/schemas/host-api-wire-v1alpha3.schema.json into the host: a SpaceID is
// valid UTF-8 of 1..255 Unicode code points, carries no leading or trailing
// Unicode White_Space code point or U+FEFF, and contains no C0/C1 control
// character and no slash. The value is opaque and case-sensitive: the host
// never trims, normalizes, or case-folds it, so anything outside the grammar
// fails closed instead of being repaired into a different space.
func validSpaceID(space string) bool {
	if !utf8.ValidString(space) {
		return false
	}
	runes := []rune(space)
	if len(runes) < 1 || len(runes) > 255 {
		return false
	}
	if isSpaceBoundaryRune(runes[0]) || isSpaceBoundaryRune(runes[len(runes)-1]) {
		return false
	}
	for _, candidate := range runes {
		if candidate == '/' || candidate <= 0x1f || (candidate >= 0x7f && candidate <= 0x9f) {
			return false
		}
	}
	return true
}

func isSpaceBoundaryRune(candidate rune) bool {
	return unicode.IsSpace(candidate) || candidate == '\uFEFF'
}

func (h *ReferenceHost) specDiagnostics(form *installedForm, spec map[string]any) ([]map[string]any, *hostError) {
	if spec == nil {
		spec = map[string]any{}
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, stableError("invalid_argument", "spec is not encodable JSON")
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, stableError("invalid_argument", "spec is not decodable JSON")
	}
	if err := form.compiled.Validate(value); err != nil {
		return []map[string]any{{
			"severity": "error",
			"message":  "desired spec violates the installed Form Definition: " + err.Error(),
		}}, nil
	}
	return []map[string]any{}, nil
}

func (h *ReferenceHost) handleValidate(w http.ResponseWriter, request *http.Request) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	var body resourceWire
	if err := formpackage.DecodeStrictIJSON(raw, &body); err != nil {
		h.writeError(w, "invalid_argument", err.Error())
		return
	}
	form, hostErr := h.resolveResourceWire(&body)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	body.Spec = form.materialize(body.Spec)
	diagnostics, hostErr := h.specDiagnostics(form, body.Spec)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	h.writeJSON(w, http.StatusOK, "", map[string]any{
		"valid":       len(diagnostics) == 0,
		"diagnostics": diagnostics,
	})
}

// Create markers bound into a prepareDigest when the target resource does
// not exist yet: there is no host-issued uid, and generation "0" precedes
// every real generation.
const (
	prepareCreateUID        = ""
	prepareCreateGeneration = "0"
)

// prepareBindingPayload is the canonical document the prepareDigest binds:
// the exact spec digest, the exact identity, the CURRENT uid and generation
// of the target resource (create markers when it does not exist), and a host
// plan marker. The packageDigest is deliberately excluded because it is audit
// evidence, not identity.
func prepareBindingPayload(specDigest string, ref FormRef, name, space, uid, generation string) ([]byte, string, error) {
	payload := map[string]any{
		"plan":              "deterministic-reference-host-plan",
		"specDigest":        specDigest,
		"group":             ref.APIVersion,
		"kind":              ref.Kind,
		"definitionVersion": ref.DefinitionVersion,
		"schemaDigest":      ref.SchemaDigest,
		"name":              name,
		"space":             space,
		"uid":               uid,
		"generation":        generation,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return nil, "", err
	}
	return canonical, formpackage.DigestBytes(canonical), nil
}

func specCanonicalDigest(spec map[string]any) (string, error) {
	if spec == nil {
		spec = map[string]any{}
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return formpackage.DigestCanonicalJSON(raw)
}

func (h *ReferenceHost) handlePrepare(w http.ResponseWriter, request *http.Request) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	var body resourceWire
	if err := formpackage.DecodeStrictIJSON(raw, &body); err != nil {
		h.writeError(w, "invalid_argument", err.Error())
		return
	}
	form, hostErr := h.resolveResourceWire(&body)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	body.Spec = form.materialize(body.Spec)
	diagnostics, hostErr := h.specDiagnostics(form, body.Spec)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if len(diagnostics) != 0 {
		h.writeError(w, "invalid_argument", "prepare requires a valid desired spec")
		return
	}
	specDigest, err := specCanonicalDigest(body.Spec)
	if err != nil {
		h.writeError(w, "invalid_argument", "spec is not canonicalizable I-JSON")
		return
	}
	// The prepare precondition is "generation-fence-when-updating": an
	// existing target requires the exact update fence, and the CURRENT uid and
	// generation are bound into the digest so a later apply at any other
	// generation cannot reuse it. A create binds the create markers.
	fence := request.Header.Get(expectedGenerationHeader)
	current := h.resources[resourceKey(body.Metadata.Space, form.Ref.APIVersion, form.Ref.Kind, body.Metadata.Name)]
	uid, generation := prepareCreateUID, prepareCreateGeneration
	if current != nil {
		if fence == "" {
			h.writeError(w, "invalid_argument", "prepare on an existing resource requires the "+expectedGenerationHeader+" fence")
			return
		}
		fenceValue, fenceErr := strconv.ParseInt(fence, 10, 64)
		if fenceErr != nil || fence[0] == '0' || fenceValue < 1 {
			h.writeError(w, "invalid_argument", "prepare generation fence must be a canonical positive decimal")
			return
		}
		if fenceValue != current.Generation {
			h.writeError(w, "generation_conflict", "prepare generation fence is stale")
			return
		}
		uid = current.UID
		generation = fence
	} else if fence != "" {
		h.writeError(w, "resource_not_found", "prepare generation fence names an absent resource")
		return
	}
	payload, prepareDigest, err := prepareBindingPayload(specDigest, form.Ref, body.Metadata.Name, body.Metadata.Space, uid, generation)
	if err != nil {
		h.writeError(w, "internal_error", err.Error())
		return
	}
	h.prepares[prepareDigest] = payload
	echoed := map[string]any{
		"apiVersion": body.APIVersion,
		"kind":       body.Kind,
		"form":       wireFormReferenceJSON(body.Form),
		"metadata":   map[string]any{"name": body.Metadata.Name, "space": body.Metadata.Space},
		"spec":       specOrEmpty(body.Spec),
	}
	h.writeJSON(w, http.StatusOK, "", map[string]any{
		"resource": echoed,
		"review": map[string]any{
			"prepareDigest": prepareDigest,
			"specDigest":    specDigest,
		},
	})
}

func wireFormReferenceJSON(reference *wireFormReference) map[string]any {
	out := map[string]any{"formRef": refJSON(reference.FormRef)}
	if reference.PackageDigest != "" {
		out["packageDigest"] = reference.PackageDigest
	}
	return out
}

func specOrEmpty(spec map[string]any) map[string]any {
	if spec == nil {
		return map[string]any{}
	}
	return spec
}

// bindingReferences returns every typed binding target reference inside a
// desired spec: any object member named "resource" whose value is exactly
// {"kind": ..., "name": ...}.
func bindingReferences(value any) [][2]string {
	var out [][2]string
	var walk func(any)
	walk = func(candidate any) {
		switch typed := candidate.(type) {
		case map[string]any:
			for key, member := range typed {
				if key == "resource" {
					if reference, ok := member.(map[string]any); ok && len(reference) == 2 {
						kind, kindOK := reference["kind"].(string)
						name, nameOK := reference["name"].(string)
						if kindOK && nameOK {
							out = append(out, [2]string{kind, name})
							continue
						}
					}
				}
				walk(member)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return out
}

// validateDesiredSemantics enforces the portable cross-resource rules that a
// desired-state schema cannot express. Every rule runs before any mutation on
// both the apply and the import path, and is re-run at async COMMIT time so a
// 202 can never commit against a store that has since moved.
func (h *ReferenceHost) validateDesiredSemantics(
	form *installedForm,
	space string,
	spec map[string]any,
) *hostError {
	if hostErr := validateDeploymentWeightSum(form, spec); hostErr != nil {
		return hostErr
	}
	for _, reference := range bindingReferences(spec) {
		if h.resources[resourceKey(space, form.Ref.APIVersion, reference[0], reference[1])] == nil {
			return stableError("resource_not_found", "typed binding target is absent in the resource space")
		}
	}
	if form.Ref.Kind == "WorkerBundle" {
		if hostErr := h.requireBundleBlobs(spec); hostErr != nil {
			return hostErr
		}
	}
	return h.requireDeclaredHandler(form, space, spec)
}

// validateDeploymentWeightSum enforces the one WorkerDeployment rule the
// Form description calls host-validated: traffic weights are basis points and
// must sum to exactly 10000, because a JSON Schema cannot add numbers.
func validateDeploymentWeightSum(form *installedForm, spec map[string]any) *hostError {
	if form.Ref.APIVersion != edgeFormsGroup || form.Ref.Kind != "WorkerDeployment" {
		return nil
	}
	versions, _ := spec["versions"].([]any)
	if len(versions) == 0 {
		return stableError("invalid_argument", "a WorkerDeployment requires at least one weighted version")
	}
	total := int64(0)
	for _, member := range versions {
		entry, _ := member.(map[string]any)
		weight, ok := entry["weight"].(json.Number)
		if !ok {
			return stableError("invalid_argument", "WorkerDeployment version weight must be an integer")
		}
		value, err := strconv.ParseInt(weight.String(), 10, 64)
		if err != nil {
			return stableError("invalid_argument", "WorkerDeployment version weight must be an integer")
		}
		total += value
	}
	if total != workerDeploymentWeightTotal {
		return stableError(
			"invalid_argument",
			"WorkerDeployment traffic weights must sum to exactly "+
				strconv.Itoa(workerDeploymentWeightTotal)+" basis points",
		)
	}
	return nil
}

// requireDeclaredHandler gates the two cross-resource attachments: a
// WorkerCronTrigger invokes the scheduled handler and a QueueConsumer invokes
// the queue handler, so attaching one to a worker whose code declares no such
// handler is an unsupported capability, not a valid desired state. Declared
// handlers are resolved from ANY stored WorkerVersion of that worker in the
// same space and group; a stored WorkerDeployment is not required.
func (h *ReferenceHost) requireDeclaredHandler(
	form *installedForm,
	space string,
	spec map[string]any,
) *hostError {
	if form.Ref.APIVersion != edgeFormsGroup {
		return nil
	}
	var handler string
	switch form.Ref.Kind {
	case "WorkerCronTrigger":
		handler = "scheduled"
	case "QueueConsumer":
		handler = "queue"
	default:
		return nil
	}
	worker := nestedName(spec, "worker")
	if worker == "" {
		return stableError("invalid_argument", form.Ref.Kind+" requires a target worker")
	}
	for _, candidate := range h.resources {
		if candidate.Space != space || candidate.Group != form.Ref.APIVersion ||
			candidate.Kind != "WorkerVersion" || nestedName(candidate.Spec, "worker") != worker {
			continue
		}
		handlers, _ := candidate.Spec["handlers"].([]any)
		for _, declared := range handlers {
			if name, _ := declared.(string); name == handler {
				return nil
			}
		}
	}
	return stableError(
		"unsupported_capability",
		"no deployed WorkerVersion of worker "+worker+" declares the "+handler+" handler",
	)
}

// dependencyInUse is the live typed-binding scan: a resource that is the
// target of another stored resource's typed binding cannot be deleted.
func (h *ReferenceHost) dependencyInUse(resource *storedResource) *hostError {
	for _, holder := range h.resources {
		if holder.Space != resource.Space || holder.Group != resource.Group ||
			(holder.Kind == resource.Kind && holder.Name == resource.Name) {
			continue
		}
		for _, reference := range bindingReferences(holder.Spec) {
			if reference[0] == resource.Kind && reference[1] == resource.Name {
				return stableError("dependency_in_use", "resource is the target of a live typed binding")
			}
		}
	}
	return nil
}

// mutationFence carries the exact precondition headers of one mutation. The
// async path evaluates them again at commit time, so they are captured as
// values instead of read from a request that is long gone.
type mutationFence struct {
	IfNoneMatch string
	Generation  string
}

func mutationFenceOf(request *http.Request) mutationFence {
	return mutationFence{
		IfNoneMatch: request.Header.Get("If-None-Match"),
		Generation:  request.Header.Get(expectedGenerationHeader),
	}
}

// mutationFences resolves the apply/import precondition surface. It returns
// the existing resource (nil on create intent) or a stable error.
func (h *ReferenceHost) mutationFences(
	fences mutationFence,
	form *installedForm,
	space, name string,
	bodyExpectedGeneration, expectedUID string,
) (existing *storedResource, create bool, hostErr *hostError) {
	ifNoneMatch := fences.IfNoneMatch
	if ifNoneMatch != "" && ifNoneMatch != "*" {
		return nil, false, stableError("invalid_argument", "If-None-Match only supports *")
	}
	headerGeneration := fences.Generation
	current := h.resources[resourceKey(space, form.Ref.APIVersion, form.Ref.Kind, name)]
	if ifNoneMatch == "*" {
		if headerGeneration != "" || bodyExpectedGeneration != "" {
			return nil, false, stableError("invalid_argument", "create must not carry a generation fence")
		}
		if expectedUID != "" {
			return nil, false, stableError("uid_mismatch", "create cannot pin an expected uid")
		}
		if current != nil {
			return nil, false, stableError("generation_conflict", "resource already exists under If-None-Match: *")
		}
		return nil, true, nil
	}
	fence := headerGeneration
	if fence == "" {
		fence = bodyExpectedGeneration
	} else if bodyExpectedGeneration != "" && bodyExpectedGeneration != fence {
		return nil, false, stableError("invalid_argument", "generation fences in header and body differ")
	}
	if fence == "" {
		return nil, false, stableError("invalid_argument", "mutation requires If-None-Match: * or a generation fence")
	}
	fenceValue, err := strconv.ParseInt(fence, 10, 64)
	if err != nil || fence[0] == '0' || fenceValue < 1 {
		return nil, false, stableError("invalid_argument", "generation fence must be a canonical positive decimal")
	}
	if current == nil {
		return nil, false, stableError("resource_not_found", "resource is absent")
	}
	if expectedUID != "" && expectedUID != current.UID {
		return nil, false, stableError("uid_mismatch", "expectedUid does not match the host-issued uid")
	}
	if fenceValue != current.Generation {
		return nil, false, stableError("generation_conflict", "generation fence is stale")
	}
	return current, false, nil
}

func (h *ReferenceHost) handleApply(w http.ResponseWriter, request *http.Request, group, kind, name string) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	var body applyWire
	if err := formpackage.DecodeStrictIJSON(raw, &body); err != nil {
		h.writeError(w, "invalid_argument", err.Error())
		return
	}
	if h.tryReplay(w, request, raw, body.Metadata.Space) {
		return
	}
	if body.Metadata.Name != name || body.Kind != kind || body.APIVersion != group {
		h.writeError(w, "invalid_argument", "resource identity differs from the request target")
		return
	}
	form, hostErr := h.resolveResourceWire(&body.resourceWire)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	// Materialization happens HERE: before validation, before the spec digest,
	// before the applyOnce closure captures body.Spec, and before anything is
	// stored or echoed. Anywhere later and the prepare digest a client already
	// holds would not match what this apply computes.
	body.Spec = form.materialize(body.Spec)
	diagnostics, hostErr := h.specDiagnostics(form, body.Spec)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if len(diagnostics) != 0 {
		h.writeError(w, "invalid_argument", "desired spec violates the installed Form Definition")
		return
	}
	if body.Review == nil || !formpackage.ValidDigest(body.Review.PrepareDigest) {
		h.writeError(w, "invalid_argument", "apply requires review.prepareDigest")
		return
	}
	specDigest, err := specCanonicalDigest(body.Spec)
	if err != nil {
		h.writeError(w, "invalid_argument", "spec is not canonicalizable I-JSON")
		return
	}
	space := body.Metadata.Space
	fences := mutationFenceOf(request)
	prepareDigest := body.Review.PrepareDigest
	// applyOnce is the complete pre-mutation gauntlet plus the commit. The
	// synchronous path runs it inline; the 202 path runs the SAME function at
	// poll time, so an accepted operation re-derives every precondition
	// against the store as it is when the mutation actually lands.
	applyOnce := func() (*storedResource, bool, *hostError) {
		if hostErr := h.validateDesiredSemantics(form, space, body.Spec); hostErr != nil {
			return nil, false, hostErr
		}
		existing, create, hostErr := h.mutationFences(
			fences, form, space, name, body.ExpectedGeneration, body.ExpectedUID,
		)
		if hostErr != nil {
			return nil, false, hostErr
		}
		// Capability, not role, decides what an apply to an EXISTING resource
		// may do. A Form Definition that omits update advertises no in-place
		// spec change, so accepting one here would let a host silently perform
		// an operation the Definition told every client was unavailable. The
		// refusal lands before any mutation, and before the prepare binding is
		// even consulted.
		if !create && !form.declaresUpdate() && specDigest != existing.SpecDigest {
			return nil, false, stableError(
				"invalid_argument",
				"the installed Form Definition declares no update capability; a spec-changing apply is not representable",
			)
		}
		if !create && form.Role == "revision" {
			return nil, false, stableError("invalid_argument", "update to a revision-role resource is not representable")
		}
		// The expected prepare binding is recomputed from the POST-fence
		// resolved state: a prepareDigest minted for another spec, another
		// resource, or another generation fails invalid_argument BEFORE any
		// mutation.
		expectedUID, expectedGeneration := prepareCreateUID, prepareCreateGeneration
		if !create {
			expectedUID = existing.UID
			expectedGeneration = strconv.FormatInt(existing.Generation, 10)
		}
		payload, _, err := prepareBindingPayload(specDigest, form.Ref, name, space, expectedUID, expectedGeneration)
		if err != nil {
			return nil, false, stableError("internal_error", err.Error())
		}
		if !bytes.Equal(h.prepares[prepareDigest], payload) {
			return nil, false, stableError(
				"invalid_argument",
				"prepared review does not bind this exact resource at its current generation",
			)
		}
		next := h.nextResource(form, existing, create, space, name, body.Spec, specDigest, false)
		h.storeResource(next)
		return next, create, nil
	}
	if request.Header.Get(ErrorProbeHeader) == ProbeAsync {
		h.acceptOperation(w, request, raw, space, func() (map[string]any, *hostError) {
			next, _, hostErr := applyOnce()
			if hostErr != nil {
				return nil, hostErr
			}
			return map[string]any{"resource": h.renderResource(next)}, nil
		})
		return
	}
	next, create, hostErr := applyOnce()
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	status := http.StatusOK
	if create {
		status = http.StatusCreated
	}
	response := encodeJSONBody(h.renderResource(next))
	etag := quotedRevision(next.Revision)
	h.writeRaw(w, status, etag, response)
	h.recordReplay(request, raw, space, status, etag, response)
}

func (h *ReferenceHost) requireBundleBlobs(spec map[string]any) *hostError {
	modules, _ := spec["modules"].([]any)
	for _, member := range modules {
		module, _ := member.(map[string]any)
		digest, _ := module["digest"].(string)
		if digest == "" || h.blobs[digest] == nil {
			return stableError("artifact_missing", "bundle module bytes were not uploaded through the artifact API")
		}
	}
	return nil
}

// nextResource computes the post-mutation identity: uid minted on create,
// generation advanced only on canonical spec change, revision advanced on
// every representation change.
func (h *ReferenceHost) nextResource(
	form *installedForm,
	existing *storedResource,
	create bool,
	space, name string,
	spec map[string]any,
	specDigest string,
	imported bool,
) *storedResource {
	if create {
		h.uidCounter++
		return &storedResource{
			Group: form.Ref.APIVersion, Kind: form.Ref.Kind, Name: name, Space: space,
			UID:        "uid-" + strconv.Itoa(h.uidCounter),
			Generation: 1, Revision: 1,
			Spec: specOrEmpty(spec), SpecDigest: specDigest,
			Imported:      imported,
			PackageDigest: form.PackageDigest,
		}
	}
	next := *existing
	next.Spec = specOrEmpty(spec)
	next.SpecDigest = specDigest
	if specDigest != existing.SpecDigest {
		next.Generation++
		next.Revision++
	}
	return &next
}

func (h *ReferenceHost) storeResource(resource *storedResource) {
	h.resources[resourceKey(resource.Space, resource.Group, resource.Kind, resource.Name)] = resource
}

func (h *ReferenceHost) renderResource(resource *storedResource) map[string]any {
	form := h.catalog.form(resource.Group, resource.Kind)
	reference := map[string]any{"formRef": refJSON(form.Ref)}
	if resource.PackageDigest != "" {
		reference["packageDigest"] = resource.PackageDigest
	}
	status := map[string]any{
		"observedGeneration": strconv.FormatInt(resource.Generation, 10),
		"conditions": []map[string]any{{
			"type":               "Ready",
			"status":             "True",
			"reason":             "Available",
			"lastTransitionTime": fixedTransitionTime,
		}},
	}
	if resource.StatusTouches > 0 {
		status["observed"] = map[string]any{"statusTouches": resource.StatusTouches}
	}
	return map[string]any{
		"apiVersion": resource.Group,
		"kind":       resource.Kind,
		"form":       reference,
		"metadata": map[string]any{
			"name":       resource.Name,
			"space":      resource.Space,
			"uid":        resource.UID,
			"generation": strconv.FormatInt(resource.Generation, 10),
			"revision":   strconv.FormatInt(resource.Revision, 10),
		},
		"spec":   resource.Spec,
		"status": status,
	}
}

func quotedRevision(revision int64) string {
	return `"` + strconv.FormatInt(revision, 10) + `"`
}

func (h *ReferenceHost) exactCurrentResource(
	w http.ResponseWriter,
	request *http.Request,
	group, kind, name string,
) (*storedResource, bool) {
	form, space, hostErr := h.exactQueryForm(request.URL.Query())
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return nil, false
	}
	if form.Ref.APIVersion != group || form.Ref.Kind != kind {
		h.writeError(w, "invalid_argument", "path and exact query identities differ")
		return nil, false
	}
	resource := h.resources[resourceKey(space, group, kind, name)]
	if resource == nil {
		h.writeError(w, "resource_not_found", "resource is absent")
		return nil, false
	}
	return resource, true
}

func (h *ReferenceHost) handleRead(w http.ResponseWriter, request *http.Request, group, kind, name string) {
	resource, ok := h.exactCurrentResource(w, request, group, kind, name)
	if !ok {
		return
	}
	h.writeRaw(w, http.StatusOK, quotedRevision(resource.Revision), encodeJSONBody(h.renderResource(resource)))
}

func (h *ReferenceHost) handleObserve(
	w http.ResponseWriter,
	request *http.Request,
	group, kind, name string,
) {
	const action = "observe"
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if len(raw) != 0 {
		h.writeError(w, "invalid_argument", action+" body must be empty")
		return
	}
	if request.Header.Get("Idempotency-Key") == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return
	}
	if h.tryReplay(w, request, raw, request.URL.Query().Get("space")) {
		return
	}
	resource, ok := h.exactCurrentResource(w, request, group, kind, name)
	if !ok {
		return
	}
	fence := request.Header.Get(expectedGenerationHeader)
	if fence == "" {
		h.writeError(w, "invalid_argument", action+" requires the "+expectedGenerationHeader+" fence")
		return
	}
	if fence != strconv.FormatInt(resource.Generation, 10) {
		h.writeError(w, "generation_conflict", action+" generation fence is stale")
		return
	}
	if request.Header.Get(ErrorProbeHeader) == ProbeTouchStatus {
		// A host-side status touch: the representation changes, so the
		// revision advances while the desired generation does not.
		resource.StatusTouches++
		resource.Revision++
	}
	response := encodeJSONBody(map[string]any{"resource": h.renderResource(resource)})
	etag := quotedRevision(resource.Revision)
	h.writeRaw(w, http.StatusOK, etag, response)
	h.recordReplay(request, raw, request.URL.Query().Get("space"), http.StatusOK, etag, response)
}

func (h *ReferenceHost) handleImport(w http.ResponseWriter, request *http.Request, group, kind, name string) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	var body importWire
	if err := formpackage.DecodeStrictIJSON(raw, &body); err != nil {
		h.writeError(w, "invalid_argument", err.Error())
		return
	}
	if h.tryReplay(w, request, raw, body.Metadata.Space) {
		return
	}
	if body.Metadata.Name != name || body.Kind != kind || body.APIVersion != group {
		h.writeError(w, "invalid_argument", "resource identity differs from the request target")
		return
	}
	if strings.TrimSpace(body.NativeID) == "" {
		h.writeError(w, "invalid_argument", "nativeId is required")
		return
	}
	form, hostErr := h.resolveResourceWire(&body.resourceWire)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	body.Spec = form.materialize(body.Spec)
	diagnostics, hostErr := h.specDiagnostics(form, body.Spec)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if len(diagnostics) != 0 {
		h.writeError(w, "invalid_argument", "desired spec violates the installed Form Definition")
		return
	}
	specDigest, err := specCanonicalDigest(body.Spec)
	if err != nil {
		h.writeError(w, "invalid_argument", "spec is not canonicalizable I-JSON")
		return
	}
	// Adoption is a mutation, so it passes the SAME cross-resource gauntlet
	// as apply: an import may not mint a resource whose typed bindings, bundle
	// bytes, deployment weights, or handler gates a fresh apply would reject.
	if hostErr := h.validateDesiredSemantics(form, body.Metadata.Space, body.Spec); hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	existing, create, hostErr := h.mutationFences(
		mutationFenceOf(request), form, body.Metadata.Space, name, body.Metadata.Generation, "",
	)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	next := h.nextResource(form, existing, create, body.Metadata.Space, name, body.Spec, specDigest, true)
	h.storeResource(next)
	status := http.StatusOK
	if create {
		status = http.StatusCreated
	}
	response := encodeJSONBody(h.renderResource(next))
	etag := quotedRevision(next.Revision)
	h.writeRaw(w, status, etag, response)
	h.recordReplay(request, raw, body.Metadata.Space, status, etag, response)
}

func (h *ReferenceHost) handleDelete(w http.ResponseWriter, request *http.Request, group, kind, name string) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if len(raw) != 0 {
		h.writeError(w, "invalid_argument", "delete body must be empty")
		return
	}
	if request.Header.Get("Idempotency-Key") == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return
	}
	if h.tryReplay(w, request, raw, request.URL.Query().Get("space")) {
		return
	}
	resource, ok := h.exactCurrentResource(w, request, group, kind, name)
	if !ok {
		return
	}
	ifMatch := request.Header.Get("If-Match")
	if ifMatch == "" {
		h.writeError(w, "invalid_argument", "delete requires the If-Match revision fence")
		return
	}
	if ifMatch != quotedRevision(resource.Revision) {
		h.writeError(w, "revision_conflict", "delete revision fence is stale")
		return
	}
	if hostErr := h.dependencyInUse(resource); hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	key := resourceKey(resource.Space, resource.Group, resource.Kind, resource.Name)
	if request.Header.Get(ErrorProbeHeader) == ProbeAsync {
		h.acceptOperation(w, request, raw, request.URL.Query().Get("space"), func() (map[string]any, *hostError) {
			// The revision fence and the live-binding scan are re-derived at
			// COMMIT time: an accepted delete must not remove a resource that
			// has since been re-bound or re-revised.
			current := h.resources[key]
			if current == nil {
				return nil, stableError("resource_not_found", "resource is absent")
			}
			if ifMatch != quotedRevision(current.Revision) {
				return nil, stableError("revision_conflict", "delete revision fence is stale")
			}
			if hostErr := h.dependencyInUse(current); hostErr != nil {
				return nil, hostErr
			}
			delete(h.resources, key)
			return map[string]any{"deleted": true}, nil
		})
		return
	}
	delete(h.resources, key)
	h.writeRaw(w, http.StatusNoContent, "", nil)
	h.recordReplay(request, raw, request.URL.Query().Get("space"), http.StatusNoContent, "", nil)
}

// acceptOperation registers a deferred mutation and answers 202 with a
// pending Operation envelope. The mutation runs when polling exhausts the
// deterministic wait; cancel before that point abandons it.
func (h *ReferenceHost) acceptOperation(
	w http.ResponseWriter,
	request *http.Request,
	raw []byte,
	space string,
	commit func() (map[string]any, *hostError),
) {
	h.opCounter++
	operation := &hostOperation{
		ID:             "op_" + strconv.Itoa(h.opCounter),
		PollsRemaining: asyncOperationPolls,
		commit:         commit,
	}
	h.operations[operation.ID] = operation
	response := encodeJSONBody(map[string]any{"operation": h.renderOperation(operation)})
	h.writeRaw(w, http.StatusAccepted, "", response)
	h.recordReplay(request, raw, space, http.StatusAccepted, "", response)
}

func (h *ReferenceHost) renderOperation(operation *hostOperation) map[string]any {
	out := map[string]any{
		"apiVersion": operationAPIVersion,
		"kind":       "Operation",
		"id":         operation.ID,
		"done":       operation.Done,
	}
	return out
}

func (h *ReferenceHost) completeOperation(operation *hostOperation, result map[string]any, hostErr *hostError) {
	operation.Done = true
	document := h.renderOperation(operation)
	if hostErr != nil {
		document["error"] = map[string]any{
			"code":      hostErr.Code,
			"message":   hostErr.Message,
			"retryable": isAutomaticallyRetryable(hostErr.Code),
		}
	} else {
		document["result"] = result
	}
	operation.terminalBody = encodeJSONBody(document)
}

func (h *ReferenceHost) handleOperationGet(w http.ResponseWriter, id string) {
	operation := h.operations[id]
	if operation == nil {
		h.writeError(w, "operation_not_found", "operation is unknown")
		return
	}
	if !operation.Done {
		operation.PollsRemaining--
		if operation.PollsRemaining > 0 {
			w.Header().Set("Retry-After", "0")
			h.writeRaw(w, http.StatusOK, "", encodeJSONBody(h.renderOperation(operation)))
			return
		}
		result, hostErr := operation.commit()
		h.completeOperation(operation, result, hostErr)
	}
	h.writeRaw(w, http.StatusOK, "", operation.terminalBody)
}

func (h *ReferenceHost) handleOperationCancel(w http.ResponseWriter, request *http.Request, id string) {
	if request.Header.Get("Idempotency-Key") == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return
	}
	operation := h.operations[id]
	if operation == nil {
		h.writeError(w, "operation_not_found", "operation is unknown")
		return
	}
	if !operation.Done {
		h.completeOperation(operation, nil, stableError("operation_cancelled", "operation was cancelled before completion"))
	}
	h.writeRaw(w, http.StatusOK, "", operation.terminalBody)
}

func (h *ReferenceHost) routeArtifacts(w http.ResponseWriter, request *http.Request, parts []string) {
	switch {
	case len(parts) == 2 && parts[1] == "uploads" && request.Method == http.MethodPost:
		h.handleArtifactUploadStart(w, request)
	case len(parts) == 3 && parts[1] == "uploads" && request.Method == http.MethodDelete:
		h.handleArtifactUploadAbandon(w, request, parts[2])
	case len(parts) == 4 && parts[1] == "uploads" && parts[3] == "commit" && request.Method == http.MethodPost:
		h.handleArtifactCommit(w, request, parts[2])
	case len(parts) == 5 && parts[1] == "uploads" && parts[3] == "blobs" && request.Method == http.MethodPut:
		h.handleArtifactBlobUpload(w, request, parts[2], parts[4])
	case len(parts) == 3 && parts[1] == "blobs" && request.Method == http.MethodHead:
		if h.blobs[parts[2]] == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	case len(parts) == 2 && request.Method == http.MethodGet:
		manifestRaw := h.manifests[parts[1]]
		if manifestRaw == nil {
			h.writeError(w, "artifact_missing", "manifest is unknown")
			return
		}
		h.writeRaw(w, http.StatusOK, "", manifestRaw)
	default:
		h.writeError(w, "resource_not_found", "unknown artifact operation")
	}
}

func validateArtifactManifest(manifest artifactManifest) *hostError {
	if manifest.APIVersion != artifactAPIVersion {
		return stableError("artifact_invalid", "manifest apiVersion must be "+artifactAPIVersion)
	}
	switch manifest.Kind {
	case "WorkerBundle":
		if manifest.MainModule == "" || len(manifest.Modules) == 0 {
			return stableError("artifact_invalid", "a WorkerBundle manifest requires mainModule and modules")
		}
	case "StaticAssetBundle", "MigrationBundle":
		if len(manifest.Files) == 0 {
			return stableError("artifact_invalid", "a file bundle manifest requires files")
		}
	default:
		return stableError("artifact_invalid", "manifest kind is not a closed artifact kind")
	}
	names := map[string]bool{}
	moduleBytes := int64(0)
	for _, module := range manifest.Modules {
		if !artifactPathPattern.MatchString(module.Name) || len(module.Name) > 240 {
			return stableError("artifact_invalid", "module path grammar is invalid")
		}
		if names[module.Name] {
			return stableError("artifact_invalid", "duplicate module path")
		}
		names[module.Name] = true
		if !artifactModuleMediaTypes[module.MediaType] {
			return stableError("artifact_invalid", "module mediaType is not closed")
		}
		if err := validateArtifactSize(module.Size); err != nil {
			return err
		}
		if !formpackage.ValidDigest(module.Digest) {
			return stableError("artifact_invalid", "module digest grammar is invalid")
		}
		size, _ := strconv.ParseInt(module.Size.String(), 10, 64)
		moduleBytes += size
	}
	if manifest.Kind == "WorkerBundle" && !names[manifest.MainModule] {
		return stableError("artifact_invalid", "mainModule must name one declared module")
	}
	// A source map is evidence about another module. "<module>.map" is the
	// portable naming rule, so a source map whose target module is not
	// declared describes nothing and is rejected.
	for _, module := range manifest.Modules {
		if module.MediaType != sourceMapMediaType {
			continue
		}
		target := strings.TrimSuffix(module.Name, sourceMapSuffix)
		if target == module.Name || !names[target] {
			return stableError("artifact_invalid", "source map target module is absent from the manifest")
		}
	}
	// The host's published limit is the enforced ceiling: a manifest whose
	// declared module sizes overrun maximumBundleBytes is rejected before any
	// blob is accepted, not discovered after the bytes are already stored.
	if moduleBytes > maximumBundleBytes {
		return stableError("artifact_invalid", "manifest module sizes overrun the host's published bundle limit")
	}
	paths := map[string]bool{}
	for _, file := range manifest.Files {
		if !artifactPathPattern.MatchString(file.Path) || len(file.Path) > 240 {
			return stableError("artifact_invalid", "file path grammar is invalid")
		}
		if paths[file.Path] {
			return stableError("artifact_invalid", "duplicate file path")
		}
		paths[file.Path] = true
		if err := validateArtifactSize(file.Size); err != nil {
			return err
		}
		if !formpackage.ValidDigest(file.Digest) {
			return stableError("artifact_invalid", "file digest grammar is invalid")
		}
	}
	return nil
}

func validateArtifactSize(size json.Number) *hostError {
	value, err := strconv.ParseInt(size.String(), 10, 64)
	if err != nil || value < 0 || value > 268435456 {
		return stableError("artifact_invalid", "artifact size is out of range")
	}
	return nil
}

func (manifest artifactManifest) blobDigests() []string {
	seen := map[string]bool{}
	var out []string
	for _, module := range manifest.Modules {
		if !seen[module.Digest] {
			seen[module.Digest] = true
			out = append(out, module.Digest)
		}
	}
	for _, file := range manifest.Files {
		if !seen[file.Digest] {
			seen[file.Digest] = true
			out = append(out, file.Digest)
		}
	}
	sort.Strings(out)
	return out
}

func (h *ReferenceHost) handleArtifactUploadStart(w http.ResponseWriter, request *http.Request) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	var envelope struct {
		Manifest json.RawMessage `json:"manifest"`
	}
	if err := formpackage.DecodeStrictIJSON(raw, &envelope); err != nil {
		h.writeError(w, "invalid_argument", err.Error())
		return
	}
	if len(envelope.Manifest) == 0 {
		h.writeError(w, "invalid_argument", "manifest is required")
		return
	}
	if h.tryReplay(w, request, raw, "") {
		return
	}
	var manifest artifactManifest
	if err := formpackage.DecodeStrictIJSON(envelope.Manifest, &manifest); err != nil {
		h.writeError(w, "artifact_invalid", err.Error())
		return
	}
	if hostErr := validateArtifactManifest(manifest); hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	manifestDigest, err := formpackage.DigestCanonicalJSON(envelope.Manifest)
	if err != nil {
		h.writeError(w, "artifact_invalid", "manifest is not canonicalizable I-JSON")
		return
	}
	h.uploadCounter++
	upload := &artifactUpload{
		ID:             "up_" + strconv.Itoa(h.uploadCounter),
		ManifestRaw:    append([]byte(nil), envelope.Manifest...),
		Manifest:       manifest,
		ManifestDigest: manifestDigest,
	}
	h.uploads[upload.ID] = upload
	missing := []string{}
	for _, digest := range manifest.blobDigests() {
		if h.blobs[digest] == nil {
			missing = append(missing, digest)
		}
	}
	response := encodeJSONBody(map[string]any{"uploadId": upload.ID, "missingBlobs": missing})
	h.writeRaw(w, http.StatusCreated, "", response)
	h.recordReplay(request, raw, "", http.StatusCreated, "", response)
}

func (h *ReferenceHost) handleArtifactBlobUpload(w http.ResponseWriter, request *http.Request, uploadID, digest string) {
	upload := h.uploads[uploadID]
	if upload == nil {
		h.writeError(w, "artifact_missing", "upload is unknown")
		return
	}
	declared := false
	var declaredSize int64
	for _, candidate := range upload.Manifest.blobDigests() {
		if candidate == digest {
			declared = true
		}
	}
	for _, module := range upload.Manifest.Modules {
		if module.Digest == digest {
			declaredSize, _ = strconv.ParseInt(module.Size.String(), 10, 64)
		}
	}
	for _, file := range upload.Manifest.Files {
		if file.Digest == digest {
			declaredSize, _ = strconv.ParseInt(file.Size.String(), 10, 64)
		}
	}
	if !declared {
		h.writeError(w, "artifact_invalid", "blob digest is not declared by the manifest")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(request.Body, 268435456+1))
	if err != nil {
		h.writeError(w, "invalid_argument", "invalid blob body")
		return
	}
	if formpackage.DigestBytes(raw) != digest || int64(len(raw)) != declaredSize {
		h.writeError(w, "artifact_invalid", "blob bytes do not match their declared digest and size")
		return
	}
	h.blobs[digest] = raw
	h.writeRaw(w, http.StatusCreated, "", nil)
}

func (h *ReferenceHost) handleArtifactCommit(w http.ResponseWriter, request *http.Request, uploadID string) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if request.Header.Get("Idempotency-Key") == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return
	}
	if h.tryReplay(w, request, raw, "") {
		return
	}
	upload := h.uploads[uploadID]
	if upload == nil {
		h.writeError(w, "artifact_missing", "upload is unknown")
		return
	}
	for _, digest := range upload.Manifest.blobDigests() {
		if h.blobs[digest] == nil {
			h.writeError(w, "artifact_missing", "manifest blob "+digest+" was not uploaded")
			return
		}
	}
	// Commit re-verifies size against the STORED bytes, not only against the
	// bytes of this upload. Blobs are content-addressed and shared, so a
	// second manifest can name an already-held digest under a size it never
	// had; that manifest must never become an immutable identity.
	if hostErr := h.verifyCommittedSizes(upload.Manifest); hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	status := http.StatusCreated
	if h.manifests[upload.ManifestDigest] != nil {
		status = http.StatusOK
	}
	h.manifests[upload.ManifestDigest] = append([]byte(nil), upload.ManifestRaw...)
	response := encodeJSONBody(map[string]any{"manifestDigest": upload.ManifestDigest})
	h.writeRaw(w, status, "", response)
	h.recordReplay(request, raw, "", status, "", response)
}

// verifyCommittedSizes binds every declared size to the stored byte length of
// the blob that digest resolves to.
func (h *ReferenceHost) verifyCommittedSizes(manifest artifactManifest) *hostError {
	declared := func(name, digest string, size json.Number) *hostError {
		want, err := strconv.ParseInt(size.String(), 10, 64)
		if err != nil {
			return stableError("artifact_invalid", "declared size of "+name+" is not an integer")
		}
		stored, ok := h.blobs[digest]
		if !ok {
			return stableError("artifact_missing", "manifest blob "+digest+" was not uploaded")
		}
		if int64(len(stored)) != want {
			return stableError("artifact_invalid", "stored bytes of "+name+" do not match the declared size")
		}
		return nil
	}
	for _, module := range manifest.Modules {
		if hostErr := declared(module.Name, module.Digest, module.Size); hostErr != nil {
			return hostErr
		}
	}
	for _, file := range manifest.Files {
		if hostErr := declared(file.Path, file.Digest, file.Size); hostErr != nil {
			return hostErr
		}
	}
	return nil
}

func (h *ReferenceHost) handleArtifactUploadAbandon(w http.ResponseWriter, request *http.Request, uploadID string) {
	if request.Header.Get("Idempotency-Key") == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return
	}
	delete(h.uploads, uploadID)
	h.writeRaw(w, http.StatusNoContent, "", nil)
}

func (h *ReferenceHost) routeSupport(w http.ResponseWriter, request *http.Request, parts []string) {
	if request.Method != http.MethodGet {
		h.writeError(w, "resource_not_found", "unknown support operation")
		return
	}
	switch {
	case len(parts) == 2 && parts[1] == "forms":
		profiles := make([]map[string]any, 0, len(h.catalog.forms))
		keys := make([]string, 0, len(h.catalog.forms))
		for key := range h.catalog.forms {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			profiles = append(profiles, h.formSupportProfile(h.catalog.forms[key]))
		}
		h.writeJSON(w, http.StatusOK, "", map[string]any{"profiles": profiles})
	case len(parts) == 5 && parts[1] == "forms":
		form := h.catalog.form(parts[2], parts[3])
		if form == nil || form.Ref.DefinitionVersion != parts[4] {
			h.writeError(w, "form_unknown", "exact Form line is unknown")
			return
		}
		h.writeJSON(w, http.StatusOK, "", h.formSupportProfile(form))
	case len(parts) == 4 && parts[1] == "interfaces":
		reference, ok := h.catalog.interfaces[parts[2]+"@"+parts[3]]
		if !ok {
			h.writeError(w, "resource_not_found", "interface support profile is unknown")
			return
		}
		h.writeJSON(w, http.StatusOK, "", map[string]any{
			"apiVersion": supportAPIVersion,
			"kind":       "InterfaceSupport",
			"interfaceRef": map[string]any{
				"apiVersion":   "interfaces.takoform.com/v1alpha1",
				"name":         reference.Name,
				"version":      reference.Version,
				"schemaDigest": reference.SchemaDigest,
			},
		})
	case len(parts) == 4 && parts[1] == "bindings":
		reference, ok := h.catalog.bindings[parts[2]+"@"+parts[3]]
		if !ok {
			h.writeError(w, "resource_not_found", "binding support profile is unknown")
			return
		}
		h.writeJSON(w, http.StatusOK, "", map[string]any{
			"apiVersion": supportAPIVersion,
			"kind":       "BindingSupport",
			"bindingRef": map[string]any{
				"apiVersion":   "bindings.takoform.com/v1alpha1",
				"name":         reference.Name,
				"version":      reference.Version,
				"schemaDigest": reference.SchemaDigest,
			},
		})
	default:
		h.writeError(w, "resource_not_found", "unknown support operation")
	}
}

// formSupportProfile declares capability subsets, inclusive ranges, and
// numeric limits only. Price, SKU, region, quota, and commercial policy
// never appear in this surface.
func (h *ReferenceHost) formSupportProfile(form *installedForm) map[string]any {
	profile := map[string]any{
		"apiVersion": supportAPIVersion,
		"kind":       "FormSupport",
		"formRef":    refJSON(form.Ref),
		"operations": form.operations(),
	}
	if form.Ref.Kind == "WorkerVersion" {
		profile["supportedEnums"] = map[string]any{
			"compatibilityFlags": []string{"nodejs_compat"},
			"handlers":           []string{"fetch", "scheduled", "queue", "tail"},
		}
		profile["supportedRanges"] = map[string]any{
			"compatibilityDate": map[string]any{
				"minimum": "2024-01-01",
				"maximum": "2026-08-06",
			},
		}
		profile["limits"] = map[string]any{"maximumBundleBytes": maximumBundleBytes}
	}
	return profile
}

// Idempotency replay: keys are namespaced by authenticated tenant, principal,
// space, and key; the fingerprint binds method, request target,
// preconditions, and exact body bytes. A recorded success replays its exact
// status, ETag, and body; a fingerprint mismatch fails invalid_argument.
func (h *ReferenceHost) tryReplay(w http.ResponseWriter, request *http.Request, raw []byte, space string) bool {
	key := request.Header.Get("Idempotency-Key")
	if key == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return true
	}
	recorded, ok := h.replays[h.replayKey(request, space, key)]
	if !ok {
		return false
	}
	if recorded.Fingerprint != requestFingerprint(request, raw) {
		h.writeError(w, "invalid_argument", "Idempotency-Key was reused for another request")
		return true
	}
	h.writeRaw(w, recorded.Status, recorded.ETag, recorded.Body)
	return true
}

func (h *ReferenceHost) recordReplay(
	request *http.Request,
	raw []byte,
	space string,
	status int,
	etag string,
	body []byte,
) {
	key := request.Header.Get("Idempotency-Key")
	if key == "" {
		return
	}
	h.replays[h.replayKey(request, space, key)] = recordedReplay{
		Fingerprint: requestFingerprint(request, raw),
		Status:      status,
		ETag:        etag,
		Body:        append([]byte(nil), body...),
	}
}

func (h *ReferenceHost) replayKey(request *http.Request, space, key string) string {
	auth, _ := hostRequestAuth(request)
	return strings.Join([]string{
		"tenant=" + auth.Tenant,
		"principal=" + auth.Principal,
		"space=" + space,
		"key=" + key,
	}, "\x00")
}

func requestFingerprint(request *http.Request, raw []byte) string {
	return formpackage.DigestBytes([]byte(strings.Join([]string{
		request.Method,
		request.URL.RequestURI(),
		request.Header.Get("If-Match"),
		request.Header.Get("If-None-Match"),
		request.Header.Get(expectedGenerationHeader),
		string(raw),
	}, "\x00")))
}

func encodeJSONBody(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte(`{"error":{"code":"internal_error","message":` +
			strconv.Quote(err.Error()) + `,"requestId":"ref3-encode","retryable":false}}`)
	}
	return append(raw, '\n')
}

func (h *ReferenceHost) writeJSON(w http.ResponseWriter, status int, etag string, value any) {
	h.writeRaw(w, status, etag, encodeJSONBody(value))
}

func (h *ReferenceHost) writeRaw(w http.ResponseWriter, status int, etag string, raw []byte) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	w.WriteHeader(status)
	if len(raw) > 0 {
		_, _ = w.Write(raw)
	}
}

func (h *ReferenceHost) writeHostError(w http.ResponseWriter, hostErr *hostError) {
	h.writeError(w, hostErr.Code, hostErr.Message)
}

func (h *ReferenceHost) writeError(w http.ResponseWriter, code, message string) {
	status, known := stableErrorHTTPStatusByCode[code]
	if !known {
		status = http.StatusInternalServerError
		code = "internal_error"
	}
	if message == "" {
		message = "stable error"
	}
	h.requestCounter++
	h.writeJSON(w, status, "", map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   message,
			"requestId": "ref3-" + code + "-" + strconv.Itoa(h.requestCounter),
			"retryable": isAutomaticallyRetryable(code),
		},
	})
}
