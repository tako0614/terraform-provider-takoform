---
page_title: "Takoform provider"
description: "Published provider v2.0.0 retained resources, the Edge Platform Family source lane, and provider v1 Legacy recovery"
---

# Takoform provider

Takoform is an **Experimental specification project** with three lanes. The
frozen Legacy Epoch is `forms.takoform.com/v1alpha1`. The
`forms.takoform.com/v1alpha2` epoch is retained as the provider-v2
compatibility surface; its packages keep the retained
`packages.forms.takoform.com/v1alpha3` envelope. Current design work is the
Edge Platform Family (`edge.forms.takoform.com/v1alpha1`) on the
`forms.takoform.com/v1alpha3` Host API lane, whose packages use
`packages.forms.takoform.com/v1alpha4`.

Provider v2 discovers its retained provider-v2 Host API wire at
`/.well-known/takoform/v1alpha2`; the v1alpha3 lane discovers at its own
path, `/.well-known/takoform/v1alpha3`. The unversioned well-known path is
retained only for provider-v1 Legacy compatibility.

The 34 published Form Package identities from v1alpha1 are immutable **Legacy**
evidence. There is no current central approval or admission. The nine `0.1.0`
v1alpha2 candidates are retained provider-v2 preview candidates, exposed by
the published provider `v2.0.0`, with real Takosumi Cloud implementations;
Cloud hosting is evidence, not maturity authority. The Edge Platform Family
ships as unpublished provider `v2.1.0` source.

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

Provider `v2.0.0` is the published current client. Terraform and OpenTofu can
install this exact pin from the canonical Registry address:

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
  endpoint = "https://forms.example.com"
  space    = "prod"
}
```

Provider v2 rejects provider-v1 state rather than guessing a migration. Pin v1
to recover or remove Legacy resources; create/import the v1alpha2 resource
explicitly when migration is intended. See [the migration boundary](../release/migrations/v1-to-v2.md).

## Verify the published current provider

Availability is verified, not declared by this immutable documentation.

```console
curl -fsS https://registry.terraform.io/v1/providers/tako0614/takoform/versions
git clone https://github.com/tako0614/terraform-provider-takoform.git
cd terraform-provider-takoform
git checkout --detach v2.0.0
```

A source tag, documentation page, or local build alone is not Registry publication or installation evidence.

## Retained v2 resources

Published provider v2.0.0 exposes exactly these nine retained candidates.
Takosumi Cloud provides all nine resource implementations:

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

## Edge Platform Family (v1alpha3 lane)

The `edge.forms.takoform.com/v1alpha1` Form Family rides the Host API
v1alpha3 lane, discovered at `/.well-known/takoform/v1alpha3`. Its typed
resources are source candidates and require provider v2.1.0 or later (not
yet published); the retained v2 resources above are unaffected:

- [ModuleWorker](resources/module_worker.md)
- [WorkerBundle](resources/worker_bundle.md)
- [WorkerVersion](resources/worker_version.md)
- [WorkerDeployment](resources/worker_deployment.md)
- [WorkerCustomDomain](resources/worker_custom_domain.md)
- [WorkerCronTrigger](resources/worker_cron_trigger.md)
- [EdgeKVNamespace](resources/edge_kv_namespace.md)
- [ObjectBucket (edge)](resources/edge_object_bucket.md)
- [SQLiteDatabase](resources/sqlite_database.md)
- [AtLeastOnceQueue](resources/at_least_once_queue.md)
- [QueueConsumer](resources/queue_consumer.md)

The Edge `ObjectBucket` registers as `takoform_edge_object_bucket` while the
retained v2 lane still owns `takoform_object_bucket`.

The generic [takoform_resource](resources/resource.md) carries any third-party
v1alpha3 Form by exact FormRef. It compiles no schema locally — the host
validates the desired spec against the exact Form — and `terraform import` is
not supported for it, because an import ID cannot supply the exact FormRef.

Takosumi Cloud separately offers `VerifiedDomain` and `AIGateway` as Cloud
services; those two are not Forms and do not appear in this provider
inventory.

## Authority boundary

- Takoform owns Form schemas, immutable identities, lifecycle vocabulary, and
  portable conformance tooling.
- A host owns installation, executable support, activation, placement,
  credentials, and recovery policy.
- Takosumi Cloud owns its managed implementations, capacity, billing, quota,
  SLA, and service catalog.
- Provider release, Form publication, Form maturity, and host availability are
  independent facts.
