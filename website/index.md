---
layout: home

hero:
  name: Takoform
  text: One provider. Dependent on none.
  tagline: Portable, host-neutral resource contracts for Terraform and OpenTofu.
  actions:
    - theme: brand
      text: Get started
      link: /docs/
    - theme: alt
      text: Read the spec
      link: /spec/
---

## What it looks like

```hcl
provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}

resource "takoform_edge_kv_namespace" "cache" {
  name = "cache"
}

resource "takoform_at_least_once_queue" "jobs" {
  name                      = "jobs"
  message_retention_seconds = 345600
}

resource "takoform_worker_custom_domain" "api" {
  name     = "api-domain"
  worker   = takoform_module_worker.api.name
  hostname = "api.example.com"
}

resource "takoform_worker_cron_trigger" "cleanup" {
  name   = "cleanup"
  worker = takoform_module_worker.api.name
  cron   = "0 6 * * *"
}
```

No credentials, no placement, no pricing — the host decides those and keeps
them out of your state. The Edge Platform Family resources above require
provider `v2.1.0`, an unpublished source candidate built from source.
Provider `v2.0.0` is the current published client and carries the retained
v2 resources; `v1.0.3` remains the published Legacy client.
[Start here](/docs/) to install and use it.

## Shape-preserving Forms

Takoform does not reduce cloud services to a least common denominator. Each
Form fixes the complete application-visible shape of one proven service
primitive — execution ABI, consistency, delivery guarantees, update units —
and leaves only the vendor's identity, account, placement, and commerce to
the host. Implementations with different semantics are different Forms; what
is exchangeable is the host, never the meaning
([decision 0008](/spec/decisions/0008-forms-preserve-service-shape.html)).

## The Edge Platform Family (v1alpha3 lane)

The current design lane is the namespaced
`edge.forms.takoform.com/v1alpha1` family
([Form Families](/spec/form-families.html)) served over the
`forms.takoform.com/v1alpha3` Host API with UID/generation/revision identity,
long-running operations, and content-addressed artifact upload.

| Resource | Role | What it declares |
| --- | --- | --- |
| [`takoform_module_worker`](/docs/resources/module_worker.html) | identity | a JavaScript module-worker application identity |
| [`takoform_worker_bundle`](/docs/resources/worker_bundle.html) | revision | an immutable uploaded code bundle (main module + modules) |
| [`takoform_worker_version`](/docs/resources/worker_version.html) | revision | an immutable snapshot: bundle, compatibility date, handlers, typed bindings |
| [`takoform_worker_deployment`](/docs/resources/worker_deployment.html) | deployment | which versions serve traffic, in basis points |
| [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html) | attachment | a hostname whose origin is the worker |
| [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html) | attachment | a UTC cron invoking the scheduled handler |
| [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html) | identity | an eventually consistent edge KV namespace |
| [`takoform_edge_object_bucket`](/docs/resources/edge_object_bucket.html) | identity | a strongly consistent object bucket |
| [`takoform_sqlite_database`](/docs/resources/sqlite_database.html) | identity | a SQLite-semantics serverless database |
| [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html) | identity | at-least-once delivery with acknowledgement and retry |
| [`takoform_queue_consumer`](/docs/resources/queue_consumer.html) | attachment | batch, retry, and dead-letter policy targeting one worker |

Workers use capability through typed bindings (`kv_bindings`,
`bucket_bindings`, `sqlite_bindings`, `queue_producer_bindings`,
`service_bindings`) backed by exact
[Interface contracts](/spec/interface-contract/) and
[Binding contracts](/spec/binding-contract/); inward activation (routes,
domains, cron, consumption) is always a separate attachment resource. A
generic `takoform_resource` carries third-party family Forms by exact
FormRef.

## Retained v2 resources

The nine `forms.takoform.com/v1alpha2` `0.1.0` candidates remain the
published provider-v2 surface, retained for compatibility
([decision 0013](/spec/decisions/0013-v1alpha3-lane-ships-in-provider-v2-1.html)):

| Resource | What it declares |
| --- | --- |
| [`takoform_edge_worker`](/docs/resources/edge_worker.html) | a request/event application from a digest-bound artifact |
| [`takoform_relational_database`](/docs/resources/relational_database.html) | a relational database by open engine token |
| [`takoform_object_bucket`](/docs/resources/object_bucket.html) | object storage |
| [`takoform_key_value_store`](/docs/resources/key_value_store.html) | key/value state |
| [`takoform_queue`](/docs/resources/queue.html) | at-least-once message delivery |
| [`takoform_schedule`](/docs/resources/schedule.html) | a cron that invokes one connected resource |
| [`takoform_container_service`](/docs/resources/container_service.html) | a service from an OCI image digest |
| [`takoform_stateful_entity`](/docs/resources/stateful_entity.html) | addressable persistent entities |
| [`takoform_vector_index`](/docs/resources/vector_index.html) | a vector index with fixed dimensions |

## How it works

1. **Declare** — write the portable fields of the exact service shape you
   need: bundles, compatibility date, handlers, typed bindings, retention.
2. **A host implements** — the provider discovers the host at a versioned
   path and drives validate/prepare/apply, observe, refresh, and delete with
   UID, generation, and revision fences. Implementation, placement,
   capacity, credentials, and routing stay with the host.
3. **Bind** — revisions hold typed capability bindings; attachments route
   the outside world in. Hosts publish what they support in Host Support
   Profiles.

<StatusNote />
