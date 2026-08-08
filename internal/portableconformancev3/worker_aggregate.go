package portableconformancev3

import (
	"sort"
	"strconv"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// worker_aggregate.go carries the Worker aggregate semantics of
// spec/decisions/0016: one Module Worker identity, the immutable Worker
// Versions holding its code, and the ONE Worker Deployment that decides which
// of those versions serve traffic.
//
// None of it is expressible in a desired-state schema. A schema cannot count
// the deployments pointing at one worker, cannot read the `/worker` relation of
// a version it does not contain, cannot add weights, and cannot know which
// handlers the code of a referenced version exports. Under decision 0014 those
// invariants therefore live in the host and in the conformance corpus.
const (
	moduleWorkerKind       = "ModuleWorker"
	workerVersionKind      = "WorkerVersion"
	workerDeploymentKind   = "WorkerDeployment"
	workerCustomDomainKind = "WorkerCustomDomain"
	workerCronTriggerKind  = "WorkerCronTrigger"
	queueConsumerKind      = "QueueConsumer"

	// workerRelationPointer is the concrete instance pointer of the `/worker`
	// reference every member of the aggregate declares. It is both the derived
	// relation pointer and the instance pointer, because no array stands
	// between the spec root and the reference.
	workerRelationPointer = "/worker"
	// bundleRelationPointer is the concrete instance pointer of the `/bundle`
	// reference a Worker Version declares: the ONE edge from a version to the
	// immutable code it runs.
	bundleRelationPointer = "/bundle"
	// deploymentVersionsRelation is the DERIVED relation pointer of a
	// deployment's weighted versions; concrete instances carry array indices,
	// which is why lookups match on Relation rather than Pointer.
	deploymentVersionsRelation = "/versions/*/workerVersion"
	// serviceBindingsRelation is the derived relation pointer of a version's
	// module-worker.service bindings — the one INBOUND reference a worker can
	// be the target of.
	serviceBindingsRelation = "/serviceBindings/*/resource"

	fetchHandler     = "fetch"
	scheduledHandler = "scheduled"
	queueHandler     = "queue"

	// The exact runtime ABI a ModuleWorker provides (spec/decisions/0019,
	// spec/interface-contract/). A host reads the handler vocabulary out of the
	// contract at this exact name rather than keeping a handler list of its
	// own, so widening the enum is a contract change with a new digest and not
	// a quiet host decision. The Interface name grammar admits no hyphen, which
	// is why the contract is `worker.runtime` and only the BINDING namespace
	// carries the hyphenated `module-worker.` prefix.
	workerRuntimeInterface = "worker.runtime"
	// runtimeHandlerOperation and runtimeHandlerProperty locate that
	// vocabulary: the item enum of the loadModule operation's declaredHandlers
	// property is the closed handler set of the ABI.
	runtimeHandlerOperation = "loadModule"
	runtimeHandlerProperty  = "declaredHandlers"

	// varsProperty and sensitiveVarsProperty are the two Worker Version
	// properties that project names into the module environment without being
	// binding lists. The binding lists themselves are discovered from the
	// served desired schema's `x-takoform-binding` annotation, so a sixth
	// binding list is covered without a host edit.
	varsProperty          = "vars"
	sensitiveVarsProperty = "requiredSensitiveVars"
)

// attachmentHandler maps each inward-activation Form to the module handler its
// events invoke. A custom domain sends HTTP requests to `fetch`, a cron
// trigger fires `scheduled`, and a queue consumer delivers batches to `queue`.
var attachmentHandler = map[string]string{
	workerCustomDomainKind: fetchHandler,
	workerCronTriggerKind:  scheduledHandler,
	queueConsumerKind:      queueHandler,
}

// sortedResources returns every stored resource in a stable key order, so
// every aggregate decision and every message this file produces is
// deterministic regardless of Go map iteration.
func (h *ReferenceHost) sortedResources() []*storedResource {
	keys := make([]string, 0, len(h.resources))
	for key := range h.resources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*storedResource, 0, len(keys))
	for _, key := range keys {
		out = append(out, h.resources[key])
	}
	return out
}

// activeDeployment returns the one WorkerDeployment governing one worker
// INCARNATION, or nil when that worker has none.
//
// The lookup is by the worker's UID, never by the name a deployment's spec
// spells: a name can be reused, and a deployment still pinned to a deleted
// worker describes traffic for a resource that no longer exists.
func (h *ReferenceHost) activeDeployment(space, workerUID string) *storedResource {
	if workerUID == "" {
		return nil
	}
	for _, candidate := range h.sortedResources() {
		if candidate.Space != space || candidate.group() != edgeFormsGroup ||
			candidate.kind() != workerDeploymentKind {
			continue
		}
		if relationTargetUID(candidate.Relations, workerRelationPointer) == workerUID {
			return candidate
		}
	}
	return nil
}

// weightedVersions resolves the stored Worker Version behind every weighted
// entry of one deployment relation set, in relation-pointer order. A relation
// whose target no longer resolves to the UID it was pinned to yields nothing:
// that deployment entry selects an incarnation that is gone, so it serves no
// handler at all.
func (h *ReferenceHost) weightedVersions(space string, relations []storedRelation) []*storedResource {
	var out []*storedResource
	for _, relation := range relations {
		if relation.Relation != deploymentVersionsRelation {
			continue
		}
		version := h.resources[resourceKey(space, relation.TargetAPIVersion, relation.TargetKind, relation.TargetName)]
		if version == nil || version.UID != relation.TargetUID {
			continue
		}
		out = append(out, version)
	}
	return out
}

// declaresHandler reports whether one stored Worker Version exports a module
// handler.
func declaresHandler(version *storedResource, handler string) bool {
	handlers, _ := version.Spec["handlers"].([]any)
	for _, declared := range handlers {
		if name, _ := declared.(string); name == handler {
			return true
		}
	}
	return false
}

// servesHandler reports whether EVERY weighted version exports one handler,
// naming the first that does not.
//
// Every, not some: traffic is split across the weighted set, so a request that
// any weighted version may answer has to find the handler in all of them. A
// deployment weighting no resolvable version serves nothing.
func servesHandler(versions []*storedResource, handler string) (offender string, serves bool) {
	if len(versions) == 0 {
		return "", false
	}
	for _, version := range versions {
		if !declaresHandler(version, handler) {
			return version.Name, false
		}
	}
	return "", true
}

// workerDependent is one live resource whose function depends on the worker's
// active deployment continuing to serve a particular handler.
type workerDependent struct {
	Kind    string
	Name    string
	Handler string
	// Detail names the exact reason, e.g. the inbound binding pointer.
	Detail string
}

// workerDependents lists every live resource that requires the active
// deployment of one worker incarnation to serve a handler: the three inward
// activation attachments, and every INBOUND module-worker.service binding held
// by another Form's Worker Version.
//
// The inbound binding belongs here because `worker.service` is provided by the
// worker IDENTITY (decision 0015): what a service binding actually reaches is
// whatever the target worker's active deployment selects, so a deployment that
// stops serving `fetch` silently breaks every caller bound to it.
func (h *ReferenceHost) workerDependents(space, workerUID string) []workerDependent {
	var out []workerDependent
	for _, candidate := range h.sortedResources() {
		if candidate.Space != space || candidate.group() != edgeFormsGroup {
			continue
		}
		if handler, attachment := attachmentHandler[candidate.kind()]; attachment {
			if relationTargetUID(candidate.Relations, workerRelationPointer) == workerUID {
				out = append(out, workerDependent{
					Kind: candidate.kind(), Name: candidate.Name, Handler: handler,
					Detail: "attachment",
				})
			}
			continue
		}
		if candidate.kind() != workerVersionKind {
			continue
		}
		for _, relation := range candidate.Relations {
			if relation.Relation == serviceBindingsRelation && relation.TargetUID == workerUID {
				out = append(out, workerDependent{
					Kind: candidate.kind(), Name: candidate.Name, Handler: fetchHandler,
					Detail: "inbound service binding " + relation.Pointer,
				})
			}
		}
	}
	return out
}

// validateWorkerAggregate is the one pre-mutation entry point for every rule
// decision 0016 states. It runs on apply and on import alike, and is re-run at
// async COMMIT time, exactly like relation resolution.
func (h *ReferenceHost) validateWorkerAggregate(
	form *InstalledForm,
	space, name string,
	spec map[string]any,
	relations []storedRelation,
) *hostError {
	if form.Ref.APIVersion != edgeFormsGroup {
		return nil
	}
	switch form.Ref.Kind {
	case workerDeploymentKind:
		return h.validateWorkerDeployment(space, name, relations)
	case workerVersionKind:
		if hostErr := h.exportedHandlerViolation(form, space, spec, relations); hostErr != nil {
			return hostErr
		}
		return h.validateInboundServiceBindings(space, relations)
	}
	if handler, attachment := attachmentHandler[form.Ref.Kind]; attachment {
		if hostErr := h.requireServingDeployment(
			space,
			nestedName(spec, "worker"),
			relationTargetUID(relations, workerRelationPointer),
			handler,
			form.Ref.Kind+" "+name,
		); hostErr != nil {
			return hostErr
		}
		// A queue consumer carries one more cardinality rule, and it is about the
		// QUEUE rather than the worker: edge.queue states that a queue has at most
		// one consumer (decision 0020).
		if form.Ref.Kind == queueConsumerKind {
			return h.validateSingleQueueConsumer(space, name, relations)
		}
	}
	return nil
}

// requireServingDeployment is the attachment gate: an attachment is admitted
// only when the worker it activates has an active deployment whose every
// weighted version exports the handler the attachment invokes.
//
// The evidence is the DEPLOYMENT, not any stored version. A version that no
// deployment selects is code nothing runs, so admitting a cron trigger on its
// word would activate a schedule against a handler no deployed version
// exports. The failure is `unsupported_capability` rather than
// `invalid_argument` because the request is well formed: what is missing is
// the capability the worker currently offers.
func (h *ReferenceHost) requireServingDeployment(
	space, workerName, workerUID, handler, subject string,
) *hostError {
	if workerName == "" || workerUID == "" {
		return stableError("invalid_argument", subject+" requires a target worker")
	}
	deployment := h.activeDeployment(space, workerUID)
	if deployment == nil {
		return stableError(
			"unsupported_capability",
			subject+" requires the active WorkerDeployment of ModuleWorker "+workerName+" at uid "+
				workerUID+", which has none; no version of that worker serves the "+handler+" handler",
		)
	}
	offender, serves := servesHandler(h.weightedVersions(space, deployment.Relations), handler)
	if serves {
		return nil
	}
	detail := "it weights no resolvable WorkerVersion"
	if offender != "" {
		detail = "weighted WorkerVersion " + offender + " does not export it"
	}
	return stableError(
		"unsupported_capability",
		subject+" requires every version weighted by WorkerDeployment "+deployment.Name+
			" to export the "+handler+" handler; "+detail,
	)
}

// validateInboundServiceBindings refuses a module-worker.service binding to a
// worker that serves nothing yet.
//
// A binding projects an Interface, and `worker.service` is an Interface the
// worker identity only actually provides once its active deployment serves
// `fetch`. Refusing at BIND time rather than reporting the binding source
// not-Ready is deliberate: the first Forms should be simple to reason about,
// and a stored binding that projects nothing is a resource whose declared
// capability is a promise no host can keep.
func (h *ReferenceHost) validateInboundServiceBindings(space string, relations []storedRelation) *hostError {
	for _, relation := range relations {
		if relation.Relation != serviceBindingsRelation {
			continue
		}
		if hostErr := h.requireServingDeployment(
			space, relation.TargetName, relation.TargetUID, fetchHandler,
			"the module-worker.service binding at "+relation.Pointer,
		); hostErr != nil {
			return hostErr
		}
	}
	return nil
}

// validateWorkerDeployment proves every rule that makes a deployment the one
// coherent answer to "what serves this worker": exactly one deployment per
// worker, a weighted set that belongs to that worker, names each version once,
// and weights only versions a host can actually run — and it refuses a change
// that would leave a live dependent unserved.
func (h *ReferenceHost) validateWorkerDeployment(
	space, name string,
	relations []storedRelation,
) *hostError {
	workerUID := relationTargetUID(relations, workerRelationPointer)
	if workerUID == "" {
		return stableError("invalid_argument", "a WorkerDeployment requires a target worker")
	}
	selfKey := resourceKey(space, edgeFormsGroup, workerDeploymentKind, name)
	// A. One active deployment per worker. Two deployments of one worker leave
	//    "which one serves" undefined, and no rule chosen after the fact — newest,
	//    lowest name, highest weight — is a rule an author can predict.
	if existing := h.activeDeployment(space, workerUID); existing != nil {
		key := existing.key()
		if key != selfKey {
			return stableError(
				"invalid_argument",
				"ModuleWorker at uid "+workerUID+" already has the active WorkerDeployment "+
					existing.Name+"; a worker has exactly one, and traffic moves by re-weighting it",
			)
		}
	}
	// B. Deployment integrity: ownership, uniqueness, and runnable versions.
	seen := map[string]string{}
	weighted := 0
	for _, relation := range relations {
		if relation.Relation != deploymentVersionsRelation {
			continue
		}
		weighted++
		version := h.resources[resourceKey(space, relation.TargetAPIVersion, relation.TargetKind, relation.TargetName)]
		if version == nil || version.UID != relation.TargetUID {
			// Relation resolution already pinned this UID moments ago, so this is
			// unreachable through the API; refusing keeps it unreachable.
			return stableError(
				"invalid_argument",
				"relation "+relation.Pointer+" no longer resolves to WorkerVersion "+
					relation.TargetName+" at uid "+relation.TargetUID,
			)
		}
		if previous, duplicate := seen[relation.TargetUID]; duplicate {
			return stableError(
				"invalid_argument",
				"relation "+relation.Pointer+" weights WorkerVersion "+version.Name+
					" a second time; "+previous+" already weights it, and one version has one share",
			)
		}
		seen[relation.TargetUID] = relation.Pointer
		if owner := relationTargetUID(version.Relations, workerRelationPointer); owner != workerUID {
			return stableError(
				"invalid_argument",
				"relation "+relation.Pointer+" weights WorkerVersion "+version.Name+
					", which belongs to the worker at uid "+owner+", not to the worker at uid "+workerUID,
			)
		}
		if reason, ok := h.versionUnavailable(version); !ok {
			return stableError(
				"invalid_argument",
				"relation "+relation.Pointer+" weights WorkerVersion "+version.Name+
					", which cannot serve traffic: "+reason,
			)
		}
	}
	if weighted == 0 {
		return stableError("invalid_argument", "a WorkerDeployment requires at least one weighted version")
	}
	// D. Reverse validation: the deployment being applied must keep every live
	//    dependent of this worker satisfied.
	versions := h.weightedVersions(space, relations)
	for _, dependent := range h.workerDependents(space, workerUID) {
		offender, serves := servesHandler(versions, dependent.Handler)
		if serves {
			continue
		}
		detail := "it weights no resolvable WorkerVersion"
		if offender != "" {
			detail = "weighted WorkerVersion " + offender + " does not export it"
		}
		return stableError(
			"unsupported_capability",
			"this WorkerDeployment would stop serving the "+dependent.Handler+" handler that live "+
				dependent.Kind+" "+dependent.Name+" ("+dependent.Detail+") depends on; "+detail,
		)
	}
	return nil
}

// versionUnavailable reports why a Worker Version cannot be put into service.
//
// A deployment is a claim about what runs. Weighting a version whose own
// relations no longer resolve, or one an accepted delete is already removing,
// would make that claim false the moment it was stored.
func (h *ReferenceHost) versionUnavailable(version *storedResource) (string, bool) {
	if reason, hostReason, drifted := h.relationDrift(version); drifted {
		return "it is not Ready (" + reason + ": " + hostReason + ")", false
	}
	key := version.key()
	if h.deletionPending(key) {
		return "an accepted delete operation is already removing it", false
	}
	return "", true
}

// deletionPending reports whether an accepted-but-unfinished delete Operation
// targets this exact resource. The record lives on the operation rather than on
// the resource, so a cancelled operation releases it without any extra
// bookkeeping.
func (h *ReferenceHost) deletionPending(key string) bool {
	for _, operation := range h.operations {
		if !operation.Done && operation.DeleteTarget == key {
			return true
		}
	}
	return false
}

// deploymentDeleteBlocked refuses to delete a WorkerDeployment while any live
// dependent needs it.
//
// Failing closed is the whole point. The alternative — letting the delete
// through and degrading every attachment to not-Ready — turns one explicit
// action into a fan-out of broken resources whose repair order the author has
// to work out. Removing the dependents first is a decision the author states.
func (h *ReferenceHost) deploymentDeleteBlocked(resource *storedResource) *hostError {
	if resource.group() != edgeFormsGroup || resource.kind() != workerDeploymentKind {
		return nil
	}
	workerUID := relationTargetUID(resource.Relations, workerRelationPointer)
	dependents := h.workerDependents(resource.Space, workerUID)
	if len(dependents) == 0 {
		return nil
	}
	names := make([]string, 0, len(dependents))
	for _, dependent := range dependents {
		names = append(names, dependent.Kind+" "+dependent.Name+" ("+dependent.Handler+")")
	}
	return stableError(
		"dependency_in_use",
		"this WorkerDeployment is what serves "+strconv.Itoa(len(dependents))+
			" live dependent(s): "+strings.Join(names, ", ")+
			"; remove them before removing what serves them",
	)
}

// workerServiceUnavailable reports why a Module Worker is not Ready.
//
// A worker's readiness is a statement about SERVICE, not about the existence of
// a row: `worker.service` is provided by the identity, and what answers a
// request is whatever the active deployment selects. A worker with no
// deployment exists and serves nothing, which is `Provisioning`; a worker whose
// deployed versions export no `fetch` is deployed but cannot answer a request,
// which is `UnsupportedCapability`.
func (h *ReferenceHost) workerServiceUnavailable(resource *storedResource) (reason, hostReason string, unavailable bool) {
	if resource.group() != edgeFormsGroup || resource.kind() != moduleWorkerKind {
		return "", "", false
	}
	deployment := h.activeDeployment(resource.Space, resource.UID)
	if deployment == nil {
		return "Provisioning",
			"ModuleWorker " + resource.Name + " at uid " + resource.UID +
				" has no active WorkerDeployment, so it serves no traffic and provides no worker.service interface yet",
			true
	}
	offender, serves := servesHandler(h.weightedVersions(resource.Space, deployment.Relations), fetchHandler)
	if serves {
		return "", "", false
	}
	detail := "it weights no resolvable WorkerVersion"
	if offender != "" {
		detail = "weighted WorkerVersion " + offender + " exports no fetch handler"
	}
	return "UnsupportedCapability",
		"WorkerDeployment " + deployment.Name + " does not serve the fetch handler for ModuleWorker " +
			resource.Name + " at uid " + resource.UID + "; " + detail,
		true
}

// ---- runtime ABI ----

// runtimeContract resolves the exact ES Module Worker runtime ABI this host
// implements: the contract the installed ModuleWorker Form Definition declares
// in providedInterfaces, held at the exact digest the host installed. It
// returns false when the lane's ModuleWorker Form declares no runtime contract
// at all, which is only the minimal in-memory FallbackCatalog used by targeted
// unit tests; the real LoadCatalog always installs it.
// The catalog is keyed by the EXACT identity, so "the installed ModuleWorker
// Form" is no longer a single thing to look up: one Form line may hold more
// than one definition, and a host that picked whichever it found first would
// advertise a vocabulary some of its installed contracts do not have. The
// runtime ABI is therefore the one contract every installed Form providing
// worker.runtime agrees on. Zero providers means this host implements no such
// ABI; two DIFFERENT ones mean it cannot say which it implements, and both fail
// closed rather than guess (decision 0022).
func (h *ReferenceHost) runtimeContract() (interfaceContract, bool) {
	var resolved interfaceContract
	found := false
	for _, form := range h.catalog.sortedForms() {
		for _, provided := range form.ProvidedInterfaces {
			if provided.Name != workerRuntimeInterface {
				continue
			}
			contract, installed := h.catalog.interfaceContractByName(provided.Name, provided.Version)
			if !installed || contract.Ref.SchemaDigest != provided.SchemaDigest {
				return interfaceContract{}, false
			}
			if found && contract.Ref != resolved.Ref {
				return interfaceContract{}, false
			}
			resolved, found = contract, true
		}
	}
	return resolved, found
}

// declaredHandlerViolation reports why one Worker Version's declared handlers
// are not the runtime ABI's, or "" when they are.
//
// `handlers` is the surface a host attaches inward activation to, and the ABI
// is what says which handlers exist at all: fetch, scheduled, queue, and tail,
// each with a fixed signature (spec/decisions/0019). Accepting anything else
// would store a version claiming an entry point no conforming runtime can
// invoke, and would let one host attach events to it while another could not —
// the divergence the exact contract exists to close.
//
// The vocabulary is read out of the INSTALLED CONTRACT, never out of a list the
// host keeps: a host that widened its handler set would have to publish a
// different runtime contract at a different digest, which the
// module-worker-runtime-contract-advertised check reads back. That is also why
// the rule is not left to the Form's `handlers` enum: the enum is a structural
// minimum derived from this contract, and a host whose installed schema drifted
// laxer must still refuse (decision 0014).
//
// It is a property of the spec ALONE — no other resource has to resolve — so it
// is reported as a desired-spec diagnostic and therefore reaches the advisory
// `validate` surface, the binding `prepare` surface, and `apply` alike. A client
// learns the version is unrunnable while planning, not after a failed apply.
func (h *ReferenceHost) declaredHandlerViolation(form *InstalledForm, spec map[string]any) string {
	if form.Ref.APIVersion != edgeFormsGroup || form.Ref.Kind != workerVersionKind {
		return ""
	}
	contract, installed := h.runtimeContract()
	if !installed || len(contract.Handlers) == 0 {
		return ""
	}
	defined := map[string]bool{}
	for _, handler := range contract.Handlers {
		defined[handler] = true
	}
	declared, _ := spec["handlers"].([]any)
	for _, item := range declared {
		handler, _ := item.(string)
		if defined[handler] {
			continue
		}
		return "WorkerVersion declares handler " + strconv.Quote(handler) + ", which the runtime contract " +
			contract.Ref.Name + "@" + contract.Ref.Version + " (" + contract.Ref.SchemaDigest + ") does not define; " +
			"the ABI defines exactly " + strings.Join(contract.Handlers, ", ")
	}
	return ""
}

// validateDeclaredHandlers is the mutation-path form of the same rule. It is
// deliberately redundant with the diagnostic: import and the asynchronous commit
// re-derive every precondition here, long after the diagnostics of the accepting
// request were computed.
func (h *ReferenceHost) validateDeclaredHandlers(form *InstalledForm, spec map[string]any) *hostError {
	if violation := h.declaredHandlerViolation(form, spec); violation != "" {
		return stableError("invalid_argument", violation)
	}
	return nil
}

// exportedHandlerViolation refuses a Worker Version declaring a handler the
// module it actually runs does not export.
//
// The `worker.runtime` contract is explicit that this fails the VERSION rather
// than a request: `loadModule` answers `handler_not_exported` before any traffic
// arrives, so a stored version in this state is code that can never serve, and
// the attachment gate would go on admitting a cron trigger or a queue consumer
// against a handler that does not exist. The refusal is `invalid_argument`
// because, exactly like the deployment-integrity rules, the request is well
// formed and states something untrue about what will run.
//
// It is a CROSS-RESOURCE rule, not a spec-shape one: the answer lives in the
// version's `/bundle` relation, in the manifest that bundle's digest addresses,
// and in the bytes of that manifest's main module. It therefore runs where
// every other relation-reading rule runs — before any mutation on apply and on
// import alike, and again when a 202 commits — and not on the advisory
// `validate` surface, which resolves nothing.
//
// What a module exports is a fact about JavaScript, and this reference host
// executes none: it holds the map from a module's content address to the
// handlers that module's default export exposes, declared by the corpus that
// pinned those exact bytes. A real host derives the same fact by loading the
// module into an isolate, which is the only general way to know it. A module
// whose bytes this host was never told about is therefore NOT refused — the
// reference host answers only about code it knows, and never guesses. That
// boundary is why the lane proves the refusal for a pinned bundle rather than
// claiming to prove it for every possible one; see spec/host-api/v1alpha3.md.
func (h *ReferenceHost) exportedHandlerViolation(
	form *InstalledForm, space string, spec map[string]any, relations []storedRelation,
) *hostError {
	if form.Ref.APIVersion != edgeFormsGroup || form.Ref.Kind != workerVersionKind {
		return nil
	}
	exported, known := h.bundleModuleExports(space, relations)
	if !known {
		return nil
	}
	exports := map[string]bool{}
	for _, handler := range exported {
		exports[handler] = true
	}
	for _, item := range anyStringSlice(spec["handlers"]) {
		if exports[item] {
			continue
		}
		return stableError("invalid_argument",
			"WorkerVersion declares handler "+strconv.Quote(item)+
				", which the main module of the WorkerBundle it references does not export; "+
				"that module exports exactly "+strings.Join(exported, ", ")+
				", and the runtime contract fails a declared handler the module does not export "+
				"(handler_not_exported) before any traffic arrives")
	}
	return nil
}

// bundleModuleExports resolves what the main module of the bundle one Worker
// Version references exports, and reports whether this host knows at all.
//
// Every step is a fact the host already holds: the relation the apply just
// resolved pins the bundle INCARNATION, the bundle's desired state is the
// manifest digest, the committed manifest names its main module, and that
// module entry carries the content address of the bytes. Nothing is read from
// the version's spec but the relation it produced, so a renamed or replaced
// bundle is a different answer rather than the same one.
func (h *ReferenceHost) bundleModuleExports(space string, relations []storedRelation) ([]string, bool) {
	bundleUID := relationTargetUID(relations, bundleRelationPointer)
	if bundleUID == "" {
		return nil, false
	}
	var bundle *storedResource
	for _, relation := range relations {
		if relation.Pointer != bundleRelationPointer {
			continue
		}
		bundle = h.resources[resourceKey(space, relation.TargetAPIVersion, relation.TargetKind, relation.TargetName)]
	}
	if bundle == nil || bundle.UID != bundleUID {
		return nil, false
	}
	manifestDigest, _ := bundle.Spec["manifestDigest"].(string)
	raw := h.manifests[manifestDigest]
	if raw == nil {
		return nil, false
	}
	var manifest artifactManifest
	if err := formpackage.DecodeStrictIJSON(raw, &manifest); err != nil {
		return nil, false
	}
	for _, module := range manifest.Modules {
		if module.Name != manifest.MainModule {
			continue
		}
		exported, known := h.moduleExports[module.Digest]
		return exported, known
	}
	return nil, false
}

// desiredSchemaEnum reads the item enum of one top-level string-array property
// of a desired schema. It is the profile's last resort when no runtime contract
// is installed; it never overrides one.
func desiredSchemaEnum(schema map[string]any, property string) []string {
	properties, _ := schema["properties"].(map[string]any)
	node, _ := properties[property].(map[string]any)
	items, _ := node["items"].(map[string]any)
	values, _ := items["enum"].([]any)
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

// ---- environment namespace ----

// validateEnvironmentNamespace proves that one Worker Version's declared names
// occupy the single runtime environment namespace exactly once.
//
// `vars` keys, `requiredSensitiveVars` entries, and every binding `name` are
// all projected into the ONE environment object the module receives, so two of
// them sharing a name is a specification of two different values under one
// identifier. The desired schema cannot see it: `uniqueItems` rejects a
// duplicated whole object, not two objects that agree only on `name`, and it
// has no reach across sibling properties at all.
//
// The binding lists are discovered from the served desired schema's
// `x-takoform-binding` annotation rather than named here, so a Form that gains
// a sixth binding list is covered without a host edit. The two non-binding
// sources are named, because nothing in the schema distinguishes a data map
// that projects environment names from one that does not.
func validateEnvironmentNamespace(form *InstalledForm, spec map[string]any) *hostError {
	if form.Ref.APIVersion != edgeFormsGroup || form.Ref.Kind != workerVersionKind {
		return nil
	}
	claimed := map[string]string{}
	claim := func(name, source string) *hostError {
		if previous, taken := claimed[name]; taken {
			return stableError(
				"invalid_argument",
				"environment name "+strconv.Quote(name)+" is declared by both "+previous+" and "+source+
					"; vars, requiredSensitiveVars, and every binding list project into one module "+
					"environment namespace, so a name belongs to exactly one of them",
			)
		}
		claimed[name] = source
		return nil
	}
	if vars, ok := spec[varsProperty].(map[string]any); ok {
		keys := make([]string, 0, len(vars))
		for key := range vars {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if hostErr := claim(key, varsProperty); hostErr != nil {
				return hostErr
			}
		}
	}
	if required, ok := spec[sensitiveVarsProperty].([]any); ok {
		for index, item := range required {
			name, _ := item.(string)
			if name == "" {
				continue
			}
			if hostErr := claim(name, sensitiveVarsProperty+"/"+strconv.Itoa(index)); hostErr != nil {
				return hostErr
			}
		}
	}
	for _, property := range bindingListProperties(form.DesiredSchema) {
		entries, _ := spec[property].([]any)
		for index, raw := range entries {
			entry, _ := raw.(map[string]any)
			name, _ := entry["name"].(string)
			if name == "" {
				continue
			}
			if hostErr := claim(name, property+"/"+strconv.Itoa(index)); hostErr != nil {
				return hostErr
			}
		}
	}
	return nil
}

// bindingListProperties names every top-level property of one desired schema
// that carries the Binding contract annotation, in a stable order.
func bindingListProperties(schema map[string]any) []string {
	properties, _ := schema["properties"].(map[string]any)
	out := make([]string, 0, len(properties))
	for name, raw := range properties {
		node, _ := raw.(map[string]any)
		if node == nil {
			continue
		}
		if contract, _ := node[currentformmodel.BindingAnnotationKey].(string); contract != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
