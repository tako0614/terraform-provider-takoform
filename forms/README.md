# Portable Form inventory

This is the current provider-v1 portable Service Form inventory, generated
from the one declaration in `internal/formcatalog`. Every Form describes what
a caller wants. None of them names a target, a credential, a placement, a
price, or an implementation: those stay with the host that realizes the Form.

## Compute and application

| Kind | Resource | Version | Portable intent |
| --- | --- | --- | --- |
| `EdgeWorker` | `takoform_edge_worker` | `4.0.0` | Portable edge/event-driven application served from a prebuilt immutable artifact. |
| `ContainerService` | `takoform_container_service` | `3.0.0` | Portable OCI container service pinned to an immutable image digest. |
| `ComputeInstance` | `takoform_compute_instance` | `3.0.0` | Portable long-running machine instance built from digest-bound boot-image bytes. |
| `StaticSite` | `takoform_static_site` | `3.0.0` | Portable static asset site served from a prebuilt immutable artifact. |
| `Workflow` | `takoform_workflow` | `3.0.0` | Portable durable workflow definition and instance-state lifecycle. |
| `StatefulEntity` | `takoform_stateful_entity` | `4.0.0` | Portable namespace of addressable persistent entities implemented by digest-bound application bytes. |
| `Schedule` | `takoform_schedule` | `3.0.0` | Portable cron lifecycle that invokes exactly one connected Resource. |

## Data and storage

| Kind | Resource | Version | Portable intent |
| --- | --- | --- | --- |
| `ObjectBucket` | `takoform_object_bucket` | `3.0.0` | Portable object storage with a portable default storage class. |
| `ObjectLifecycleRule` | `takoform_object_lifecycle_rule` | `1.0.0` | One portable expiration or storage-transition action applied to a connected object store. |
| `KeyValueStore` | `takoform_key_value_store` | `2.0.0` | Portable key/value state with an optional consistency preference. |
| `CacheCluster` | `takoform_cache_cluster` | `1.0.0` | Portable in-memory cache sized by an open capability token. |
| `RelationalDatabase` | `takoform_relational_database` | `3.0.0` | Portable relational database addressed through an open engine capability token. |
| `IndexedStore` | `takoform_indexed_store` | `1.0.0` | Portable bounded key/value item store with declared queryable attributes and no query language. |
| `Queue` | `takoform_queue` | `3.0.0` | Portable asynchronous delivery with at-least-once semantics. |
| `StreamTopic` | `takoform_stream_topic` | `1.0.0` | Portable published event stream that many independent consumers can read. |
| `SearchIndex` | `takoform_search_index` | `1.0.0` | Portable full-text index over a declared set of document fields. |
| `VectorIndex` | `takoform_vector_index` | `3.0.0` | Portable vector index with dimensions fixed for the index lifecycle. |

## Analytics and inference

| Kind | Resource | Version | Portable intent |
| --- | --- | --- | --- |
| `AnalyticsDataset` | `takoform_analytics_dataset` | `1.0.0` | Portable append-oriented dataset queried for analysis rather than transactions. |
| `ModelEndpoint` | `takoform_model_endpoint` | `4.0.0` | Portable inference endpoint serving digest-bound model bytes for one declared task. |

## Network and delivery

| Kind | Resource | Version | Portable intent |
| --- | --- | --- | --- |
| `DnsZone` | `takoform_dns_zone` | `1.0.0` | Portable authoritative DNS zone for one domain. |
| `DnsRecord` | `takoform_dns_record` | `1.0.0` | Portable DNS record published into one connected zone. |
| `TlsCertificate` | `takoform_tls_certificate` | `1.0.0` | Portable managed TLS certificate for a fixed set of domains. Key material stays with the host. |
| `HttpRoute` | `takoform_http_route` | `1.0.0` | Portable hostname and path binding that sends HTTP traffic to one connected Resource. |
| `LoadBalancer` | `takoform_load_balancer` | `1.0.0` | Portable listener that distributes connections across connected backends. |
| `PrivateNetwork` | `takoform_private_network` | `1.0.0` | Portable private address space that other Resources can attach to. |

## Operations and integration

| Kind | Resource | Version | Portable intent |
| --- | --- | --- | --- |
| `ContainerRegistry` | `takoform_container_registry` | `1.0.0` | Portable OCI artifact registry namespace. |
| `LogSink` | `takoform_log_sink` | `1.0.0` | Portable destination that retains structured application logs. |
| `MetricSink` | `takoform_metric_sink` | `1.0.0` | Portable destination that retains numeric time series. |
| `EmailSender` | `takoform_email_sender` | `1.0.0` | Portable outbound mail identity for one verified domain. |
| `WebhookEndpoint` | `takoform_webhook_endpoint` | `1.0.0` | Portable inbound HTTP endpoint that forwards received requests to one connected Resource. |
| `IdentityClient` | `takoform_identity_client` | `1.0.0` | Portable OIDC relying-party registration. Issued client material stays with the host. |
| `FeatureFlag` | `takoform_feature_flag` | `1.0.0` | Portable named runtime switch expressed as one complete enabled percentage. |
| `RateLimitPolicy` | `takoform_rate_limit_policy` | `1.0.0` | Portable request budget applied to one connected Resource. |
| `BackupPolicy` | `takoform_backup_policy` | `1.0.0` | Portable scheduled copy and retention rule for one connected Resource. |

## Declared runtime interfaces

A Form may declare the runtime interfaces its service exposes. The names are
author-defined and open: there is no registry, no allowlist, and no central
approval. A declaration states what exists and how its non-secret values are
filled; the host creates the record, authorizes consumers, and owns its
lifecycle.

| Kind | Interface |
| --- | --- |
| `EdgeWorker` | `http.request@1` (request) |
| `ContainerService` | `http.request@1` (request) |
| `StaticSite` | `http.request@1` (request) |
| `Workflow` | `workflow.invoke@1` (cancel, invoke, status) |
| `StatefulEntity` | `entity.invoke@1` (invoke) |
| `ObjectBucket` | `object.storage@1` (delete, get, list, put) |
| `KeyValueStore` | `keyvalue.store@1` (delete, get, list, put) |
| `CacheCluster` | `cache.store@1` (delete, get, put) |
| `RelationalDatabase` | `sql.query@1` (execute, query, transaction) |
| `IndexedStore` | `data.indexed@1` (delete, get, put, query) |
| `Queue` | `queue.messages@1` (acknowledge, receive, send) |
| `StreamTopic` | `stream.publish@1` (publish, subscribe) |
| `SearchIndex` | `search.query@1` (delete, index, query) |
| `VectorIndex` | `vector.query@1` (delete, query, upsert) |
| `AnalyticsDataset` | `analytics.query@1` (append, query) |
| `ModelEndpoint` | `model.invoke@1` (invoke) |
| `DnsZone` | `dns.zone@1` (list, resolve) |
| `TlsCertificate` | `tls.certificate@1` (status) |
| `LoadBalancer` | `network.endpoint@1` (status) |
| `PrivateNetwork` | `network.attach@1` (attach, detach) |
| `ContainerRegistry` | `registry.images@1` (list, pull, push) |
| `LogSink` | `log.ingest@1` (query, write) |
| `MetricSink` | `metric.ingest@1` (query, write) |
| `EmailSender` | `email.send@1` (send, status) |
| `WebhookEndpoint` | `http.request@1` (request) |
| `IdentityClient` | `identity.oidc@1` (metadata) |
| `FeatureFlag` | `flag.evaluate@1` (evaluate) |

## Immutable fields

Every Form fixes its `/name`. A Form that additionally fixes a field states so
in its definition, and the provider enforces replacement for exactly those
fields; the protocol lifecycle proves both.

| Kind | Immutable |
| --- | --- |
| `EdgeWorker` | `/name` |
| `ContainerService` | `/name` |
| `ComputeInstance` | `/name` |
| `StaticSite` | `/name` |
| `Workflow` | `/name` |
| `StatefulEntity` | `/name` |
| `Schedule` | `/name` |
| `ObjectBucket` | `/name` |
| `ObjectLifecycleRule` | `/name` |
| `KeyValueStore` | `/name` |
| `CacheCluster` | `/name` |
| `RelationalDatabase` | `/databaseName`, `/engine`, `/name` |
| `IndexedStore` | `/name`, `/partitionKey`, `/sortKey` |
| `Queue` | `/name` |
| `StreamTopic` | `/name`, `/partitions` |
| `SearchIndex` | `/name` |
| `VectorIndex` | `/dimensions`, `/name` |
| `AnalyticsDataset` | `/name`, `/partitionField` |
| `ModelEndpoint` | `/name` |
| `DnsZone` | `/domain`, `/name` |
| `DnsRecord` | `/name`, `/recordType` |
| `TlsCertificate` | `/domains`, `/name` |
| `HttpRoute` | `/name` |
| `LoadBalancer` | `/name` |
| `PrivateNetwork` | `/addressSpace`, `/name` |
| `ContainerRegistry` | `/name` |
| `LogSink` | `/name` |
| `MetricSink` | `/name` |
| `EmailSender` | `/domain`, `/name` |
| `WebhookEndpoint` | `/name` |
| `IdentityClient` | `/name` |
| `FeatureFlag` | `/flagKey`, `/name` |
| `RateLimitPolicy` | `/name` |
| `BackupPolicy` | `/name` |

## Status

This inventory is `structural-candidate`: the packages verify locally, the
provider derives the same schema from the same declaration, and the protocol
lifecycle runs against an in-process host. None of that admits a Form. Signed
release bytes, a conforming host's signed lifecycle report, Registry
installation and readback, and signed admission evidence are external
requirements, tracked in [`standard-package-set.json`](standard-package-set.json).

The previously published generation is retired, not erased: its immutable bytes
and admission evidence stay verifiable through
[`retired-package-set.json`](retired-package-set.json). Those releases are never
rewritten, re-signed, or reshaped.
