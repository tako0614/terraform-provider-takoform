# Documentation

This page starts with the first public API/Core release identity, `v1.0.0`,
and its existing `/v1` wire/discovery lane, plus the exact 8-family, 31-Form
current corpus. Compatible API/Core `1.x` checkpoints stay on `/v1`. The
historical Specification 1.1 is a sealed source receipt, not API release 1.1
or 1.1.0; it does not create `/v1.1` or an ongoing Specification stream.
Provider and historical lanes remain separate so an implementation release
cannot become API/Core authority by implication.

## API/Core 1.x / sealed Specification receipt

| Identity | Current identity | Meaning and availability |
| -------- | ---------------- | ------------------------ |
| API/Core release SemVer | **`v1.0.0`** | First public release identity; human-readable checkpoint on the `forms.takoform.com/v1` wire/discovery lane. Compatible `1.y.0` checkpoints remain on `/v1`. |
| Form `definitionVersion` | **8 versionless families / 31 Forms** | Exact current `0.x` FormRefs; every Form remains Experimental and advances independently. |
| Host API wire/discovery lane | **`forms.takoform.com/v1`** | Protocol path used by API/Core `1.x` checkpoints; not a third domain axis. |
| Historical Specification receipt | **1.1** | Sealed exact source receipt; not API release `1.1` or `1.1.0`, no `/v1.1`, and no ongoing Specification stream. |
| Form Package envelope | `packages.forms.takoform.com/v1alpha5` | Separate package/distribution schema identifier; package artifacts remain unpublished. |
| Provider | **3.0.0, Registry-published** | Independent non-normative reference implementation; it cannot block API/Core or alter the sealed Specification receipt. Provider 2.1.1 is retained history. |

The [release-evidence policy](/spec/publication-freeze.html) keeps those two
domain axes machine-checkable. A sealed Specification receipt does not publish
or promote the API/Core lane, relabel any current Form as `1.0.0`, mint a
`/v1.1` or v2 lane, or publish a package; a stable Form identity requires an
explicit per-Form decision.

## Edge reference family (16 of 31 current Experimental Forms) {#beta-edge-platform-family}

The versionless Edge family targets Host API v1, discovered at
`/.well-known/takoform/v1`, with UID/generation/revision identity,
long-running operations, and content-addressed artifact upload.

A worker becomes reachable through a chain, not a single resource: an identity,
an immutable bundle of module bytes, an immutable version that names the
handlers those bytes export, a deployment that sends traffic to it, and an
attachment that gives it an address. An endpoint whose worker has no active
deployment never becomes Ready, so the whole chain is one configuration:

This shape uses the independent Registry-published Provider 3 reference
implementation. It is non-normative and does not claim Specification readiness
or Form Package publication.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = ">= 3.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}

resource "takoform_worker_bundle" "api" {
  name        = "api-bundle"
  main_module = "worker.mjs"

  modules = [
    {
      name         = "worker.mjs"
      content_type = "application/javascript+module"
      content_file = "${path.module}/dist/worker.mjs"
    },
  ]
}

resource "takoform_worker_version" "api" {
  name      = "api-v1"
  worker    = takoform_module_worker.api.name
  bundle    = takoform_worker_bundle.api.name
  handlers  = ["fetch"]
  vars_json = jsonencode({ "LOG_LEVEL" = "info" })
}

resource "takoform_worker_deployment" "api" {
  name   = "api"
  worker = takoform_module_worker.api.name

  versions = [
    {
      worker_version = takoform_worker_version.api.name
      weight         = 10000
    },
  ]
}

resource "takoform_worker_endpoint" "api" {
  name   = "api"
  worker = takoform_module_worker.api.name
}
```

Each resource's own page carries the same source-candidate pin and boundary.
Capability is added to a version through typed bindings; inward activation — a
custom domain, a cron trigger, or a queue consumer — is always a separate
attachment resource.

## Current Provider 3 resource reference {#resource-reference}

The independent Registry-published Provider 3 maps all 31 current Experimental
`0.x` Forms. These names are non-normative Provider metadata and do not change
Form maturity.

### Edge family

- [module_worker](/docs/resources/module_worker.html)
- [worker_bundle](/docs/resources/worker_bundle.html)
- [static_asset_bundle](/docs/resources/static_asset_bundle.html)
- [worker_version](/docs/resources/worker_version.html)
- [worker_deployment](/docs/resources/worker_deployment.html)
- [worker_custom_domain](/docs/resources/worker_custom_domain.html)
- [worker_endpoint](/docs/resources/worker_endpoint.html)
- [worker_cron_trigger](/docs/resources/worker_cron_trigger.html)
- [edge_kv_namespace](/docs/resources/edge_kv_namespace.html)
- [sqlite_database](/docs/resources/sqlite_database.html)
- [sqlite_migration_set](/docs/resources/sqlite_migration_set.html)
- [sqlite_migration_application](/docs/resources/sqlite_migration_application.html)
- [at_least_once_queue](/docs/resources/at_least_once_queue.html)
- [queue_consumer](/docs/resources/queue_consumer.html)
- [durable_workflow](/docs/resources/durable_workflow.html)
- [actor_namespace](/docs/resources/actor_namespace.html)

### Function family

- [function](/docs/resources/function.html)
- [function_version](/docs/resources/function_version.html)
- [function_deployment](/docs/resources/function_deployment.html)
- [function_endpoint](/docs/resources/function_endpoint.html)

### Container family

- [serverless_container_service](/docs/resources/serverless_container_service.html)
- [container_revision](/docs/resources/container_revision.html)
- [container_traffic](/docs/resources/container_traffic.html)
- [container_endpoint](/docs/resources/container_endpoint.html)
- [container_custom_domain](/docs/resources/container_custom_domain.html)

### Table, queue, topic, schedule, and vector families

- [table](/docs/resources/table.html)
- [pull_queue](/docs/resources/pull_queue.html)
- [topic](/docs/resources/topic.html)
- [topic_subscription](/docs/resources/topic_subscription.html)
- [message_schedule](/docs/resources/message_schedule.html)
- [dense_vector_index](/docs/resources/dense_vector_index.html)

There is no generic carrier for a Form the provider was not built against: the
typed surface gives a client a way to verify only the exact FormRefs it
compiled in ([decision 0021](/spec/decisions/0021-third-party-forms-and-contract-distribution.html)).

## Withdrawn epochs {#lanes}

The two pre-Beta epochs (`forms.takoform.com/v1alpha1` Legacy and
`forms.takoform.com/v1alpha2`) were withdrawn
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html)).
The provider releases that carried them, **Provider 2.0.0** and
**Provider 1.0.3**, remain immutable Registry history under exact pins, but
their resources have no successors and this site no longer documents them.
Existing state follows the
[v2 to v3 migration boundary](/release/migrations/v2-to-v3.html).

## More project surfaces

- [Form Proposals](/proposals/) — design material for Forms that have not earned a public FormRef
- [Form inventory](/forms/) — the current 31 Forms and retained compatibility identities
- [Conformance evidence](/conformance/) — how compatibility is proven
- [Release](/release/) — provider publication boundary, Form Packages, and migrations
- [Glossary](/docs/glossary.html) — terms used across this documentation

## Host boundary

Takoform owns workload semantics, schemas, exact identities, packages, and
conformance. Hosts own capability support, placement, routing, scaling,
credentials, recovery, and any managed service's live catalog, billing, quota,
and SLA.

<StatusNote />
