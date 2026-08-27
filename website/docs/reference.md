---
page_title: "Takoform provider"
description: "Terraform/OpenTofu Provider 3 resource reference"
---

# Takoform Provider

Takoform Provider is a Terraform/OpenTofu client for a compatible Host. It
maps typed resource configuration to exact Form contracts and stores the
resulting identity and desired state in Terraform state; the Host runs the
service. API/Core checkpoint **`v1.0.1`** uses the existing
`forms.takoform.com/v1` wire and discovery lane.

## Install and configure

Install the current Registry release and point it at a compatible Host:

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

## Resource reference

### Edge (16)

- [ModuleWorker](resources/module_worker.md), [WorkerBundle](resources/worker_bundle.md), [StaticAssetBundle](resources/static_asset_bundle.md), [WorkerVersion](resources/worker_version.md)
- [WorkerDeployment](resources/worker_deployment.md), [WorkerCustomDomain](resources/worker_custom_domain.md), [WorkerEndpoint](resources/worker_endpoint.md), [WorkerCronTrigger](resources/worker_cron_trigger.md)
- [EdgeKVNamespace](resources/edge_kv_namespace.md), [SQLiteDatabase](resources/sqlite_database.md), [SQLiteMigrationSet](resources/sqlite_migration_set.md), [SQLiteMigrationApplication](resources/sqlite_migration_application.md)
- [AtLeastOnceQueue](resources/at_least_once_queue.md), [QueueConsumer](resources/queue_consumer.md), [DurableWorkflow](resources/durable_workflow.md), [ActorNamespace](resources/actor_namespace.md)

### Function (4)

- [Function](resources/function.md), [FunctionVersion](resources/function_version.md), [FunctionDeployment](resources/function_deployment.md), [FunctionEndpoint](resources/function_endpoint.md)

### Container (5)

- [ContainerService](resources/serverless_container_service.md), [ContainerRevision](resources/container_revision.md), [ContainerTraffic](resources/container_traffic.md), [ContainerEndpoint](resources/container_endpoint.md), [ContainerCustomDomain](resources/container_custom_domain.md)

### Queue, schedule, table, topic, and vector (6)

- [PullQueue](resources/pull_queue.md), [Schedule](resources/message_schedule.md), [Table](resources/table.md), [Topic](resources/topic.md), [TopicSubscription](resources/topic_subscription.md), [VectorIndex](resources/dense_vector_index.md)

Each generated resource page shows the full four-field FormRef, its separate
package digest, typed arguments, state behavior, import contract, and source
Form. The [mapping inventory](../forms/) lists the roster and each
`definitionVersion`; the [identity ledger](../release/provider-form-identities.json)
retains the exact release identities.

## Verify the release

```console
curl -fsS https://registry.terraform.io/v1/providers/tako0614/takoform/versions
git clone https://github.com/tako0614/terraform-provider-takoform.git
cd terraform-provider-takoform
git checkout --detach v3.0.0
```

Version and migration history are kept in `website/docs/versions.md` and the
[v2-to-v3 migration guide](../release/migrations/v2-to-v3.md).

## Host requirements

Before applying, confirm that the configured Host advertises support for every
exact FormRef in the plan. Executable compatibility checks are listed in
[Conformance](../conformance/index.md).
