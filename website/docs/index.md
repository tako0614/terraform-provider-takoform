# Documentation

This page starts with the current design target: Provider 2.1.1, Host API
v1beta1, and the Edge Form Family v1beta1. Compatibility, migration, and
history are kept below in a collapsed section so the old contracts remain
reachable without competing with the current stack.

## Current design target — Provider 2.1.1 / Host API v1beta1

| Axis                  | Current identity                       | Meaning and availability                                                                                                 |
| --------------------- | -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Provider              | **Provider 2.1.1**                     | Registry-published stable client distribution; repository descriptor remains `candidate-only` metadata by design after owner publication. |
| Host API              | **Host API v1beta1**                   | Beta protocol for discovery, exact Form availability, operations, fencing, and errors.                                   |
| Form Family           | **Edge Form Family v1beta1**           | Beta family with 15 current Experimental Form definitions.                                                                  |
| Form definition       | **definition 0.1.0**                   | Exact definition version for each current Form.                                                                          |
| Form Package envelope | `packages.forms.takoform.com/v1alpha4` | Separate package/distribution schema identifier; package artifacts remain unpublished.                                   |

The provider target, Host API maturity, Form family maturity, and package
publication state are independent facts. Provider 2.1.1 is not called Beta:
the Host API is Beta, while the 15 Forms are Experimental. Registry readback
establishes Provider availability; the repository descriptor remains
`candidate-only` metadata by design after owner publication.
The [release-policy obligations](/spec/publication-freeze.html) remain separate
from that target status.

Takoform is an Experimental specification project. Provider 2.1.1 is the
Registry-published stable client distribution; its `release/version.json`
descriptor intentionally remains `candidate-only` metadata after owner
publication. The pre-Beta epochs were withdrawn
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html));
no current central Takoform-wide approval or admission is implied by anything
they published.

## Current design target — Edge Form Family v1beta1 (15 Experimental Forms) {#beta-edge-platform-family}

The family speaks the Host API v1beta1, discovered at
`/.well-known/takoform/v1beta1`, with UID/generation/revision identity,
long-running operations, and content-addressed artifact upload.

A worker becomes reachable through a chain, not a single resource: an identity,
an immutable bundle of module bytes, an immutable version that names the
handlers those bytes export, a deployment that sends traffic to it, and an
attachment that gives it an address. An endpoint whose worker has no active
deployment never becomes Ready, so the whole chain is one configuration:

This shape example pins the Registry-published Provider 2.1.1. The release
descriptor's `candidate-only` value is metadata only and does not revoke the
published client.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.1.1"
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

## Current resource reference {#resource-reference}

These are the exact 15 typed resources in the current Edge Form Family. Every
one is an Experimental Form at definition 0.1.0:

- [module_worker](/docs/resources/module_worker.html)
- [worker_bundle](/docs/resources/worker_bundle.html)
- [static_asset_bundle](/docs/resources/static_asset_bundle.html)
- [worker_version](/docs/resources/worker_version.html)
- [worker_deployment](/docs/resources/worker_deployment.html)
- [worker_custom_domain](/docs/resources/worker_custom_domain.html)
- [worker_endpoint](/docs/resources/worker_endpoint.html)
- [worker_cron_trigger](/docs/resources/worker_cron_trigger.html)
- [edge_kv_namespace](/docs/resources/edge_kv_namespace.html)
- [edge_object_bucket](/docs/resources/edge_object_bucket.html)
- [sqlite_database](/docs/resources/sqlite_database.html)
- [sqlite_migration_set](/docs/resources/sqlite_migration_set.html)
- [sqlite_migration_application](/docs/resources/sqlite_migration_application.html)
- [at_least_once_queue](/docs/resources/at_least_once_queue.html)
- [queue_consumer](/docs/resources/queue_consumer.html)

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
- [Form inventory](/forms/) — the current 15 Forms and retained compatibility identities
- [Conformance evidence](/conformance/) — how compatibility is proven
- [Release](/release/) — provider publication boundary, Form Packages, and migrations
- [Glossary](/docs/glossary.html) — terms used across this documentation

## Host boundary

Takoform owns workload semantics, schemas, exact identities, packages, and
conformance. Hosts own capability support, placement, routing, scaling,
credentials, recovery, and any managed service's live catalog, billing, quota,
and SLA.

<StatusNote />
