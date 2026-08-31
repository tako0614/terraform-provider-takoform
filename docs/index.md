---
page_title: "Takoform provider"
description: "Stable Takoform Host API v1, official Form mappings, and retained Provider history"
---

# Takoform provider

The Host API at `forms.takoform.com/v1` is the stable Takoform wire contract
for discovery, exact Form availability, lifecycle operations, fencing, and
errors. Specification 1.0/1.1 receipts are archive evidence: they do not
create an API v1.1, promote a Form, or release a Provider.

The current official publisher corpus is one versionless family,
`edge.forms.takoform.com`, with 16 exact Experimental `0.x` Forms, 7
Interfaces, and 6 Bindings. Its source roster is pinned to [`takoform-forms`
commit `3a395e4`](https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e).
The Form Package identifier `packages.forms.takoform.com/v1alpha5` is a
wire/envelope format, not an additional product version axis.

The canonical `registry.terraform.io/tako0614/takoform` Provider is an
official-Forms-only typed tool. Independent third parties may distribute Forms
under their own namespaces using the same Host API path and package/verification
contracts; each Provider build must explicitly map the Forms it supports. A
module may combine the official Takoform Provider with other Takoform or
industry-standard Providers. It is not a universal infrastructure provider.

Provider `3.0.0` is the released Registry implementation. Its immutable
8-family/31-resource projection is Provider release history, not the current
publisher corpus. The official-only Edge16 Provider mapping is a next-major
candidate and is not published; examples requiring `>=3.1.0` are likewise
candidate/unpublished until a release record says otherwise.

The two pre-Beta epochs (`forms.takoform.com/v1alpha1` Legacy and the
`forms.takoform.com/v1alpha2` provider-v2 epoch) were withdrawn
([decision 0042](../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md)).
Published Provider releases that carried them remain immutable Registry
history; the withdrawn resources have no successors in this documentation, and
[the migration boundary](../release/migrations/v2-to-v3.md) says what existing
state does.

## Install the released Provider 3.0.0

Terraform and OpenTofu can install this exact historical Provider pin from the
canonical Registry address. It exposes the immutable Provider 3 projection;
that release does not redefine the current publisher roster.

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
```

`endpoint`, `space`, and bearer `token` may instead come from
`TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.

## Current Edge Form reference

The current publisher family contains these 16 exact Forms. The corresponding
Provider names are non-normative mapping metadata:

- [ModuleWorker](resources/module_worker.md)
- [WorkerBundle](resources/worker_bundle.md)
- [StaticAssetBundle](resources/static_asset_bundle.md)
- [WorkerVersion](resources/worker_version.md)
- [WorkerDeployment](resources/worker_deployment.md)
- [WorkerCustomDomain](resources/worker_custom_domain.md)
- [WorkerEndpoint](resources/worker_endpoint.md)
- [WorkerCronTrigger](resources/worker_cron_trigger.md)
- [EdgeKVNamespace](resources/edge_kv_namespace.md)
- [SQLiteDatabase](resources/sqlite_database.md)
- [SQLiteMigrationSet](resources/sqlite_migration_set.md)
- [SQLiteMigrationApplication](resources/sqlite_migration_application.md)
- [AtLeastOnceQueue](resources/at_least_once_queue.md)
- [QueueConsumer](resources/queue_consumer.md)
- [DurableWorkflow](resources/durable_workflow.md)
- [ActorNamespace](resources/actor_namespace.md)

## Deferred candidate resource source

The following non-Edge pages are retained as historical/deferred Provider
source. They are not current Forms or current Host support and are kept out of
Current navigation:

### Function

- [Function](resources/function.md)
- [FunctionVersion](resources/function_version.md)
- [FunctionDeployment](resources/function_deployment.md)
- [FunctionEndpoint](resources/function_endpoint.md)

### Container

- [ContainerService](resources/serverless_container_service.md)
- [ContainerRevision](resources/container_revision.md)
- [ContainerTraffic](resources/container_traffic.md)
- [ContainerEndpoint](resources/container_endpoint.md)
- [ContainerCustomDomain](resources/container_custom_domain.md)

### Table, queue, topic, schedule, and vector

- [Table](resources/table.md)
- [PullQueue](resources/pull_queue.md)
- [Topic](resources/topic.md)
- [TopicSubscription](resources/topic_subscription.md)
- [Schedule](resources/message_schedule.md)
- [VectorIndex](resources/dense_vector_index.md)

The old Provider projection of these pages is an immutable 8-family/31-resource
release record. Keeping links here preserves source history without presenting
those families as current.

## Verify the published Provider artifact

Availability is verified, not declared by this immutable documentation.

```console
curl -fsS https://registry.terraform.io/v1/providers/tako0614/takoform/versions
git clone https://github.com/tako0614/terraform-provider-takoform.git
cd terraform-provider-takoform
git checkout --detach v3.0.0
```

A source tag, documentation page, or local build alone is not Registry
publication or installation evidence.

## Authority boundary

- Takoform owns Form schemas, immutable identities, lifecycle vocabulary, and
  portable conformance tooling.
- A host owns installation, executable support, activation, placement,
  credentials, and recovery policy.
- Each host or commercial service owns its implementations, capacity, billing,
  quota, SLA, and live catalog.
- Provider release, Form publication, Form maturity, and host availability are
  independent facts.
