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
	"strings"
	"time"
)

// API constants for the frozen wire contract.
const (
	// APIVersion is the Resource object apiVersion this provider speaks.
	APIVersion = "forms.takoform.com/v1alpha1"

	// KindEdgeWorker is the Service Form kind for HTTP services.
	KindEdgeWorker = "EdgeWorker"

	KindObjectBucket           = "ObjectBucket"
	KindKVStore                = "KVStore"
	KindQueue                  = "Queue"
	KindSQLDatabase            = "SQLDatabase"
	KindContainerService       = "ContainerService"
	KindVectorIndex            = "VectorIndex"
	KindDurableWorkflow        = "DurableWorkflow"
	KindStatefulActorNamespace = "StatefulActorNamespace"
	KindSchedule               = "Schedule"

	// ManagedByOpenTofu is used only by the explicit legacy /v1 compatibility
	// fallback. The versioned Form host owns manager metadata.
	ManagedByOpenTofu = "opentofu"

	defaultUserAgent     = "terraform-provider-takoform"
	maxResponseBodyBytes = 8 * 1024 * 1024
)

// ErrNotFound is returned when a resource read targets a resource that the
// server reports as gone (HTTP 404). Callers map this to "remove from state".
var ErrNotFound = errors.New("takoform: resource not found")

// Discovery is the parsed body of GET /.well-known/takoform.
//
// Features is intentionally a map so the provider stays capability-driven
// (it inspects named capabilities) rather than edition-driven (it never
// branches on an "edition" string).
type Discovery struct {
	APIVersions []string        `json:"api_versions"`
	Edition     string          `json:"edition,omitempty"`
	Features    map[string]bool `json:"features"`
	Endpoints   Endpoints       `json:"endpoints"`
}

// Endpoints carries advertised service URLs from discovery.
type Endpoints struct {
	API              string `json:"api,omitempty"`
	Forms            string `json:"forms,omitempty"`
	Capabilities     string `json:"capabilities,omitempty"`
	CompatibilityAPI string `json:"compatibility_api,omitempty"`
	OIDCIssuer       string `json:"oidc_issuer,omitempty"`
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
	Name            string            `json:"name"`
	Space           string            `json:"space,omitempty"`
	Project         string            `json:"project,omitempty"`
	Environment     string            `json:"environment,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	ManagedBy       string            `json:"managedBy,omitempty"`
	// ID may be returned by the server inside metadata; the provider also
	// accepts a top-level Resource.ID. Either, if present, wins over the
	// synthesized tkrn id.
	ID string `json:"id,omitempty"`
}

// Resolution is the resolver's chosen implementation/target for a resource.
type Resolution struct {
	SelectedImplementation string `json:"selectedImplementation,omitempty"`
	Target                 string `json:"target,omitempty"`
	Locked                 bool   `json:"locked,omitempty"`
	Portability            string `json:"portability,omitempty"`
}

// Condition is a Kubernetes-style status condition.
type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Status is the observed state returned by the server on PUT/GET/preview.
type Status struct {
	Phase              string         `json:"phase,omitempty"`
	ObservedGeneration int64          `json:"observedGeneration,omitempty"`
	Portability        string         `json:"portability,omitempty"`
	Resolution         Resolution     `json:"resolution"`
	Outputs            map[string]any `json:"outputs,omitempty"`
	Conditions         []Condition    `json:"conditions,omitempty"`
	// DriftStatus is transport evidence from the versioned observe response.
	// It is intentionally not part of the Resource wire envelope.
	DriftStatus string `json:"-"`
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
	// ID is an optional top-level server identifier.
	ID string `json:"id,omitempty"`
}

// FormRef pins one immutable typed Form Definition. Publication and admission
// are external to this value.
type FormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
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

// NativeResourceRef is an opaque host-side native-resource handle returned in
// preview evidence. The provider never creates or selects this object.
type NativeResourceRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// PreviewResourceResult decodes the versioned preview response and the
// historical compatibility response without exposing either as provider state.
type PreviewResourceResult struct {
	Resource               Resource            `json:"resource"`
	SelectedImplementation string              `json:"selectedImplementation"`
	SelectedTarget         string              `json:"selectedTarget"`
	Portability            string              `json:"portability"`
	NativeResourcePlan     []NativeResourceRef `json:"nativeResourcePlan"`
	RiskNotes              []string            `json:"riskNotes"`
	Summary                string              `json:"summary"`
	PlanDigest             string              `json:"planDigest"`
	SpecDigest             string              `json:"specDigest"`
	ResolutionFingerprint  string              `json:"resolutionFingerprint"`
	Quote                  *DeploymentQuote    `json:"quote,omitempty"`
	Review                 PreviewReview       `json:"review,omitempty"`
}

type PreviewReview struct {
	PlanDigest string `json:"planDigest"`
	SpecDigest string `json:"specDigest"`
}

// DeploymentQuote is the immutable price snapshot attached to a Deploy API
// preview by a commercial host. OSS endpoints can omit it.
type DeploymentQuote struct {
	QuoteID                 string `json:"quoteId"`
	QuoteDigest             string `json:"quoteDigest"`
	RatingStatus            string `json:"ratingStatus"`
	Currency                string `json:"currency"`
	EstimatedTotalUSDmicros int64  `json:"estimatedTotalUsdMicros"`
	ExpiresAt               string `json:"expiresAt"`
}

// DeploymentReview presents the exact preview evidence accepted by apply.
// Quote evidence is required only when the host returned a priced quote.
type DeploymentReview struct {
	PlanDigest  string `json:"planDigest"`
	QuoteID     string `json:"quoteId,omitempty"`
	QuoteDigest string `json:"quoteDigest,omitempty"`
}

type applyResourceBody struct {
	Resource
	Review DeploymentReview `json:"review"`
}

// ProductCapabilities is the parsed body of GET /v1/capabilities.
type ProductCapabilities struct {
	APIVersion string          `json:"apiVersion"`
	Resources  map[string]bool `json:"resources"`
	Adapters   map[string]bool `json:"adapters"`
	Compat     map[string]bool `json:"compat"`
	Identity   map[string]bool `json:"identity"`
	Commercial map[string]bool `json:"commercial"`
}

// SupportsResource reports whether a Service Form kind is advertised.
func (p ProductCapabilities) SupportsResource(kind string) bool {
	return p.Resources[kind]
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
	// Details is the optional, free-form details payload, kept raw.
	Details json.RawMessage
}

// errorEnvelope decodes the nested wire shape of an error response.
type errorEnvelope struct {
	Error struct {
		Code      string          `json:"code"`
		Message   string          `json:"message"`
		RequestID string          `json:"requestId"`
		Retryable bool            `json:"retryable"`
		HostCode  string          `json:"hostCode,omitempty"`
		Details   json.RawMessage `json:"details,omitempty"`
	} `json:"error"`
}

func (e *APIError) Error() string {
	var b strings.Builder
	b.WriteString("takoform api error")
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

// statusCode reports the HTTP status carried by err, if it is an *APIError.
func statusCode(err error) (int, bool) {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.StatusCode, true
	}
	return 0, false
}

// Client is a thin Takoform Service Form API HTTP client.
type Client struct {
	endpoint                   string // normalized origin, no trailing slash
	token                      string
	httpClient                 *http.Client
	userAgent                  string
	apiBase                    string
	formsURL                   string
	compatibilityAPI           string
	allowCompatibilityFallback bool
	compatibilityFallback      bool
	retryAttempts              int

	// Discovery is populated by Discover and cached for capability checks.
	Discovery    Discovery
	Capabilities ProductCapabilities
}

// Options controls the one intentionally non-default compatibility lane.
type Options struct {
	AllowCompatibilityFallback bool
	RetryAttempts              int
}

// New constructs a Client. If httpClient is nil, http.DefaultClient is used.
func New(endpoint, token string, httpClient *http.Client) *Client {
	return NewWithOptions(endpoint, token, httpClient, Options{})
}

// NewCompatibilityFallback constructs the explicit historical /v1 client.
// Provider configuration never calls this implicitly.
func NewCompatibilityFallback(endpoint, token string, httpClient *http.Client) *Client {
	return NewWithOptions(endpoint, token, httpClient, Options{AllowCompatibilityFallback: true})
}

// NewWithOptions constructs a client. The historical /v1 API is never chosen
// unless AllowCompatibilityFallback is explicitly true.
func NewWithOptions(endpoint, token string, httpClient *http.Client, options Options) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	retryAttempts := options.RetryAttempts
	if retryAttempts <= 0 {
		retryAttempts = 3
	}
	client := &Client{
		endpoint:                   strings.TrimRight(endpoint, "/"),
		token:                      token,
		httpClient:                 httpClient,
		userAgent:                  defaultUserAgent,
		allowCompatibilityFallback: options.AllowCompatibilityFallback,
		retryAttempts:              retryAttempts,
	}
	if options.AllowCompatibilityFallback {
		client.compatibilityFallback = true
		client.compatibilityAPI = client.endpoint + "/v1"
		client.apiBase = client.compatibilityAPI
	}
	return client
}

// Endpoint returns the normalized endpoint origin.
func (c *Client) Endpoint() string { return c.endpoint }

// UsesCompatibilityFallback reports the explicit historical /v1 mode.
func (c *Client) UsesCompatibilityFallback() bool { return c.compatibilityFallback }

// Discover performs GET {endpoint}/.well-known/takoform and caches the result.
func (c *Client) Discover(ctx context.Context) (Discovery, error) {
	var disco Discovery
	if _, err := c.configuredOrigin(); err != nil {
		return Discovery{}, err
	}
	if err := c.doJSON(ctx, http.MethodGet, c.endpoint+"/.well-known/takoform", nil, &disco); err != nil {
		return Discovery{}, err
	}
	c.Discovery = disco
	if err := c.negotiateEndpoints(disco); err != nil {
		return Discovery{}, err
	}
	return disco, nil
}

func (c *Client) negotiateEndpoints(disco Discovery) error {
	c.compatibilityFallback = false
	c.apiBase = ""
	c.formsURL = ""
	if !disco.SupportsServiceForms() {
		return errors.New("takoform: discovery does not advertise features.service_forms")
	}
	if !containsString(disco.APIVersions, APIVersion) {
		return fmt.Errorf("takoform: discovery does not advertise API version %s", APIVersion)
	}
	if strings.TrimSpace(disco.Endpoints.API) == "" {
		if !c.allowCompatibilityFallback {
			return errors.New("takoform: discovery omitted endpoints.api; set compatibility_fallback explicitly to use historical /v1")
		}
		c.compatibilityFallback = true
		c.compatibilityAPI = strings.TrimRight(c.endpoint, "/") + "/v1"
		if disco.Endpoints.CompatibilityAPI != "" {
			resolved, err := c.validAdvertisedEndpoint(disco.Endpoints.CompatibilityAPI)
			if err != nil {
				return fmt.Errorf("takoform: invalid discovery compatibility endpoint: %w", err)
			}
			c.compatibilityAPI = strings.TrimRight(resolved, "/")
		}
		c.apiBase = c.compatibilityAPI
		return nil
	}
	for _, feature := range []string{"exact_form_ref", "optimistic_concurrency", "idempotent_lifecycle"} {
		if !disco.HasFeature(feature) {
			return fmt.Errorf("takoform: versioned discovery does not advertise features.%s", feature)
		}
	}

	apiBase, err := c.validAdvertisedEndpoint(disco.Endpoints.API)
	if err != nil {
		return fmt.Errorf("takoform: invalid discovery API endpoint: %w", err)
	}
	c.apiBase = strings.TrimRight(apiBase, "/")
	c.formsURL = c.apiBase + "/forms"
	if disco.Endpoints.Forms != "" {
		formsURL, err := c.validAdvertisedEndpoint(disco.Endpoints.Forms)
		if err != nil {
			return fmt.Errorf("takoform: invalid discovery forms endpoint: %w", err)
		}
		c.formsURL = strings.TrimRight(formsURL, "/")
	}
	if disco.Endpoints.CompatibilityAPI != "" {
		compatibilityAPI, err := c.validAdvertisedEndpoint(disco.Endpoints.CompatibilityAPI)
		if err != nil {
			return fmt.Errorf("takoform: invalid discovery compatibility endpoint: %w", err)
		}
		c.compatibilityAPI = strings.TrimRight(compatibilityAPI, "/")
	}
	return nil
}

func (c *Client) validAdvertisedEndpoint(raw string) (string, error) {
	advertised, err := url.Parse(raw)
	if err != nil || !advertised.IsAbs() || advertised.Host == "" || (advertised.Scheme != "http" && advertised.Scheme != "https") {
		return "", fmt.Errorf("endpoint must be an absolute URL")
	}
	configured, err := c.configuredOrigin()
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(advertised.Scheme, configured.Scheme) || !strings.EqualFold(advertised.Host, configured.Host) {
		return "", errors.New("cross-origin discovery endpoints are rejected to protect bearer credentials")
	}
	if advertised.User != nil || advertised.Fragment != "" || advertised.RawQuery != "" {
		return "", errors.New("endpoint must not contain userinfo, query, or fragment")
	}
	return advertised.String(), nil
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

// GetCapabilities performs GET {endpoint}/v1/capabilities and caches the result.
func (c *Client) GetCapabilities(ctx context.Context) (ProductCapabilities, error) {
	if !c.compatibilityFallback {
		return ProductCapabilities{}, errors.New("takoform: /v1 capabilities are available only in explicit compatibility fallback mode")
	}
	fullURL := c.compatibilityAPI + "/capabilities"
	if c.Discovery.Endpoints.Capabilities != "" {
		resolved, err := c.validAdvertisedEndpoint(c.Discovery.Endpoints.Capabilities)
		if err != nil {
			return ProductCapabilities{}, fmt.Errorf("takoform: invalid discovery capabilities endpoint: %w", err)
		}
		fullURL = resolved
	}
	var caps ProductCapabilities
	if err := c.doJSON(ctx, http.MethodGet, fullURL, nil, &caps); err != nil {
		return ProductCapabilities{}, err
	}
	c.Capabilities = caps
	return caps, nil
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

func (c *Client) EnsureFormAvailable(ctx context.Context, space string, form InstalledFormReference, operation string) error {
	if c.compatibilityFallback {
		return nil
	}
	if err := validateInstalledFormReference(form.FormRef.Kind, form); err != nil {
		return err
	}
	if strings.TrimSpace(space) == "" {
		return errors.New("takoform: exact FormRef availability requires a space")
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
// that plan and optional quote evidence to PUT. Backend selection and pricing
// remain server-side concerns.
func (c *Client) PutResource(ctx context.Context, kind, name string, body *Resource) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("takoform: apply requires a Resource body")
	}
	if !c.compatibilityFallback {
		if err := validateResourceIdentity(kind, body); err != nil {
			return nil, err
		}
		if body.Metadata.Name != name {
			return nil, errors.New("takoform: Resource metadata.name does not match the requested URL name")
		}
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
	if c.compatibilityFallback {
		transportResource.Form = nil
		transportResource.Metadata.ResourceVersion = ""
	}
	preview, err := c.PreviewResource(ctx, &transportResource)
	if err != nil {
		return nil, err
	}
	planDigest := preview.PlanDigest
	if !c.compatibilityFallback {
		planDigest = preview.Review.PlanDigest
	}
	if strings.TrimSpace(planDigest) == "" {
		return nil, errors.New("takoform: Deploy API preview omitted planDigest")
	}
	review := DeploymentReview{PlanDigest: planDigest}
	if c.compatibilityFallback && preview.Quote != nil {
		if strings.TrimSpace(preview.Quote.QuoteID) == "" || strings.TrimSpace(preview.Quote.QuoteDigest) == "" {
			return nil, errors.New("takoform: Deploy API preview returned incomplete quote evidence")
		}
		review.QuoteID = preview.Quote.QuoteID
		review.QuoteDigest = preview.Quote.QuoteDigest
	}

	var out Resource
	request := applyResourceBody{Resource: transportResource, Review: review}
	headers := c.resourceMutationHeaders("apply", body, request)
	responseHeaders, err := c.doJSONWithHeaders(ctx, http.MethodPut, c.putResourceURL(kind, name), headers, &request, &out, !c.compatibilityFallback)
	if err != nil {
		return nil, err
	}
	if !c.compatibilityFallback {
		if err := verifyResourceIdentity(body.Form, name, body.Metadata.Space, &out); err != nil {
			return nil, err
		}
		if err := captureResourceVersion(&out, responseHeaders); err != nil {
			return nil, err
		}
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
	var out Resource
	request := importResourceBody{Resource: *body, NativeID: nativeID}
	if c.compatibilityFallback {
		request.Form = nil
		request.Metadata.ResourceVersion = ""
		if err := c.doJSON(ctx, http.MethodPost, c.importResourceURL(kind, name), &request, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
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
	responseHeaders, err := c.doJSONWithHeaders(ctx, http.MethodPost, c.importResourceURL(kind, name), headers, &request, &wrapped, true)
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

// GetResource reads a resource. A 404 is translated to ErrNotFound.
func (c *Client) GetResource(ctx context.Context, kind, name, space string, form ...InstalledFormReference) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	query := spaceQuery(space)
	var expected *InstalledFormReference
	if !c.compatibilityFallback {
		if len(form) != 1 {
			return nil, errors.New("takoform: versioned Resource read requires one exact FormRef")
		}
		if err := validateInstalledFormReference(kind, form[0]); err != nil {
			return nil, err
		}
		expected = &form[0]
		query = exactResourceQuery(space, form[0])
	}
	var out Resource
	responseHeaders, err := c.doJSONWithHeaders(ctx, http.MethodGet, c.resourceURL(kind, name, query), nil, nil, &out, false)
	if err != nil {
		if code, ok := statusCode(err); ok && code == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if expected != nil {
		if err := verifyResourceIdentity(expected, name, space, &out); err != nil {
			return nil, err
		}
		if err := captureResourceVersion(&out, responseHeaders); err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// ObserveResource performs a read-only backend drift check and returns the
// Resource projection with updated conditions. A 404 is translated to
// ErrNotFound, matching GetResource.
func (c *Client) ObserveResource(ctx context.Context, kind, name, space string, options ...MutationFence) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	query := spaceQuery(space)
	var expected *InstalledFormReference
	resourceVersion, form := mutationIdentity(options)
	if !c.compatibilityFallback {
		if len(form) != 1 || !validResourceVersion(resourceVersion) {
			return nil, errors.New("takoform: versioned observe requires one exact FormRef and resourceVersion")
		}
		if err := validateInstalledFormReference(kind, form[0]); err != nil {
			return nil, err
		}
		expected = &form[0]
		query = exactResourceQuery(space, form[0])
	}
	var out Resource
	var target any = &out
	var wrapped struct {
		Resource    Resource `json:"resource"`
		Observation struct {
			Status string `json:"status"`
		} `json:"observation"`
	}
	if expected != nil {
		target = &wrapped
	}
	headers := map[string]string{}
	if expected != nil {
		headers["If-Match"] = quoteResourceVersion(resourceVersion)
		headers["Idempotency-Key"] = mutationKey("observe", kind, name, space, resourceVersion, expected)
	}
	responseHeaders, err := c.doJSONWithHeaders(ctx, http.MethodPost, c.actionResourceURL(kind, name, "observe", query), headers, nil, target, !c.compatibilityFallback)
	if err != nil {
		if code, ok := statusCode(err); ok && code == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if expected != nil {
		out = wrapped.Resource
		if wrapped.Observation.Status != "current" && wrapped.Observation.Status != "drifted" && wrapped.Observation.Status != "missing" {
			return nil, errors.New("takoform: host observe response omitted a valid observation status")
		}
		if out.Status == nil {
			out.Status = &Status{}
		}
		out.Status.DriftStatus = wrapped.Observation.Status
		if err := verifyResourceIdentity(expected, name, space, &out); err != nil {
			return nil, err
		}
		if err := captureResourceVersion(&out, responseHeaders); err != nil {
			return nil, err
		}
	}
	if expected == nil && out.Status != nil {
		out.Status.DriftStatus = driftStatusFromConditions(out.Status.Conditions)
	}
	return &out, nil
}

// RefreshResource updates the Resource-owned backend state and public outputs
// without mutating native provider resources. A 404 is translated to
// ErrNotFound, matching GetResource and ObserveResource.
func (c *Client) RefreshResource(ctx context.Context, kind, name, space string, options ...MutationFence) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	query := spaceQuery(space)
	var expected *InstalledFormReference
	resourceVersion, form := mutationIdentity(options)
	if !c.compatibilityFallback {
		if len(form) != 1 || !validResourceVersion(resourceVersion) {
			return nil, errors.New("takoform: versioned refresh requires one exact FormRef and resourceVersion")
		}
		if err := validateInstalledFormReference(kind, form[0]); err != nil {
			return nil, err
		}
		expected = &form[0]
		query = exactResourceQuery(space, form[0])
	}
	var out Resource
	var target any = &out
	var wrapped struct {
		Resource Resource `json:"resource"`
	}
	if expected != nil {
		target = &wrapped
	}
	headers := map[string]string{}
	if expected != nil {
		headers["If-Match"] = quoteResourceVersion(resourceVersion)
		headers["Idempotency-Key"] = mutationKey("refresh", kind, name, space, resourceVersion, expected)
	}
	responseHeaders, err := c.doJSONWithHeaders(ctx, http.MethodPost, c.actionResourceURL(kind, name, "refresh", query), headers, nil, target, !c.compatibilityFallback)
	if err != nil {
		if code, ok := statusCode(err); ok && code == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if expected != nil {
		out = wrapped.Resource
		if err := verifyResourceIdentity(expected, name, space, &out); err != nil {
			return nil, err
		}
		if err := captureResourceVersion(&out, responseHeaders); err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// DeleteResource deletes a resource. 200/204 => done; a 404 is treated as
// already-deleted (no error).
func (c *Client) DeleteResource(ctx context.Context, kind, name, space string, options ...MutationFence) error {
	if err := c.requireReady(); err != nil {
		return err
	}
	query := spaceQuery(space)
	resourceVersion, form := mutationIdentity(options)
	if query == nil {
		query = url.Values{}
	}
	headers := map[string]string{}
	if c.compatibilityFallback {
		query.Set("managedBy", ManagedByOpenTofu)
	} else {
		if len(form) != 1 || !validResourceVersion(resourceVersion) {
			return errors.New("takoform: versioned delete requires one exact FormRef and resourceVersion")
		}
		if err := validateInstalledFormReference(kind, form[0]); err != nil {
			return err
		}
		query = exactResourceQuery(space, form[0])
		headers["If-Match"] = quoteResourceVersion(resourceVersion)
		headers["Idempotency-Key"] = mutationKey("delete", kind, name, space, resourceVersion, &form[0])
	}
	if _, err := c.doJSONWithHeaders(ctx, http.MethodDelete, c.resourceURL(kind, name, query), headers, nil, nil, !c.compatibilityFallback); err != nil {
		if code, ok := statusCode(err); ok && code == http.StatusNotFound {
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
	if !c.compatibilityFallback {
		if err := validateResourceIdentity(body.Kind, body); err != nil {
			return nil, err
		}
		if body.Metadata.ResourceVersion == "" {
			headers["If-None-Match"] = "*"
		} else {
			headers["If-Match"] = quoteResourceVersion(body.Metadata.ResourceVersion)
		}
	}
	if _, err := c.doJSONWithHeaders(ctx, http.MethodPost, c.previewURL(), headers, body, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

// doJSON marshals body (if any), sends the request, and decodes a 2xx response
// into out (if any). Non-2xx responses are parsed into *APIError.
func (c *Client) doJSON(ctx context.Context, method, fullURL string, body, out any) error {
	_, err := c.doJSONWithHeaders(ctx, method, fullURL, nil, body, out, false)
	return err
}

func (c *Client) doJSONWithHeaders(
	ctx context.Context,
	method, fullURL string,
	headers map[string]string,
	body, out any,
	retry bool,
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
	if retry {
		attempts = c.retryAttempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
		if err != nil {
			return nil, fmt.Errorf("takoform: building request: %w", err)
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
			if retry && attempt+1 < attempts {
				if err := waitForRetry(ctx, attempt); err != nil {
					return nil, err
				}
				continue
			}
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
			if retry && attempt+1 < attempts && apiErr.Retryable && (apiErr.Code == "resource_busy" || apiErr.Code == "backend_unavailable") {
				if err := waitForRetry(ctx, attempt); err != nil {
					return nil, err
				}
				continue
			}
			return nil, apiErr
		}

		if out != nil && len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return nil, fmt.Errorf("takoform: decoding response from %s: %w", fullURL, err)
			}
		}
		return resp.Header.Clone(), nil
	}
	return nil, errors.New("takoform: retry attempts exhausted")
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
	apiErr := &APIError{StatusCode: status}
	if len(bytes.TrimSpace(data)) > 0 {
		var env errorEnvelope
		if err := json.Unmarshal(data, &env); err == nil {
			apiErr.Code = env.Error.Code
			apiErr.Message = env.Error.Message
			apiErr.RequestID = env.Error.RequestID
			apiErr.Retryable = env.Error.Retryable
			apiErr.HostCode = env.Error.HostCode
			apiErr.Details = env.Error.Details
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

func (c *Client) requireReady() error {
	if c.apiBase == "" {
		return errors.New("takoform: Discover must complete before using the Resource API")
	}
	return nil
}

func (c *Client) resourceMutationHeaders(operation string, resource *Resource, request any) map[string]string {
	if c.compatibilityFallback {
		return nil
	}
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
	if resource.APIVersion != APIVersion || resource.Kind != kind || resource.Form.FormRef.APIVersion != APIVersion || resource.Form.FormRef.Kind != kind {
		return errors.New("takoform: Resource and exact FormRef identities do not match")
	}
	if strings.TrimSpace(resource.Metadata.Name) == "" || strings.TrimSpace(resource.Metadata.Space) == "" {
		return errors.New("takoform: versioned Resource requires metadata.name and metadata.space")
	}
	if err := validateInstalledFormReference(kind, *resource.Form); err != nil {
		return err
	}
	if resource.Metadata.ResourceVersion != "" && !validResourceVersion(resource.Metadata.ResourceVersion) {
		return errors.New("takoform: resourceVersion must be a positive decimal generation")
	}
	return nil
}

func validateInstalledFormReference(kind string, form InstalledFormReference) error {
	if kind == "" || form.FormRef.APIVersion != APIVersion || form.FormRef.Kind != kind ||
		strings.TrimSpace(form.FormRef.DefinitionVersion) == "" || !validSHA256Digest(form.FormRef.SchemaDigest) ||
		!validSHA256Digest(form.PackageDigest) {
		return errors.New("takoform: exact InstalledFormReference is incomplete or invalid")
	}
	return nil
}

func validSHA256Digest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
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
	return true
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

func driftStatusFromConditions(conditions []Condition) string {
	for _, condition := range conditions {
		if condition.Type != "Drifted" {
			continue
		}
		switch strings.ToLower(condition.Status) {
		case "true":
			return "drifted"
		case "false":
			return "current"
		}
	}
	return ""
}

func captureResourceVersion(resource *Resource, headers http.Header) error {
	if resource == nil {
		return errors.New("takoform: host response omitted the Resource")
	}
	bodyVersion := resource.Metadata.ResourceVersion
	if bodyVersion != "" && !validResourceVersion(bodyVersion) {
		return errors.New("takoform: host response returned an invalid resourceVersion")
	}
	etag := strings.TrimSpace(headers.Get("ETag"))
	etagVersion := ""
	if len(etag) >= 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
		etagVersion = etag[1 : len(etag)-1]
		if !validResourceVersion(etagVersion) {
			return errors.New("takoform: host response returned an invalid ETag resourceVersion")
		}
	} else if etag != "" {
		return errors.New("takoform: host response returned an unquoted ETag resourceVersion")
	}
	if bodyVersion != "" && etagVersion != "" && bodyVersion != etagVersion {
		return errors.New("takoform: host response resourceVersion and ETag disagree")
	}
	if bodyVersion == "" {
		bodyVersion = etagVersion
		resource.Metadata.ResourceVersion = bodyVersion
	}
	if bodyVersion == "" {
		return errors.New("takoform: host response omitted the resourceVersion fence")
	}
	return nil
}

func mutationIdentity(options []MutationFence) (string, []InstalledFormReference) {
	if len(options) != 1 {
		return "", nil
	}
	return options[0].ResourceVersion, []InstalledFormReference{options[0].Form}
}
