package portableconformancev3

// lane.go carries what differs between the two Host API lanes this runner can
// drive, so that one runner measures both instead of a second copy measuring
// the second one.
//
// The runner and the reference host were already lane-neutral in every place
// that matters: both read the discovery path, the API root, and the served
// group out of the corpus contract. What was NOT neutral was the assertion
// that a corpus IS the expected lane, and the handful of contract facts the
// assertion checked against package constants. Those facts are the lane, so
// they move here and the assertion selects by the corpus's own declared
// apiVersion.
//
// Duplicating the runner instead would have been the obvious move and the
// wrong one. A second copy measures a second lane exactly until someone fixes
// a check in one copy, and a conformance corpus whose two halves disagree
// cannot tell a host which half it failed.

import "net/http"

// lane is one Host API protocol lane's identity and the contract facts that
// differ between lanes. Everything a lane does NOT change stays out of it: a
// field here is a claim that the two lanes genuinely disagree.
type lane struct {
	APIVersion     string
	ContractFormat string
	ManifestFormat string
	DiscoveryPath  string
	APIPath        string

	// ErrorCodeOrder is the canonical taxonomy ordering, ErrorHTTPStatus its
	// code-to-status map. v1beta2 withdrew two codes and moved one status.
	ErrorCodeOrder  []string
	ErrorHTTPStatus map[string]int

	// ConditionReasons is the closed portable reason vocabulary. v1beta1
	// published twelve; v1beta2 publishes exactly the five its type-reason
	// matrix admits, because a reason no type may carry cannot be answered.
	ConditionReasons []string

	// LifecycleCapabilities is the closed capability vocabulary an
	// availability answer may name. v1beta2 drops `refresh`, which no lane
	// route ever served.
	LifecycleCapabilities []string

	// AvailabilityCarriesDeprecated reports whether the availability answer
	// has a `deprecated` member. v1beta1 carried one with no source of truth
	// behind it; v1beta2 removed it rather than invent one.
	AvailabilityCarriesDeprecated bool

	// FormsResponseEnumerates reports whether /forms answers the full
	// installed set for a partial key. v1beta1 capped the array at one, which
	// made the route a probe that could not enumerate.
	FormsResponseEnumerates bool

	// StatusCarriesObserved reports whether a resource status carries the
	// host's observed document. v1beta2 dropped it: the envelope owns status,
	// and an observed document duplicated what outputs already published.
	StatusCarriesObserved bool

	// OperationSchemaVersion and SupportProfileSchemaVersion name the exact
	// referenced schema lanes, which moved for reasons of their own.
	OperationSchemaVersion      string
	SupportProfileSchemaVersion string

	// RequiredChecks is the closed executed-check list a run of this lane must
	// complete. v1beta2's is v1beta1's plus the checks that measure what it
	// changed: a lane that added rules and not checks would be exactly the
	// unmeasured contract decision 0046 forbids.
	RequiredChecks []string
}

// lanes is the closed set of lanes this runner drives, keyed by the apiVersion
// a corpus declares. A corpus naming anything else is refused rather than
// guessed at.
var lanes = map[string]lane{
	beta1Lane.APIVersion: beta1Lane,
	beta2Lane.APIVersion: beta2Lane,
	beta3Lane.APIVersion: beta3Lane,
}

// beta1Lane is the retained lane. Registry-published provider 2.1.1 speaks it
// and hosts serve it today, so every value here is a published fact and none
// of them may be corrected in place.
var beta1Lane = lane{
	APIVersion:     "forms.takoform.com/v1beta1",
	ContractFormat: "takoform.portable-host-conformance@v1beta1",
	ManifestFormat: "takoform.portable-host-conformance-manifest@v1beta1",
	DiscoveryPath:  "/.well-known/takoform/v1beta1",
	APIPath:        "/apis/forms.takoform.com/v1beta1",

	ErrorCodeOrder: []string{
		"invalid_argument", "unauthenticated", "permission_denied", "form_unknown",
		"form_not_installed", "form_unavailable", "form_identity_conflict",
		"resource_not_found", "resource_busy", "import_conflict", "policy_denied",
		"backend_unavailable", "internal_error", "rate_limited",
		"deadline_exceeded", "operation_cancelled", "operation_not_found",
		"dependency_in_use", "deletion_protected", "artifact_missing",
		"artifact_invalid", "unsupported_capability", "migration_required",
		"uid_mismatch", "revision_conflict", "generation_conflict",
	},
	ErrorHTTPStatus: map[string]int{
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
	},
	ConditionReasons: []string{
		"Available", "Provisioning", "Reconciling", "Failed", "BackendUnavailable",
		"SpecDrift", "ExternalChange", "DependencyMissing", "DependencyInUse",
		"PolicyDenied", "UnsupportedCapability", "Deleting",
	},
	LifecycleCapabilities: []string{
		"create", "read", "update", "delete", "import", "observe", "refresh",
	},
	AvailabilityCarriesDeprecated: true,
	FormsResponseEnumerates:       false,
	StatusCarriesObserved:         true,
	OperationSchemaVersion:        "v1alpha1",
	SupportProfileSchemaVersion:   "v1alpha1",
	RequiredChecks:                beta1RequiredChecks,
}

// beta2Lane is the Beta 2 lane (spec/host-api/v1beta2.md, decisions 0039 and
// 0046). Its differences from beta1Lane are the hardening review's structural
// findings, and each one is a repair rather than a preference.
var beta2Lane = lane{
	APIVersion:     "forms.takoform.com/v1beta2",
	ContractFormat: "takoform.portable-host-conformance@v1beta2",
	ManifestFormat: "takoform.portable-host-conformance-manifest@v1beta2",
	DiscoveryPath:  "/.well-known/takoform/v1beta2",
	APIPath:        "/apis/forms.takoform.com/v1beta2",

	// Two codes leave: form_identity_conflict was never given a trigger, and
	// deletion_protected claimed a policy surface no Form provides. One status
	// moves: form_unavailable is transient backing capacity, which is 503 like
	// every other "nothing about your request is wrong" answer, not 409.
	ErrorCodeOrder: []string{
		"invalid_argument", "unauthenticated", "permission_denied", "form_unknown",
		"form_not_installed", "form_unavailable",
		"resource_not_found", "resource_busy", "import_conflict", "policy_denied",
		"backend_unavailable", "internal_error", "rate_limited",
		"deadline_exceeded", "operation_cancelled", "operation_not_found",
		"dependency_in_use", "artifact_missing",
		"artifact_invalid", "unsupported_capability", "migration_required",
		"uid_mismatch", "revision_conflict", "generation_conflict",
	},
	ErrorHTTPStatus: map[string]int{
		"invalid_argument":       http.StatusBadRequest,
		"unauthenticated":        http.StatusUnauthorized,
		"permission_denied":      http.StatusForbidden,
		"form_unknown":           http.StatusNotFound,
		"form_not_installed":     http.StatusConflict,
		"form_unavailable":       http.StatusServiceUnavailable,
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
		"artifact_missing":       http.StatusNotFound,
		"artifact_invalid":       http.StatusBadRequest,
		"unsupported_capability": http.StatusUnprocessableEntity,
		"migration_required":     http.StatusConflict,
		"uid_mismatch":           http.StatusConflict,
		"revision_conflict":      http.StatusPreconditionFailed,
		"generation_conflict":    http.StatusPreconditionFailed,
	},
	// Exactly the reasons the type-reason matrix admits. The seven that go
	// appeared in no row, so a host could answer one and a client would have
	// no rule to read it by.
	ConditionReasons: []string{
		"Available", "Provisioning", "ExternalChange", "DependencyMissing",
		"UnsupportedCapability",
	},
	LifecycleCapabilities: []string{
		"create", "read", "update", "delete", "import", "observe",
	},
	AvailabilityCarriesDeprecated: false,
	FormsResponseEnumerates:       true,
	StatusCarriesObserved:         false,
	OperationSchemaVersion:        "v1alpha2",
	SupportProfileSchemaVersion:   "v1alpha2",
	RequiredChecks:                beta2RequiredChecks,
}

// beta3Lane states its cross-resource rules as MECHANISMS a family
// instantiates rather than as rules about particular Forms. Its wire envelope
// is v1beta2's: what changed is not what a host puts on the wire but what it
// must DO — derive the rules from a Definition's declarations instead of
// carrying a table of Form kinds. That is why its document names no Form kind,
// and why a check enforces that rather than the prose claiming it.
var beta3Lane = lane{
	APIVersion:     "forms.takoform.com/v1beta3",
	ContractFormat: "takoform.portable-host-conformance@v1beta3",
	ManifestFormat: "takoform.portable-host-conformance-manifest@v1beta3",
	DiscoveryPath:  "/.well-known/takoform/v1beta3",
	APIPath:        "/apis/forms.takoform.com/v1beta3",

	ErrorCodeOrder:        beta2Lane.ErrorCodeOrder,
	ErrorHTTPStatus:       beta2Lane.ErrorHTTPStatus,
	ConditionReasons:      beta2Lane.ConditionReasons,
	LifecycleCapabilities: beta2Lane.LifecycleCapabilities,

	AvailabilityCarriesDeprecated: false,
	FormsResponseEnumerates:       true,
	StatusCarriesObserved:         false,
	OperationSchemaVersion:        "v1alpha2",
	SupportProfileSchemaVersion:   "v1alpha2",
	RequiredChecks:                beta3RequiredChecks,
}

// beta3RequiredChecks is v1beta2's plus the one that measures what this lane
// states: that a declared exclusive hold is enforced, and enforced by the
// declaration rather than by a rule the host happens to carry for one Form.
var beta3RequiredChecks = append(append([]string{}, beta2RequiredChecks...),
	"declared-exclusive-holds-enforced",
)

// TerminalErrorIsClosed reports whether this lane's operation schema holds a
// terminal error to the closed shape: a required requestId and retryable
// false. v1alpha2 does; v1alpha1 left both open, which is why a host could
// answer a correlation-less failure and a client had nothing to refuse it
// with.
func (l lane) TerminalErrorIsClosed() bool {
	return l.OperationSchemaVersion == "v1alpha2"
}

// beta2RequiredChecks is v1beta1's list plus one check per rule this lane
// introduced. Each addition names a rule the v1beta2 document states and the
// v1beta1 corpus could not have measured, because the rule did not exist.
var beta2RequiredChecks = append(append([]string{}, beta1RequiredChecks...),
	// The fence matrix as behavior: which fence each operation requires, what
	// an absent one does, and what a stale one answers.
	"fence-matrix-observed",
	// /forms enumerates: a partial key answers the installed set, the fully
	// keyed probe answers zero or one, and neither blends tenants.
	"forms-route-enumerates",
	// The availability booleans mean what the document says they mean, and the
	// error a lifecycle route answers follows from them.
	"availability-truth-conditions",
	// Cancel is a state machine with three outcomes, not a boolean.
	"cancel-outcomes-closed",
	// Sealed external standard-service slots: the projected closure, the
	// closed protocol vocabulary, and prepare-time refusal.
	"external-service-slots-sealed",
	// The materialization rule, driven by the one request the runner never
	// otherwise sends: a spec with a defaulted property omitted. Every other
	// probe is materialized locally, so without this the whole portable
	// defaults section was unmeasured while reporting green.
	"portable-defaults-materialized",
)

// familyDerivedChecks are required of a corpus by the FAMILY generation it
// installs, not by the lane it drives. Putting one in a lane's list would say
// that lane can only ever be driven against a generation carrying those Forms,
// which is the coupling decision 0047 exists to end: the second lane is
// perfectly able to serve a family with no class-selecting identities in it,
// and a corpus that did so would be required to produce a check its runner
// correctly skips.
var familyDerivedChecks = []struct {
	Name     string
	Declared func(RunnerInput) bool
}{
	{
		// The class-export gate needs Forms that select a class.
		Name:     "class-holder-rules-enforced",
		Declared: func(input RunnerInput) bool { return input.DurableWorkflow.Name != "" },
	},
}

// requires reports whether this lane's own check list names one check. A step
// guarded by "is this lane X" stops running the moment a LATER lane inherits
// the rule, which is how a v1beta3 run silently skipped every check v1beta2
// introduced; guarding on the requirement itself cannot drift from the list
// the run is graded against.
func (l lane) requires(name string) bool {
	for _, check := range l.RequiredChecks {
		if check == name {
			return true
		}
	}
	return false
}

// requiredChecksFor is the complete list one corpus must produce: its lane's,
// plus each family-derived check whose subject that corpus declares.
func requiredChecksFor(lane lane, input RunnerInput) []string {
	out := append([]string(nil), lane.RequiredChecks...)
	for _, derived := range familyDerivedChecks {
		if derived.Declared(input) {
			out = append(out, derived.Name)
		}
	}
	return out
}

// ConvergingReason is what a lane calls the state "the host is actively
// converging this resource toward its desired generation". v1beta1 published a
// standalone Reconciling reason; v1beta2's type-reason matrix gives that state
// to the Reconciling condition TYPE and names its reason Provisioning, so a
// Ready condition can no longer borrow the type's name as its reason.
func (l lane) ConvergingReason() string {
	if l.isPortableConditionReason("Reconciling") {
		return "Reconciling"
	}
	return "Provisioning"
}

// laneFor resolves the lane a corpus declares. An unknown apiVersion is an
// error rather than a default: a runner that guessed would measure a host
// against rules nobody published.
func laneFor(apiVersion string) (lane, bool) {
	found, known := lanes[apiVersion]
	return found, known
}

func (l lane) isAutomaticallyRetryable(code string) bool {
	for _, candidate := range automaticallyRetryableCodes {
		if candidate == code {
			return true
		}
	}
	return false
}

func (l lane) isPortableConditionReason(reason string) bool {
	for _, candidate := range l.ConditionReasons {
		if candidate == reason {
			return true
		}
	}
	return false
}

// DrivesManifestFormat reports whether this runner drives a corpus declaring
// that manifest format. The CLI asks before loading, so a corpus from a lane
// this build does not know is refused by name rather than by a decode error
// three layers down.
func DrivesManifestFormat(format string) bool {
	for _, candidate := range lanes {
		if candidate.ManifestFormat == format {
			return true
		}
	}
	return false
}

// DrivenManifestFormats lists the corpus manifest formats this runner drives,
// for a diagnostic that has to say what it would have accepted.
func DrivenManifestFormats() []string {
	return []string{beta1Lane.ManifestFormat, beta2Lane.ManifestFormat}
}
