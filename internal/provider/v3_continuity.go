package provider

// v3_continuity.go carries the two state-continuity rules of the Host API
// v1alpha3 resource lane (spec/decisions/0017): what a read does when the
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

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	// when a diagnostic error was raised, or when the operation is still running
	// and the resource does not exist yet.
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
}

// v3NoPendingOperation is the outcome of a read with no recorded operation:
// the ordinary read, where absence is deletion.
var v3NoPendingOperation = v3PendingOutcome{RemoveOnAbsent: true}

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
//	                           representation settles state but the marker
//	                           stays, because nothing has committed yet
//	terminal, success          the operation's result resource is verified
//	                           against the exact identity, its uid is adopted
//	                           when state has none, and the ordinary read
//	                           settles state and clears the marker
//	terminal, error            the exact resource GET is the final word:
//	                           absent means state may be removed, present means
//	                           the uid decides between adoption and hard error
//	operation_not_found        the record is gone; the exact resource GET is
//	                           the final word, under the same uid rule
//
// Nothing in the table ever re-binds by name alone: a known uid is verified in
// every branch (v3RequireStateUID), and an unknown uid is adopted only from a
// representation the host served under the exact FormRef state records.
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
	operation, err := c.GetOperation(ctx, request.OperationID)
	switch {
	case err != nil && clientv3.IsOperationNotFound(err):
		// The host no longer holds the record, which the lane explicitly permits
		// once an operation record expires. The resource itself is then the only
		// evidence left, so the ordinary read decides — including that absence is
		// deletion.
		diags.AddWarning(
			kind+" operation "+request.OperationID+" is no longer addressable",
			fmt.Sprintf(
				"State recorded an accepted mutation as operation %s, and the host no longer holds that record. "+
					"The exact resource read is the final word: %s/%s is adopted when it exists under the "+
					"identity state records, and removed from state when it does not.",
				request.OperationID, request.Space, request.Name,
			),
		)
		return v3NoPendingOperation
	case err != nil:
		diags.AddError(
			"Failed to resume "+kind+" operation "+request.OperationID,
			"State records an accepted mutation that this refresh could not consult, so the refresh cannot "+
				"decide whether the resource exists. State is preserved. "+err.Error(),
		)
		return v3PendingOutcome{Stop: true}
	}
	if !operation.Done {
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
	return v3NoPendingOperation
}
