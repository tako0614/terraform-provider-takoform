package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	FeatureResourceFormTransition = "resource_form_transition"

	formTransitionEvidenceFormat                     = "takoform.module-form-transition@v1"
	formTransitionOperationFormat                    = "takoform.resource-form-transition-operation@v1"
	formTransitionRequestFormat                      = "takoform.resource-form-transition-request@v1"
	relationalDatabaseV2ToV3TransitionEvidenceMarker = "relational-database-v2-to-v3"
)

// NativeResourceIdentity is the non-secret native identity already exposed by
// the resource state. It is evidence for same-resource continuity, never an
// instruction to replace or select a native object.
type NativeResourceIdentity struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type FormTransitionExpected struct {
	ResourceVersion string                  `json:"resourceVersion,omitempty"`
	NativeIdentity  *NativeResourceIdentity `json:"nativeIdentity,omitempty"`
}

type FormTransitionEvidence struct {
	Format string `json:"format"`
	Marker string `json:"marker"`
	Digest string `json:"digest"`
}

type FormTransitionRequest struct {
	OperationID        string                  `json:"operationId"`
	FromForm           InstalledFormReference  `json:"fromForm"`
	ToForm             InstalledFormReference  `json:"toForm"`
	Resource           Resource                `json:"resource"`
	Expected           *FormTransitionExpected `json:"expected,omitempty"`
	TransitionEvidence FormTransitionEvidence  `json:"transitionEvidence"`
}

type FormTransitionOperation struct {
	OperationID       string `json:"operationId"`
	Status            string `json:"status"`
	RequestDigest     string `json:"requestDigest"`
	ReconcilePath     string `json:"reconcilePath"`
	DispatchAttempted *bool  `json:"dispatchAttempted,omitempty"`
}

type FormTransitionProof struct {
	OperationID              string                 `json:"operationId"`
	FromForm                 InstalledFormReference `json:"fromForm"`
	ToForm                   InstalledFormReference `json:"toForm"`
	TransitionEvidenceDigest string                 `json:"transitionEvidenceDigest"`
	ObservedSpecDigest       string                 `json:"observedSpecDigest"`
	ResourceVersion          string                 `json:"resourceVersion"`
	NativeIdentity           NativeResourceIdentity `json:"nativeIdentity"`
	Committed                bool                   `json:"committed"`
}

type FormTransitionResponse struct {
	Operation       FormTransitionOperation `json:"operation"`
	Resource        *Resource               `json:"resource,omitempty"`
	TransitionProof *FormTransitionProof    `json:"transitionProof,omitempty"`
}

// FormTransitionIndeterminateError means the mutation might have committed,
// but exact readback could not yet prove either outcome. Callers must preserve
// prior state and reconcile OperationID; they must not invent a new mutation.
type FormTransitionIndeterminateError struct {
	OperationID   string
	ReconcilePath string
	Cause         error
}

func (e *FormTransitionIndeterminateError) Error() string {
	detail := ""
	if e.Cause != nil {
		detail = ": " + e.Cause.Error()
	}
	return fmt.Sprintf(
		"takoform: Form transition %s is indeterminate; reconcile %s before issuing another mutation%s",
		e.OperationID,
		e.ReconcilePath,
		detail,
	)
}

func (e *FormTransitionIndeterminateError) Unwrap() error { return e.Cause }

func NewFormTransitionEvidence(
	marker string,
	fromForm InstalledFormReference,
	toForm InstalledFormReference,
) (FormTransitionEvidence, error) {
	digest, err := TransitionEvidenceDigest(marker, fromForm, toForm)
	if err != nil {
		return FormTransitionEvidence{}, err
	}
	return FormTransitionEvidence{
		Format: formTransitionEvidenceFormat,
		Marker: marker,
		Digest: digest,
	}, nil
}

// TransitionEvidenceDigest binds the closed product/module declaration to one
// exact Form pair. Resource identity and desired spec are independently bound
// by the request OperationID and observedSpecDigest proof.
func TransitionEvidenceDigest(
	marker string,
	fromForm InstalledFormReference,
	toForm InstalledFormReference,
) (string, error) {
	statement := struct {
		Format   string                 `json:"format"`
		Marker   string                 `json:"marker"`
		FromForm InstalledFormReference `json:"fromForm"`
		ToForm   InstalledFormReference `json:"toForm"`
	}{
		Format:   formTransitionEvidenceFormat,
		Marker:   marker,
		FromForm: fromForm,
		ToForm:   toForm,
	}
	return canonicalValueDigest(statement)
}

func NewFormTransitionRequest(
	fromForm InstalledFormReference,
	toForm InstalledFormReference,
	resource Resource,
	expected FormTransitionExpected,
	evidence FormTransitionEvidence,
) (FormTransitionRequest, error) {
	request := FormTransitionRequest{
		FromForm:           fromForm,
		ToForm:             toForm,
		Resource:           resource,
		TransitionEvidence: evidence,
	}
	if expected.ResourceVersion != "" || expected.NativeIdentity != nil {
		request.Expected = &expected
	}
	if err := validateFormTransitionRequest(request); err != nil {
		return FormTransitionRequest{}, err
	}
	operationID, err := formTransitionOperationID(request)
	if err != nil {
		return FormTransitionRequest{}, err
	}
	request.OperationID = operationID
	return request, nil
}

func formTransitionOperationID(request FormTransitionRequest) (string, error) {
	desiredSpecDigest, err := canonicalValueDigest(request.Resource.Spec)
	if err != nil {
		return "", fmt.Errorf("takoform: digesting Form transition desired spec: %w", err)
	}
	statement := struct {
		Format             string                  `json:"format"`
		Resource           formTransitionResource  `json:"resource"`
		FromForm           InstalledFormReference  `json:"fromForm"`
		ToForm             InstalledFormReference  `json:"toForm"`
		DesiredSpecDigest  string                  `json:"desiredSpecDigest"`
		Expected           *FormTransitionExpected `json:"expected,omitempty"`
		TransitionEvidence FormTransitionEvidence  `json:"transitionEvidence"`
	}{
		Format: formTransitionOperationFormat,
		Resource: formTransitionResource{
			Space: request.Resource.Metadata.Space,
			Kind:  request.Resource.Kind,
			Name:  request.Resource.Metadata.Name,
		},
		FromForm:           request.FromForm,
		ToForm:             request.ToForm,
		DesiredSpecDigest:  desiredSpecDigest,
		Expected:           request.Expected,
		TransitionEvidence: request.TransitionEvidence,
	}
	digest, err := canonicalValueDigest(statement)
	if err != nil {
		return "", fmt.Errorf("takoform: binding Form transition operation: %w", err)
	}
	return "formtx_" + strings.TrimPrefix(digest, "sha256:"), nil
}

type formTransitionResource struct {
	Space string `json:"space"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
}

func formTransitionRequestDigest(request FormTransitionRequest) (string, error) {
	desiredSpecDigest, err := canonicalValueDigest(request.Resource.Spec)
	if err != nil {
		return "", fmt.Errorf("takoform: digesting Form transition desired spec: %w", err)
	}
	statement := struct {
		Format             string                  `json:"format"`
		OperationID        string                  `json:"operationId"`
		FromForm           InstalledFormReference  `json:"fromForm"`
		ToForm             InstalledFormReference  `json:"toForm"`
		DesiredSpecDigest  string                  `json:"desiredSpecDigest"`
		Expected           *FormTransitionExpected `json:"expected,omitempty"`
		TransitionEvidence FormTransitionEvidence  `json:"transitionEvidence"`
	}{
		Format:             formTransitionRequestFormat,
		OperationID:        request.OperationID,
		FromForm:           request.FromForm,
		ToForm:             request.ToForm,
		DesiredSpecDigest:  desiredSpecDigest,
		Expected:           request.Expected,
		TransitionEvidence: request.TransitionEvidence,
	}
	return canonicalValueDigest(statement)
}

func validateFormTransitionRequest(request FormTransitionRequest) error {
	if request.FromForm == request.ToForm {
		return errors.New("takoform: Form transition requires two different exact FormRefs")
	}
	if err := validateInstalledFormReference(request.FromForm.FormRef.Kind, request.FromForm); err != nil {
		return fmt.Errorf("takoform: invalid fromForm: %w", err)
	}
	if err := validateInstalledFormReference(request.ToForm.FormRef.Kind, request.ToForm); err != nil {
		return fmt.Errorf("takoform: invalid toForm: %w", err)
	}
	if request.FromForm.FormRef.Kind != request.ToForm.FormRef.Kind {
		return errors.New("takoform: Form transition cannot change Resource kind")
	}
	if err := validateResourceIdentity(request.ToForm.FormRef.Kind, &request.Resource); err != nil {
		return fmt.Errorf("takoform: invalid transition Resource: %w", err)
	}
	if request.Resource.Form == nil || *request.Resource.Form != request.ToForm {
		return errors.New("takoform: transition Resource must be bound to exact toForm")
	}
	if request.Resource.Status != nil {
		return errors.New("takoform: transition Resource must carry desired state only")
	}
	if request.Expected == nil || !validResourceVersion(request.Expected.ResourceVersion) ||
		request.Expected.ResourceVersion != request.Resource.Metadata.ResourceVersion {
		return errors.New("takoform: Form transition requires the exact state-recorded resourceVersion")
	}
	if request.Expected != nil {
		if request.Expected.NativeIdentity != nil &&
			(strings.TrimSpace(request.Expected.NativeIdentity.Type) == "" || strings.TrimSpace(request.Expected.NativeIdentity.ID) == "") {
			return errors.New("takoform: expected nativeIdentity is incomplete")
		}
	}
	digest, err := TransitionEvidenceDigest(
		request.TransitionEvidence.Marker,
		request.FromForm,
		request.ToForm,
	)
	if err != nil {
		return err
	}
	if request.TransitionEvidence.Format != formTransitionEvidenceFormat ||
		request.TransitionEvidence.Marker != relationalDatabaseV2ToV3TransitionEvidenceMarker ||
		request.TransitionEvidence.Digest != digest {
		return errors.New("takoform: transition evidence is not the compiled exact DB2-to-DB3 declaration")
	}
	return nil
}

func (c *Client) formTransitionURL(kind, name, space, operationID string) string {
	path := fmt.Sprintf(
		"%s/resources/%s/%s/form-transitions",
		c.apiBase,
		url.PathEscape(kind),
		url.PathEscape(name),
	)
	if operationID != "" {
		path += "/" + url.PathEscape(operationID)
	}
	query := spaceQuery(space)
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return path
}

// TransitionResourceForm first reads the deterministic operation identity.
// Only the protocol's exact operation-not-found response or an exact bound
// prepared operation with dispatchAttempted=false permits one POST. An
// ambiguous mutation acknowledgement is followed by at most one additional
// read-only operation readback; the POST is never retried.
func (c *Client) TransitionResourceForm(
	ctx context.Context,
	request FormTransitionRequest,
) (*FormTransitionResponse, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if !c.Discovery.HasFeature(FeatureResourceFormTransition) {
		return nil, fmt.Errorf("takoform: host does not advertise features.%s", FeatureResourceFormTransition)
	}
	if strings.TrimSpace(request.OperationID) == "" {
		return nil, errors.New("takoform: Form transition requires operationId")
	}
	if err := validateFormTransitionRequest(request); err != nil {
		return nil, err
	}
	wantOperationID, err := formTransitionOperationID(request)
	if err != nil {
		return nil, err
	}
	if request.OperationID != wantOperationID {
		return nil, errors.New("takoform: Form transition operationId does not bind the exact request")
	}
	requestDigest, err := formTransitionRequestDigest(request)
	if err != nil {
		return nil, err
	}
	preflight, preflightMetadata, absent, err := c.readFormTransition(ctx, request)
	if err != nil {
		return nil, err
	}
	if !absent {
		resume, err := c.preflightFormTransition(request, requestDigest, preflight, preflightMetadata)
		if err != nil {
			return nil, err
		}
		if !resume {
			return c.projectFormTransitionReadback(request, requestDigest, preflight, preflightMetadata)
		}
	}
	url := c.formTransitionURL(
		request.ToForm.FormRef.Kind,
		request.Resource.Metadata.Name,
		request.Resource.Metadata.Space,
		"",
	)
	var response FormTransitionResponse
	headers := map[string]string{
		"Idempotency-Key": request.OperationID,
		"If-Match":        quoteResourceVersion(request.Resource.Metadata.ResourceVersion),
	}
	responseMetadata, err := c.doJSONWithStatus(
		ctx,
		http.MethodPost,
		url,
		headers,
		&request,
		&response,
		false,
		http.StatusOK,
		http.StatusAccepted,
	)
	if err == nil {
		if responseMetadata.StatusCode == http.StatusAccepted {
			cause := validateUnresolvedTransitionOperation(request, requestDigest, response.Operation)
			if response.Resource != nil || response.TransitionProof != nil {
				cause = errors.Join(cause, errors.New("takoform: unresolved transition readback carried committed Resource or proof"))
			}
			reconcilePath := response.Operation.ReconcilePath
			if cause != nil {
				reconcilePath = c.formTransitionURL(
					request.ToForm.FormRef.Kind,
					request.Resource.Metadata.Name,
					request.Resource.Metadata.Space,
					request.OperationID,
				)
			}
			return nil, &FormTransitionIndeterminateError{
				OperationID:   request.OperationID,
				ReconcilePath: reconcilePath,
				Cause:         cause,
			}
		}
		return c.validateFormTransitionResponse(request, requestDigest, response, responseMetadata.Headers)
	}
	if !isAmbiguousTransportError(err) {
		return nil, err
	}
	return c.reconcileFormTransition(ctx, request, err)
}

func (c *Client) preflightFormTransition(
	request FormTransitionRequest,
	requestDigest string,
	readback FormTransitionResponse,
	metadata jsonResponseMetadata,
) (bool, error) {
	if readback.Operation.RequestDigest != requestDigest {
		return false, errors.New("takoform: existing Form transition operation is bound to a different exact request")
	}
	if metadata.StatusCode != http.StatusAccepted || readback.Operation.Status != "prepared" ||
		readback.Operation.DispatchAttempted == nil || *readback.Operation.DispatchAttempted {
		return false, nil
	}
	if readback.Resource != nil || readback.TransitionProof != nil {
		return false, &FormTransitionIndeterminateError{
			OperationID: request.OperationID,
			ReconcilePath: c.formTransitionURL(
				request.ToForm.FormRef.Kind,
				request.Resource.Metadata.Name,
				request.Resource.Metadata.Space,
				request.OperationID,
			),
			Cause: errors.New("takoform: prepared transition readback carried committed Resource or proof"),
		}
	}
	if err := validateUnresolvedTransitionOperation(request, requestDigest, readback.Operation); err != nil {
		return false, &FormTransitionIndeterminateError{
			OperationID:   request.OperationID,
			ReconcilePath: readback.Operation.ReconcilePath,
			Cause:         err,
		}
	}
	return true, nil
}

func (c *Client) reconcileFormTransition(
	ctx context.Context,
	request FormTransitionRequest,
	cause error,
) (*FormTransitionResponse, error) {
	reconcileURL := c.formTransitionURL(
		request.ToForm.FormRef.Kind,
		request.Resource.Metadata.Name,
		request.Resource.Metadata.Space,
		request.OperationID,
	)
	response, metadata, absent, err := c.readFormTransition(ctx, request)
	if err != nil || absent {
		if absent {
			err = errors.New("takoform: transition operation disappeared after ambiguous mutation acknowledgement")
		}
		return nil, &FormTransitionIndeterminateError{
			OperationID:   request.OperationID,
			ReconcilePath: reconcileURL,
			Cause:         errors.Join(cause, err),
		}
	}
	requestDigest, err := formTransitionRequestDigest(request)
	if err != nil {
		return nil, err
	}
	result, err := c.projectFormTransitionReadback(request, requestDigest, response, metadata)
	if err != nil {
		var indeterminate *FormTransitionIndeterminateError
		if errors.As(err, &indeterminate) {
			indeterminate.Cause = errors.Join(cause, indeterminate.Cause)
		}
	}
	return result, err
}

func (c *Client) readFormTransition(
	ctx context.Context,
	request FormTransitionRequest,
) (FormTransitionResponse, jsonResponseMetadata, bool, error) {
	reconcileURL := c.formTransitionURL(
		request.ToForm.FormRef.Kind,
		request.Resource.Metadata.Name,
		request.Resource.Metadata.Space,
		request.OperationID,
	)
	var response FormTransitionResponse
	metadata, err := c.doJSONWithStatus(
		ctx,
		http.MethodGet,
		reconcileURL,
		nil,
		nil,
		&response,
		false,
		http.StatusOK,
		http.StatusAccepted,
	)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) &&
			apiErr.StatusCode == http.StatusNotFound &&
			apiErr.Code == "resource_not_found" &&
			apiErr.HostCode == "form_transition_operation_not_found" &&
			!apiErr.Retryable &&
			!apiErr.ProtocolInvalid {
			return FormTransitionResponse{}, jsonResponseMetadata{}, true, nil
		}
		if errors.As(err, &apiErr) && isExactFailedFormTransition(apiErr, request) {
			return FormTransitionResponse{}, jsonResponseMetadata{}, false, fmt.Errorf(
				"takoform: Form transition %s failed definitively [%s]; change the exact resource revision before requesting another operation",
				request.OperationID,
				apiErr.HostCode,
			)
		}
		return FormTransitionResponse{}, jsonResponseMetadata{}, false, &FormTransitionIndeterminateError{
			OperationID:   request.OperationID,
			ReconcilePath: reconcileURL,
			Cause:         err,
		}
	}
	return response, metadata, false, nil
}

type failedFormTransitionDetails struct {
	OperationID   string `json:"operationId"`
	RequestDigest string `json:"requestDigest"`
	Status        string `json:"status"`
	FailureCode   string `json:"failureCode"`
}

func isExactFailedFormTransition(apiErr *APIError, request FormTransitionRequest) bool {
	if apiErr == nil || apiErr.StatusCode != http.StatusConflict ||
		apiErr.Code != "form_identity_conflict" || apiErr.ProtocolInvalid || apiErr.Retryable ||
		strings.TrimSpace(apiErr.HostCode) == "" {
		return false
	}
	requestDigest, err := formTransitionRequestDigest(request)
	if err != nil {
		return false
	}
	var details failedFormTransitionDetails
	if err := decodeStrictJSON(apiErr.Details, &details); err != nil {
		return false
	}
	return details.OperationID == request.OperationID &&
		details.RequestDigest == requestDigest &&
		details.Status == "failed" &&
		details.FailureCode == apiErr.HostCode
}

func (c *Client) projectFormTransitionReadback(
	request FormTransitionRequest,
	requestDigest string,
	readback FormTransitionResponse,
	metadata jsonResponseMetadata,
) (*FormTransitionResponse, error) {
	if readback.Operation.RequestDigest != requestDigest {
		return nil, errors.New("takoform: existing Form transition operation is bound to a different exact request")
	}
	if metadata.StatusCode == http.StatusAccepted {
		err := validateUnresolvedTransitionOperation(request, requestDigest, readback.Operation)
		if readback.Resource != nil || readback.TransitionProof != nil {
			err = errors.Join(err, errors.New("takoform: unresolved transition readback carried committed Resource or proof"))
		}
		if err != nil {
			return nil, &FormTransitionIndeterminateError{
				OperationID: request.OperationID,
				ReconcilePath: c.formTransitionURL(
					request.ToForm.FormRef.Kind,
					request.Resource.Metadata.Name,
					request.Resource.Metadata.Space,
					request.OperationID,
				),
				Cause: err,
			}
		}
		return nil, &FormTransitionIndeterminateError{
			OperationID:   request.OperationID,
			ReconcilePath: readback.Operation.ReconcilePath,
		}
	}
	if metadata.StatusCode != http.StatusOK {
		return nil, errors.New("takoform: host returned an invalid Form transition readback status")
	}
	return c.validateFormTransitionResponse(request, requestDigest, readback, metadata.Headers)
}

func validateUnresolvedTransitionOperation(
	request FormTransitionRequest,
	requestDigest string,
	operation FormTransitionOperation,
) error {
	if operation.OperationID != request.OperationID ||
		(operation.Status != "prepared" && operation.Status != "indeterminate") ||
		operation.RequestDigest != requestDigest ||
		strings.TrimSpace(operation.ReconcilePath) == "" ||
		operation.DispatchAttempted == nil ||
		(operation.Status == "prepared" && *operation.DispatchAttempted) ||
		(operation.Status == "indeterminate" && !*operation.DispatchAttempted) {
		return errors.New("takoform: host did not return exact operation-bound indeterminate readback")
	}
	return nil
}

func (c *Client) validateFormTransitionResponse(
	request FormTransitionRequest,
	requestDigest string,
	response FormTransitionResponse,
	headers http.Header,
) (*FormTransitionResponse, error) {
	if response.Operation.OperationID != request.OperationID ||
		response.Operation.Status != "committed" ||
		response.Operation.RequestDigest != requestDigest ||
		strings.TrimSpace(response.Operation.ReconcilePath) == "" {
		return nil, errors.New("takoform: host did not return exact committed transition operation readback")
	}
	if response.Resource == nil || response.TransitionProof == nil {
		return nil, errors.New("takoform: committed transition readback omitted Resource or proof")
	}
	proof := *response.TransitionProof
	expectedGeneration, expectedErr := strconv.ParseInt(request.Expected.ResourceVersion, 10, 64)
	proofGeneration, proofErr := strconv.ParseInt(proof.ResourceVersion, 10, 64)
	evidenceDigest, err := TransitionEvidenceDigest(
		request.TransitionEvidence.Marker,
		request.FromForm,
		request.ToForm,
	)
	if err != nil {
		return nil, err
	}
	specDigest, err := canonicalValueDigest(request.Resource.Spec)
	if err != nil {
		return nil, err
	}
	if !proof.Committed ||
		proof.OperationID != request.OperationID ||
		proof.FromForm != request.FromForm ||
		proof.ToForm != request.ToForm ||
		proof.TransitionEvidenceDigest != evidenceDigest ||
		proof.ObservedSpecDigest != specDigest ||
		!validResourceVersion(proof.ResourceVersion) ||
		expectedErr != nil || proofErr != nil || proofGeneration != expectedGeneration+1 {
		return nil, errors.New("takoform: host returned substituted or incomplete Form transition proof")
	}
	if request.Expected != nil && request.Expected.NativeIdentity != nil &&
		proof.NativeIdentity != *request.Expected.NativeIdentity {
		return nil, errors.New("takoform: transition proof changed expected native identity")
	}
	if strings.TrimSpace(proof.NativeIdentity.Type) == "" || strings.TrimSpace(proof.NativeIdentity.ID) == "" {
		return nil, errors.New("takoform: transition proof omitted native identity")
	}
	if err := verifyResourceIdentity(
		&request.ToForm,
		request.Resource.Metadata.Name,
		request.Resource.Metadata.Space,
		response.Resource,
	); err != nil {
		return nil, fmt.Errorf("takoform: transition proof Resource changed identity: %w", err)
	}
	if err := captureResourceVersion(response.Resource, headers); err != nil {
		return nil, err
	}
	if response.Resource.Metadata.ResourceVersion != proof.ResourceVersion {
		return nil, errors.New("takoform: transition proof and Resource versions disagree")
	}
	returnedDigest, err := canonicalValueDigest(response.Resource.Spec)
	if err != nil || returnedDigest != specDigest {
		return nil, errors.New("takoform: transition response did not prove exact desired spec")
	}
	return &response, nil
}

func isAmbiguousTransportError(err error) bool {
	var uncertain *requestOutcomeUncertainError
	return errors.As(err, &uncertain)
}
