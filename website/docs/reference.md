---
page_title: "Takoform provider"
description: "Published provider v2.1.1 for the Beta Host API and the 15 Experimental Edge Platform Forms"
---

# Takoform provider

Takoform is an **Experimental specification project** with one current epoch:
the Beta Edge Platform Family (`edge.forms.takoform.com/v1beta1`) on the
`forms.takoform.com/v1beta1` Host API channel, whose packages use the
`packages.forms.takoform.com/v1alpha4` envelope. The provider discovers the
Beta wire at `/.well-known/takoform/v1beta1`.

The two pre-Beta epochs (`forms.takoform.com/v1alpha1` Legacy and the
`forms.takoform.com/v1alpha2` provider-v2 epoch) were withdrawn
([decision 0042](../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md)).
Published provider releases that carried them remain immutable Registry
history; the withdrawn resources have no successors in this documentation, and
[the migration boundary](../release/migrations/v2-to-v3.md) says what existing
state does.

Provider `v2.1.1` is the current Registry-published provider and carries the
exact 15 Experimental Edge Forms. Using them requires a compatible host; this
repository does not assert a hosted service's live availability. The stable
`v2.1.1` release target's source descriptor remains `candidate-only` metadata
after owner publication. The 15 Beta Form Packages remain unpublished, and the
next release published from this repository will be a major, `3.0.0`.

## Install the provider

Terraform and OpenTofu can install this exact pin from the canonical Registry
address:

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
  endpoint = "https://forms.example.com"
  space    = "prod"
}
```

`endpoint`, `space`, and bearer `token` may instead come from
`TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.

## Verify the published current provider

Availability is verified, not declared by this immutable documentation.

```console
curl -fsS https://registry.terraform.io/v1/providers/tako0614/takoform/versions
git clone https://github.com/tako0614/terraform-provider-takoform.git
cd terraform-provider-takoform
git checkout --detach v2.1.1
```

A source tag, documentation page, or local build alone is not Registry publication or installation evidence.

## Edge Platform Family (v1beta1)

The `edge.forms.takoform.com/v1beta1` Form Family rides the Beta Host API,
discovered at `/.well-known/takoform/v1beta1`. Its 15 typed resources are
Experimental `0.1.0` Forms:

- [ModuleWorker](resources/module_worker.md)
- [WorkerBundle](resources/worker_bundle.md)
- [StaticAssetBundle](resources/static_asset_bundle.md)
- [WorkerVersion](resources/worker_version.md)
- [WorkerDeployment](resources/worker_deployment.md)
- [WorkerCustomDomain](resources/worker_custom_domain.md)
- [WorkerEndpoint](resources/worker_endpoint.md)
- [WorkerCronTrigger](resources/worker_cron_trigger.md)
- [EdgeKVNamespace](resources/edge_kv_namespace.md)
- [ObjectBucket](resources/edge_object_bucket.md)
- [SQLiteDatabase](resources/sqlite_database.md)
- [SQLiteMigrationSet](resources/sqlite_migration_set.md)
- [SQLiteMigrationApplication](resources/sqlite_migration_application.md)
- [AtLeastOnceQueue](resources/at_least_once_queue.md)
- [QueueConsumer](resources/queue_consumer.md)

The Edge `ObjectBucket` registers as `takoform_edge_object_bucket`; the bare
`takoform_object_bucket` resource type belongs to the withdrawn v1alpha2 lane
and is deliberately not reused.

These are the whole v1beta1 surface. The provider exposes no generic carrier
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
