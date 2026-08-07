// Package portableconformancev3 is the Host API v1alpha3 conformance lane:
// a deterministic reference host speaking the complete v1alpha3 wire, a
// black-box runner, and the loader for the conformance/portable-host-v3
// corpus. Passing this lane is local implementation evidence only; it is
// never publication, admission, support, or maturity evidence.
package portableconformancev3

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// Wire identity of the v1alpha3 lane. These mirror internal/clientv3 without
// importing it, so the conformance lane stays black-box with respect to the
// client implementation.
const (
	ContractFormat = "takoform.portable-host-conformance@v3"
	ManifestFormat = "takoform.portable-host-conformance-manifest@v3"
	APIVersion     = "forms.takoform.com/v1alpha3"
	DiscoveryPath  = "/.well-known/takoform/v1alpha3"
	APIPath        = "/apis/forms.takoform.com/v1alpha3"
)

// stableErrorHTTPStatusByCode is the closed 26-code v1alpha3 taxonomy,
// exactly spec/host-api/operations-v1alpha3.json errorEnvelope.
var stableErrorHTTPStatusByCode = map[string]int{
	"invalid_argument":       http.StatusBadRequest,
	"unauthenticated":        http.StatusUnauthorized,
	"permission_denied":      http.StatusForbidden,
	"form_unknown":           http.StatusNotFound,
	"form_not_installed":     http.StatusConflict,
	"form_unavailable":       http.StatusConflict,
	"form_identity_conflict": http.StatusConflict,
	"resource_not_found":     http.StatusNotFound,
	"resource_busy":          http.StatusConflict,
	"import_conflict":        http.StatusConflict,
	"policy_denied":          http.StatusForbidden,
	"backend_unavailable":    http.StatusServiceUnavailable,
	"internal_error":         http.StatusInternalServerError,
	"rate_limited":           http.StatusTooManyRequests,
	"deadline_exceeded":      http.StatusGatewayTimeout,
	"operation_cancelled":    http.StatusConflict,
	"operation_not_found":    http.StatusNotFound,
	"dependency_in_use":      http.StatusConflict,
	"deletion_protected":     http.StatusConflict,
	"artifact_missing":       http.StatusNotFound,
	"artifact_invalid":       http.StatusBadRequest,
	"unsupported_capability": http.StatusUnprocessableEntity,
	"migration_required":     http.StatusConflict,
	"uid_mismatch":           http.StatusConflict,
	"revision_conflict":      http.StatusPreconditionFailed,
	"generation_conflict":    http.StatusPreconditionFailed,
}

// stableErrorCodeOrder is the canonical contract ordering of the taxonomy.
var stableErrorCodeOrder = []string{
	"invalid_argument", "unauthenticated", "permission_denied", "form_unknown",
	"form_not_installed", "form_unavailable", "form_identity_conflict",
	"resource_not_found", "resource_busy", "import_conflict", "policy_denied",
	"backend_unavailable", "internal_error", "rate_limited",
	"deadline_exceeded", "operation_cancelled", "operation_not_found",
	"dependency_in_use", "deletion_protected", "artifact_missing",
	"artifact_invalid", "unsupported_capability", "migration_required",
	"uid_mismatch", "revision_conflict", "generation_conflict",
}

// automaticallyRetryableCodes are the only codes that may carry
// retryable: true.
var automaticallyRetryableCodes = []string{
	"resource_busy", "backend_unavailable", "rate_limited", "deadline_exceeded",
}

func isAutomaticallyRetryable(code string) bool {
	for _, candidate := range automaticallyRetryableCodes {
		if candidate == code {
			return true
		}
	}
	return false
}

// portableConditionReasons is the closed portable condition reason
// vocabulary of $defs/condition/properties/reason in
// spec/schemas/host-api-wire-v1alpha3.schema.json. Two conforming hosts must
// name the same state with the same reason; host-specific detail belongs in
// the free-form hostReason.
var portableConditionReasons = []string{
	"Available",
	"Provisioning",
	"Reconciling",
	"Failed",
	"BackendUnavailable",
	"SpecDrift",
	"ExternalChange",
	"DependencyMissing",
	"DependencyInUse",
	"PolicyDenied",
	"UnsupportedCapability",
	"Deleting",
}

func isPortableConditionReason(reason string) bool {
	for _, candidate := range portableConditionReasons {
		if candidate == reason {
			return true
		}
	}
	return false
}

// requiredRunnerChecks is the closed 50-entry executed-check list every v3
// runner invocation must complete.
var requiredRunnerChecks = []string{
	"discovery-exact",
	"forms-exact-availability",
	"form-definition-exact",
	"validate-accepts-canonical",
	"validate-rejects-negative-fixtures",
	"prepare-binds-exact-spec",
	"prepare-substitution-rejected",
	"apply-create-uid-minted",
	"apply-headers-required",
	"apply-idempotency-replay",
	"create-conflict-when-exists",
	"update-generation-fence",
	"stale-generation-rejected",
	"revision-etag-exact",
	"observe-fence-exact",
	"status-change-bumps-revision-not-generation",
	"spec-change-bumps-generation",
	"delete-revision-fence",
	"stale-revision-rejected",
	"delete-then-recreate-uid-changes",
	"expected-uid-mismatch-rejected",
	"package-digest-not-identity",
	"same-kind-two-groups",
	"revision-role-update-rejected",
	"no-update-spec-change-rejected",
	"binding-target-missing-404-before-mutation",
	"dependency-in-use-on-bound-target-delete",
	"async-operation-flow",
	"operation-replay-terminal",
	"operation-cancel",
	"artifact-upload-missing-blob",
	"artifact-digest-mismatch",
	"artifact-commit-idempotent",
	"artifact-then-bundle-apply",
	"support-profiles-present",
	"error-envelope-taxonomy",
	"cross-space-isolation",
	"cross-principal-idempotency-isolation",
	"prepare-requires-update-fence",
	"stale-prepare-rejected",
	"condition-reason-closed",
	"space-id-grammar-enforced",
	"concurrent-unrelated-mutation",
	"async-commit-revalidates",
	"import-adopts-native-resource",
	"import-validates-like-apply",
	"deployment-weight-sum-enforced",
	"handler-gated-attachments",
	"artifact-manifest-reject-list",
	"artifact-commit-binds-declared-size",
}

// FormRef is the exact four-field v1alpha3 Form identity.
type FormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
}

// InstalledFormReference pins one exact FormRef together with the package
// digest audit evidence the host recorded at installation.
type InstalledFormReference struct {
	FormRef       FormRef `json:"formRef"`
	PackageDigest string  `json:"packageDigest"`
}

// ResourceProbe is one pinned probe resource the runner drives through the
// complete lifecycle. LifecycleCapabilities pins the EXACT capability set the
// host must advertise for this exact Form, so the runner never has to assume
// which operations a Form ought to support.
type ResourceProbe struct {
	Name                  string                 `json:"name"`
	Identity              InstalledFormReference `json:"identity"`
	LifecycleCapabilities []string               `json:"lifecycleCapabilities"`
	Desired               map[string]any         `json:"desired"`
}

// WorkerBundleProbe additionally carries the exact module source bytes the
// runner uploads through the artifact API; the probe desired modules pin
// their sha256.
type WorkerBundleProbe struct {
	ResourceProbe
	ModuleSource string `json:"moduleSource"`
}

// NegativeFixture is exact desired-request evidence hydrated from a byte-
// pinned file under the corpus directory.
type NegativeFixture struct {
	Name   string         `json:"name"`
	Kind   string         `json:"kind"`
	Stage  string         `json:"stage"`
	Path   string         `json:"path"`
	SHA256 string         `json:"sha256"`
	Input  map[string]any `json:"-"`
}

// NameVersion selects one support profile by display identity.
type NameVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// SupportProbes names the interface and binding support profiles the runner
// must be able to read.
type SupportProbes struct {
	Interface NameVersion `json:"interface"`
	Binding   NameVersion `json:"binding"`
}

// RunnerInput is the pinned black-box input of one v3 conformance run.
type RunnerInput struct {
	Space                string            `json:"space"`
	AlternateSpace       string            `json:"alternateSpace"`
	ModuleWorker         ResourceProbe     `json:"moduleWorker"`
	EdgeKvNamespace      ResourceProbe     `json:"edgeKvNamespace"`
	AtLeastOnceQueue     ResourceProbe     `json:"atLeastOnceQueue"`
	WorkerVersion        ResourceProbe     `json:"workerVersion"`
	WorkerBundle         WorkerBundleProbe `json:"workerBundle"`
	WorkerDeployment     ResourceProbe     `json:"workerDeployment"`
	WorkerCronTrigger    ResourceProbe     `json:"workerCronTrigger"`
	QueueConsumer        ResourceProbe     `json:"queueConsumer"`
	SyntheticSecondGroup FormRef           `json:"syntheticSecondGroup"`
	SupportProbes        SupportProbes     `json:"supportProbes"`
	NegativeFixtures     []NegativeFixture `json:"negativeFixtures"`
}

// IdentityContract carries the normative uid/generation/revision semantics
// strings.
type IdentityContract struct {
	UID           string `json:"uid"`
	Generation    string `json:"generation"`
	Revision      string `json:"revision"`
	PackageDigest string `json:"packageDigest"`
}

// ErrorEnvelopeContract pins the closed error taxonomy.
type ErrorEnvelopeContract struct {
	Codes                  []string       `json:"codes"`
	AutomaticallyRetryable []string       `json:"automaticallyRetryable"`
	HTTPStatusByCode       map[string]int `json:"httpStatusByCode"`
}

// Contract is the verified conformance/portable-host-v3 contract.
type Contract struct {
	Format               string                `json:"format"`
	APIVersion           string                `json:"apiVersion"`
	DiscoveryPath        string                `json:"discoveryPath"`
	APIPath              string                `json:"apiPath"`
	Identity             IdentityContract      `json:"identity"`
	ErrorEnvelope        ErrorEnvelopeContract `json:"errorEnvelope"`
	RunnerInput          RunnerInput           `json:"runnerInput"`
	RequiredRunnerChecks []string              `json:"requiredRunnerChecks"`

	// root is the verified corpus directory; it is derived, never decoded.
	root string
}

// Root returns the corpus directory this contract was verified from.
func (c Contract) Root() string { return c.root }

type manifest struct {
	Format   string `json:"format"`
	Contract string `json:"contract"`
	SHA256   string `json:"sha256"`
}

var (
	resourceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	uidPattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	spacePattern        = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
)

// Verify loads and fail-closed-verifies a portable-host-v3 corpus: manifest
// identity, contract byte digest, strict I-JSON decoding, fixture bytes, and
// the pinned identity/taxonomy/check surfaces.
func Verify(root string) (Contract, error) {
	var index manifest
	if err := decodeStrictFile(filepath.Join(root, "manifest.json"), &index); err != nil {
		return Contract{}, err
	}
	if index.Format != ManifestFormat || index.Contract != "contract.json" {
		return Contract{}, errors.New("portable host v3 conformance manifest identity is invalid")
	}
	contractPath := filepath.Join(root, index.Contract)
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		return Contract{}, err
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != index.SHA256 {
		return Contract{}, errors.New("portable host v3 conformance contract digest drifted")
	}
	var contract Contract
	if err := formpackage.DecodeStrictIJSON(raw, &contract); err != nil {
		return Contract{}, fmt.Errorf("decode %s: %w", contractPath, err)
	}
	contract.root = root
	if err := hydrateNegativeFixtures(root, &contract); err != nil {
		return Contract{}, err
	}
	if err := validateContract(contract); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func validateContract(contract Contract) error {
	if contract.Format != ContractFormat ||
		contract.APIVersion != APIVersion ||
		contract.DiscoveryPath != DiscoveryPath ||
		contract.APIPath != APIPath {
		return errors.New("portable host v3 contract identity is invalid")
	}
	wantIdentity := IdentityContract{
		UID:           "host-issued-immutable; delete-and-recreate-changes-uid",
		Generation:    "increments-only-on-portable-desired-spec-change; fence=expectedGeneration-or-Takoform-Expected-Generation; stale=generation_conflict/412",
		Revision:      "increments-on-any-representation-change; etag=exactly-one-strong-quoted-revision; fence=If-Match; stale=revision_conflict/412",
		PackageDigest: "audit-evidence-only; never-identity-query-or-fence",
	}
	if contract.Identity != wantIdentity {
		return errors.New("portable host v3 identity contract drifted")
	}
	if !reflect.DeepEqual(contract.ErrorEnvelope.Codes, stableErrorCodeOrder) {
		return errors.New("portable host v3 stable error taxonomy drifted")
	}
	if !reflect.DeepEqual(contract.ErrorEnvelope.AutomaticallyRetryable, automaticallyRetryableCodes) {
		return errors.New("portable host v3 retryable taxonomy drifted")
	}
	if !reflect.DeepEqual(contract.ErrorEnvelope.HTTPStatusByCode, stableErrorHTTPStatusByCode) {
		return errors.New("portable host v3 error HTTP status map drifted")
	}
	if !reflect.DeepEqual(contract.RequiredRunnerChecks, requiredRunnerChecks) {
		return errors.New("portable host v3 required runner checks drifted")
	}
	input := contract.RunnerInput
	if !spacePattern.MatchString(input.Space) || !spacePattern.MatchString(input.AlternateSpace) ||
		input.Space == input.AlternateSpace {
		return errors.New("portable host v3 runner spaces are invalid")
	}
	probes := []struct {
		label string
		probe ResourceProbe
		kind  string
	}{
		{"moduleWorker", input.ModuleWorker, "ModuleWorker"},
		{"edgeKvNamespace", input.EdgeKvNamespace, "EdgeKVNamespace"},
		{"atLeastOnceQueue", input.AtLeastOnceQueue, "AtLeastOnceQueue"},
		{"workerVersion", input.WorkerVersion, "WorkerVersion"},
		{"workerBundle", input.WorkerBundle.ResourceProbe, "WorkerBundle"},
		{"workerDeployment", input.WorkerDeployment, "WorkerDeployment"},
		{"workerCronTrigger", input.WorkerCronTrigger, "WorkerCronTrigger"},
		{"queueConsumer", input.QueueConsumer, "QueueConsumer"},
	}
	for _, entry := range probes {
		if err := validateProbe(entry.label, entry.probe, entry.kind); err != nil {
			return err
		}
	}
	if len(input.AtLeastOnceQueue.Desired) == 0 {
		return errors.New("portable host v3 queue probe desired must not be empty")
	}
	if err := validateWorkerVersionProbe(input); err != nil {
		return err
	}
	if err := validateWorkerBundleProbe(input.WorkerBundle); err != nil {
		return err
	}
	if err := validateCrossResourceProbes(input); err != nil {
		return err
	}
	synthetic := input.SyntheticSecondGroup
	if synthetic.APIVersion == input.EdgeKvNamespace.Identity.FormRef.APIVersion ||
		!strings.Contains(synthetic.APIVersion, ".") ||
		synthetic.Kind != "EdgeKVNamespace" ||
		synthetic.DefinitionVersion == "" ||
		!formpackage.ValidDigest(synthetic.SchemaDigest) {
		return errors.New("portable host v3 synthetic second-group FormRef is invalid")
	}
	if input.SupportProbes.Interface.Name == "" || input.SupportProbes.Interface.Version == "" ||
		input.SupportProbes.Binding.Name == "" || input.SupportProbes.Binding.Version == "" {
		return errors.New("portable host v3 support probes are incomplete")
	}
	if err := validateNegativeFixtureInventory(input); err != nil {
		return err
	}
	return nil
}

func validateProbe(label string, probe ResourceProbe, kind string) error {
	if !resourceNamePattern.MatchString(probe.Name) {
		return fmt.Errorf("portable host v3 %s probe name is invalid", label)
	}
	ref := probe.Identity.FormRef
	refRaw, err := json.Marshal(ref)
	if err != nil {
		return fmt.Errorf("portable host v3 %s probe FormRef: %w", label, err)
	}
	if _, err := formpackage.ValidateFormRef(refRaw); err != nil {
		return fmt.Errorf("portable host v3 %s probe FormRef: %w", label, err)
	}
	if ref.Kind != kind {
		return fmt.Errorf("portable host v3 %s probe kind must be %s", label, kind)
	}
	if !formpackage.ValidDigest(probe.Identity.PackageDigest) {
		return fmt.Errorf("portable host v3 %s probe packageDigest is invalid", label)
	}
	if probe.Desired == nil {
		return fmt.Errorf("portable host v3 %s probe desired must be present", label)
	}
	return validateProbeCapabilities(label, probe.LifecycleCapabilities)
}

// baseCapabilities is the closed set every v1alpha3 Form must advertise; the
// only permitted addition is update. refresh is not a v1alpha3 capability.
var baseCapabilities = []string{"create", "read", "delete", "import", "observe"}

func validateProbeCapabilities(label string, capabilities []string) error {
	if len(capabilities) == 0 {
		return fmt.Errorf("portable host v3 %s probe must pin its exact lifecycle capabilities", label)
	}
	seen := map[string]bool{}
	for _, capability := range capabilities {
		switch capability {
		case "create", "read", "update", "delete", "import", "observe":
		case "refresh":
			return fmt.Errorf("portable host v3 %s probe pins refresh; the v1alpha3 lane has no refresh capability", label)
		default:
			return fmt.Errorf("portable host v3 %s probe pins unknown capability %q", label, capability)
		}
		if seen[capability] {
			return fmt.Errorf("portable host v3 %s probe pins duplicate capability %q", label, capability)
		}
		seen[capability] = true
	}
	for _, capability := range baseCapabilities {
		if !seen[capability] {
			return fmt.Errorf("portable host v3 %s probe omits the mandatory capability %q", label, capability)
		}
	}
	return nil
}

func validateWorkerVersionProbe(input RunnerInput) error {
	desired := input.WorkerVersion.Desired
	if nestedName(desired, "worker") != input.ModuleWorker.Name {
		return errors.New("portable host v3 workerVersion probe must reference the moduleWorker probe")
	}
	if nestedName(desired, "bundle") != input.WorkerBundle.Name {
		return errors.New("portable host v3 workerVersion probe must reference the workerBundle probe")
	}
	bindings, _ := desired["kvBindings"].([]any)
	if len(bindings) != 1 {
		return errors.New("portable host v3 workerVersion probe must carry exactly one kvBinding")
	}
	binding, _ := bindings[0].(map[string]any)
	resource, _ := binding["resource"].(map[string]any)
	if resource == nil || resource["kind"] != "EdgeKVNamespace" ||
		resource["name"] != input.EdgeKvNamespace.Name {
		return errors.New("portable host v3 workerVersion kvBinding must reference the edgeKvNamespace probe")
	}
	if value, _ := desired["compatibilityDate"].(string); value == "" {
		return errors.New("portable host v3 workerVersion probe must pin a compatibilityDate")
	}
	return nil
}

func validateWorkerBundleProbe(probe WorkerBundleProbe) error {
	if probe.ModuleSource == "" {
		return errors.New("portable host v3 workerBundle probe moduleSource is empty")
	}
	modules, _ := probe.Desired["modules"].([]any)
	if len(modules) != 1 {
		return errors.New("portable host v3 workerBundle probe must declare exactly one module")
	}
	module, _ := modules[0].(map[string]any)
	if module == nil {
		return errors.New("portable host v3 workerBundle probe module is invalid")
	}
	mainModule, _ := probe.Desired["mainModule"].(string)
	if name, _ := module["name"].(string); mainModule == "" || name != mainModule {
		return errors.New("portable host v3 workerBundle mainModule must name its one module")
	}
	digest, _ := module["digest"].(string)
	if digest != formpackage.DigestBytes([]byte(probe.ModuleSource)) {
		return errors.New("portable host v3 workerBundle module digest does not match moduleSource bytes")
	}
	size, ok := module["size"].(json.Number)
	if !ok || size.String() != fmt.Sprintf("%d", len(probe.ModuleSource)) {
		return errors.New("portable host v3 workerBundle module size does not match moduleSource bytes")
	}
	return nil
}

// validateCrossResourceProbes pins the three probes whose portable rules are
// cross-resource semantics a schema cannot express: the deployment weight
// sum, and the two handler-gated attachments.
func validateCrossResourceProbes(input RunnerInput) error {
	deployment := input.WorkerDeployment.Desired
	if nestedName(deployment, "worker") != input.ModuleWorker.Name {
		return errors.New("portable host v3 workerDeployment probe must reference the moduleWorker probe")
	}
	versions, _ := deployment["versions"].([]any)
	if len(versions) != 1 {
		return errors.New("portable host v3 workerDeployment probe must carry exactly one weighted version")
	}
	entry, _ := versions[0].(map[string]any)
	if nestedName(entry, "workerVersion") != input.WorkerVersion.Name {
		return errors.New("portable host v3 workerDeployment probe must weight the workerVersion probe")
	}
	weight, ok := entry["weight"].(json.Number)
	if !ok || weight.String() != "10000" {
		return errors.New("portable host v3 workerDeployment probe must declare the exact 10000 basis-point sum")
	}
	if nestedName(input.WorkerCronTrigger.Desired, "worker") != input.ModuleWorker.Name {
		return errors.New("portable host v3 workerCronTrigger probe must reference the moduleWorker probe")
	}
	if cron, _ := input.WorkerCronTrigger.Desired["cron"].(string); cron == "" {
		return errors.New("portable host v3 workerCronTrigger probe must pin a cron expression")
	}
	if nestedName(input.QueueConsumer.Desired, "worker") != input.ModuleWorker.Name {
		return errors.New("portable host v3 queueConsumer probe must reference the moduleWorker probe")
	}
	if nestedName(input.QueueConsumer.Desired, "queue") != input.AtLeastOnceQueue.Name {
		return errors.New("portable host v3 queueConsumer probe must reference the atLeastOnceQueue probe")
	}
	return nil
}

func validateNegativeFixtureInventory(input RunnerInput) error {
	if len(input.NegativeFixtures) < 2 {
		return errors.New("portable host v3 negative fixture inventory is incomplete")
	}
	names := map[string]string{}
	paths := map[string]bool{}
	for _, fixture := range input.NegativeFixtures {
		if fixture.Name == "" || fixture.Stage != "desired" || fixture.Path == "" ||
			!formpackage.ValidDigest(fixture.SHA256) || len(fixture.Input) == 0 ||
			names[fixture.Name] != "" || paths[fixture.Path] {
			return errors.New("portable host v3 negative fixture inventory is invalid")
		}
		switch fixture.Kind {
		case input.ModuleWorker.Identity.FormRef.Kind,
			input.EdgeKvNamespace.Identity.FormRef.Kind,
			input.AtLeastOnceQueue.Identity.FormRef.Kind,
			input.WorkerVersion.Identity.FormRef.Kind,
			input.WorkerBundle.Identity.FormRef.Kind:
		default:
			return fmt.Errorf("portable host v3 negative fixture %q names an unknown probe kind", fixture.Name)
		}
		names[fixture.Name] = fixture.Kind
		paths[fixture.Path] = true
	}
	if names["reject-unexpected-property"] != "ModuleWorker" {
		return errors.New("portable host v3 must pin reject-unexpected-property for ModuleWorker")
	}
	if names["reject-bad-retention"] != "AtLeastOnceQueue" {
		return errors.New("portable host v3 must pin reject-bad-retention for AtLeastOnceQueue")
	}
	return nil
}

func nestedName(desired map[string]any, member string) string {
	object, _ := desired[member].(map[string]any)
	if object == nil {
		return ""
	}
	name, _ := object["name"].(string)
	return name
}

// hydrateNegativeFixtures loads fixture inputs from the corpus, verifying
// path containment and exact byte digests before strict decoding.
func hydrateNegativeFixtures(root string, contract *Contract) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	absoluteRoot, err := filepath.Abs(realRoot)
	if err != nil {
		return err
	}
	for index := range contract.RunnerInput.NegativeFixtures {
		fixture := &contract.RunnerInput.NegativeFixtures[index]
		if filepath.IsAbs(fixture.Path) || filepath.ToSlash(filepath.Clean(fixture.Path)) != fixture.Path {
			return fmt.Errorf("portable host v3 fixture %q path is not a clean relative path", fixture.Name)
		}
		source := filepath.Join(root, filepath.FromSlash(fixture.Path))
		realSource, err := filepath.EvalSymlinks(source)
		if err != nil {
			return fmt.Errorf("portable host v3 fixture %q: %w", fixture.Name, err)
		}
		absoluteSource, err := filepath.Abs(realSource)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(absoluteRoot, absoluteSource)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("portable host v3 fixture %q escapes the conformance corpus", fixture.Name)
		}
		info, err := os.Stat(absoluteSource)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("portable host v3 fixture %q is not a regular file", fixture.Name)
		}
		raw, err := os.ReadFile(absoluteSource)
		if err != nil {
			return err
		}
		if formpackage.DigestBytes(raw) != fixture.SHA256 {
			return fmt.Errorf("portable host v3 fixture %q byte digest drifted", fixture.Name)
		}
		if err := formpackage.DecodeStrictIJSON(raw, &fixture.Input); err != nil {
			return fmt.Errorf("portable host v3 fixture %q: %w", fixture.Name, err)
		}
	}
	return nil
}

func decodeStrictFile(path string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := formpackage.DecodeStrictIJSON(raw, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
