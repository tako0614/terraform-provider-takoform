---
layout: home

hero:
  name: Takoform
  text: Portable contracts. Many providers.
  tagline: Typed Terraform/OpenTofu tooling for host-neutral resource contracts.
  actions:
    - theme: brand
      text: Read the current boundary
      link: /docs/
    - theme: alt
      text: Browse historical source
      link: /spec/
---

## Stable Host API v1 / independent version axes

The Takoform Host API at `forms.takoform.com/v1` is the stable wire contract
for discovery, exact Form availability, lifecycle operations, fencing, and
errors. Numbered Specification 1.0 and 1.1 documents are immutable historical
receipts; they are not a current version stream and do not create an API v1.1,
new Form maturity, or a Provider release.

The current design keeps each identity on its own axis so a Provider release,
Host protocol, Form definition, and package envelope cannot be mistaken for
one another.

| Axis                  | Current identity                       | Meaning and availability                                                                                                      |
| --------------------- | -------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Host API              | **`forms.takoform.com/v1`**            | Stable protocol for discovery, exact Form availability, operations, fencing, and errors.                                     |
| Form corpus           | **8 families / 31 Forms**              | Exact current `0.x` FormRefs; every Form remains Experimental and independently versioned.                                   |
| Form Package envelope | `packages.forms.takoform.com/v1alpha5` | Distribution envelope for data-only Form packages; publication is separate from Host API and Provider release.               |
| Provider              | **3.0.0, Registry-published**          | Software tooling with typed mappings for official Forms; Provider SemVer is independent of Form and Host API identities.     |

The canonical Registry address `registry.terraform.io/tako0614/takoform`
publishes the official Takoform Provider mappings for official Forms only. Form
packages may also be distributed by independent third parties under their own
namespaces through the same package and verification path. A Provider must be
built against a Form before it can expose that Form; no configuration switch
turns it into a generic carrier.

A Terraform/OpenTofu module may combine multiple Takoform Providers with
industry-standard providers. The Takoform Provider is software tooling, not a
universal infrastructure provider:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = ">= 3.0.0"
    }
    aws = {
      source = "hashicorp/aws"
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

The official Registry Provider 3 also maps the other 15 current Forms:

- Function: [`takoform_function`](/docs/resources/function.html), [`takoform_function_version`](/docs/resources/function_version.html), [`takoform_function_deployment`](/docs/resources/function_deployment.html), [`takoform_function_endpoint`](/docs/resources/function_endpoint.html)
- Container: [`takoform_serverless_container_service`](/docs/resources/serverless_container_service.html), [`takoform_container_revision`](/docs/resources/container_revision.html), [`takoform_container_traffic`](/docs/resources/container_traffic.html), [`takoform_container_endpoint`](/docs/resources/container_endpoint.html), [`takoform_container_custom_domain`](/docs/resources/container_custom_domain.html)
- Table and queue: [`takoform_table`](/docs/resources/table.html), [`takoform_pull_queue`](/docs/resources/pull_queue.html)
- Topic: [`takoform_topic`](/docs/resources/topic.html), [`takoform_topic_subscription`](/docs/resources/topic_subscription.html)
- Schedule and vector: [`takoform_message_schedule`](/docs/resources/message_schedule.html), [`takoform_dense_vector_index`](/docs/resources/dense_vector_index.html)

::: warning Provider distribution boundary
These resource names are the Provider 3 implementation surface. Provider
3.0.0 is Registry-published for the official Form set, but the names are not
normative and Provider publication does not publish a Host, Form Package, or
third-party Form. The [historical release records](/release/) retain the
numbered Specification receipts and earlier Provider identities.
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
