// Package portableconformancev3 is the Host API v1alpha3 conformance lane:
// a deterministic reference host speaking the complete v1alpha3 wire, a
// black-box runner, and the loader for the conformance/portable-host-v3
// corpus. Passing this lane is local implementation evidence only; it is
// never publication, admission, support, or maturity evidence.
package portableconformancev3

import (
	"bytes"
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
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

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

// requiredRunnerChecks is the closed 103-entry executed-check list every v3
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
	"relation-target-missing-rejected",
	"relation-target-deletion-blocked",
	"relation-incarnation-change-detected",
	"relation-reapply-repins",
	"binding-contract-verified",
	"async-operation-flow",
	"operation-replay-terminal",
	"operation-resumable-after-settlement",
	"operation-cancel",
	"exact-form-ref-fails-closed-on-unknown-definition",
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
	"async-commit-binds-the-accepted-identity",
	"import-adopts-native-resource",
	"import-validates-like-apply",
	"deployment-weight-sum-enforced",
	"deployment-single-active-per-worker",
	"deployment-version-ownership",
	"deployment-version-duplicate-rejected",
	"attachment-requires-active-deployment",
	"handler-gated-attachments",
	"binding-name-collision-rejected",
	"deployment-change-preserves-dependents",
	"deployment-delete-blocked-by-dependent",
	"deployment-delete-blocked-by-inbound-binding",
	"dependent-revision-advances-with-rendering",
	"artifact-manifest-reject-list",
	"artifact-commit-binds-declared-size",
	"artifact-manifest-kind-exclusive",
	"bundle-main-module-is-loadable",
	"artifact-retention-while-referenced",
	// Operability behind ordinary infrastructure (spec/decisions/0018).
	"namespaced-group-travels-as-two-path-segments",
	"operation-bound-to-its-creating-principal",
	"upload-session-bound-to-its-creating-principal",
	"artifact-digest-is-not-a-capability",
	"manifest-reference-is-not-a-capability",
	// The ES Module Worker ABI is an exact contract (spec/decisions/0019).
	"module-worker-runtime-contract-advertised",
	"undeclared-runtime-handler-rejected",
	"declared-handler-not-exported-rejected",
	// The edge Interfaces state their data and delivery model
	// (spec/decisions/0020).
	"edge-interface-contracts-advertised",
	"cron-grammar-enforced",
	"queue-single-consumer-enforced",
	// An attachment's claim is decided on canonical, resolved identity
	// (spec/decisions/0026).
	"custom-domain-hostname-canonicalized",
	"custom-domain-hostname-claim-unique",
	"dead-letter-cycle-rejected",
	// A host answers for the exact ref recorded in state, and a relation pins
	// the target's contract rather than its incarnation alone
	// (spec/decisions/0022).
	"two-definition-versions-answer-independently",
	"resource-answers-only-under-its-recorded-form-ref",
	"relation-target-form-ref-verified",
	"relation-target-interface-verified",
	"relation-pin-records-target-form-ref",
	// A worker is reachable over HTTPS at an address the host assigns
	// (spec/decisions/0024).
	"worker-endpoint-address-is-host-assigned",
	"worker-endpoint-single-per-worker",
	"worker-endpoint-follows-the-active-deployment",
	"worker-endpoint-address-is-stable-for-its-uid",
	// Declared outputs are a contract in both directions
	// (spec/decisions/0025).
	"form-declared-outputs-are-exact",
	// The resource plane is tenant-isolated (spec/decisions/0028). Every one of
	// these is driven black box, with the two credentials the runner input
	// already carries: a runner that cannot authenticate as two tenants cannot
	// measure any of them. The list is enumerated by SURFACE and bound to
	// nameAddressedResourceSurfaces, so a name-addressed endpoint cannot be
	// added without one.
	"resource-address-is-tenant-scoped",
	"resource-read-is-tenant-isolated",
	"resource-observe-is-tenant-isolated",
	"resource-update-is-tenant-isolated",
	"resource-import-is-tenant-isolated",
	"resource-delete-is-tenant-isolated",
	"relation-resolution-is-tenant-scoped",
	"prepare-is-tenant-scoped",
	"idempotency-is-tenant-scoped",
}

// nameAddressedResourceSurface is one v1alpha3 surface that takes a resource
// NAME, and what measures its tenant boundary.
//
// The tenant matrix was first enumerated by INTENT — read, update, delete,
// relations, reviews, replay — and an enumeration by intent is complete only
// until someone thinks of another intent. Two surfaces that take a name were
// missed that way: `observe`, which returns a whole representation, and
// `import`, which mutates. A host that scoped GET, PUT and DELETE while
// resolving either of those host-wide passed the entire matrix.
//
// So the enumeration is by SURFACE, and it is this table. It is bound in three
// directions by tests in tenant_isolation_test.go:
//
//   - to the routes the reference host serves (resourcePlaneHandlers), so a new
//     endpoint cannot be routed without an entry here;
//   - to the Lifecycle route block of spec/host-api/v1alpha3.md, so a new
//     endpoint cannot be published without one either;
//   - to requiredRunnerChecks, so an entry cannot name a check that does not
//     exist, and cannot name none unless it states why.
//
// It deliberately stays in the Go corpus rather than in contract.json: a runner
// consumes the JSON contract at run time and needs none of this, while what the
// table protects is the completeness of the check list this repository ships.
type nameAddressedResourceSurface struct {
	// Method and Route are the surface exactly as the Lifecycle route block of
	// spec/host-api/v1alpha3.md writes it.
	Method string
	Route  string
	// Action is the segment after the name, empty when the name is last.
	Action string
	// NameInBody marks the two surfaces whose name travels in the request
	// document rather than in the path, so they route by their own path and not
	// through resourcePlaneHandlers.
	NameInBody bool
	// TenantChecks are the required checks that hold this surface to the
	// boundary. Empty exactly when Unresolved says why none is owed.
	TenantChecks []string
	// Unresolved states why a surface that carries a name owes no tenant check.
	Unresolved string
}

// nameAddressedResourceSurfaces is the closed set of v1alpha3 surfaces that
// take a resource name.
var nameAddressedResourceSurfaces = []nameAddressedResourceSurface{
	{
		Method: "POST", Route: "{api}/resources/validate", NameInBody: true,
		// validate carries a name and resolves nothing: it answers diagnostics
		// about the document it was handed and never reads the store, so it has
		// no tenant-dependent answer to give and nothing to disclose. A host that
		// made it report "that name is taken" would be adding a diagnostic this
		// lane does not define, and would fail the closed-vocabulary rules first.
		Unresolved: "resolves no stored resource; diagnostics are computed from the submitted document alone",
	},
	{
		Method: "POST", Route: "{api}/resources/prepare", NameInBody: true,
		TenantChecks: []string{"resource-update-is-tenant-isolated", "prepare-is-tenant-scoped"},
	},
	{
		Method: "PUT", Route: "{api}/resources/{formGroup}/{formVersion}/{kind}/{name}",
		TenantChecks: []string{
			"resource-address-is-tenant-scoped",
			"resource-update-is-tenant-isolated",
			"relation-resolution-is-tenant-scoped",
			"idempotency-is-tenant-scoped",
		},
	},
	{
		Method: "GET", Route: "{api}/resources/{formGroup}/{formVersion}/{kind}/{name}",
		TenantChecks: []string{"resource-read-is-tenant-isolated"},
	},
	{
		Method: "POST", Route: "{api}/resources/{formGroup}/{formVersion}/{kind}/{name}/import",
		Action:       "import",
		TenantChecks: []string{"resource-import-is-tenant-isolated"},
	},
	{
		Method: "POST", Route: "{api}/resources/{formGroup}/{formVersion}/{kind}/{name}/observe",
		Action:       "observe",
		TenantChecks: []string{"resource-observe-is-tenant-isolated"},
	},
	{
		Method: "DELETE", Route: "{api}/resources/{formGroup}/{formVersion}/{kind}/{name}",
		TenantChecks: []string{"resource-delete-is-tenant-isolated"},
	},
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
	// DeclaredOutputSchema is this Form's `outputSchema` — the WHOLE closed
	// Draft 2020-12 contract its Definition declares — or absent when it
	// declares none.
	//
	// It is corpus data because the lane cannot read it from the host: the
	// published form-definition response is a closed object carrying identity,
	// display name, description, and desiredSchema, and its bytes are immutable
	// (decision 0014), so there is no wire surface that serves an outputSchema.
	//
	// It carries the schema rather than the member names for the reason the wire
	// rule is stated the way it is: `status.outputs` VALIDATES against the
	// installed Definition's outputSchema, so a runner holding only the names
	// could check that a member came back and nothing about what came back —
	// which admits `hostname: "not a hostname"` and rejects the integer and
	// boolean outputs the authoring model permits.
	//
	// And it is pinned in the corpus rather than derived inside the runner from
	// the Form identity, because the corpus is the digest-pinned artifact a host
	// is measured against. A runner that compiled its own copy of the catalog
	// could hold two hosts to two different contracts under one corpus digest,
	// and the evidence a host publishes would no longer say what it was held to.
	// Corpus and Form cannot drift apart: TestCorpusPinsTheFormsOwnOutputSchema
	// compares these bytes against the installed Definition at the exact pinned
	// FormRef.
	DeclaredOutputSchema map[string]any `json:"declaredOutputSchema,omitempty"`
}

// declaresOutputs reports whether this probe's Form publishes an output
// contract at all. A Form that declares none must OMIT `status.outputs`, not
// return an empty document.
func (probe ResourceProbe) declaresOutputs() bool { return len(probe.DeclaredOutputSchema) > 0 }

// outputContract compiles the pinned `outputSchema` and returns the exact
// member set it declares alongside it.
//
// The member list is DERIVED from the schema's `required` rather than pinned
// beside it: two statements of one fact could disagree, and the schema is the
// one the wire rule names.
func (probe ResourceProbe) outputContract() ([]string, *jsonschema.Schema, error) {
	if !probe.declaresOutputs() {
		return nil, nil, nil
	}
	kind := probe.Identity.FormRef.Kind
	members, err := declaredOutputMembers(probe.DeclaredOutputSchema)
	if err != nil {
		return nil, nil, fmt.Errorf("portable host v3 %s probe declaredOutputSchema: %w", kind, err)
	}
	compiled, err := compileOutputSchema(kind, probe.DeclaredOutputSchema)
	if err != nil {
		return nil, nil, err
	}
	return members, compiled, nil
}

// declaredOutputMembers reads the exact member set out of one output contract,
// refusing a schema that is not the closed, wholly-required object the
// authoring model produces (decision 0025 rule 1).
//
// The refusal matters more than it looks: a corpus that pinned a schema with
// `additionalProperties` open, or with a member missing from `required`, would
// pin a contract that no host could fail — the exact toothless shape the check
// exists to close.
func declaredOutputMembers(schema map[string]any) ([]string, error) {
	if kind, _ := schema["type"].(string); kind != "object" {
		return nil, errors.New("an output contract is an object schema")
	}
	if closed, ok := schema["additionalProperties"].(bool); !ok || closed {
		return nil, errors.New("an output contract is closed: additionalProperties must be false")
	}
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil, errors.New("an output contract declares at least one member")
	}
	// `required` arrives as []any from the corpus bytes and as []string from a
	// schema rendered in process; both are the same statement.
	var required []string
	switch entries := schema["required"].(type) {
	case []any:
		required = anyStringSlice(entries)
	case []string:
		required = entries
	}
	members := make([]string, 0, len(properties))
	seen := map[string]bool{}
	for _, name := range required {
		if name == "" || seen[name] {
			return nil, errors.New("required lists an empty or duplicated member")
		}
		if _, declared := properties[name]; !declared {
			return nil, fmt.Errorf("required names %q, which properties does not declare", name)
		}
		seen[name] = true
		members = append(members, name)
	}
	for name := range properties {
		if !seen[name] {
			return nil, fmt.Errorf(
				"member %q is declared but not required; every declared output is returned, so a host could omit it and still pass",
				name,
			)
		}
	}
	sort.Strings(members)
	return members, nil
}

// compileOutputSchema compiles one pinned output contract for validation.
func compileOutputSchema(kind string, schema map[string]any) (*jsonschema.Schema, error) {
	document, err := jsonSchemaDocument(schema)
	if err != nil {
		return nil, fmt.Errorf("portable host v3 %s output schema: %w", kind, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	id := "https://forms.takoform.com/portable-host-v3/" + kind + ".outputs.schema.json"
	if err := compiler.AddResource(id, document); err != nil {
		return nil, fmt.Errorf("portable host v3 %s output schema: %w", kind, err)
	}
	compiled, err := compiler.Compile(id)
	if err != nil {
		return nil, fmt.Errorf("portable host v3 %s output schema: %w", kind, err)
	}
	return compiled, nil
}

// jsonSchemaDocument re-encodes a decoded JSON value into the form the schema
// library validates against, so numbers keep their exact literal.
func jsonSchemaDocument(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(raw))
}

// WorkerBundleProbe carries what the WorkerBundle probe needs beyond its
// desired state. Its desired state is only a manifest digest, so the corpus
// also pins the exact artifact manifest that digest addresses and the exact
// module source bytes the runner uploads: the runner commits the manifest
// through the artifact API and drives the resource with the digest the host
// returned.
//
// ExportedHandlers is what the default export of ModuleSource actually exposes
// as callable own properties. The `worker.runtime` contract fails a version
// whose declared handler the module does not export (`handler_not_exported`),
// so a corpus that did not say what its own module exports could not tell a
// coherent Worker Version from one no conforming host may store — and every
// check driving such a version would fail exactly the hosts that enforce the
// contract. It is corpus data rather than manifest data because the published
// artifact-manifest schema is closed and admits no such member
// (spec/schemas/artifact-manifest-v1alpha1.schema.json).
type WorkerBundleProbe struct {
	ResourceProbe
	ModuleSource     string         `json:"moduleSource"`
	Manifest         map[string]any `json:"manifest"`
	ExportedHandlers []string       `json:"exportedHandlers"`
}

// ModuleBundleProbe pins one ADDITIONAL Worker Bundle the lane drives. Its Form
// identity, lifecycle capabilities, and desired shape are the workerBundle
// probe's — one Form line, stated once — so only the resource name, the module
// bytes, the manifest addressing them, and what that module exports differ.
//
// The lane needs a second bundle for exactly one reason: proving a host refuses
// a declared-but-unexported handler requires a module that exports LESS than
// the vocabulary, while every other check needs one that exports enough. One
// bundle cannot be both.
type ModuleBundleProbe struct {
	Name             string         `json:"name"`
	ManifestDigest   string         `json:"manifestDigest"`
	Manifest         map[string]any `json:"manifest"`
	ModuleSource     string         `json:"moduleSource"`
	ExportedHandlers []string       `json:"exportedHandlers"`
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

// RuntimeContractProbe pins the exact ES Module Worker runtime ABI the lane
// requires a host to implement (spec/decisions/0019). Unlike the display-only
// support probes, it carries the exact schemaDigest: the whole point of the
// contract is that "this host runs module workers" means one exact set of
// handler signatures, environment rules, exception behavior, and Web APIs, and
// only the digest says which set. UndefinedHandler is a handler token the
// contract does NOT define, used to prove a host refuses one.
type RuntimeContractProbe struct {
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	SchemaDigest     string   `json:"schemaDigest"`
	Handlers         []string `json:"handlers"`
	UndefinedHandler string   `json:"undefinedHandler"`
}

// defines reports whether the pinned contract vocabulary carries one handler.
func (probe RuntimeContractProbe) defines(handler string) bool {
	for _, candidate := range probe.Handlers {
		if candidate == handler {
			return true
		}
	}
	return false
}

// exports reports whether the pinned module's default export exposes one
// handler.
func (probe ModuleBundleProbe) exports(handler string) bool {
	for _, candidate := range probe.ExportedHandlers {
		if candidate == handler {
			return true
		}
	}
	return false
}

// unexportedHandler names the first handler the pinned runtime vocabulary
// defines that this module does NOT export — the one a Worker Version can
// legally declare and no conforming host may store against this bundle. The
// corpus proves one exists, so the empty string is unreachable from a verified
// contract.
func (probe ModuleBundleProbe) unexportedHandler(runtime RuntimeContractProbe) string {
	for _, handler := range runtime.Handlers {
		if !probe.exports(handler) {
			return handler
		}
	}
	return ""
}

// InterfaceContractProbe pins one exact data-plane Interface contract the lane
// requires a host to advertise. Like the runtime ABI probe it carries the
// schemaDigest, because "this host has a KV store" is worth nothing until it
// says WHICH contract that store implements: the consistency model, the value
// model, the closed error vocabulary, and the limits all live in the bytes that
// digest names (spec/decisions/0020).
type InterfaceContractProbe struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	SchemaDigest string `json:"schemaDigest"`
}

// SupportProbes names the interface and binding support profiles the runner
// must be able to read, pins the exact runtime ABI contract the ModuleWorker
// Form must both provide and advertise, and pins the exact data-plane Interface
// contracts the family's storage and queue Forms provide.
type SupportProbes struct {
	Interface       NameVersion              `json:"interface"`
	Binding         NameVersion              `json:"binding"`
	RuntimeContract RuntimeContractProbe     `json:"runtimeContract"`
	DataInterfaces  []InterfaceContractProbe `json:"dataInterfaces"`
}

// SyntheticDefinitionProbe pins a SECOND definition version of one kind the
// corpus already installs, as exact bytes.
//
// The lane needs one for a property no single-version corpus can express: that
// a host answers for the exact ref recorded in state rather than for a kind. A
// second version invented at runtime would prove only that the host agrees with
// whatever the runner made up; a byte-pinned Definition proves the host holds
// two real contracts of one Form line at once and keeps them apart
// (decision 0022).
//
// WithheldInterface names the Interface the FIRST definition version provides
// and this one does not. It is what gives a required-interface relation a live
// target of exactly the right group and kind that truthfully does not satisfy
// the requirement — a target no other corpus resource can be.
type SyntheticDefinitionProbe struct {
	Name              string      `json:"name"`
	FormRef           FormRef     `json:"formRef"`
	Path              string      `json:"path"`
	SHA256            string      `json:"sha256"`
	WithheldInterface NameVersion `json:"withheldInterface"`

	// Definition is hydrated from the byte-pinned corpus file, never decoded
	// from the contract document.
	Definition *formpackage.FormDefinition `json:"-"`
}

// RunnerInput is the pinned black-box input of one v3 conformance run.
type RunnerInput struct {
	Space                            string                   `json:"space"`
	AlternateSpace                   string                   `json:"alternateSpace"`
	ModuleWorker                     ResourceProbe            `json:"moduleWorker"`
	EdgeKvNamespace                  ResourceProbe            `json:"edgeKvNamespace"`
	AtLeastOnceQueue                 ResourceProbe            `json:"atLeastOnceQueue"`
	WorkerVersion                    ResourceProbe            `json:"workerVersion"`
	WorkerBundle                     WorkerBundleProbe        `json:"workerBundle"`
	FetchOnlyBundle                  ModuleBundleProbe        `json:"fetchOnlyBundle"`
	WorkerDeployment                 ResourceProbe            `json:"workerDeployment"`
	WorkerCustomDomain               ResourceProbe            `json:"workerCustomDomain"`
	WorkerEndpoint                   ResourceProbe            `json:"workerEndpoint"`
	WorkerCronTrigger                ResourceProbe            `json:"workerCronTrigger"`
	QueueConsumer                    ResourceProbe            `json:"queueConsumer"`
	SyntheticSecondGroup             FormRef                  `json:"syntheticSecondGroup"`
	SyntheticSecondDefinitionVersion SyntheticDefinitionProbe `json:"syntheticSecondDefinitionVersion"`
	SupportProbes                    SupportProbes            `json:"supportProbes"`
	NegativeFixtures                 []NegativeFixture        `json:"negativeFixtures"`
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
	if err := hydrateSyntheticDefinition(root, &contract); err != nil {
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
		{"workerCustomDomain", input.WorkerCustomDomain, "WorkerCustomDomain"},
		{"workerEndpoint", input.WorkerEndpoint, "WorkerEndpoint"},
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
	// The runtime contract is verified first, and the bundles before the version:
	// what a version may declare is decided by the ABI vocabulary and by what the
	// module it references exports, so neither answer may come from an unverified
	// source.
	if err := validateRuntimeContractProbe(input.SupportProbes.RuntimeContract); err != nil {
		return err
	}
	if err := validateDataInterfaceProbes(input.SupportProbes); err != nil {
		return err
	}
	if err := validateWorkerBundleProbe(input.WorkerBundle, input.SupportProbes.RuntimeContract); err != nil {
		return err
	}
	if err := validateFetchOnlyBundleProbe(
		input.FetchOnlyBundle, input.WorkerBundle, input.SupportProbes.RuntimeContract,
	); err != nil {
		return err
	}
	if err := validateWorkerVersionProbe(input); err != nil {
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
	if err := validateSyntheticDefinitionProbe(input); err != nil {
		return err
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

// validateSyntheticDefinitionProbe proves the second definition version is a
// second contract of a Form line the corpus already installs, and that it
// really withholds an Interface the first version provides.
//
// Both halves are load-bearing. A probe that named a different kind would prove
// nothing about one Form line holding two contracts; a probe whose withheld
// Interface the first version never provided would give the required-interface
// check a refusal it would have got anyway.
func validateSyntheticDefinitionProbe(input RunnerInput) error {
	probe := input.SyntheticSecondDefinitionVersion
	first := input.ModuleWorker.Identity.FormRef
	if probe.Name == "" {
		return errors.New("portable host v3 synthetic second definition version must be named")
	}
	refRaw, err := json.Marshal(probe.FormRef)
	if err != nil {
		return fmt.Errorf("portable host v3 synthetic second definition version FormRef: %w", err)
	}
	if _, err := formpackage.ValidateFormRef(refRaw); err != nil {
		return fmt.Errorf("portable host v3 synthetic second definition version FormRef: %w", err)
	}
	if probe.FormRef.APIVersion != first.APIVersion || probe.FormRef.Kind != first.Kind {
		return errors.New(
			"portable host v3 synthetic second definition version must be a second contract of the ModuleWorker line",
		)
	}
	if probe.FormRef.DefinitionVersion == first.DefinitionVersion ||
		probe.FormRef.SchemaDigest == first.SchemaDigest {
		return errors.New(
			"portable host v3 synthetic second definition version is not distinguishable from the installed one",
		)
	}
	if probe.WithheldInterface.Name == "" || probe.WithheldInterface.Version == "" {
		return errors.New("portable host v3 synthetic second definition version must name the Interface it withholds")
	}
	if probe.WithheldInterface.Name != input.SupportProbes.RuntimeContract.Name ||
		probe.WithheldInterface.Version != input.SupportProbes.RuntimeContract.Version {
		return errors.New(
			"portable host v3 synthetic second definition version must withhold the pinned runtime ABI contract, " +
				"which is the one an inward-activation relation requires",
		)
	}
	if probe.Definition == nil {
		return errors.New("portable host v3 synthetic second definition version was not hydrated")
	}
	for _, provided := range probe.Definition.ProvidedInterfaces {
		if provided.Name == probe.WithheldInterface.Name && provided.Version == probe.WithheldInterface.Version {
			return errors.New(
				"portable host v3 synthetic second definition version provides the Interface it claims to withhold",
			)
		}
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
	// The pinned output contract is compiled at LOAD time. A corpus whose output
	// schema does not compile, or that pins a schema no host could fail, must
	// stop the run before it starts rather than pass every host it measures.
	if _, _, err := probe.outputContract(); err != nil {
		return fmt.Errorf("portable host v3 %s probe: %w", label, err)
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
	// A version pins no runtime selector at all: there is no compatibilityDate
	// and no compatibilityFlags in this lane, because a date names no behavior
	// without a registry saying what each date changes (spec/decisions/0019).
	// The runtime is the exact contract the ModuleWorker Form provides.
	for _, removed := range []string{"compatibilityDate", "compatibilityFlags"} {
		if _, present := desired[removed]; present {
			return fmt.Errorf(
				"portable host v3 workerVersion probe carries %s; the runtime is fixed by the exact %s contract, not a token",
				removed, input.SupportProbes.RuntimeContract.Name,
			)
		}
	}
	// The version probe is what the deployment probe weights, and the
	// deployment is what every attachment probe is admitted against
	// (spec/decisions/0016). A corpus whose version declared fewer handlers than
	// its attachments invoke could never drive the lane at all.
	declared := map[string]bool{}
	for _, handler := range anyStringSlice(desired["handlers"]) {
		declared[handler] = true
	}
	for _, handler := range []string{fetchHandler, scheduledHandler, queueHandler} {
		if !declared[handler] {
			return fmt.Errorf(
				"portable host v3 workerVersion probe must declare the %s handler its attachment probes invoke",
				handler,
			)
		}
	}
	// Every handler the corpus sends must be one the pinned runtime contract
	// defines, or the corpus itself would be asking a conforming host to accept
	// something the ABI does not describe.
	runtime := input.SupportProbes.RuntimeContract
	for handler := range declared {
		if !runtime.defines(handler) {
			return fmt.Errorf(
				"portable host v3 workerVersion probe declares handler %q, which the pinned %s contract does not define",
				handler, runtime.Name,
			)
		}
	}
	// And every handler it declares must be one the bundle it references
	// actually exports. `loadModule` fails a declared-but-unexported handler
	// with handler_not_exported before any traffic arrives, so a version pairing
	// a declaration with a module that cannot answer it is a version NO
	// conforming host may store — and a check driving it would fail exactly the
	// hosts that implement the contract.
	exported := map[string]bool{}
	for _, handler := range input.WorkerBundle.ExportedHandlers {
		exported[handler] = true
	}
	for handler := range declared {
		if !exported[handler] {
			return fmt.Errorf(
				"portable host v3 workerVersion probe declares handler %q, which the module of bundle %q does not export",
				handler, input.WorkerBundle.Name,
			)
		}
	}
	return nil
}

// validateRuntimeContractProbe proves the corpus pins a usable runtime ABI: an
// exact digest to hold a host to, a non-empty handler vocabulary, and one
// handler token OUTSIDE that vocabulary to drive the refusal check with. Without
// the last one the check could not exist, because the vocabulary and the Form's
// enum are the same set by construction.
func validateRuntimeContractProbe(probe RuntimeContractProbe) error {
	if probe.Name == "" || probe.Version == "" || !formpackage.ValidDigest(probe.SchemaDigest) {
		return errors.New("portable host v3 runtime contract probe must pin an exact name, version, and schemaDigest")
	}
	if len(probe.Handlers) == 0 {
		return errors.New("portable host v3 runtime contract probe must pin the ABI handler vocabulary")
	}
	seen := map[string]bool{}
	for _, handler := range probe.Handlers {
		if handler == "" || seen[handler] {
			return errors.New("portable host v3 runtime contract handler vocabulary is invalid")
		}
		seen[handler] = true
	}
	if probe.UndefinedHandler == "" || seen[probe.UndefinedHandler] {
		return errors.New(
			"portable host v3 runtime contract probe must pin an undefinedHandler the contract does not define",
		)
	}
	return nil
}

// familyDataInterfaceNames are the data-plane Interface contracts the Edge
// Platform Family declares. The corpus must pin exactly these, by name: a
// cardinality test would accept two versions of one contract in place of two
// others, and the runner would then record
// `edge-interface-contracts-advertised` having never asked the host about the
// contracts it skipped.
var familyDataInterfaceNames = []string{"edge.kv", "edge.objects", "edge.sql", "edge.queue"}

// validateDataInterfaceProbes proves the corpus pins a usable data-plane
// contract set: every entry names an exact contract at an exact digest, no
// entry repeats, and none of them is the runtime ABI, which has its own probe
// and its own check.
func validateDataInterfaceProbes(probes SupportProbes) error {
	seen := map[string]bool{}
	pinned := map[string]bool{}
	for _, probe := range probes.DataInterfaces {
		if probe.Name == "" || probe.Version == "" || !formpackage.ValidDigest(probe.SchemaDigest) {
			return errors.New(
				"portable host v3 data-plane Interface probes must each pin an exact name, version, and schemaDigest",
			)
		}
		if probe.Name == probes.RuntimeContract.Name {
			return fmt.Errorf(
				"portable host v3 data-plane Interface probe %s is the runtime ABI, which the runtime contract probe already pins",
				probe.Name,
			)
		}
		key := probe.Name + "@" + probe.Version
		if seen[key] {
			return fmt.Errorf("portable host v3 data-plane Interface probe %s is pinned twice", key)
		}
		seen[key] = true
		pinned[probe.Name] = true
	}
	for _, name := range familyDataInterfaceNames {
		if !pinned[name] {
			return fmt.Errorf(
				"portable host v3 must pin the %s data-plane Interface contract; without it the lane records edge-interface-contracts-advertised without ever asking a host about %s",
				name, name,
			)
		}
	}
	return nil
}

// anyStringSlice reads a decoded JSON string array.
func anyStringSlice(value any) []string {
	items, _ := value.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, _ := item.(string)
		out = append(out, text)
	}
	return out
}

// validatePinnedModuleBundle proves one pinned bundle is internally exact and
// returns the identity its manifest addresses. The pinned manifest declares
// exactly the one module the pinned moduleSource bytes are, and its digest and
// size are RE-DERIVED here rather than trusted: that is what makes the corpus
// self-verifying, so a manifest edited without re-deriving the digest, or a
// digest copied from another bundle, fails to load rather than silently driving
// a run against a resource nothing uploaded.
//
// exportedHandlers is held to the pinned ABI vocabulary for the same reason the
// version probe's declarations are: a module claiming to export something the
// contract does not define describes a runtime no host implements.
func validatePinnedModuleBundle(
	label string, moduleSource string, manifestValue map[string]any,
	exportedHandlers []string, runtime RuntimeContractProbe,
) (string, error) {
	if moduleSource == "" {
		return "", fmt.Errorf("portable host v3 %s probe moduleSource is empty", label)
	}
	if manifestValue["apiVersion"] != artifactAPIVersion || manifestValue["kind"] != "WorkerBundle" {
		return "", fmt.Errorf("portable host v3 %s probe manifest identity is invalid", label)
	}
	if _, present := manifestValue["files"]; present {
		return "", fmt.Errorf("portable host v3 %s probe manifest must not carry files", label)
	}
	modules, _ := manifestValue["modules"].([]any)
	if len(modules) != 1 {
		return "", fmt.Errorf("portable host v3 %s probe manifest must declare exactly one module", label)
	}
	module, _ := modules[0].(map[string]any)
	if module == nil {
		return "", fmt.Errorf("portable host v3 %s probe module is invalid", label)
	}
	mainModule, _ := manifestValue["mainModule"].(string)
	if name, _ := module["name"].(string); mainModule == "" || name != mainModule {
		return "", fmt.Errorf("portable host v3 %s mainModule must name its one module", label)
	}
	digest, _ := module["digest"].(string)
	if digest != formpackage.DigestBytes([]byte(moduleSource)) {
		return "", fmt.Errorf("portable host v3 %s module digest does not match moduleSource bytes", label)
	}
	size, ok := module["size"].(json.Number)
	if !ok || size.String() != fmt.Sprintf("%d", len(moduleSource)) {
		return "", fmt.Errorf("portable host v3 %s module size does not match moduleSource bytes", label)
	}
	if len(exportedHandlers) == 0 {
		return "", fmt.Errorf("portable host v3 %s probe must state what its module exports", label)
	}
	seen := map[string]bool{}
	for _, handler := range exportedHandlers {
		if seen[handler] {
			return "", fmt.Errorf("portable host v3 %s probe exportedHandlers repeats %q", label, handler)
		}
		seen[handler] = true
		if !runtime.defines(handler) {
			return "", fmt.Errorf(
				"portable host v3 %s probe claims to export handler %q, which the pinned %s contract does not define",
				label, handler, runtime.Name,
			)
		}
	}
	manifestDigest, err := canonicalDigestOfValue(manifestValue)
	if err != nil {
		return "", fmt.Errorf("portable host v3 %s probe manifest: %w", label, err)
	}
	return manifestDigest, nil
}

// validateWorkerBundleProbe proves the corpus states one bundle, once, and that
// it is the bundle the rest of the corpus can actually be driven against. The
// desired state is exactly the manifest digest the pinned manifest addresses.
//
// Its module must export the WHOLE pinned vocabulary, because the positive
// control of `undeclared-runtime-handler-rejected` is a Worker Version
// declaring exactly that vocabulary against this bundle. A module exporting
// less would make that control a version a conforming host MUST refuse, so the
// check could never be completed by a host that implements the contract — a
// required check no correct host can pass is worse than a missing one.
func validateWorkerBundleProbe(probe WorkerBundleProbe, runtime RuntimeContractProbe) error {
	manifestDigest, err := validatePinnedModuleBundle(
		"workerBundle", probe.ModuleSource, probe.Manifest, probe.ExportedHandlers, runtime,
	)
	if err != nil {
		return err
	}
	if len(probe.Desired) != 1 || probe.Desired["manifestDigest"] != manifestDigest {
		return fmt.Errorf(
			"portable host v3 workerBundle probe desired must be exactly {manifestDigest: %s}",
			manifestDigest,
		)
	}
	exported := map[string]bool{}
	for _, handler := range probe.ExportedHandlers {
		exported[handler] = true
	}
	for _, handler := range runtime.Handlers {
		if !exported[handler] {
			return fmt.Errorf(
				"portable host v3 workerBundle module does not export %q; the lane's positive controls declare the whole %s vocabulary against it",
				handler, runtime.Name,
			)
		}
	}
	return nil
}

// validateFetchOnlyBundleProbe proves the corpus carries a bundle that exports
// STRICTLY LESS than the vocabulary, which is the only way the lane can drive a
// declared-but-unexported handler at all. It borrows the workerBundle probe's
// Form identity and desired shape, so only its name, bytes, manifest, and
// exports are stated here — and its name must differ, or the two would be one
// resource.
func validateFetchOnlyBundleProbe(probe ModuleBundleProbe, bundle WorkerBundleProbe, runtime RuntimeContractProbe) error {
	if !resourceNamePattern.MatchString(probe.Name) {
		return errors.New("portable host v3 fetchOnlyBundle probe name is invalid")
	}
	if probe.Name == bundle.Name {
		return errors.New("portable host v3 fetchOnlyBundle probe must name a resource of its own")
	}
	manifestDigest, err := validatePinnedModuleBundle(
		"fetchOnlyBundle", probe.ModuleSource, probe.Manifest, probe.ExportedHandlers, runtime,
	)
	if err != nil {
		return err
	}
	if probe.ManifestDigest != manifestDigest {
		return fmt.Errorf(
			"portable host v3 fetchOnlyBundle probe manifestDigest must be exactly %s",
			manifestDigest,
		)
	}
	exported := map[string]bool{}
	for _, handler := range probe.ExportedHandlers {
		exported[handler] = true
	}
	for _, handler := range runtime.Handlers {
		if !exported[handler] {
			return nil
		}
	}
	return fmt.Errorf(
		"portable host v3 fetchOnlyBundle module exports the whole %s vocabulary; "+
			"the lane needs one handler it does NOT export to drive handler_not_exported",
		runtime.Name,
	)
}

// canonicalDigestOfValue is the RFC 8785 identity of one decoded JSON
// document, computed exactly the way a host computes a committed manifest
// digest.
func canonicalDigestOfValue(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return formpackage.DigestCanonicalJSON(raw)
}

// validateCrossResourceProbes pins the probes whose portable rules are
// cross-resource semantics a schema cannot express: the deployment weight sum,
// and the three attachments gated on the worker's active deployment.
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
	if nestedName(input.WorkerCustomDomain.Desired, "worker") != input.ModuleWorker.Name {
		return errors.New("portable host v3 workerCustomDomain probe must reference the moduleWorker probe")
	}
	if hostname, _ := input.WorkerCustomDomain.Desired["hostname"].(string); hostname == "" {
		return errors.New("portable host v3 workerCustomDomain probe must pin a hostname")
	}
	// The endpoint probe's desired state is the worker reference and NOTHING
	// else. That is what makes the address check meaningful at all: a corpus
	// that sent a hostname could not tell an address the host assigned from one
	// it echoed back (decision 0024).
	endpoint := input.WorkerEndpoint
	if len(endpoint.Desired) != 1 || nestedName(endpoint.Desired, "worker") != input.ModuleWorker.Name {
		return errors.New(
			"portable host v3 workerEndpoint probe desired must be exactly the moduleWorker reference; " +
				"a probe carrying an address of its own could not prove the host assigned one",
		)
	}
	members, _, err := endpoint.outputContract()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(members, []string{"hostname", "url"}) {
		return errors.New(
			"portable host v3 workerEndpoint probe must pin the output contract its Form declares, [hostname url]",
		)
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
	for index := range contract.RunnerInput.NegativeFixtures {
		fixture := &contract.RunnerInput.NegativeFixtures[index]
		raw, err := readPinnedCorpusFile(root, fixture.Name, fixture.Path, fixture.SHA256)
		if err != nil {
			return err
		}
		if err := formpackage.DecodeStrictIJSON(raw, &fixture.Input); err != nil {
			return fmt.Errorf("portable host v3 fixture %q: %w", fixture.Name, err)
		}
	}
	return nil
}

// hydrateSyntheticDefinition loads the byte-pinned second Form Definition and
// re-derives its schemaDigest from the bytes. The corpus states the identity
// and the bytes; agreement between them is proved here rather than assumed, so
// a Definition edited without re-pinning fails the lane instead of installing a
// different contract under the pinned identity.
func hydrateSyntheticDefinition(root string, contract *Contract) error {
	probe := &contract.RunnerInput.SyntheticSecondDefinitionVersion
	raw, err := readPinnedCorpusFile(root, probe.Name, probe.Path, probe.SHA256)
	if err != nil {
		return err
	}
	definition, err := formpackage.ValidateDefinition(raw)
	if err != nil {
		return fmt.Errorf("portable host v3 synthetic definition %q: %w", probe.Name, err)
	}
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		return err
	}
	if digest != probe.FormRef.SchemaDigest {
		return fmt.Errorf("portable host v3 synthetic definition %q schemaDigest drifted from its bytes", probe.Name)
	}
	if definition.APIVersion != probe.FormRef.APIVersion || definition.Kind != probe.FormRef.Kind ||
		definition.DefinitionVersion != probe.FormRef.DefinitionVersion {
		return fmt.Errorf("portable host v3 synthetic definition %q identity differs from its pinned FormRef", probe.Name)
	}
	probe.Definition = &definition
	return nil
}

// readPinnedCorpusFile reads one corpus file, proving path containment and the
// exact byte digest the contract pinned.
func readPinnedCorpusFile(root, name, path, sha256Digest string) ([]byte, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	absoluteRoot, err := filepath.Abs(realRoot)
	if err != nil {
		return nil, err
	}
	if filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path {
		return nil, fmt.Errorf("portable host v3 fixture %q path is not a clean relative path", name)
	}
	source := filepath.Join(root, filepath.FromSlash(path))
	realSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return nil, fmt.Errorf("portable host v3 fixture %q: %w", name, err)
	}
	absoluteSource, err := filepath.Abs(realSource)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(absoluteRoot, absoluteSource)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("portable host v3 fixture %q escapes the conformance corpus", name)
	}
	info, err := os.Stat(absoluteSource)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("portable host v3 fixture %q is not a regular file", name)
	}
	raw, err := os.ReadFile(absoluteSource)
	if err != nil {
		return nil, err
	}
	if formpackage.DigestBytes(raw) != sha256Digest {
		return nil, fmt.Errorf("portable host v3 fixture %q byte digest drifted", name)
	}
	return raw, nil
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
