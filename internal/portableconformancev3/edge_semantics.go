package portableconformancev3

import (
	"strings"

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

// deadLetterRelationPointer is the optional edge from a consumer to the queue
// that receives the messages the consumer exhausted.
const deadLetterRelationPointer = "/deadLetterQueue"

// canonicalizeEdgeSpec rewrites the family's canonical spellings into one
// desired spec, at the host's single materialization entry point, before the
// spec is validated, digested, stored, or echoed.
//
// Today that is one field. `WorkerCustomDomain.hostname` is a DNS name, and
// DNS says "API.Example.com", "api.example.com." and "api.example.com" are
// one name. A host comparing the written bytes sees three, so two attachments
// could claim one hostname and both be stored. Canonicalizing HERE rather than
// at the comparison is what makes the claim decidable at all: the stored value
// is the identity, so uniqueness is an equality test on stored specs and a
// re-apply under any other spelling moves nothing (spec/decisions/0023).
func canonicalizeEdgeSpec(form *InstalledForm, spec map[string]any) map[string]any {
	if form.Ref.APIVersion != edgeFormsGroup || form.Ref.Kind != workerCustomDomainKind {
		return spec
	}
	written, present := spec["hostname"].(string)
	if !present {
		return spec
	}
	canonical := currentformmodel.CanonicalHostname(written)
	if canonical == written {
		return spec
	}
	out := make(map[string]any, len(spec))
	for key, value := range spec {
		out[key] = value
	}
	out["hostname"] = canonical
	return out
}

// validateSingleHostnameClaim refuses a second Worker Custom Domain claiming a
// hostname another attachment already serves.
//
// A hostname is not a label inside this host's namespace; it is a name in
// DNS, and exactly one thing answers it. Two attachments claiming it would
// leave the host with two answers and no rule choosing between them, which is
// the incompleteness decision 0008 forbids — and, unlike a name collision,
// neither resource is wrong on its own, so no desired-state schema can see it.
//
// The comparison is on the CANONICAL hostname, which is the only spelling that
// reaches the store (canonicalizeEdgeSpec), so a host cannot be defeated by
// case or by a trailing dot.
//
// The scan spans every space, because the rule is per TENANT: spaces partition
// one tenant's resources, and DNS does not partition with them, so two spaces
// claiming one hostname is the same collision as two resources in one space.
// This reference host's resource store IS one tenant's, so scanning it whole
// is that per-tenant scan.
func (h *ReferenceHost) validateSingleHostnameClaim(space, name string, spec map[string]any) *hostError {
	hostname, _ := spec["hostname"].(string)
	if hostname == "" {
		return stableError("invalid_argument", "a WorkerCustomDomain requires a hostname")
	}
	selfKey := resourceKey(space, edgeFormsGroup, workerCustomDomainKind, name)
	for _, candidate := range h.sortedResources() {
		if candidate.group() != edgeFormsGroup || candidate.kind() != workerCustomDomainKind {
			continue
		}
		if candidate.key() == selfKey {
			continue
		}
		claimed, _ := candidate.Spec["hostname"].(string)
		if claimed != hostname {
			continue
		}
		return stableError(
			"invalid_argument",
			"hostname "+quoteText(hostname)+" is already served by WorkerCustomDomain "+candidate.Name+
				" in space "+quoteText(candidate.Space)+
				"; one DNS hostname has one answer, and the comparison is on the canonical spelling",
		)
	}
	return nil
}

// validateDeadLetterAcyclic refuses a dead-letter destination that leads back
// to the queue the messages came from.
//
// `edge.queue` says a message that exhausts its retries moves to the
// dead-letter queue as a NEW message, with a new identity and its attempt
// count starting again at 1 (decision 0020). If that destination drains back
// to the origin, the platform has built a loop that never terminates: the
// copy exhausts its retries, is dead-lettered onward, and arrives where it
// started with a fresh attempt count, forever. maxRetries bounds one message's
// deliveries; nothing bounds a cycle.
//
// The graph is over QUEUES, not consumers: the edge Q -> D exists when the
// consumer of Q declares D as its dead-letter destination. Because a queue has
// at most one consumer, a queue has at most one outgoing edge, so the walk from
// the proposed destination is a single path. It terminates on ANY graph shape
// for two independent reasons: `seen` admits each queue UID once, and the
// number of stored consumers is finite and never grows during the walk. A
// pre-existing cycle a laxer state left behind therefore ends the walk instead
// of running it forever.
func (h *ReferenceHost) validateDeadLetterAcyclic(
	space, name string,
	relations []storedRelation,
) *hostError {
	origin := relationTargetUID(relations, queueRelationPointer)
	destination := relationTargetUID(relations, deadLetterRelationPointer)
	if destination == "" {
		return nil
	}
	if destination == origin {
		return stableError(
			"invalid_argument",
			"the dead-letter queue resolves to the same AtLeastOnceQueue at uid "+origin+
				" this consumer drains; an exhausted message would be delivered back where it came from",
		)
	}
	// The consumer under test supersedes whatever it previously declared, so
	// its own stored edge is replaced rather than walked.
	selfKey := resourceKey(space, edgeFormsGroup, queueConsumerKind, name)
	successor := map[string]string{origin: destination}
	for _, candidate := range h.sortedResources() {
		if candidate.group() != edgeFormsGroup || candidate.kind() != queueConsumerKind ||
			candidate.key() == selfKey {
			continue
		}
		from := relationTargetUID(candidate.Relations, queueRelationPointer)
		to := relationTargetUID(candidate.Relations, deadLetterRelationPointer)
		if from == "" || to == "" {
			continue
		}
		if _, taken := successor[from]; !taken {
			successor[from] = to
		}
	}
	seen := map[string]bool{origin: true}
	path := []string{origin}
	for at := destination; at != ""; at = successor[at] {
		if at == origin {
			return stableError(
				"invalid_argument",
				"the dead-letter queue closes a cycle "+strings.Join(append(path, origin), " -> ")+
					"; an exhausted message would circulate forever instead of coming to rest",
			)
		}
		if seen[at] {
			return nil
		}
		seen[at] = true
		path = append(path, at)
	}
	return nil
}

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
func cronExpressionViolation(form *InstalledForm, spec map[string]any) string {
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
func validateCronExpression(form *InstalledForm, spec map[string]any) *hostError {
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
		if candidate.Space != space || candidate.group() != edgeFormsGroup ||
			candidate.kind() != queueConsumerKind {
			continue
		}
		if candidate.key() == selfKey {
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
