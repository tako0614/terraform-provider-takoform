# Takoform Provider

Takoform Provider is the Terraform/OpenTofu client for a compatible Takoform
Host. It maps typed configuration to exact Form contracts and keeps resource
identity and desired state in Terraform state; the Host runs the service.
The current API/Core checkpoint is **`v1.0.1`** on the existing
`forms.takoform.com/v1` wire and discovery lane.

## Publisher-specific next major

This source checkout keeps the existing
`registry.terraform.io/tako0614/takoform` address and registers only the 17
exact Forms selected from `github.com/tako0614/takoform-forms`. Provider `3.0.0` remains
immutable 31-resource aggregate history. No next-major Registry publication is
claimed yet.

This relationship is identified by publisher repository and exact FormRefs;
Takoform assigns it no privileged classification.

After the next major is published, install and configure it as follows:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 4.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://forms.example.com"
  space    = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}
```

`endpoint`, `space`, and bearer `token` may instead come from
`TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.

Resource names are Provider metadata; contract meaning comes from the linked
publisher Form Definitions and the [Core v1.0.1 specification](https://github.com/tako0614/takoform/tree/v1.0.1/spec).

## Resource reference {#resource-reference}

The generated reference covers the complete publisher-selected Provider roster:

### Edge (17)

- [`takoform_module_worker`](/docs/resources/module_worker.html), [`takoform_worker_bundle`](/docs/resources/worker_bundle.html), [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html), [`takoform_worker_version`](/docs/resources/worker_version.html)
- [`takoform_worker_deployment`](/docs/resources/worker_deployment.html), [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html), [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html), [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)
- [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html), [`takoform_edge_object_bucket`](/docs/resources/edge_object_bucket.html), [`takoform_sqlite_database`](/docs/resources/sqlite_database.html), [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html), [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html)
- [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html), [`takoform_queue_consumer`](/docs/resources/queue_consumer.html), [`takoform_durable_workflow`](/docs/resources/durable_workflow.html), [`takoform_actor_namespace`](/docs/resources/actor_namespace.html)

Each generated page shows the full four-field FormRef, its separate package
digest, typed arguments, state behavior, import contract, and source Form. The
[mapping inventory](/forms/) lists the roster and each `definitionVersion`;
the [identity ledger](/release/provider-form-identities.json) retains exact
release identities.

## History and migration

Current compatibility and retained releases are summarized in
[Versions and compatibility](/docs/versions.html). Existing users of an older
provider should follow the [v2-to-v3 migration boundary](/release/migrations/v2-to-v3.html).
Provider 3 users should read the explicit [v3-to-v4 publisher-set
boundary](/release/migrations/v3-to-v4.html) before upgrading.

## Compose native providers

AWS, Cloudflare, Kubernetes, PostgreSQL, and other industry providers remain
ordinary peers in `required_providers`. OpenTofu installs all declared sources
and builds one dependency graph; Takoform does not wrap their resources as
Forms or maintain a provider catalog. See the [native provider composition
example](/examples/native-provider-composition/main.tf).

The executable compatibility checks are listed in [Conformance](/conformance/).

## Before apply

Before applying, confirm that the configured Host advertises support for every
exact FormRef in the plan.
