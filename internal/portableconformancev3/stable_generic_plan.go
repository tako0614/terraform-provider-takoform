package portableconformancev3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformsnapshot"
)

// genericPlanSeed is the complete immutable input shared by the two generic
// conformance adapters. The Snapshot supplies exact data-only contracts; the
// probe supplies Host-owned fixture values. Neither input names an official
// Form family.
type genericPlanSeed struct {
	Snapshot *currentformsnapshot.Snapshot
	Contract Contract
	Probe    stableGenericHostProbe
	Artifact stableGenericArtifactTransport
}

type genericCredential string

const (
	genericCredentialNone        genericCredential = "none"
	genericCredentialPrimary     genericCredential = "primary"
	genericCredentialAlternate   genericCredential = "alternate"
	genericCredentialOtherTenant genericCredential = "other-tenant"
	genericCredentialUnknown     genericCredential = "unknown"
)

type genericActor struct {
	Credential genericCredential
}

type genericCompletionControl string

const (
	genericCompletionImmediate genericCompletionControl = "immediate"
	genericCompletionAsync     genericCompletionControl = "async"
)

type genericBackendEffect string

const (
	genericBackendEffectNone           genericBackendEffect = "none"
	genericBackendEffectTouchStatus    genericBackendEffect = "touch-status"
	genericBackendEffectExternalChange genericBackendEffect = "external-change"
)

type genericFault string

const (
	genericFaultNone              genericFault = "none"
	genericFaultWrongBlobBytes    genericFault = "wrong-blob-bytes"
	genericFaultWrongDeclaredSize genericFault = "wrong-declared-size"
	genericFaultErrorPrefix                    = "error-code:"
)

func genericErrorFault(code string) genericFault {
	return genericFault(genericFaultErrorPrefix + code)
}

func genericFaultErrorCode(fault genericFault) (string, bool) {
	raw := string(fault)
	return strings.TrimPrefix(raw, genericFaultErrorPrefix), strings.HasPrefix(raw, genericFaultErrorPrefix)
}

// genericControls is deliberately the only conformance-only input. Portable
// semantics remain in the typed request variants below; controls merely make
// an asynchronous completion, backend observation, or transport fault
// deterministic in the disposable adapters.
type genericControls struct {
	Completion    genericCompletionControl
	BackendEffect genericBackendEffect
	Fault         genericFault
}

type genericCatalogAction string

const (
	genericCatalogDiscover genericCatalogAction = "discover"
	genericCatalogList     genericCatalogAction = "list"
	genericCatalogGet      genericCatalogAction = "get"
)

type genericCatalogSurface string

const (
	genericCatalogSurfaceDefinition genericCatalogSurface = "definition"
	genericCatalogSurfaceSupport    genericCatalogSurface = "support"
	genericCatalogSurfaceService    genericCatalogSurface = "standard-service-support"
)

type genericCatalogRequest struct {
	Action   genericCatalogAction
	Surface  genericCatalogSurface
	Space    string
	Ref      FormRef
	Protocol string
}

type genericAdmissionAction string

const (
	genericAdmissionValidate genericAdmissionAction = "validate"
	genericAdmissionPrepare  genericAdmissionAction = "prepare"
)

type genericAdmissionRequest struct {
	Action             genericAdmissionAction
	PreparationHandle  string
	Resource           genericResourceInput
	ExpectedGeneration string
}

type genericResourceAction string

const (
	genericResourceApply   genericResourceAction = "apply"
	genericResourceRead    genericResourceAction = "read"
	genericResourceObserve genericResourceAction = "observe"
	genericResourceImport  genericResourceAction = "import"
	genericResourceDelete  genericResourceAction = "delete"
)

type genericResourceInput struct {
	Handle                    string
	Ref                       FormRef
	PackageDigest             string
	Name                      string
	Space                     string
	Desired                   map[string]any
	NativeID                  string
	ExpectedUID               string
	ExpectedGeneration        string
	BodyGeneration            string
	DisagreeingBodyGeneration string
	OmitGeneration            bool
	CreateGenerationFence     string
	ExpectedRevision          string
	PreparationHandle         string
	ReviewSpecHash            string
	IdempotencyKey            string
	Create                    bool
}

type genericResourceRequest struct {
	Action   genericResourceAction
	Resource genericResourceInput
}

type genericOperationAction string

const (
	genericOperationGet    genericOperationAction = "get"
	genericOperationCancel genericOperationAction = "cancel"
)

type genericOperationRequest struct {
	Action genericOperationAction
	Handle string
}

type genericArtifactAction string

const (
	genericArtifactBegin       genericArtifactAction = "begin"
	genericArtifactPut         genericArtifactAction = "put"
	genericArtifactCommit      genericArtifactAction = "commit"
	genericArtifactGetManifest genericArtifactAction = "get-manifest"
	genericArtifactHeadBlob    genericArtifactAction = "head-blob"
)

type genericArtifactRequest struct {
	Action         genericArtifactAction
	UploadHandle   string
	ManifestHandle string
	BlobDigest     string
	Blob           []byte
	DeclaredSize   int
	ContentType    string
	IdempotencyKey string
}

// genericCommand is a tagged union. Exactly one request variant must be set.
// Check IDs and expected outcomes never cross this boundary.
type genericCommand struct {
	Catalog   *genericCatalogRequest
	Admission *genericAdmissionRequest
	Resource  *genericResourceRequest
	Operation *genericOperationRequest
	Artifact  *genericArtifactRequest
	Controls  genericControls
}

func (command genericCommand) validate() error {
	variants := 0
	for _, set := range []bool{
		command.Catalog != nil, command.Admission != nil, command.Resource != nil,
		command.Operation != nil, command.Artifact != nil,
	} {
		if set {
			variants++
		}
	}
	if variants != 1 {
		return fmt.Errorf("generic command carries %d request variants, want exactly one", variants)
	}
	return nil
}

type genericObservedResource struct {
	Handle     string
	UID        string
	Generation string
	Revision   string
	Desired    map[string]any
	NativeID   string
	Outputs    map[string]any
	Conditions []string
}

type genericObservedForm struct {
	Ref                  FormRef
	PackageDigest        string
	DefinitionKnown      bool
	Installed            bool
	Executable           bool
	Activated            bool
	AvailableToPrincipal bool
	Operations           []string
	DeprecatedPresent    bool
}

type genericObservedDefinition struct {
	Ref               FormRef
	PackageDigest     string
	Title             string
	Description       string
	DesiredSchemaHash string
	ConstraintsHash   string
}

type genericObservedSupport struct {
	APIVersion  string
	Kind        string
	Ref         FormRef
	Protocol    string
	Satisfiable *bool
	Operations  []string
	ExtraKeys   []string
}

// genericObservation is adapter-neutral evidence. Adapter-private ids are
// normalized to symbolic handles before they leave the adapter.
type genericObservation struct {
	Code                string
	HTTPStatus          int
	Retryable           *bool
	RequestIDPresent    bool
	APIVersions         []string
	Features            []string
	EndpointPaths       map[string]string
	AddressPath         string
	Forms               []genericObservedForm
	DefinitionDigest    string
	Definition          *genericObservedDefinition
	Support             *genericObservedSupport
	Valid               *bool
	HasDiagnostics      *bool
	EffectiveDesired    map[string]any
	PreparationHandle   string
	PreparationSpecHash string
	ETag                string
	Resource            *genericObservedResource
	OperationHandle     string
	OperationDone       *bool
	OperationCancelled  bool
	UploadHandle        string
	MissingBlobs        []string
	ManifestHandle      string
	BlobPresent         *bool
}

type genericExpected struct {
	Code                string
	HTTPStatus          *int
	Retryable           *bool
	RequestIDPresent    *bool
	APIVersions         []string
	Features            []string
	EndpointPaths       map[string]string
	AddressPath         string
	Valid               *bool
	HasDiagnostics      *bool
	FormCount           *int
	Forms               []genericObservedForm
	DefinitionDigest    string
	Definition          *genericObservedDefinition
	Support             *genericObservedSupport
	PreparationHandle   string
	PreparationSpecHash string
	ETag                string
	EffectiveDesired    map[string]any
	ResourceHandle      string
	Generation          string
	Revision            string
	Desired             map[string]any
	NativeID            string
	Outputs             map[string]any
	Conditions          []string
	SameUIDAs           string
	DifferentUIDFrom    string
	OperationHandle     string
	OperationDone       *bool
	OperationCancelled  *bool
	UploadHandle        string
	MissingBlobCount    *int
	ManifestHandle      string
	SameManifestAs      string
	BlobPresent         *bool
}

type genericPlanCase struct {
	ID       string
	Actor    genericActor
	Command  genericCommand
	Expected genericExpected
	Checks   []string
}

type genericPlan struct {
	Cases []genericPlanCase
}

type genericPlanAdapter interface {
	Call(context.Context, genericActor, genericCommand) (genericObservation, error)
}

type genericPlanAdapterFactory func(genericPlanSeed) (genericPlanAdapter, func(), error)

// stableRunGenericPlan starts both adapters from the same seed and sends every
// typed call to both. Relational assertions and check-to-case reporting stay
// here, outside both subjects.
func stableRunGenericPlan(
	ctx context.Context,
	seed genericPlanSeed,
	plan genericPlan,
	requiredChecks []string,
	memoryFactory, httpFactory genericPlanAdapterFactory,
) (map[string]bool, error) {
	if seed.Snapshot == nil {
		return nil, errors.New("generic plan seed carries no Snapshot")
	}
	if err := validateGenericPlan(plan, requiredChecks); err != nil {
		return nil, err
	}
	memory, closeMemory, err := memoryFactory(seed)
	if err != nil {
		return nil, fmt.Errorf("construct generic memory adapter: %w", err)
	}
	if closeMemory != nil {
		defer closeMemory()
	}
	httpAdapter, closeHTTP, err := httpFactory(seed)
	if err != nil {
		return nil, fmt.Errorf("construct generic HTTP adapter: %w", err)
	}
	if closeHTTP != nil {
		defer closeHTTP()
	}

	memoryHistory := map[string]genericObservation{}
	httpHistory := map[string]genericObservation{}
	executedEvidence := make(map[string]int, len(requiredChecks))
	for _, planCase := range plan.Cases {
		if err := planCase.Command.validate(); err != nil {
			return nil, fmt.Errorf("generic plan case %s: %w", planCase.ID, err)
		}
		memoryObserved, err := memory.Call(ctx, planCase.Actor, planCase.Command)
		if err != nil {
			return nil, fmt.Errorf("generic plan case %s memory adapter: %w", planCase.ID, err)
		}
		httpObserved, err := httpAdapter.Call(ctx, planCase.Actor, planCase.Command)
		if err != nil {
			return nil, fmt.Errorf("generic plan case %s HTTP adapter: %w", planCase.ID, err)
		}
		if err := verifyGenericExpected(planCase, memoryObserved, memoryHistory); err != nil {
			return nil, fmt.Errorf("generic plan case %s memory observation: %w", planCase.ID, err)
		}
		if err := verifyGenericExpected(planCase, httpObserved, httpHistory); err != nil {
			return nil, fmt.Errorf("generic plan case %s HTTP observation: %w", planCase.ID, err)
		}
		if !reflect.DeepEqual(memoryObserved, httpObserved) {
			memoryRaw, _ := json.Marshal(memoryObserved)
			httpRaw, _ := json.Marshal(httpObserved)
			return nil, fmt.Errorf("generic plan case %s adapters diverged: memory=%s http=%s", planCase.ID, memoryRaw, httpRaw)
		}
		memoryHistory[planCase.ID] = memoryObserved
		httpHistory[planCase.ID] = httpObserved
		for _, check := range planCase.Checks {
			executedEvidence[check]++
		}
	}
	completed := make(map[string]bool, len(requiredChecks))
	for _, check := range requiredChecks {
		wantEvidence := 0
		for _, planCase := range plan.Cases {
			for _, candidate := range planCase.Checks {
				if candidate == check {
					wantEvidence++
				}
			}
		}
		if wantEvidence == 0 || executedEvidence[check] != wantEvidence {
			return nil, fmt.Errorf(
				"generic check %s executed %d/%d mapped evidence cases",
				check, executedEvidence[check], wantEvidence,
			)
		}
		completed[check] = true
	}
	return completed, nil
}

func validateGenericPlan(plan genericPlan, requiredChecks []string) error {
	caseIDs := map[string]bool{}
	checkEvidence := map[string]int{}
	required := map[string]bool{}
	for _, check := range requiredChecks {
		required[check] = true
	}
	for _, planCase := range plan.Cases {
		if planCase.ID == "" || caseIDs[planCase.ID] {
			return fmt.Errorf("generic plan repeats empty or duplicate case id %q", planCase.ID)
		}
		caseIDs[planCase.ID] = true
		for _, check := range planCase.Checks {
			if !required[check] {
				return fmt.Errorf("generic plan case %s names non-generic check %q", planCase.ID, check)
			}
			checkEvidence[check]++
		}
	}
	missing := make([]string, 0)
	for _, check := range requiredChecks {
		if checkEvidence[check] == 0 {
			missing = append(missing, check)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("generic plan omits exact mappings for %v", missing)
	}
	return nil
}

func verifyGenericExpected(
	planCase genericPlanCase,
	observed genericObservation,
	history map[string]genericObservation,
) error {
	want := planCase.Expected
	if observed.Code != want.Code {
		return fmt.Errorf("code = %q, want %q", observed.Code, want.Code)
	}
	if want.HTTPStatus != nil && observed.HTTPStatus != *want.HTTPStatus {
		return fmt.Errorf("http status = %d, want %d", observed.HTTPStatus, *want.HTTPStatus)
	}
	if want.Retryable != nil && (observed.Retryable == nil || *observed.Retryable != *want.Retryable) {
		return fmt.Errorf("retryable = %v, want %v", observed.Retryable, *want.Retryable)
	}
	if want.RequestIDPresent != nil && observed.RequestIDPresent != *want.RequestIDPresent {
		return fmt.Errorf("request id present = %v, want %v", observed.RequestIDPresent, *want.RequestIDPresent)
	}
	if want.APIVersions != nil && !reflect.DeepEqual(observed.APIVersions, want.APIVersions) {
		return fmt.Errorf("api versions = %#v, want %#v", observed.APIVersions, want.APIVersions)
	}
	if want.Features != nil && !reflect.DeepEqual(observed.Features, want.Features) {
		return fmt.Errorf("features = %#v, want %#v", observed.Features, want.Features)
	}
	if want.EndpointPaths != nil && !reflect.DeepEqual(observed.EndpointPaths, want.EndpointPaths) {
		return fmt.Errorf("endpoint paths = %#v, want %#v", observed.EndpointPaths, want.EndpointPaths)
	}
	if want.AddressPath != "" && observed.AddressPath != want.AddressPath {
		return fmt.Errorf("address path = %q, want %q", observed.AddressPath, want.AddressPath)
	}
	if want.Valid != nil && (observed.Valid == nil || *observed.Valid != *want.Valid) {
		return fmt.Errorf("valid = %v, want %v", observed.Valid, *want.Valid)
	}
	if want.HasDiagnostics != nil && (observed.HasDiagnostics == nil || *observed.HasDiagnostics != *want.HasDiagnostics) {
		return fmt.Errorf("has diagnostics = %v, want %v", observed.HasDiagnostics, *want.HasDiagnostics)
	}
	if want.FormCount != nil && len(observed.Forms) != *want.FormCount {
		return fmt.Errorf("form count = %d, want %d", len(observed.Forms), *want.FormCount)
	}
	if want.Forms != nil && !reflect.DeepEqual(observed.Forms, want.Forms) {
		return fmt.Errorf("forms = %#v, want %#v", observed.Forms, want.Forms)
	}
	if want.DefinitionDigest != "" && observed.DefinitionDigest != want.DefinitionDigest {
		return fmt.Errorf("definition digest = %q, want %q", observed.DefinitionDigest, want.DefinitionDigest)
	}
	if want.Definition != nil && !reflect.DeepEqual(observed.Definition, want.Definition) {
		return fmt.Errorf("definition = %#v, want %#v", observed.Definition, want.Definition)
	}
	if want.Support != nil && !reflect.DeepEqual(observed.Support, want.Support) {
		return fmt.Errorf("support = %#v, want %#v", observed.Support, want.Support)
	}
	if want.PreparationHandle != "" && observed.PreparationHandle != want.PreparationHandle {
		return fmt.Errorf("preparation handle = %q, want %q", observed.PreparationHandle, want.PreparationHandle)
	}
	if want.PreparationSpecHash != "" && observed.PreparationSpecHash != want.PreparationSpecHash {
		return fmt.Errorf("preparation spec hash = %q, want %q", observed.PreparationSpecHash, want.PreparationSpecHash)
	}
	if want.ETag != "" && observed.ETag != want.ETag {
		return fmt.Errorf("etag = %q, want %q", observed.ETag, want.ETag)
	}
	if want.EffectiveDesired != nil && !reflect.DeepEqual(observed.EffectiveDesired, want.EffectiveDesired) {
		return fmt.Errorf("effective desired = %#v, want %#v", observed.EffectiveDesired, want.EffectiveDesired)
	}
	if want.ResourceHandle != "" {
		if observed.Resource == nil || observed.Resource.Handle != want.ResourceHandle {
			return fmt.Errorf("resource handle = %v, want %q", observed.Resource, want.ResourceHandle)
		}
		if want.Generation != "" && observed.Resource.Generation != want.Generation {
			return fmt.Errorf("generation = %q, want %q", observed.Resource.Generation, want.Generation)
		}
		if want.Revision != "" && observed.Resource.Revision != want.Revision {
			return fmt.Errorf("revision = %q, want %q", observed.Resource.Revision, want.Revision)
		}
		if want.Desired != nil && !reflect.DeepEqual(observed.Resource.Desired, want.Desired) {
			return fmt.Errorf("desired = %#v, want %#v", observed.Resource.Desired, want.Desired)
		}
		if want.NativeID != "" && observed.Resource.NativeID != want.NativeID {
			return fmt.Errorf("native id = %q, want %q", observed.Resource.NativeID, want.NativeID)
		}
		if want.Outputs != nil && !reflect.DeepEqual(observed.Resource.Outputs, want.Outputs) {
			return fmt.Errorf("outputs = %#v, want %#v", observed.Resource.Outputs, want.Outputs)
		}
		if want.Conditions != nil && !reflect.DeepEqual(observed.Resource.Conditions, want.Conditions) {
			return fmt.Errorf("conditions = %#v, want %#v", observed.Resource.Conditions, want.Conditions)
		}
	}
	if want.SameUIDAs != "" {
		prior := history[want.SameUIDAs].Resource
		if prior == nil || observed.Resource == nil || prior.UID != observed.Resource.UID {
			return fmt.Errorf("uid does not match case %s", want.SameUIDAs)
		}
	}
	if want.DifferentUIDFrom != "" {
		prior := history[want.DifferentUIDFrom].Resource
		if prior == nil || observed.Resource == nil || prior.UID == observed.Resource.UID {
			return fmt.Errorf("uid does not differ from case %s", want.DifferentUIDFrom)
		}
	}
	if want.OperationHandle != "" && observed.OperationHandle != want.OperationHandle {
		return fmt.Errorf("operation handle = %q, want %q", observed.OperationHandle, want.OperationHandle)
	}
	if want.OperationDone != nil && (observed.OperationDone == nil || *observed.OperationDone != *want.OperationDone) {
		return fmt.Errorf("operation done = %v, want %v", observed.OperationDone, *want.OperationDone)
	}
	if want.OperationCancelled != nil && observed.OperationCancelled != *want.OperationCancelled {
		return fmt.Errorf("operation cancelled = %v, want %v", observed.OperationCancelled, *want.OperationCancelled)
	}
	if want.UploadHandle != "" && observed.UploadHandle != want.UploadHandle {
		return fmt.Errorf("upload handle = %q, want %q", observed.UploadHandle, want.UploadHandle)
	}
	if want.MissingBlobCount != nil && len(observed.MissingBlobs) != *want.MissingBlobCount {
		return fmt.Errorf("missing blob count = %d, want %d", len(observed.MissingBlobs), *want.MissingBlobCount)
	}
	if want.ManifestHandle != "" && observed.ManifestHandle != want.ManifestHandle {
		return fmt.Errorf("manifest handle = %q, want %q", observed.ManifestHandle, want.ManifestHandle)
	}
	if want.SameManifestAs != "" && observed.ManifestHandle != history[want.SameManifestAs].ManifestHandle {
		return fmt.Errorf("manifest handle does not match case %s", want.SameManifestAs)
	}
	if want.BlobPresent != nil && (observed.BlobPresent == nil || *observed.BlobPresent != *want.BlobPresent) {
		return fmt.Errorf("blob present = %v, want %v", observed.BlobPresent, *want.BlobPresent)
	}
	return nil
}

func genericSortedOperations(values []string) []string {
	output := append([]string(nil), values...)
	sort.Strings(output)
	return output
}

func genericBool(value bool) *bool { return &value }
func genericInt(value int) *int    { return &value }

func genericCanonicalHash(value any) (string, error) {
	// Wrap scalars and nil so the canonicalizer always receives a complete
	// object document; the member value remains the evidence being hashed.
	raw, err := json.Marshal(map[string]any{"value": value})
	if err != nil {
		return "", err
	}
	return formpackage.DigestCanonicalJSON(raw)
}
