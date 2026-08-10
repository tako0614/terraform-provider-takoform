# Documentation

Get from zero to a running resource, then choose the lane that matches your
job.

| Tier | What it covers | How you get it |
| --- | --- | --- |
| **Current published** | provider `v2.0.0` and the retained nine `forms.takoform.com/v1alpha2` resources | `terraform init` installs it from the Registry |
| **Beta release candidate** | stable provider target `v2.1.1`, Beta Host API, and 15 Experimental `edge.forms.takoform.com/v1beta1` Forms | build the provider from repository source until the owner publishes it |

The quick start below is the current published tier. The
[Beta release candidate](#beta-edge-platform-family) is further down and is marked
as such everywhere it appears.

## Quick start

Provider `v2.0.0` is the published client and the installable path; it
exposes the nine retained provider-v2 resources. To see the provider and all
nine resources exercised together, run the repository conformance matrix:

```sh
bun run check:current-form-candidates
go run ./cmd/provider-lifecycle-conformance matrix \
  --opentofu tofu --terraform terraform
```

The matrix proves preview/apply/observe/refresh/delete against the exact
v1alpha2 contracts without touching a real host. Against a real host, first
verify it advertises the exact v1alpha2 FormRefs at its versioned discovery
path (`/.well-known/takoform/v1alpha2`).

### Pinning the published provider

Pin provider `v2.0.0` to use the retained provider-v2 lane. `init` installs
it from the Registry.

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

### Against a real host

The matrix exercises the provider against an in-process reference host. To
drive a real host, ask the host for three values: its API `endpoint`, a
`space` to target, and a bearer `token`. They can go in provider configuration
or in the `TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`
environment variables.

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

The host must advertise the exact v1alpha2 FormRefs at
`/.well-known/takoform/v1alpha2` before the provider issues a mutation; a host
that cannot serve the exact identity fails closed. This repository does not
assert any hosted service's live availability; obtain endpoint, Space, and
token from the host you chose. Use the digest and URL of an artifact you can
actually fetch; the values above are shape only.

## Lanes

One provider address, `registry.terraform.io/tako0614/takoform`, serves three
lanes:

| Lane | Use for | Install |
| --- | --- | --- |
| **v1.0.3** (published) | existing Legacy state, refresh, delete, recovery | from the Registry |
| **v2.0.0** (published, current client) | the nine retained provider-v2 contracts | from the Registry |
| **v2.1.1** (stable release target; descriptor `candidate-only`) | [the Experimental Edge Platform Family](#beta-edge-platform-family) on v1beta1 | build from source until owner publication |

### Maintain published Legacy

For existing v1 state, pin the published provider `v1.0.3`. It does not turn
that state into v2 semantics.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.3"
    }
  }
}
```

### Migrate from v1

Migration is an explicit create/import, never an automatic state rewrite.
Provider v2 refuses provider-v1 state.

1. Pin provider v1 and refresh the Legacy resource.
2. Capture non-secret desired configuration and required public outputs.
3. Create under the exact v1alpha2 FormRef, or import only with host
   conformance proof.
4. Move consumers, observe the result, then delete Legacy through v1 after
   rollback is no longer needed.

## Beta: Edge Platform Family

::: warning Beta provider release candidate
The `edge.forms.takoform.com/v1beta1` family rides the stable provider `v2.1.1`
release target. Its descriptor remains `candidate-only` until the release
owner publishes it, so build from repository source for now. All 15 exact
`0.1.0` Forms are Experimental and locked in the provider identity ledger;
their package artifacts remain unpublished. Open
[release-policy obligations](/spec/publication-freeze.html) remain later
Stable/GA and package/public-service work, not a reason to relabel the provider
version as a prerelease. The published `v2.0.0` quick start above remains the
Registry-installable path.
:::

Family resources speak the `forms.takoform.com/v1beta1` Host API, discovered
at `/.well-known/takoform/v1beta1`, with UID/generation/revision identity,
long-running operations, and content-addressed artifact upload.

A worker becomes reachable through a chain, not a single resource: an identity,
an immutable bundle of module bytes, an immutable version that names the
handlers those bytes export, a deployment that sends traffic to it, and an
attachment that gives it an address. An endpoint whose worker has no active
deployment never becomes Ready, so the whole chain is one configuration:

```hcl
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

Each resource's own page carries a single-resource example with the `v2.1.1`
stable-target pin and the same candidate-only boundary. Capability is added to a
version through typed bindings; inward activation — a custom domain, a cron
trigger, a queue consumer — is always a separate attachment resource.

## Resource reference

Each page documents the arguments, read-only attributes, declared interfaces,
and import behavior of one resource.

Retained provider-v2 resources (published `v2.0.0`):

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

Experimental Edge Platform Family resources (`v2.1.1` stable release target;
descriptor `candidate-only`):

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

That is the entire v1beta1 surface. There is no generic carrier for a Form the
provider was not built against: the lane offers no way for a client to verify a
FormRef it did not compile in, so supporting a third-party Form is a provider
build rather than a configuration value
([decision 0021](/spec/decisions/0021-third-party-forms-and-contract-distribution.html)).

## More project surfaces

- [Form Proposals](/proposals/) — design material for Forms that have not
  earned a public FormRef
- [Form inventory](/forms/) — the retained nine v1alpha2 candidates and the
  Edge Platform Family
- [Conformance evidence](/conformance/) — how compatibility is proven
- [Release](/release/) — provider publication boundary, Form Packages, and
  migrations
- [Glossary](/docs/glossary.html) — terms used across this documentation

## Host boundary

Takoform owns workload semantics, schemas, exact identities, packages, and
conformance. Hosts own capability support, placement, routing, scaling,
credentials, recovery, and any managed service's live catalog, billing, quota,
and SLA.

<StatusNote />
