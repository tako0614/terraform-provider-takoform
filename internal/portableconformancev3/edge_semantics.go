package portableconformancev3

import (
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// edge_semantics.go carries the two family rules decision 0020 states that no
// desired-state schema can express, and that the Worker aggregate rules of
// decision 0016 do not cover.
//
// Both exist for the same reason the aggregate rules do: schema validity is
// never sufficient (spec/conformance.md, decision 0014). One is a property of
// one spec, the other a property of the store, and they are therefore enforced
// in the two different places those two kinds of rule belong.

// queueRelationPointer is the concrete instance pointer of the `/queue`
// reference a Queue Consumer declares: the one edge from a consumer to the
// queue it drains.
const queueRelationPointer = "/queue"

// cronExpressionViolation reports why one Worker Cron Trigger's expression is
// not a schedule, or "" when it is.
//
// The Form's `cron` pattern is the STRUCTURAL half of the grammar. It has to
// be: a host that has only the Form Definition still needs to reject obvious
// junk, and a regex is what a Definition can carry. But a regex admits `0 24 *
// * *`, `5-1 * * * *`, and `*/0 * * * *` — shapes that name no schedule at all
// — so a host that stopped at the pattern would store a trigger it could never
// fire, and two hosts would then disagree about which of those they accepted.
//
// The parser is the same one the provider runs at plan time, so a configuration
// that plans is a configuration that applies (decision 0020).
//
// It is a property of the spec ALONE — no other resource has to resolve — so it
// is reported as a desired-spec diagnostic and therefore reaches the advisory
// `validate` surface, the binding `prepare` surface, and `apply` alike.
func cronExpressionViolation(form *installedForm, spec map[string]any) string {
	if form.Ref.APIVersion != edgeFormsGroup || form.Ref.Kind != workerCronTriggerKind {
		return ""
	}
	expression, _ := spec["cron"].(string)
	if err := currentformmodel.ValidateCron(expression); err != nil {
		return "WorkerCronTrigger cron " + quoteText(expression) +
			" is not a portable UTC cron expression: " + err.Error()
	}
	return ""
}

// validateCronExpression is the mutation-path form of the same rule. It is
// deliberately redundant with the diagnostic: import and the asynchronous
// commit re-derive every precondition, long after the diagnostics of the
// accepting request were computed.
func validateCronExpression(form *installedForm, spec map[string]any) *hostError {
	if violation := cronExpressionViolation(form, spec); violation != "" {
		return stableError("invalid_argument", violation)
	}
	return nil
}

// validateSingleQueueConsumer refuses a second Queue Consumer against one queue
// incarnation.
//
// `edge.queue` states that a queue has at most one consumer, and the reason is
// in the consumer's own fields: maxRetries, retryDelaySeconds, maxConcurrency,
// and the optional dead-letter queue are properties of how THAT QUEUE is
// drained, not of one attachment. Two consumers would split one stream between
// two retry policies and two dead-letter destinations, and no rule chosen after
// the fact decides which message got which — so the queue's own delivery
// behavior would stop being statable, which is exactly the incompleteness
// decision 0008 forbids.
//
// The lookup is by the queue's UID, never by the name a consumer's spec spells:
// a name can be reused, and a consumer still pinned to a deleted queue is not a
// consumer of the queue that exists now.
func (h *ReferenceHost) validateSingleQueueConsumer(
	space, name string,
	relations []storedRelation,
) *hostError {
	queueUID := relationTargetUID(relations, queueRelationPointer)
	if queueUID == "" {
		return stableError("invalid_argument", "a QueueConsumer requires a target queue")
	}
	selfKey := resourceKey(space, edgeFormsGroup, queueConsumerKind, name)
	for _, candidate := range h.sortedResources() {
		if candidate.Space != space || candidate.Group != edgeFormsGroup ||
			candidate.Kind != queueConsumerKind {
			continue
		}
		if resourceKey(candidate.Space, candidate.Group, candidate.Kind, candidate.Name) == selfKey {
			continue
		}
		if relationTargetUID(candidate.Relations, queueRelationPointer) != queueUID {
			continue
		}
		return stableError(
			"invalid_argument",
			"the AtLeastOnceQueue at uid "+queueUID+" is already drained by QueueConsumer "+candidate.Name+
				"; a queue has at most one consumer, because two would split it between two retry policies "+
				"and two dead-letter destinations",
		)
	}
	return nil
}

// quoteText renders one value for a diagnostic without pulling in strconv at
// every call site.
func quoteText(value string) string { return "\"" + value + "\"" }
