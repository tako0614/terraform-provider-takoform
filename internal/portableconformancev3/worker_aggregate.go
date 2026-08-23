package portableconformancev3

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
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
	workerEndpointKind     = "WorkerEndpoint"
	workerCronTriggerKind  = "WorkerCronTrigger"
	queueConsumerKind      = "QueueConsumer"
	durableWorkflowKind    = "DurableWorkflow"
	actorNamespaceKind     = "ActorNamespace"

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

// requiredEntrypoint reports which module handler an inward-activation Form's
// events invoke, READ OUT OF the installed Definition's
// `x-takoform-required-entrypoint` annotation rather than out of a table kept
// here. A custom domain and a host-assigned endpoint both send HTTP requests to
// `fetch`, a cron trigger fires `scheduled`, and a queue consumer delivers
// batches to `queue` — but this host does not know that, and a family adding a
// fifth attachment is covered without a host edit, exactly as a sixth binding
// list already is.
//
// A Worker Endpoint carries the annotation for the same reason the other three
// do, and every consequence follows from carrying it rather than from an
// endpoint-shaped special case: it is gated on the active deployment serving
// `fetch`, it is a dependent that blocks a deployment change that would stop
// serving `fetch`, and it blocks that deployment's deletion.
func requiredEntrypoint(form *InstalledForm) (string, bool) {
	if form == nil {
		return "", false
	}
	properties, _ := form.DesiredSchema["properties"].(map[string]any)
	node, _ := properties["worker"].(map[string]any)
	if node == nil {
		return "", false
	}
	entrypoint, _ := node[currentformmodel.RequiredEntrypointAnnotationKey].(string)
	return entrypoint, entrypoint != ""
}

// classHolderKinds are the members of the aggregate that name a CLASS the
// serving module must export, rather than a handler.
//
// They are deliberately not in attachmentHandler, and the difference is not
// cosmetic. An attachment refuses to exist at all until the deployment serves
// its handler, because an activation with nothing to activate is a promise no
// host can keep. A workflow or an actor namespace is the other way round: its
// own worker's deployment cannot exist before the version that deployment
// weights, so refusing before any deployment would make the ordinary
// self-bound wiring unconstructible in a single apply. What it refuses is a
// LIVE deployment that visibly lacks the class; with no deployment at all it
// stores and reports Provisioning.
var classHolderKinds = map[string]bool{
	durableWorkflowKind: true,
	actorNamespaceKind:  true,
}

// sortedResources returns every stored resource of EVERY tenant in a stable key
// order, so every scan built on it is deterministic regardless of Go map
// iteration.
//
// It is the unfiltered view, and almost nothing should use it. A rule decided
// over the store is a rule about one tenant's plane, so the two callers that take
// this view are the two that say why in their own doc comments: the tenant-wide
// hostname claim scan, which supplies its own tenant filter, and the dead-letter
// successor map, which is keyed by uid and can therefore only ever be walked into
// from inside the tenant that owns those uids. Everything else takes
// scopedResources (spec/decisions/0028).
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

// scopedResources is the store as ONE scope sees it: the resources of one tenant
// in one space, in a stable key order. Every cross-resource scan that decides a
// mutation, and the derived-rendering pass that writes revisions, iterate this
// rather than the whole host.
func (h *ReferenceHost) scopedResources(scope resourceScope) []*storedResource {
	out := make([]*storedResource, 0, len(h.resources))
	for _, candidate := range h.sortedResources() {
		if candidate.Tenant != scope.Tenant || candidate.Space != scope.Space {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

// activeDeployment returns the one WorkerDeployment governing one worker
// INCARNATION, or nil when that worker has none.
//
// The lookup is by the worker's UID, never by the name a deployment's spec
// spells: a name can be reused, and a deployment still pinned to a deleted
// worker describes traffic for a resource that no longer exists.
func (h *ReferenceHost) activeDeployment(scope resourceScope, workerUID string) *storedResource {
	if workerUID == "" {
		return nil
	}
	for _, candidate := range h.scopedResources(scope) {
		if candidate.group() != h.edgeGroup() || candidate.kind() != workerDeploymentKind {
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
func (h *ReferenceHost) weightedVersions(scope resourceScope, relations []storedRelation) []*storedResource {
	var out []*storedResource
	for _, relation := range relations {
		if relation.Relation != deploymentVersionsRelation {
			continue
		}
		version := h.resources[resourceKey(scope, relation.TargetAPIVersion, relation.TargetKind, relation.TargetName)]
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
	// Class is set instead of Handler when the dependent needs a class export
	// rather than a module handler. Exactly one of the two is ever set.
	Class string
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
func (h *ReferenceHost) workerDependents(scope resourceScope, workerUID string) []workerDependent {
	var out []workerDependent
	for _, candidate := range h.scopedResources(scope) {
		if candidate.group() != h.edgeGroup() {
			continue
		}
		if handler, attachment := requiredEntrypoint(h.catalog.exact(candidate.Ref)); attachment {
			if relationTargetUID(candidate.Relations, workerRelationPointer) == workerUID {
				out = append(out, workerDependent{
					Kind: candidate.kind(), Name: candidate.Name, Handler: handler,
					Detail: "attachment",
				})
			}
			continue
		}
		if classHolderKinds[candidate.kind()] {
			if relationTargetUID(candidate.Relations, workerRelationPointer) == workerUID {
				className, _ := candidate.Spec["className"].(string)
				out = append(out, workerDependent{
					Kind: candidate.kind(), Name: candidate.Name, Class: className,
					Detail: "class holder",
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
//
// The SCOPE travels with the name because every rule below is decided inside one
// tenant. Most of them are decided on a host-issued UID — one deployment per
// worker INCARNATION, one consumer per queue INCARNATION — and a uid names one
// resource inside one tenant, so those scans could not reach past it even
// without the filter; they are scoped anyway, because a scan that only happens
// to be safe is one refactor away from not being. The hostname claim is the
// deliberate exception in the other direction: it compares a name DNS owns
// rather than a uid this host issued, so it spans every space OF THE TENANT and
// takes the tenant alone (spec/decisions/0026, 0028).
func (h *ReferenceHost) validateWorkerAggregate(
	form *InstalledForm,
	scope resourceScope,
	name string,
	spec map[string]any,
	relations []storedRelation,
) *hostError {
	if form.Ref.APIVersion != h.edgeGroup() {
		return nil
	}
	switch form.Ref.Kind {
	case workerDeploymentKind:
		return h.validateWorkerDeployment(scope, name, relations)
	case workerVersionKind:
		if hostErr := h.exportedHandlerViolation(form, scope, spec, relations); hostErr != nil {
			return hostErr
		}
		return h.validateInboundServiceBindings(scope, relations)
	}
	if classHolderKinds[form.Ref.Kind] {
		return h.validateWorkerClassHolder(form, scope, name, spec, relations)
	}
	if handler, attachment := requiredEntrypoint(form); attachment {
		if hostErr := h.requireServingDeployment(
			scope,
			nestedName(spec, "worker"),
			relationTargetUID(relations, workerRelationPointer),
			handler,
			form.Ref.Kind+" "+name,
		); hostErr != nil {
			return hostErr
		}
		// Two attachments carry a further rule that is about what they CLAIM
		// rather than about the worker they activate (decisions 0020 and 0026).
		switch form.Ref.Kind {
		case queueConsumerKind:
			// A queue has at most one consumer, and a consumer's dead-letter
			// destination must lead somewhere a message can come to rest.
			if hostErr := h.validateSingleQueueConsumer(scope, name, relations); hostErr != nil {
				return hostErr
			}
			return h.validateDeadLetterAcyclic(scope, name, relations)
		case workerCustomDomainKind:
			// One DNS hostname has one answer, per tenant, on the canonical
			// spelling this host stored.
			return h.validateSingleHostnameClaim(scope, name, spec)
		}
		if form.Ref.Kind == workerEndpointKind {
			return h.validateSingleWorkerEndpoint(scope, name, relations)
		}
	}
	return nil
}

// validateSingleWorkerEndpoint refuses a second Worker Endpoint against one
// worker INCARNATION (decision 0024).
//
// The rule is host-enforced rather than stated in the Form, because a desired
// schema cannot see it: nothing in one endpoint's document mentions any other,
// and the question "how many endpoints point at this worker" is a query over
// the store. Putting it here is the only placement that cannot be bypassed —
// the same reasoning, and the same placement, as the one-active-deployment rule
// and the one-consumer-per-queue rule.
//
// It is `invalid_argument` (400) for the same reason those two are: the request
// is well formed, and what is untrue is what it says about the worker it points
// at. `dependency_in_use` would describe the wrong edge — nothing here is being
// removed — and `unsupported_capability` would blame the host for a limit the
// contract states, not one this host happens to have.
func (h *ReferenceHost) validateSingleWorkerEndpoint(
	scope resourceScope,
	name string,
	relations []storedRelation,
) *hostError {
	workerUID := relationTargetUID(relations, workerRelationPointer)
	if workerUID == "" {
		return stableError("invalid_argument", "a WorkerEndpoint requires a target worker")
	}
	selfKey := resourceKey(scope, h.edgeGroup(), workerEndpointKind, name)
	for _, candidate := range h.scopedResources(scope) {
		if candidate.group() != h.edgeGroup() ||
			candidate.kind() != workerEndpointKind || candidate.key() == selfKey {
			continue
		}
		if relationTargetUID(candidate.Relations, workerRelationPointer) != workerUID {
			continue
		}
		return stableError(
			"invalid_argument",
			"ModuleWorker at uid "+workerUID+" already has the WorkerEndpoint "+candidate.Name+
				"; a worker has at most one host-assigned endpoint, and two addresses for one service "+
				"would leave neither of them canonical",
		)
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
	scope resourceScope,
	workerName, workerUID, handler, subject string,
) *hostError {
	if workerName == "" || workerUID == "" {
		return stableError("invalid_argument", subject+" requires a target worker")
	}
	deployment := h.activeDeployment(scope, workerUID)
	if deployment == nil {
		return stableError(
			"unsupported_capability",
			subject+" requires the active WorkerDeployment of ModuleWorker "+workerName+" at uid "+
				workerUID+", which has none; no version of that worker serves the "+handler+" handler",
		)
	}
	offender, serves := servesHandler(h.weightedVersions(scope, deployment.Relations), handler)
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

// validateWorkerClassHolder proves both rules a Durable Workflow and an Actor
// Namespace carry, in the order a host must answer them.
//
// One worker carries at most one holder per class name. Two workflows over one
// class would give one instance history two identities; two namespaces over one
// class would give one class two disjoint id spaces. It is invalid_argument
// rather than dependency_in_use for the same reason one-deployment-per-worker
// is: the request is well formed, and what is untrue is what it says about the
// worker it points at.
//
// Then the class must actually be exported — but only when a deployment is
// live. Before any deployment there is nothing to contradict, and the resource
// stores Provisioning.
func (h *ReferenceHost) validateWorkerClassHolder(
	form *InstalledForm, scope resourceScope, name string, spec map[string]any, relations []storedRelation,
) *hostError {
	workerUID := relationTargetUID(relations, workerRelationPointer)
	workerName := nestedName(spec, "worker")
	if workerName == "" || workerUID == "" {
		return stableError("invalid_argument", form.Ref.Kind+" "+name+" requires a target worker")
	}
	className, _ := spec["className"].(string)
	selfKey := resourceKey(scope, h.edgeGroup(), form.Ref.Kind, name)
	for _, candidate := range h.scopedResources(scope) {
		if candidate.group() != h.edgeGroup() || candidate.kind() != form.Ref.Kind ||
			candidate.key() == selfKey {
			continue
		}
		if relationTargetUID(candidate.Relations, workerRelationPointer) != workerUID {
			continue
		}
		if existing, _ := candidate.Spec["className"].(string); existing == className {
			return stableError("invalid_argument",
				form.Ref.Kind+" "+name+" claims class "+strconv.Quote(className)+" of ModuleWorker "+
					workerName+" at uid "+workerUID+", which "+form.Ref.Kind+" "+candidate.Name+
					" already holds; one worker carries at most one "+form.Ref.Kind+" per class name")
		}
	}
	deployment := h.activeDeployment(scope, workerUID)
	if deployment == nil {
		return nil
	}
	offender, exports := h.versionsExportClass(scope, h.weightedVersions(scope, deployment.Relations), className)
	if exports {
		return nil
	}
	detail := "it weights no resolvable WorkerVersion"
	if offender != "" {
		detail = "weighted WorkerVersion " + offender + " runs a module that does not export it"
	}
	return stableError("unsupported_capability",
		form.Ref.Kind+" "+name+" requires every version weighted by WorkerDeployment "+deployment.Name+
			" to export the class "+strconv.Quote(className)+"; "+detail)
}

// versionsExportClass reports whether EVERY weighted version's module exports
// one class, naming the first that does not.
//
// Every, not some: traffic is split across the weighted set, so an instance
// that replays into a version missing the class is an instance the host
// promised to finish and cannot. A version whose module exports this host does
// not know at all is treated as exporting it — the same conservative answer
// exportedHandlerViolation gives, and for the same reason: a reference host
// that runs no JavaScript must not refuse what it merely cannot see.
func (h *ReferenceHost) versionsExportClass(
	scope resourceScope, versions []*storedResource, className string,
) (offender string, exports bool) {
	if len(versions) == 0 {
		return "", false
	}
	for _, version := range versions {
		classes, known := h.bundleModuleClasses(scope, version.Relations)
		if !known {
			continue
		}
		if !slices.Contains(classes, className) {
			return version.Name, false
		}
	}
	return "", true
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
func (h *ReferenceHost) validateInboundServiceBindings(
	scope resourceScope, relations []storedRelation,
) *hostError {
	for _, relation := range relations {
		if relation.Relation != serviceBindingsRelation {
			continue
		}
		if hostErr := h.requireServingDeployment(
			scope, relation.TargetName, relation.TargetUID, fetchHandler,
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
	scope resourceScope,
	name string,
	relations []storedRelation,
) *hostError {
	workerUID := relationTargetUID(relations, workerRelationPointer)
	if workerUID == "" {
		return stableError("invalid_argument", "a WorkerDeployment requires a target worker")
	}
	selfKey := resourceKey(scope, h.edgeGroup(), workerDeploymentKind, name)
	// A. One active deployment per worker. Two deployments of one worker leave
	//    "which one serves" undefined, and no rule chosen after the fact — newest,
	//    lowest name, highest weight — is a rule an author can predict.
	if existing := h.activeDeployment(scope, workerUID); existing != nil {
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
		version := h.resources[resourceKey(scope, relation.TargetAPIVersion, relation.TargetKind, relation.TargetName)]
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
	versions := h.weightedVersions(scope, relations)
	for _, dependent := range h.workerDependents(scope, workerUID) {
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

// referenceEndpointHostname is the address THIS host assigns to one Worker
// Endpoint incarnation.
//
// It is derived from the space and the resource UID and from nothing the author
// wrote, which is the whole point: the Form's desired state carries no hostname,
// so an address that could be reconstructed from the configuration would mean
// the host had not assigned anything. Deriving it from the UID also makes it
// stable for the life of one incarnation — re-weighting the deployment, reading,
// observing, and re-applying all return the same address — while a delete and
// re-create is a new incarnation and legitimately gets a new one.
//
// The shape below is this host's private detail and no portable rule. A
// conforming client may rely on the value, on the https scheme, and on the `/`
// path root; anything it inferred from the label or the apex would be a fact
// about this host rather than about the Form (decision 0024).
func referenceEndpointHostname(space, uid string) string {
	sum := sha256.Sum256([]byte(space + "\x00" + uid))
	return "e" + hex.EncodeToString(sum[:8]) + ".endpoints.portable-conformance.invalid"
}

// workerEndpointOutputs is the `status.outputs` document of one Worker
// Endpoint: the assigned hostname and the one absolute HTTPS address a client
// uses, which is exactly that hostname at the path root. There is no plaintext
// form and no port; a host that could not assign an address at all would have
// to refuse the endpoint with `unsupported_capability` (422) rather than answer
// here with something it did not assign.
func workerEndpointOutputs(resource *storedResource) map[string]any {
	hostname := referenceEndpointHostname(resource.Space, resource.UID)
	return map[string]any{"hostname": hostname, "url": "https://" + hostname + "/"}
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
	if resource.group() != h.edgeGroup() || resource.kind() != workerDeploymentKind {
		return nil
	}
	workerUID := relationTargetUID(resource.Relations, workerRelationPointer)
	dependents := h.workerDependents(resource.scope(), workerUID)
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
	if resource.group() != h.edgeGroup() || resource.kind() != moduleWorkerKind {
		return "", "", false
	}
	deployment := h.activeDeployment(resource.scope(), resource.UID)
	if deployment == nil {
		return "Provisioning",
			"ModuleWorker " + resource.Name + " at uid " + resource.UID +
				" has no active WorkerDeployment, so it serves no traffic and provides no worker.service interface yet",
			true
	}
	offender, serves := servesHandler(h.weightedVersions(resource.scope(), deployment.Relations), fetchHandler)
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
// is what says which handlers exist at all: fetch, scheduled, and queue,
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
	if form.Ref.APIVersion != h.edgeGroup() || form.Ref.Kind != workerVersionKind {
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
// claiming to prove it for every possible one; see spec/host-api/v1beta1.md.
func (h *ReferenceHost) exportedHandlerViolation(
	form *InstalledForm, scope resourceScope, spec map[string]any, relations []storedRelation,
) *hostError {
	if form.Ref.APIVersion != h.edgeGroup() || form.Ref.Kind != workerVersionKind {
		return nil
	}
	exported, known := h.bundleModuleExports(scope, relations)
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
func (h *ReferenceHost) bundleModuleExports(
	scope resourceScope, relations []storedRelation,
) ([]string, bool) {
	bundleUID := relationTargetUID(relations, bundleRelationPointer)
	if bundleUID == "" {
		return nil, false
	}
	var bundle *storedResource
	for _, relation := range relations {
		if relation.Pointer != bundleRelationPointer {
			continue
		}
		bundle = h.resources[resourceKey(scope, relation.TargetAPIVersion, relation.TargetKind, relation.TargetName)]
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

// bundleMainModuleDigest is the content address of the main module of the
// bundle one Worker Version references, resolved the way bundleModuleExports
// resolves it: through the relation the apply already pinned, the bundle's
// committed manifest digest, and that manifest's own main-module entry. A
// renamed or replaced bundle is a different answer rather than the same one.
func (h *ReferenceHost) bundleMainModuleDigest(
	scope resourceScope, relations []storedRelation,
) (string, bool) {
	bundleUID := relationTargetUID(relations, bundleRelationPointer)
	if bundleUID == "" {
		return "", false
	}
	var bundle *storedResource
	for _, relation := range relations {
		if relation.Pointer != bundleRelationPointer {
			continue
		}
		bundle = h.resources[resourceKey(scope, relation.TargetAPIVersion, relation.TargetKind, relation.TargetName)]
	}
	if bundle == nil || bundle.UID != bundleUID {
		return "", false
	}
	manifestDigest, _ := bundle.Spec["manifestDigest"].(string)
	raw := h.manifests[manifestDigest]
	if raw == nil {
		return "", false
	}
	var manifest artifactManifest
	if err := formpackage.DecodeStrictIJSON(raw, &manifest); err != nil {
		return "", false
	}
	for _, module := range manifest.Modules {
		if module.Name == manifest.MainModule {
			return module.Digest, true
		}
	}
	return "", false
}

// bundleModuleClasses resolves which CLASSES the main module of the bundle one
// Worker Version references exports, by the same content-addressed path
// bundleModuleExports uses for handlers: the fact is a property of the bytes,
// so the same module uploaded again under any manifest exports the same
// classes.
func (h *ReferenceHost) bundleModuleClasses(
	scope resourceScope, relations []storedRelation,
) ([]string, bool) {
	digest, ok := h.bundleMainModuleDigest(scope, relations)
	if !ok {
		return nil, false
	}
	classes, known := h.moduleClasses[digest]
	return classes, known
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
func validateEnvironmentNamespace(familyGroup string, form *InstalledForm, spec map[string]any) *hostError {
	if form.Ref.APIVersion != familyGroup || form.Ref.Kind != workerVersionKind {
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

// classHolderUnavailable reports why a Durable Workflow or an Actor Namespace
// is not Ready.
//
// It is the readiness half of the rule validateWorkerClassHolder refuses on,
// and the two must agree: what the create gate refuses outright is what this
// reports as a stable Ready=False on a resource that already exists. The
// difference is only WHEN each applies — before any deployment there is
// nothing to contradict, so the resource stores and reports Provisioning
// rather than being refused, and a deployment landing later moves it to Ready
// with no apply of its own (decision 0016: rendered from other resources, so
// it moves revision, never generation).
func (h *ReferenceHost) classHolderUnavailable(resource *storedResource) (reason, hostReason string, unavailable bool) {
	if resource.group() != h.edgeGroup() || !classHolderKinds[resource.kind()] {
		return "", "", false
	}
	className, _ := resource.Spec["className"].(string)
	workerUID := relationTargetUID(resource.Relations, workerRelationPointer)
	deployment := h.activeDeployment(resource.scope(), workerUID)
	if deployment == nil {
		return "Provisioning",
			resource.kind() + " " + resource.Name + " names class " + strconv.Quote(className) +
				" of a ModuleWorker at uid " + workerUID +
				" with no active WorkerDeployment, so nothing serves that class yet",
			true
	}
	offender, exports := h.versionsExportClass(
		resource.scope(), h.weightedVersions(resource.scope(), deployment.Relations), className,
	)
	if exports {
		return "", "", false
	}
	detail := "it weights no resolvable WorkerVersion"
	if offender != "" {
		detail = "weighted WorkerVersion " + offender + " runs a module that does not export it"
	}
	return "UnsupportedCapability",
		"WorkerDeployment " + deployment.Name + " does not serve class " + strconv.Quote(className) +
			" for " + resource.kind() + " " + resource.Name + "; " + detail,
		true
}
