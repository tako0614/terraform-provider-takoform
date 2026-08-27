# Provider mapping inventory

This inventory records the current Provider 3 projection. Provider resource
type names and schema choices are adapter metadata. Published Edge definitions
are maintained in the standalone [takoform-forms source](https://github.com/tako0614/takoform-forms);
other entries are exact embedded Provider projections. Each current FormRef
carries an exact `definitionVersion` within its versionless family group.

The generated candidate index is `forms/candidates/current-family-index.json`.
It binds the eight family candidate sets plus shared Interface and Binding
sets by SHA-256, so this inventory and the checked-in contracts stay aligned.

## `edge.forms.takoform.com`

| Kind | Role | Version | Portable intent |
| --- | --- | --- | --- |
| `ModuleWorker` | `identity` | `0.1.0` | Long-lived logical identity of one ES Module Worker application. The Form fixes the ES Module Worker ABI by identity, and states it exactly: the runtime contract worker.runtime@1.1.0 in this Form's providedInterfaces fixes the module's default-export shape, the fetch, scheduled, and queue handler signatures, the binding environment, ctx.waitUntil, exception handling, body streaming, the minimum Web API surface, and module loading. A host supporting this Form implements that exact digest; a runtime that behaves differently is a different contract version and a different Form version, never a compatibility date. Code, configuration, and bindings live on Worker Version revisions; traffic selection lives on Worker Deployments. |
| `WorkerBundle` | `revision` | `0.1.0` | Immutable content-addressed module bundle of one worker build, named by the digest of the artifact manifest committed through the content-addressed upload API (decision 0012). The manifest, not this Form, describes the main module and every additional module with its closed media type, exact size, and sha256 digest, so the bundle keeps exactly one source of truth for its bytes. Different bytes commit a different manifest, which is a different bundle. |
| `StaticAssetBundle` | `revision` | `0.1.0` | Immutable content-addressed set of files served beside one Worker Version. The whole portable desired state is the digest of a committed artifacts.takoform.com/v1alpha1 StaticAssetBundle manifest; the manifest is the sole ordered inventory of path, media type, exact size, and sha256 digest for every file. Raw file bytes, upload locations, backend identities, and serving policy never enter this resource. Serving order and not-found behavior belong to the Worker Version attachment so the same bytes can participate in different immutable versions without giving the bundle two meanings. |
| `WorkerVersion` | `revision` | `0.2.0` | Immutable executable snapshot of one Module Worker: a bundle, the handlers its module exports, non-secret vars, and the typed capability bindings the code may use. A change is a new Worker Version; traffic moves only through Worker Deployments. The runtime this code runs on is not a field of this Form: it is fixed by the worker.runtime@1.1.0 contract the Module Worker identity provides, so a version carries no compatibility date and no compatibility flag (decision 0019). |
| `WorkerDeployment` | `deployment` | `0.1.0` | Selects which Worker Versions of one Module Worker serve traffic and in what proportion. Weights are basis points and must sum to exactly 10000 across entries; the sum is host-validated semantics because a schema cannot add weights. Rollback is re-weighting, never mutating a revision. |
| `WorkerCustomDomain` | `attachment` | `0.1.0` | Attaches one DNS hostname to a Module Worker so its active deployment serves that hostname over HTTPS. Inward activation is an attachment, never a binding; deleting the attachment detaches the hostname and never deletes the worker. A hostname is a name in DNS rather than a label in this host's namespace, so it is CANONICALIZED before it is compared and before it is stored — trailing root dot removed, ASCII letters lowercased — and one canonical hostname is served by AT MOST ONE attachment per tenant, across every space. A second attachment claiming a hostname a live one already serves is refused before any mutation; releasing the holder makes the claim representable (decision 0026). |
| `WorkerEndpoint` | `attachment` | `0.1.0` | Makes one Module Worker reachable over HTTPS at an address the HOST assigns, with no customer-owned domain and no DNS of the author's. The desired state is the worker and nothing else: the author asks for reachability, and where that reachability lives is the host's decision, exactly as an account, a region, and a vendor subdomain are. Requests arriving at the address invoke the fetch handler of the worker's ACTIVE DEPLOYMENT, so promotion and rollback move what answers without the endpoint being re-applied and without its address changing. The scheme is https and the path root is `/`; TLS is not an option a host may decline, because an address that is only reachable in plaintext is a different promise from the one this Form makes. A worker has AT MOST ONE endpoint: two would be two addresses for one service with nothing saying which is canonical, and the second is refused. The assigned address is published as outputs — a portable author may rely on a value being returned, on its scheme being https, and on it routing to the active deployment, and on nothing about its SHAPE: which subdomain, which apex, and how long the label is are host detail no portable configuration may parse or reconstruct. The address is IMMUTABLE for the lifetime of the endpoint's attachment UID. It MUST NOT change on deployment promotion, on status refresh, on a host's internal placement change, or on a backend migration; a host that needs to change the address deletes the endpoint and the author creates a new one, which is a new attachment with a new UID. |
| `WorkerCronTrigger` | `attachment` | `0.1.0` | Attaches one cron schedule to a Module Worker, invoking its scheduled handler at each match. Schedules are interpreted in UTC only; there is no timezone field, so two hosts can never fire the same trigger at different instants and no schedule ever skips or repeats an hour for a daylight-saving transition. The grammar is five fields separated by single spaces — minute 0-59, hour 0-23, day-of-month 1-31, month 1-12, day-of-week 0-6 with 0 Sunday — and each field is a comma-separated list of `*`, a literal, a range `low-high`, `*/step`, or `low-high/step`. Names and a step on a bare literal are not accepted, and neither is any value outside its field's own range, an inverted range, or a step outside 1..span. When day-of-month and day-of-week are BOTH restricted the trigger fires on a day either of them selects; when only one is restricted only that one constrains the day. A missed run is not made up: a host that could not fire a match — because it was unavailable, or because the previous invocation was still running — skips it rather than firing late, so a schedule never produces a backlog. At-least-once delivery applies to each match: a handler may be invoked more than once for one matched minute, and it must be idempotent. An uncaught exception in the handler is a failed invocation reported to host diagnostics; it is not retried within the matched minute and it never becomes an HTTP response. |
| `EdgeKVNamespace` | `identity` | `0.1.0` | Globally replicated key/value namespace of opaque BYTES with eventual consistency, exactly as fixed by the edge.kv Interface. Eventual consistency is the Form's semantics, not an option: a store with different convergence behavior is a different Form, and this one promises no read-your-writes, in any session, at any location. Values are byte strings carried in the family's encoded-bytes shape, so the declared byte limit and the structural string ceiling measure the same thing (decision 0020). |
| `SQLiteDatabase` | `identity` | `0.1.0` | Embedded SQLite database with bounded EdgeSqlValue values, rollback-only queries, and serializable all-or-none transactions, exactly as fixed by the edge.sql Interface. SQLite semantics are the identity: a database with different SQL, typing, or isolation behavior is a different Form, never an engine token. Numbers stay finite binary64 values inside Number.MAX_SAFE_INTEGER and do not expose the INTEGER/REAL storage-class distinction; BLOB uses the common canonical encoded-bytes object. Runtime SQL cannot migrate schema; SQLiteMigrationApplication owns that administrative path (decision 0034). |
| `SQLiteMigrationSet` | `revision` | `0.1.0` | Immutable ordered SQLite migration history backed by one committed artifacts.takoform.com/v1alpha1 MigrationBundle manifest. The manifest files array is the migration order; every entry has a unique portable path, media type application/sql, exact size, and sha256 digest. SQL bytes travel only through the content-addressed artifact upload and never enter desired state. A new set may append entries, but an application refuses any set whose prefix rewrites, reorders, or removes an already applied path+digest pair. |
| `SQLiteMigrationApplication` | `attachment` | `0.1.0` | Applies one exact SQLite Migration Set to one exact SQLite Database. Both relations are immutable and UID-pinned before mutation. The database's durable migration ledger records each applied manifest entry as its ordered path+digest pair. The requested set must extend that ledger exactly; a rewrite, reorder, or removal is refused before SQL executes, and only the unapplied suffix runs. Each file and its ledger append commit atomically, so an interrupted application retries the same suffix without replaying a recorded migration. Ready means the ledger equals the referenced set. Deleting this attachment only stops managing the application resource: it never runs down-migrations, rewrites the ledger, reverts schema, or deletes the database. |
| `AtLeastOnceQueue` | `identity` | `0.1.0` | Message queue with at-least-once delivery and no ordering guarantee, exactly as fixed by the edge.queue Interface. There is no ordering field: a FIFO queue is a different Form. Message bodies are opaque bytes, a message identity is stable across redeliveries, and a queue has AT MOST ONE consumer — two would split the stream between two retry policies and two dead-letter destinations, which leaves the queue's own behavior unstatable (decision 0020). |
| `QueueConsumer` | `attachment` | `0.1.0` | Attaches one Module Worker as the batch consumer of one At-Least-Once Queue, invoking its queue handler with message batches and redelivering messages that were not acknowledged. Consumption is inward activation and therefore an attachment, never a binding. One queue has at most one consumer, so a second attachment against the same queue is refused. A handler that returns normally without settling anything acknowledges the whole batch; one that throws retries every message it had not already acknowledged. maxRetries counts REDELIVERIES only — the first delivery does not count toward it — so a message is delivered at most 1 + maxRetries times, and a message that exhausts them moves to dead_letter_queue when one is declared and is dropped otherwise. The dead-letter copy is a new message there: new identity, new acceptance timestamp, and an attempt count starting again at 1 (decision 0020). Because that transfer resets the attempt count, dead_letter_queue MUST NOT lead back: a destination resolving to the queue this consumer drains, or closing a cycle of any length through the dead-letter graph, is refused before any mutation, because an exhausted message would circulate forever instead of coming to rest (decision 0026). |
| `DurableWorkflow` | `identity` | `0.1.0` | Long-lived identity of one code-defined durable workflow: a class the worker's active deployment serves, whose instances survive process death. The Form fixes the execution model by identity — the worker.workflow@1.0.0 contract in its providedInterfaces states memoized replay, at-least-once step execution, the closed status vocabulary, and the two bounds that keep an instance finite. It carries NO implementation snapshot: which code answers is whatever the worker's active Worker Deployment selects, so behavior upgrades and rollback ride the deployment like any other traffic change. Instances are runtime data reached through module-worker.workflow bindings, never Resources. One worker carries at most one Durable Workflow per class name. |
| `ActorNamespace` | `identity` | `0.1.0` | Long-lived identity of one addressable-actor id space: a class the worker's active deployment serves, with at most one live execution context per actor id, private durable storage per id, and one alarm per id. The Form fixes that model by identity through the worker.actor@1.0.0 contract in its providedInterfaces; it carries no implementation snapshot, so which code answers is whatever the worker's active Worker Deployment selects. Actors are runtime data reached through module-worker.actor bindings, never Resources: every id addresses an actor and the first delivery is its creation. One worker carries at most one Actor Namespace per class name, because two namespaces over one class would give that class two disjoint id spaces. |

## `function.forms.takoform.com`

| Kind | Role | Version | Portable intent |
| --- | --- | --- | --- |
| `Function` | `identity` | `0.1.0` | Logical identity of one regional JavaScript function. Code, configuration, resource bounds, and traffic are represented by the surrounding revision and deployment Forms; this identity carries no desired fields. |
| `FunctionVersion` | `revision` | `0.1.0` | Immutable content-addressed executable snapshot of one Function: artifact manifest, handler, environment declarations, sealed slots, and invocation bounds. |
| `FunctionDeployment` | `deployment` | `0.1.0` | The one active traffic selection for a Function. One or two immutable versions carry positive basis-point weights totaling exactly 10000. |
| `FunctionEndpoint` | `attachment` | `0.1.0` | Host-assigned HTTPS reachability for the active Function Deployment. The address is output, not desired state, and remains stable for the attachment UID. |

## `container.forms.takoform.com`

| Kind | Role | Version | Portable intent |
| --- | --- | --- | --- |
| `ContainerService` | `identity` | `0.1.0` | Logical identity of one request-driven serverless container service. Image, process configuration, resources, scaling, and traffic are represented by immutable revisions and the active traffic Form around this identity. |
| `ContainerRevision` | `revision` | `0.1.0` | Immutable serving snapshot of one Container Service: a digest-pinned OCI image, process arguments, environment declarations, sealed slots, and resource and scaling bounds. |
| `ContainerTraffic` | `deployment` | `0.1.0` | The one active basis-point traffic selection for a Container Service. One to eight immutable revisions carry positive weights totaling exactly 10000. |
| `ContainerEndpoint` | `attachment` | `0.1.0` | Host-assigned HTTPS reachability for the active Container Traffic. The address is output, not desired state, and remains stable for the attachment UID. |
| `ContainerCustomDomain` | `attachment` | `0.1.0` | HTTPS attachment that serves one canonical customer-owned hostname from the active Container Traffic. ACME certificate issuance and TLS termination are host duties. |

## `table.forms.takoform.com`

| Kind | Role | Version | Portable intent |
| --- | --- | --- | --- |
| `Table` | `identity` | `0.1.0` | Key-addressed document table with a declared partition key, optional sort key, mutable secondary indexes, and optional lazy TTL. The table.document Interface fixes document values, conditional writes, consistent single-item reads, and key-ordered partition queries; this identity carries only the table's addressing declaration. |

## `queue.forms.takoform.com`

| Kind | Role | Version | Portable intent |
| --- | --- | --- | --- |
| `PullQueue` | `identity` | `0.1.0` | Unordered at-least-once pull queue with visibility timeout, receive counting, optional dead-lettering, and bounded long polling. The queue.pull Interface fixes the send, receive, delete, and changeVisibility behavior; these fields are the queue's retention and delivery-policy declaration. |

## `topic.forms.takoform.com`

| Kind | Role | Version | Portable intent |
| --- | --- | --- | --- |
| `Topic` | `identity` | `0.1.0` | Fanout topic whose accepted publishes are delivered at least once to every matching TopicSubscription. The topic retains and replays nothing; the topic.publish Interface fixes the message and publish semantics. |
| `TopicSubscription` | `attachment` | `0.1.0` | Attachment that delivers each matching Topic publish into one PullQueue. Delivery is independent and at least once per subscription; retry and dead-letter behavior belong to this attachment. |

## `schedule.forms.takoform.com`

| Kind | Role | Version | Portable intent |
| --- | --- | --- | --- |
| `Schedule` | `identity` | `0.1.0` | UTC five-field cron schedule that delivers one declared message at each matched window to either a PullQueue or Topic. Delivery is at least once; failed attempts use the declared bounded retry policy and missed windows are never replayed. |

## `vector.forms.takoform.com`

| Kind | Role | Version | Portable intent |
| --- | --- | --- | --- |
| `VectorIndex` | `identity` | `0.1.0` | Fixed-dimension dense vector index with a creation-time distance metric. The vector.index Interface fixes namespaced whole-record upsert, read-after-write fetch, approximate top-k query, closed metadata filtering, and deletion; this identity carries only the embedding dimension and metric. |

Current Provider 3 mapping has exactly 31 typed resources in 8 versionless families. Edge has
exactly 16 members in that mapping and intentionally has no ObjectBucket Form,
`edge.objects` Interface, or ObjectBucket Binding. Retained versioned
Provider 2.1.1 packages remain immutable history and are not members of this
mapping.

The Terraform Provider reference docs and examples under
`docs/resources` and `examples/resources` cover only mappings that
the provider explicitly owns. Their `takoform_*` names are non-normative
and cannot affect a Form Definition or digest. Missing mappings for other
families are provider-registration work, not a reason to omit or alter those
families here.
