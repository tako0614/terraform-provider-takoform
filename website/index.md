---
layout: home

hero:
  name: Takoform
  text: One provider. Dependent on none.
  tagline: Portable, host-neutral resource contracts for Terraform and OpenTofu.
  actions:
    - theme: brand
      text: Get started
      link: /docs/
    - theme: alt
      text: Read the spec
      link: /spec/

features:
  - title: Host-neutral
    details: Declare what you need — a worker, a database, a queue, a bucket. Placement, credentials, capacity, and pricing stay with the host.
  - title: Nine resource kinds
    details: EdgeWorker, RelationalDatabase, ObjectBucket, KeyValueStore, Queue, Schedule, ContainerService, StatefulEntity, and VectorIndex.
  - title: Versioned contracts
    details: Every Form is an immutable FormRef — API group, kind, version, and schema digest. Compatibility is never inferred from a name.
  - title: Real hosts
    details: Takosumi Cloud implements all nine kinds and feeds production feedback into the design.
---

## What it looks like

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
  endpoint = "https://host.example.com"
  space    = "prod"
}

resource "takoform_edge_worker" "example" {
  name                = "edge-worker"
  artifact_media_type = "application/vnd.takoform.edge-worker+tar"
  artifact_sha256     = "sha256:0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37"
  artifact_url        = "https://artifacts.portable-conformance.invalid/edge-worker.tar"
  entrypoint          = "worker.mjs"
  runtime             = "javascript"
  runtime_version     = "2026.1"
  configuration       = { "LOG_LEVEL" = "info" }
}
```

No credentials, no placement, no pricing — the host decides those and keeps
them out of your state. Provider `v2.0.0` is the current published client;
`v1.0.3` remains the published Legacy client.
[Start here](/docs/) to install and use it.

## The resources

All nine are `forms.takoform.com/v1alpha2` `0.1.0` candidates.

| Resource | What it declares |
| --- | --- |
| [`takoform_edge_worker`](/docs/resources/edge_worker.html) | a request/event application from a digest-bound artifact |
| [`takoform_relational_database`](/docs/resources/relational_database.html) | a relational database by open engine token |
| [`takoform_object_bucket`](/docs/resources/object_bucket.html) | object storage |
| [`takoform_key_value_store`](/docs/resources/key_value_store.html) | key/value state |
| [`takoform_queue`](/docs/resources/queue.html) | at-least-once message delivery |
| [`takoform_schedule`](/docs/resources/schedule.html) | a cron that invokes one connected resource |
| [`takoform_container_service`](/docs/resources/container_service.html) | a service from an OCI image digest |
| [`takoform_stateful_entity`](/docs/resources/stateful_entity.html) | addressable persistent entities |
| [`takoform_vector_index`](/docs/resources/vector_index.html) | a vector index with fixed dimensions |

Every resource exposes a read-only interface other resources can connect to —
`http.request@1`, `sql.query@1`, `object.storage@1`, `queue.messages@1`, and
more. See the [resource reference](/docs/#resource-reference).

## How it works

1. **Declare** — write the portable fields: name, artifact digest, entrypoint,
   runtime, configuration, connections.
2. **A host implements** — the provider discovers the host at a versioned path
   and drives preview/apply, observe, refresh, and delete. Implementation,
   placement, capacity, credentials, and routing stay with the host.
3. **Connect** — the host publishes the declared interfaces; other resources
   bind to them by name and permissions.

> **Status:** Takoform is an Experimental specification project. Current
> FormRefs use `forms.takoform.com/v1alpha2` and current package envelopes use
> `packages.forms.takoform.com/v1alpha3`. Provider `v2.0.0` is the current
> published client; provider `v1.0.3` is the published Legacy client. The 34
> published Form Package identities from `forms.takoform.com/v1alpha1` are
> immutable Legacy evidence. There is no current central approval or admission.
