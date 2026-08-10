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

## Current design target — Provider 2.1.1 / Host API v1beta1

Takoform is an Experimental specification project. The current stack is
described on five independent axes so a client version, a host protocol, a
Form identity, and package publication cannot be mistaken for one another.

| Axis                  | Current identity                       | Meaning and availability                                                                                                     |
| --------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Provider              | **Provider 2.1.1**                     | Registry-published stable client distribution. The repository descriptor remains `candidate-only` metadata by design after owner publication. |
| Host API              | **Host API v1beta1**                   | Beta protocol for discovery, exact Form availability, operations, fencing, and errors.                                       |
| Form Family           | **Edge Form Family v1beta1**           | Beta family containing the current 15 Experimental Form definitions.                                                          |
| Form definition       | **definition 0.1.0**                   | Exact definition version for each current Form.                                                                              |
| Form Package envelope | `packages.forms.takoform.com/v1alpha4` | Separate package/distribution schema identifier; package artifacts remain unpublished.                                       |

Provider 2.1.1 is the Registry-published current client distribution. The
repository's `release/version.json` descriptor intentionally remains
`candidate-only` metadata after owner publication; this does not revoke the
published client. Provider distribution availability is reported separately
below and in the [machine-readable status](/.well-known/takoform-site.json).
Use the [current quick start](/docs/) for an executable configuration.

Provider 2.1.1 is Registry-published; its descriptor remains `candidate-only`
metadata after owner publication. The 34 published Form Package identities
belong to immutable Legacy history. No current central Takoform-wide approval
or admission is implied by that historical publication set.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.1.1"
    }
  }
}
```

## Current design target — Edge Form Family v1beta1 (15 Experimental Forms)

The Host API v1beta1 carries the current Edge Form Family v1beta1
([Form Families](/spec/form-families.html)). Each of its
15 Forms is Experimental and uses definition 0.1.0. A worker becomes reachable
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
| [`takoform_edge_object_bucket`](/docs/resources/edge_object_bucket.html)                     | identity   | a strongly consistent object bucket                                            |
| [`takoform_sqlite_database`](/docs/resources/sqlite_database.html)                           | identity   | a SQLite-semantics serverless database                                         |
| [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)                 | revision   | an immutable ordered SQL migration set                                         |
| [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html) | attachment | checksum-safe suffix application to one database                               |
| [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)                   | identity   | at-least-once delivery with acknowledgement and retry                          |
| [`takoform_queue_consumer`](/docs/resources/queue_consumer.html)                             | attachment | batch, retry, and dead-letter policy targeting one worker                      |

::: warning Provider distribution boundary
Provider 2.1.1 is a Registry-published stable distribution. Its repository
descriptor remains `candidate-only` metadata after owner publication, and the
15 Form Package artifacts are unpublished by this page. The target's SemVer,
the Host API's Beta protocol, the Form family's Beta maturity, and the 15 Forms'
Experimental maturity are separate facts. The
[release-policy obligations](/spec/publication-freeze.html) remain separate.
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

## Published compatibility / Migration / History

<details>
<summary>Published compatibility: Provider 2.0.0, retained v1alpha2 resources, and Legacy Provider 1.0.3</summary>

### Provider 2.0.0 / Host API v1alpha2

Provider 2.0.0 is the Registry-published compatibility client for the retained
`forms.takoform.com/v1alpha2` surface. It retains these nine resources:

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
them out of your state. Provider 2.0.0 discovers exact retained FormRefs at
`/.well-known/takoform/v1alpha2`; its retained package index is
`packages.forms.takoform.com/v1alpha3`
([decision 0035](/spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.html)).

| Resource                                                                   | What it declares                                         |
| -------------------------------------------------------------------------- | -------------------------------------------------------- |
| [`takoform_edge_worker`](/docs/resources/edge_worker.html)                 | a request/event application from a digest-bound artifact |
| [`takoform_relational_database`](/docs/resources/relational_database.html) | a relational database by open engine token               |
| [`takoform_object_bucket`](/docs/resources/object_bucket.html)             | object storage                                           |
| [`takoform_key_value_store`](/docs/resources/key_value_store.html)         | key/value state                                          |
| [`takoform_queue`](/docs/resources/queue.html)                             | at-least-once message delivery                           |
| [`takoform_schedule`](/docs/resources/schedule.html)                       | a cron that invokes one connected resource               |
| [`takoform_container_service`](/docs/resources/container_service.html)     | a service from an OCI image digest                       |
| [`takoform_stateful_entity`](/docs/resources/stateful_entity.html)         | addressable persistent entities                          |
| [`takoform_vector_index`](/docs/resources/vector_index.html)               | a vector index with fixed dimensions                     |

### Provider 1.0.3 / Host API v1alpha1

Provider 1.0.3 is the published Legacy client for existing v1 state. Keep it
pinned for refresh, delete, recovery, and migration steps that still need the
v1 wire. Its Host API boundary is `forms.takoform.com/v1alpha1`, with discovery
at `/.well-known/takoform/v1alpha1`; published v1 Form Package identities are
immutable history.

### Migration

Migration is explicit create/import, never an automatic state rewrite:

1. Pin Provider 1.0.3 and refresh the Legacy resource.
2. Capture non-secret desired configuration and required public outputs.
3. Create under an exact v1alpha2 FormRef, or import only with host conformance
   proof, using Provider 2.0.0.
4. Move consumers, observe the result, then delete Legacy after rollback is no
   longer needed.

See the [Provider 1 to 2 migration guide](/release/migrations/v1-to-v2.html).

</details>

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
