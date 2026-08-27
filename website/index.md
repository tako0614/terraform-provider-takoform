---
layout: home

hero:
  name: Takoform
  text: One provider. Dependent on none.
  tagline: Portable, host-neutral resource contracts for Terraform and OpenTofu.
  actions:
    - theme: brand
      text: See the current stack
      link: /docs/
    - theme: alt
      text: Read the spec
      link: /spec/
---

## API/Core 1.x / sealed Specification receipt

Takoform is an Experimental specification project. The first public API/Core
release identity is **`v1.0.0`**, using the existing
`forms.takoform.com/v1` wire and discovery lane. Release numbers are
human-readable compatibility checkpoints: future compatible `v1.1.0`, `v1.2.0`,
and later `v1.y.0` releases remain on `/v1`.

The historical Specification 1.1 is a sealed exact source receipt, not API
release 1.1 or 1.1.0; it does not create `/v1.1` and is not an ongoing
Specification version stream. Form Package publication remains separate and
unpublished, and Provider release remains an independent client artifact.

| Identity | Current identity | Meaning and availability |
| -------- | ---------------- | ------------------------ |
| API/Core release SemVer | **`v1.0.0`** | First public release identity; human-readable checkpoint on the `forms.takoform.com/v1` wire/discovery lane. Compatible `1.y.0` checkpoints remain on `/v1`. |
| Form `definitionVersion` | **8 versionless families / 31 Forms** | Exact current `0.x` FormRefs; every Form remains Experimental and advances independently. |
| Host API wire/discovery lane | **`forms.takoform.com/v1`** | Protocol path used by API/Core `1.x` checkpoints; not a third domain axis. |
| Historical Specification receipt | **1.1** | Sealed exact source receipt; not API release `1.1` or `1.1.0`, no `/v1.1`, and no ongoing Specification stream. |
| Form Package envelope | `packages.forms.takoform.com/v1alpha5` | Separate package/distribution schema identifier; package artifacts remain unpublished. |
| Provider | **3.0.0, Registry-published** | Independent non-normative reference implementation for all 31 current Forms; Provider 2.1.1 is retained history. |

API/Core release, Form maturity, Form Package publication, and Provider release
are independent. The sealed Specification receipt does not publish or promote
the API/Core lane, promote current Forms to `1.0.0`, mint `/v1.1` or v2 lanes, or
publish a package; Provider 3 cannot block it. Provider 2.1.1 remains immutable
Registry history for the historical identities it shipped.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = ">= 3.0.0"
    }
  }
}
```

## Edge reference family (16 of 31 current Experimental Forms)

Host API v1 carries the versionless current Form families
([Form Families](/spec/form-families.html)). The Edge family's 16 exact
`0.x` Forms are Experimental. A worker becomes reachable
through a chain of immutable resources: identity, module bytes, an exported
handler version, a traffic deployment, and an attachment that receives the
host-assigned address.

| Resource                                                                                     | Role       | What it declares                                                               |
| -------------------------------------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------ |
| [`takoform_module_worker`](/docs/resources/module_worker.html)                               | identity   | a JavaScript module-worker application identity                                |
| [`takoform_worker_bundle`](/docs/resources/worker_bundle.html)                               | revision   | an immutable uploaded code bundle (main module + modules)                      |
| [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html)                   | revision   | an immutable uploaded static-file inventory                                    |
| [`takoform_worker_version`](/docs/resources/worker_version.html)                             | revision   | an immutable snapshot: bundle, handlers, vars, sensitive slots, typed bindings |
| [`takoform_worker_deployment`](/docs/resources/worker_deployment.html)                       | deployment | which versions serve traffic, in basis points                                  |
| [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html)                 | attachment | a hostname whose origin is the worker                                          |
| [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html)                           | attachment | reachability over HTTPS at an address the host assigns                         |
| [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)                   | attachment | a UTC cron invoking the scheduled handler                                      |
| [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html)                       | identity   | an eventually consistent edge KV namespace                                     |
| [`takoform_sqlite_database`](/docs/resources/sqlite_database.html)                           | identity   | a SQLite-semantics serverless database                                         |
| [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)                 | revision   | an immutable ordered SQL migration set                                         |
| [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html) | attachment | checksum-safe suffix application to one database                               |
| [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)                   | identity   | at-least-once delivery with acknowledgement and retry                          |
| [`takoform_queue_consumer`](/docs/resources/queue_consumer.html)                             | attachment | batch, retry, and dead-letter policy targeting one worker                      |
| [`takoform_durable_workflow`](/docs/resources/durable_workflow.html)                         | identity   | durable multi-step execution as a class the worker's deployment serves         |
| [`takoform_actor_namespace`](/docs/resources/actor_namespace.html)                           | identity   | addressable actors with one live context, private storage, and one alarm       |

The Registry-published Provider 3 also maps the other 15 current Forms:

- Function: [`takoform_function`](/docs/resources/function.html), [`takoform_function_version`](/docs/resources/function_version.html), [`takoform_function_deployment`](/docs/resources/function_deployment.html), [`takoform_function_endpoint`](/docs/resources/function_endpoint.html)
- Container: [`takoform_serverless_container_service`](/docs/resources/serverless_container_service.html), [`takoform_container_revision`](/docs/resources/container_revision.html), [`takoform_container_traffic`](/docs/resources/container_traffic.html), [`takoform_container_endpoint`](/docs/resources/container_endpoint.html), [`takoform_container_custom_domain`](/docs/resources/container_custom_domain.html)
- Table and queue: [`takoform_table`](/docs/resources/table.html), [`takoform_pull_queue`](/docs/resources/pull_queue.html)
- Topic: [`takoform_topic`](/docs/resources/topic.html), [`takoform_topic_subscription`](/docs/resources/topic_subscription.html)
- Schedule and vector: [`takoform_message_schedule`](/docs/resources/message_schedule.html), [`takoform_dense_vector_index`](/docs/resources/dense_vector_index.html)

::: warning Provider distribution boundary
These resource names are the independent Provider 3 implementation surface.
Provider 3.0.0 is Registry-published, but the names are not normative and the
31 current Form Packages remain unpublished. The
[release-evidence policy](/spec/publication-freeze.html) keeps Specification
authority separate.
:::

Workers use capability through typed bindings backed by exact
[Interface contracts](/spec/interface-contract/) and
[Binding contracts](/spec/binding-contract/). Inward activation is always a
separate attachment resource
([decision 0021](/spec/decisions/0021-third-party-forms-and-contract-distribution.html)).

## Shape-preserving Forms

Takoform does not reduce cloud services to a least common denominator. Each
Form fixes the complete application-visible shape of one proven service
primitive — execution ABI, consistency, delivery guarantees, update units —
and leaves only the vendor's identity, account, placement, and commerce to the
host. Implementations with different semantics are different Forms; what is
exchangeable is the host, never the meaning
([decision 0008](/spec/decisions/0008-forms-preserve-service-shape.html)).

## Withdrawn epochs and published history

Earlier epochs preceded the current stack and were withdrawn or retained as
immutable history
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html)).
Published provider releases that carried them remain immutable Registry
history: **Provider 2.0.0** (the `forms.takoform.com/v1alpha2` compatibility
client) and **Provider 1.0.3** (the `forms.takoform.com/v1alpha1` Legacy
client) stay installable under exact pins for existing state, recovery, and
migration, but their resources have no successors and this site no longer
documents them. Existing users of withdrawn resources follow the
[v2 to v3 migration boundary](/release/migrations/v2-to-v3.html).

## How it works

1. **Declare** — write the portable fields of the exact service shape you
   need: bundle, handlers, vars, sensitive slots, typed bindings, retention.
2. **A host implements** — the provider discovers the host at a versioned path
   and drives validate/prepare/apply, observe, and delete with UID, generation,
   and revision fences. Implementation, placement, capacity, credentials, and
   routing stay with the host.
3. **Bind** — revisions hold typed capability bindings; attachments route the
   outside world in. Hosts publish what they support in Host Support Profiles.

<StatusNote />
