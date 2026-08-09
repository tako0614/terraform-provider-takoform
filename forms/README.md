# Current v1alpha2 Form candidates

This is the provider-v2 source candidate inventory for the nine Form-backed
Resources currently operated by Takosumi Cloud. Every entry is a local
Proposal-derived publication candidate under `forms.takoform.com/v1alpha2`,
awaiting an explicit lifecycle transition before Experimental; none is published, Experimental, Stable,
centrally approved, or guaranteed commercially available.
Each contract describes what a caller wants without
naming a target, credential, placement, price, or implementation. A host may
publish support and activate an exact FormRef under its own policy.

The frozen v1alpha1 inventory remains verifiable through
[`standard-package-set.json`](standard-package-set.json) and immutable
release sources, but it is not rendered as the current provider catalog.

## Compute and application

| Kind | Resource | Version | Portable intent |
| --- | --- | --- | --- |
| `EdgeWorker` | `takoform_edge_worker` | `0.1.0` | Portable request/event application executed from digest-bound artifact bytes near an ingress boundary. |
| `Schedule` | `takoform_schedule` | `0.1.0` | Portable cron lifecycle that invokes exactly one connected Resource. |
| `ContainerService` | `takoform_container_service` | `0.1.0` | Portable service executed from an immutable OCI image digest. |
| `StatefulEntity` | `takoform_stateful_entity` | `0.1.0` | Portable namespace of addressable persistent entities implemented by digest-bound application bytes. |

## Data and storage

| Kind | Resource | Version | Portable intent |
| --- | --- | --- | --- |
| `RelationalDatabase` | `takoform_relational_database` | `0.1.0` | Portable relational database identified by an open engine capability token. |
| `ObjectBucket` | `takoform_object_bucket` | `0.1.0` | Portable object storage namespace. |
| `KeyValueStore` | `takoform_key_value_store` | `0.1.0` | Portable key/value state with declared consistency and expiry semantics. |
| `Queue` | `takoform_queue` | `0.1.0` | Portable asynchronous at-least-once message delivery. |
| `VectorIndex` | `takoform_vector_index` | `0.1.0` | Portable vector index with dimensions fixed for the index lifecycle. |

## Declared runtime interfaces

A Form may declare the runtime interfaces its service exposes. The names are
author-defined and open: there is no registry, no allowlist, and no central
approval. A declaration states what exists and how its non-secret values are
filled; the host creates the record, authorizes consumers, and owns its
lifecycle.

| Kind | Interface |
| --- | --- |
| `EdgeWorker` | `http.request@1` (request) |
| `RelationalDatabase` | `sql.query@1` (execute, query, transaction) |
| `ObjectBucket` | `object.storage@1` (delete, get, list, put) |
| `KeyValueStore` | `keyvalue.store@1` (delete, get, list, put) |
| `Queue` | `queue.messages@1` (acknowledge, receive, send) |
| `ContainerService` | `http.request@1` (request) |
| `StatefulEntity` | `entity.invoke@1` (invoke) |
| `VectorIndex` | `vector.query@1` (delete, query, upsert) |

## Immutable fields

Every Form fixes its `/name`. A Form that additionally fixes a field states so
in its definition, and the provider enforces replacement for exactly those
fields; the protocol lifecycle proves both.

| Kind | Immutable |
| --- | --- |
| `EdgeWorker` | `/name` |
| `RelationalDatabase` | `/databaseName`, `/engine`, `/name` |
| `ObjectBucket` | `/name` |
| `KeyValueStore` | `/name` |
| `Queue` | `/name` |
| `Schedule` | `/name` |
| `ContainerService` | `/name` |
| `StatefulEntity` | `/name` |
| `VectorIndex` | `/dimensions`, `/name` |

## Status

Every entry in this inventory is an unpublished `0.1.0` candidate.
Takosumi Cloud implementation is workload and first-host evidence only; it
does not turn a Proposal into a portable standard or authorize publication.

The earlier ten-package generation is also retired, not erased. Its immutable
bytes and admission evidence stay verifiable through
[`retired-package-set.json`](retired-package-set.json). Neither retained
set may be rewritten, re-signed, promoted, or used to derive a current approved
subset. Current lifecycle truth comes only from
[`lifecycle.json`](lifecycle.json); Host Support and activation remain
separate host-owned facts.

## Edge Platform Family (edge.forms.takoform.com/v1alpha1)

The first official Form Family fixes the shape of a proven edge developer
platform without naming its vendor (spec/form-families.md). Its members are
source candidates for the Host API v1alpha3 resource lane; the typed
resources require provider v2.1.0 or later (source candidate; not yet
published). Roles come from the closed v1alpha3 role enum and decide
lifecycle mechanics: revisions are immutable, deployments move traffic,
attachments activate inward events.

| Kind | Resource | Role | Version | Portable intent |
| --- | --- | --- | --- | --- |
| `ModuleWorker` | `takoform_module_worker` | `identity` | `0.1.0` | Long-lived logical identity of one ES Module Worker application. The Form fixes the ES Module Worker ABI by identity, and states it exactly: the runtime contract worker.runtime@1.0.0 in this Form's providedInterfaces fixes the module's default-export shape, the fetch, scheduled, and queue handler signatures, the binding environment, ctx.waitUntil, exception handling, body streaming, the minimum Web API surface, and module loading. A host supporting this Form implements that exact digest; a runtime that behaves differently is a different contract version and a different Form version, never a compatibility date. Code, configuration, and bindings live on Worker Version revisions; traffic selection lives on Worker Deployments. |
| `WorkerBundle` | `takoform_worker_bundle` | `revision` | `0.1.0` | Immutable content-addressed module bundle of one worker build, named by the digest of the artifact manifest committed through the content-addressed upload API (decision 0012). The manifest, not this Form, describes the main module and every additional module with its closed media type, exact size, and sha256 digest, so the bundle keeps exactly one source of truth for its bytes. Different bytes commit a different manifest, which is a different bundle. |
| `WorkerVersion` | `takoform_worker_version` | `revision` | `0.1.0` | Immutable executable snapshot of one Module Worker: a bundle, the handlers its module exports, non-secret vars, and the typed capability bindings the code may use. A change is a new Worker Version; traffic moves only through Worker Deployments. The runtime this code runs on is not a field of this Form: it is fixed by the worker.runtime@1.0.0 contract the Module Worker identity provides, so a version carries no compatibility date and no compatibility flag (decision 0019). |
| `WorkerDeployment` | `takoform_worker_deployment` | `deployment` | `0.1.0` | Selects which Worker Versions of one Module Worker serve traffic and in what proportion. Weights are basis points and must sum to exactly 10000 across entries; the sum is host-validated semantics because a schema cannot add weights. Rollback is re-weighting, never mutating a revision. |
| `WorkerCustomDomain` | `takoform_worker_custom_domain` | `attachment` | `0.1.0` | Attaches one DNS hostname to a Module Worker so its active deployment serves that hostname over HTTPS. Inward activation is an attachment, never a binding; deleting the attachment detaches the hostname and never deletes the worker. A hostname is a name in DNS rather than a label in this host's namespace, so it is CANONICALIZED before it is compared and before it is stored — trailing root dot removed, ASCII letters lowercased — and one canonical hostname is served by AT MOST ONE attachment per tenant, across every space. A second attachment claiming a hostname a live one already serves is refused before any mutation; releasing the holder makes the claim representable (decision 0026). |
| `WorkerEndpoint` | `takoform_worker_endpoint` | `attachment` | `0.1.0` | Makes one Module Worker reachable over HTTPS at an address the HOST assigns, with no customer-owned domain and no DNS of the author's. The desired state is the worker and nothing else: the author asks for reachability, and where that reachability lives is the host's decision, exactly as an account, a region, and a vendor subdomain are. Requests arriving at the address invoke the fetch handler of the worker's ACTIVE DEPLOYMENT, so promotion and rollback move what answers without the endpoint being re-applied and without its address changing. The scheme is https and the path root is `/`; TLS is not an option a host may decline, because an address that is only reachable in plaintext is a different promise from the one this Form makes. A worker has AT MOST ONE endpoint: two would be two addresses for one service with nothing saying which is canonical, and the second is refused. The assigned address is published as outputs — a portable author may rely on a value being returned, on its scheme being https, and on it routing to the active deployment, and on nothing about its SHAPE: which subdomain, which apex, and how long the label is are host detail no portable configuration may parse or reconstruct. The address is IMMUTABLE for the lifetime of the endpoint's attachment UID. It MUST NOT change on deployment promotion, on status refresh, on a host's internal placement change, or on a backend migration; a host that needs to change the address deletes the endpoint and the author creates a new one, which is a new attachment with a new UID. |
| `WorkerCronTrigger` | `takoform_worker_cron_trigger` | `attachment` | `0.1.0` | Attaches one cron schedule to a Module Worker, invoking its scheduled handler at each match. Schedules are interpreted in UTC only; there is no timezone field, so two hosts can never fire the same trigger at different instants and no schedule ever skips or repeats an hour for a daylight-saving transition. The grammar is five fields separated by single spaces — minute 0-59, hour 0-23, day-of-month 1-31, month 1-12, day-of-week 0-6 with 0 Sunday — and each field is a comma-separated list of `*`, a literal, a range `low-high`, `*/step`, or `low-high/step`. Names and a step on a bare literal are not accepted, and neither is any value outside its field's own range, an inverted range, or a step outside 1..span. When day-of-month and day-of-week are BOTH restricted the trigger fires on a day either of them selects; when only one is restricted only that one constrains the day. A missed run is not made up: a host that could not fire a match — because it was unavailable, or because the previous invocation was still running — skips it rather than firing late, so a schedule never produces a backlog. At-least-once delivery applies to each match: a handler may be invoked more than once for one matched minute, and it must be idempotent. An uncaught exception in the handler is a failed invocation reported to host diagnostics; it is not retried within the matched minute and it never becomes an HTTP response. |
| `EdgeKVNamespace` | `takoform_edge_kv_namespace` | `identity` | `0.1.0` | Globally replicated key/value namespace of opaque BYTES with eventual consistency, exactly as fixed by the edge.kv Interface. Eventual consistency is the Form's semantics, not an option: a store with different convergence behavior is a different Form, and this one promises no read-your-writes, in any session, at any location. Values are byte strings carried in the family's encoded-bytes shape, so the declared byte limit and the structural string ceiling measure the same thing (decision 0020). |
| `ObjectBucket` | `takoform_edge_object_bucket` | `identity` | `0.1.0` | Flat-namespace object store with strong read-after-write consistency, streaming bodies, ranged and conditional reads, and multipart upload, exactly as fixed by the edge.objects Interface. An object body is a byte stream, never a JSON string: the contract's 5 GiB ceiling is only meaningful because bodies never travel inside an operation document (decision 0020). Operating rules such as CORS, lifecycle, and lock are separate policy resources, never desired fields of the bucket identity. |
| `SQLiteDatabase` | `takoform_sqlite_database` | `identity` | `0.1.0` | Embedded SQLite database with serializable transactions and TAGGED values, exactly as fixed by the edge.sql Interface. SQLite semantics are the identity: a database with different SQL, typing, or isolation behavior is a different Form, never an engine token. Values carry their storage class, so a 64-bit INTEGER and a BLOB round-trip losslessly instead of being flattened into a JSON scalar (decision 0020). |
| `AtLeastOnceQueue` | `takoform_at_least_once_queue` | `identity` | `0.1.0` | Message queue with at-least-once delivery and no ordering guarantee, exactly as fixed by the edge.queue Interface. There is no ordering field: a FIFO queue is a different Form. Message bodies are opaque bytes, a message identity is stable across redeliveries, and a queue has AT MOST ONE consumer — two would split the stream between two retry policies and two dead-letter destinations, which leaves the queue's own behavior unstatable (decision 0020). |
| `QueueConsumer` | `takoform_queue_consumer` | `attachment` | `0.1.0` | Attaches one Module Worker as the batch consumer of one At-Least-Once Queue, invoking its queue handler with message batches and redelivering messages that were not acknowledged. Consumption is inward activation and therefore an attachment, never a binding. One queue has at most one consumer, so a second attachment against the same queue is refused. A handler that returns normally without settling anything acknowledges the whole batch; one that throws retries every message it had not already acknowledged. maxRetries counts REDELIVERIES only — the first delivery does not count toward it — so a message is delivered at most 1 + maxRetries times, and a message that exhausts them moves to dead_letter_queue when one is declared and is dropped otherwise. The dead-letter copy is a new message there: new identity, new acceptance timestamp, and an attempt count starting again at 1 (decision 0020). Because that transfer resets the attempt count, dead_letter_queue MUST NOT lead back: a destination resolving to the queue this consumer drains, or closing a cycle of any length through the dead-letter graph, is refused before any mutation, because an exhausted message would circulate forever instead of coming to rest (decision 0026). |

The provider exposes exactly these typed resources on the v1alpha3 lane, and no
generic carrier for a Form it was not built against: nothing in the lane lets a
client verify a FormRef it did not compile in, so a carrier would offer reach
with no verification behind it (spec/decisions/0021). Family membership grants
no maturity: these members are tracked in the family candidate set, a lifecycle
record begins only at an Experimental transition, and hosts state their
supported subset in their Host Support Profiles.
