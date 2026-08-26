package portableconformancev3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// This file is the independent executable model required by W06. It imports
// no HTTP package and does not call ReferenceHost or v3Runner. All decisions
// come from the immutable seed and the state owned below.

type genericMemoryActor struct {
	tenant    string
	principal string
}

func (actor genericMemoryActor) scope() string { return actor.tenant + "\x00" + actor.principal }

type genericMemoryForm struct {
	ref           FormRef
	packageDigest string
	definition    formpackage.FormDefinition
}

type genericMemoryResource struct {
	address      genericResourceInput
	uid          string
	generation   int64
	revision     int64
	desired      map[string]any
	relationPins []genericMemoryRelationPin
	nativeID     string
	outputs      map[string]any
}

type genericMemoryPreparation struct {
	actor      genericMemoryActor
	resource   genericResourceInput
	desired    map[string]any
	generation string
}

type genericMemoryReplay struct {
	resourceKey string
	fingerprint string
	observed    genericObservation
}

type genericMemoryOperation struct {
	owner     genericMemoryActor
	request   genericResourceRequest
	controls  genericControls
	done      bool
	cancelled bool
	result    genericObservation
}

type genericMemoryUpload struct {
	owner          genericMemoryActor
	blobDigest     string
	declaredSize   int
	contentType    string
	blob           []byte
	committed      bool
	manifestHandle string
}

type genericMemoryFaults struct {
	keepGenerationOnUpdate bool
}

type genericMemoryAdapter struct {
	seed         genericPlanSeed
	forms        map[string]genericMemoryForm
	resources    map[string]*genericMemoryResource
	nativeClaims map[string]string
	replays      map[string]genericMemoryReplay
	preparations map[string]genericMemoryPreparation
	operations   map[string]*genericMemoryOperation
	uploads      map[string]*genericMemoryUpload
	blobs        map[string]map[string][]byte
	manifests    map[string]map[string]string
	incarnations map[string]int
	faults       genericMemoryFaults
}

const genericMemorySupportAPIVersionPrefix = "support.takoform.com/"

var (
	genericPlanResourceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	genericPlanSpacePattern        = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
)

func newGenericMemoryAdapter(seed genericPlanSeed) (*genericMemoryAdapter, error) {
	if seed.Snapshot == nil {
		return nil, errors.New("memory adapter seed has no Snapshot")
	}
	adapter := &genericMemoryAdapter{
		seed: seed, forms: map[string]genericMemoryForm{}, resources: map[string]*genericMemoryResource{},
		nativeClaims: map[string]string{}, replays: map[string]genericMemoryReplay{},
		preparations: map[string]genericMemoryPreparation{}, operations: map[string]*genericMemoryOperation{},
		uploads: map[string]*genericMemoryUpload{}, blobs: map[string]map[string][]byte{},
		manifests: map[string]map[string]string{}, incarnations: map[string]int{},
	}
	for _, compiled := range seed.Snapshot.Forms() {
		raw, ok := seed.Snapshot.Definition(compiled.Ref)
		if !ok {
			return nil, fmt.Errorf("memory adapter Snapshot omitted Definition %+v", compiled.Ref)
		}
		definition, err := formpackage.ValidateDefinition(raw)
		if err != nil {
			return nil, err
		}
		ref := portableFormRef(compiled.Ref)
		adapter.forms[stableFormRefKey(ref)] = genericMemoryForm{
			ref: ref, packageDigest: compiled.PackageDigest, definition: definition,
		}
	}
	return adapter, nil
}

func genericMemoryFactory(seed genericPlanSeed) (genericPlanAdapter, func(), error) {
	adapter, err := newGenericMemoryAdapter(seed)
	return adapter, nil, err
}

func (adapter *genericMemoryAdapter) authenticate(actor genericActor) (genericMemoryActor, bool) {
	switch actor.Credential {
	case genericCredentialPrimary:
		return genericMemoryActor{tenant: "reference-tenant", principal: "reference-primary"}, true
	case genericCredentialAlternate:
		return genericMemoryActor{tenant: "reference-tenant", principal: "reference-alternate"}, true
	case genericCredentialOtherTenant:
		return genericMemoryActor{tenant: "reference-other-tenant", principal: "reference-primary"}, true
	default:
		return genericMemoryActor{}, false
	}
}

func (adapter *genericMemoryAdapter) Call(
	_ context.Context,
	actor genericActor,
	command genericCommand,
) (genericObservation, error) {
	if err := command.validate(); err != nil {
		return genericObservation{}, err
	}
	auth, authenticated := adapter.authenticate(actor)
	if !authenticated {
		return adapter.decorateError(genericObservation{Code: "unauthenticated"}), nil
	}
	if code, probe := genericFaultErrorCode(command.Controls.Fault); probe {
		if _, known := adapter.seed.Contract.lane.ErrorHTTPStatus[code]; !known {
			code = "invalid_argument"
		}
		return adapter.decorateError(genericObservation{Code: code}), nil
	}
	var observed genericObservation
	var err error
	switch {
	case command.Catalog != nil:
		observed, err = adapter.catalog(auth, *command.Catalog)
	case command.Admission != nil:
		observed, err = adapter.admission(auth, *command.Admission)
	case command.Resource != nil:
		observed, err = adapter.resource(auth, *command.Resource, command.Controls)
	case command.Operation != nil:
		observed, err = adapter.operation(auth, *command.Operation)
	case command.Artifact != nil:
		observed, err = adapter.artifact(auth, *command.Artifact, command.Controls)
	default:
		return genericObservation{}, errors.New("unreachable generic memory command")
	}
	if err != nil {
		return genericObservation{}, err
	}
	return adapter.decorateError(observed), nil
}

func (adapter *genericMemoryAdapter) decorateError(observed genericObservation) genericObservation {
	if observed.OperationHandle != "" {
		// A terminal operation failure is embedded in a successful operation
		// record; it is not an HTTP error envelope for this call.
		return observed
	}
	status, isError := adapter.seed.Contract.lane.ErrorHTTPStatus[observed.Code]
	if !isError {
		return observed
	}
	observed.HTTPStatus = status
	observed.Retryable = genericBool(adapter.seed.Contract.lane.isAutomaticallyRetryable(observed.Code))
	observed.RequestIDPresent = true
	return observed
}

func (adapter *genericMemoryAdapter) catalog(
	_ genericMemoryActor,
	request genericCatalogRequest,
) (genericObservation, error) {
	switch request.Action {
	case genericCatalogDiscover:
		return genericObservation{
			Code: "ok", APIVersions: []string{adapter.seed.Contract.APIVersion},
			EndpointPaths: map[string]string{"api": adapter.seed.Contract.APIPath},
			Features: []string{
				"artifact_upload", "exact_form_ref", "idempotent_lifecycle", "operations",
				"optimistic_concurrency", "service_forms", "support_profiles",
			}}, nil
	case genericCatalogList:
		if !genericPlanSpacePattern.MatchString(request.Space) {
			return genericObservation{Code: "invalid_argument"}, nil
		}
		forms := make([]genericObservedForm, 0)
		for _, form := range adapter.forms {
			if request.Ref != (FormRef{}) && form.ref != request.Ref {
				continue
			}
			forms = append(forms, genericObservedForm{
				Ref: form.ref, PackageDigest: form.packageDigest,
				DefinitionKnown: true, Installed: true, Executable: true, Activated: true,
				AvailableToPrincipal: true,
				Operations:           genericSortedOperations(form.definition.LifecycleCapabilities),
			})
		}
		sort.Slice(forms, func(i, j int) bool { return stableFormRefKey(forms[i].Ref) < stableFormRefKey(forms[j].Ref) })
		return genericObservation{Code: "ok", Forms: forms}, nil
	case genericCatalogGet:
		if request.Surface == genericCatalogSurfaceService {
			satisfiable := request.Protocol == adapter.seed.Probe.ExternalServices.SupportedProtocol
			return genericObservation{Code: "ok", Support: &genericObservedSupport{
				APIVersion: genericMemorySupportAPIVersionPrefix + adapter.seed.Contract.lane.SupportProfileSchemaVersion,
				Kind:       "StandardServiceSupport", Protocol: request.Protocol,
				Satisfiable: genericBool(satisfiable), ExtraKeys: []string{},
			}}, nil
		}
		form, ok := adapter.forms[stableFormRefKey(request.Ref)]
		if !ok {
			return genericObservation{Code: "form_unknown"}, nil
		}
		if request.Surface == genericCatalogSurfaceSupport {
			return genericObservation{Code: "ok", Support: &genericObservedSupport{
				APIVersion: genericMemorySupportAPIVersionPrefix + adapter.seed.Contract.lane.SupportProfileSchemaVersion,
				Kind:       "FormSupport", Ref: form.ref,
				Operations: genericSortedOperations(form.definition.LifecycleCapabilities),
				ExtraKeys:  []string{},
			}}, nil
		}
		if request.Surface != "" && request.Surface != genericCatalogSurfaceDefinition {
			return genericObservation{}, fmt.Errorf("memory adapter unknown catalog surface %q", request.Surface)
		}
		desiredSchemaHash, err := genericCanonicalHash(form.definition.DesiredSchema)
		if err != nil {
			return genericObservation{}, err
		}
		constraintsHash, err := genericCanonicalHash(form.definition.Constraints)
		if err != nil {
			return genericObservation{}, err
		}
		return genericObservation{
			Code: "ok", DefinitionDigest: form.ref.SchemaDigest,
			AddressPath: adapter.seed.Contract.APIPath + "/form-definitions/" +
				strings.Join(strings.Split(form.ref.APIVersion, "/"), "/") + "/" + form.ref.Kind,
			Definition: &genericObservedDefinition{
				Ref: form.ref, PackageDigest: form.packageDigest,
				Title: form.definition.Title, Description: form.definition.Description,
				DesiredSchemaHash: desiredSchemaHash, ConstraintsHash: constraintsHash,
			},
			Forms: []genericObservedForm{{
				Ref: form.ref, PackageDigest: form.packageDigest,
				Operations: genericSortedOperations(form.definition.LifecycleCapabilities),
			}},
		}, nil
	default:
		return genericObservation{}, fmt.Errorf("memory adapter unknown catalog action %q", request.Action)
	}
}

func (adapter *genericMemoryAdapter) admission(
	auth genericMemoryActor,
	request genericAdmissionRequest,
) (genericObservation, error) {
	if !genericPlanResourceNamePattern.MatchString(request.Resource.Name) ||
		!genericPlanSpacePattern.MatchString(request.Resource.Space) {
		return genericObservation{Code: "invalid_argument"}, nil
	}
	materialized, code, err := adapter.materializeAndValidate(request.Resource)
	if err != nil || code != "ok" {
		if err != nil {
			return genericObservation{}, err
		}
		if request.Action == genericAdmissionValidate && code == "invalid_argument" {
			return genericObservation{Code: "ok", Valid: genericBool(false), HasDiagnostics: genericBool(true)}, nil
		}
		return genericObservation{Code: code}, nil
	}
	switch request.Action {
	case genericAdmissionValidate:
		key := adapter.resourceKey(auth.tenant, request.Resource)
		if code := genericValidateMemoryAdmissionConstraints(adapter, auth, key, request.Resource, materialized, adapter.resources[key]); code != "ok" {
			return genericObservation{Code: "ok", Valid: genericBool(false), HasDiagnostics: genericBool(true)}, nil
		}
		return genericObservation{
			Code: "ok", Valid: genericBool(true), HasDiagnostics: genericBool(false),
			EffectiveDesired: materialized,
		}, nil
	case genericAdmissionPrepare:
		if request.PreparationHandle == "" {
			return genericObservation{}, errors.New("memory adapter prepare has no symbolic handle")
		}
		resourceKey := adapter.resourceKey(auth.tenant, request.Resource)
		stored := adapter.resources[resourceKey]
		switch {
		case request.ExpectedGeneration == "" && stored != nil:
			return genericObservation{Code: "invalid_argument"}, nil
		case request.ExpectedGeneration != "" && stored == nil:
			return genericObservation{Code: "resource_not_found"}, nil
		case stored != nil && request.ExpectedGeneration != stored.generationString():
			return genericObservation{Code: "generation_conflict"}, nil
		}
		if code := genericValidateMemoryAdmissionConstraints(adapter, auth, resourceKey, request.Resource, materialized, stored); code != "ok" {
			return genericObservation{Code: code}, nil
		}
		key := auth.scope() + "\x00" + request.PreparationHandle
		adapter.preparations[key] = genericMemoryPreparation{
			actor: auth, resource: request.Resource, desired: genericCloneJSONMap(materialized),
			generation: request.ExpectedGeneration,
		}
		specHash, err := genericSpecCanonicalDigest(materialized)
		if err != nil {
			return genericObservation{}, err
		}
		return genericObservation{
			Code: "ok", PreparationHandle: request.PreparationHandle,
			PreparationSpecHash: specHash,
			EffectiveDesired:    genericCloneJSONMap(materialized),
		}, nil
	default:
		return genericObservation{}, fmt.Errorf("memory adapter unknown admission action %q", request.Action)
	}
}

func (adapter *genericMemoryAdapter) materializeAndValidate(input genericResourceInput) (map[string]any, string, error) {
	if !genericPlanResourceNamePattern.MatchString(input.Name) || !genericPlanSpacePattern.MatchString(input.Space) {
		return nil, "invalid_argument", nil
	}
	_, ok := adapter.forms[stableFormRefKey(input.Ref)]
	if !ok {
		return nil, "form_unknown", nil
	}
	if input.PackageDigest != "" && !formpackage.ValidDigest(input.PackageDigest) {
		return nil, "invalid_argument", nil
	}
	raw, err := json.Marshal(input.Desired)
	if err != nil {
		return nil, "", err
	}
	materializedRaw, err := adapter.seed.Snapshot.Materialize(formpackageFormRef(input.Ref), raw)
	if err != nil {
		return nil, "invalid_argument", nil
	}
	var materialized map[string]any
	if err := formpackage.DecodeStrictIJSON(materializedRaw, &materialized); err != nil {
		return nil, "", err
	}
	if code := adapter.genericMemoryStandardServices(materialized); code != "ok" {
		return nil, code, nil
	}
	return materialized, "ok", nil
}

func (adapter *genericMemoryAdapter) genericMemoryStandardServices(desired map[string]any) string {
	property := adapter.seed.Probe.ExternalServices.Property
	entries, _ := desired[property].([]any)
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		service, _ := entry["service"].(map[string]any)
		protocol, _ := service["protocol"].(string)
		required, present := entry["required"].(bool)
		if !present {
			required = true
		}
		if protocol != "" && protocol != adapter.seed.Probe.ExternalServices.SupportedProtocol && required {
			return "unsupported_capability"
		}
	}
	return "ok"
}

func (adapter *genericMemoryAdapter) resource(
	auth genericMemoryActor,
	request genericResourceRequest,
	controls genericControls,
) (genericObservation, error) {
	input := request.Resource
	key := adapter.resourceKey(auth.tenant, input)
	switch request.Action {
	case genericResourceRead:
		stored := adapter.resources[key]
		if stored == nil || stored.address.Ref != input.Ref {
			return genericObservation{Code: "resource_not_found"}, nil
		}
		return genericObservation{Code: "ok", ETag: genericMemoryETag(stored), Resource: adapter.observeResource(stored)}, nil
	case genericResourceObserve:
		stored := adapter.resources[key]
		if stored == nil || stored.address.Ref != input.Ref {
			return genericObservation{Code: "resource_not_found"}, nil
		}
		if input.ExpectedGeneration == "" {
			return genericObservation{Code: "invalid_argument"}, nil
		}
		if input.ExpectedGeneration != stored.generationString() {
			return genericObservation{Code: "generation_conflict"}, nil
		}
		if controls.BackendEffect == genericBackendEffectTouchStatus {
			stored.revision++
		}
		return genericObservation{Code: "ok", ETag: genericMemoryETag(stored), Resource: adapter.observeResource(stored)}, nil
	case genericResourceDelete:
		stored := adapter.resources[key]
		if stored == nil || stored.address.Ref != input.Ref {
			return genericObservation{Code: "resource_not_found"}, nil
		}
		if input.ExpectedGeneration == "" {
			return genericObservation{Code: "invalid_argument"}, nil
		}
		if input.ExpectedGeneration != stored.generationString() {
			return genericObservation{Code: "generation_conflict"}, nil
		}
		if input.ExpectedRevision != "" && input.ExpectedRevision != stored.revisionString() {
			return genericObservation{Code: "revision_conflict"}, nil
		}
		adapter.releaseResource(key, stored)
		return genericObservation{Code: "deleted"}, nil
	case genericResourceImport:
		return adapter.importResource(auth, request)
	case genericResourceApply:
		return adapter.applyResource(auth, request, controls)
	default:
		return genericObservation{}, fmt.Errorf("memory adapter unknown resource action %q", request.Action)
	}
}

func (adapter *genericMemoryAdapter) applyResource(
	auth genericMemoryActor,
	request genericResourceRequest,
	controls genericControls,
) (genericObservation, error) {
	input := request.Resource
	replayKey := auth.scope() + "\x00" + input.IdempotencyKey
	fingerprint, err := genericMemoryFingerprint(request)
	if err != nil {
		return genericObservation{}, err
	}
	if replay, ok := adapter.replays[replayKey]; ok {
		if replay.fingerprint != fingerprint {
			return genericObservation{Code: "invalid_argument"}, nil
		}
		return cloneGenericObservation(replay.observed), nil
	}
	materialized, code, err := adapter.materializeAndValidate(input)
	if err != nil || code != "ok" {
		if err != nil {
			return genericObservation{}, err
		}
		return genericObservation{Code: code}, nil
	}
	key := adapter.resourceKey(auth.tenant, input)
	stored := adapter.resources[key]
	effectiveGeneration := input.ExpectedGeneration
	if input.Create {
		if input.CreateGenerationFence != "" || input.ExpectedGeneration != "" || input.BodyGeneration != "" {
			return genericObservation{Code: "invalid_argument"}, nil
		}
	} else {
		if input.OmitGeneration || (input.ExpectedGeneration == "" && input.BodyGeneration == "") {
			return genericObservation{Code: "invalid_argument"}, nil
		}
		if input.BodyGeneration != "" {
			effectiveGeneration = input.BodyGeneration
		}
		if input.DisagreeingBodyGeneration != "" && input.DisagreeingBodyGeneration != input.ExpectedGeneration {
			return genericObservation{Code: "invalid_argument"}, nil
		}
	}
	preparation, ok := adapter.preparations[auth.scope()+"\x00"+input.PreparationHandle]
	if !ok || preparation.generation != effectiveGeneration ||
		!sameGenericResourceIntent(preparation.resource, input) {
		return genericObservation{Code: "invalid_argument"}, nil
	}
	if !reflect.DeepEqual(materialized, preparation.desired) {
		return genericObservation{Code: "invalid_argument"}, nil
	}
	if input.ReviewSpecHash != "" {
		preparedHash, err := genericSpecCanonicalDigest(preparation.desired)
		if err != nil {
			return genericObservation{}, err
		}
		if input.ReviewSpecHash != preparedHash {
			return genericObservation{Code: "invalid_argument"}, nil
		}
	}
	if input.Create {
		if stored != nil {
			return genericObservation{Code: "generation_conflict"}, nil
		}
	} else {
		if stored == nil || stored.address.Ref != input.Ref {
			return genericObservation{Code: "resource_not_found"}, nil
		}
		if effectiveGeneration != stored.generationString() {
			return genericObservation{Code: "generation_conflict"}, nil
		}
		if input.ExpectedUID != "" && input.ExpectedUID != stored.uid {
			return genericObservation{Code: "uid_mismatch"}, nil
		}
	}
	if code := genericValidateMemoryConstraints(adapter, auth, key, input, materialized, stored); code != "ok" {
		return genericObservation{Code: code}, nil
	}
	pins, code := genericMemoryCaptureRelationPins(adapter, auth, key, input, materialized, stored)
	if code != "ok" {
		return genericObservation{Code: code}, nil
	}
	if controls.Completion == genericCompletionAsync {
		handle := input.Handle + "-operation"
		adapter.operations[handle] = &genericMemoryOperation{
			owner: auth, request: request, controls: genericControls{Completion: genericCompletionImmediate},
		}
		return genericObservation{Code: "accepted", OperationHandle: handle, OperationDone: genericBool(false)}, nil
	}

	form := adapter.forms[stableFormRefKey(input.Ref)]
	if stored == nil {
		adapter.incarnations[input.Handle]++
		stored = &genericMemoryResource{
			address: input, uid: fmt.Sprintf("%s#%d", input.Handle, adapter.incarnations[input.Handle]),
			generation: 1, revision: 1, desired: genericCloneJSONMap(materialized),
			relationPins: append([]genericMemoryRelationPin(nil), pins...),
		}
		if input.Ref == adapter.seed.Probe.Resources.Output.FormRef {
			stored.outputs = genericCloneJSONMap(adapter.seed.Probe.Resources.Output.HostAssignedOutputs)
		}
		adapter.resources[key] = stored
	} else {
		changed := !reflect.DeepEqual(stored.desired, materialized)
		if changed && !genericContainsString(form.definition.LifecycleCapabilities, "update") {
			return genericObservation{Code: "invalid_argument"}, nil
		}
		if changed {
			if adapter.faults.keepGenerationOnUpdate {
				stored.revision++
			} else {
				stored.generation++
				stored.revision++
			}
			stored.desired = genericCloneJSONMap(materialized)
		}
		stored.relationPins = append([]genericMemoryRelationPin(nil), pins...)
	}
	observed := genericObservation{
		Code: map[bool]string{true: "created", false: "ok"}[input.Create],
		ETag: genericMemoryETag(stored), Resource: adapter.observeResource(stored),
	}
	adapter.replays[replayKey] = genericMemoryReplay{resourceKey: key, fingerprint: fingerprint, observed: cloneGenericObservation(observed)}
	return observed, nil
}

func (adapter *genericMemoryAdapter) importResource(
	auth genericMemoryActor,
	request genericResourceRequest,
) (genericObservation, error) {
	input := request.Resource
	materialized, code, err := adapter.materializeAndValidate(input)
	if err != nil || code != "ok" {
		if err != nil {
			return genericObservation{}, err
		}
		return genericObservation{Code: code}, nil
	}
	key := adapter.resourceKey(auth.tenant, input)
	claimKey := auth.tenant + "\x00" + input.NativeID
	if holder := adapter.nativeClaims[claimKey]; holder != "" && holder != key {
		return genericObservation{Code: "import_conflict"}, nil
	}
	stored := adapter.resources[key]
	if code := genericValidateMemoryConstraints(adapter, auth, key, input, materialized, stored); code != "ok" {
		return genericObservation{Code: code}, nil
	}
	pins, code := genericMemoryCaptureRelationPins(adapter, auth, key, input, materialized, stored)
	if code != "ok" {
		return genericObservation{Code: code}, nil
	}
	if input.Create {
		if stored != nil {
			return genericObservation{Code: "generation_conflict"}, nil
		}
		adapter.incarnations[input.Handle]++
		stored = &genericMemoryResource{
			address: input, uid: fmt.Sprintf("%s#%d", input.Handle, adapter.incarnations[input.Handle]),
			generation: 1, revision: 1, desired: genericCloneJSONMap(materialized), nativeID: input.NativeID,
			relationPins: append([]genericMemoryRelationPin(nil), pins...),
		}
		adapter.resources[key] = stored
	} else {
		if stored == nil || stored.address.Ref != input.Ref {
			return genericObservation{Code: "resource_not_found"}, nil
		}
		if input.ExpectedGeneration != stored.generationString() {
			return genericObservation{Code: "generation_conflict"}, nil
		}
		if stored.nativeID != "" && stored.nativeID != input.NativeID {
			return genericObservation{Code: "import_conflict"}, nil
		}
		stored.nativeID = input.NativeID
		stored.relationPins = append([]genericMemoryRelationPin(nil), pins...)
	}
	adapter.nativeClaims[claimKey] = key
	return genericObservation{
		Code: map[bool]string{true: "created", false: "ok"}[input.Create],
		ETag: genericMemoryETag(stored), Resource: adapter.observeResource(stored),
	}, nil
}

func (adapter *genericMemoryAdapter) operation(
	auth genericMemoryActor,
	request genericOperationRequest,
) (genericObservation, error) {
	operation := adapter.operations[request.Handle]
	if operation == nil || operation.owner != auth {
		return genericObservation{Code: "operation_not_found"}, nil
	}
	switch request.Action {
	case genericOperationCancel:
		if !operation.done {
			operation.done = true
			operation.cancelled = true
		}
		return genericObservation{
			Code: "cancelled", OperationHandle: request.Handle,
			OperationDone: genericBool(true), OperationCancelled: true,
		}, nil
	case genericOperationGet:
		if operation.cancelled {
			return genericObservation{
				Code: "cancelled", OperationHandle: request.Handle,
				OperationDone: genericBool(true), OperationCancelled: true,
			}, nil
		}
		if !operation.done {
			result, err := adapter.applyResource(operation.owner, operation.request, operation.controls)
			if err != nil {
				return genericObservation{}, err
			}
			operation.done = true
			operation.result = result
		}
		if operation.result.Code != "created" && operation.result.Code != "ok" {
			return genericObservation{
				Code: operation.result.Code, OperationHandle: request.Handle,
				OperationDone: genericBool(true),
			}, nil
		}
		return genericObservation{
			Code: "completed", OperationHandle: request.Handle,
			OperationDone: genericBool(true), Resource: operation.result.Resource,
		}, nil
	default:
		return genericObservation{}, fmt.Errorf("memory adapter unknown operation action %q", request.Action)
	}
}

func (adapter *genericMemoryAdapter) artifact(
	auth genericMemoryActor,
	request genericArtifactRequest,
	controls genericControls,
) (genericObservation, error) {
	switch request.Action {
	case genericArtifactBegin:
		if request.UploadHandle == "" || !formpackage.ValidDigest(request.BlobDigest) || request.DeclaredSize < 0 || request.ContentType == "" {
			return genericObservation{Code: "artifact_invalid"}, nil
		}
		missing := []string{}
		if adapter.blobs[auth.tenant] == nil || adapter.blobs[auth.tenant][request.BlobDigest] == nil {
			missing = append(missing, request.BlobDigest)
		}
		adapter.uploads[request.UploadHandle] = &genericMemoryUpload{
			owner: auth, blobDigest: request.BlobDigest, declaredSize: request.DeclaredSize,
			contentType: request.ContentType, manifestHandle: request.ManifestHandle,
		}
		return genericObservation{Code: "ok", UploadHandle: request.UploadHandle, MissingBlobs: missing}, nil
	case genericArtifactPut:
		upload := adapter.uploads[request.UploadHandle]
		if upload == nil || upload.owner != auth {
			return genericObservation{Code: "artifact_missing"}, nil
		}
		blob := append([]byte(nil), request.Blob...)
		if controls.Fault == genericFaultWrongBlobBytes {
			blob = []byte("deliberately-wrong-blob")
		}
		if formpackage.DigestBytes(blob) != upload.blobDigest {
			return genericObservation{Code: "artifact_invalid"}, nil
		}
		upload.blob = blob
		return genericObservation{Code: "uploaded", UploadHandle: request.UploadHandle}, nil
	case genericArtifactCommit:
		upload := adapter.uploads[request.UploadHandle]
		if upload == nil || upload.owner != auth {
			return genericObservation{Code: "artifact_missing"}, nil
		}
		if upload.committed {
			return genericObservation{Code: "committed", UploadHandle: request.UploadHandle, ManifestHandle: upload.manifestHandle}, nil
		}
		blob := upload.blob
		if len(blob) == 0 {
			blob = adapter.blobs[auth.tenant][upload.blobDigest]
		}
		declaredSize := upload.declaredSize
		if controls.Fault == genericFaultWrongDeclaredSize {
			declaredSize++
		}
		if blob == nil {
			return genericObservation{Code: "artifact_missing"}, nil
		}
		if len(blob) != declaredSize {
			return genericObservation{Code: "artifact_invalid"}, nil
		}
		if adapter.blobs[auth.tenant] == nil {
			adapter.blobs[auth.tenant] = map[string][]byte{}
		}
		adapter.blobs[auth.tenant][upload.blobDigest] = append([]byte(nil), blob...)
		if adapter.manifests[auth.tenant] == nil {
			adapter.manifests[auth.tenant] = map[string]string{}
		}
		adapter.manifests[auth.tenant][upload.manifestHandle] = request.UploadHandle
		upload.committed = true
		return genericObservation{Code: "committed", UploadHandle: request.UploadHandle, ManifestHandle: upload.manifestHandle}, nil
	case genericArtifactGetManifest:
		if adapter.manifests[auth.tenant][request.ManifestHandle] == "" {
			return genericObservation{Code: "artifact_missing"}, nil
		}
		return genericObservation{Code: "ok", ManifestHandle: request.ManifestHandle}, nil
	case genericArtifactHeadBlob:
		present := adapter.blobs[auth.tenant] != nil && adapter.blobs[auth.tenant][request.BlobDigest] != nil
		if !present {
			return genericObservation{Code: "artifact_missing", BlobPresent: genericBool(false)}, nil
		}
		return genericObservation{Code: "ok", BlobPresent: genericBool(true)}, nil
	default:
		return genericObservation{}, fmt.Errorf("memory adapter unknown artifact action %q", request.Action)
	}
}

func (adapter *genericMemoryAdapter) resourceKey(tenant string, input genericResourceInput) string {
	return tenant + "\x00" + input.Space + "\x00" + input.Ref.APIVersion + "\x00" + input.Ref.Kind + "\x00" + input.Name
}

func (adapter *genericMemoryAdapter) observeResource(stored *genericMemoryResource) *genericObservedResource {
	if stored == nil {
		return nil
	}
	return &genericObservedResource{
		Handle: stored.address.Handle, UID: stored.uid,
		Generation: stored.generationString(), Revision: stored.revisionString(),
		Desired: genericCloneJSONMap(stored.desired), Outputs: genericCloneJSONMap(stored.outputs),
		Conditions: []string{"Ready=True:Available"},
	}
}

func genericMemoryETag(stored *genericMemoryResource) string {
	return `"` + stored.revisionString() + `"`
}

func (stored *genericMemoryResource) generationString() string {
	return strconv.FormatInt(stored.generation, 10)
}
func (stored *genericMemoryResource) revisionString() string {
	return strconv.FormatInt(stored.revision, 10)
}

func (adapter *genericMemoryAdapter) releaseResource(key string, stored *genericMemoryResource) {
	delete(adapter.resources, key)
	if stored.nativeID != "" {
		delete(adapter.nativeClaims, strings.SplitN(key, "\x00", 2)[0]+"\x00"+stored.nativeID)
	}
	for replayKey, replay := range adapter.replays {
		if replay.resourceKey == key {
			delete(adapter.replays, replayKey)
		}
	}
}

func genericMemoryFingerprint(request genericResourceRequest) (string, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}

func sameGenericResourceIntent(prepared, applied genericResourceInput) bool {
	return prepared.Ref == applied.Ref && prepared.Name == applied.Name && prepared.Space == applied.Space &&
		reflect.DeepEqual(prepared.Desired, applied.Desired)
}

func cloneGenericObservation(input genericObservation) genericObservation {
	output := input
	output.Features = append([]string(nil), input.Features...)
	output.Forms = append([]genericObservedForm(nil), input.Forms...)
	if input.EffectiveDesired != nil {
		output.EffectiveDesired = genericCloneJSONMap(input.EffectiveDesired)
	}
	output.MissingBlobs = append([]string(nil), input.MissingBlobs...)
	if input.Resource != nil {
		resource := *input.Resource
		resource.Desired = genericCloneJSONMap(input.Resource.Desired)
		resource.Outputs = genericCloneJSONMap(input.Resource.Outputs)
		output.Resource = &resource
	}
	return output
}
