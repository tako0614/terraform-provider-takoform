# Takoform Provider

Takoform Provider is the Terraform/OpenTofu client for a compatible Takoform
Host. It maps typed configuration to exact Form contracts and keeps resource
identity and desired state in Terraform state; the Host runs the service.
The current API/Core checkpoint is **`v1.0.1`** on the existing
`forms.takoform.com/v1` wire and discovery lane.

## Install and configure

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 3.0.0"
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

Provider 3 contains 31 mappings across eight families. Resource names are
Provider metadata; contract meaning comes from the linked Form Definitions and
the [Core v1.0.1 specification](https://github.com/tako0614/takoform/tree/v1.0.1/spec).

## Resource reference {#resource-reference}

The generated reference covers every Provider 3 mapping:

### Edge (16)

- [`takoform_module_worker`](/docs/resources/module_worker.html), [`takoform_worker_bundle`](/docs/resources/worker_bundle.html), [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html), [`takoform_worker_version`](/docs/resources/worker_version.html)
- [`takoform_worker_deployment`](/docs/resources/worker_deployment.html), [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html), [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html), [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)
- [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html), [`takoform_sqlite_database`](/docs/resources/sqlite_database.html), [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html), [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html)
- [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html), [`takoform_queue_consumer`](/docs/resources/queue_consumer.html), [`takoform_durable_workflow`](/docs/resources/durable_workflow.html), [`takoform_actor_namespace`](/docs/resources/actor_namespace.html)

### Function (4)

- [`takoform_function`](/docs/resources/function.html), [`takoform_function_version`](/docs/resources/function_version.html), [`takoform_function_deployment`](/docs/resources/function_deployment.html), [`takoform_function_endpoint`](/docs/resources/function_endpoint.html)

### Container (5)

- [`takoform_serverless_container_service`](/docs/resources/serverless_container_service.html), [`takoform_container_revision`](/docs/resources/container_revision.html), [`takoform_container_traffic`](/docs/resources/container_traffic.html), [`takoform_container_endpoint`](/docs/resources/container_endpoint.html), [`takoform_container_custom_domain`](/docs/resources/container_custom_domain.html)

### Queue, schedule, table, topic, and vector (6)

- [`takoform_pull_queue`](/docs/resources/pull_queue.html), [`takoform_message_schedule`](/docs/resources/message_schedule.html), [`takoform_table`](/docs/resources/table.html), [`takoform_topic`](/docs/resources/topic.html), [`takoform_topic_subscription`](/docs/resources/topic_subscription.html), [`takoform_dense_vector_index`](/docs/resources/dense_vector_index.html)

Each generated page shows the full four-field FormRef, its separate package
digest, typed arguments, state behavior, import contract, and source Form. The
[mapping inventory](/forms/) lists the roster and each `definitionVersion`;
the [identity ledger](/release/provider-form-identities.json) retains exact
release identities.

## History and migration

Current compatibility and retained releases are summarized in
[Versions and compatibility](/docs/versions.html). Existing users of an older
provider should follow the [v2-to-v3 migration boundary](/release/migrations/v2-to-v3.html).

The executable compatibility checks are listed in [Conformance](/conformance/).

## Before apply

Before applying, confirm that the configured Host advertises support for every
exact FormRef in the plan.
