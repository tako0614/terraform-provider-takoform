// Package clientv3 is the Host API v1alpha3 client lane
// (forms.takoform.com/v1alpha3, spec/decisions/0013). It is deliberately
// transport-only: it speaks the v1alpha3 resource envelope
// (apiVersion/kind/form/metadata/spec/status) over I-JSON, carries the
// UID/generation/revision identity fences of spec/decisions/0011, polls
// long-running Operations, uploads content-addressed artifacts, and reads
// Host Support Profiles. It never selects a backend and never manages
// credentials.
//
// # Namespaced groups travel as two ordinary path segments
//
// v1alpha3 Form groups are namespaced apiVersions such as
// "edge.forms.takoform.com/v1alpha1", which contain a slash. Wherever a URL
// path template names a group, this client sends the group NAME and the group
// VERSION as two separate, ordinary path segments:
//
//	/apis/forms.takoform.com/v1alpha3/resources/edge.forms.takoform.com/v1alpha1/ModuleWorker/app
//
// No path segment this client builds ever percent-encodes a slash. Proxies,
// gateways, and web frameworks disagree about whether %2F inside a path
// segment is passed through, decoded, rejected, or normalized, so a lane that
// required it could not be deployed behind ordinary infrastructure
// (spec/decisions/0018). The exact FormRef apiVersion string is unchanged
// everywhere else: request bodies, responses, and the "group" query key still
// carry "edge.forms.takoform.com/v1alpha1" verbatim.
//
// The retained v1alpha2 client in internal/client is frozen; this lane
// shares its proven mechanics (strict I-JSON decoding, same-origin endpoint
// negotiation, one-shot request bodies, retry only on complete stable error
// envelopes, 8MiB response cap) but none of its wire vocabulary.
package clientv3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// API constants for the v1alpha3 lane. The retained v1alpha1/v1alpha2 lanes
// keep their own discovery paths; a v1alpha3 client can never select an old
// lane accidentally.
const (
	APIVersion    = "forms.takoform.com/v1alpha3"
	DiscoveryPath = "/.well-known/takoform/v1alpha3"
	APIRootPath   = "/apis/forms.takoform.com/v1alpha3"

	defaultUserAgent     = "terraform-provider-takoform"
	maxResponseBodyBytes = 8 * 1024 * 1024
)

// requiredFeatures are the discovery capabilities every v1alpha3 host must
// advertise as true.
var requiredFeatures = []string{
	"service_forms",
	"exact_form_ref",
	"optimistic_concurrency",
	"idempotent_lifecycle",
	"operations",
	"artifact_upload",
	"support_profiles",
}

// Discovery is the parsed body of the versioned well-known endpoint.
type Discovery struct {
	APIVersions []string        `json:"api_versions"`
	Features    map[string]bool `json:"features"`
	Endpoints   Endpoints       `json:"endpoints"`
}

// Endpoints carries advertised service URLs from discovery. All non-issuer
// endpoints must be same-origin with the configured endpoint.
type Endpoints struct {
	API        string `json:"api"`
	Artifacts  string `json:"artifacts,omitempty"`
	Operations string `json:"operations,omitempty"`
	Support    string `json:"support,omitempty"`
	OIDCIssuer string `json:"oidc_issuer,omitempty"`
}

// HasFeature reports whether a named server capability is advertised.
func (d Discovery) HasFeature(name string) bool { return d.Features[name] }

// Client is a thin Host API v1alpha3 HTTP client.
type Client struct {
	endpoint        string // normalized origin, no trailing slash
	token           string
	httpClient      *http.Client
	userAgent       string
	apiBase         string
	retryAttempts   int
	overallDeadline time.Duration

	// Discovery is populated by Discover and cached for capability checks.
	Discovery Discovery
}

// Options controls optional client behaviour.
type Options struct {
	// RetryAttempts bounds automatic retries of mutating requests that fail
	// with a complete stable retryable error envelope. Default 3.
	RetryAttempts int
	// OverallDeadline bounds AwaitOperation polling when the caller passes a
	// non-positive deadline. Default 10 minutes.
	OverallDeadline time.Duration
}

// New constructs a Client. If httpClient is nil, a client that refuses to
// follow redirects is used: a redirect would re-send the bearer credential
// to a location the same-origin negotiation never approved, so the 3xx
// response surfaces as a protocol-invalid error instead.
func New(endpoint, token string, httpClient *http.Client) *Client {
	return NewWithOptions(endpoint, token, httpClient, Options{})
}

// NewWithOptions constructs a client. Every request uses the versioned API
// base advertised by discovery; there is no unversioned fallback lane.
func NewWithOptions(endpoint, token string, httpClient *http.Client, options Options) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	retryAttempts := options.RetryAttempts
	if retryAttempts <= 0 {
		retryAttempts = 3
	}
	overallDeadline := options.OverallDeadline
	if overallDeadline <= 0 {
		overallDeadline = defaultOverallDeadline
	}
	return &Client{
		endpoint:        strings.TrimRight(endpoint, "/"),
		token:           token,
		httpClient:      httpClient,
		userAgent:       defaultUserAgent,
		retryAttempts:   retryAttempts,
		overallDeadline: overallDeadline,
	}
}

// Endpoint returns the normalized endpoint origin.
func (c *Client) Endpoint() string { return c.endpoint }

// Discover performs GET {endpoint}{DiscoveryPath}, validates the v1alpha3
// discovery contract, and caches the negotiated API base.
func (c *Client) Discover(ctx context.Context) (Discovery, error) {
	if _, err := c.configuredOrigin(); err != nil {
		return Discovery{}, err
	}
	_, _, data, err := c.do(ctx, http.MethodGet, c.endpoint+DiscoveryPath, nil, nil, false, http.StatusOK)
	if err != nil {
		return Discovery{}, err
	}
	var disco Discovery
	if err := decodeStrictJSON(data, &disco); err != nil {
		return Discovery{}, fmt.Errorf("takoform: decoding discovery document: %w", err)
	}
	c.Discovery = disco
	if err := c.negotiateEndpoints(disco); err != nil {
		return Discovery{}, err
	}
	return disco, nil
}

func (c *Client) negotiateEndpoints(disco Discovery) error {
	c.apiBase = ""
	if len(disco.APIVersions) != 1 || disco.APIVersions[0] != APIVersion {
		return fmt.Errorf("takoform: versioned discovery must advertise only API version %s", APIVersion)
	}
	for _, feature := range requiredFeatures {
		if !disco.HasFeature(feature) {
			return fmt.Errorf("takoform: versioned discovery does not advertise features.%s", feature)
		}
	}
	if strings.TrimSpace(disco.Endpoints.API) == "" {
		return errors.New("takoform: discovery must advertise an absolute same-origin endpoints.api")
	}
	apiBase, err := c.validAdvertisedEndpoint(disco.Endpoints.API, APIRootPath)
	if err != nil {
		return fmt.Errorf("takoform: invalid discovery API endpoint: %w", err)
	}
	optional := []struct {
		name, raw, path string
	}{
		{"artifacts", disco.Endpoints.Artifacts, APIRootPath + "/artifacts"},
		{"operations", disco.Endpoints.Operations, APIRootPath + "/operations"},
		{"support", disco.Endpoints.Support, APIRootPath + "/support"},
	}
	for _, endpoint := range optional {
		if endpoint.raw == "" {
			continue
		}
		if _, err := c.validAdvertisedEndpoint(endpoint.raw, endpoint.path); err != nil {
			return fmt.Errorf("takoform: invalid discovery %s endpoint: %w", endpoint.name, err)
		}
	}
	if disco.Endpoints.OIDCIssuer != "" {
		if err := validateOIDCIssuer(disco.Endpoints.OIDCIssuer); err != nil {
			return fmt.Errorf("takoform: invalid discovery OIDC issuer: %w", err)
		}
	}
	c.apiBase = strings.TrimRight(apiBase, "/")
	return nil
}

func (c *Client) validAdvertisedEndpoint(raw, expectedPath string) (string, error) {
	advertised, err := url.Parse(raw)
	if err != nil || !advertised.IsAbs() || advertised.Host == "" ||
		(advertised.Scheme != "http" && advertised.Scheme != "https") {
		return "", errors.New("endpoint must be an absolute URL")
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
	// The advertised path is compared in its ESCAPED form and must carry no
	// percent-encoding at all. Every segment of a v1alpha3 path is an ordinary
	// segment, so a host advertising an escaped one — most of all a
	// percent-encoded slash — is describing a shape this lane does not have and
	// that intermediaries would not agree on (spec/decisions/0018). Comparing the
	// decoded path instead would let "%2Fv1alpha3" pass as "/v1alpha3".
	if strings.Contains(advertised.EscapedPath(), "%") {
		return "", errors.New(
			"endpoint path must not percent-encode any character; v1alpha3 paths are ordinary segments",
		)
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

func (c *Client) requireReady() error {
	if c.apiBase == "" {
		return errors.New("takoform: Discover must complete before using the v1alpha3 API")
	}
	return nil
}

// groupPathSegments renders a namespaced Form group as the TWO ordinary path
// segments the lane's URL templates declare — {formGroup}/{formVersion} — so
// the slash inside the apiVersion is a real path separator and never a
// percent-encoded character inside one segment (see package comment and
// spec/decisions/0018). A group carrying no version separator cannot reach
// here: ValidateFormRef rejects it before any URL is built.
func groupPathSegments(group string) string {
	name, version, split := strings.Cut(group, "/")
	if !split {
		return url.PathEscape(group)
	}
	return url.PathEscape(name) + "/" + url.PathEscape(version)
}

// exactFormQuery carries the exact FormRef and Space on read/lifecycle URLs.
// The group travels under the query key "group"; the packageDigest is
// deliberately absent because it is audit evidence, never identity.
func exactFormQuery(space string, ref FormRef) url.Values {
	query := url.Values{}
	query.Set("space", space)
	query.Set("group", ref.APIVersion)
	query.Set("kind", ref.Kind)
	query.Set("definitionVersion", ref.DefinitionVersion)
	query.Set("schemaDigest", ref.SchemaDigest)
	return query
}

func (c *Client) resourceURL(ref FormRef, name, suffix string, query url.Values) string {
	u := fmt.Sprintf(
		"%s/resources/%s/%s/%s",
		c.apiBase,
		groupPathSegments(ref.APIVersion),
		url.PathEscape(ref.Kind),
		url.PathEscape(name),
	)
	if suffix != "" {
		u += "/" + suffix
	}
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

// do sends one request. bodyBytes of nil means no request body. When
// retryStable is true the request carries a one-shot body (net/http treats
// Idempotency-Key as permission to replay after some transport failures;
// only this client's stable structured-error policy may start another
// attempt) and is retried on complete stable retryable error envelopes.
// A 2xx status outside successStatuses is an error; non-2xx responses are
// parsed into *APIError; response bodies are capped at 8MiB.
func (c *Client) do(
	ctx context.Context,
	method, fullURL string,
	headers map[string]string,
	bodyBytes []byte,
	retryStable bool,
	successStatuses ...int,
) (int, http.Header, []byte, error) {
	attempts := 1
	if retryStable {
		attempts = c.retryAttempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		var reader io.Reader
		if bodyBytes != nil {
			if retryStable {
				reader = struct{ io.Reader }{Reader: bytes.NewReader(bodyBytes)}
			} else {
				reader = bytes.NewReader(bodyBytes)
			}
		} else if retryStable {
			reader = struct{ io.Reader }{Reader: bytes.NewReader(nil)}
		}
		req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("takoform: building request: %w", err)
		}
		if retryStable && bodyBytes != nil {
			req.ContentLength = int64(len(bodyBytes))
		}
		if bodyBytes != nil && headers["Content-Type"] == "" {
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
			return 0, nil, nil, fmt.Errorf("takoform: request to %s failed: %w", fullURL, err)
		}
		// A HEAD response transfers no body: its Content-Length describes the
		// representation a GET would return, so the cap does not apply.
		if method != http.MethodHead && resp.ContentLength > maxResponseBodyBytes {
			_ = resp.Body.Close()
			return 0, nil, nil, fmt.Errorf("takoform: response from %s exceeds %d bytes", fullURL, maxResponseBodyBytes)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
		_ = resp.Body.Close()
		if readErr != nil {
			return 0, nil, nil, fmt.Errorf("takoform: reading response body: %w", readErr)
		}
		if len(data) > maxResponseBodyBytes {
			return 0, nil, nil, fmt.Errorf("takoform: response from %s exceeds %d bytes", fullURL, maxResponseBodyBytes)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			apiErr := parseAPIError(resp.StatusCode, data, resp.Header.Get("Retry-After"))
			if retryStable && attempt+1 < attempts && isPortableRetryable(apiErr) {
				if err := waitForRetry(ctx, attempt, apiErr.RetryAfter); err != nil {
					return 0, nil, nil, err
				}
				continue
			}
			return 0, nil, nil, apiErr
		}
		if !containsStatus(successStatuses, resp.StatusCode) {
			return 0, nil, nil, fmt.Errorf(
				"takoform: response from %s returned unexpected success status %d",
				fullURL,
				resp.StatusCode,
			)
		}
		if resp.StatusCode == http.StatusNoContent && len(data) != 0 {
			return 0, nil, nil, fmt.Errorf(
				"takoform: response from %s returned a non-empty response body for HTTP 204",
				fullURL,
			)
		}
		return resp.StatusCode, resp.Header.Clone(), data, nil
	}
	return 0, nil, nil, errors.New("takoform: retry attempts exhausted")
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

func decodeBody(data []byte, fullURL string, out any) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return fmt.Errorf("takoform: response from %s returned an empty JSON response body", fullURL)
	}
	if err := decodeStrictJSON(data, out); err != nil {
		return fmt.Errorf("takoform: decoding response from %s: %w", fullURL, err)
	}
	return nil
}

// retryBaseDelay is the exponential backoff base for retrying stable
// retryable error envelopes; retryMaxDelay bounds a host-supplied Retry-After
// only for a caller whose context carries no deadline of its own. Operation
// polling uses its own pollBaseDelay/pollMaxDelay pair (see operations.go).
const (
	retryBaseDelay = 25 * time.Millisecond
	retryMaxDelay  = 60 * time.Second
)

// waitForRetry sleeps before the next attempt. A host Retry-After hint wins
// over the exponential backoff and is honored in full, bounded only by the
// caller's deadline (retryAfterDelay); otherwise the backoff window
// base*2^attempt is applied with full jitter so retrying clients do not
// synchronize.
func waitForRetry(ctx context.Context, attempt int, retryAfter time.Duration) error {
	var delay time.Duration
	if retryAfter > 0 {
		delay = retryAfterDelay(ctx, retryAfter, retryMaxDelay)
	} else {
		window := retryBaseDelay * time.Duration(1<<attempt)
		if window <= 0 {
			window = retryBaseDelay
		}
		delay = time.Duration(rand.Int63n(int64(window)))
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// mutationKey derives the deterministic Idempotency-Key for one lifecycle
// mutation from the values it is given. The host namespaces the key by tenant,
// principal, and space, so equal keys from different principals never collide
// (spec/host-api/operations-v1alpha3.json: idempotency.namespace).
func mutationKey(values ...any) string {
	raw, _ := json.Marshal(values)
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("takoform-%x", digest[:])
}

// incarnationKey is the Idempotency-Key of one resource-lifecycle operation:
// (operation, exact FormRef, name, space, UID, generation, body digest).
//
// The uid is in the key because neither counter distinguishes incarnations. A
// name is reusable and a re-created resource starts at generation 1 exactly as
// it starts at revision 1, so a key built from a counter alone is the same key
// for two different resources that merely shared a name. The host replays a
// recorded response BEFORE it resolves the resource the request addresses —
// that is what an idempotency record is for — so the second operation would be
// answered with the first one's terminal result and never happen at all. For a
// delete that means Terraform drops the resource from state while the host
// leaves it running.
//
// Putting the uid in the key is decision 0015 rule 10 — an accepted mutation is
// bound to the incarnation it was accepted for — applied to the key instead of
// to the commit. It stays a client-side derivation: the uid is not sent on the
// wire, no host behaviour changes, and the uid fence remains the MAY that
// decision 0011 left it. It also keeps the key deterministic, which is the
// whole point of having one: the SAME operation on the SAME incarnation, retried
// after a lost response, still derives the same key and still replays its
// terminal answer instead of executing twice.
//
// Two operations legitimately pass an empty uid, because at the moment they
// build the request no incarnation exists to name: a create, and a new-adoption
// import. Both are fenced against the free NAME rather than against an
// incarnation (decision 0015 rule 10), and for both the deterministic key is
// the only answer to a lost response — the required check
// apply-idempotency-replay measures it, and without it If-None-Match: * turns
// the retry into already_exists and hands the operator an import. What that
// leaves open is a create replayed across an incarnation boundary, and no
// client can close it: the discriminator would have to be the deletion the host
// performed in between, which the client does not know about and which is not
// part of its request. It closes at the host, by retiring a record whose
// incarnation is gone, and that is a normative host rule this lane has not
// stated.
func incarnationKey(operation string, ref FormRef, name, space, uid, generation, body string) string {
	return mutationKey(operation, ref.APIVersion, ref.Kind, name, space, uid, generation, body)
}

func bodyDigest(raw []byte) string {
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("%x", digest[:])
}
