// Package edgeformcatalog declares, as data, the Edge Platform Family:
// the first official Form Family (edge.forms.takoform.com/v1alpha1, decision
// 0009) together with its exact Interface and Binding contracts (decision
// 0010). Every member fixes the application-visible semantics of one proven
// edge service primitive completely (decision 0008); no field selects a
// vendor, an account, a region, or an implementation.
package edgeformcatalog

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// Family is the Edge Platform Family group.
var Family = model.Family{Group: "edge.forms.takoform.com", Version: "v1alpha1"}

// edgeDefinitionVersion is the definition SemVer every MVP member starts at.
const edgeDefinitionVersion = "0.1.0"

// ref renders one exact in-family cross-resource reference: the target group,
// the target kind, and the target name. All three travel on the wire; the HCL
// surface still asks the author for the bare name.
func ref(kind, name string) map[string]any {
	return map[string]any{"apiVersion": Family.APIVersion(), "kind": kind, "name": name}
}

// bindingInstance renders one typed binding instance: the JavaScript
// identifier the binding is projected under, plus the exact target reference.
func bindingInstance(bindingName, kind, targetName string) map[string]any {
	return map[string]any{"name": bindingName, "resource": ref(kind, targetName)}
}

// runtimeHandlerVocabulary is the WorkerVersion `handlers` enum, read out of
// the worker.runtime contract instead of being spelled a second time. On a
// broken contract it yields nil, which makes the string-set field unbounded and
// fails Form.Validate — the catalog refuses to render rather than emitting a
// Form whose handler set is not the ABI's.
func runtimeHandlerVocabulary() []string {
	handlers, err := RuntimeHandlers()
	if err != nil {
		return nil
	}
	return handlers
}

// requiresInterface states that a reference depends only on an exact Interface
// its target provides: what the source needs is the behavior that contract
// fixes, and any Form providing it would serve.
func requiresInterface(name, version string) model.TargetContract {
	return model.TargetContract{Interface: &model.InterfaceRefSource{Name: name, Version: version}}
}

// requiresExactForm states that a reference depends on the target's exact
// desired contract — because the source, or the host acting for it, reads a
// member of the target's desired spec or enforces a rule stated over the target
// Form itself. Nothing weaker states that requirement (decision 0022).
var requiresExactForm = model.TargetContract{ExactForm: true}

// moduleWorkerRef renders the inward-activation reference every attachment
// carries. What an attachment needs from the worker is the ES Module Worker
// ABI — the handler its events invoke exists only because worker.runtime
// defines it — so the reference requires that exact Interface and nothing more.
func moduleWorkerRef(hcl, wire, doc string, required, immutable bool) model.Field {
	return model.Field{
		HCL: hcl, Wire: wire, Kind: model.KindResourceRef, TargetKind: "ModuleWorker",
		Target:   requiresInterface(WorkerRuntimeInterfaceName, "1.0.0"),
		Required: required, Immutable: immutable, Doc: doc,
		Example: ref("ModuleWorker", "module-worker"),
	}
}

// Forms is the complete Edge Platform Family MVP set, in a stable order.
var Forms = []model.Form{
	{
		Family: Family,
		Kind:   "ModuleWorker", Slug: "module-worker", ResourceType: "takoform_module_worker",
		Role: model.RoleIdentity, DefinitionVersion: edgeDefinitionVersion,
		Title: "Module Worker",
		Description: "Long-lived logical identity of one ES Module Worker application. The Form fixes the ES " +
			"Module Worker ABI by identity, and states it exactly: the runtime contract " +
			"worker.runtime@1.0.0 in this Form's providedInterfaces fixes the module's default-export shape, " +
			"the fetch, scheduled, queue, and tail handler signatures, the binding environment, " +
			"ctx.waitUntil, exception handling, body streaming, the minimum Web API surface, and module " +
			"loading. A host supporting this Form implements that exact digest; a runtime that behaves " +
			"differently is a different contract version and a different Form version, never a compatibility " +
			"date. Code, configuration, and bindings live on Worker Version revisions; traffic selection " +
			"lives on Worker Deployments.",
		// Two exact Interfaces, in two directions.
		//
		// worker.runtime is what the HOST provides to the code: it is the ABI
		// this identity claims to fix, so a host that supports ModuleWorker
		// implements it, and a Worker Version is the code that fills it.
		//
		// worker.service is what the identity provides to OTHER workers, and it
		// is provided by the IDENTITY rather than a revision: a
		// module-worker.service binding addresses another worker by logical
		// identity and is served by whichever versions that worker's active
		// deployment selects. Declaring it on a Worker Version would name a
		// target no binding may point at, and would leave the binding's
		// allowedTargetForms Form providing no Interface at all.
		ProvidedInterfaces: []model.InterfaceRefSource{
			{Name: WorkerRuntimeInterfaceName, Version: "1.0.0"},
			{Name: "worker.service", Version: "1.0.0"},
		},
	},
	{
		Family: Family,
		Kind:   "WorkerBundle", Slug: "worker-bundle", ResourceType: "takoform_worker_bundle",
		Role: model.RoleRevision, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Bundle",
		Description: "Immutable content-addressed module bundle of one worker build, named by the digest of the " +
			"artifact manifest committed through the content-addressed upload API (decision 0012). The manifest, " +
			"not this Form, describes the main module and every additional module with its closed media type, " +
			"exact size, and sha256 digest, so the bundle keeps exactly one source of truth for its bytes. " +
			"Different bytes commit a different manifest, which is a different bundle.",
		Fields: []model.Field{
			{HCL: "manifest_digest", Wire: "manifestDigest", Kind: model.KindString,
				Required: true, Immutable: true,
				Pattern: model.PatternCanonicalSHA256,
				Doc: "Immutable identity of the committed artifact manifest that describes this bundle: the RFC 8785 " +
					"canonical sha256 digest the host returned when the manifest was committed. A host resolves that " +
					"manifest and holds it to the artifact contract before it mutates anything, and rejects a digest it " +
					"has not committed.",
				Example:        "sha256:6a5cbf24f5d0c86479ae13b9d1731a626a1729f01aef65403c5c8ac82ed85f43",
				AltExample:     "sha256:8624fd492ece196a5048414afd598275b811beafc00aab602bfb59978f880765",
				CounterExample: "sha256:not-a-canonical-manifest-digest"},
		},
	},
	{
		Family: Family,
		Kind:   "WorkerVersion", Slug: "worker-version", ResourceType: "takoform_worker_version",
		Role: model.RoleRevision, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Version",
		Description: "Immutable executable snapshot of one Module Worker: a bundle, the handlers its module " +
			"exports, non-secret vars, and the typed capability bindings the code may use. A change is a new " +
			"Worker Version; traffic moves only through Worker Deployments. The runtime this code runs on is " +
			"not a field of this Form: it is fixed by the worker.runtime@1.0.0 contract the Module Worker " +
			"identity provides, so a version carries no compatibility date and no compatibility flag " +
			"(decision 0019).",
		AcceptedBindings: []model.BindingRefSource{
			{Name: "module-worker.edge-kv", Version: "1.0.0"},
			{Name: "module-worker.object-bucket", Version: "1.0.0"},
			{Name: "module-worker.sqlite", Version: "1.0.0"},
			{Name: "module-worker.queue-producer", Version: "1.0.0"},
			{Name: "module-worker.service", Version: "1.0.0"},
		},
		Fields: []model.Field{
			// The worker reference states an ABI requirement, not a Form one: what
			// this version needs from the identity it belongs to is the runtime
			// contract that decides which handlers exist at all and what signature
			// each has. Any worker identity providing worker.runtime@1.0.0 serves
			// this version, whatever else its Definition declares.
			{HCL: "worker", Wire: "worker", Kind: model.KindResourceRef, TargetKind: "ModuleWorker", Required: true,
				Target:  requiresInterface(WorkerRuntimeInterfaceName, "1.0.0"),
				Doc:     "Module Worker identity this version belongs to.",
				Example: ref("ModuleWorker", "module-worker")},
			// The bundle reference is the opposite case. A Worker Bundle provides
			// no Interface at all, and a host READS the bundle's desired state —
			// its manifestDigest — to resolve the committed manifest, its main
			// module, and the handlers that module exports. That is a dependency on
			// the Worker Bundle Form's exact desired contract: a Definition that
			// renamed or retyped manifestDigest would leave every version
			// referencing it unable to say what it runs.
			{HCL: "bundle", Wire: "bundle", Kind: model.KindResourceRef, TargetKind: "WorkerBundle", Required: true,
				Target:  requiresExactForm,
				Doc:     "Worker Bundle carrying the exact module bytes this version executes.",
				Example: ref("WorkerBundle", "worker-bundle")},
			// The handler enum is not written here twice: it is the closed
			// vocabulary the worker.runtime contract publishes, read back out of
			// that contract so the Form and the ABI can never disagree.
			{HCL: "handlers", Wire: "handlers", Kind: model.KindStringSet, Required: true, MinItems: 1,
				Enum: runtimeHandlerVocabulary(),
				Doc: "Module event handlers this version exports, from the closed vocabulary the " +
					"worker.runtime@1.0.0 contract defines. A host rejects a handler that contract does not " +
					"define, and rejects an attachment whose event kind is not declared here.",
				Example: []any{"fetch"}},
			{HCL: "vars", Wire: "vars", Kind: model.KindJSONMap,
				Default:                  map[string]any{},
				ProjectsEnvironmentNames: true,
				Doc: "Non-secret configuration values projected into the module environment. Sensitive material never enters portable state. " +
					"Omitting it projects no variable.",
				Example: map[string]any{"LOG_LEVEL": "info"}},
			// Every binding reference states the Interface its Binding contract
			// projects, and nothing more. A binding IS the projection of an
			// Interface into the module environment, so the exact contract the
			// target provides is the whole requirement; pinning the target's Form
			// as well would refuse a different store that implements the same
			// contract, for a reason the binding does not have. The named contract
			// is the Binding Definition's own targetInterface, so the two cannot
			// disagree — the catalog resolves one from the other and the host
			// verifies they agree before it stores anything.
			{HCL: "kv_bindings", Wire: "kvBindings", Kind: model.KindBindingList,
				TargetKind: "EdgeKVNamespace", BindingType: "module-worker.edge-kv",
				Target:  requiresInterface("edge.kv", "1.0.0"),
				Default: []any{},
				Doc:     "Typed module-worker.edge-kv bindings projecting the edge.kv API under JavaScript identifier names. Omitting it declares no such binding.",
				Example: []any{bindingInstance("CACHE", "EdgeKVNamespace", "edge-kv-namespace")}},
			{HCL: "bucket_bindings", Wire: "bucketBindings", Kind: model.KindBindingList,
				TargetKind: "ObjectBucket", BindingType: "module-worker.object-bucket",
				Target:  requiresInterface("edge.objects", "1.0.0"),
				Default: []any{},
				Doc:     "Typed module-worker.object-bucket bindings projecting the edge.objects API. Omitting it declares no such binding.",
				Example: []any{bindingInstance("MEDIA", "ObjectBucket", "object-bucket")}},
			{HCL: "sqlite_bindings", Wire: "sqliteBindings", Kind: model.KindBindingList,
				TargetKind: "SQLiteDatabase", BindingType: "module-worker.sqlite",
				Target:  requiresInterface("edge.sql", "1.0.0"),
				Default: []any{},
				Doc:     "Typed module-worker.sqlite bindings projecting the edge.sql API. Omitting it declares no such binding.",
				Example: []any{bindingInstance("DB", "SQLiteDatabase", "sqlite-database")}},
			{HCL: "queue_producer_bindings", Wire: "queueProducerBindings", Kind: model.KindBindingList,
				TargetKind: "AtLeastOnceQueue", BindingType: "module-worker.queue-producer",
				Target:  requiresInterface("edge.queue", "1.0.0"),
				Default: []any{},
				Doc:     "Typed module-worker.queue-producer bindings projecting only send and sendBatch. Omitting it declares no such binding.",
				Example: []any{bindingInstance("EVENTS", "AtLeastOnceQueue", "at-least-once-queue")}},
			{HCL: "service_bindings", Wire: "serviceBindings", Kind: model.KindBindingList,
				TargetKind: "ModuleWorker", BindingType: "module-worker.service",
				Target:  requiresInterface("worker.service", "1.0.0"),
				Default: []any{},
				Doc:     "Typed module-worker.service bindings projecting worker.service fetch toward another Module Worker. Omitting it declares no such binding.",
				Example: []any{bindingInstance("AUTH", "ModuleWorker", "auth-worker")}},
			{HCL: "required_sensitive_vars", Wire: "requiredSensitiveVars", Kind: model.KindStringSet,
				ItemPattern:              model.PatternSensitiveVarName,
				Default:                  []any{},
				ProjectsEnvironmentNames: true,
				Doc: "Names of sensitive values this version requires the host to supply out-of-band. " +
					"Only the names are portable state; values travel through each host's own sealed path. " +
					"Omitting it requires no sensitive value.",
				Example: []any{"API_SIGNING_TOKEN_NAME"}, CounterExample: []any{"lowercase name"}},
		},
	},
	{
		Family: Family,
		Kind:   "WorkerDeployment", Slug: "worker-deployment", ResourceType: "takoform_worker_deployment",
		Role: model.RoleDeployment, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Deployment",
		Description: "Selects which Worker Versions of one Module Worker serve traffic and in what " +
			"proportion. Weights are basis points and must sum to exactly 10000 across entries; the sum " +
			"is host-validated semantics because a schema cannot add weights. Rollback is re-weighting, " +
			"never mutating a revision.",
		Fields: []model.Field{
			// The one reference in the family that WRITES to its target. A
			// deployment is the sole resource that makes a Module Worker Ready, and
			// the aggregate rules of decision 0016 — one active deployment per
			// worker incarnation, the attachment gate, readiness rendered onto the
			// worker's own representation — are stated over the Module Worker Form
			// itself, never over an Interface it provides to application code. So
			// this reference depends on the target's exact Form, unlike every other
			// worker reference in the family, which only invokes the ABI.
			{HCL: "worker", Wire: "worker", Kind: model.KindResourceRef, TargetKind: "ModuleWorker",
				Target:   requiresExactForm,
				Required: true, Immutable: true,
				Doc:     "Module Worker identity whose traffic this deployment governs.",
				Example: ref("ModuleWorker", "module-worker")},
			{HCL: "versions", Wire: "versions", Kind: model.KindObjectList, Required: true, MinItems: 1, MaxItems: 8,
				Doc: "Active Worker Versions and their traffic weights in basis points. Weights must sum to exactly 10000.",
				Fields: []model.Field{
					// A host READS the weighted version's desired state: its /worker
					// relation decides ownership, and its handlers decide what the
					// deployment serves and therefore which attachments are admissible.
					// That is a dependency on the Worker Version Form's exact contract.
					{HCL: "worker_version", Wire: "workerVersion", Kind: model.KindResourceRef, TargetKind: "WorkerVersion", Required: true,
						Target: requiresExactForm,
						Doc:    "Worker Version receiving this weight."},
					{HCL: "weight", Wire: "weight", Kind: model.KindInteger, Required: true,
						Min: model.I64(1), Max: model.I64(10000),
						Doc: "Traffic share in basis points (1..10000)."},
				},
				Example: []any{map[string]any{
					"workerVersion": ref("WorkerVersion", "worker-version"),
					"weight":        10000,
				}},
				CounterExample: []any{map[string]any{
					"workerVersion": ref("WorkerVersion", "worker-version"),
					"weight":        0,
				}}},
		},
	},
	{
		Family: Family,
		Kind:   "WorkerCustomDomain", Slug: "worker-custom-domain", ResourceType: "takoform_worker_custom_domain",
		Role: model.RoleAttachment, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Custom Domain",
		Description: "Attaches one DNS hostname to a Module Worker so its active deployment serves that " +
			"hostname over HTTPS. Inward activation is an attachment, never a binding; deleting the " +
			"attachment detaches the hostname and never deletes the worker. A hostname is a name in DNS " +
			"rather than a label in this host's namespace, so it is CANONICALIZED before it is compared " +
			"and before it is stored — trailing root dot removed, ASCII letters lowercased — and one " +
			"canonical hostname is served by AT MOST ONE attachment per tenant, across every space. " +
			"A second attachment claiming a hostname a live one already serves is refused before any " +
			"mutation; releasing the holder makes the claim representable (decision 0026).",
		Fields: []model.Field{
			moduleWorkerRef("worker", "worker", "Module Worker served on this hostname.", true, true),
			{HCL: "hostname", Wire: "hostname", Kind: model.KindString, Required: true, Immutable: true,
				Pattern: model.PatternHostname, MaxLength: 253,
				Doc: "Dotted DNS hostname this attachment serves. Changing it replaces the attachment. The pattern " +
					"admits the spellings DNS treats as one name — an uppercase letter, a trailing root dot — because " +
					"a host canonicalizes rather than refusing them; it admits no non-ASCII byte, so an " +
					"internationalized name travels as its A-label.",
				Example: "app.portable-conformance.invalid", AltExample: "alt.portable-conformance.invalid",
				CounterExample: "not a hostname"},
		},
	},
	{
		Family: Family,
		Kind:   "WorkerEndpoint", Slug: "worker-endpoint", ResourceType: "takoform_worker_endpoint",
		Role: model.RoleAttachment, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Endpoint",
		Description: "Makes one Module Worker reachable over HTTPS at an address the HOST assigns, with no " +
			"customer-owned domain and no DNS of the author's. The desired state is the worker and nothing " +
			"else: the author asks for reachability, and where that reachability lives is the host's decision, " +
			"exactly as an account, a region, and a vendor subdomain are. Requests arriving at the address " +
			"invoke the fetch handler of the worker's ACTIVE DEPLOYMENT, so promotion and rollback move what " +
			"answers without the endpoint being re-applied and without its address changing. The scheme is " +
			"https and the path root is `/`; TLS is not an option a host may decline, because an address that " +
			"is only reachable in plaintext is a different promise from the one this Form makes. A worker has " +
			"AT MOST ONE endpoint: two would be two addresses for one service with nothing saying which is " +
			"canonical, and the second is refused. The assigned address is published as outputs — a portable " +
			"author may rely on a value being returned, on its scheme being https, and on it routing to the " +
			"active deployment, and on nothing about its SHAPE: which subdomain, which apex, and how long the " +
			"label is are host detail no portable configuration may parse or reconstruct.",
		Fields: []model.Field{
			moduleWorkerRef("worker", "worker",
				"Module Worker whose active deployment answers requests at the assigned address. Changing it replaces the endpoint.",
				true, true),
		},
		// The two observable facts, and only those. `hostname` is what the host
		// assigned; `url` is the one address a client uses. Both are published
		// because either alone forces a consumer to reconstruct the other, and
		// reconstruction is precisely the parsing of host detail this Form's
		// portability boundary forbids.
		Outputs: []model.Field{
			{HCL: "hostname", Wire: "hostname", Kind: model.KindString,
				Pattern: model.PatternCanonicalHostname, MaxLength: 253,
				Doc: "Dotted DNS hostname the host assigned to this endpoint, in canonical form: lowercase where " +
					"DNS is case-insensitive and no trailing root dot. An author's hostname admits those spellings " +
					"because a host canonicalizes what it is given; an assigned name has no earlier spelling to " +
					"preserve. Its VALUE is portable to read and pass on; its SHAPE is host detail, so a portable " +
					"configuration never parses it, never asserts a suffix, and never reconstructs it from the " +
					"resource name."},
			{HCL: "url", Wire: "url", Kind: model.KindString,
				Pattern:   `^https://[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+/$`,
				MaxLength: 264,
				Doc: "Absolute HTTPS URL of the endpoint's path root: exactly `https://` + the assigned hostname + " +
					"`/`. The scheme is fixed by the Form and the path root is `/`; there is no plaintext address and " +
					"no port, so a consumer composes deeper paths onto this value rather than deriving an origin."},
		},
	},
	{
		Family: Family,
		Kind:   "WorkerCronTrigger", Slug: "worker-cron-trigger", ResourceType: "takoform_worker_cron_trigger",
		Role: model.RoleAttachment, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Cron Trigger",
		Description: "Attaches one cron schedule to a Module Worker, invoking its scheduled handler at each " +
			"match. Schedules are interpreted in UTC only; there is no timezone field, so two hosts can never " +
			"fire the same trigger at different instants and no schedule ever skips or repeats an hour for a " +
			"daylight-saving transition. The grammar is five fields separated by single spaces — minute 0-59, " +
			"hour 0-23, day-of-month 1-31, month 1-12, day-of-week 0-6 with 0 Sunday — and each field is a " +
			"comma-separated list of `*`, a literal, a range `low-high`, `*/step`, or `low-high/step`. Names " +
			"and a step on a bare literal are not accepted, and neither is any value outside its field's own " +
			"range, an inverted range, or a step outside 1..span. When day-of-month and day-of-week are BOTH " +
			"restricted the trigger fires on a day either of them selects; when only one is restricted only " +
			"that one constrains the day. A missed run is not made up: a host that could not fire a match — " +
			"because it was unavailable, or because the previous invocation was still running — skips it " +
			"rather than firing late, so a schedule never produces a backlog. At-least-once delivery applies " +
			"to each match: a handler may be invoked more than once for one matched minute, and it must be " +
			"idempotent. An uncaught exception in the handler is a failed invocation reported to host " +
			"diagnostics; it is not retried within the matched minute and it never becomes an HTTP response.",
		Fields: []model.Field{
			moduleWorkerRef("worker", "worker", "Module Worker whose scheduled handler this trigger invokes.", true, true),
			{HCL: "cron", Wire: "cron", Kind: model.KindString, Required: true,
				Pattern: model.PatternCron, MaxLength: 64,
				Doc: "Portable five-field cron expression, interpreted in UTC only. The pattern bounds the shape; a host " +
					"also parses it and refuses a value outside its field's range, an inverted range, or an out-of-span step.",
				Example: "*/5 * * * *", AltExample: "0 3 * * *", CounterExample: "0 3 * *"},
		},
	},
	{
		Family: Family,
		Kind:   "EdgeKVNamespace", Slug: "edge-kv-namespace", ResourceType: "takoform_edge_kv_namespace",
		Role: model.RoleIdentity, DefinitionVersion: edgeDefinitionVersion,
		Title: "Edge KV Namespace",
		Description: "Globally replicated key/value namespace of opaque BYTES with eventual consistency, exactly " +
			"as fixed by the edge.kv Interface. Eventual consistency is the Form's semantics, not an option: a " +
			"store with different convergence behavior is a different Form, and this one promises no " +
			"read-your-writes, in any session, at any location. Values are byte strings carried in the family's " +
			"encoded-bytes shape, so the declared byte limit and the structural string ceiling measure the same " +
			"thing (decision 0020).",
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: "edge.kv", Version: "1.0.0"}},
	},
	{
		Family: Family,
		// The resource type is takoform_edge_object_bucket, not
		// takoform_object_bucket: the retained v2 lane still owns that name
		// while both lanes are co-registered in one provider binary. This is a
		// transitional name; a future provider release that drops the retained
		// v2 lane (a major, per versioning.md) reclaims takoform_object_bucket
		// for this Form.
		Kind: "ObjectBucket", Slug: "object-bucket", ResourceType: "takoform_edge_object_bucket",
		Role: model.RoleIdentity, DefinitionVersion: edgeDefinitionVersion,
		Title: "Object Bucket",
		Description: "Flat-namespace object store with strong read-after-write consistency, streaming bodies, " +
			"ranged and conditional reads, and multipart upload, exactly as fixed by the edge.objects Interface. " +
			"An object body is a byte stream, never a JSON string: the contract's 5 GiB ceiling is only " +
			"meaningful because bodies never travel inside an operation document (decision 0020). Operating " +
			"rules such as CORS, lifecycle, and lock are separate policy resources, never desired fields of the " +
			"bucket identity.",
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: "edge.objects", Version: "1.0.0"}},
	},
	{
		Family: Family,
		Kind:   "SQLiteDatabase", Slug: "sqlite-database", ResourceType: "takoform_sqlite_database",
		Role: model.RoleIdentity, DefinitionVersion: edgeDefinitionVersion,
		Title: "SQLite Database",
		Description: "Embedded SQLite database with serializable transactions and TAGGED values, exactly as " +
			"fixed by the edge.sql Interface. SQLite semantics are the identity: a database with different SQL, " +
			"typing, or isolation behavior is a different Form, never an engine token. Values carry their " +
			"storage class, so a 64-bit INTEGER and a BLOB round-trip losslessly instead of being flattened " +
			"into a JSON scalar (decision 0020).",
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: "edge.sql", Version: "1.0.0"}},
	},
	{
		Family: Family,
		Kind:   "AtLeastOnceQueue", Slug: "at-least-once-queue", ResourceType: "takoform_at_least_once_queue",
		Role: model.RoleIdentity, DefinitionVersion: edgeDefinitionVersion,
		Title: "At-Least-Once Queue",
		Description: "Message queue with at-least-once delivery and no ordering guarantee, exactly as fixed " +
			"by the edge.queue Interface. There is no ordering field: a FIFO queue is a different Form. Message " +
			"bodies are opaque bytes, a message identity is stable across redeliveries, and a queue has AT MOST " +
			"ONE consumer — two would split the stream between two retry policies and two dead-letter " +
			"destinations, which leaves the queue's own behavior unstatable (decision 0020).",
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: "edge.queue", Version: "1.0.0"}},
		Fields: []model.Field{
			// Retention has no portable default: how long a host keeps an
			// undelivered message is the whole operational meaning of the queue,
			// and no value is right for every workload. It is therefore required
			// rather than silently chosen.
			{HCL: "message_retention_seconds", Wire: "messageRetentionSeconds", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(60), Max: model.I64(1209600),
				Doc:     "How long an undelivered message is retained before it is dropped, in seconds.",
				Example: 345600},
			{HCL: "delivery_delay_seconds", Wire: "deliveryDelaySeconds", Kind: model.KindInteger,
				Min: model.I64(0), Max: model.I64(43200), Default: 0,
				Doc:     "Default delay before a sent message becomes deliverable, in seconds. Omitting it delivers immediately.",
				Example: 0},
		},
	},
	{
		Family: Family,
		Kind:   "QueueConsumer", Slug: "queue-consumer", ResourceType: "takoform_queue_consumer",
		Role: model.RoleAttachment, DefinitionVersion: edgeDefinitionVersion,
		Title: "Queue Consumer",
		Description: "Attaches one Module Worker as the batch consumer of one At-Least-Once Queue, invoking its " +
			"queue handler with message batches and redelivering messages that were not acknowledged. " +
			"Consumption is inward activation and therefore an attachment, never a binding. One queue has at " +
			"most one consumer, so a second attachment against the same queue is refused. A handler that " +
			"returns normally without settling anything acknowledges the whole batch; one that throws retries " +
			"every message it had not already acknowledged. maxRetries counts REDELIVERIES only — the first " +
			"delivery does not count toward it — so a message is delivered at most 1 + maxRetries times, and a " +
			"message that exhausts them moves to dead_letter_queue when one is declared and is dropped " +
			"otherwise. The dead-letter copy is a new message there: new identity, new acceptance timestamp, " +
			"and an attempt count starting again at 1 (decision 0020). Because that transfer resets the " +
			"attempt count, dead_letter_queue MUST NOT lead back: a destination resolving to the queue this " +
			"consumer drains, or closing a cycle of any length through the dead-letter graph, is refused " +
			"before any mutation, because an exhausted message would circulate forever instead of coming to " +
			"rest (decision 0026).",
		Fields: []model.Field{
			// Both queue references state the edge.queue contract and nothing more.
			// What a consumer needs from the queue it drains is the delivery model
			// — at-least-once, a stable message identity across redeliveries, no
			// ordering — and what a dead-letter destination needs is the same
			// contract's acceptance side. Neither reads a member of the queue's
			// desired spec, so pinning the exact Form would refuse any other queue
			// Form implementing the same contract for no requirement either
			// reference actually has.
			{HCL: "queue", Wire: "queue", Kind: model.KindResourceRef, TargetKind: "AtLeastOnceQueue",
				Target:   requiresInterface("edge.queue", "1.0.0"),
				Required: true, Immutable: true,
				Doc:     "Queue this consumer drains. Changing it replaces the attachment.",
				Example: ref("AtLeastOnceQueue", "at-least-once-queue")},
			moduleWorkerRef("worker", "worker", "Module Worker whose queue handler receives the batches. Changing it replaces the attachment.", true, true),
			// Batching, retry, and concurrency decide throughput, duplicate
			// exposure, and downstream load together. No single value is portable
			// across workloads, so the consumer states all five rather than
			// inheriting whatever a host would otherwise pick.
			{HCL: "max_batch_size", Wire: "maxBatchSize", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(1), Max: model.I64(100),
				Doc:     "Largest number of messages delivered in one batch.",
				Example: 10},
			{HCL: "max_batch_timeout_seconds", Wire: "maxBatchTimeoutSeconds", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(0), Max: model.I64(60),
				Doc:     "Longest time the host waits to fill a batch before delivering it, in seconds.",
				Example: 5},
			{HCL: "max_retries", Wire: "maxRetries", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(0), Max: model.I64(100),
				Doc:     "How many times a failed batch is redelivered before its messages go to the dead-letter queue or are dropped.",
				Example: 3},
			{HCL: "retry_delay_seconds", Wire: "retryDelaySeconds", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(0), Max: model.I64(43200),
				Doc:     "Delay before a failed batch becomes deliverable again, in seconds.",
				Example: 60},
			// The one field whose ABSENCE is the semantics: there is no queue that
			// means "drop exhausted messages", so no default can express it.
			{HCL: "dead_letter_queue", Wire: "deadLetterQueue", Kind: model.KindResourceRef, TargetKind: "AtLeastOnceQueue",
				Target:            requiresInterface("edge.queue", "1.0.0"),
				AbsenceIsSemantic: true,
				Doc: "Queue receiving messages that exhausted their retries. Without it, exhausted messages are dropped. " +
					"It must not resolve to the queue this consumer drains, or close a cycle through other consumers' " +
					"dead-letter destinations; a host refuses either before any mutation.",
				Example: ref("AtLeastOnceQueue", "dead-letters")},
			{HCL: "max_concurrency", Wire: "maxConcurrency", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(1), Max: model.I64(250),
				Doc:     "Largest number of concurrent batch invocations.",
				Example: 4},
		},
	},
}

// Validate proves every structural catalog rule: per-form model rules, the
// open-token guard, unique identities, and resolvable contract references.
func Validate() error {
	if err := model.ValidateNoOpenTokens(Forms); err != nil {
		return err
	}
	kinds := map[string]struct{}{}
	slugs := map[string]struct{}{}
	resourceTypes := map[string]struct{}{}
	for _, form := range Forms {
		if err := form.Validate(); err != nil {
			return err
		}
		if form.DefinitionVersion != edgeDefinitionVersion {
			return fmt.Errorf("form %s declares definition version %q; the MVP family line is %q", form.Kind, form.DefinitionVersion, edgeDefinitionVersion)
		}
		for name, set := range map[string]map[string]struct{}{form.Kind: kinds, form.Slug: slugs, form.ResourceType: resourceTypes} {
			if _, duplicate := set[name]; duplicate {
				return fmt.Errorf("duplicate catalog identity %q", name)
			}
			set[name] = struct{}{}
		}
		for _, source := range form.ProvidedInterfaces {
			if _, err := interfaceDefinitionByName(source.Name); err != nil {
				return fmt.Errorf("form %s: %w", form.Kind, err)
			}
		}
	}
	for _, field := range bindingListTargets() {
		if _, known := kinds[field]; !known {
			return fmt.Errorf("binding target kind %q is not a catalog Form", field)
		}
	}
	if err := validateBindingContracts(); err != nil {
		return err
	}
	if err := validateDerivedRelations(); err != nil {
		return err
	}
	if err := validateEnvironmentNamespaces(); err != nil {
		return err
	}
	if err := ValidateInterfaceDefinitions(InterfaceDefinitions()); err != nil {
		return err
	}
	if err := validateRuntimeContract(); err != nil {
		return err
	}
	return validateAbsenceSemanticExemptions()
}

// Grammars and bounds the published Interface Definition meta-schema fixes
// (spec/schemas/interface-definition-v1alpha1.schema.json). They are re-stated
// here so the catalog fails while it is being authored rather than after it has
// emitted bytes no host would accept.
var (
	interfaceOperationNamePattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{0,63}$`)
	interfaceErrorCodePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)
	interfaceFixtureNamePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
)

const (
	interfaceDescriptionMaxLength          = 4096
	interfaceOperationDescriptionMaxLength = 1024
	interfaceTitleMaxLength                = 160
)

// ValidateInterfaceDefinitions proves, at authoring time, that every Interface
// contract is internally coherent — not only the runtime ABI, which was the
// only one checked before decision 0020.
//
// The rules are the ones a reader of a definition assumes without being told:
// an operation name appears once, a fixture exercises an operation the contract
// declares, and a fixture that expects a failure names an error THAT operation
// lists. The last one is the important one. A fixture expecting an error the
// operation's closed vocabulary does not carry describes a conforming host as
// failing, so it is a trace no correct implementation can pass — the same class
// of defect as a required conformance check no correct host can complete.
//
// The meta-schema bounds are re-proved here for a different reason: those
// documents are generated, so a description that grew past the published
// maxLength would only be caught after the bytes were written, by whatever read
// them next.
// It takes the set rather than reading the catalog so a test can hand it a
// deliberately broken definition and prove the rule refuses it. A rule nothing
// can be shown to break is a rule nobody can trust.
func ValidateInterfaceDefinitions(definitions []InterfaceDefinition) error {
	names := map[string]struct{}{}
	for _, definition := range definitions {
		if _, duplicate := names[definition.Name]; duplicate {
			return fmt.Errorf("duplicate interface identity %q", definition.Name)
		}
		names[definition.Name] = struct{}{}
		if len(definition.Title) > interfaceTitleMaxLength {
			return fmt.Errorf("interface %s title is %d characters; the published schema bounds it at %d",
				definition.Name, len(definition.Title), interfaceTitleMaxLength)
		}
		if len(definition.Description) > interfaceDescriptionMaxLength {
			return fmt.Errorf("interface %s description is %d characters; the published schema bounds it at %d",
				definition.Name, len(definition.Description), interfaceDescriptionMaxLength)
		}
		operations := map[string]InterfaceOperation{}
		for _, operation := range definition.Operations {
			if !interfaceOperationNamePattern.MatchString(operation.Name) {
				return fmt.Errorf("interface %s declares operation %q outside the published name grammar",
					definition.Name, operation.Name)
			}
			if _, duplicate := operations[operation.Name]; duplicate {
				return fmt.Errorf("interface %s declares operation %q twice", definition.Name, operation.Name)
			}
			if len(operation.Description) > interfaceOperationDescriptionMaxLength {
				return fmt.Errorf(
					"interface %s operation %s description is %d characters; the published schema bounds it at %d",
					definition.Name, operation.Name, len(operation.Description), interfaceOperationDescriptionMaxLength,
				)
			}
			codes := map[string]struct{}{}
			for _, code := range operation.Errors {
				if !interfaceErrorCodePattern.MatchString(code) {
					return fmt.Errorf("interface %s operation %s declares error %q outside the published grammar",
						definition.Name, operation.Name, code)
				}
				if _, duplicate := codes[code]; duplicate {
					return fmt.Errorf("interface %s operation %s declares error %q twice",
						definition.Name, operation.Name, code)
				}
				codes[code] = struct{}{}
			}
			operations[operation.Name] = operation
		}
		if err := validateInterfaceFixtures(definition, operations); err != nil {
			return err
		}
	}
	return nil
}

func validateInterfaceFixtures(definition InterfaceDefinition, operations map[string]InterfaceOperation) error {
	fixtures := map[string]struct{}{}
	for _, fixture := range definition.Fixtures {
		if !interfaceFixtureNamePattern.MatchString(fixture.Name) {
			return fmt.Errorf("interface %s fixture %q is outside the published name grammar",
				definition.Name, fixture.Name)
		}
		if _, duplicate := fixtures[fixture.Name]; duplicate {
			return fmt.Errorf("interface %s declares fixture %q twice", definition.Name, fixture.Name)
		}
		fixtures[fixture.Name] = struct{}{}
		if len(fixture.Steps) == 0 {
			return fmt.Errorf("interface %s fixture %s has no steps", definition.Name, fixture.Name)
		}
		for _, step := range fixture.Steps {
			operation, declared := operations[step.Operation]
			if !declared {
				return fmt.Errorf("interface %s fixture %s exercises undeclared operation %q",
					definition.Name, fixture.Name, step.Operation)
			}
			if step.Input == nil {
				return fmt.Errorf("interface %s fixture %s step %s carries no input",
					definition.Name, fixture.Name, step.Operation)
			}
			if step.ExpectedError == "" {
				continue
			}
			if len(step.Expected) > 0 {
				return fmt.Errorf(
					"interface %s fixture %s step %s expects both an output and the error %q; a step has one outcome",
					definition.Name, fixture.Name, step.Operation, step.ExpectedError,
				)
			}
			if !slices.Contains(operation.Errors, step.ExpectedError) {
				return fmt.Errorf(
					"interface %s fixture %s expects error %q, which operation %s does not declare; "+
						"a conforming host may not produce it, so no correct implementation could pass the trace",
					definition.Name, fixture.Name, step.ExpectedError, step.Operation,
				)
			}
		}
	}
	return nil
}

// validateRuntimeContract proves, at authoring time, the three facts that make
// the ES Module Worker ABI an exact contract rather than a claim
// ([decision 0019](../../spec/decisions/0019-the-module-worker-abi-is-an-exact-contract.md)):
//
//  1. the ModuleWorker identity — what a host implements — provides the runtime
//     contract, so the exact digest travels in the Form Definition a host
//     installs;
//  2. the WorkerVersion `handlers` enum IS the contract's handler vocabulary,
//     so the code side and the ABI side cannot drift apart;
//  3. every handler in that vocabulary is a real operation of the contract, and
//     every fixture step names an operation the contract declares.
//
// A Form that stated a runtime by a compatibility date could satisfy none of
// these: a date names no operations, no digest, and no behavior.
func validateRuntimeContract() error {
	definition, err := interfaceDefinitionByName(WorkerRuntimeInterfaceName)
	if err != nil {
		return err
	}
	worker, known := ByKind("ModuleWorker")
	if !known {
		return fmt.Errorf("the family declares no ModuleWorker Form to own the %s contract", WorkerRuntimeInterfaceName)
	}
	provides := false
	for _, provided := range worker.ProvidedInterfaces {
		if provided.Name == WorkerRuntimeInterfaceName && provided.Version == definition.Version {
			provides = true
			break
		}
	}
	if !provides {
		return fmt.Errorf(
			"form ModuleWorker does not provide %s@%s; the ABI it claims to fix would be unstated",
			WorkerRuntimeInterfaceName, definition.Version,
		)
	}
	handlers, err := runtimeHandlersOf(definition)
	if err != nil {
		return err
	}
	operations := map[string]struct{}{}
	for _, operation := range definition.Operations {
		operations[operation.Name] = struct{}{}
	}
	for _, handler := range handlers {
		if _, declared := operations[handler]; !declared {
			return fmt.Errorf(
				"interface %s names handler %q in its %s vocabulary without declaring it as an operation",
				WorkerRuntimeInterfaceName, handler, RuntimeHandlerProperty,
			)
		}
	}
	for _, fixture := range definition.Fixtures {
		for _, step := range fixture.Steps {
			if _, declared := operations[step.Operation]; !declared {
				return fmt.Errorf(
					"interface %s fixture %s exercises undeclared operation %q",
					WorkerRuntimeInterfaceName, fixture.Name, step.Operation,
				)
			}
		}
	}
	version, known := ByKind("WorkerVersion")
	if !known {
		return errors.New("the family declares no WorkerVersion Form to fill the runtime contract")
	}
	for _, field := range version.Fields {
		if field.Wire != "handlers" {
			continue
		}
		if !slices.Equal(field.Enum, handlers) {
			return fmt.Errorf(
				"form WorkerVersion handlers enum %v is not the %s handler vocabulary %v",
				field.Enum, WorkerRuntimeInterfaceName, handlers,
			)
		}
		return nil
	}
	return errors.New("form WorkerVersion declares no handlers field to hold the runtime contract vocabulary")
}

// validateEnvironmentNamespaces proves each Form's canonical Examples declare a
// collision-free runtime environment namespace
// ([decision 0016](../../spec/decisions/0016-the-worker-aggregate-has-one-active-deployment.md)).
//
// Only the WorkerVersion has such a namespace today: it is the one Form that
// holds typed bindings alongside `vars` and `requiredSensitiveVars`, and all
// three project into the single environment object its code receives. The rule
// is stated for every Form because the next revision Form that holds bindings
// inherits it without an edit here.
//
// The collision this catches is the one the AUTHORING model can decide. Whether
// two declarations collide is otherwise a property of one instance, not of a
// Form: a Form declaring five binding lists is not wrong, its author may simply
// write one name in two of them. That instance rule is enforced by the provider
// at plan time and by a conforming host before mutation.
func validateEnvironmentNamespaces() error {
	for _, form := range Forms {
		if err := model.ValidateEnvironmentNamespace(form); err != nil {
			return err
		}
	}
	return nil
}

// validateBindingContracts proves, at authoring time, exactly the rules a
// conforming host verifies at apply time (spec/binding-contract/README.md): the
// declaring Form's role is the Binding's sourceRole, the field's target Form is
// listed in allowedTargetForms, and that target Form provides the Binding's
// targetInterface. A catalog that violates any of them would ship a Form whose
// every binding a correct host must refuse.
func validateBindingContracts() error {
	definitions, err := BindingDefinitions()
	if err != nil {
		return err
	}
	byName := map[string]BindingDefinition{}
	for _, definition := range definitions {
		byName[definition.Name] = definition
	}
	for _, form := range Forms {
		accepted := map[string]struct{}{}
		for _, source := range form.AcceptedBindings {
			definition, known := byName[source.Name]
			if !known {
				return fmt.Errorf("form %s accepts unknown binding %q", form.Kind, source.Name)
			}
			if definition.Version != source.Version {
				return fmt.Errorf("form %s accepts binding %s@%s; the catalog carries %s", form.Kind, source.Name, source.Version, definition.Version)
			}
			accepted[source.Name] = struct{}{}
		}
		for _, field := range form.Fields {
			if field.Kind != model.KindBindingList {
				continue
			}
			definition, known := byName[field.BindingType]
			if !known {
				return fmt.Errorf("form %s field %s names unknown binding %q", form.Kind, field.Wire, field.BindingType)
			}
			if _, declared := accepted[field.BindingType]; !declared {
				return fmt.Errorf("form %s field %s carries binding %s without accepting it", form.Kind, field.Wire, field.BindingType)
			}
			if string(form.Role) != definition.SourceRole {
				return fmt.Errorf(
					"form %s role %s holds binding %s whose sourceRole is %s",
					form.Kind, form.Role, definition.Name, definition.SourceRole,
				)
			}
			allowed := false
			for _, target := range definition.AllowedTargetForms {
				if target.APIVersion == Family.APIVersion() && target.Kind == field.TargetKind {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf(
					"form %s field %s targets %s, which binding %s does not list in allowedTargetForms",
					form.Kind, field.Wire, field.TargetKind, definition.Name,
				)
			}
			target, known := ByKind(field.TargetKind)
			if !known {
				return fmt.Errorf("form %s field %s targets unknown kind %q", form.Kind, field.Wire, field.TargetKind)
			}
			provides := false
			for _, provided := range target.ProvidedInterfaces {
				if provided.Name == definition.TargetInterface.Name && provided.Version == definition.TargetInterface.Version {
					provides = true
					break
				}
			}
			if !provides {
				return fmt.Errorf(
					"form %s field %s binds %s, but target Form %s does not provide interface %s@%s",
					form.Kind, field.Wire, definition.Name, target.Kind,
					definition.TargetInterface.Name, definition.TargetInterface.Version,
				)
			}
		}
	}
	return nil
}

// validateDerivedRelations proves the derivation reads back what the catalog
// declared: every reference field appears in the relations derived from the
// emitted desired schema, with the target group and kind the field pinned, and
// with exactly one stated target contract that the declared target can actually
// satisfy. Relations are derived, never declared, so this is the only place the
// two views are compared.
func validateDerivedRelations() error {
	resolver := newTargetContractResolver()
	for _, form := range Forms {
		schema, err := form.DesiredSchema(resolver)
		if err != nil {
			return fmt.Errorf("form %s: %w", form.Kind, err)
		}
		relations, err := model.DeriveRelations(schema)
		if err != nil {
			return fmt.Errorf("form %s: %w", form.Kind, err)
		}
		for _, relation := range relations {
			if relation.TargetAPIVersion != Family.APIVersion() {
				return fmt.Errorf(
					"form %s relation %s targets group %q outside the family",
					form.Kind, relation.Pointer, relation.TargetAPIVersion,
				)
			}
			target, known := ByKind(relation.TargetKind)
			if !known {
				return fmt.Errorf(
					"form %s relation %s targets kind %q, which is not a catalog Form",
					form.Kind, relation.Pointer, relation.TargetKind,
				)
			}
			if err := validateRelationTargetContract(form.Kind, relation, target); err != nil {
				return err
			}
		}
		if got, want := len(relations), declaredReferenceCount(form.Fields); got != want {
			return fmt.Errorf("form %s derives %d relations from %d declared reference fields", form.Kind, got, want)
		}
	}
	return nil
}

// validateRelationTargetContract proves one derived relation's stated
// requirement is one the declared target can meet, and that a binding relation
// requires exactly the Interface its Binding Definition projects.
//
// The last rule is what keeps the annotation from becoming a second source of
// truth: the Binding Definition's targetInterface is the authority, the
// annotation is its projection onto the reference, and a catalog whose two
// spellings disagreed does not render at all (decision 0014).
func validateRelationTargetContract(kind string, relation model.Relation, target model.Form) error {
	switch {
	case len(relation.TargetFormRefs) > 0:
		for _, ref := range relation.TargetFormRefs {
			if ref.Kind != target.Kind || ref.DefinitionVersion != target.DefinitionVersion {
				return fmt.Errorf(
					"form %s relation %s pins target identity %s, which is not the catalog's %s@%s",
					kind, relation.Pointer, ref, target.Kind, target.DefinitionVersion,
				)
			}
		}
	case relation.RequiredInterface != nil:
		provides := false
		for _, provided := range target.ProvidedInterfaces {
			if provided.Name == relation.RequiredInterface.Name &&
				provided.Version == relation.RequiredInterface.Version {
				provides = true
				break
			}
		}
		if !provides {
			return fmt.Errorf(
				"form %s relation %s requires interface %s@%s, which target Form %s does not provide",
				kind, relation.Pointer, relation.RequiredInterface.Name,
				relation.RequiredInterface.Version, target.Kind,
			)
		}
	default:
		return fmt.Errorf("form %s relation %s states no target contract", kind, relation.Pointer)
	}
	if !relation.IsBinding() {
		return nil
	}
	definitions, err := BindingDefinitions()
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		if definition.Name != relation.Binding {
			continue
		}
		if relation.RequiredInterface == nil ||
			relation.RequiredInterface.Name != definition.TargetInterface.Name ||
			relation.RequiredInterface.Version != definition.TargetInterface.Version {
			return fmt.Errorf(
				"form %s binding relation %s must require exactly binding %s's targetInterface %s@%s",
				kind, relation.Pointer, definition.Name,
				definition.TargetInterface.Name, definition.TargetInterface.Version,
			)
		}
		return nil
	}
	return fmt.Errorf("form %s relation %s names unknown binding %q", kind, relation.Pointer, relation.Binding)
}

func declaredReferenceCount(fields []model.Field) int {
	count := 0
	for _, field := range fields {
		switch field.Kind {
		case model.KindResourceRef, model.KindResourceRefList, model.KindBindingList:
			count++
		}
		count += declaredReferenceCount(field.Fields)
	}
	return count
}

// absenceSemanticExemptions is the complete, reviewed list of family fields
// whose absence carries portable meaning instead of a default. It is written
// out so the exemption is an auditable fact rather than a marker anyone can
// add: a new AbsenceIsSemantic field fails the catalog until it is listed
// here, which forces the review that decides whether it deserves the
// exemption at all.
var absenceSemanticExemptions = map[string]struct{}{
	"QueueConsumer/deadLetterQueue": {},
}

func validateAbsenceSemanticExemptions() error {
	for _, form := range Forms {
		for _, field := range form.Fields {
			if !field.AbsenceIsSemantic {
				continue
			}
			key := form.Kind + "/" + field.Wire
			if _, allowed := absenceSemanticExemptions[key]; !allowed {
				return fmt.Errorf(
					"form %s field %s marks AbsenceIsSemantic without a reviewed exemption; "+
						"declare a portable Default instead, or add %s to absenceSemanticExemptions",
					form.Kind, field.Wire, key,
				)
			}
		}
	}
	return nil
}

func bindingListTargets() []string {
	var targets []string
	for _, form := range Forms {
		for _, field := range form.Fields {
			if field.Kind == model.KindBindingList || field.Kind == model.KindResourceRef || field.Kind == model.KindResourceRefList {
				targets = append(targets, field.TargetKind)
			}
		}
	}
	return targets
}

// ByKind returns the declared Form for a portable kind token.
func ByKind(kind string) (model.Form, bool) {
	for _, candidate := range Forms {
		if candidate.Kind == kind {
			return candidate, true
		}
	}
	return model.Form{}, false
}
