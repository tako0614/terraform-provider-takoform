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

## Stable Host API v1 / independent version streams

The Takoform Host API at `forms.takoform.com/v1` is the stable wire contract
for discovery, exact Form availability, lifecycle operations, fencing, and
errors. Numbered Specification 1.0/1.1 receipts are archive evidence; they do
not create an API v1.1, change a Form's maturity, or release a Provider.

The current official Form corpus is one versionless family,
`edge.forms.takoform.com`, with 16 exact Experimental Forms. The authoritative
catalog is maintained by the provider-neutral
[`takoform-forms` source at commit `3a395e4`](https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e).

Takoform has four independent version streams: Host API, each Form
definition, Core/library software, and Provider software. A Form Package API
identifier is a wire/envelope format, not a fifth release stream.

| Stream | Current identity | Meaning and availability |
| --- | --- | --- |
| Host API | **`forms.takoform.com/v1`** | Stable protocol for discovery, exact Form availability, operations, fencing, and errors. |
| Form | Each Form's **`definitionVersion`** | Exact independently versioned contract; the official corpus has 16 Experimental Forms in one family. |
| Core/library | Independent software SemVer | SDK, verifier, compiler, and CLI releases; they do not version the Host API or a Form. |
| Provider | **3.0.0, Registry-published** | Typed software tooling for explicitly supported official Forms; Provider SemVer is independent of Form and Host API identities. |

The canonical Registry address
`registry.terraform.io/tako0614/takoform` publishes official-Form mappings
only. Third-party Form packages use the same Host API path and package /
verification contracts under their own namespaces. A Provider must be built
against every Form it exposes; no configuration switch turns it into a generic
carrier or a universal infrastructure provider.

Provider `3.0.0` remains an immutable Registry release whose historical
projection contains the earlier 8-family/31-resource aggregate. That release
record is not the current Form corpus. The official-only 16-Form Provider
direction is not yet published, so this page makes no install claim for a
future Provider version.

A Terraform/OpenTofu module may combine the official Takoform Provider with
other Takoform or industry-standard providers:

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

## Current Edge reference family (16 Experimental Forms)

Host API v1 carries versionless Form Families. The Edge family has exactly 16
current `0.x` Forms. A worker becomes reachable through a chain of immutable
resources: identity, module bytes, an exported handler version, a traffic
deployment, and an attachment that receives the host-assigned address.

| Resource | Role | What it declares |
| --- | --- | --- |
| [`takoform_module_worker`](/docs/resources/module_worker.html) | identity | A JavaScript module-worker application identity. |
| [`takoform_worker_bundle`](/docs/resources/worker_bundle.html) | revision | An immutable uploaded code bundle. |
| [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html) | revision | An immutable uploaded static-file inventory. |
| [`takoform_worker_version`](/docs/resources/worker_version.html) | revision | An immutable snapshot of handlers, vars, bindings, and bundle. |
| [`takoform_worker_deployment`](/docs/resources/worker_deployment.html) | deployment | Which versions serve traffic, in basis points. |
| [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html) | attachment | A customer-owned hostname whose origin is the worker. |
| [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html) | attachment | HTTPS reachability at a host-assigned address. |
| [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html) | attachment | A UTC cron invoking the scheduled handler. |
| [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html) | identity | An eventually consistent edge KV namespace. |
| [`takoform_sqlite_database`](/docs/resources/sqlite_database.html) | identity | A SQLite-semantics serverless database. |
| [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html) | revision | An immutable ordered SQL migration set. |
| [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html) | attachment | Checksum-safe suffix application to one database. |
| [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html) | identity | At-least-once delivery with acknowledgement and retry. |
| [`takoform_queue_consumer`](/docs/resources/queue_consumer.html) | attachment | Batch, retry, and dead-letter policy for one worker. |
| [`takoform_durable_workflow`](/docs/resources/durable_workflow.html) | identity | Durable multi-step execution as a workflow class. |
| [`takoform_actor_namespace`](/docs/resources/actor_namespace.html) | identity | Addressable actors with one live context, private storage, and one alarm. |

Workers use capability through typed [Interface contracts](/spec/interface-contract/)
and [Binding contracts](/spec/binding-contract/). Inward activation is always
a separate attachment resource
([decision 0021](/spec/decisions/0021-third-party-forms-and-contract-distribution.html)).

## Deferred candidate families (historical source)

The repository retains provider-generated documentation for earlier candidate
families so old state and source history remain inspectable. Function,
Container, Table, Pull Queue, Topic, Schedule, and Vector candidates are
**historical/deferred**, not members of the current official corpus. They are
not advertised as current Host support or current Form identities.

<details>
<summary>Retained deferred resource names</summary>

`takoform_function`, `takoform_function_version`,
`takoform_function_deployment`, `takoform_function_endpoint`,
`takoform_serverless_container_service`, `takoform_container_revision`,
`takoform_container_traffic`, `takoform_container_endpoint`,
`takoform_container_custom_domain`, `takoform_table`, `takoform_pull_queue`,
`takoform_topic`, `takoform_topic_subscription`,
`takoform_message_schedule`, `takoform_dense_vector_index`

</details>

Their pages are retained as historical/deferred references and are kept out
of the Current navigation. The immutable source is not deleted merely because
the current corpus narrowed to Edge 16.

## Historical receipts and retained releases

Specification 1.0/1.1 receipts, withdrawn Host API epochs, and Provider 1/2
compatibility releases remain available as archive evidence for exact pins,
recovery, and migration. They are not a current version lane. Read the
[historical release records](/release/) and the
[v2 to v3 migration boundary](/release/migrations/v2-to-v3.html) for those
identities.

<StatusNote />
