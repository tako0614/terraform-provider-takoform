---
page_title: "Takoform provider"
description: "Provider v2 current candidates and provider v1 Legacy recovery"
---

# Takoform provider

Takoform is an **Experimental specification project**. The current
Specification Epoch is `forms.takoform.com/v1alpha2`; the frozen Legacy Epoch
is `forms.takoform.com/v1alpha1`. Current packages use
`packages.forms.takoform.com/v1alpha3`.

Provider v2 discovers the matching current Host API at
`/.well-known/takoform/v1alpha2`. The unversioned well-known path is retained
only for provider-v1 Legacy compatibility.

The 34 published Form Package identities from v1alpha1 are immutable **Legacy**
evidence. There is no current central approval or admission. The current set is
instead nine `0.1.0` publication candidates with real Takosumi Cloud
implementations; Cloud hosting is evidence, not maturity authority.

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

Provider `v2.0.0` is an unpublished source candidate. Its examples require a
reviewed local build and Terraform/OpenTofu development override; this exact
pin must not be mistaken for Registry availability:

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

## Verify the published Legacy provider

Availability is verified, not declared by this immutable documentation.

```console
curl -fsS https://registry.terraform.io/v1/providers/tako0614/takoform/versions
git clone https://github.com/tako0614/terraform-provider-takoform.git
cd terraform-provider-takoform
git checkout --detach v1.0.3
```

A source tag, documentation page, or local build alone is not Registry publication or installation evidence.

## Current resources

Provider v2 exposes exactly these nine current candidates:

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

Takosumi Cloud provides all nine resource implementations. It separately
offers `VerifiedDomain` and `AIGateway` as Cloud services; those two are not
Forms and do not appear in this provider inventory.

## Authority boundary

- Takoform owns Form schemas, immutable identities, lifecycle vocabulary, and
  portable conformance tooling.
- A host owns installation, executable support, activation, placement,
  credentials, and recovery policy.
- Takosumi Cloud owns its managed implementations, capacity, billing, quota,
  SLA, and service catalog.
- Provider release, Form publication, Form maturity, and host availability are
  independent facts.
