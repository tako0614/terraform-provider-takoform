// Package client is a thin HTTP client for the portable Takoform Service Form API.
//
// It is deliberately transport-only: it speaks the Takoform Resource object
// envelope (apiVersion/kind/metadata/spec/status) over JSON and maps error
// envelopes to typed errors. It never talks to AWS / Cloudflare / Kubernetes
// or any southbound API, never selects a backend, and never manages
// credentials. Placement and implementation selection remain host concerns;
// this client only carries desired form state and sanitized observed status.
package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// API constants for the current provider-v2 wire contract. Provider v1 keeps
// the frozen v1alpha1 discovery and API paths in its own release line.
const (
	APIVersion    = formpackage.CurrentFormAPIVersion
	DiscoveryPath = "/.well-known/takoform/v1alpha2"
	APIRootPath   = "/apis/forms.takoform.com/v1alpha2"

	defaultUserAgent     = "terraform-provider-takoform"
	maxResponseBodyBytes = 8 * 1024 * 1024
)

// ErrNotFound is returned only for the stable resource_not_found error on
// HTTP 404. Callers map this exact condition to "remove from state".
var ErrNotFound = errors.New("takoform: resource not found")

// Discovery is the parsed body of the current versioned well-known endpoint.
//
// Features is intentionally a map so the provider stays capability-driven
// (it inspects named capabilities) rather than edition-driven (it never
// branches on an "edition" string).
type Discovery struct {
	APIVersions []string        `json:"api_versions"`
	Features    map[string]bool `json:"features"`
	Endpoints   Endpoints       `json:"endpoints"`
}

// Endpoints carries advertised service URLs from discovery.
type Endpoints struct {
	API        string `json:"api,omitempty"`
	Forms      string `json:"forms,omitempty"`
	Interfaces string `json:"interfaces,omitempty"`
	OIDCIssuer string `json:"oidc_issuer,omitempty"`
}

// HasFeature reports whether a named server capability is advertised.
func (d Discovery) HasFeature(name string) bool {
	return d.Features[name]
}

// SupportsServiceForms reports whether the endpoint exposes the portable
// Service Form API. The provider refuses to configure when this is false.
func (d Discovery) SupportsServiceForms() bool {
	return d.Features["service_forms"]
}

// Metadata is the Resource object metadata block.
type Metadata struct {
	Name            string `json:"name"`
	Space           string `json:"space,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

// Status carries only the Form's portable observed and output documents.
type Status struct {
	Observed map[string]any `json:"observed,omitempty"`
	Output   map[string]any `json:"output,omitempty"`
}

// Resource is the Takoform Resource object envelope. Spec is kept generic so
// the same transport carries every Service Form; the provider's resource
// layer owns the per-shape spec contents.
type Resource struct {
	APIVersion string                  `json:"apiVersion"`
	Kind       string                  `json:"kind"`
	Form       *InstalledFormReference `json:"form,omitempty"`
	Metadata   Metadata                `json:"metadata"`
	Spec       map[string]any          `json:"spec,omitempty"`
	Status     *Status                 `json:"status,omitempty"`
}

// FormRef pins one immutable typed Form Definition. Publication and admission
// are external to this value.
type FormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
}

// UnmarshalJSON preserves the normative raw-document validation boundary.
// Validating only a decoded Go struct would lose duplicate member names before
// the Form Package I-JSON validator can reject them.
func (ref *FormRef) UnmarshalJSON(raw []byte) error {
	validated, err := formpackage.ValidateFormRef(raw)
	if err != nil {
		return fmt.Errorf("takoform: invalid FormRef JSON: %w", err)
	}
	ref.APIVersion = validated.APIVersion
	ref.Kind = validated.Kind
	ref.DefinitionVersion = validated.DefinitionVersion
	ref.SchemaDigest = validated.SchemaDigest
	return nil
}

// InstalledFormReference adds the exact package bytes selected by this
// provider release. Hosts must match all five identity fields.
type InstalledFormReference struct {
	FormRef       FormRef `json:"formRef"`
	PackageDigest string  `json:"packageDigest"`
}

// MutationFence carries the exact identity and generation required by an
// existing-resource lifecycle mutation.
type MutationFence struct {
	ResourceVersion string
	Form            InstalledFormReference
}

// PreviewResourceResult decodes the exact portable Resource and review fence.
type PreviewResourceResult struct {
	Resource Resource      `json:"resource"`
	Review   PreviewReview `json:"review,omitempty"`
}

type PreviewReview struct {
	PlanDigest string `json:"planDigest"`
	SpecDigest string `json:"specDigest"`
}

// DeploymentReview presents the exact preview evidence accepted by apply.
// Pricing, rating, and quote authority belong to a host's own commercial
// layer; the portable protocol carries only the reviewed plan digest.
type DeploymentReview struct {
	PlanDigest string `json:"planDigest"`
}

type applyResourceBody struct {
	Resource
	Review DeploymentReview `json:"review"`
}

// APIError is the typed form of the Takoform error envelope for non-2xx
// responses. The wire envelope is nested: the top-level "error" field is an
// object, e.g.
//
//	{ "error": { "code": "<code>", "message": "<msg>", "requestId": "<id>", "details": <any> } }
type APIError struct {
	// StatusCode is the HTTP status; it is not part of the wire body.
	StatusCode int
	Code       string
	Message    string
	RequestID  string
	Retryable  bool
	HostCode   string
	// ProtocolInvalid means the response did not match the closed stable error
	// taxonomy and therefore carries no portable code semantics.
	ProtocolInvalid bool
	// Details is the optional, free-form details payload, kept raw.
	Details json.RawMessage
}

var stableErrorHTTPStatusByCode = map[string]int{
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

var automaticallyRetryableErrorCodes = map[string]struct{}{
	"resource_busy":       {},
	"backend_unavailable": {},
}

// errorEnvelope decodes the nested wire shape of an error response.
type errorEnvelope struct {
	Error struct {
		Code      string          `json:"code"`
		Message   string          `json:"message"`
		RequestID string          `json:"requestId"`
		Retryable *bool           `json:"retryable"`
		HostCode  string          `json:"hostCode,omitempty"`
		Details   json.RawMessage `json:"details,omitempty"`
	} `json:"error"`
}

func (e *APIError) Error() string {
	var b strings.Builder
	if e.ProtocolInvalid {
		b.WriteString("takoform protocol-invalid error response")
	} else {
		b.WriteString("takoform api error")
	}
	if e.StatusCode != 0 {
		fmt.Fprintf(&b, " (http %d)", e.StatusCode)
	}
	if e.Code != "" {
		fmt.Fprintf(&b, " [%s]", e.Code)
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (requestId=%s)", e.RequestID)
	}
	return b.String()
}

func isResourceNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) &&
		apiErr.StatusCode == http.StatusNotFound &&
		apiErr.Code == "resource_not_found"
}

// Client is a thin Takoform Service Form API HTTP client.
type Client struct {
	endpoint      string // normalized origin, no trailing slash
	token         string
	httpClient    *http.Client
	userAgent     string
	apiBase       string
	formsURL      string
	interfacesURL string
	retryAttempts int

	// Discovery is populated by Discover and cached for capability checks.
	Discovery Discovery
}

// Options controls optional client behaviour.
type Options struct {
	RetryAttempts int
}

// New constructs a Client. If httpClient is nil, http.DefaultClient is used.
func New(endpoint, token string, httpClient *http.Client) *Client {
	return NewWithOptions(endpoint, token, httpClient, Options{})
}

// NewWithOptions constructs a client. Every request uses the versioned API
// base advertised by discovery; there is no unversioned fallback lane.
func NewWithOptions(endpoint, token string, httpClient *http.Client, options Options) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	retryAttempts := options.RetryAttempts
	if retryAttempts <= 0 {
		retryAttempts = 3
	}
	return &Client{
		endpoint:      strings.TrimRight(endpoint, "/"),
		token:         token,
		httpClient:    httpClient,
		userAgent:     defaultUserAgent,
		retryAttempts: retryAttempts,
	}
}

// Endpoint returns the normalized endpoint origin.
func (c *Client) Endpoint() string { return c.endpoint }

// Discover performs GET {endpoint}{DiscoveryPath} and caches the result.
func (c *Client) Discover(ctx context.Context) (Discovery, error) {
	var disco Discovery
	if _, err := c.configuredOrigin(); err != nil {
		return Discovery{}, err
	}
	if err := c.doJSON(ctx, http.MethodGet, c.endpoint+DiscoveryPath, nil, &disco); err != nil {
		return Discovery{}, err
	}
	c.Discovery = disco
	if err := c.negotiateEndpoints(disco); err != nil {
		return Discovery{}, err
	}
	return disco, nil
}

func (c *Client) negotiateEndpoints(disco Discovery) error {
	c.apiBase = ""
	c.formsURL = ""
	c.interfacesURL = ""
	if !disco.SupportsServiceForms() {
		return errors.New("takoform: discovery does not advertise features.service_forms")
	}
	if len(disco.APIVersions) != 1 || disco.APIVersions[0] != APIVersion {
		return fmt.Errorf("takoform: versioned discovery must advertise only API version %s", APIVersion)
	}
	if strings.TrimSpace(disco.Endpoints.API) == "" {
		return errors.New("takoform: discovery must advertise an absolute same-origin endpoints.api")
	}
	for _, feature := range []string{"exact_form_ref", "optimistic_concurrency", "idempotent_lifecycle"} {
		if !disco.HasFeature(feature) {
			return fmt.Errorf("takoform: versioned discovery does not advertise features.%s", feature)
		}
	}

	apiBase, err := c.validAdvertisedEndpoint(disco.Endpoints.API, APIRootPath)
	if err != nil {
		return fmt.Errorf("takoform: invalid discovery API endpoint: %w", err)
	}
	c.apiBase = strings.TrimRight(apiBase, "/")
	c.formsURL = c.apiBase + "/forms"
	if disco.Endpoints.Forms != "" {
		formsURL, err := c.validAdvertisedEndpoint(disco.Endpoints.Forms, APIRootPath+"/forms")
		if err != nil {
			return fmt.Errorf("takoform: invalid discovery forms endpoint: %w", err)
		}
		c.formsURL = strings.TrimRight(formsURL, "/")
	}
	if disco.Endpoints.OIDCIssuer != "" {
		if err := validateOIDCIssuer(disco.Endpoints.OIDCIssuer); err != nil {
			return fmt.Errorf("takoform: invalid discovery OIDC issuer: %w", err)
		}
	}
	// Runtime interface declarations are an optional, read-only surface. A host
	// that advertises no declarations remains a conforming Form host.
	if disco.HasFeature(FeatureInterfaceDeclarations) {
		c.interfacesURL = c.apiBase + "/interfaces"
		if disco.Endpoints.Interfaces != "" {
			interfacesURL, err := c.validAdvertisedEndpoint(disco.Endpoints.Interfaces, APIRootPath+"/interfaces")
			if err != nil {
				return fmt.Errorf("takoform: invalid discovery interfaces endpoint: %w", err)
			}
			c.interfacesURL = strings.TrimRight(interfacesURL, "/")
		}
	}
	return nil
}

func (c *Client) validAdvertisedEndpoint(raw, expectedPath string) (string, error) {
	advertised, err := url.Parse(raw)
	if err != nil || !advertised.IsAbs() || advertised.Host == "" || (advertised.Scheme != "http" && advertised.Scheme != "https") {
		return "", fmt.Errorf("endpoint must be an absolute URL")
	}
	configured, err := c.configuredOrigin()
	if err != nil {
		return "", err
	}
	if !sameOrigin(advertised, configured) {
		return "", errors.New("cross-origin discovery endpoints are rejected to protect bearer credentials")
	}
	if advertised.User != nil || advertised.Fragment != "" || advertised.RawQuery != "" {
		return "", errors.New("endpoint must not contain userinfo, query, or fragment")
	}
	if advertised.EscapedPath() != expectedPath {
		return "", fmt.Errorf("endpoint path must be %s", expectedPath)
	}
	return advertised.String(), nil
}

func validateOIDCIssuer(raw string) error {
	issuer, err := url.Parse(raw)
	if err != nil || !issuer.IsAbs() || !strings.EqualFold(issuer.Scheme, "https") || issuer.Host == "" {
		return errors.New("issuer must be an absolute HTTPS URL")
	}
	if issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" {
		return errors.New("issuer must not contain userinfo, query, or fragment")
	}
	return nil
}

func sameOrigin(left, right *url.URL) bool {
	return left != nil &&
		right != nil &&
		strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectivePort(left) == effectivePort(right)
}

func effectivePort(endpoint *url.URL) string {
	if port := endpoint.Port(); port != "" {
		return port
	}
	switch strings.ToLower(endpoint.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func (c *Client) configuredOrigin() (*url.URL, error) {
	configured, err := url.Parse(c.endpoint)
	if err != nil || !configured.IsAbs() || configured.Host == "" ||
		(configured.Scheme != "http" && configured.Scheme != "https") {
		return nil, errors.New("takoform: configured endpoint must be an absolute HTTP(S) origin")
	}
	if configured.User != nil || configured.RawQuery != "" || configured.Fragment != "" ||
		(configured.Path != "" && configured.Path != "/") {
		return nil, errors.New("takoform: configured endpoint must not contain userinfo, path, query, or fragment")
	}
	if configured.Scheme == "http" && !isLoopbackHostname(configured.Hostname()) {
		return nil, errors.New("takoform: configured endpoint must use HTTPS unless it is a loopback development origin")
	}
	return configured, nil
}

func isLoopbackHostname(hostname string) bool {
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// resourceURL builds {advertised API base}/resources/{kind}/{name}.
func (c *Client) resourceURL(kind, name string, query url.Values) string {
	u := fmt.Sprintf("%s/resources/%s/%s", c.apiBase, url.PathEscape(kind), url.PathEscape(name))
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func spaceQuery(space string) url.Values {
	if space == "" {
		return nil
	}
	q := url.Values{}
	q.Set("space", space)
	return q
}

func (c *Client) putResourceURL(kind, name string) string {
	return fmt.Sprintf("%s/resources/%s/%s", c.apiBase, url.PathEscape(kind), url.PathEscape(name))
}

func (c *Client) importResourceURL(kind, name string) string {
	return fmt.Sprintf("%s/resources/%s/%s/import", c.apiBase, url.PathEscape(kind), url.PathEscape(name))
}

func (c *Client) observeResourceURL(kind, name, space string) string {
	u := fmt.Sprintf("%s/resources/%s/%s/observe", c.apiBase, url.PathEscape(kind), url.PathEscape(name))
	if query := spaceQuery(space); len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func (c *Client) refreshResourceURL(kind, name, space string) string {
	u := fmt.Sprintf("%s/resources/%s/%s/refresh", c.apiBase, url.PathEscape(kind), url.PathEscape(name))
	if query := spaceQuery(space); len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func (c *Client) previewURL() string {
	return c.apiBase + "/resources/preview"
}

func (c *Client) actionResourceURL(kind, name, action string, query url.Values) string {
	u := fmt.Sprintf("%s/resources/%s/%s/%s", c.apiBase, url.PathEscape(kind), url.PathEscape(name), action)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func exactResourceQuery(space string, form InstalledFormReference) url.Values {
	query := url.Values{}
	query.Set("space", space)
	query.Set("apiVersion", form.FormRef.APIVersion)
	query.Set("kind", form.FormRef.Kind)
	query.Set("definitionVersion", form.FormRef.DefinitionVersion)
	query.Set("schemaDigest", form.FormRef.SchemaDigest)
	query.Set("packageDigest", form.PackageDigest)
	return query
}

type FormAvailability struct {
	Identity             InstalledFormReference `json:"identity"`
	DefinitionKnown      bool                   `json:"definitionKnown"`
	Installed            bool                   `json:"installed"`
	Executable           bool                   `json:"executable"`
	Activated            bool                   `json:"activated"`
	AvailableToPrincipal bool                   `json:"availableToPrincipal"`
	Operations           []string               `json:"operations"`
	Deprecated           bool                   `json:"deprecated"`
}

// FormDefinitionResponse is the principal-readable desired-state contract for
// one exact current FormRef. It carries no lifecycle, host implementation, or
// commercial authority.
type FormDefinitionResponse struct {
	Identity      InstalledFormReference `json:"identity"`
	DisplayName   string                 `json:"displayName,omitempty"`
	Description   string                 `json:"description,omitempty"`
	DesiredSchema map[string]any         `json:"desiredSchema"`
}

// GetFormDefinition reads the exact current desired-state schema selected by
// the complete FormRef query. A substituted response identity fails closed.
func (c *Client) GetFormDefinition(
	ctx context.Context,
	space string,
	form InstalledFormReference,
) (FormDefinitionResponse, error) {
	if err := validateInstalledFormReference(form.FormRef.Kind, form); err != nil {
		return FormDefinitionResponse{}, err
	}
	if err := ValidateSpaceID(space); err != nil {
		return FormDefinitionResponse{}, fmt.Errorf("takoform: exact Form Definition has invalid SpaceID: %w", err)
	}
	query := exactResourceQuery(space, form)
	endpoint := fmt.Sprintf(
		"%s/form-definitions/%s?%s",
		c.apiBase,
		url.PathEscape(form.FormRef.Kind),
		query.Encode(),
	)
	var response FormDefinitionResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return FormDefinitionResponse{}, err
	}
	if !sameForm(&form, &response.Identity) {
		return FormDefinitionResponse{}, errors.New("takoform: host substituted the requested exact Form Definition identity")
	}
	if response.DesiredSchema == nil {
		return FormDefinitionResponse{}, errors.New("takoform: host omitted the exact Form Definition desiredSchema")
	}
	return response, nil
}

func (c *Client) EnsureFormAvailable(ctx context.Context, space string, form InstalledFormReference, operation string) error {
	if err := validateInstalledFormReference(form.FormRef.Kind, form); err != nil {
		return err
	}
	if err := ValidateSpaceID(space); err != nil {
		return fmt.Errorf("takoform: exact FormRef availability has invalid SpaceID: %w", err)
	}
	query := exactResourceQuery(space, form)
	var response struct {
		Forms []FormAvailability `json:"forms"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.formsURL+"?"+query.Encode(), nil, &response); err != nil {
		return err
	}
	if len(response.Forms) != 1 || !sameForm(&form, &response.Forms[0].Identity) {
		return errors.New("takoform: host did not return the requested exact FormRef")
	}
	available := response.Forms[0]
	if !available.DefinitionKnown || !available.Installed || !available.Executable || !available.Activated || !available.AvailableToPrincipal {
		return fmt.Errorf("takoform: exact FormRef %s is not available to this principal", form.FormRef.Kind)
	}
	if !containsString(available.Operations, operation) {
		return fmt.Errorf("takoform: exact FormRef %s does not support %s", form.FormRef.Kind, operation)
	}
	return nil
}

// PutResource creates or updates a resource through the canonical reviewed
// Deploy API lifecycle. It previews the exact desired Resource, then presents
// that reviewed plan digest to PUT. Backend selection, placement, and any
// commercial evidence remain server-side concerns.
func (c *Client) PutResource(ctx context.Context, kind, name string, body *Resource) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("takoform: apply requires a Resource body")
	}
	if err := validateResourceIdentity(kind, body); err != nil {
		return nil, err
	}
	if body.Metadata.Name != name {
		return nil, errors.New("takoform: Resource metadata.name does not match the requested URL name")
	}
	operation := "create"
	if body.Metadata.ResourceVersion != "" {
		operation = "update"
	}
	if body.Form != nil {
		if err := c.EnsureFormAvailable(ctx, body.Metadata.Space, *body.Form, operation); err != nil {
			return nil, err
		}
	}
	transportResource := *body
	preview, err := c.PreviewResource(ctx, &transportResource)
	if err != nil {
		return nil, err
	}
	planDigest := preview.Review.PlanDigest
	if strings.TrimSpace(planDigest) == "" {
		return nil, errors.New("takoform: Deploy API preview omitted planDigest")
	}
	review := DeploymentReview{PlanDigest: planDigest}

	var out Resource
	request := applyResourceBody{Resource: transportResource, Review: review}
	headers := c.resourceMutationHeaders("apply", body, request)
	responseHeaders, err := c.doJSONWithHeaders(
		ctx, http.MethodPut, c.putResourceURL(kind, name), headers, &request, &out, true,
		http.StatusOK, http.StatusCreated,
	)
	if err != nil {
		return nil, err
	}
	if err := verifyResourceIdentity(body.Form, name, body.Metadata.Space, &out); err != nil {
		return nil, err
	}
	if err := captureResourceVersion(&out, responseHeaders); err != nil {
		return nil, err
	}
	return &out, nil
}

type importResourceBody struct {
	Resource
	NativeID string `json:"nativeId"`
}

// ImportResource adopts one existing provider-native object using the full
// desired Resource spec. The server plans and applies a read-only
// config-driven import before publishing Resource-owned state and outputs.
func (c *Client) ImportResource(ctx context.Context, kind, name, nativeID string, body *Resource) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("takoform: import requires a Resource body")
	}
	request := importResourceBody{Resource: *body, NativeID: nativeID}
	if err := validateResourceIdentity(kind, body); err != nil {
		return nil, err
	}
	if body.Metadata.Name != name {
		return nil, errors.New("takoform: Resource metadata.name does not match the requested URL name")
	}
	if err := c.EnsureFormAvailable(ctx, body.Metadata.Space, *body.Form, "import"); err != nil {
		return nil, err
	}
	var wrapped struct {
		Resource Resource `json:"resource"`
	}
	headers := c.resourceMutationHeaders("import", body, request)
	responseHeaders, err := c.doJSONWithHeaders(
		ctx, http.MethodPost, c.importResourceURL(kind, name), headers, &request, &wrapped, true,
		http.StatusOK, http.StatusCreated,
	)
	if err != nil {
		return nil, err
	}
	if err := verifyResourceIdentity(body.Form, name, body.Metadata.Space, &wrapped.Resource); err != nil {
		return nil, err
	}
	if err := captureResourceVersion(&wrapped.Resource, responseHeaders); err != nil {
		return nil, err
	}
	return &wrapped.Resource, nil
}

// GetResource reads a resource. Only the stable resource_not_found code is
// translated to ErrNotFound; other errors retain their typed API identity.
func (c *Client) GetResource(ctx context.Context, kind, name, space string, form ...InstalledFormReference) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if err := ValidateSpaceID(space); err != nil {
		return nil, fmt.Errorf("takoform: resource read has invalid SpaceID: %w", err)
	}
	if len(form) != 1 {
		return nil, errors.New("takoform: versioned Resource read requires one exact FormRef")
	}
	if err := validateInstalledFormReference(kind, form[0]); err != nil {
		return nil, err
	}
	expected := &form[0]
	query := exactResourceQuery(space, form[0])
	var out Resource
	responseHeaders, err := c.doJSONWithHeaders(
		ctx, http.MethodGet, c.resourceURL(kind, name, query), nil, nil, &out, false,
		http.StatusOK,
	)
	if err != nil {
		if isResourceNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := verifyResourceIdentity(expected, name, space, &out); err != nil {
		return nil, err
	}
	if err := captureResourceVersion(&out, responseHeaders); err != nil {
		return nil, err
	}
	return &out, nil
}

// ObserveResource performs a read-only observation and returns the Resource's
// portable observed and output documents. Only resource_not_found maps to
// ErrNotFound.
func (c *Client) ObserveResource(ctx context.Context, kind, name, space string, options ...MutationFence) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if err := ValidateSpaceID(space); err != nil {
		return nil, fmt.Errorf("takoform: resource observe has invalid SpaceID: %w", err)
	}
	resourceVersion, form := mutationIdentity(options)
	if len(form) != 1 || !validResourceVersion(resourceVersion) {
		return nil, errors.New("takoform: versioned observe requires one exact FormRef and resourceVersion")
	}
	if err := validateInstalledFormReference(kind, form[0]); err != nil {
		return nil, err
	}
	expected := &form[0]
	query := exactResourceQuery(space, form[0])
	var wrapped struct {
		Resource Resource `json:"resource"`
	}
	headers := map[string]string{
		"If-Match":        quoteResourceVersion(resourceVersion),
		"Idempotency-Key": mutationKey("observe", kind, name, space, resourceVersion, expected),
	}
	responseHeaders, err := c.doJSONWithHeaders(
		ctx, http.MethodPost, c.actionResourceURL(kind, name, "observe", query), headers, nil, &wrapped, true,
		http.StatusOK,
	)
	if err != nil {
		if isResourceNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	out := wrapped.Resource
	if err := verifyResourceIdentity(expected, name, space, &out); err != nil {
		return nil, err
	}
	if err := captureResourceVersion(&out, responseHeaders); err != nil {
		return nil, err
	}
	if err := requireExactResourceVersion(&out, resourceVersion); err != nil {
		return nil, err
	}
	return &out, nil
}

// RefreshResource updates the Resource-owned backend state and public outputs
// without mutating native provider resources. Only resource_not_found maps to
// ErrNotFound.
func (c *Client) RefreshResource(ctx context.Context, kind, name, space string, options ...MutationFence) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if err := ValidateSpaceID(space); err != nil {
		return nil, fmt.Errorf("takoform: resource refresh has invalid SpaceID: %w", err)
	}
	resourceVersion, form := mutationIdentity(options)
	if len(form) != 1 || !validResourceVersion(resourceVersion) {
		return nil, errors.New("takoform: versioned refresh requires one exact FormRef and resourceVersion")
	}
	if err := validateInstalledFormReference(kind, form[0]); err != nil {
		return nil, err
	}
	expected := &form[0]
	query := exactResourceQuery(space, form[0])
	var wrapped struct {
		Resource Resource `json:"resource"`
	}
	headers := map[string]string{
		"If-Match":        quoteResourceVersion(resourceVersion),
		"Idempotency-Key": mutationKey("refresh", kind, name, space, resourceVersion, expected),
	}
	responseHeaders, err := c.doJSONWithHeaders(
		ctx, http.MethodPost, c.actionResourceURL(kind, name, "refresh", query), headers, nil, &wrapped, true,
		http.StatusOK,
	)
	if err != nil {
		if isResourceNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	out := wrapped.Resource
	if err := verifyResourceIdentity(expected, name, space, &out); err != nil {
		return nil, err
	}
	if err := captureResourceVersion(&out, responseHeaders); err != nil {
		return nil, err
	}
	if err := requireExactResourceVersion(&out, resourceVersion); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteResource deletes a resource. resource_not_found is already deleted;
// a different stable error retains its typed API identity.
func (c *Client) DeleteResource(ctx context.Context, kind, name, space string, options ...MutationFence) error {
	if err := c.requireReady(); err != nil {
		return err
	}
	if err := ValidateSpaceID(space); err != nil {
		return fmt.Errorf("takoform: resource delete has invalid SpaceID: %w", err)
	}
	resourceVersion, form := mutationIdentity(options)
	headers := map[string]string{}
	if len(form) != 1 || !validResourceVersion(resourceVersion) {
		return errors.New("takoform: versioned delete requires one exact FormRef and resourceVersion")
	}
	if err := validateInstalledFormReference(kind, form[0]); err != nil {
		return err
	}
	query := exactResourceQuery(space, form[0])
	headers["If-Match"] = quoteResourceVersion(resourceVersion)
	headers["Idempotency-Key"] = mutationKey("delete", kind, name, space, resourceVersion, &form[0])
	if _, err := c.doJSONWithHeaders(
		ctx, http.MethodDelete, c.resourceURL(kind, name, query), headers, nil, nil, true,
		http.StatusNoContent,
	); err != nil {
		if isResourceNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

// PreviewResource plans one desired Resource at the negotiated API base.
func (c *Client) PreviewResource(ctx context.Context, body *Resource) (*PreviewResourceResult, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("takoform: preview requires a Resource body")
	}
	var out PreviewResourceResult
	headers := map[string]string{}
	if err := validateResourceIdentity(body.Kind, body); err != nil {
		return nil, err
	}
	if body.Metadata.ResourceVersion == "" {
		headers["If-None-Match"] = "*"
	} else {
		headers["If-Match"] = quoteResourceVersion(body.Metadata.ResourceVersion)
	}
	if _, err := c.doJSONWithHeaders(
		ctx, http.MethodPost, c.previewURL(), headers, body, &out, false,
		http.StatusOK,
	); err != nil {
		return nil, err
	}
	if err := validatePreviewResult(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// doJSON marshals body (if any), sends the request, and decodes a 2xx response
// into out (if any). Non-2xx responses are parsed into *APIError.
func (c *Client) doJSON(ctx context.Context, method, fullURL string, body, out any) error {
	_, err := c.doJSONWithHeaders(ctx, method, fullURL, nil, body, out, false, http.StatusOK)
	return err
}

func (c *Client) doJSONWithHeaders(
	ctx context.Context,
	method, fullURL string,
	headers map[string]string,
	body, out any,
	retryStableAPIError bool,
	successStatuses ...int,
) (http.Header, error) {
	var raw []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("takoform: encoding request body: %w", err)
		}
		raw = encoded
	}
	attempts := 1
	if retryStableAPIError {
		attempts = c.retryAttempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		var reader io.Reader
		if retryStableAPIError {
			// net/http treats Idempotency-Key as permission to replay a request
			// after some transport failures. Lifecycle mutations deliberately
			// use a one-shot body, including an empty one, so only this client's
			// stable structured-error policy can start another attempt.
			reader = struct{ io.Reader }{Reader: bytes.NewReader(raw)}
		} else if body != nil {
			reader = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
		if err != nil {
			return nil, fmt.Errorf("takoform: building request: %w", err)
		}
		if retryStableAPIError && body != nil {
			req.ContentLength = int64(len(raw))
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("takoform: request to %s failed: %w", fullURL, err)
		}
		if resp.ContentLength > maxResponseBodyBytes {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("takoform: response from %s exceeds %d bytes", fullURL, maxResponseBodyBytes)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("takoform: reading response body: %w", readErr)
		}
		if len(data) > maxResponseBodyBytes {
			return nil, fmt.Errorf("takoform: response from %s exceeds %d bytes", fullURL, maxResponseBodyBytes)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			apiErr := parseAPIError(resp.StatusCode, data)
			if retryStableAPIError && attempt+1 < attempts && isPortableRetryable(apiErr) {
				if err := waitForRetry(ctx, attempt); err != nil {
					return nil, err
				}
				continue
			}
			return nil, apiErr
		}
		if !containsStatus(successStatuses, resp.StatusCode) {
			return nil, fmt.Errorf(
				"takoform: response from %s returned unexpected success status %d",
				fullURL,
				resp.StatusCode,
			)
		}
		if resp.StatusCode == http.StatusNoContent && len(data) != 0 {
			return nil, fmt.Errorf(
				"takoform: response from %s returned a non-empty response body for HTTP 204",
				fullURL,
			)
		}

		if out != nil {
			if len(bytes.TrimSpace(data)) == 0 {
				return nil, fmt.Errorf(
					"takoform: response from %s returned an empty JSON response body",
					fullURL,
				)
			}
			if err := decodeStrictJSON(data, out); err != nil {
				return nil, fmt.Errorf("takoform: decoding response from %s: %w", fullURL, err)
			}
		}
		return resp.Header.Clone(), nil
	}
	return nil, errors.New("takoform: retry attempts exhausted")
}

func containsStatus(allowed []int, status int) bool {
	for _, candidate := range allowed {
		if status == candidate {
			return true
		}
	}
	return false
}

func decodeStrictJSON(data []byte, out any) error {
	return formpackage.DecodeStrictIJSON(data, out)
}

func waitForRetry(ctx context.Context, attempt int) error {
	delay := 25 * time.Millisecond * time.Duration(1<<attempt)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// parseAPIError decodes the nested error envelope
// ({ "error": { "code", "message", "requestId", "details" } }), falling back to
// the raw body when the response is not the expected JSON shape.
func parseAPIError(status int, data []byte) *APIError {
	apiErr := &APIError{StatusCode: status, ProtocolInvalid: true}
	if len(bytes.TrimSpace(data)) > 0 {
		var env errorEnvelope
		if err := decodeStrictJSON(data, &env); err == nil &&
			strings.TrimSpace(env.Error.Code) != "" &&
			strings.TrimSpace(env.Error.Message) != "" &&
			strings.TrimSpace(env.Error.RequestID) != "" &&
			env.Error.Retryable != nil &&
			isStableErrorEnvelope(status, env.Error.Code, *env.Error.Retryable) {
			apiErr.Code = env.Error.Code
			apiErr.Message = env.Error.Message
			apiErr.RequestID = env.Error.RequestID
			apiErr.Retryable = *env.Error.Retryable
			apiErr.HostCode = env.Error.HostCode
			apiErr.Details = env.Error.Details
			apiErr.ProtocolInvalid = false
		}
	}
	if apiErr.Message == "" {
		if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
			apiErr.Message = trimmed
		} else {
			apiErr.Message = http.StatusText(status)
		}
	}
	return apiErr
}

func isStableErrorEnvelope(status int, code string, retryable bool) bool {
	expectedStatus, known := stableErrorHTTPStatusByCode[code]
	if !known || status != expectedStatus {
		return false
	}
	if retryable {
		_, allowed := automaticallyRetryableErrorCodes[code]
		return allowed
	}
	return true
}

func isPortableRetryable(apiErr *APIError) bool {
	if apiErr == nil || apiErr.ProtocolInvalid || !apiErr.Retryable {
		return false
	}
	return isStableErrorEnvelope(apiErr.StatusCode, apiErr.Code, true)
}

func (c *Client) requireReady() error {
	if c.apiBase == "" {
		return errors.New("takoform: Discover must complete before using the Resource API")
	}
	return nil
}

func (c *Client) resourceMutationHeaders(operation string, resource *Resource, request any) map[string]string {
	headers := map[string]string{}
	if resource.Metadata.ResourceVersion == "" {
		headers["If-None-Match"] = "*"
	} else {
		headers["If-Match"] = quoteResourceVersion(resource.Metadata.ResourceVersion)
	}
	headers["Idempotency-Key"] = mutationKey(
		operation,
		resource.Kind,
		resource.Metadata.Name,
		resource.Metadata.Space,
		resource.Metadata.ResourceVersion,
		request,
	)
	return headers
}

func quoteResourceVersion(version string) string {
	return `"` + version + `"`
}

func mutationKey(values ...any) string {
	raw, _ := json.Marshal(values)
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("takoform-%x", digest[:])
}

func validateResourceIdentity(kind string, resource *Resource) error {
	if resource == nil || resource.Form == nil {
		return errors.New("takoform: versioned Resource requires an exact FormRef")
	}
	if resource.APIVersion != APIVersion || resource.Kind != kind || resource.Form.FormRef.Kind != kind {
		return errors.New("takoform: Resource and exact FormRef identities do not match")
	}
	if err := ValidateResourceName(resource.Metadata.Name); err != nil {
		return fmt.Errorf("takoform: Resource metadata.name is invalid: %w", err)
	}
	if err := ValidateSpaceID(resource.Metadata.Space); err != nil {
		return fmt.Errorf("takoform: Resource metadata.space has invalid SpaceID: %w", err)
	}
	if err := validateInstalledFormReference(kind, *resource.Form); err != nil {
		return err
	}
	if resource.Metadata.ResourceVersion != "" && !validResourceVersion(resource.Metadata.ResourceVersion) {
		return errors.New("takoform: resourceVersion must be canonical decimal in the range 1..9223372036854775807")
	}
	return nil
}

func validateInstalledFormReference(kind string, form InstalledFormReference) error {
	raw, err := json.Marshal(form.FormRef)
	if err != nil {
		return fmt.Errorf("takoform: encoding exact FormRef for validation: %w", err)
	}
	if _, err := formpackage.ValidateFormRef(raw); err != nil {
		return fmt.Errorf("takoform: exact FormRef is invalid: %w", err)
	}
	if kind == "" || form.FormRef.Kind != kind || !formpackage.ValidDigest(form.PackageDigest) {
		return errors.New("takoform: exact InstalledFormReference is incomplete or invalid")
	}
	return nil
}

func validSHA256Digest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") ||
		value != strings.ToLower(value) ||
		len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validResourceVersion(value string) bool {
	if value == "" || value[0] == '0' {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	generation, err := strconv.ParseInt(value, 10, 64)
	return err == nil && generation > 0
}

func verifyResourceIdentity(expected *InstalledFormReference, expectedName, expectedSpace string, resource *Resource) error {
	if expected == nil || resource == nil || resource.Form == nil || !sameForm(expected, resource.Form) {
		return errors.New("takoform: host response changed the exact FormRef/package identity")
	}
	if resource.APIVersion != APIVersion || resource.Kind != expected.FormRef.Kind {
		return errors.New("takoform: host response changed the Resource identity")
	}
	if resource.Metadata.Name != expectedName || resource.Metadata.Space != expectedSpace {
		return errors.New("takoform: host response changed the requested Resource name or space")
	}
	return nil
}

func sameForm(left, right *InstalledFormReference) bool {
	return left != nil && right != nil &&
		left.FormRef.APIVersion == right.FormRef.APIVersion &&
		left.FormRef.Kind == right.FormRef.Kind &&
		left.FormRef.DefinitionVersion == right.FormRef.DefinitionVersion &&
		left.FormRef.SchemaDigest == right.FormRef.SchemaDigest &&
		left.PackageDigest == right.PackageDigest
}

func validatePreviewResult(request *Resource, preview *PreviewResourceResult) error {
	if request == nil || preview == nil {
		return errors.New("takoform: host preview omitted the reviewed Resource")
	}
	if err := verifyResourceIdentity(
		request.Form,
		request.Metadata.Name,
		request.Metadata.Space,
		&preview.Resource,
	); err != nil {
		return fmt.Errorf("takoform: host preview changed the requested Resource: %w", err)
	}
	if preview.Resource.Metadata.ResourceVersion != request.Metadata.ResourceVersion {
		return errors.New("takoform: host preview changed the Resource generation fence")
	}
	if preview.Resource.Status != nil {
		return errors.New("takoform: host preview returned status outside the portable review envelope")
	}

	requestSpecDigest, err := canonicalValueDigest(request.Spec)
	if err != nil {
		return fmt.Errorf("takoform: digesting requested spec: %w", err)
	}
	previewSpecDigest, err := canonicalValueDigest(preview.Resource.Spec)
	if err != nil {
		return fmt.Errorf("takoform: digesting previewed spec: %w", err)
	}
	if previewSpecDigest != requestSpecDigest {
		return errors.New("takoform: host preview changed the requested spec")
	}
	if preview.Review.SpecDigest != requestSpecDigest {
		return errors.New("takoform: host preview returned a specDigest for different desired state")
	}
	if !validSHA256Digest(preview.Review.PlanDigest) {
		return errors.New("takoform: host preview returned an invalid planDigest")
	}
	return nil
}

func canonicalValueDigest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return formpackage.DigestCanonicalJSON(raw)
}

func captureResourceVersion(resource *Resource, headers http.Header) error {
	if resource == nil {
		return errors.New("takoform: host response omitted the Resource")
	}
	bodyVersion := resource.Metadata.ResourceVersion
	if !validResourceVersion(bodyVersion) {
		return errors.New("takoform: host response omitted resourceVersion or returned a value outside 1..9223372036854775807")
	}
	etagValues := headers.Values("ETag")
	if len(etagValues) != 1 {
		return errors.New("takoform: host response must return exactly one ETag resourceVersion fence")
	}
	if etagValues[0] != quoteResourceVersion(bodyVersion) {
		return errors.New("takoform: host response resourceVersion and ETag disagree")
	}
	return nil
}

func requireExactResourceVersion(resource *Resource, expected string) error {
	if resource == nil || !validResourceVersion(expected) ||
		resource.Metadata.ResourceVersion != expected {
		return errors.New("takoform: host response changed the generation protected by If-Match")
	}
	return nil
}

func mutationIdentity(options []MutationFence) (string, []InstalledFormReference) {
	if len(options) != 1 {
		return "", nil
	}
	return options[0].ResourceVersion, []InstalledFormReference{options[0].Form}
}
