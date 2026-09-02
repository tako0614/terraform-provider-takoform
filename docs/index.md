---
page_title: "Takoform provider"
description: "Publisher-specific Takoform Terraform/OpenTofu Provider resource reference"
---

# Takoform Provider

Takoform Provider is a Terraform/OpenTofu client for a compatible Host. It
maps typed resource configuration to exact Form contracts and stores the
resulting identity and desired state in Terraform state; the Host runs the
service. API/Core checkpoint **`v1.0.1`** uses the existing
`forms.takoform.com/v1` wire and discovery lane.

## Publisher-specific major

This checkout implements the Provider major published at the
`registry.terraform.io/tako0614/takoform` source address. It registers only the
17 exact Forms selected from `github.com/tako0614/takoform-forms`. Provider `3.0.0`
remains immutable 31-Form aggregate history and is not silently rewritten.

This relationship is identified by publisher repository and exact FormRefs;
Takoform assigns it no privileged classification.

Point it at a compatible Host:

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

Provider `4.0.0` is recorded as Registry-published in the
[Provider release identity ledger](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/provider-release-identities.json), whose entry carries the immutable
GitHub Release and the Registry readback for that version.
Availability is verified, not declared by this immutable documentation.
The exact removal and state boundary is documented in the
[v3-to-v4 migration guide](../release/migrations/v3-to-v4.md).

## Resource reference

### Edge (17)

- [ModuleWorker](resources/module_worker.md), [WorkerBundle](resources/worker_bundle.md), [StaticAssetBundle](resources/static_asset_bundle.md), [WorkerVersion](resources/worker_version.md)
- [WorkerDeployment](resources/worker_deployment.md), [WorkerCustomDomain](resources/worker_custom_domain.md), [WorkerEndpoint](resources/worker_endpoint.md), [WorkerCronTrigger](resources/worker_cron_trigger.md)
- [EdgeKVNamespace](resources/edge_kv_namespace.md), [ObjectBucket](resources/edge_object_bucket.md), [SQLiteDatabase](resources/sqlite_database.md), [SQLiteMigrationSet](resources/sqlite_migration_set.md), [SQLiteMigrationApplication](resources/sqlite_migration_application.md)
- [AtLeastOnceQueue](resources/at_least_once_queue.md), [QueueConsumer](resources/queue_consumer.md), [DurableWorkflow](resources/durable_workflow.md), [ActorNamespace](resources/actor_namespace.md)

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
git checkout --detach v4.0.0
```

A source tag, documentation page, or local build alone is not
Registry publication or installation evidence.

Version and migration history are kept in `website/docs/versions.md`, the
[v2-to-v3 guide](../release/migrations/v2-to-v3.md), and the
[publisher-set v3-to-v4 guide](../release/migrations/v3-to-v4.md).

## Native composition with other providers

AWS, Cloudflare, Kubernetes, PostgreSQL, and other providers are peers, not
Takoform implementation details. A module declares any number of provider
sources in `required_providers` and connects their resources through the native
OpenTofu graph. Takoform does not create wrapper Forms, proxy their credentials,
or maintain a central provider catalog. See the
[composition example](../examples/native-provider-composition/main.tf).

## Host requirements

Before applying, confirm that the configured Host advertises support for every
exact FormRef in the plan. Executable compatibility checks are listed in
[Conformance](../conformance/README.md).
