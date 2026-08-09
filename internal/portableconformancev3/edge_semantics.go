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

const (
	assetBundleRelationPointer  = "/assets/bundle"
	migrationDatabasePointer    = "/database"
	migrationSetRelationPointer = "/migrationSet"
)

// migrationLedgerEntry is the portable identity of one applied migration.
// SQL bytes remain in the content-addressed artifact store; durable database
// history records only the ordered path and digest needed to detect a rewrite,
// reorder, or removal.
type migrationLedgerEntry struct {
	Path   string
	Digest string
}

func (h *ReferenceHost) relationTargetResource(
	scope resourceScope, relations []storedRelation, pointer string,
) *storedResource {
	for _, relation := range relations {
		if relation.Pointer != pointer {
			continue
		}
		target := h.resources[resourceKey(scope, relation.TargetAPIVersion, relation.TargetKind, relation.TargetName)]
		if target != nil && target.UID == relation.TargetUID && target.Ref == relation.TargetRef {
			return target
		}
	}
	return nil
}

// validateWorkerVersionAssets proves the one asset rule a desired-state schema
// cannot see: SPA fallback requires index.html to exist in the exact referenced
// StaticAssetBundle manifest. The ordinary relation resolver already proves
// the exact FormRef and pins the bundle incarnation.
func (h *ReferenceHost) validateWorkerVersionAssets(
	caller hostAuthContext,
	form *InstalledForm,
	scope resourceScope,
	spec map[string]any,
	relations []storedRelation,
) *hostError {
	if form.Ref.APIVersion != edgeFormsGroup || form.Ref.Kind != workerVersionKind {
		return nil
	}
	assets, present := spec["assets"].(map[string]any)
	if !present {
		return nil
	}
	bundle := h.relationTargetResource(scope, relations, assetBundleRelationPointer)
	if bundle == nil {
		return stableError("resource_not_found", "WorkerVersion assets relation does not resolve to its pinned StaticAssetBundle")
	}
	manifest, hostErr := h.requireReferencedArtifactManifest(caller, bundle.Spec, staticAssetBundleKind)
	if hostErr != nil {
		return hostErr
	}
	if handling, _ := assets["notFoundHandling"].(string); handling == "single_page_application" {
		for _, file := range manifest.Files {
			if file.Path == "index.html" {
				return nil
			}
		}
		return stableError("invalid_argument", "single_page_application requires index.html in the referenced StaticAssetBundle")
	}
	return nil
}

// sqliteMigrationPlan resolves one application to the exact database and
// MigrationBundle it pins, then proves the durable database ledger is an exact
// prefix of that ordered manifest. Anything else is a rewrite, reorder, or
// removal of applied history and is never repaired by replaying SQL.
func (h *ReferenceHost) sqliteMigrationPlan(
	caller hostAuthContext,
	form *InstalledForm,
	scope resourceScope,
	relations []storedRelation,
) (string, []migrationLedgerEntry, *hostError) {
	if form.Ref.APIVersion != edgeFormsGroup || form.Ref.Kind != sqliteMigrationApplicationKind {
		return "", nil, nil
	}
	database := h.relationTargetResource(scope, relations, migrationDatabasePointer)
	set := h.relationTargetResource(scope, relations, migrationSetRelationPointer)
	if database == nil || set == nil {
		return "", nil, stableError("resource_not_found", "SQLiteMigrationApplication relations do not resolve to their pinned resources")
	}
	manifest, hostErr := h.requireReferencedArtifactManifest(caller, set.Spec, migrationBundleKind)
	if hostErr != nil {
		return "", nil, hostErr
	}
	desired := make([]migrationLedgerEntry, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		desired = append(desired, migrationLedgerEntry{Path: file.Path, Digest: file.Digest})
	}
	applied := h.migrationLedgers[database.UID]
	if len(applied) > len(desired) {
		return "", nil, stableError("migration_required", "the migration set removes already-applied database history")
	}
	for index, prior := range applied {
		if desired[index] != prior {
			return "", nil, stableError("migration_required", "the migration set rewrites or reorders already-applied database history")
		}
	}
	return database.UID, desired, nil
}

func (h *ReferenceHost) validateSQLiteMigrationApplication(
	caller hostAuthContext,
	form *InstalledForm,
	scope resourceScope,
	relations []storedRelation,
) *hostError {
	_, _, hostErr := h.sqliteMigrationPlan(caller, form, scope, relations)
	return hostErr
}

// applySQLiteMigrationSuffix records only the unapplied suffix. A production
// host executes each file and appends its ledger entry in one database
// transaction; this deterministic reference host has no SQL engine, so its
// portable control-plane evidence is the prefix/suffix state machine itself.
func (h *ReferenceHost) applySQLiteMigrationSuffix(
	caller hostAuthContext,
	form *InstalledForm,
	scope resourceScope,
	relations []storedRelation,
) *hostError {
	databaseUID, desired, hostErr := h.sqliteMigrationPlan(caller, form, scope, relations)
	if hostErr != nil || databaseUID == "" {
		return hostErr
	}
	prior := h.migrationLedgers[databaseUID]
	h.migrationLedgers[databaseUID] = append(append([]migrationLedgerEntry(nil), prior...), desired[len(prior):]...)
	return nil
}

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
// re-apply under any other spelling moves nothing (spec/decisions/0026).
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
// The scope is the CALLER'S TENANT, and both halves of that are the rule.
//
// It spans every space of that tenant, because spaces partition one tenant's
// resources and DNS does not partition with them: two spaces claiming one
// hostname is the same collision as two resources in one space. This is the ONE
// deliberate exception to the tenant-and-space address of spec/decisions/0028:
// every other scan takes a whole resourceScope, and this one drops the space on
// purpose and says so.
//
// It stops at the tenant, because what one tenant may claim is a question about
// who controls the name — authority this contract does not pretend to answer —
// so a hostname another tenant serves is none of this scan's business. Every
// other cross-resource rule in this file is decided on a host-issued uid, which
// already names one resource inside one tenant; a hostname is a name DNS owns,
// so the comparison carries no boundary of its own and a scan over the store
// alone would silently enforce the rule host-wide (spec/decisions/0026).
func (h *ReferenceHost) validateSingleHostnameClaim(
	scope resourceScope,
	name string,
	spec map[string]any,
) *hostError {
	hostname, _ := spec["hostname"].(string)
	if hostname == "" {
		return stableError("invalid_argument", "a WorkerCustomDomain requires a hostname")
	}
	selfKey := resourceKey(scope, edgeFormsGroup, workerCustomDomainKind, name)
	for _, candidate := range h.sortedResources() {
		if candidate.Tenant != scope.Tenant {
			continue
		}
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
//
// The successor map is built from every stored consumer OF THIS TENANT, and is
// deliberately given no space filter of its own. Its keys are queue UIDs, and a
// uid names ONE queue incarnation: the walk starts from a uid this caller's own
// relation resolution produced, and every edge it can follow is an edge out of a
// uid it has already arrived at, so a consumer in another space contributes edges
// between uids the walk can never reach. Those are a disconnected component
// rather than a leak. Narrowing to the space would not make the answer more
// correct — it would only hide the fact that a cycle is decided on resolved
// identity, which is the whole of decision 0026. The hostname claim is the one
// rule here that cannot borrow that property, because it compares a name rather
// than a uid.
//
// The TENANT filter is different and is not about correctness of the cycle. The
// walk could not reach another tenant's edges anyway, because a relation resolves
// inside its own tenant and no foreign consumer can pin a uid of this one
// (spec/decisions/0028). It is here so that this scan does not READ another
// tenant's stored relations to decide a question about this one — a scan that is
// merely safe today is one refactor away from not being.
func (h *ReferenceHost) validateDeadLetterAcyclic(
	scope resourceScope,
	name string,
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
	selfKey := resourceKey(scope, edgeFormsGroup, queueConsumerKind, name)
	successor := map[string]string{origin: destination}
	for _, candidate := range h.sortedResources() {
		if candidate.Tenant != scope.Tenant {
			continue
		}
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
// consumer of the queue that exists now. That is also what scopes the rule: a
// uid names one queue incarnation inside one tenant, so the scope filter below
// narrows the scan without deciding it. Two consumers of ONE queue are two
// consumers of one queue whoever applied them — and only one tenant can ever
// hold a relation to that queue's uid, because relations resolve inside the
// caller's own scope (spec/decisions/0028).
func (h *ReferenceHost) validateSingleQueueConsumer(
	scope resourceScope,
	name string,
	relations []storedRelation,
) *hostError {
	queueUID := relationTargetUID(relations, queueRelationPointer)
	if queueUID == "" {
		return stableError("invalid_argument", "a QueueConsumer requires a target queue")
	}
	selfKey := resourceKey(scope, edgeFormsGroup, queueConsumerKind, name)
	for _, candidate := range h.scopedResources(scope) {
		if candidate.group() != edgeFormsGroup || candidate.kind() != queueConsumerKind {
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
