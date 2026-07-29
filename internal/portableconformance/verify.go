package portableconformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

const APIVersion = formpackage.FormAPIVersion

type FormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
}

// UnmarshalJSON validates the original nested bytes before encoding/json can
// collapse duplicate member names.
func (ref *FormRef) UnmarshalJSON(raw []byte) error {
	validated, err := formpackage.ValidateFormRef(raw)
	if err != nil {
		return fmt.Errorf("portable host FormRef JSON: %w", err)
	}
	ref.APIVersion = validated.APIVersion
	ref.Kind = validated.Kind
	ref.DefinitionVersion = validated.DefinitionVersion
	ref.SchemaDigest = validated.SchemaDigest
	return nil
}

type InstalledFormReference struct {
	FormRef       FormRef `json:"formRef"`
	PackageDigest string  `json:"packageDigest"`
}

type RunnerInput struct {
	Space            string                     `json:"space"`
	AlternateSpace   string                     `json:"alternateSpace"`
	Name             string                     `json:"name"`
	Identity         InstalledFormReference     `json:"identity"`
	Desired          map[string]any             `json:"desired"`
	ConnectionProbe  ConnectionProbeInput       `json:"connectionProbe"`
	NegativeFixtures []RunnerNegativeFixture    `json:"negativeFixtures"`
	ImportNativeID   string                     `json:"importNativeId"`
	Definition       formpackage.FormDefinition `json:"-"`
}

type ConnectionProbeInput struct {
	SourceName     string                 `json:"sourceName"`
	SourceIdentity InstalledFormReference `json:"sourceIdentity"`
	Desired        map[string]any         `json:"desired"`
	TargetKind     string                 `json:"targetKind"`
	TargetName     string                 `json:"targetName"`
}

// RunnerNegativeFixture is exact desired-request evidence. Input is hydrated
// from Path after the byte digest and containment checks pass; it is never
// serialized into the contract or its input digest.
type RunnerNegativeFixture struct {
	Name   string         `json:"name"`
	Stage  string         `json:"stage"`
	Path   string         `json:"path"`
	SHA256 string         `json:"sha256"`
	Input  map[string]any `json:"-"`
}

type RunnerEvidence struct {
	Subject    string `json:"subject"`
	Entrypoint string `json:"entrypoint"`
	SelfTest   string `json:"selfTest"`
	SHA256     string `json:"sha256"`
}

type ResourceVersionContract struct {
	Encoding              string `json:"encoding"`
	Minimum               string `json:"minimum"`
	Maximum               string `json:"maximum"`
	ETag                  string `json:"etag"`
	ObserveRefreshSuccess string `json:"observeRefreshSuccess"`
	StaleFence            string `json:"staleFence"`
}

type PlanBindingContract struct {
	Canonicalization    string                        `json:"canonicalization"`
	PortableInputs      []string                      `json:"portableInputs"`
	HostPlan            string                        `json:"hostPlan"`
	SubstitutionFailure string                        `json:"substitutionFailure"`
	PureBlackBoxInputs  []string                      `json:"pureBlackBoxInputs"`
	InstrumentedAdapter InstrumentedPlanProbeContract `json:"instrumentedAdapter"`
}

type InstrumentedPlanProbeContract struct {
	Scope                    string   `json:"scope"`
	RequestHeader            string   `json:"requestHeader"`
	ResultHeader             string   `json:"resultHeader"`
	Inputs                   []string `json:"inputs"`
	BindingPath              string   `json:"bindingPath"`
	RejectedResult           string   `json:"rejectedResult"`
	AcceptedNoMutationResult string   `json:"acceptedNoMutationResult"`
	UnknownValueFailure      string   `json:"unknownValueFailure"`
	AuthorizationOrder       string   `json:"authorizationOrder"`
}

type IdempotencyContract struct {
	ScopeDimensions        []string `json:"scopeDimensions"`
	Fingerprint            []string `json:"fingerprint"`
	ReplayAuthorization    string   `json:"replayAuthorization"`
	CrossPrincipalRule     string   `json:"crossPrincipalRule"`
	CrossTenantRule        string   `json:"crossTenantRule"`
	CrossTenantProbe       string   `json:"crossTenantProbe"`
	RevokedRule            string   `json:"revokedRule"`
	ReplayDenialCodes      []string `json:"replayDenialCodes"`
	DeniedReplayRecordRule string   `json:"deniedReplayRecordRule"`
}

type WireJSONContract struct {
	Encoding                   string `json:"encoding"`
	RawValidationOrder         string `json:"rawValidationOrder"`
	UnknownRequestFieldFailure string `json:"unknownRequestFieldFailure"`
	DuplicateRequestFailure    string `json:"duplicateRequestFailure"`
	DuplicateResponseFailure   string `json:"duplicateResponseFailure"`
	DuplicateContractFailure   string `json:"duplicateContractFailure"`
}

type ConnectionResolutionContract struct {
	SourceSpace string `json:"sourceSpace"`
	Reference   string `json:"reference"`
	CrossSpace  string `json:"crossSpace"`
	Missing     string `json:"missing"`
}

type InterfaceDeclarations struct {
	Optional              bool     `json:"optional"`
	FeatureFlag           string   `json:"featureFlag"`
	EndpointKey           string   `json:"endpointKey"`
	ReadOnly              bool     `json:"readOnly"`
	EndpointOriginRule    string   `json:"endpointOriginRule"`
	MaterializationSource string   `json:"materializationSource"`
	SpaceQuery            string   `json:"spaceQuery"`
	SpaceSelectionRule    string   `json:"spaceSelectionRule"`
	ListPath              string   `json:"listPath"`
	GetPath               string   `json:"getPath"`
	DescriptorIdentity    []string `json:"descriptorIdentity"`
	RuntimeIdentity       []string `json:"runtimeIdentity"`
	VersionQuery          string   `json:"versionQuery"`
	OmittedVersionRule    string   `json:"omittedVersionRule"`
	ResourceKindQuery     string   `json:"resourceKindQuery"`
	ResourceNameQuery     string   `json:"resourceNameQuery"`
	OmittedResourceRule   string   `json:"omittedResourceRule"`
	IdentityAmbiguousCode string   `json:"identityAmbiguousCode"`
	InstanceAmbiguousCode string   `json:"instanceAmbiguousCode"`
	DocumentRule          string   `json:"documentRule"`
	RequiredReadinessRule string   `json:"requiredReadinessRule"`
	Checks                []string `json:"checks"`
}

type Contract struct {
	Format                 string                       `json:"format"`
	APIVersion             string                       `json:"apiVersion"`
	DiscoveryPath          string                       `json:"discoveryPath"`
	APIPath                string                       `json:"apiPath"`
	RunnerInput            RunnerInput                  `json:"runnerInput"`
	ResourceVersion        ResourceVersionContract      `json:"resourceVersion"`
	PlanBinding            PlanBindingContract          `json:"planBinding"`
	Idempotency            IdempotencyContract          `json:"idempotency"`
	WireJSON               WireJSONContract             `json:"wireJSON"`
	ConnectionResolution   ConnectionResolutionContract `json:"connectionResolution"`
	Preconditions          map[string]string            `json:"preconditions"`
	IdempotentOperations   []string                     `json:"idempotentOperations"`
	ValidationErrorCode    string                       `json:"validationErrorCode"`
	StableErrorCodes       []string                     `json:"stableErrorCodes"`
	RetryableCodes         []string                     `json:"retryableCodes"`
	RequiredRunnerChecks   []string                     `json:"requiredRunnerChecks"`
	InterfaceDeclarations  InterfaceDeclarations        `json:"interfaceDeclarations"`
	ForbiddenProviderState []string                     `json:"forbiddenProviderState"`
	RunnerEvidence         RunnerEvidence               `json:"runnerEvidence"`
}

type manifest struct {
	Format   string `json:"format"`
	Contract string `json:"contract"`
	SHA256   string `json:"sha256"`
}

func Verify(root string) (Contract, error) {
	var index manifest
	if err := decodeStrict(filepath.Join(root, "manifest.json"), &index); err != nil {
		return Contract{}, err
	}
	if index.Format != "takoform.portable-host-conformance-manifest@v1" || index.Contract != "contract.json" {
		return Contract{}, errors.New("portable host conformance manifest identity is invalid")
	}
	contractPath := filepath.Join(root, index.Contract)
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		return Contract{}, err
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != index.SHA256 {
		return Contract{}, errors.New("portable host conformance contract digest drifted")
	}
	var contract Contract
	if err := decodeStrict(contractPath, &contract); err != nil {
		return Contract{}, err
	}
	if err := hydrateRunnerFixtures(root, &contract); err != nil {
		return Contract{}, err
	}
	if err := validate(contract); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func validate(contract Contract) error {
	if contract.Format != "takoform.portable-host-conformance@v1" || contract.APIVersion != APIVersion ||
		contract.DiscoveryPath != "/.well-known/takoform" || contract.APIPath != "/apis/forms.takoform.com/v1alpha1" ||
		contract.RunnerEvidence.Subject != "takoform.portable-host-conformance-runner@v1" ||
		contract.RunnerEvidence.Entrypoint != "cmd/portable-host-conformance" ||
		contract.RunnerEvidence.SelfTest != "go run ./cmd/portable-host-conformance self-test" {
		return errors.New("portable host contract identity is invalid")
	}
	identity := contract.RunnerInput.Identity
	formRefRaw, err := json.Marshal(identity.FormRef)
	if err != nil {
		return fmt.Errorf("portable host runner FormRef: %w", err)
	}
	if _, err := formpackage.ValidateFormRef(formRefRaw); err != nil {
		return fmt.Errorf("portable host runner FormRef: %w", err)
	}
	if !formpackage.ValidDigest(identity.PackageDigest) ||
		client.ValidateSpaceID(contract.RunnerInput.Space) != nil ||
		client.ValidateSpaceID(contract.RunnerInput.AlternateSpace) != nil ||
		contract.RunnerInput.AlternateSpace == contract.RunnerInput.Space ||
		client.ValidateResourceName(contract.RunnerInput.Name) != nil ||
		contract.RunnerInput.ImportNativeID == "" ||
		len(contract.RunnerInput.Desired) == 0 || len(contract.RunnerInput.NegativeFixtures) == 0 {
		return errors.New("portable host runner input is incomplete")
	}
	connectionProbe := contract.RunnerInput.ConnectionProbe
	connectionFormRefRaw, err := json.Marshal(connectionProbe.SourceIdentity.FormRef)
	if err != nil {
		return fmt.Errorf("portable host connection probe FormRef: %w", err)
	}
	if _, err := formpackage.ValidateFormRef(connectionFormRefRaw); err != nil {
		return fmt.Errorf("portable host connection probe FormRef: %w", err)
	}
	if !formpackage.ValidDigest(connectionProbe.SourceIdentity.PackageDigest) ||
		connectionProbe.SourceName == "" ||
		connectionProbe.TargetKind == "" ||
		connectionProbe.TargetName == "" ||
		len(connectionProbe.Desired) == 0 {
		return errors.New("portable host connection probe input is incomplete")
	}
	if desiredName, ok := connectionProbe.Desired["name"].(string); !ok || desiredName != connectionProbe.SourceName {
		return errors.New("portable host connection probe name must equal desired.name")
	}
	if !connectionProbeReferencesTarget(connectionProbe) {
		return errors.New("portable host connection probe desired input does not reference its exact target")
	}
	desiredName, ok := contract.RunnerInput.Desired["name"].(string)
	if !ok || desiredName != contract.RunnerInput.Name {
		return errors.New("portable host runner name must equal desired.name")
	}
	fixtureNames := map[string]bool{}
	fixturePaths := map[string]bool{}
	for _, fixture := range contract.RunnerInput.NegativeFixtures {
		if fixture.Name == "" || fixture.Stage != "desired" || fixture.Path == "" ||
			!formpackage.ValidDigest(fixture.SHA256) || len(fixture.Input) == 0 ||
			fixtureNames[fixture.Name] || fixturePaths[fixture.Path] {
			return errors.New("portable host runner desired negative fixture inventory is invalid")
		}
		fixtureNames[fixture.Name] = true
		fixturePaths[fixture.Path] = true
	}
	const (
		createPrecondition = "If-None-Match: *; conflict=resource_version_conflict/412"
		updatePrecondition = "If-Match: quoted positive-decimal resourceVersion; stale=resource_version_conflict/412"
	)
	wantPreconditions := map[string]string{
		"create": createPrecondition, "update": updatePrecondition,
		"import":  createPrecondition + " or " + updatePrecondition,
		"observe": updatePrecondition, "refresh": updatePrecondition,
		"delete": updatePrecondition,
	}
	if !reflect.DeepEqual(contract.Preconditions, wantPreconditions) {
		return errors.New("portable host mutation preconditions drifted")
	}
	wantResourceVersion := ResourceVersionContract{
		Encoding:              "canonical-decimal-string",
		Minimum:               "1",
		Maximum:               "9223372036854775807",
		ETag:                  "exactly-one-strong-quoted-resource-version",
		ObserveRefreshSuccess: "equals-if-match",
		StaleFence:            "resource_version_conflict/412",
	}
	if contract.ResourceVersion != wantResourceVersion {
		return errors.New("portable host resourceVersion contract drifted")
	}
	wantPlanBinding := PlanBindingContract{
		Canonicalization: "RFC8785",
		PortableInputs: []string{
			"specDigest",
			"resource.apiVersion",
			"resource.kind",
			"resource.metadata.name",
			"resource.metadata.space",
			"resource.metadata.resourceVersion",
			"resource.form.formRef.apiVersion",
			"resource.form.formRef.kind",
			"resource.form.formRef.definitionVersion",
			"resource.form.formRef.schemaDigest",
			"resource.form.packageDigest",
		},
		HostPlan:            "exact-reviewed-plan",
		SubstitutionFailure: "invalid_argument/400-before-mutation",
		PureBlackBoxInputs: []string{
			"specDigest",
			"resource.metadata.space",
			"resource.metadata.resourceVersion",
		},
		InstrumentedAdapter: InstrumentedPlanProbeContract{
			Scope:         "disposable-conformance-adapter-only; production-forbidden",
			RequestHeader: PlanBindingProbeHeader,
			ResultHeader:  PlanBindingProbeResultHeader,
			Inputs: []string{
				"resource.apiVersion",
				"resource.kind",
				"resource.metadata.name",
				"resource.form.formRef.apiVersion",
				"resource.form.formRef.kind",
				"resource.form.formRef.definitionVersion",
				"resource.form.formRef.schemaDigest",
				"resource.form.packageDigest",
			},
			BindingPath:              "same-canonical-plan-binding-function",
			RejectedResult:           "invalid_argument/400; result=rejected",
			AcceptedNoMutationResult: "204; result=accepted-no-mutation; no-state-mutation",
			UnknownValueFailure:      "invalid_argument/400",
			AuthorizationOrder:       "authenticate-and-authorize-before-probe",
		},
	}
	if !reflect.DeepEqual(contract.PlanBinding, wantPlanBinding) {
		return errors.New("portable host plan binding contract drifted")
	}
	wantIdempotency := IdempotencyContract{
		ScopeDimensions: []string{
			"authenticated-tenant",
			"authenticated-principal",
			"space",
			"idempotency-key",
		},
		Fingerprint: []string{
			"method",
			"request-target",
			"preconditions",
			"request-body-bytes",
		},
		ReplayAuthorization:    "authenticate-and-authorize-current-request-before-cache-lookup",
		CrossPrincipalRule:     "same-key-is-independent",
		CrossTenantRule:        "same-key-is-independent",
		CrossTenantProbe:       "same-principal-different-tenant-shared-disposable-resource",
		RevokedRule:            "current-authentication-or-authorization-error-not-cached-success",
		ReplayDenialCodes:      []string{"unauthenticated", "permission_denied", "policy_denied"},
		DeniedReplayRecordRule: "denial-neither-returns-nor-overwrites-cached-success",
	}
	if !reflect.DeepEqual(contract.Idempotency, wantIdempotency) {
		return errors.New("portable host idempotency authorization contract drifted")
	}
	wantWireJSON := WireJSONContract{
		Encoding:                   "UTF-8 I-JSON",
		RawValidationOrder:         "validate-complete-raw-document-before-typed-decode",
		UnknownRequestFieldFailure: "invalid_argument/400-before-mutation",
		DuplicateRequestFailure:    "invalid_argument/400-before-mutation",
		DuplicateResponseFailure:   "protocol-invalid-before-typed-semantics",
		DuplicateContractFailure:   "verification-failure-before-typed-decode",
	}
	if contract.WireJSON != wantWireJSON {
		return errors.New("portable host raw JSON contract drifted")
	}
	wantConnectionResolution := ConnectionResolutionContract{
		SourceSpace: "resource.metadata.space",
		Reference:   "Kind/name",
		CrossSpace:  "unrepresentable",
		Missing:     "resource_not_found/404-before-mutation",
	}
	if contract.ConnectionResolution != wantConnectionResolution {
		return errors.New("portable host connection resolution contract drifted")
	}
	wantIdempotent := []string{"apply", "import", "observe", "refresh", "delete"}
	if !reflect.DeepEqual(contract.IdempotentOperations, wantIdempotent) {
		return errors.New("portable host idempotency operations drifted")
	}
	wantRetryable := []string{"resource_busy", "backend_unavailable"}
	if !reflect.DeepEqual(contract.RetryableCodes, wantRetryable) {
		return errors.New("portable host retry taxonomy drifted")
	}
	if contract.ValidationErrorCode != "invalid_argument" {
		return errors.New("portable host validation errors must normalize to invalid_argument")
	}
	wantErrors := []string{
		"invalid_argument", "unauthenticated", "permission_denied", "form_unknown", "form_not_installed",
		"form_unavailable", "form_identity_conflict", "resource_not_found", "resource_version_conflict",
		"resource_busy", "import_conflict", "policy_denied", "backend_unavailable",
		"interface_identity_ambiguous", "interface_instance_ambiguous", "internal_error",
	}
	if !reflect.DeepEqual(contract.StableErrorCodes, wantErrors) {
		return errors.New("portable host stable error taxonomy drifted")
	}
	wantChecks := []string{
		"discovery", "exact-availability", "preview", "desired-negative-fixtures", "resource-version-bounds",
		"preview-plan-spec-binding", "preview-plan-resource-identity-binding", "preview-plan-space-binding",
		"preview-plan-form-ref-binding", "preview-plan-package-digest-binding",
		"unknown-top-level-request-field-rejected-before-mutation",
		"unknown-metadata-field-rejected-before-mutation",
		"unknown-desired-authority-field-rejected-before-mutation",
		"duplicate-metadata-space-rejected-before-typed-decode",
		"duplicate-resource-version-rejected-before-typed-decode",
		"duplicate-spec-rejected-before-typed-decode",
		"non-utf8-request-rejected-before-typed-decode",
		"duplicate-error-code-response-rejected-before-typed-decode",
		"apply-headers-required", "apply", "apply-idempotency",
		"idempotency-cross-principal-isolated", "idempotency-cross-tenant-isolated",
		"idempotency-replay-authentication-before-cache",
		"idempotency-replay-permission-before-cache",
		"idempotency-replay-policy-before-cache",
		"resource-cross-space-read-isolated", "idempotency-cross-space-isolated",
		"resource-cross-space-mutation-isolated",
		"connection-cross-space-target-not-found", "connection-missing-target-no-source-mutation",
		"connection-same-space-resolves",
		"idempotency-key-reuse-rejected", "create-precondition-conflict",
		"update-headers-required", "update", "update-idempotency", "interface-ready-after-update", "stale-update-rejected",
		"read",
		"canonical-resource-parity", "exact-digest-substitution-rejected", "invalid-argument-normalization",
		"observe", "observe-idempotency", "observe-headers-required", "observe-generation-fence",
		"refresh", "refresh-idempotency", "refresh-headers-required", "refresh-generation-fence",
		"stale-delete-rejected", "retryable-code-semantics", "stable-error-http-status-mapping",
		"interface-required-feature-advertised", "interface-endpoint-same-origin",
		"interface-space-required", "interface-query-vocabulary-closed",
		"interface-absent-before-ready", "interface-ready-projection",
		"portable-interface-routes-reject-writes", "interface-space-isolation",
		"interface-ready-projection-exactly-matches-form-descriptors",
		"interface-exact-pair-read", "interface-omitted-version-unique-resolves",
		"interface-exact-resource-instance-read", "interface-multi-resource-instance-fails-closed",
		"interface-document-exact-copy", "interface-document-schema-valid",
		"interface-required-ready-projection-present",
		"interface-projection-contains-no-authority-fields",
		"delete-headers-required", "delete", "delete-idempotency", "interface-absent-after-delete",
		"import-headers-required", "import", "import-idempotency", "interface-ready-after-import",
		"import-update-headers-required", "import-update-stale-rejected", "import-update", "interface-ready-after-import-update", "post-import-delete-readback",
	}
	if !reflect.DeepEqual(contract.RequiredRunnerChecks, wantChecks) {
		return errors.New("portable host required runner checks drifted")
	}
	wantDeclarations := InterfaceDeclarations{
		Optional:              true,
		FeatureFlag:           "interface_declarations",
		EndpointKey:           "interfaces",
		ReadOnly:              true,
		EndpointOriginRule:    "same-origin-with-api",
		MaterializationSource: "form-declared-descriptors",
		SpaceQuery:            "space",
		SpaceSelectionRule:    "explicit-selector-else-provider-default-required",
		ListPath:              "{api}/interfaces",
		GetPath:               "{api}/interfaces/{name}",
		DescriptorIdentity:    []string{"name", "version"},
		RuntimeIdentity:       []string{"space", "resource.kind", "resource.name", "name", "version"},
		VersionQuery:          "version",
		OmittedVersionRule:    "only-when-visible-name-is-unique",
		ResourceKindQuery:     "resourceKind",
		ResourceNameQuery:     "resourceName",
		OmittedResourceRule:   "only-when-visible-instance-is-unique",
		IdentityAmbiguousCode: "interface_identity_ambiguous",
		InstanceAmbiguousCode: "interface_instance_ambiguous",
		DocumentRule:          "copy-exact-declared-document",
		RequiredReadinessRule: "not-ready-unless-materialized-validated-resolved",
		Checks: []string{
			"interface-required-feature-advertised", "interface-endpoint-same-origin",
			"interface-space-required", "interface-query-vocabulary-closed",
			"interface-absent-before-ready", "interface-ready-projection",
			"portable-interface-routes-reject-writes", "interface-space-isolation",
			"interface-ready-projection-exactly-matches-form-descriptors",
			"interface-exact-pair-read", "interface-omitted-version-unique-resolves",
			"interface-exact-resource-instance-read", "interface-multi-resource-instance-fails-closed",
			"interface-document-exact-copy", "interface-document-schema-valid",
			"interface-required-ready-projection-present",
			"interface-projection-contains-no-authority-fields", "interface-absent-after-delete",
		},
	}
	if !reflect.DeepEqual(contract.InterfaceDeclarations, wantDeclarations) {
		return errors.New("portable host interface declaration surface drifted")
	}
	requiredChecks := make(map[string]int, len(contract.RequiredRunnerChecks))
	for _, check := range contract.RequiredRunnerChecks {
		requiredChecks[check]++
	}
	for _, check := range contract.InterfaceDeclarations.Checks {
		if requiredChecks[check] != 1 {
			return fmt.Errorf("portable host Interface check %q must appear exactly once in required runner checks", check)
		}
	}
	runnerDigest, err := RunnerEvidenceDigest(contract)
	if err != nil {
		return fmt.Errorf("portable host neutral runner evidence: %w", err)
	}
	if contract.RunnerEvidence.SHA256 != runnerDigest {
		return errors.New("portable host neutral runner evidence digest drifted")
	}
	wantForbidden := []string{
		"credential", "secret", "price", "quote", "billing", "backend", "selected_implementation", "target", "locked",
	}
	if !reflect.DeepEqual(contract.ForbiddenProviderState, wantForbidden) {
		return errors.New("portable host forbidden provider state list drifted")
	}
	return nil
}

func connectionProbeReferencesTarget(probe ConnectionProbeInput) bool {
	connections, ok := probe.Desired["connections"].(map[string]any)
	if !ok || len(connections) != 1 {
		return false
	}
	for _, value := range connections {
		connection, ok := value.(map[string]any)
		if !ok {
			return false
		}
		return connection["resource"] == probe.TargetKind+"/"+probe.TargetName
	}
	return false
}

// RunnerEvidenceDigest binds the neutral runner subject to every portable
// contract input that changes black-box lifecycle behavior. It authenticates
// no execution result; callers must supply and authenticate that separately.
func RunnerEvidenceDigest(contract Contract) (string, error) {
	payload := struct {
		Subject                string                       `json:"subject"`
		Entrypoint             string                       `json:"entrypoint"`
		SelfTest               string                       `json:"selfTest"`
		APIVersion             string                       `json:"apiVersion"`
		DiscoveryPath          string                       `json:"discoveryPath"`
		APIPath                string                       `json:"apiPath"`
		RunnerInput            RunnerInput                  `json:"runnerInput"`
		ResourceVersion        ResourceVersionContract      `json:"resourceVersion"`
		PlanBinding            PlanBindingContract          `json:"planBinding"`
		Idempotency            IdempotencyContract          `json:"idempotency"`
		WireJSON               WireJSONContract             `json:"wireJSON"`
		ConnectionResolution   ConnectionResolutionContract `json:"connectionResolution"`
		Preconditions          map[string]string            `json:"preconditions"`
		IdempotentOperations   []string                     `json:"idempotentOperations"`
		ValidationErrorCode    string                       `json:"validationErrorCode"`
		StableErrorCodes       []string                     `json:"stableErrorCodes"`
		RetryableCodes         []string                     `json:"retryableCodes"`
		RequiredRunnerChecks   []string                     `json:"requiredRunnerChecks"`
		InterfaceDeclarations  InterfaceDeclarations        `json:"interfaceDeclarations"`
		ForbiddenProviderState []string                     `json:"forbiddenProviderState"`
	}{
		Subject:                contract.RunnerEvidence.Subject,
		Entrypoint:             contract.RunnerEvidence.Entrypoint,
		SelfTest:               contract.RunnerEvidence.SelfTest,
		APIVersion:             contract.APIVersion,
		DiscoveryPath:          contract.DiscoveryPath,
		APIPath:                contract.APIPath,
		RunnerInput:            contract.RunnerInput,
		ResourceVersion:        contract.ResourceVersion,
		PlanBinding:            contract.PlanBinding,
		Idempotency:            contract.Idempotency,
		WireJSON:               contract.WireJSON,
		ConnectionResolution:   contract.ConnectionResolution,
		Preconditions:          contract.Preconditions,
		IdempotentOperations:   contract.IdempotentOperations,
		ValidationErrorCode:    contract.ValidationErrorCode,
		StableErrorCodes:       contract.StableErrorCodes,
		RetryableCodes:         contract.RetryableCodes,
		RequiredRunnerChecks:   contract.RequiredRunnerChecks,
		InterfaceDeclarations:  contract.InterfaceDeclarations,
		ForbiddenProviderState: contract.ForbiddenProviderState,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func decodeStrict(path string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := formpackage.DecodeStrictIJSON(raw, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func hydrateRunnerFixtures(root string, contract *Contract) error {
	if contract == nil {
		return errors.New("portable host contract is nil")
	}
	conformanceRoot, err := filepath.Abs(filepath.Dir(root))
	if err != nil {
		return err
	}
	realConformanceRoot, err := filepath.EvalSymlinks(conformanceRoot)
	if err != nil {
		return err
	}
	packageRoot := ""
	for index := range contract.RunnerInput.NegativeFixtures {
		fixture := &contract.RunnerInput.NegativeFixtures[index]
		if filepath.IsAbs(fixture.Path) || filepath.ToSlash(filepath.Clean(fixture.Path)) != fixture.Path {
			return fmt.Errorf("portable host runner fixture %q path is not a clean relative path", fixture.Name)
		}
		source := filepath.Join(root, filepath.FromSlash(fixture.Path))
		absoluteSource, err := filepath.Abs(source)
		if err != nil {
			return err
		}
		realSource, err := filepath.EvalSymlinks(absoluteSource)
		if err != nil {
			return fmt.Errorf("portable host runner fixture %q: %w", fixture.Name, err)
		}
		relative, err := filepath.Rel(realConformanceRoot, realSource)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("portable host runner fixture %q escapes the conformance corpus", fixture.Name)
		}
		info, err := os.Stat(realSource)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("portable host runner fixture %q is not a regular file", fixture.Name)
		}
		raw, err := os.ReadFile(realSource)
		if err != nil {
			return err
		}
		if formpackage.DigestBytes(raw) != fixture.SHA256 {
			return fmt.Errorf("portable host runner fixture %q byte digest drifted", fixture.Name)
		}
		if err := decodeStrictBytes(raw, &fixture.Input); err != nil {
			return fmt.Errorf("portable host runner fixture %q: %w", fixture.Name, err)
		}
		currentPackageRoot := filepath.Dir(filepath.Dir(realSource))
		if packageRoot == "" {
			packageRoot = currentPackageRoot
		} else if currentPackageRoot != packageRoot {
			return errors.New("portable host runner desired fixtures do not come from one exact Form Package")
		}
	}
	if packageRoot == "" {
		return errors.New("portable host runner desired fixture inventory is empty")
	}
	definitionRaw, err := os.ReadFile(filepath.Join(packageRoot, "definition.json"))
	if err != nil {
		return err
	}
	definition, err := formpackage.ValidateDefinition(definitionRaw)
	if err != nil {
		return fmt.Errorf("portable host runner Form Definition: %w", err)
	}
	identity := contract.RunnerInput.Identity.FormRef
	definitionDigest, err := formpackage.DigestCanonicalJSON(definitionRaw)
	if err != nil {
		return err
	}
	if definition.APIVersion != identity.APIVersion || definition.Kind != identity.Kind ||
		definition.DefinitionVersion != identity.DefinitionVersion || definitionDigest != identity.SchemaDigest {
		return errors.New("portable host runner Form Definition differs from the exact FormRef")
	}
	desiredFixtures := make([]formpackage.NegativeFixture, 0, len(definition.NegativeFixtures))
	for _, fixture := range definition.NegativeFixtures {
		if fixture.Stage == "desired" {
			desiredFixtures = append(desiredFixtures, fixture)
		}
	}
	if len(desiredFixtures) != len(contract.RunnerInput.NegativeFixtures) {
		return errors.New("portable host runner does not inventory every exact desired-stage negative fixture")
	}
	for index, fixture := range desiredFixtures {
		inventory := contract.RunnerInput.NegativeFixtures[index]
		wantPath, err := filepath.EvalSymlinks(filepath.Join(packageRoot, filepath.FromSlash(fixture.InputPath)))
		if err != nil {
			return err
		}
		gotPathInput, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(inventory.Path)))
		if err != nil {
			return err
		}
		gotPath, err := filepath.EvalSymlinks(gotPathInput)
		if err != nil {
			return err
		}
		if fixture.Name != inventory.Name || fixture.Stage != inventory.Stage || wantPath != gotPath {
			return errors.New("portable host runner desired fixture inventory differs from the exact Form Definition")
		}
	}
	contract.RunnerInput.Definition = definition
	return nil
}
