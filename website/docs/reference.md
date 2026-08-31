---
page_title: "Takoform provider"
description: "Stable Takoform Host API v1, official Form mappings, and retained Provider history"
---

# Takoform provider

The Host API at `forms.takoform.com/v1` is the stable Takoform wire contract
for discovery, exact Form availability, lifecycle operations, fencing, and
errors. Numbered Specification 1.0/1.1 documents are immutable historical
receipts, not a current version stream; they do not create an API v1.1, promote
a Form, or release a Provider. The current corpus has eight versionless
families and 31 Experimental `0.x` Forms, using package envelope
`packages.forms.takoform.com/v1alpha5`.

The two pre-Beta epochs (`forms.takoform.com/v1alpha1` Legacy and the
`forms.takoform.com/v1alpha2` provider-v2 epoch) were withdrawn
([decision 0042](../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md)).
Published provider releases that carried them remain immutable Registry
history; the withdrawn resources have no successors in this documentation, and
[the migration boundary](../release/migrations/v2-to-v3.md) says what existing
state does.

Provider `v3.0.0` is the current Registry-published software tooling with typed
mappings for official Forms only. The canonical
`registry.terraform.io/tako0614/takoform` distribution does not publish a
universal infrastructure provider. Independent third parties may distribute
Forms under their own namespaces through the same package and verification
path; each Provider build must explicitly map the Forms it supports. Provider
`v2.1.1` remains immutable Registry history for the exact `v1beta1` identities
it shipped. Using any Provider requires a compatible host; publication does
not assert a hosted service's live availability.

## Install the provider

Terraform and OpenTofu can install this exact pin from the canonical Registry
address:

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

Terraform/OpenTofu modules can declare multiple Takoform and industry-standard
providers together. This Provider is typed software tooling for the Forms it
supports, not a universal infrastructure provider.

## Verify the published current provider

Availability is verified, not declared by this immutable documentation.

```console
curl -fsS https://registry.terraform.io/v1/providers/tako0614/takoform/versions
git clone https://github.com/tako0614/terraform-provider-takoform.git
cd terraform-provider-takoform
git checkout --detach v3.0.0
```

A source tag, documentation page, or local build alone is not Registry publication or installation evidence.

## Current Provider 3 Form reference

The official Registry-published Provider 3 maps the current official
Experimental Forms. These resource type names are non-normative Provider
metadata; Form maturity and Form Package publication remain separate.

### Edge family

The versionless `edge.forms.takoform.com` family contains 16 exact
Experimental `0.x` Forms and intentionally has no current `ObjectBucket`. It
is one of the eight current families:

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

### Function family

- [Function](resources/function.md)
- [FunctionVersion](resources/function_version.md)
- [FunctionDeployment](resources/function_deployment.md)
- [FunctionEndpoint](resources/function_endpoint.md)

### Container family

- [ContainerService](resources/serverless_container_service.md)
- [ContainerRevision](resources/container_revision.md)
- [ContainerTraffic](resources/container_traffic.md)
- [ContainerEndpoint](resources/container_endpoint.md)
- [ContainerCustomDomain](resources/container_custom_domain.md)

### Table, queue, topic, schedule, and vector families

- [Table](resources/table.md)
- [PullQueue](resources/pull_queue.md)
- [Topic](resources/topic.md)
- [TopicSubscription](resources/topic_subscription.md)
- [Schedule](resources/message_schedule.md)
- [VectorIndex](resources/dense_vector_index.md)

The provider exposes no generic carrier
for a Form it was not built against: a resource whose Form identity comes from
the configuration cannot verify that identity, because the lane's Form
Definition response carries neither the canonical definition bytes the
`schemaDigest` pins nor the Form's role
([decision 0021](../spec/decisions/0021-third-party-forms-and-contract-distribution.md)).
Supporting a third-party Form is a provider build, not a configuration value.

## Authority boundary

- Takoform owns Form schemas, immutable identities, lifecycle vocabulary, and
  portable conformance tooling.
- A host owns installation, executable support, activation, placement,
  credentials, and recovery policy.
- Each host or commercial service owns its implementations, capacity, billing,
  quota, SLA, and live catalog.
- Provider release, Form publication, Form maturity, and host availability are
  independent facts.
