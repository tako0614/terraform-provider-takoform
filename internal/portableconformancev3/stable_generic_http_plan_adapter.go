package portableconformancev3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// genericHTTPPlanAdapter is deliberately only a serializer/normalizer over a
// real Snapshot-backed Host. It owns wire mappings, actual opaque handles, and
// normalization to the symbolic handles shared with the memory adapter.
type genericHTTPPlanAdapter struct {
	seed           genericPlanSeed
	runner         *v3Runner
	server         *httptest.Server
	preparations   map[string]string
	operations     map[string]string
	uploads        map[string]genericHTTPUpload
	manifests      map[string]string
	rawUIDToSymbol map[string]string
	symbolToRawUID map[string]string
	incarnations   map[string]int
}

type genericHTTPUpload struct {
	actualID       string
	manifest       map[string]any
	blobDigest     string
	manifestHandle string
}

func genericHTTPFactory(seed genericPlanSeed) (genericPlanAdapter, func(), error) {
	return genericHTTPFactoryWithHost(seed, nil)
}

func genericHTTPFactoryWithHost(
	seed genericPlanSeed,
	configure func(*ReferenceHost),
) (genericPlanAdapter, func(), error) {
	catalog, err := stableGenericCatalog(seed.Snapshot, &seed.Probe)
	if err != nil {
		return nil, nil, err
	}
	if catalog.family != "" {
		return nil, nil, errors.New("generic HTTP plan selected a concrete family")
	}
	host := NewReferenceHost(seed.Contract, catalog)
	if configure != nil {
		configure(host)
	}
	server := httptest.NewServer(host)
	runner := stableGenericRunner(context.Background(), seed.Contract, server)
	runner.pinDesiredSchemas()
	adapter := &genericHTTPPlanAdapter{
		seed: seed, runner: runner, server: server,
		preparations: map[string]string{}, operations: map[string]string{},
		uploads: map[string]genericHTTPUpload{}, manifests: map[string]string{},
		rawUIDToSymbol: map[string]string{}, symbolToRawUID: map[string]string{},
		incarnations: map[string]int{},
	}
	return adapter, server.Close, nil
}

func (adapter *genericHTTPPlanAdapter) Call(
	ctx context.Context,
	actor genericActor,
	command genericCommand,
) (genericObservation, error) {
	if err := command.validate(); err != nil {
		return genericObservation{}, err
	}
	adapter.runner.ctx = ctx
	if code, probe := genericFaultErrorCode(command.Controls.Fault); probe {
		response, err := adapter.request(
			actor, http.MethodGet, adapter.server.URL+adapter.seed.Contract.DiscoveryPath,
			map[string]string{ErrorProbeHeader: ProbeErrorPrefix + code}, nil,
		)
		if err != nil {
			return genericObservation{}, err
		}
		return adapter.errorObservation(response)
	}
	switch {
	case command.Catalog != nil:
		return adapter.catalog(actor, *command.Catalog)
	case command.Admission != nil:
		return adapter.admission(actor, *command.Admission)
	case command.Resource != nil:
		return adapter.resource(actor, *command.Resource, command.Controls)
	case command.Operation != nil:
		return adapter.operation(actor, *command.Operation)
	case command.Artifact != nil:
		return adapter.artifact(actor, *command.Artifact, command.Controls)
	default:
		return genericObservation{}, errors.New("unreachable generic HTTP command")
	}
}

func (adapter *genericHTTPPlanAdapter) authorization(actor genericActor) string {
	switch actor.Credential {
	case genericCredentialPrimary:
		return "Bearer " + referencePrimaryToken
	case genericCredentialAlternate:
		return "Bearer " + referenceAlternateToken
	case genericCredentialOtherTenant:
		return "Bearer " + referenceAlternateTenantToken
	case genericCredentialUnknown:
		return "Bearer takoform-conformance-unissued-credential"
	default:
		return ""
	}
}

func (adapter *genericHTTPPlanAdapter) request(
	actor genericActor,
	method, target string,
	headers map[string]string,
	body []byte,
) (wireResponse, error) {
	return adapter.runner.requestWithAuthorization(adapter.authorization(actor), method, target, headers, body)
}

func (adapter *genericHTTPPlanAdapter) catalog(
	actor genericActor,
	request genericCatalogRequest,
) (genericObservation, error) {
	switch request.Action {
	case genericCatalogDiscover:
		response, err := adapter.request(actor, http.MethodGet, adapter.server.URL+adapter.seed.Contract.DiscoveryPath, nil, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		var discovery struct {
			APIVersions []string          `json:"api_versions"`
			Features    map[string]bool   `json:"features"`
			Endpoints   map[string]string `json:"endpoints"`
		}
		if err := decodeStrictResponse(response, &discovery); err != nil {
			return genericObservation{}, err
		}
		features := make([]string, 0, len(discovery.Features))
		for feature, enabled := range discovery.Features {
			if enabled {
				features = append(features, feature)
			}
		}
		sort.Strings(features)
		endpointPaths := make(map[string]string, len(discovery.Endpoints))
		for name, raw := range discovery.Endpoints {
			parsed, err := url.Parse(raw)
			if err != nil || !parsed.IsAbs() {
				return genericObservation{}, fmt.Errorf("discovery endpoint %s is not absolute: %q", name, raw)
			}
			endpointPaths[name] = parsed.EscapedPath()
		}
		return genericObservation{
			Code: "ok", APIVersions: discovery.APIVersions,
			Features: features, EndpointPaths: endpointPaths,
		}, nil
	case genericCatalogList:
		query := adapter.runner.formsAvailabilityQuery(request.Space, request.Ref)
		response, err := adapter.request(actor, http.MethodGet, adapter.runner.apiBase+"/forms?"+query.Encode(), nil, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		var answer struct {
			Forms []formsAvailabilityEntry `json:"forms"`
		}
		if err := decodeStrictResponse(response, &answer); err != nil {
			return genericObservation{}, err
		}
		forms := make([]genericObservedForm, 0, len(answer.Forms))
		for _, form := range answer.Forms {
			forms = append(forms, genericObservedForm{
				Ref: form.Identity.FormRef, PackageDigest: form.Identity.PackageDigest,
				DefinitionKnown: form.DefinitionKnown, Installed: form.Installed,
				Executable: form.Executable, Activated: form.Activated,
				AvailableToPrincipal: form.AvailableToPrincipal,
				Operations:           genericSortedOperations(form.Operations),
				DeprecatedPresent:    form.Deprecated != nil,
			})
		}
		sort.Slice(forms, func(i, j int) bool { return stableFormRefKey(forms[i].Ref) < stableFormRefKey(forms[j].Ref) })
		return genericObservation{Code: "ok", Forms: forms}, nil
	case genericCatalogGet:
		if request.Surface == genericCatalogSurfaceService {
			response, err := adapter.request(actor, http.MethodGet,
				adapter.runner.apiBase+"/support/standard-services/"+url.PathEscape(request.Protocol), nil, nil)
			if err != nil {
				return genericObservation{}, err
			}
			if response.Status != http.StatusOK {
				return adapter.errorObservation(response)
			}
			var profile struct {
				APIVersion string `json:"apiVersion"`
				Kind       string `json:"kind"`
				ServiceRef struct {
					APIVersion string `json:"apiVersion"`
					Protocol   string `json:"protocol"`
				} `json:"serviceRef"`
				Satisfiable bool `json:"satisfiable"`
			}
			if err := decodeStrictResponse(response, &profile); err != nil {
				return genericObservation{}, err
			}
			if profile.ServiceRef.APIVersion != adapter.seed.Probe.ExternalServices.ServiceAPIVersion {
				return genericObservation{}, fmt.Errorf("standard-service ref apiVersion = %q", profile.ServiceRef.APIVersion)
			}
			return genericObservation{Code: "ok", Support: &genericObservedSupport{
				APIVersion: profile.APIVersion, Kind: profile.Kind,
				Protocol: profile.ServiceRef.Protocol, Satisfiable: genericBool(profile.Satisfiable),
				ExtraKeys: []string{},
			}}, nil
		}
		if request.Surface == genericCatalogSurfaceSupport {
			response, err := adapter.request(actor, http.MethodGet, adapter.runner.formSupportURL(request.Ref), nil, nil)
			if err != nil {
				return genericObservation{}, err
			}
			if response.Status != http.StatusOK {
				return adapter.errorObservation(response)
			}
			var profile struct {
				APIVersion        string                    `json:"apiVersion"`
				Kind              string                    `json:"kind"`
				FormRef           FormRef                   `json:"formRef"`
				Operations        []string                  `json:"operations"`
				SupportedEnums    map[string][]string       `json:"supportedEnums,omitempty"`
				SupportedRanges   map[string]map[string]any `json:"supportedRanges,omitempty"`
				SupportedBindings []string                  `json:"supportedBindings,omitempty"`
				Limits            map[string]int            `json:"limits,omitempty"`
				ServiceRef        map[string]any            `json:"serviceRef,omitempty"`
				Satisfiable       *bool                     `json:"satisfiable,omitempty"`
			}
			if err := decodeStrictResponse(response, &profile); err != nil {
				return genericObservation{}, err
			}
			extraKeys := []string{}
			if len(profile.SupportedEnums) != 0 {
				extraKeys = append(extraKeys, "supportedEnums")
			}
			if len(profile.SupportedRanges) != 0 {
				extraKeys = append(extraKeys, "supportedRanges")
			}
			if len(profile.SupportedBindings) != 0 {
				extraKeys = append(extraKeys, "supportedBindings")
			}
			if len(profile.Limits) != 0 {
				extraKeys = append(extraKeys, "limits")
			}
			if profile.ServiceRef != nil {
				extraKeys = append(extraKeys, "serviceRef")
			}
			if profile.Satisfiable != nil {
				extraKeys = append(extraKeys, "satisfiable")
			}
			sort.Strings(extraKeys)
			return genericObservation{Code: "ok", Support: &genericObservedSupport{
				APIVersion: profile.APIVersion, Kind: profile.Kind, Ref: profile.FormRef,
				Operations: genericSortedOperations(profile.Operations), ExtraKeys: extraKeys,
			}}, nil
		}
		definitionURL := adapter.runner.formDefinitionURL(request.Ref)
		response, err := adapter.request(actor, http.MethodGet, definitionURL, nil, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		var definition wireFormDefinition
		if err := decodeStrictResponse(response, &definition); err != nil {
			return genericObservation{}, err
		}
		packageDigest := definition.Identity.PackageDigest
		raw, known := adapter.seed.Snapshot.Definition(formpackageFormRef(request.Ref))
		if !known {
			return genericObservation{}, errors.New("HTTP adapter seed omitted requested definition")
		}
		parsed, err := formpackage.ValidateDefinition(raw)
		if err != nil {
			return genericObservation{}, err
		}
		desiredSchemaHash, err := genericCanonicalHash(definition.DesiredSchema)
		if err != nil {
			return genericObservation{}, err
		}
		constraintsHash, err := genericCanonicalHash(definition.Constraints)
		if err != nil {
			return genericObservation{}, err
		}
		parsedDefinitionURL, err := url.Parse(definitionURL)
		if err != nil {
			return genericObservation{}, err
		}
		return genericObservation{
			Code: "ok", DefinitionDigest: definition.Identity.FormRef.SchemaDigest,
			AddressPath: parsedDefinitionURL.EscapedPath(),
			Definition: &genericObservedDefinition{
				Ref: definition.Identity.FormRef, PackageDigest: packageDigest,
				Title: definition.DisplayName, Description: definition.Description,
				DesiredSchemaHash: desiredSchemaHash, ConstraintsHash: constraintsHash,
			},
			Forms: []genericObservedForm{{
				Ref: definition.Identity.FormRef, PackageDigest: packageDigest,
				Operations: genericSortedOperations(parsed.LifecycleCapabilities),
			}},
		}, nil
	default:
		return genericObservation{}, fmt.Errorf("HTTP adapter unknown catalog action %q", request.Action)
	}
}

func (adapter *genericHTTPPlanAdapter) admission(
	actor genericActor,
	request genericAdmissionRequest,
) (genericObservation, error) {
	target := adapter.target(request.Resource)
	body, err := encodeRunnerJSON(adapter.runner.resourceBody(target))
	if err != nil {
		return genericObservation{}, err
	}
	switch request.Action {
	case genericAdmissionValidate:
		response, err := adapter.request(actor, http.MethodPost, adapter.runner.apiBase+"/resources/validate", nil, body)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		var answer struct {
			Valid       bool             `json:"valid"`
			Diagnostics []map[string]any `json:"diagnostics"`
			Resource    *wireResource    `json:"resource,omitempty"`
		}
		if err := decodeStrictResponse(response, &answer); err != nil {
			return genericObservation{}, err
		}
		observed := genericObservation{
			Code: "ok", Valid: genericBool(answer.Valid),
			HasDiagnostics: genericBool(len(answer.Diagnostics) != 0),
		}
		if answer.Resource != nil {
			observed.EffectiveDesired = cloneJSONMap(answer.Resource.Spec)
		} else if answer.Valid {
			materialized, err := stableGenericMaterialize(adapter.seed.Snapshot, request.Resource.Ref, request.Resource.Desired)
			if err != nil {
				return genericObservation{}, err
			}
			observed.EffectiveDesired = materialized
		}
		return observed, nil
	case genericAdmissionPrepare:
		headers := map[string]string{}
		if request.ExpectedGeneration != "" {
			headers[expectedGenerationHeader] = request.ExpectedGeneration
		}
		response, err := adapter.request(actor, http.MethodPost, adapter.runner.apiBase+"/resources/prepare", headers, body)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		var answer struct {
			Resource wireResource `json:"resource"`
			Review   struct {
				PrepareDigest string `json:"prepareDigest"`
				SpecDigest    string `json:"specDigest"`
			} `json:"review"`
		}
		if err := decodeStrictResponse(response, &answer); err != nil {
			return genericObservation{}, err
		}
		adapter.preparations[adapter.actorScope(actor)+"\x00"+request.PreparationHandle] = answer.Review.PrepareDigest
		return genericObservation{
			Code: "ok", PreparationHandle: request.PreparationHandle,
			PreparationSpecHash: answer.Review.SpecDigest,
			EffectiveDesired:    cloneJSONMap(answer.Resource.Spec),
		}, nil
	default:
		return genericObservation{}, fmt.Errorf("HTTP adapter unknown admission action %q", request.Action)
	}
}

func (adapter *genericHTTPPlanAdapter) resource(
	actor genericActor,
	request genericResourceRequest,
	controls genericControls,
) (genericObservation, error) {
	target := adapter.target(request.Resource)
	switch request.Action {
	case genericResourceRead:
		response, err := adapter.request(actor, http.MethodGet,
			adapter.runner.resourceURL(target.Ref, target.Name, "", adapter.runner.exactQuery(target.Space, target.Ref)), nil, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		resource, err := decodeResource(response, http.StatusOK)
		if err != nil {
			return genericObservation{}, err
		}
		return genericObservation{
			Code: "ok", ETag: response.Header.Get("ETag"),
			Resource: adapter.normalizeResource(request.Resource.Handle, resource),
		}, nil
	case genericResourceObserve:
		headers := map[string]string{
			expectedGenerationHeader: request.Resource.ExpectedGeneration,
			"Idempotency-Key":        request.Resource.IdempotencyKey,
		}
		if controls.BackendEffect == genericBackendEffectTouchStatus {
			headers[ErrorProbeHeader] = ProbeTouchStatus
		}
		response, err := adapter.request(actor, http.MethodPost,
			adapter.runner.resourceURL(target.Ref, target.Name, "observe", adapter.runner.exactQuery(target.Space, target.Ref)),
			headers, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		resource, err := decodeResourceEnvelope(response)
		if err != nil {
			return genericObservation{}, err
		}
		return genericObservation{
			Code: "ok", ETag: response.Header.Get("ETag"),
			Resource: adapter.normalizeResource(request.Resource.Handle, resource),
		}, nil
	case genericResourceDelete:
		headers := map[string]string{
			expectedGenerationHeader: request.Resource.ExpectedGeneration,
			"Idempotency-Key":        request.Resource.IdempotencyKey,
		}
		if controls.BackendEffect == genericBackendEffectExternalChange {
			headers[ErrorProbeHeader] = ProbeExternalChange
		}
		if request.Resource.ExpectedRevision != "" {
			headers["If-Match"] = `"` + request.Resource.ExpectedRevision + `"`
		}
		response, err := adapter.request(actor, http.MethodDelete,
			adapter.runner.resourceURL(target.Ref, target.Name, "", adapter.runner.exactQuery(target.Space, target.Ref)),
			headers, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusNoContent {
			return adapter.errorObservation(response)
		}
		return genericObservation{Code: "deleted"}, nil
	case genericResourceImport:
		document := adapter.runner.resourceBody(target)
		document["nativeId"] = request.Resource.NativeID
		body, err := encodeRunnerJSON(document)
		if err != nil {
			return genericObservation{}, err
		}
		headers := map[string]string{"Idempotency-Key": request.Resource.IdempotencyKey}
		if request.Resource.Create {
			headers["If-None-Match"] = "*"
		} else {
			headers[expectedGenerationHeader] = request.Resource.ExpectedGeneration
		}
		response, err := adapter.request(actor, http.MethodPost,
			adapter.runner.resourceURL(target.Ref, target.Name, "import", nil), headers, body)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusCreated && response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		resource, err := decodeResource(response, response.Status)
		if err != nil {
			return genericObservation{}, err
		}
		code := "ok"
		if response.Status == http.StatusCreated {
			code = "created"
		}
		return genericObservation{
			Code: code, ETag: response.Header.Get("ETag"),
			Resource: adapter.normalizeResource(request.Resource.Handle, resource),
		}, nil
	case genericResourceApply:
		return adapter.apply(actor, request.Resource, controls)
	default:
		return genericObservation{}, fmt.Errorf("HTTP adapter unknown resource action %q", request.Action)
	}
}

func (adapter *genericHTTPPlanAdapter) apply(
	actor genericActor,
	input genericResourceInput,
	controls genericControls,
) (genericObservation, error) {
	target := adapter.target(input)
	expectedUID := adapter.symbolToRawUID[input.ExpectedUID]
	if input.ExpectedUID != "" && expectedUID == "" {
		expectedUID = "uid-that-does-not-name-the-live-resource"
	}
	options := applyOptions{
		Create: input.Create, ExpectedGeneration: input.ExpectedGeneration,
		ExpectedUID: expectedUID, IdempotencyKey: input.IdempotencyKey,
		PrepareDigest:             adapter.preparations[adapter.actorScope(actor)+"\x00"+input.PreparationHandle],
		BodyGeneration:            input.BodyGeneration,
		DisagreeingBodyGeneration: input.DisagreeingBodyGeneration,
		OmitPrecondition:          input.OmitGeneration,
		CreateWithGenerationFence: input.CreateGenerationFence,
		SpecDigestEcho:            input.ReviewSpecHash,
	}
	if controls.Completion == genericCompletionAsync {
		options.ExtraHeaders = map[string]string{ErrorProbeHeader: ProbeAsync}
	}
	targetURL, headers, body, err := adapter.runner.applyRequestParts(target, options)
	if err != nil {
		return genericObservation{}, err
	}
	response, err := adapter.request(actor, http.MethodPut, targetURL, headers, body)
	if err != nil {
		return genericObservation{}, err
	}
	if response.Status == http.StatusAccepted {
		var envelope struct {
			Operation wireOperation `json:"operation"`
		}
		if err := decodeStrictResponse(response, &envelope); err != nil {
			return genericObservation{}, err
		}
		handle := input.Handle + "-operation"
		adapter.operations[handle] = envelope.Operation.ID
		return genericObservation{Code: "accepted", OperationHandle: handle, OperationDone: genericBool(false)}, nil
	}
	if response.Status != http.StatusCreated && response.Status != http.StatusOK {
		return adapter.errorObservation(response)
	}
	resource, err := decodeResource(response, response.Status)
	if err != nil {
		return genericObservation{}, err
	}
	code := "ok"
	if response.Status == http.StatusCreated {
		code = "created"
	}
	return genericObservation{
		Code: code, ETag: response.Header.Get("ETag"),
		Resource: adapter.normalizeResource(input.Handle, resource),
	}, nil
}

func (adapter *genericHTTPPlanAdapter) operation(
	actor genericActor,
	request genericOperationRequest,
) (genericObservation, error) {
	actual := adapter.operations[request.Handle]
	if actual == "" {
		actual = "op_no_such_operation_here"
	}
	operationURL := adapter.runner.apiBase + "/operations/" + url.PathEscape(actual)
	if request.Action == genericOperationCancel {
		response, err := adapter.request(actor, http.MethodPost, operationURL+"/cancel",
			map[string]string{"Idempotency-Key": "key-plan-cancel-" + request.Handle}, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		var operation wireOperation
		if err := decodeStrictResponse(response, &operation); err != nil {
			return genericObservation{}, err
		}
		cancelled := operation.Error != nil && operation.Error["code"] == "operation_cancelled"
		return genericObservation{
			Code: "cancelled", OperationHandle: request.Handle,
			OperationDone: genericBool(operation.Done), OperationCancelled: cancelled,
		}, nil
	}
	if request.Action != genericOperationGet {
		return genericObservation{}, fmt.Errorf("HTTP adapter unknown operation action %q", request.Action)
	}
	for poll := 0; poll < asyncOperationPolls+3; poll++ {
		response, err := adapter.request(actor, http.MethodGet, operationURL, nil, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		var operation wireOperation
		if err := decodeStrictResponse(response, &operation); err != nil {
			return genericObservation{}, err
		}
		if !operation.Done {
			continue
		}
		if operation.Error != nil {
			code, _ := operation.Error["code"].(string)
			if code == "operation_cancelled" {
				return genericObservation{
					Code: "cancelled", OperationHandle: request.Handle,
					OperationDone: genericBool(true), OperationCancelled: true,
				}, nil
			}
			return genericObservation{Code: code, OperationHandle: request.Handle, OperationDone: genericBool(true)}, nil
		}
		raw, err := encodeRunnerJSON(operation.Result["resource"])
		if err != nil {
			return genericObservation{}, err
		}
		var resource wireResource
		if err := formpackage.DecodeStrictIJSON(raw, &resource); err != nil {
			return genericObservation{}, err
		}
		handle := strings.TrimSuffix(request.Handle, "-operation")
		return genericObservation{
			Code: "completed", OperationHandle: request.Handle,
			OperationDone: genericBool(true), Resource: adapter.normalizeResource(handle, resource),
		}, nil
	}
	return genericObservation{}, fmt.Errorf("HTTP operation %s did not settle", request.Handle)
}

func (adapter *genericHTTPPlanAdapter) artifact(
	actor genericActor,
	request genericArtifactRequest,
	controls genericControls,
) (genericObservation, error) {
	switch request.Action {
	case genericArtifactBegin:
		transport := stableGenericArtifactTransport{
			BlobSource: string(request.Blob), DeclaredSize: request.DeclaredSize, ContentType: request.ContentType,
		}
		if controls.Fault == genericFaultWrongDeclaredSize {
			transport.DeclaredSize = len(request.Blob)
		}
		mapped, err := stableGenericHTTPArtifact(transport)
		if err != nil {
			return genericObservation{}, err
		}
		if controls.Fault == genericFaultWrongDeclaredSize {
			files, _ := mapped.Manifest["files"].([]any)
			file, _ := files[0].(map[string]any)
			file["size"] = request.DeclaredSize
		}
		body, err := encodeRunnerJSON(map[string]any{"manifest": mapped.Manifest})
		if err != nil {
			return genericObservation{}, err
		}
		response, err := adapter.request(actor, http.MethodPost, adapter.runner.apiBase+"/artifacts/uploads",
			map[string]string{"Idempotency-Key": request.IdempotencyKey}, body)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK && response.Status != http.StatusCreated {
			return adapter.errorObservation(response)
		}
		var status struct {
			UploadID     string   `json:"uploadId"`
			MissingBlobs []string `json:"missingBlobs"`
		}
		if err := decodeStrictResponse(response, &status); err != nil {
			return genericObservation{}, err
		}
		adapter.uploads[request.UploadHandle] = genericHTTPUpload{
			actualID: status.UploadID, manifest: mapped.Manifest,
			blobDigest: request.BlobDigest, manifestHandle: request.ManifestHandle,
		}
		return genericObservation{Code: "ok", UploadHandle: request.UploadHandle, MissingBlobs: status.MissingBlobs}, nil
	case genericArtifactPut:
		upload := adapter.uploads[request.UploadHandle]
		actual := upload.actualID
		if actual == "" {
			actual = "up_no_such_upload_here"
		}
		blob := request.Blob
		if controls.Fault == genericFaultWrongBlobBytes {
			blob = []byte("deliberately-wrong-blob")
		}
		response, err := adapter.request(actor, http.MethodPut,
			adapter.runner.apiBase+"/artifacts/uploads/"+url.PathEscape(actual)+"/blobs/"+url.PathEscape(request.BlobDigest),
			map[string]string{"Content-Type": request.ContentType}, blob)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusCreated && response.Status != http.StatusNoContent {
			return adapter.errorObservation(response)
		}
		return genericObservation{Code: "uploaded", UploadHandle: request.UploadHandle}, nil
	case genericArtifactCommit:
		upload := adapter.uploads[request.UploadHandle]
		actual := upload.actualID
		if actual == "" {
			actual = "up_no_such_upload_here"
		}
		response, err := adapter.request(actor, http.MethodPost,
			adapter.runner.apiBase+"/artifacts/uploads/"+url.PathEscape(actual)+"/commit",
			map[string]string{"Idempotency-Key": request.IdempotencyKey}, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK && response.Status != http.StatusCreated {
			return adapter.errorObservation(response)
		}
		var committed struct {
			ManifestDigest string `json:"manifestDigest"`
		}
		if err := decodeStrictResponse(response, &committed); err != nil {
			return genericObservation{}, err
		}
		adapter.manifests[upload.manifestHandle] = committed.ManifestDigest
		return genericObservation{
			Code: "committed", UploadHandle: request.UploadHandle,
			ManifestHandle: upload.manifestHandle,
		}, nil
	case genericArtifactGetManifest:
		actual := adapter.manifests[request.ManifestHandle]
		if actual == "" {
			actual = formpackage.DigestBytes([]byte("absent-generic-manifest"))
		}
		response, err := adapter.request(actor, http.MethodGet,
			adapter.runner.apiBase+"/artifacts/"+url.PathEscape(actual), nil, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			return adapter.errorObservation(response)
		}
		return genericObservation{Code: "ok", ManifestHandle: request.ManifestHandle}, nil
	case genericArtifactHeadBlob:
		response, err := adapter.request(actor, http.MethodHead,
			adapter.runner.apiBase+"/artifacts/blobs/"+url.PathEscape(request.BlobDigest), nil, nil)
		if err != nil {
			return genericObservation{}, err
		}
		if response.Status != http.StatusOK {
			observed, err := adapter.errorObservation(response)
			observed.BlobPresent = genericBool(false)
			return observed, err
		}
		return genericObservation{Code: "ok", BlobPresent: genericBool(true)}, nil
	default:
		return genericObservation{}, fmt.Errorf("HTTP adapter unknown artifact action %q", request.Action)
	}
}

func (adapter *genericHTTPPlanAdapter) target(input genericResourceInput) probeTarget {
	return probeTarget{
		Ref: input.Ref, PackageDigest: input.PackageDigest,
		Name: input.Name, Space: input.Space, Spec: cloneJSONMap(input.Desired),
	}
}

func (adapter *genericHTTPPlanAdapter) normalizeResource(handle string, resource wireResource) *genericObservedResource {
	symbol := adapter.rawUIDToSymbol[resource.Metadata.UID]
	if symbol == "" {
		adapter.incarnations[handle]++
		symbol = fmt.Sprintf("%s#%d", handle, adapter.incarnations[handle])
		adapter.rawUIDToSymbol[resource.Metadata.UID] = symbol
		adapter.symbolToRawUID[symbol] = resource.Metadata.UID
	}
	var outputs map[string]any
	conditions := []string{}
	if resource.Status != nil {
		outputs = cloneJSONMap(resource.Status.Outputs)
		for _, condition := range resource.Status.Conditions {
			conditions = append(conditions, condition.Type+"="+condition.Status+":"+condition.Reason)
		}
		sort.Strings(conditions)
	}
	return &genericObservedResource{
		Handle: handle, UID: symbol, Generation: resource.Metadata.Generation,
		Revision: resource.Metadata.Revision, Desired: cloneJSONMap(resource.Spec), Outputs: outputs,
		Conditions: conditions,
	}
}

func (adapter *genericHTTPPlanAdapter) errorObservation(response wireResponse) (genericObservation, error) {
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"requestId"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil || envelope.Error.Code == "" {
		return genericObservation{}, fmt.Errorf("HTTP %d did not carry a normalized error: %s", response.Status, strings.TrimSpace(string(response.Body)))
	}
	return genericObservation{
		Code: envelope.Error.Code, HTTPStatus: response.Status,
		Retryable:        genericBool(envelope.Error.Retryable),
		RequestIDPresent: envelope.Error.RequestID != "",
	}, nil
}

func (adapter *genericHTTPPlanAdapter) actorScope(actor genericActor) string {
	return string(actor.Credential)
}
