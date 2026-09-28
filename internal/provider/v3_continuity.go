package provider

// v3_continuity.go carries the two state-continuity rules of the Host API
// v1beta1 resource lane (spec/decisions/0017): what a read does when the
// host serves a DIFFERENT incarnation than the one state names, and what a
// read does when state records a mutation the host accepted but has not
// finished.
//
// Both exist for the same reason. Terraform's refresh is the only place a
// provider learns what the host holds, and whatever refresh writes is what the
// next plan is computed from. A refresh that removes a resource from state
// removes it from management: the next apply creates a second one, fences on
// `If-None-Match: *`, and fails against the resource the host still owns —
// leaving the operator with no path forward from inside Terraform. So neither
// an unexplained UID nor an unfinished operation may remove state.

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
)

// v3StateStringValue reads one optional state string as a plain value; an
// absent or unknown value is the empty string.
func v3StateStringValue(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

// v3RejectPendingOperation prevents planning or applying another desired
// mutation while an accepted operation remains unresolved. A refresh may keep
// this marker while the Host still reports the operation as pending; allowing
// an update then could turn its empty generation fence into a second create.
func v3RejectPendingOperation(
	ctx context.Context,
	state tfsdk.State,
	kind string,
	diags *diag.Diagnostics,
) bool {
	if state.Raw.IsNull() {
		return false
	}
	var operationID types.String
	diags.Append(state.GetAttribute(ctx, path.Root("pending_operation_id"), &operationID)...)
	if diags.HasError() || operationID.IsNull() || operationID.IsUnknown() || operationID.ValueString() == "" {
		return diags.HasError()
	}
	diags.AddError(
		"Unresolved "+kind+" operation "+operationID.ValueString(),
		"The Host accepted a mutation that has not been proven terminal, so the provider will not plan or send "+
			"another mutation for this resource. Refresh under the principal that submitted the operation until "+
			"the exact resource or a terminal operation settles it; do not re-create or re-apply it while this "+
			"marker remains.",
	)
	return true
}

// v3RequireStateUID holds a read to the incarnation state names.
//
// A same-named resource carrying a different UID is a DIFFERENT resource: the
// one this state was applied against was deleted and something re-used its
// name. That is a hard error, and state is preserved, because the provider
// cannot know which resource the operator meant. Every alternative is worse:
// re-binding state to the new UID adopts a resource the author never applied
// (the same reasoning decision 0015 uses to refuse automatic re-binding of a
// relation whose target changed incarnation), and removing state makes the
// next apply fence on `If-None-Match: *` against a resource that exists, which
// fails with no remedy reachable from Terraform.
//
// The two failures are deliberately not symmetric. Relation drift is reported
// as a WARNING with a proposed repairing apply, because the resource itself is
// still the one state names and an apply re-pins the reference. A UID mismatch
// on the resource ITSELF has no such apply: nothing the provider can send
// converts one incarnation into another, so the operator must choose.
func v3RequireStateUID(
	kind, space, name, stateUID string,
	res *clientv3.Resource,
	diags *diag.Diagnostics,
) bool {
	if stateUID == "" || res == nil || stateUID == res.Metadata.UID {
		return true
	}
	diags.Append(v3Diagnostic{
		Summary:           kind + " is a different incarnation than state records",
		Space:             space,
		Name:              name,
		Pointer:           "/metadata/uid",
		Code:              v3CodeUIDMismatch,
		ExpectedUID:       stateUID,
		CurrentUID:        res.Metadata.UID,
		CurrentGeneration: res.Metadata.Generation,
		CurrentRevision:   res.Metadata.Revision,
		Detail: "The resource this state was applied against no longer exists, and the provider will not " +
			"re-bind state to a resource it never applied — nor remove state, which would make the next " +
			"apply fail against the resource that does exist.",
		Repair: fmt.Sprintf(
			"Resolve it explicitly, by one of:\n"+
				"  1. import the new incarnation: remove this resource from state "+
				"(terraform state rm) and import it again, which binds state to uid %s;\n"+
				"  2. restore the prior incarnation, if the resource the state names can be recovered "+
				"host-side under uid %s;\n"+
				"  3. delete the host-side replacement, then re-apply to create the resource this "+
				"configuration describes.\n"+
				"State is preserved until you choose.",
			res.Metadata.UID, stateUID,
		),
	}.error())
	return false
}

// v3PendingRequest names the one accepted-but-unfinished mutation a read must
// consult before it reads the resource.
type v3PendingRequest struct {
	OperationID string
	Ref         clientv3.FormRef
	Space       string
	Name        string
	StateUID    string
}

// v3PendingOutcome is what the consultation decided about the resource read
// that follows it.
type v3PendingOutcome struct {
	// Stop ends the read immediately, leaving state exactly as it is. It is set
	// when a diagnostic error was raised, including when the accepted create
	// lacks a Host-issued UID and cannot safely adopt a same-name resource.
	Stop bool
	// RemoveOnAbsent authorizes the caller to treat `resource_not_found` as
	// deletion. It is FALSE while an accepted mutation is still in flight: on a
	// host where the resource does not exist until the operation commits, a 404
	// during that window means "not yet", not "gone".
	RemoveOnAbsent bool
	// KeepMarker preserves pending_operation_id after a successful state write.
	// An operation that has not reached a terminal state still has something to
	// resume, even when the resource is already readable.
	KeepMarker bool
	// FailOnAbsent makes an exact-resource 404 a hard error when the operation
	// handle is not addressable to this caller. The missing handle is not proof
	// that an accepted mutation failed or stopped committing.
	FailOnAbsent bool
	// ExpectedUID is a Host-issued identity from a verified terminal result.
	// The following resource GET must match it even when accepted state had no
	// target UID; otherwise a same-name replacement could be adopted.
	ExpectedUID string
}

// v3NoPendingOperation is the outcome of a read with no recorded operation:
// the ordinary read, where absence is deletion.
var v3NoPendingOperation = v3PendingOutcome{RemoveOnAbsent: true}

// A malformed accepted 202 has no Host-issued handle. Keep a deliberately
// invalid operation ID in state so every plan is fenced, without ever sending
// a fabricated ID to the Host.
const v3UnaddressableDeleteOperation = "unaddressable_delete_operation"

const v3UnaddressableCreateOperation = "unaddressable_create_operation"

const v3PendingDeletePrivateKey = "pending_delete"

const v3DeleteRetryActionPrefix = "retry_delete:"

func v3FailedDeleteOperationForRetry(values v3Values) string {
	action := v3StateStringValue(values.PendingOperationAction)
	if !strings.HasPrefix(action, v3DeleteRetryActionPrefix) {
		return ""
	}
	return strings.TrimPrefix(action, v3DeleteRetryActionPrefix)
}

// v3PendingDelete distinguishes a deletion from an interrupted create. A
// readable create may already have UID and generation while its operation is
// still pending, so neither identity value can stand in for an action marker.
// Private state is retained as a redundant recovery copy on an errored Delete.
func v3PendingDelete(ctx context.Context, values v3Values, private interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
}, diags *diag.Diagnostics) bool {
	if v3StateStringValue(values.PendingOperationID) == "" {
		return false
	}
	action := v3StateStringValue(values.PendingOperationAction)
	if action == "delete" {
		return true
	}
	if action == "create" {
		return false
	}
	if action != "" {
		diags.AddError("Unknown pending operation action", "State records an unsupported pending operation action. State is preserved until it is resolved explicitly.")
		return false
	}
	if private != nil {
		raw, privateDiags := private.GetKey(ctx, v3PendingDeletePrivateKey)
		diags.Append(privateDiags...)
		if string(raw) == "true" {
			return true
		}
	}
	return false
}

type v3DeleteOutcome uint8

const (
	v3DeleteUnresolved v3DeleteOutcome = iota
	v3DeleteSucceeded
	v3DeleteFailed
)

// v3ResumePendingDelete never treats a same-name GET as adoption or a missing
// operation as deletion proof. A verifiable terminal failure permits an exact
// read to decide whether the old UID still exists and may be retried explicitly.
func v3ResumePendingDelete(
	ctx context.Context,
	c *clientv3.Client,
	kind string,
	request v3PendingRequest,
	diags *diag.Diagnostics,
) v3DeleteOutcome {
	if request.OperationID == v3UnaddressableDeleteOperation {
		diags.AddError("Cannot reconcile accepted "+kind+" delete", "The Host accepted the delete but did not return a usable operation handle. State is preserved; resolve the Host-side outcome explicitly before another mutation.")
		return v3DeleteUnresolved
	}
	operation, err := c.GetOperation(ctx, request.OperationID)
	if err != nil {
		diags.AddError("Cannot reconcile accepted "+kind+" delete", "The delete operation is not verifiably terminal. State and its operation marker are preserved. "+err.Error())
		return v3DeleteUnresolved
	}
	if !operation.Done {
		diags.AddWarning(kind+" delete is still running on the Host", "Operation "+request.OperationID+" is pending. State and its operation marker are preserved; refresh after it settles.")
		return v3DeleteUnresolved
	}
	if operation.Target != nil && operation.Target.UID != "" && operation.Target.UID != request.StateUID {
		diags.AddError(kind+" delete operation targets a different incarnation", "Operation "+request.OperationID+" targets uid "+operation.Target.UID+", but state records "+request.StateUID+". State is preserved.")
		return v3DeleteUnresolved
	}
	if operation.Error != nil {
		diags.AddWarning(kind+" delete failed on the Host", "Operation "+request.OperationID+" ended with "+operation.Error.Code+": "+operation.Error.Message+". The provider checks the exact old resource before allowing a new explicit delete attempt.")
		return v3DeleteFailed
	}
	return v3DeleteSucceeded
}

// v3ResumePendingOperation consults the operation recorded in state before the
// resource is read, and decides what the following read may conclude.
//
// The order is what makes the lane resumable at all. `pending_operation_id` is
// written by a create the host ACCEPTED as a long-running Operation that did
// not reach a terminal state before the deadline. Reading the resource first
// would ask the wrong question: on a host that commits the resource only when
// the operation commits, the resource legitimately does not exist yet, and a
// 404 read as deletion drops a resource the host is actively creating.
//
//	operation state            what the read may then conclude
//	-------------------------  ------------------------------------------------
//	still running              absence is NOT deletion; a readable
//	                           representation settles state only when a
//	                           Host-issued uid is already recorded, and the
//	                           marker stays because nothing has committed yet
//	terminal, success          the operation's result resource is verified
//	                           against the exact identity, its uid is adopted
//	                           when state has none, and the ordinary read
//	                           settles state and clears the marker
//	terminal, error            a known uid permits an exact resource GET:
//	                           absent means state may be removed, present means
//	                           the uid decides between continuity and hard error
//	operation_not_found        not addressable to this caller; a present
//	                           resource settles only when its known uid matches;
//	                           absence is an error that retains state
//
// Nothing in the table ever re-binds by name alone: a known uid is verified in
// every branch (v3RequireStateUID), and an unknown uid is adopted only after
// the terminal Operation's verified result supplies that Host-issued identity.
func v3ResumePendingOperation(
	ctx context.Context,
	c *clientv3.Client,
	kind string,
	request v3PendingRequest,
	diags *diag.Diagnostics,
) v3PendingOutcome {
	if request.OperationID == "" {
		return v3NoPendingOperation
	}
	if request.OperationID == v3UnaddressableCreateOperation {
		diags.AddError(
			"Cannot reconcile accepted "+kind+" create",
			"The Host accepted the create but did not return a usable operation handle. State is preserved; "+
				"resolve the Host-side outcome explicitly before another mutation.",
		)
		return v3PendingOutcome{Stop: true}
	}
	operation, err := c.GetOperation(ctx, request.OperationID)
	switch {
	case err != nil && clientv3.IsOperationNotFound(err):
		// operation_not_found also hides handles from a foreign principal. It
		// therefore does not establish that the accepted mutation expired or
		// failed. Without a Host-issued uid, even a same-name resource cannot
		// prove the incarnation this state names, so do not query/adopt it.
		if request.StateUID == "" {
			diags.AddError(
				"Cannot reconcile "+kind+" operation "+request.OperationID,
				"The Host says this operation is not addressable to the configured principal. State has no verified "+
					"resource uid, so a same-name resource cannot prove which incarnation the accepted mutation "+
					"created. State and the operation id are preserved; refresh with the principal that submitted "+
					"the operation or explicitly resolve the Host-side outcome before applying.",
			)
			return v3PendingOutcome{Stop: true}
		}
		diags.AddWarning(
			kind+" operation "+request.OperationID+" is not addressable to this caller",
			fmt.Sprintf(
				"The Host returned operation_not_found for accepted operation %s. This can mean the operation is "+
					"not addressable to this principal, not only that its record expired. The provider will read %s/%s "+
					"only to verify the uid already recorded in state; a matching representation settles state, "+
					"while absence retains state and blocks further plans.",
				request.OperationID, request.Space, request.Name,
			),
		)
		return v3PendingOutcome{RemoveOnAbsent: false, FailOnAbsent: true}
	case err != nil:
		diags.AddError(
			"Failed to resume "+kind+" operation "+request.OperationID,
			"State records an accepted mutation that this refresh could not consult, so the refresh cannot "+
				"decide whether the resource exists. State is preserved. "+err.Error(),
		)
		return v3PendingOutcome{Stop: true}
	}
	if !operation.Done {
		if request.StateUID == "" {
			diags.AddError(
				"Cannot reconcile accepted "+kind+" operation "+request.OperationID,
				"The operation is still running and state has no Host-issued resource uid. A same-name "+
					"resource cannot prove the accepted create's incarnation. State and the operation id are preserved.",
			)
			return v3PendingOutcome{Stop: true}
		}
		diags.AddWarning(
			kind+" mutation is still running on the host",
			fmt.Sprintf(
				"Operation %s for %s/%s has not reached a terminal state. The resource may not exist yet, so this "+
					"refresh does not treat its absence as deletion and keeps the resource under management. "+
					"Refresh again once the host settles; poll the operation directly to see its progress.",
				request.OperationID, request.Space, request.Name,
			),
		)
		return v3PendingOutcome{RemoveOnAbsent: false, KeepMarker: true}
	}
	if operation.Error != nil {
		if request.StateUID == "" {
			diags.AddError(
				"Cannot reconcile failed "+kind+" operation "+request.OperationID,
				"The operation ended with "+operation.Error.Code+" but state has no Host-issued resource uid. "+
					"A same-name resource cannot prove which incarnation the accepted create named. State and the "+
					"operation id are preserved for explicit resolution.",
			)
			return v3PendingOutcome{Stop: true}
		}
		diags.AddWarning(
			kind+" mutation failed on the host",
			fmt.Sprintf(
				"Operation %s for %s/%s terminated with %s: %s. Whether anything was committed is decided by "+
					"reading the resource at its exact identity, which this refresh does next.",
				request.OperationID, request.Space, request.Name, operation.Error.Code, operation.Error.Message,
			),
		)
		return v3NoPendingOperation
	}
	result, err := clientv3.OperationResultResource(operation, request.Ref, request.Name, request.Space)
	if err != nil {
		diags.AddError(
			"Terminal "+kind+" operation "+request.OperationID+" did not carry a usable result",
			"The host reported the accepted mutation as successful but its result is not a verified "+
				"representation of this resource, so the refresh cannot settle state from it. State is preserved. "+
				err.Error(),
		)
		return v3PendingOutcome{Stop: true}
	}
	if !v3RequireStateUID(kind, request.Space, request.Name, request.StateUID, result, diags) {
		return v3PendingOutcome{Stop: true}
	}
	// The operation committed. The ordinary read follows so state settles against
	// the representation that exists NOW rather than the one the operation
	// happened to return, and it clears the marker by writing state.
	return v3PendingOutcome{RemoveOnAbsent: true, ExpectedUID: result.Metadata.UID}
}
