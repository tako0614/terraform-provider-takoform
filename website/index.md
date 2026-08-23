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
metadata after owner publication. The pre-Beta epochs were withdrawn
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html));
their identities are retired, their bytes stay in repository history, and no
current approval or admission is implied by anything they published.

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
| [`takoform_durable_workflow`](/docs/resources/durable_workflow.html)                         | identity   | durable multi-step execution as a class the worker's deployment serves         |
| [`takoform_actor_namespace`](/docs/resources/actor_namespace.html)                           | identity   | addressable actors with one live context, private storage, and one alarm       |

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

## Withdrawn epochs and published history

Two pre-Beta epochs preceded the current stack and were withdrawn
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html)).
Published provider releases that carried them remain immutable Registry
history: **Provider 2.0.0** (the `forms.takoform.com/v1alpha2` compatibility
client) and **Provider 1.0.3** (the `forms.takoform.com/v1alpha1` Legacy
client) stay installable under exact pins for existing state, recovery, and
migration, but their resources have no successors and this site no longer
documents them. The next release published from this repository will be a
major, `3.0.0`; existing users of the withdrawn resources follow the
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
