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
