---
title: Get started
description: Install the Takoform Provider, configure a host, and declare your first resource.
---

# Get started

This is the shortest path from an empty Terraform or OpenTofu module to a
typed Takoform resource. The current model has four independent streams; this
page only gets a Provider talking to a host.

## 1. Install the Provider

Add the Registry source and pin the Provider major you intend to use:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 3.0"
    }
  }
}
```

Run `terraform init` (or `tofu init`) to select the exact Provider release.
Provider SemVer is independent from the Host API lane, each Form's
`definitionVersion`, and the Core library SemVer.

## 2. Point at a host

The Provider needs an endpoint and a host-owned space. Credentials, placement,
capacity, routing, and recovery stay with that host.

```hcl
provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}
```

The host advertises supported Forms through its Host Support Profile. The
current wire lane is the literal `forms.takoform.com/v1`; there is no `/v1.1`
lane.

## 3. Declare a shape

Choose a resource from the [reference landing page](/docs/reference-landing.html)
and fill only the portable fields that belong to that Form. For example, a
worker endpoint is an attachment to a worker identity:

```hcl
resource "takoform_module_worker" "api" {
  name = "api"
}

resource "takoform_worker_endpoint" "api" {
  name   = "api"
  worker = takoform_module_worker.api.name
}
```

Each Form page explains its exact fields, lifecycle, and attachment rules.
The Provider does not invent a generic carrier for a Form it was not compiled
against.

Form Package publication remains unpublished; that is artifact availability,
not another version stream.

## 4. Inspect the plan

Run `terraform plan` (or `tofu plan`) and check the host response before
applying. A host may reject an unsupported Form, stale generation, or invalid
attachment. Those failures are evidence that the contract boundary is doing
its job; fix the declaration or host support rather than changing a version
label.

## Next

<details>
<summary>Resource index (31 current Forms)</summary>

- [`takoform_serverless_container_service`](/docs/resources/serverless_container_service.html)
- [`takoform_container_revision`](/docs/resources/container_revision.html)
- [`takoform_container_traffic`](/docs/resources/container_traffic.html)
- [`takoform_container_endpoint`](/docs/resources/container_endpoint.html)
- [`takoform_container_custom_domain`](/docs/resources/container_custom_domain.html)
- [`takoform_module_worker`](/docs/resources/module_worker.html)
- [`takoform_worker_bundle`](/docs/resources/worker_bundle.html)
- [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html)
- [`takoform_worker_version`](/docs/resources/worker_version.html)
- [`takoform_worker_deployment`](/docs/resources/worker_deployment.html)
- [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html)
- [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html)
- [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)
- [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html)
- [`takoform_sqlite_database`](/docs/resources/sqlite_database.html)
- [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)
- [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html)
- [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)
- [`takoform_queue_consumer`](/docs/resources/queue_consumer.html)
- [`takoform_durable_workflow`](/docs/resources/durable_workflow.html)
- [`takoform_actor_namespace`](/docs/resources/actor_namespace.html)
- [`takoform_function`](/docs/resources/function.html)
- [`takoform_function_version`](/docs/resources/function_version.html)
- [`takoform_function_deployment`](/docs/resources/function_deployment.html)
- [`takoform_function_endpoint`](/docs/resources/function_endpoint.html)
- [`takoform_pull_queue`](/docs/resources/pull_queue.html)
- [`takoform_message_schedule`](/docs/resources/message_schedule.html)
- [`takoform_table`](/docs/resources/table.html)
- [`takoform_topic`](/docs/resources/topic.html)
- [`takoform_topic_subscription`](/docs/resources/topic_subscription.html)
- [`takoform_dense_vector_index`](/docs/resources/dense_vector_index.html)

</details>

- [Version model](/docs/versions.html) — the four streams and the artifact identities that are not streams.
- [Concepts](/docs/concepts.html) — portability, Form identity, and lifecycle.
- [Ownership](/docs/ownership.html) — which repository or runtime owns each decision.
- [Reference landing](/docs/reference-landing.html) — current families and resource links.
- [History](/docs/history.html) — retained Specification receipt and withdrawn provider epochs.

<StatusNote />
