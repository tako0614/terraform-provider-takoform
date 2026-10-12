package hostv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
)

var (
	idPattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{15,127}$`)
)

// Client is a thin Host API v2 client. It never retries a mutation or creates
// an idempotency key on the caller's behalf. Discovery/GetSupport expose
// current capabilities for planning new intent, but mutations do not gate an
// already formed caller request on those mutable advertisements; the Host
// decides whether an exact replay is accepted or rejected.
type Client struct {
	endpoint   string
	token      string
	httpClient *http.Client

	discoverMu sync.Mutex
	mu         sync.RWMutex
	discovery  Discovery
	baseURL    string
	discovered bool
}

// New constructs a v2-only client. A supplied HTTP client is cloned and its
// redirect policy replaced so credentials cannot follow an untrusted redirect.
func New(endpoint, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{endpoint: endpoint, token: token, httpClient: &clientCopy}
}

// Discover validates the unauthenticated v2 discovery document and stores its
// exact same-origin API base URL for subsequent requests.
func (c *Client) Discover(ctx context.Context) (Discovery, error) {
	c.discoverMu.Lock()
	defer c.discoverMu.Unlock()
	c.mu.Lock()
	c.discovery = Discovery{}
	c.baseURL = ""
	c.discovered = false
	c.mu.Unlock()

	origin, err := c.endpointOrigin()
	if err != nil {
		return Discovery{}, err
	}
	response, err := c.do(ctx, http.MethodGet, origin+DiscoveryPath, nil, false, maxResponseBytes)
	if err != nil {
		return Discovery{}, err
	}
	if response.status != http.StatusOK {
		return Discovery{}, response.apiError()
	}
	var discovery Discovery
	if err := decodeRequired(response.body, &discovery,
		"api", "baseUrl", "documentation", "authentication", "capabilities", "limits"); err != nil {
		return Discovery{}, errors.New("takoform v2: malformed discovery document")
	}
	var discoveryFields map[string]json.RawMessage
	if err := json.Unmarshal(response.body, &discoveryFields); err != nil ||
		requireNestedObjectKeys(discoveryFields["authentication"], "schemes", "documentation") != nil ||
		requireNestedObjectKeys(discoveryFields["capabilities"], "offerings", "previews", "privateInputs") != nil ||
		requireNestedObjectKeys(discoveryFields["limits"], "maxRequestBytes", "maxPageSize", "replayWindowSeconds") != nil {
		return Discovery{}, errors.New("takoform v2: malformed nested discovery fields")
	}
	if err := validateDiscovery(discovery, origin); err != nil {
		return Discovery{}, err
	}
	c.mu.Lock()
	c.discovery = discovery
	c.baseURL = discovery.BaseURL
	c.discovered = true
	c.mu.Unlock()
	return discovery, nil
}

// GetSupport checks support for one exact HTTPS Form URL. The client never
// fetches the Form URL itself.
func (c *Client) GetSupport(ctx context.Context, form string) (Support, error) {
	if err := validateFormURL(form); err != nil {
		return Support{}, err
	}
	base, _, err := c.apiBase()
	if err != nil {
		return Support{}, err
	}
	u, _ := url.Parse(base + "/support")
	query := u.Query()
	query.Set("form", form)
	u.RawQuery = query.Encode()
	response, err := c.do(ctx, http.MethodGet, u.String(), nil, true, maxResponseBytes)
	if err != nil {
		return Support{}, err
	}
	if response.status != http.StatusOK {
		return Support{}, response.apiError()
	}
	var support Support
	if err := decodeRequired(response.body, &support, "form", "supported", "operations", "privateInputs"); err != nil || support.Form != form {
		return Support{}, errors.New("takoform v2: malformed Form support response")
	}
	if err := validateSupport(support); err != nil {
		return Support{}, err
	}
	return support, nil
}

// ListOfferings returns one authorized Host page. It does not select an
// Offering; create requires the caller to pass an explicit id and revision.
func (c *Client) ListOfferings(ctx context.Context, form, space string, options PageOptions) (Page[Offering], error) {
	var page Page[Offering]
	if err := validateFormURL(form); err != nil {
		return page, err
	}
	if !idPattern.MatchString(space) {
		return page, errors.New("takoform v2: invalid Space id")
	}
	base, discovery, err := c.apiBase()
	if err != nil {
		return page, err
	}
	if !discovery.Capabilities.Offerings {
		return page, errors.New("takoform v2: Offerings are not advertised")
	}
	query, err := pageQuery(options, discovery.Limits.MaxPageSize)
	if err != nil {
		return page, err
	}
	query.Set("form", form)
	query.Set("space", space)
	u, _ := url.Parse(base + "/offerings")
	u.RawQuery = query.Encode()
	response, err := c.do(ctx, http.MethodGet, u.String(), nil, true, maxResponseBytes)
	if err != nil {
		return page, err
	}
	if response.status != http.StatusOK {
		return page, response.apiError()
	}
	if err := decodeRequiredNullable(response.body, &page, map[string]bool{"nextCursor": true}, "items", "nextCursor"); err != nil || page.Items == nil {
		return Page[Offering]{}, errors.New("takoform v2: malformed Offering page")
	}
	var pageEnvelope struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(response.body, &pageEnvelope); err != nil || len(pageEnvelope.Items) != len(page.Items) {
		return Page[Offering]{}, errors.New("takoform v2: malformed Offering page")
	}
	for index, offering := range page.Items {
		if !idPattern.MatchString(offering.ID) || offering.Revision == "" || offering.Form != form {
			return Page[Offering]{}, errors.New("takoform v2: malformed Offering")
		}
		if err := decodeRequired(pageEnvelope.Items[index], &offering, "id", "revision", "form", "label", "description"); err != nil || validateOptionalObject(pageEnvelope.Items[index], "termsUrl") != nil {
			return Page[Offering]{}, errors.New("takoform v2: malformed Offering fields")
		}
		if offering.TermsURL != "" && !validHTTPSURL(offering.TermsURL) {
			return Page[Offering]{}, errors.New("takoform v2: malformed Offering terms URL")
		}
	}
	return page, nil
}

// CreateResource sends exactly one create request with the caller's key and
// body. Capability planning remains outside this exact-intent transport method.
func (c *Client) CreateResource(ctx context.Context, key string, input CreateRequest) (Operation, error) {
	if err := validateIdempotencyKey(key); err != nil {
		return Operation{}, err
	}
	if err := validateCreate(input); err != nil {
		return Operation{}, err
	}
	base, _, err := c.apiBase()
	if err != nil {
		return Operation{}, err
	}
	if input.PrivateInputs != nil && *input.PrivateInputs == nil {
		return Operation{}, errors.New("takoform v2: privateInputs must be an object")
	}
	if err := jsonv2.ValidateValue(input); err != nil {
		return Operation{}, err
	}
	body, err := marshalRequest(input)
	if err != nil {
		return Operation{}, err
	}
	return c.mutate(ctx, http.MethodPost, base+"/resources", "create", "", key, "", 1, body)
}

// ReadResource reads one Host-owned Resource by UID.
func (c *Client) ReadResource(ctx context.Context, uid string) (Resource, error) {
	var resource Resource
	if !idPattern.MatchString(uid) {
		return resource, errors.New("takoform v2: invalid Resource UID")
	}
	base, _, err := c.apiBase()
	if err != nil {
		return resource, err
	}
	response, err := c.do(ctx, http.MethodGet, base+"/resources/"+uid, nil, true, maxResponseBytes)
	if err != nil {
		return resource, err
	}
	if response.status != http.StatusOK {
		return resource, response.apiError()
	}
	if err := decodeRequiredNullable(response.body, &resource, map[string]bool{"observedAt": true}, "uid", "form", "space", "name", "generation", "observedGeneration", "observedAt", "phase", "spec", "observed", "output", "lastOperation"); err != nil || resource.UID != uid || validateOptionalObject(response.body, "offering") != nil {
		return Resource{}, errors.New("takoform v2: malformed Resource response")
	}
	if err := validateResource(resource); err != nil {
		return Resource{}, err
	}
	return resource, nil
}

// ListResources returns one page using exact Space, name, and Form filters.
func (c *Client) ListResources(ctx context.Context, filters ResourceFilters, options PageOptions) (Page[Resource], error) {
	var page Page[Resource]
	if filters.Space != "" && !idPattern.MatchString(filters.Space) || filters.Name != "" && !idPattern.MatchString(filters.Name) {
		return page, errors.New("takoform v2: invalid Resource list filter")
	}
	if filters.Form != "" {
		if err := validateFormURL(filters.Form); err != nil {
			return page, err
		}
	}
	base, discovery, err := c.apiBase()
	if err != nil {
		return page, err
	}
	query, err := pageQuery(options, discovery.Limits.MaxPageSize)
	if err != nil {
		return page, err
	}
	if filters.Space != "" {
		query.Set("space", filters.Space)
	}
	if filters.Name != "" {
		query.Set("name", filters.Name)
	}
	if filters.Form != "" {
		query.Set("form", filters.Form)
	}
	u, _ := url.Parse(base + "/resources")
	u.RawQuery = query.Encode()
	response, err := c.do(ctx, http.MethodGet, u.String(), nil, true, maxResponseBytes)
	if err != nil {
		return page, err
	}
	if response.status != http.StatusOK {
		return page, response.apiError()
	}
	if err := decodeResourcePage(response.body, &page); err != nil {
		return Page[Resource]{}, errors.New("takoform v2: malformed Resource page")
	}
	return page, nil
}

// UpdateResource replaces the complete spec and uses the exact caller-provided
// key and expected-generation fence.
func (c *Client) UpdateResource(ctx context.Context, uid, key string, expectedGeneration int64, input UpdateRequest) (Operation, error) {
	if !idPattern.MatchString(uid) || expectedGeneration <= 0 {
		return Operation{}, errors.New("takoform v2: invalid Resource UID or generation")
	}
	if err := validateIdempotencyKey(key); err != nil {
		return Operation{}, err
	}
	if input.Spec == nil {
		return Operation{}, errors.New("takoform v2: update spec is required")
	}
	base, _, err := c.apiBase()
	if err != nil {
		return Operation{}, err
	}
	if input.PrivateInputs != nil && *input.PrivateInputs == nil {
		return Operation{}, errors.New("takoform v2: privateInputs must be an object")
	}
	if err := jsonv2.ValidateValue(input); err != nil {
		return Operation{}, err
	}
	if expectedGeneration >= maxSafeInteger {
		return Operation{}, errors.New("takoform v2: generation cannot advance beyond the common integer limit")
	}
	body, err := marshalRequest(input)
	if err != nil {
		return Operation{}, err
	}
	return c.mutate(ctx, http.MethodPut, base+"/resources/"+uid, "update", uid, key, strconv.FormatInt(expectedGeneration, 10), expectedGeneration+1, body)
}

// DeleteResource sends one bodyless delete with the caller's generation fence.
func (c *Client) DeleteResource(ctx context.Context, uid, key string, expectedGeneration int64) (Operation, error) {
	if !idPattern.MatchString(uid) || expectedGeneration <= 0 {
		return Operation{}, errors.New("takoform v2: invalid Resource UID or generation")
	}
	if err := validateIdempotencyKey(key); err != nil {
		return Operation{}, err
	}
	base, _, err := c.apiBase()
	if err != nil {
		return Operation{}, err
	}
	if expectedGeneration >= maxSafeInteger {
		return Operation{}, errors.New("takoform v2: generation cannot advance beyond the common integer limit")
	}
	return c.mutate(ctx, http.MethodDelete, base+"/resources/"+uid, "delete", uid, key, strconv.FormatInt(expectedGeneration, 10), expectedGeneration+1, nil)
}

// ReadOperation reads an accepted Operation. It never starts or resumes work.
func (c *Client) ReadOperation(ctx context.Context, id string) (Operation, error) {
	var operation Operation
	if !idPattern.MatchString(id) {
		return operation, errors.New("takoform v2: invalid Operation id")
	}
	base, _, err := c.apiBase()
	if err != nil {
		return operation, err
	}
	response, err := c.do(ctx, http.MethodGet, base+"/operations/"+id, nil, true, maxResponseBytes)
	if err != nil {
		return operation, err
	}
	if response.status != http.StatusOK {
		return operation, response.apiError()
	}
	if err := decodeOperation(response.body, &operation); err != nil || operation.ID != id {
		return Operation{}, errors.New("takoform v2: malformed Operation response")
	}
	return operation, nil
}

// ContinuePrivateInputs sends the entire original private-input map to the
// same accepted Operation. It deliberately does not discover, check Form
// support, add a key or generation, or retry the request.
func (c *Client) ContinuePrivateInputs(ctx context.Context, original Operation, privateInputs map[string]string) (Operation, error) {
	if !idPattern.MatchString(original.ID) || !idPattern.MatchString(original.ResourceUID) || original.Generation < 1 || original.Generation > maxSafeInteger {
		return Operation{}, errors.New("takoform v2: invalid Operation identity for private-input continuation")
	}
	switch original.Action {
	case "create", "update", "delete":
	default:
		return Operation{}, errors.New("takoform v2: invalid Operation action for private-input continuation")
	}
	switch original.Status {
	case "queued", "running", "waiting_input", "reconciling":
	default:
		return Operation{}, errors.New("takoform v2: private inputs cannot continue a terminal or unknown Operation")
	}
	if privateInputs == nil {
		return Operation{}, errors.New("takoform v2: original privateInputs map is required")
	}
	request := struct {
		PrivateInputs map[string]string `json:"privateInputs"`
	}{PrivateInputs: privateInputs}
	if err := jsonv2.ValidateValue(request); err != nil {
		return Operation{}, errors.New("takoform v2: private-input request is not valid JSON")
	}
	body, err := marshalRequest(request)
	if err != nil {
		return Operation{}, err
	}
	base, _, err := c.apiBase()
	if err != nil {
		return Operation{}, err
	}
	response, err := c.do(ctx, http.MethodPut, base+"/operations/"+original.ID+"/private-inputs", body, true, maxResponseBytes)
	if err != nil {
		return Operation{}, &IndeterminateError{Action: "private-input continuation"}
	}
	if response.status != http.StatusOK {
		if response.status >= 400 && response.status < 500 {
			apiError, validProblem := response.validProblemError()
			if validProblem {
				if apiError.OperationID != "" && apiError.OperationID != original.ID {
					return Operation{}, &IndeterminateError{Action: "private-input continuation"}
				}
				return Operation{}, apiError
			}
		}
		return Operation{}, &IndeterminateError{Action: "private-input continuation"}
	}
	var operation Operation
	if err := decodeOperation(response.body, &operation); err != nil || operation.ID != original.ID || operation.ResourceUID != original.ResourceUID || operation.Action != original.Action || operation.Generation != original.Generation {
		return Operation{}, &IndeterminateError{Action: "private-input continuation"}
	}
	switch operation.Status {
	case "queued", "running", "waiting_input", "reconciling":
		return operation, nil
	default:
		return Operation{}, &IndeterminateError{Action: "private-input continuation"}
	}
}

func (c *Client) mutate(ctx context.Context, method, target, action, expectedUID, key, generation string, acceptedGeneration int64, body []byte) (Operation, error) {
	var empty Operation
	headers := make(http.Header)
	headers.Set("Idempotency-Key", key)
	if generation != "" {
		headers.Set("Takoform-Expected-Generation", generation)
	}
	response, err := c.doWithHeaders(ctx, method, target, body, true, maxResponseBytes, headers)
	if err != nil {
		return empty, &IndeterminateError{Action: action}
	}
	if response.status != http.StatusOK && response.status != http.StatusAccepted {
		if response.status < 400 || response.status >= 500 {
			return empty, &IndeterminateError{Action: action}
		}
		apiError, validProblem := response.validProblemError()
		if !validProblem {
			return empty, &IndeterminateError{Action: action}
		}
		return empty, apiError
	}
	var operation Operation
	if err := decodeOperation(response.body, &operation); err != nil {
		return empty, &IndeterminateError{Action: action}
	}
	if operation.Action != action || operation.ResourceUID == "" || expectedUID != "" && operation.ResourceUID != expectedUID || operation.Generation != acceptedGeneration {
		return empty, &IndeterminateError{Action: action}
	}
	if response.status == http.StatusAccepted {
		retryAfter, err := strconv.ParseInt(response.header.Get("Retry-After"), 10, 64)
		if err != nil || retryAfter <= 0 {
			return empty, &IndeterminateError{Action: action}
		}
	}
	if err := c.validateOperationLocation(response.header.Get("Location"), operation.ID); err != nil {
		return empty, &IndeterminateError{Action: action}
	}
	return operation, nil
}

func (c *Client) do(ctx context.Context, method, target string, body []byte, authenticated bool, limit int64) (httpResponse, error) {
	return c.doWithHeaders(ctx, method, target, body, authenticated, limit, nil)
}

func (c *Client) doWithHeaders(ctx context.Context, method, target string, body []byte, authenticated bool, limit int64, headers http.Header) (httpResponse, error) {
	var requestBody io.Reader = bytes.NewReader(body)
	if body == nil {
		requestBody = oneShotEmptyReader{}
	}
	request, err := http.NewRequestWithContext(ctx, method, target, requestBody)
	if err != nil {
		return httpResponse{}, errors.New("takoform v2: invalid request target")
	}
	// net/http treats Idempotency-Key as an idempotency signal and may replay
	// a request after a connection failure when GetBody is populated. Mutations
	// must never be retried implicitly, including by the transport.
	request.GetBody = nil
	request.Header.Set("Accept", "application/json, application/problem+json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated && c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return httpResponse{}, errors.New("takoform v2: HTTP request failed")
	}
	defer response.Body.Close()
	if limit <= 0 || limit > maxResponseBytes {
		limit = maxResponseBytes
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return httpResponse{}, errors.New("takoform v2: response body unavailable or too large")
	}
	return httpResponse{status: response.StatusCode, header: response.Header.Clone(), body: data}, nil
}

type oneShotEmptyReader struct{}

func (oneShotEmptyReader) Read([]byte) (int, error) { return 0, io.EOF }

func (c *Client) apiBase() (string, Discovery, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.discovered {
		return "", Discovery{}, errors.New("takoform v2: Discover must succeed before API requests")
	}
	bearerAdvertised := false
	for _, scheme := range c.discovery.Authentication.Schemes {
		if strings.EqualFold(scheme, "Bearer") {
			bearerAdvertised = true
			break
		}
	}
	if !bearerAdvertised || c.token == "" {
		return "", Discovery{}, errors.New("takoform v2: API access requires an advertised Bearer scheme and configured credential")
	}
	return c.baseURL, c.discovery, nil
}

func (c *Client) endpointOrigin() (string, error) {
	u, err := url.Parse(c.endpoint)
	if err != nil || !u.IsAbs() || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("takoform v2: endpoint must be an HTTPS origin")
	}
	return "https://" + u.Host, nil
}

func (c *Client) validateOperationLocation(raw, operationID string) error {
	base, _, err := c.apiBase()
	if err != nil {
		return err
	}
	location, err := url.Parse(raw)
	if err != nil || !location.IsAbs() || !sameOriginURL(location, mustParseURL(base)) || location.User != nil || location.RawQuery != "" || location.Fragment != "" || location.Path != strings.TrimRight(mustParseURL(base).Path, "/")+"/operations/"+operationID {
		return errors.New("takoform v2: invalid Operation Location")
	}
	return nil
}

type httpResponse struct {
	status int
	header http.Header
	body   []byte
}

func (r httpResponse) apiError() error {
	if parsed, ok := r.validProblemError(); ok {
		return parsed
	}
	return &APIError{Status: r.status, RetryAfter: r.header.Get("Retry-After")}
}

func (r httpResponse) validProblemError() (*APIError, bool) {
	contentType, _, err := mime.ParseMediaType(r.header.Get("Content-Type"))
	if err != nil || contentType != "application/problem+json" {
		return nil, false
	}
	var problem struct {
		Type        string  `json:"type"`
		Title       string  `json:"title"`
		Status      int     `json:"status"`
		Code        string  `json:"code"`
		OperationID *string `json:"operationId"`
	}
	if decodeRequired(r.body, &problem, "type", "title", "status", "code") != nil || problem.Type != "about:blank" || problem.Title == "" || problem.Status != r.status {
		return nil, false
	}
	if validateOptionalObject(r.body, "operationId") != nil {
		return nil, false
	}
	if problem.OperationID != nil && !idPattern.MatchString(*problem.OperationID) {
		return nil, false
	}
	if !validProblemCode(r.status, problem.Code) {
		return nil, false
	}
	apiError := &APIError{Status: r.status, Code: problem.Code, RetryAfter: r.header.Get("Retry-After")}
	if problem.OperationID != nil {
		apiError.OperationID = *problem.OperationID
	}
	return apiError, true
}

func validProblemCode(status int, code string) bool {
	switch status {
	case http.StatusBadRequest:
		return code == "invalid_request"
	case http.StatusUnauthorized:
		return code == "unauthenticated"
	case http.StatusForbidden:
		return code == "forbidden"
	case http.StatusNotFound:
		return code == "not_found" || code == "capability_unavailable"
	case http.StatusMethodNotAllowed:
		return code == "method_not_allowed"
	case http.StatusConflict:
		switch code {
		case "name_conflict", "generation_conflict", "resource_busy", "idempotency_conflict", "offering_changed", "dependency_conflict", "private_inputs_conflict", "private_inputs_unverifiable", "operation_terminal", "cursor_expired":
			return true
		}
	case http.StatusGone:
		return code == "gone"
	case http.StatusRequestEntityTooLarge:
		return code == "request_too_large"
	case http.StatusUnsupportedMediaType:
		return code == "unsupported_media_type"
	case http.StatusUnprocessableEntity:
		return code == "unsupported_form" || code == "invalid_spec" || code == "capability_required"
	case http.StatusPreconditionRequired:
		return code == "expected_generation_required"
	case http.StatusTooManyRequests:
		return code == "rate_limited"
	case http.StatusServiceUnavailable:
		return code == "temporarily_unavailable"
	case http.StatusInternalServerError:
		return code == "internal_error"
	}
	return false
}

func validateDiscovery(d Discovery, origin string) error {
	if d.API != APIVersion || !validHTTPSURL(d.Documentation) || !validHTTPSURL(d.Authentication.Documentation) || len(d.Authentication.Schemes) == 0 {
		return errors.New("takoform v2: invalid v2 discovery metadata")
	}
	for _, scheme := range d.Authentication.Schemes {
		if strings.TrimSpace(scheme) == "" {
			return errors.New("takoform v2: invalid authentication scheme")
		}
	}
	if d.Limits.MaxRequestBytes <= 0 || d.Limits.MaxRequestBytes > 9007199254740991 || d.Limits.MaxPageSize <= 0 || d.Limits.MaxPageSize > 9007199254740991 || d.Limits.ReplayWindowSeconds <= 0 || d.Limits.ReplayWindowSeconds > 9007199254740991 {
		return errors.New("takoform v2: invalid discovery limits")
	}
	if !utf8.ValidString(d.BaseURL) || strings.TrimRight(d.BaseURL, "/") != d.BaseURL {
		return errors.New("takoform v2: invalid discovery baseUrl")
	}
	base, err := url.Parse(d.BaseURL)
	if err != nil || !base.IsAbs() || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || strings.ContainsAny(d.BaseURL, "?#") || !isASCII(d.BaseURL) || !sameOriginURL(base, mustParseURL(origin)) {
		return errors.New("takoform v2: discovery baseUrl must be an absolute same-origin HTTPS URL")
	}
	return nil
}

func validateSupport(s Support) error {
	if s.Operations == nil {
		return errors.New("takoform v2: malformed Form support operations")
	}
	seen := map[string]bool{}
	for _, operation := range s.Operations {
		switch operation {
		case "create", "read", "update", "delete":
		default:
			return errors.New("takoform v2: unknown operation in Form support")
		}
		if seen[operation] {
			return errors.New("takoform v2: repeated operation in Form support")
		}
		seen[operation] = true
	}
	if !s.Supported && (len(s.Operations) != 0 || s.PrivateInputs) {
		return errors.New("takoform v2: unsupported Form response must have empty capabilities")
	}
	return nil
}

func validateCreate(input CreateRequest) error {
	if err := validateFormURL(input.Form); err != nil {
		return err
	}
	if !idPattern.MatchString(input.Space) || !idPattern.MatchString(input.Name) || input.Spec == nil {
		return errors.New("takoform v2: invalid create Space, name, or spec")
	}
	if input.Offering != nil && (!idPattern.MatchString(input.Offering.ID) || input.Offering.Revision == "") {
		return errors.New("takoform v2: invalid Offering selection")
	}
	return nil
}

func validateResource(r Resource) error {
	if !idPattern.MatchString(r.UID) || !idPattern.MatchString(r.Space) || !idPattern.MatchString(r.Name) || validateFormURL(r.Form) != nil || r.Generation < 1 || r.Generation > maxSafeInteger || r.ObservedGeneration < 0 || r.ObservedGeneration > maxSafeInteger || r.ObservedGeneration > r.Generation || r.Spec == nil || r.Observed == nil || r.Output == nil || !idPattern.MatchString(r.LastOperation) {
		return errors.New("takoform v2: invalid Resource fields")
	}
	switch r.Phase {
	case "pending", "idle", "deleting", "error":
	default:
		return errors.New("takoform v2: unknown Resource phase")
	}
	if r.ObservedAt != nil && !validTimestamp(*r.ObservedAt) {
		return errors.New("takoform v2: invalid Resource observation timestamp")
	}
	if r.ObservedGeneration > 0 && r.ObservedAt == nil {
		return errors.New("takoform v2: verified observedGeneration requires observedAt")
	}
	if r.Offering != nil && (!idPattern.MatchString(r.Offering.ID) || r.Offering.Revision == "") {
		return errors.New("takoform v2: invalid Resource Offering")
	}
	return nil
}

func decodeOperation(data []byte, operation *Operation) error {
	if err := decodeRequired(data, operation, "id", "resourceUid", "action", "generation", "status", "effect", "createdAt", "updatedAt", "retainUntil"); err != nil {
		return err
	}
	if !idPattern.MatchString(operation.ID) || !idPattern.MatchString(operation.ResourceUID) || operation.Generation < 1 || operation.Generation > maxSafeInteger || !validTimestamp(operation.CreatedAt) || !validTimestamp(operation.UpdatedAt) || !validTimestamp(operation.RetainUntil) {
		return errors.New("invalid Operation identity or timestamps")
	}
	switch operation.Action {
	case "create", "update", "delete":
	default:
		return errors.New("unknown Operation action")
	}
	switch operation.Status {
	case "queued", "running", "waiting_input", "reconciling", "succeeded", "failed":
	default:
		return errors.New("unknown Operation status")
	}
	switch operation.Effect {
	case "none", "unknown", "partial", "complete":
	default:
		return errors.New("unknown Operation effect")
	}
	if operation.Status == "succeeded" && operation.Effect != "complete" || operation.Status == "failed" && operation.Effect != "none" && operation.Effect != "partial" || operation.Status == "reconciling" && operation.Effect != "unknown" {
		return errors.New("inconsistent Operation status and effect")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if operation.Status == "failed" && (operation.Error == nil || operation.Error.Code == "" || operation.Error.Message == "") || operation.Status != "failed" && operation.Error != nil && operation.Status != "reconciling" {
		return errors.New("inconsistent Operation error field")
	}
	if operation.Status == "waiting_input" {
		if operation.InputRequired == nil || operation.InputRequired.Names == nil || len(operation.InputRequired.Names) == 0 || (operation.InputRequired.Reason != "expired" && operation.InputRequired.Reason != "unavailable") {
			return errors.New("invalid waiting_input Operation")
		}
		if err := requireNestedObjectKeys(fields["inputRequired"], "names", "reason"); err != nil {
			return err
		}
		if err := validateInputRequiredNames(fields["inputRequired"]); err != nil {
			return err
		}
	} else if operation.InputRequired != nil {
		return errors.New("inputRequired is valid only while waiting_input")
	}
	if err := validateOptionalObject(data, "error", "inputRequired"); err != nil {
		return err
	}
	if operation.Error != nil {
		if err := requireNestedObjectKeys(fields["error"], "code", "message"); err != nil || operation.Error.Code == "" || operation.Error.Message == "" {
			return errors.New("invalid Operation error")
		}
	}
	return nil
}

func validateInputRequiredNames(raw json.RawMessage) error {
	var inputRequired map[string]json.RawMessage
	if err := json.Unmarshal(raw, &inputRequired); err != nil {
		return err
	}
	var names []json.RawMessage
	if err := json.Unmarshal(inputRequired["names"], &names); err != nil || len(names) == 0 {
		return errors.New("inputRequired.names must be a non-empty array of strings")
	}
	for _, nameJSON := range names {
		var value any
		if err := json.Unmarshal(nameJSON, &value); err != nil {
			return errors.New("inputRequired.names elements must be JSON strings")
		}
		if _, ok := value.(string); !ok {
			return errors.New("inputRequired.names elements must be JSON strings")
		}
	}
	return nil
}

func validateOptionalObject(data []byte, names ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range names {
		if raw, present := fields[name]; present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("optional field must be omitted rather than null")
		}
	}
	return nil
}

func requireNestedObjectKeys(raw json.RawMessage, required ...string) error {
	if err := jsonv2.Validate(raw); err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return errors.New("nested value must be an object")
	}
	for _, key := range required {
		value, ok := object[key]
		if !ok {
			return errors.New("nested object is missing a required field")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("nested required field must not be null")
		}
	}
	return nil
}

func decodeRequired(data []byte, destination any, required ...string) error {
	return decodeRequiredNullable(data, destination, nil, required...)
}

func decodeRequiredNullable(data []byte, destination any, nullable map[string]bool, required ...string) error {
	if err := jsonv2.Validate(data); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("response must be a JSON object")
	}
	for _, field := range required {
		raw, ok := fields[field]
		if !ok {
			return fmt.Errorf("missing required field %s", field)
		}
		if !nullable[field] && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("required field %s must not be null", field)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(destination)
}

func decodeResourcePage(data []byte, page *Page[Resource]) error {
	if err := decodeRequiredNullable(data, page, map[string]bool{"nextCursor": true}, "items", "nextCursor"); err != nil || page.Items == nil {
		return errors.New("invalid page envelope")
	}
	var envelope struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Items) != len(page.Items) {
		return errors.New("invalid page items")
	}
	required := []string{"uid", "form", "space", "name", "generation", "observedGeneration", "observedAt", "phase", "spec", "observed", "output", "lastOperation"}
	for index, raw := range envelope.Items {
		var resource Resource
		if err := decodeRequiredNullable(raw, &resource, map[string]bool{"observedAt": true}, required...); err != nil || validateResource(resource) != nil || validateOptionalObject(raw, "offering") != nil {
			return errors.New("invalid Resource page item")
		}
		page.Items[index] = resource
	}
	return nil
}

func validateFormURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, "?#") || !isASCII(raw) {
		return errors.New("takoform v2: Form identity must be an exact absolute HTTPS URL")
	}
	return nil
}

func validHTTPSURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.IsAbs() && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && isASCII(raw)
}

func sameOriginURL(left, right *url.URL) bool {
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Hostname(), right.Hostname()) && port(left) == port(right)
}

func pageQuery(options PageOptions, maxPageSize int64) (url.Values, error) {
	query := make(url.Values)
	if options.Limit < 0 || options.Limit > maxPageSize {
		return nil, errors.New("takoform v2: page limit exceeds Host limit")
	}
	if options.Limit > 0 {
		query.Set("limit", strconv.FormatInt(options.Limit, 10))
	}
	if options.Cursor != "" {
		query.Set("cursor", options.Cursor)
	}
	return query, nil
}

func marshalRequest(input any) ([]byte, error) {
	body, err := jsonv2.Marshal(input)
	if err != nil {
		return nil, errors.New("takoform v2: request is not valid JSON")
	}
	return body, nil
}

func validateIdempotencyKey(key string) error {
	if !idempotencyPattern.MatchString(key) {
		return errors.New("takoform v2: invalid Idempotency-Key")
	}
	return nil
}

func validTimestamp(value string) bool {
	if !strings.HasSuffix(value, "Z") {
		return false
	}
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func isASCII(value string) bool {
	for _, character := range value {
		if character > 0x7f {
			return false
		}
	}
	return true
}

func mustParseURL(value string) *url.URL {
	u, _ := url.Parse(value)
	return u
}
