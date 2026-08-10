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
publication. The 34 published Form Package identities belong to immutable
Legacy history. No current central Takoform-wide approval or admission is
implied by that historical publication set.

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

## Published compatibility / Migration / History {#lanes}

<details>
<summary>Published compatibility: Provider 2.0.0, retained v1alpha2 resources, and Legacy Provider 1.0.3</summary>

### Quick start for Provider 2.0.0 / Host API v1alpha2 {#quick-start}

Provider 2.0.0 is the Registry-published compatibility client on the retained
Host API boundary `forms.takoform.com/v1alpha2`. It exposes the nine retained
provider-v2 resources. To exercise the provider and
all nine resources together, run the repository conformance matrix:

```sh
bun run check:current-form-candidates
go run ./cmd/provider-lifecycle-conformance matrix \
  --opentofu tofu --terraform terraform
```

The matrix proves preview/apply/observe/refresh/delete against exact v1alpha2
contracts without touching a real host. Against a real host, first verify it
advertises the exact v1alpha2 FormRefs at
`/.well-known/takoform/v1alpha2`.

#### Pinning the published Provider 2.0.0

`terraform init` installs Provider 2.0.0 from the Registry:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.0.0"
    }
  }
}
```

#### Against a real host

The matrix exercises the provider against an in-process reference host. To
drive a real host, ask it for its API `endpoint`, a `space` to target, and a
bearer `token`. They can go in provider configuration or in
`TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.

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

resource "takoform_edge_worker" "example" {
  name                = "edge-worker"
  artifact_media_type = "application/vnd.takoform.edge-worker+tar"
  artifact_sha256     = "sha256:0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37"
  artifact_url        = "https://artifacts.portable-conformance.invalid/edge-worker.tar"
  entrypoint          = "worker.mjs"
  runtime             = "javascript"
  runtime_version     = "2026.1"
  configuration       = { "LOG_LEVEL" = "info" }
}
```

```console
terraform init
terraform plan
terraform apply
```

The host must advertise exact v1alpha2 FormRefs before the provider issues a
mutation; a host that cannot serve the exact identity fails closed. This
repository does not assert any hosted service's live availability. Obtain the
endpoint, Space, and token from the host you chose, and use an artifact digest
and URL that you can actually fetch.

### Provider 2.0.0 retained resources

These nine resource URLs remain the compatibility surface:

- [edge_worker](/docs/resources/edge_worker.html)
- [relational_database](/docs/resources/relational_database.html)
- [object_bucket](/docs/resources/object_bucket.html)
- [key_value_store](/docs/resources/key_value_store.html)
- [queue](/docs/resources/queue.html)
- [schedule](/docs/resources/schedule.html)
- [container_service](/docs/resources/container_service.html)
- [stateful_entity](/docs/resources/stateful_entity.html)
- [vector_index](/docs/resources/vector_index.html)
- [interface data source](/docs/data-sources/interface.html)

The retained package envelope is `packages.forms.takoform.com/v1alpha3`.

### Provider 1.0.3 / Host API v1alpha1

For existing v1 state, pin the published Legacy Provider 1.0.3. It does not
turn that state into v2 semantics. Its v1 Host API boundary is
`forms.takoform.com/v1alpha1`, discovered at `/.well-known/takoform/v1alpha1`,
and the published v1 Form Package identities are immutable history.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      # Published Legacy Provider 1.0.3 for existing v1 state.
      version = "= 1.0.3"
    }
  }
}
```

### Migration from Provider 1.0.3

Migration is an explicit create/import, never an automatic state rewrite:

1. Pin Provider 1.0.3 and refresh the Legacy resource.
2. Capture non-secret desired configuration and required public outputs.
3. Create under an exact v1alpha2 FormRef, or import only with host conformance
   proof, using Provider 2.0.0.
4. Move consumers, observe the result, then delete Legacy through Provider 1.0.3
   after rollback is no longer needed.

See the [Provider 1 to 2 migration guide](/release/migrations/v1-to-v2.html).

</details>

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
