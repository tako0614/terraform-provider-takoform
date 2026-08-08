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
| `ModuleWorker` | `takoform_module_worker` | `identity` | `0.1.0` | Long-lived logical identity of one ES Module Worker application. The Form fixes the ES Module Worker ABI by identity, and states it exactly: the runtime contract worker.runtime@1.0.0 in this Form's providedInterfaces fixes the module's default-export shape, the fetch, scheduled, queue, and tail handler signatures, the binding environment, ctx.waitUntil, exception handling, body streaming, the minimum Web API surface, and module loading. A host supporting this Form implements that exact digest; a runtime that behaves differently is a different contract version and a different Form version, never a compatibility date. Code, configuration, and bindings live on Worker Version revisions; traffic selection lives on Worker Deployments. |
| `WorkerBundle` | `takoform_worker_bundle` | `revision` | `0.1.0` | Immutable content-addressed module bundle of one worker build, named by the digest of the artifact manifest committed through the content-addressed upload API (decision 0012). The manifest, not this Form, describes the main module and every additional module with its closed media type, exact size, and sha256 digest, so the bundle keeps exactly one source of truth for its bytes. Different bytes commit a different manifest, which is a different bundle. |
| `WorkerVersion` | `takoform_worker_version` | `revision` | `0.1.0` | Immutable executable snapshot of one Module Worker: a bundle, the handlers its module exports, non-secret vars, and the typed capability bindings the code may use. A change is a new Worker Version; traffic moves only through Worker Deployments. The runtime this code runs on is not a field of this Form: it is fixed by the worker.runtime@1.0.0 contract the Module Worker identity provides, so a version carries no compatibility date and no compatibility flag (decision 0019). |
| `WorkerDeployment` | `takoform_worker_deployment` | `deployment` | `0.1.0` | Selects which Worker Versions of one Module Worker serve traffic and in what proportion. Weights are basis points and must sum to exactly 10000 across entries; the sum is host-validated semantics because a schema cannot add weights. Rollback is re-weighting, never mutating a revision. |
| `WorkerCustomDomain` | `takoform_worker_custom_domain` | `attachment` | `0.1.0` | Attaches one DNS hostname to a Module Worker so its active deployment serves that hostname over HTTPS. Inward activation is an attachment, never a binding; deleting the attachment detaches the hostname and never deletes the worker. |
| `WorkerCronTrigger` | `takoform_worker_cron_trigger` | `attachment` | `0.1.0` | Attaches one cron schedule to a Module Worker, invoking its scheduled handler at each match. Schedules are interpreted in UTC only; there is no timezone field, so two hosts can never fire the same trigger at different instants. The accepted grammar is exactly five single-value fields separated by single spaces: minute is a literal 0-59 and hour a literal 0-23, day-of-month is `*` or 1-31, month is `*` or 1-12, and day-of-week is `*` or 0-6. Ranges, lists, steps such as `*/5`, names, and `*` in the minute or hour field are all rejected, so the most frequent representable schedule is once per day at one fixed UTC time. Hourly and sub-hourly schedules are not expressible and need a future grammar revision, which is a new definition version of this Form. |
| `EdgeKVNamespace` | `takoform_edge_kv_namespace` | `identity` | `0.1.0` | Globally replicated key/value namespace with eventual consistency, exactly as fixed by the edge.kv Interface. Eventual consistency is the Form's semantics, not an option: a store with different convergence behavior is a different Form. |
| `ObjectBucket` | `takoform_edge_object_bucket` | `identity` | `0.1.0` | Flat-namespace object store with read-after-write consistency, exactly as fixed by the edge.objects Interface. Operating rules such as CORS, lifecycle, and lock are separate policy resources, never desired fields of the bucket identity. |
| `SQLiteDatabase` | `takoform_sqlite_database` | `identity` | `0.1.0` | Embedded SQLite database with serializable transactions, exactly as fixed by the edge.sql Interface. SQLite semantics are the identity: a database with different SQL, typing, or isolation behavior is a different Form, never an engine token. |
| `AtLeastOnceQueue` | `takoform_at_least_once_queue` | `identity` | `0.1.0` | Message queue with at-least-once delivery and no ordering guarantee, exactly as fixed by the edge.queue Interface. There is no ordering field: a FIFO queue is a different Form. |
| `QueueConsumer` | `takoform_queue_consumer` | `attachment` | `0.1.0` | Attaches one Module Worker as the batch consumer of one At-Least-Once Queue, invoking its queue handler with message batches and redelivering failed batches. Consumption is inward activation and therefore an attachment, never a binding. |

A generic `takoform_resource` carries any third-party v1alpha3 Form by exact
FormRef. Family membership grants no maturity: these members are tracked in
the family candidate set, a lifecycle record begins only at an Experimental
transition, and hosts state their supported subset in their Host Support
Profiles.
