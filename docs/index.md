---
page_title: "Takoform provider"
description: "Published provider v2.1.1, published v2.0.0 compatibility predecessor, and provider v1 Legacy recovery"
---

# Takoform provider

Takoform is an **Experimental specification project** with three lanes. The
frozen Legacy Epoch is `forms.takoform.com/v1alpha1`. The
`forms.takoform.com/v1alpha2` epoch is retained as the provider-v2
compatibility surface; its packages keep the retained
`packages.forms.takoform.com/v1alpha3` envelope. Current design work is the
Beta Edge Platform Family (`edge.forms.takoform.com/v1beta1`) on the
`forms.takoform.com/v1beta1` Host API channel, whose packages use
`packages.forms.takoform.com/v1alpha4`.

Provider v2 discovers its retained provider-v2 Host API wire at
`/.well-known/takoform/v1alpha2`; the Beta channel discovers at its own
path, `/.well-known/takoform/v1beta1`. The unversioned well-known path is
retained only for provider-v1 Legacy compatibility.

The 34 published Form Package identities from v1alpha1 are immutable **Legacy**
evidence. There is no current central approval or admission. Provider `v2.1.1`
is the current Registry-published provider for the Beta lane and carries the
exact 15 Experimental Edge Forms. Provider `v2.0.0` remains the published
compatibility predecessor for the retained nine `0.1.0` v1alpha2 candidates;
using either lane requires a compatible host. This repository does not assert a
hosted service's live availability. The stable `v2.1.1` release target's source
descriptor remains `candidate-only` metadata after owner publication. The 15
Beta Form Packages remain unpublished.

## Choose the provider line

Use published provider `v1.0.3` only for Legacy FormRefs and existing v1 state:

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

Provider `v2.1.1` is the current published client. Terraform and OpenTofu can
install this exact pin from the canonical Registry address:

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

Provider v2 rejects provider-v1 state rather than guessing a migration. Pin v1
to recover or remove Legacy resources; create/import the v1alpha2 resource
explicitly when migration is intended. See [the migration boundary](../release/migrations/v1-to-v2.md).

Provider `v2.0.0` remains published as the compatibility predecessor for the
retained v1alpha2 lane. Keep this exact pin when reproducing that predecessor's
client identity:

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

## Verify the published current provider

Availability is verified, not declared by this immutable documentation.

```console
curl -fsS https://registry.terraform.io/v1/providers/tako0614/takoform/versions
git clone https://github.com/tako0614/terraform-provider-takoform.git
cd terraform-provider-takoform
git checkout --detach v2.1.1
```

A source tag, documentation page, or local build alone is not Registry publication or installation evidence.

## Retained v2 resources

Published provider v2.1.1 carries exactly these nine retained v1alpha2
candidates alongside the Beta family. Provider v2.0.0 is the published
compatibility predecessor. A compatible host must independently advertise the
exact FormRef it supports:

- [EdgeWorker](resources/edge_worker.md)
- [RelationalDatabase](resources/relational_database.md)
- [ObjectBucket](resources/object_bucket.md)
- [KeyValueStore](resources/key_value_store.md)
- [Queue](resources/queue.md)
- [Schedule](resources/schedule.md)
- [ContainerService](resources/container_service.md)
- [StatefulEntity](resources/stateful_entity.md)
- [VectorIndex](resources/vector_index.md)

The read-only [Interface data source](data-sources/interface.md) resolves
host-materialized declarations. An Interface declaration grants no
authorization and carries no credential.

## Edge Platform Family (v1beta1)

The `edge.forms.takoform.com/v1beta1` Form Family rides the Beta Host API,
discovered at `/.well-known/takoform/v1beta1`. Its 15 typed resources are
Experimental `0.1.0` Forms and require the published provider v2.1.1 or later.
The provider release descriptor remains `candidate-only` metadata. The Form
Package v1alpha4 artifacts remain unpublished, and the retained v2 resources
above are unaffected:

- [ModuleWorker](resources/module_worker.md)
- [WorkerBundle](resources/worker_bundle.md)
- [StaticAssetBundle](resources/static_asset_bundle.md)
- [WorkerVersion](resources/worker_version.md)
- [WorkerDeployment](resources/worker_deployment.md)
- [WorkerCustomDomain](resources/worker_custom_domain.md)
- [WorkerEndpoint](resources/worker_endpoint.md)
- [WorkerCronTrigger](resources/worker_cron_trigger.md)
- [EdgeKVNamespace](resources/edge_kv_namespace.md)
- [ObjectBucket (edge)](resources/edge_object_bucket.md)
- [SQLiteDatabase](resources/sqlite_database.md)
- [SQLiteMigrationSet](resources/sqlite_migration_set.md)
- [SQLiteMigrationApplication](resources/sqlite_migration_application.md)
- [AtLeastOnceQueue](resources/at_least_once_queue.md)
- [QueueConsumer](resources/queue_consumer.md)

The Edge `ObjectBucket` registers as `takoform_edge_object_bucket` while the
retained v2 lane still owns `takoform_object_bucket`.

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
