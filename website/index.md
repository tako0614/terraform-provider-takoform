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

## What is published and what is a release candidate

This project ships in two tiers. Every page says which tier it is describing,
and nothing here mixes them.

| Tier | What it covers | How you get it |
| --- | --- | --- |
| **Current published** | provider `v2.0.0` and the retained nine `forms.takoform.com/v1alpha2` resources | `terraform init` installs it from the Registry |
| **Beta release candidate** | stable provider target `v2.1.0`, the Beta Host API, and 15 Experimental `edge.forms.takoform.com/v1beta1` Forms | build from source until owner publication |

The v2.1.0 descriptor remains `candidate-only` until the release owner
publishes it. Its exact 15 Beta FormRefs/digests are already immutable provider
compatibility data; their package artifacts remain unpublished. Open
[release-policy obligations](/spec/publication-freeze.html) stay in force for
package/public-service publication and later Stable/GA qualification. The
[machine-readable status](/.well-known/takoform-site.json) distinguishes the
published provider from the release target.

## What it looks like

This is the **current published** tier: provider `v2.0.0` from the Registry,
and the retained v2 resources it exposes.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}

resource "takoform_key_value_store" "cache" {
  name                = "cache"
  consistency         = "eventual"
  default_ttl_seconds = 3600
}

resource "takoform_queue" "jobs" {
  name                      = "jobs"
  message_retention_seconds = 345600
  ordering                  = "best_effort"
}

resource "takoform_edge_worker" "api" {
  name                = "api"
  artifact_media_type = "application/vnd.takoform.edge-worker+tar"
  artifact_sha256     = "sha256:0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37"
  artifact_url        = "https://artifacts.example.com/api.tar"
  entrypoint          = "worker.mjs"
  runtime             = "javascript"
  runtime_version     = "2026.1"
  configuration       = { "LOG_LEVEL" = "info" }

  connections = [
    {
      name        = "data"
      resource    = "KeyValueStore/cache"
      permissions = ["read", "write"]
      projection  = "keyvalue.binding.v1"
    },
  ]
}
```

No credentials, no placement, no pricing — the host decides those and keeps
them out of your state. `v1.0.3` remains the published Legacy client for
existing v1 state. [Start here](/docs/) to install and use it.

## Shape-preserving Forms

Takoform does not reduce cloud services to a least common denominator. Each
Form fixes the complete application-visible shape of one proven service
primitive — execution ABI, consistency, delivery guarantees, update units —
and leaves only the vendor's identity, account, placement, and commerce to
the host. Implementations with different semantics are different Forms; what
is exchangeable is the host, never the meaning
([decision 0008](/spec/decisions/0008-forms-preserve-service-shape.html)).

## Current published: the retained v2 resources

The nine `forms.takoform.com/v1alpha2` `0.1.0` candidates are the surface of
the published provider `v2.0.0`, retained for compatibility
([decision 0035](/spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.html)):

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

## Beta: the Experimental Edge Platform Family

::: warning Provider release candidate
Everything in this section rides the stable provider `v2.1.0` release target.
Its descriptor remains `candidate-only` until owner publication. The 15 exact
Forms are Experimental; Beta is their API/family channel, not Stable maturity,
and their package artifacts remain unpublished. Runnable configuration lives
on the [Beta quick start](/docs/#beta-edge-platform-family), not on this page,
because a fragment of it cannot be applied.
:::

Current design work happens in the namespaced
`edge.forms.takoform.com/v1beta1` family
([Form Families](/spec/form-families.html)), served over the
`forms.takoform.com/v1beta1` Host API with UID/generation/revision identity,
long-running operations, and content-addressed artifact upload.

| Resource | Role | What it declares |
| --- | --- | --- |
| [`takoform_module_worker`](/docs/resources/module_worker.html) | identity | a JavaScript module-worker application identity |
| [`takoform_worker_bundle`](/docs/resources/worker_bundle.html) | revision | an immutable uploaded code bundle (main module + modules) |
| [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html) | revision | an immutable uploaded static-file inventory |
| [`takoform_worker_version`](/docs/resources/worker_version.html) | revision | an immutable snapshot: bundle, handlers, vars, sensitive slots, typed bindings |
| [`takoform_worker_deployment`](/docs/resources/worker_deployment.html) | deployment | which versions serve traffic, in basis points |
| [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html) | attachment | a hostname whose origin is the worker |
| [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html) | attachment | reachability over HTTPS at an address the host assigns |
| [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html) | attachment | a UTC cron invoking the scheduled handler |
| [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html) | identity | an eventually consistent edge KV namespace |
| [`takoform_edge_object_bucket`](/docs/resources/edge_object_bucket.html) | identity | a strongly consistent object bucket |
| [`takoform_sqlite_database`](/docs/resources/sqlite_database.html) | identity | a SQLite-semantics serverless database |
| [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html) | revision | an immutable ordered SQL migration set |
| [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html) | attachment | checksum-safe suffix application to one database |
| [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html) | identity | at-least-once delivery with acknowledgement and retry |
| [`takoform_queue_consumer`](/docs/resources/queue_consumer.html) | attachment | batch, retry, and dead-letter policy targeting one worker |

Workers use capability through typed bindings (`kv_bindings`,
`bucket_bindings`, `sqlite_bindings`, `queue_producer_bindings`,
`service_bindings`) backed by exact
[Interface contracts](/spec/interface-contract/) and
[Binding contracts](/spec/binding-contract/); inward activation (routes,
domains, cron, consumption) is always a separate attachment resource. These
typed resources are the whole lane: there is no generic carrier for a Form the
provider was not built against, because the lane gives a client no way to
verify a FormRef it did not compile in
([decision 0021](/spec/decisions/0021-third-party-forms-and-contract-distribution.html)).

## How it works

1. **Declare** — write the portable fields of the exact service shape you
   need: bundle, handlers, vars, sensitive slots, typed bindings, retention.
2. **A host implements** — the provider discovers the host at a versioned
   path and drives validate/prepare/apply, observe, and delete with
   UID, generation, and revision fences. Implementation, placement,
   capacity, credentials, and routing stay with the host.
3. **Bind** — revisions hold typed capability bindings; attachments route
   the outside world in. Hosts publish what they support in Host Support
   Profiles.

<StatusNote />
