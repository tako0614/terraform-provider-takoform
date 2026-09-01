package provider

// v3_diagnostics.go is the one renderer every v1beta1-lane diagnostic goes
// through.
//
// A provider diagnostic is the only thing a practitioner sees when an apply
// stops, and the lane's own vocabulary is what makes one actionable: the exact
// Form identity the request was made under, the JSON pointer into the desired
// document, the fences that were sent and the values the host holds, the
// operation and request the host can be asked about, a stable code to search
// for, whether waiting helps, and one concrete repair. Before this file the
// package rendered `err.Error()` at most of those sites and the operator had to
// reconstruct the rest.
//
// Two vocabularies meet here and are deliberately kept apart. A HOST error
// carries a code from the closed v1beta1 taxonomy
// (spec/host-api/operations-v1beta1.json); it is rendered verbatim and marked
// as the host's, because that enum is published and this provider does not
// extend it. A PROVIDER error carries a code from the closed set below, in its
// own `takoform.provider/` namespace, because a refusal the provider makes
// before any request is not a host outcome and must not borrow a host code.
//
// On the Terraform resource address: a provider cannot know it. Terraform core
// prefixes every diagnostic raised during a resource operation with
// `with <type>.<name>, on <file> line <n>`, so the address is always on screen;
// what the provider owns is the HOST address — resource type, space, and
// portable name — and that is what the `Resource:` line carries.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	"github.com/tako0614/terraform-provider-takoform/internal/providerdiagnostics"
)

// The closed set of provider-side stable error codes. They are namespaced so
// no reader can mistake one for a host code, and they are constants so a
// diagnostic's code is greppable in this repository and in a user's logs.
const (
	v3CodeNotConfigured              = "takoform.provider/not-configured"
	v3CodeLaneUnavailable            = "takoform.provider/lane-unavailable"
	v3CodeImmutableRevisionSameName  = providerdiagnostics.ImmutableRevisionSameName
	v3CodeNameUnresolved             = "takoform.provider/revision-name-unresolved"
	v3CodeRevisionOwnerMissing       = "takoform.provider/revision-owner-missing"
	v3CodeRevisionOwnerIgnored       = "takoform.provider/revision-owner-ignored"
	v3CodeApplyIdempotencyKeyReuse   = "takoform.provider/apply-idempotency-key-reuse"
	v3CodeApplyIdempotencyKeyUnknown = "takoform.provider/apply-idempotency-key-unknown"
	v3CodeRuntimeInputsInvalid       = "takoform.provider/runtime-inputs-invalid"
	v3CodeStateRefUnsupported        = "takoform.provider/state-form-ref-unsupported"
	v3CodeStateRefMissing            = "takoform.provider/state-form-ref-missing"
	v3CodeImportIDInvalid            = "takoform.provider/import-id-invalid"
	v3CodeUIDMismatch                = "takoform.provider/uid-mismatch"
	v3CodeRelationTargetChanged      = "takoform.provider/relation-target-changed"
	v3CodeHostResponseInvalid        = "takoform.provider/host-response-invalid"
	v3CodeHostSupportUnknown         = "takoform.provider/host-support-unreadable"
	v3CodeFormUnsupported            = "takoform.provider/host-does-not-support-form"
	v3CodeInterfaceUnsupported       = "takoform.provider/host-does-not-support-interface"
	v3CodeBindingUnsupported         = "takoform.provider/host-does-not-support-binding"
	v3CodeCapabilityUnsupported      = providerdiagnostics.HostDoesNotSupportValue
	v3CodeLimitExceeded              = "takoform.provider/host-limit-exceeded"
	v3CodeProviderBug                = "takoform.provider/internal"
)

// v3HostFault is everything the host said about one failure.
type v3HostFault struct {
	Code            string
	Message         string
	RequestID       string
	StatusCode      int
	HostCode        string
	Retryable       bool
	ProtocolInvalid bool
	// Accepted marks a mutation the host took but did not finish verifiably.
	Accepted    bool
	OperationID string
	UID         string
}

// v3HostFaultFrom destructures whatever the client returned. It reports false
// for an error that is neither a typed API error nor an accepted mutation —
// a transport failure, say — so the caller still renders its own envelope with
// the raw cause.
func v3HostFaultFrom(err error) (v3HostFault, bool) {
	fault := v3HostFault{}
	found := false
	var accepted *clientv3.AcceptedError
	if errors.As(err, &accepted) {
		fault.Accepted = true
		fault.OperationID = accepted.OperationID
		fault.UID = accepted.UID
		found = true
	}
	var apiErr *clientv3.APIError
	if errors.As(err, &apiErr) {
		fault.Code = apiErr.Code
		fault.Message = apiErr.Message
		fault.RequestID = apiErr.RequestID
		fault.StatusCode = apiErr.StatusCode
		fault.HostCode = apiErr.HostCode
		fault.Retryable = apiErr.Retryable
		fault.ProtocolInvalid = apiErr.ProtocolInvalid
		found = true
	}
	return fault, found
}

// v3HostRepairs is the concrete next action for each host code this lane can
// return. It is keyed by the PUBLISHED taxonomy and adds nothing to it: the
// codes are the host's, the sentences are the provider's.
var v3HostRepairs = map[string]string{
	"invalid_argument":       "Correct the desired state the message names and re-plan; the host mutated nothing.",
	"unauthenticated":        "Configure a valid `token` (or TAKOFORM_TOKEN) for this endpoint and re-run.",
	"permission_denied":      "Grant this credential the operation on this space, or apply as a principal that already has it.",
	"form_unknown":           "This host does not carry the exact Form identity in the message. Install it on the host, or pin a provider build whose default create target the host does carry.",
	"form_not_installed":     "Install this exact Form on the host before applying resources of this kind.",
	"form_unavailable":       "The Form is installed but not activated for this scope. Ask the host operator to activate it, then re-run.",
	"form_identity_conflict": "The host holds a different definition under this Form line. Reconcile the host's installed identity with the one this provider build carries.",
	"resource_not_found":     "Refresh so state reflects the host, then re-plan.",
	"resource_busy":          "Another operation holds this resource. This is retryable: wait and re-run the same apply.",
	"import_conflict":        "Some resource on this host already manages the native resource this import names, or this address was adopted onto a different one. A native resource has exactly one managed resource: find the holder the message names and remove it — or import the address it already occupies — rather than importing again.",
	"policy_denied":          "A host policy refuses this desired state. Change the configuration the message names, or have the policy amended.",
	"backend_unavailable":    "The host's backend is unavailable. This is retryable: re-run the same apply.",
	"internal_error":         "The host failed internally. Report the requestId above to the host operator; do not retry blindly.",
	"rate_limited":           "This is retryable: wait for the interval the host asked for and re-run the same apply.",
	"deadline_exceeded":      "The host did not finish in time. This is retryable: re-run, and raise the resource's *_timeout attribute if it keeps happening.",
	"operation_cancelled":    "The long-running operation was cancelled. Refresh, then re-plan against what the host now holds.",
	"operation_not_found":    "The operation record expired. Refresh: the resource read is the final word on whether the mutation landed.",
	"dependency_in_use":      "A live relation still holds this resource. Remove or re-point the holders the message names first, then delete this one.",
	"deletion_protected":     "Host policy protects this resource from deletion. Lift the protection on the host, then re-run.",
	"artifact_missing":       "The manifest or blob this desired state references is not held by this tenant. Re-upload the artifact, or reference one this tenant committed.",
	"artifact_invalid":       "The artifact manifest does not satisfy the artifact contract. Rebuild the bundle and re-apply.",
	"unsupported_capability": "This host does not offer the capability the message names. Remove what needs it, or apply against a host whose Host Support Profile declares it.",
	"migration_required":     "The host requires an explicit migration for this resource. Follow the host's migration path; the provider will not migrate state implicitly.",
	"uid_mismatch":           "The host holds a different incarnation than state records. Refresh, then either import the incarnation that exists or restore the one state names.",
	"revision_conflict":      "The representation moved under an If-Match precondition. Refresh and re-plan; a delete fences on the generation, so only a caller that asked about the representation sees this.",
	"generation_conflict":    "The desired generation moved under the update or delete fence. Refresh and re-plan.",
}

// v3Diagnostic is one rendered lane diagnostic.
type v3Diagnostic struct {
	Summary string
	// Detail is optional prose placed before the identity block.
	Detail string

	ResourceType string
	Space        string
	Name         string
	Ref          v3FormRef
	// Pointer is the JSON pointer into the portable desired or status document
	// the fault is about (`/kvBindings/0/resource`, `/metadata/name`).
	Pointer string
	// Attribute routes the diagnostic to one HCL attribute, so the CLI prints
	// the configuration line as well as the address.
	Attribute *path.Path

	ExpectedUID, CurrentUID               string
	ExpectedGeneration, CurrentGeneration string
	ExpectedRevision, CurrentRevision     string
	OperationID                           string

	// Code is the provider-side stable code. It is ignored when Host carries a
	// host code, because the host's outcome is the more specific fact.
	Code string
	Host *v3HostFault
	// Cause is the raw error, rendered when the host said nothing structured.
	Cause error

	Repair string
}

func (d v3Diagnostic) detail() string {
	var body strings.Builder
	if d.Detail != "" {
		body.WriteString(d.Detail)
		body.WriteString("\n\n")
	}
	if d.Host != nil && d.Host.Message != "" {
		body.WriteString("Host: " + d.Host.Message + "\n\n")
	} else if d.Host == nil && d.Cause != nil {
		body.WriteString("Cause: " + d.Cause.Error() + "\n\n")
	}
	for _, line := range d.fields() {
		body.WriteString(line)
		body.WriteString("\n")
	}
	repair := d.repair()
	if repair != "" {
		body.WriteString("\n")
		body.WriteString(repair)
	}
	return strings.TrimRight(body.String(), "\n")
}

// fields renders the identity block: every fact this diagnostic actually has,
// each on its own line, always in the same order.
func (d v3Diagnostic) fields() []string {
	lines := make([]string, 0, 12)
	add := func(label, value string) {
		if value != "" {
			lines = append(lines, label+": "+value)
		}
	}
	add("Resource", v3ResourceAddress(d.ResourceType, d.Space, d.Name))
	add("Form", v3FormRefText(d.Ref))
	add("Pointer", d.Pointer)
	add("Expected UID", d.ExpectedUID)
	add("Current UID", d.CurrentUID)
	add("Expected generation", d.ExpectedGeneration)
	add("Current generation", d.CurrentGeneration)
	add("Expected revision", d.ExpectedRevision)
	add("Current revision", d.CurrentRevision)
	operation := d.OperationID
	if operation == "" && d.Host != nil {
		operation = d.Host.OperationID
	}
	add("Operation", operation)
	if d.Host != nil {
		add("Request", d.Host.RequestID)
		add("Host reason", d.Host.HostCode)
	}
	add("Code", d.codeText())
	lines = append(lines, "Retryable: "+v3YesNo(d.retryable()))
	return lines
}

// codeText names the stable code and where it comes from. A host answer that
// did not match the closed taxonomy is labelled as such rather than presented
// as a portable code.
func (d v3Diagnostic) codeText() string {
	if d.Host != nil {
		switch {
		case d.Host.ProtocolInvalid:
			return fmt.Sprintf("protocol-invalid host response (HTTP %d)", d.Host.StatusCode)
		case d.Host.Code != "" && d.Host.StatusCode != 0:
			return fmt.Sprintf("%s (host, HTTP %d)", d.Host.Code, d.Host.StatusCode)
		case d.Host.Code != "":
			return d.Host.Code + " (host)"
		case d.Host.Accepted:
			return "accepted-without-representation (host)"
		}
	}
	return d.Code
}

func (d v3Diagnostic) retryable() bool {
	return d.Host != nil && d.Host.Retryable && !d.Host.ProtocolInvalid
}

// repair is the diagnostic's own repair when it states one, and the closed
// per-host-code repair otherwise. A diagnostic with neither is a bug this
// renderer makes visible rather than hides.
func (d v3Diagnostic) repair() string {
	if d.Repair != "" {
		return d.Repair
	}
	if d.Host != nil {
		if repair, known := v3HostRepairs[d.Host.Code]; known {
			return repair
		}
		if d.Host.ProtocolInvalid {
			return "The host's error response is not the closed v1beta1 envelope, so it carries no portable " +
				"meaning. Report it to the host operator; the provider will not guess a remedy from it."
		}
	}
	return ""
}

func (d v3Diagnostic) error() diag.Diagnostic {
	if d.Attribute != nil {
		return diag.NewAttributeErrorDiagnostic(*d.Attribute, d.Summary, d.detail())
	}
	return diag.NewErrorDiagnostic(d.Summary, d.detail())
}

func (d v3Diagnostic) warning() diag.Diagnostic {
	if d.Attribute != nil {
		return diag.NewAttributeWarningDiagnostic(*d.Attribute, d.Summary, d.detail())
	}
	return diag.NewWarningDiagnostic(d.Summary, d.detail())
}

// v3ResourceAddress renders the HOST address of one resource.
func v3ResourceAddress(resourceType, space, name string) string {
	switch {
	case resourceType == "":
		return ""
	case space != "" && name != "":
		return fmt.Sprintf("%s (%s/%s)", resourceType, space, name)
	case name != "":
		return fmt.Sprintf("%s (%s)", resourceType, name)
	}
	return resourceType
}

// v3FormRefText renders one exact FormRef. The package digest is deliberately
// absent: it is distribution provenance, never identity (spec/decisions/0011),
// and a reader comparing two diagnostics must compare the four members that
// decide dispatch.
func v3FormRefText(ref v3FormRef) string {
	if ref.APIVersion == "" || ref.Kind == "" {
		return ""
	}
	return fmt.Sprintf("%s %s@%s schema=%s", ref.APIVersion, ref.Kind, ref.DefinitionVersion, ref.SchemaDigest)
}

func v3YesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// v3HostCallDiagnostic is the shared envelope for a failed host call: it
// destructures whatever the client returned and fills in the identity the
// caller already holds.
func v3HostCallDiagnostic(summary string, err error, base v3Diagnostic) diag.Diagnostic {
	base.Summary = summary
	base.Cause = err
	if fault, structured := v3HostFaultFrom(err); structured {
		base.Host = &fault
		if base.Code == "" {
			base.Code = v3CodeHostResponseInvalid
		}
	}
	return base.error()
}
