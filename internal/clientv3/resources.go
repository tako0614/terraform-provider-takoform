package clientv3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// expectedGenerationHeader fences desired-state mutations; a stale value
// fails generation_conflict (412).
const expectedGenerationHeader = "Takoform-Expected-Generation"

// FormAvailability is one exact-FormRef availability entry.
type FormAvailability struct {
	Identity             FormReference `json:"identity"`
	DefinitionKnown      bool          `json:"definitionKnown"`
	Installed            bool          `json:"installed"`
	Executable           bool          `json:"executable"`
	Activated            bool          `json:"activated"`
	AvailableToPrincipal bool          `json:"availableToPrincipal"`
	Operations           []string      `json:"operations"`
	Deprecated           bool          `json:"deprecated"`
}

// FormDefinitionResponse is the principal-readable desired-state contract
// for one exact FormRef. It carries no lifecycle, host implementation, or
// commercial authority.
type FormDefinitionResponse struct {
	Identity      FormReference  `json:"identity"`
	DisplayName   string         `json:"displayName,omitempty"`
	Description   string         `json:"description,omitempty"`
	DesiredSchema map[string]any `json:"desiredSchema"`
}

// EnsureFormAvailable requires that the exact FormRef resolves to exactly
// one availability entry that is installed, executable, activated, and
// available to this principal for the named operation. Identity is compared
// by FormRef alone; the response packageDigest is audit evidence.
func (c *Client) EnsureFormAvailable(ctx context.Context, space string, ref FormRef, operation string) error {
	if err := c.requireReady(); err != nil {
		return err
	}
	if err := ValidateFormRef(ref); err != nil {
		return err
	}
	if err := ValidateSpaceID(space); err != nil {
		return fmt.Errorf("takoform: exact FormRef availability has invalid SpaceID: %w", err)
	}
	fullURL := c.apiBase + "/forms?" + exactFormQuery(space, ref).Encode()
	_, _, data, err := c.do(ctx, http.MethodGet, fullURL, nil, nil, false, http.StatusOK)
	if err != nil {
		return err
	}
	var response struct {
		Forms []FormAvailability `json:"forms"`
	}
	if err := decodeBody(data, fullURL, &response); err != nil {
		return err
	}
	if len(response.Forms) != 1 || !sameFormRef(ref, response.Forms[0].Identity.FormRef) {
		return errors.New("takoform: host did not return the requested exact FormRef")
	}
	available := response.Forms[0]
	if !available.DefinitionKnown || !available.Installed || !available.Executable ||
		!available.Activated || !available.AvailableToPrincipal {
		return fmt.Errorf("takoform: exact FormRef %s is not available to this principal", ref.Kind)
	}
	if !containsString(available.Operations, operation) {
		return fmt.Errorf("takoform: exact FormRef %s does not support %s", ref.Kind, operation)
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// GetFormDefinition reads the exact desired-state schema selected by the
// complete FormRef query. A substituted response identity fails closed.
func (c *Client) GetFormDefinition(ctx context.Context, space string, ref FormRef) (FormDefinitionResponse, error) {
	if err := c.requireReady(); err != nil {
		return FormDefinitionResponse{}, err
	}
	if err := ValidateFormRef(ref); err != nil {
		return FormDefinitionResponse{}, err
	}
	if err := ValidateSpaceID(space); err != nil {
		return FormDefinitionResponse{}, fmt.Errorf("takoform: exact Form Definition has invalid SpaceID: %w", err)
	}
	fullURL := fmt.Sprintf(
		"%s/form-definitions/%s/%s?%s",
		c.apiBase,
		groupPathSegments(ref.APIVersion),
		ref.Kind,
		exactFormQuery(space, ref).Encode(),
	)
	_, _, data, err := c.do(ctx, http.MethodGet, fullURL, nil, nil, false, http.StatusOK)
	if err != nil {
		return FormDefinitionResponse{}, err
	}
	var response FormDefinitionResponse
	if err := decodeBody(data, fullURL, &response); err != nil {
		return FormDefinitionResponse{}, err
	}
	if !sameFormRef(ref, response.Identity.FormRef) {
		return FormDefinitionResponse{}, errors.New("takoform: host substituted the requested exact Form Definition identity")
	}
	if response.DesiredSchema == nil {
		return FormDefinitionResponse{}, errors.New("takoform: host omitted the exact Form Definition desiredSchema")
	}
	return response, nil
}

// resourceRequestBody is the resourceCore wire shape shared by validate,
// prepare, apply, and import requests.
type resourceRequestBody struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Form       *FormReference `json:"form"`
	Metadata   Metadata       `json:"metadata"`
	Spec       map[string]any `json:"spec"`
}

func requestBodyFor(resource *Resource) resourceRequestBody {
	spec := resource.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	return resourceRequestBody{
		APIVersion: resource.APIVersion,
		Kind:       resource.Kind,
		Form:       resource.Form,
		Metadata: Metadata{
			Name:  resource.Metadata.Name,
			Space: resource.Metadata.Space,
		},
		Spec: spec,
	}
}

// ValidateResource reports diagnostics for one desired resource. It never
// mutates and never mints a prepare digest; a client MUST NOT describe it as
// "reviewed in plan".
func (c *Client) ValidateResource(ctx context.Context, resource *Resource) (*ValidateResult, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if err := validateRequestResource(resource); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(requestBodyFor(resource))
	if err != nil {
		return nil, fmt.Errorf("takoform: encoding validate request: %w", err)
	}
	fullURL := c.apiBase + "/resources/validate"
	_, _, data, err := c.do(ctx, http.MethodPost, fullURL, nil, raw, false, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var result ValidateResult
	if err := decodeBody(data, fullURL, &result); err != nil {
		return nil, err
	}
	if result.Diagnostics == nil {
		return nil, errors.New("takoform: host validate response omitted diagnostics")
	}
	return &result, nil
}

// PrepareResource binds the exact spec, identity, and fences to a
// short-lived prepareDigest. The response must echo the requested identity
// and spec byte-for-byte at the RFC 8785 canonical level. An update prepare
// carries the generation fence as Takoform-Expected-Generation
// (spec/host-api/operations-v1beta1.json: "generation-fence-when-updating");
// the host binds the current resource's uid and generation into the digest
// itself, so the client sends no uid channel.
func (c *Client) PrepareResource(ctx context.Context, resource *Resource, fence Fence) (*PrepareResult, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if err := validateRequestResource(resource); err != nil {
		return nil, err
	}
	if fence.ExpectedGeneration != "" && !validCanonicalDecimal(fence.ExpectedGeneration) {
		return nil, errors.New("takoform: prepare fence expectedGeneration must be canonical decimal in 1..9223372036854775807")
	}
	request := requestBodyFor(resource)
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("takoform: encoding prepare request: %w", err)
	}
	var headers map[string]string
	if fence.ExpectedGeneration != "" {
		headers = map[string]string{expectedGenerationHeader: fence.ExpectedGeneration}
	}
	fullURL := c.apiBase + "/resources/prepare"
	_, _, data, err := c.do(ctx, http.MethodPost, fullURL, headers, raw, false, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var result PrepareResult
	if err := decodeBody(data, fullURL, &result); err != nil {
		return nil, err
	}
	if err := validatePrepareResult(resource, request.Spec, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func validatePrepareResult(request *Resource, requestSpec map[string]any, result *PrepareResult) error {
	prepared := &result.Resource
	if prepared.Form == nil || !sameFormRef(request.Form.FormRef, prepared.Form.FormRef) {
		return errors.New("takoform: host prepare changed the exact FormRef identity")
	}
	if prepared.APIVersion != request.APIVersion || prepared.Kind != request.Kind {
		return errors.New("takoform: host prepare changed the resource identity")
	}
	if prepared.Metadata.Name != request.Metadata.Name || prepared.Metadata.Space != request.Metadata.Space {
		return errors.New("takoform: host prepare changed the requested resource name or space")
	}
	requestSpecDigest, err := canonicalValueDigest(requestSpec)
	if err != nil {
		return fmt.Errorf("takoform: digesting requested spec: %w", err)
	}
	preparedSpec := prepared.Spec
	if preparedSpec == nil {
		preparedSpec = map[string]any{}
	}
	preparedSpecDigest, err := canonicalValueDigest(preparedSpec)
	if err != nil {
		return fmt.Errorf("takoform: digesting prepared spec: %w", err)
	}
	if preparedSpecDigest != requestSpecDigest {
		return errors.New("takoform: host prepare changed the requested spec")
	}
	if result.Review.SpecDigest != requestSpecDigest {
		return errors.New("takoform: host prepare returned a specDigest for different desired state")
	}
	if !validWireDigest(result.Review.PrepareDigest) {
		return errors.New("takoform: host prepare returned an invalid prepareDigest")
	}
	return nil
}

type applyRequestBody struct {
	resourceRequestBody
	Review             applyReview `json:"review"`
	ExpectedUID        string      `json:"expectedUid,omitempty"`
	ExpectedGeneration string      `json:"expectedGeneration,omitempty"`
}

type applyReview struct {
	PrepareDigest string `json:"prepareDigest"`
}

// ApplyResource orchestrates the reviewed lifecycle: it requires the exact
// FormRef available for create or update, prepares the exact spec, and PUTs
// the applyRequest carrying the prepareDigest under the fence's
// preconditions. A 202 response is polled to its terminal Operation and the
// result resource is returned.
//
// An UPDATE names its incarnation in the Idempotency-Key twice over: through
// the caller's expectedUid, and through the prepareDigest inside the body,
// which the host mints against the target's CURRENT uid and generation
// (spec/host-api/operations-v1beta1.json: prepareBinding.prepareDigest). Two
// incarnations of one name therefore cannot share an update key even when the
// caller supplies no uid. A CREATE cannot name one, because none exists yet:
// its prepare binds the create markers, which are the same for every
// incarnation. See incarnationKey.
func (c *Client) ApplyResource(ctx context.Context, resource *Resource, fence Fence) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if err := validateRequestResource(resource); err != nil {
		return nil, err
	}
	operation := "create"
	if fence.ExpectedGeneration != "" {
		if !validCanonicalDecimal(fence.ExpectedGeneration) {
			return nil, errors.New("takoform: apply fence expectedGeneration must be canonical decimal in 1..9223372036854775807")
		}
		operation = "update"
	}
	if fence.ExpectedUID != "" && !uidPattern.MatchString(fence.ExpectedUID) {
		return nil, errors.New("takoform: apply fence expectedUid is not a valid UID")
	}
	ref := resource.Form.FormRef
	if err := c.EnsureFormAvailable(ctx, resource.Metadata.Space, ref, operation); err != nil {
		return nil, err
	}
	prepared, err := c.PrepareResource(ctx, resource, fence)
	if err != nil {
		return nil, err
	}
	request := applyRequestBody{
		resourceRequestBody: requestBodyFor(resource),
		Review:              applyReview{PrepareDigest: prepared.Review.PrepareDigest},
		ExpectedUID:         fence.ExpectedUID,
		ExpectedGeneration:  fence.ExpectedGeneration,
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("takoform: encoding apply request: %w", err)
	}
	headers := map[string]string{
		"Idempotency-Key": incarnationKey(
			"apply", ref, resource.Metadata.Name, resource.Metadata.Space,
			fence.ExpectedUID, fence.ExpectedGeneration, bodyDigest(raw),
		),
	}
	if operation == "create" {
		headers["If-None-Match"] = "*"
	} else {
		headers[expectedGenerationHeader] = fence.ExpectedGeneration
	}
	fullURL := c.resourceURL(ref, resource.Metadata.Name, "", nil)
	status, responseHeaders, data, err := c.do(
		ctx, http.MethodPut, fullURL, headers, raw, true,
		http.StatusOK, http.StatusCreated, http.StatusAccepted,
	)
	if err != nil {
		return nil, err
	}
	return c.finishResourceMutation(ctx, status, responseHeaders, data, fullURL, ref, resource.Metadata.Name, resource.Metadata.Space)
}

// finishResourceMutation resolves the mutationAccepted envelope: a direct
// 200/201 resource with its revision ETag, or a 202 Operation polled to a
// terminal result carrying { "resource": ... }.
//
// Every failure from here on is wrapped in *AcceptedError: the host already
// answered with a success status, so the mutation was accepted and the
// resource may exist even though this call cannot return a verified
// representation. Callers that persist state depend on that distinction to
// avoid orphaning what the host now owns.
func (c *Client) finishResourceMutation(
	ctx context.Context,
	status int,
	responseHeaders http.Header,
	data []byte,
	fullURL string,
	ref FormRef,
	name, space string,
) (*Resource, error) {
	if status == http.StatusAccepted {
		operation, err := c.decodeOperationEnvelope(data, fullURL)
		if err != nil {
			// The host accepted the mutation; only the operation envelope naming
			// it was unreadable, so there is no id to resume from.
			return nil, acceptedMutation("", "", err)
		}
		operationID := operation.ID
		uid := operationTargetUID(operation)
		terminal, err := c.awaitOperation(ctx, operation, parseRetryAfter(responseHeaders.Get("Retry-After")), 0)
		if got := operationTargetUID(terminal); got != "" {
			uid = got
		}
		if err != nil {
			return nil, acceptedMutation(operationID, uid, err)
		}
		var envelope struct {
			Resource Resource `json:"resource"`
		}
		if err := decodeBody(terminal.Result, fullURL, &envelope); err != nil {
			return nil, acceptedMutation(operationID, uid,
				fmt.Errorf("takoform: terminal operation result is not a resource envelope: %w", err))
		}
		out := envelope.Resource
		if err := verifyResourceResponse(ref, name, space, &out); err != nil {
			return nil, acceptedMutation(operationID, uid, err)
		}
		return &out, nil
	}
	var out Resource
	if err := decodeBody(data, fullURL, &out); err != nil {
		return nil, acceptedMutation("", "", err)
	}
	if err := verifyResourceResponse(ref, name, space, &out); err != nil {
		return nil, acceptedMutation("", "", err)
	}
	if err := requireRevisionETag(responseHeaders.Values("ETag"), out.Metadata.Revision); err != nil {
		return nil, acceptedMutation("", out.Metadata.UID, err)
	}
	return &out, nil
}

// operationTargetUID returns the host-issued resource identity an operation
// discloses through its target, when it names a syntactically valid one.
func operationTargetUID(operation *Operation) string {
	if operation == nil || operation.Target == nil || !uidPattern.MatchString(operation.Target.UID) {
		return ""
	}
	return operation.Target.UID
}

// GetResource reads a resource by exact FormRef query. Only the stable
// resource_not_found code is translated to ErrNotFound; other errors retain
// their typed API identity.
func (c *Client) GetResource(ctx context.Context, space string, ref FormRef, name string) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if err := ValidateFormRef(ref); err != nil {
		return nil, err
	}
	if err := ValidateSpaceID(space); err != nil {
		return nil, fmt.Errorf("takoform: resource read has invalid SpaceID: %w", err)
	}
	if err := ValidateResourceName(name); err != nil {
		return nil, err
	}
	fullURL := c.resourceURL(ref, name, "", exactFormQuery(space, ref))
	_, responseHeaders, data, err := c.do(ctx, http.MethodGet, fullURL, nil, nil, false, http.StatusOK)
	if err != nil {
		if isResourceNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var out Resource
	if err := decodeBody(data, fullURL, &out); err != nil {
		return nil, err
	}
	if err := verifyResourceResponse(ref, name, space, &out); err != nil {
		return nil, err
	}
	if err := requireRevisionETag(responseHeaders.Values("ETag"), out.Metadata.Revision); err != nil {
		return nil, err
	}
	return &out, nil
}

// ObserveResource performs a read-only observation of the incarnation named by
// uid, under a generation fence; the response must reflect exactly the fenced
// generation. The uid names the incarnation in the Idempotency-Key
// (incarnationKey); it is not sent on the wire.
func (c *Client) ObserveResource(ctx context.Context, space string, ref FormRef, name, uid, generation string) (*Resource, error) {
	return c.fencedStatusAction(ctx, "observe", space, ref, name, uid, generation)
}

// fencedStatusAction performs one fenced, read-only status action. observe is
// the lane's only such action: v1beta1 has no refresh operation, because two
// fenced read-only re-observations with the same contract were one operation
// spelled two ways.
func (c *Client) fencedStatusAction(ctx context.Context, action, space string, ref FormRef, name, uid, generation string) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if err := ValidateFormRef(ref); err != nil {
		return nil, err
	}
	if err := ValidateSpaceID(space); err != nil {
		return nil, fmt.Errorf("takoform: resource %s has invalid SpaceID: %w", action, err)
	}
	if err := ValidateResourceName(name); err != nil {
		return nil, err
	}
	if !validCanonicalDecimal(generation) {
		return nil, fmt.Errorf("takoform: %s requires a canonical decimal generation fence", action)
	}
	if !uidPattern.MatchString(uid) {
		return nil, fmt.Errorf("takoform: %s requires the recorded metadata.uid of the incarnation being observed", action)
	}
	headers := map[string]string{
		expectedGenerationHeader: generation,
		"Idempotency-Key": incarnationKey(
			action, ref, name, space, uid, generation, "",
		),
	}
	fullURL := c.resourceURL(ref, name, action, exactFormQuery(space, ref))
	_, responseHeaders, data, err := c.do(ctx, http.MethodPost, fullURL, headers, nil, true, http.StatusOK)
	if err != nil {
		if isResourceNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var envelope struct {
		Resource Resource `json:"resource"`
	}
	if err := decodeBody(data, fullURL, &envelope); err != nil {
		return nil, err
	}
	out := envelope.Resource
	if err := verifyResourceResponse(ref, name, space, &out); err != nil {
		return nil, err
	}
	if out.Metadata.Generation != generation {
		return nil, fmt.Errorf("takoform: host %s response changed the generation protected by the fence", action)
	}
	// The envelope wraps the representation; when the host supplies an ETag
	// it must still be the quoted revision. Absence is tolerated because the
	// operations table does not require one for the envelope shape.
	if etags := responseHeaders.Values("ETag"); len(etags) > 0 {
		if err := requireRevisionETag(etags, out.Metadata.Revision); err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// DeleteResource deletes the incarnation named by uid, under the
// expected-generation fence.
//
// A delete withdraws desired state, so it fences like every other desired-state
// mutation of this lane and NOT on the representation revision. The revision
// moves for reasons the deleting client did not cause: a host-side status
// change, and above all the derived rendering of decision 0016 — removing a
// dependent re-renders whatever was rendered from it, which during a teardown
// is precisely the resource the client is about to delete next. A client that
// fenced on the revision would be refused by a revision its own teardown moved,
// with a plan already computed and nothing left to do but waive the fence.
//
// The uid is REQUIRED, and it names the incarnation in the Idempotency-Key
// rather than on the wire (incarnationKey). It costs nothing to supply: a
// generation is only ever read off a verified representation, and such a
// representation always carries a uid (verifyResourceResponse), so a caller
// that can name the fence can name the incarnation.
//
// A direct 404 or a terminal delete Operation failing resource_not_found
// returns ErrNotFound so callers can treat "already gone" deliberately.
func (c *Client) DeleteResource(ctx context.Context, space string, ref FormRef, name, uid, generation string) error {
	if err := c.requireReady(); err != nil {
		return err
	}
	if err := ValidateFormRef(ref); err != nil {
		return err
	}
	if err := ValidateSpaceID(space); err != nil {
		return fmt.Errorf("takoform: resource delete has invalid SpaceID: %w", err)
	}
	if err := ValidateResourceName(name); err != nil {
		return err
	}
	if !validCanonicalDecimal(generation) {
		return errors.New("takoform: delete requires a canonical decimal generation fence")
	}
	if !uidPattern.MatchString(uid) {
		return errors.New("takoform: delete requires the recorded metadata.uid of the incarnation being removed")
	}
	headers := map[string]string{
		expectedGenerationHeader: generation,
		"Idempotency-Key": incarnationKey(
			"delete", ref, name, space, uid, generation, "",
		),
	}
	fullURL := c.resourceURL(ref, name, "", exactFormQuery(space, ref))
	status, responseHeaders, data, err := c.do(
		ctx, http.MethodDelete, fullURL, headers, nil, true,
		http.StatusNoContent, http.StatusAccepted,
	)
	if err != nil {
		if isResourceNotFound(err) {
			return ErrNotFound
		}
		return err
	}
	if status == http.StatusNoContent {
		return nil
	}
	operation, err := c.decodeOperationEnvelope(data, fullURL)
	if err != nil {
		return err
	}
	if _, err := c.awaitOperation(ctx, operation, parseRetryAfter(responseHeaders.Get("Retry-After")), 0); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == "resource_not_found" {
			return ErrNotFound
		}
		return err
	}
	return nil
}

type importRequestBody struct {
	resourceRequestBody
	NativeID string `json:"nativeId"`
}

// ImportResource adopts one existing provider-native object using the full
// desired resource. New adoptions carry If-None-Match: *; a resource whose
// metadata.generation is set is fenced as an existing-resource import. A 202
// response is polled to its terminal Operation.
//
// A fenced import addresses an incarnation the caller has already read, so
// metadata.uid names it in the Idempotency-Key (incarnationKey) and a second
// incarnation of the name cannot be answered with the first one's record. A new
// adoption names no incarnation because none exists yet; see incarnationKey for
// what that leaves open.
func (c *Client) ImportResource(ctx context.Context, resource *Resource, nativeID string) (*Resource, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if err := validateRequestResource(resource); err != nil {
		return nil, err
	}
	if strings.TrimSpace(nativeID) == "" {
		return nil, errors.New("takoform: import requires a nativeId")
	}
	generation := resource.Metadata.Generation
	if generation != "" && !validCanonicalDecimal(generation) {
		return nil, errors.New("takoform: import generation fence must be canonical decimal in 1..9223372036854775807")
	}
	uid := resource.Metadata.UID
	if uid != "" && !uidPattern.MatchString(uid) {
		return nil, errors.New("takoform: import metadata.uid is not a valid UID")
	}
	if generation != "" && uid == "" {
		return nil, errors.New("takoform: a fenced import requires the recorded metadata.uid of the incarnation being adopted")
	}
	ref := resource.Form.FormRef
	request := importRequestBody{
		resourceRequestBody: requestBodyFor(resource),
		NativeID:            nativeID,
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("takoform: encoding import request: %w", err)
	}
	headers := map[string]string{
		"Idempotency-Key": incarnationKey(
			"import", ref, resource.Metadata.Name, resource.Metadata.Space,
			uid, generation, bodyDigest(raw),
		),
	}
	if generation == "" {
		headers["If-None-Match"] = "*"
	} else {
		headers[expectedGenerationHeader] = generation
	}
	fullURL := c.resourceURL(ref, resource.Metadata.Name, "import", nil)
	status, responseHeaders, data, err := c.do(
		ctx, http.MethodPost, fullURL, headers, raw, true,
		http.StatusOK, http.StatusCreated, http.StatusAccepted,
	)
	if err != nil {
		return nil, err
	}
	return c.finishResourceMutation(ctx, status, responseHeaders, data, fullURL, ref, resource.Metadata.Name, resource.Metadata.Space)
}

func canonicalValueDigest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return digestCanonical(raw)
}
