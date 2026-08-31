# Documentation

This is the current documentation entry point for the stable Host API v1 and
the publisher's official Form corpus: one versionless family,
`edge.forms.takoform.com`, with 16 exact Experimental Forms. The source roster
is pinned to [`takoform-forms` commit
`3a395e4`](https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e)
and records 7 Interfaces and 6 Bindings.

## Current normative contracts

The current contract surface is organized by the Host API and the Form/Core
contracts, not by numbered Specification receipts:

- [Host API v1](/spec/host-api/v1.html) — discovery, exact Form availability,
  operations, identity, fencing, and errors.
- [Form Definition](/spec/form-definition/) — the data-only contract for one
  exact Form.
- [Form Package](/spec/form-package/) — the data envelope used to distribute
  Form Definitions and fixtures.
- [Interface contracts](/spec/interface-contract/) — exact capabilities a
  Form exposes.
- [Binding contracts](/spec/binding-contract/) — typed capability use held by
  a revision.
- [Core contracts](/spec/core/) — shared validation, canonicalization, and
  conformance behavior.

Specification 1.0/1.1 receipts are retained archive evidence. They do not make
an API v1.1, a new Form version, or a current release lane.

## Provider boundary and independent version streams

Takoform has four independent version streams: Host API, each Form definition,
Core/library software, and Provider software. The Form Package API identifier
(`packages.forms.takoform.com/v1alpha5`) names a wire/envelope format; it is not
an additional product version axis.

The canonical `registry.terraform.io/tako0614/takoform` Provider is an
official-Forms-only tool. It has typed mappings only for Forms it explicitly
supports; it is not a generic carrier or a universal infrastructure provider.
Third-party Form packages use the same Host API path and package/verification
contracts under their own namespaces, with their own explicit Provider
mappings. A Terraform/OpenTofu module may combine the official Takoform
Provider with other Takoform or industry-standard Providers.

Provider `3.0.0` is the released Registry implementation. Its immutable
8-family/31-resource projection is historical Provider metadata, not the
publisher's current Form corpus. The official-only Edge16 Provider direction
is a next-major candidate and is not published; this page makes no install
claim for that future version. Examples that require `>=3.1.0` are likewise
candidate/unpublished until a release record says otherwise.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = ">= 3.0.0"
    }
    aws = {
      source = "hashicorp/aws"
    }
  }
}
```

## Current Edge reference family (16 Experimental Forms)

Host API v1 carries versionless Form Families. The current Edge family has
exactly these 16 Forms:

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

The worker shape is a chain of immutable identity, bundle, version,
deployment, and attachment resources. Capability use is through typed
Interface and Binding contracts; inward activation remains a separate
attachment.

## Deferred candidate resource source

The repository retains these non-Edge candidate resource pages as historical
source. They are **deferred**, not current Forms or current Host support, and
they stay out of the Current navigation:

- Function: [`takoform_function`](/docs/resources/function.html),
  [`takoform_function_version`](/docs/resources/function_version.html),
  [`takoform_function_deployment`](/docs/resources/function_deployment.html),
  [`takoform_function_endpoint`](/docs/resources/function_endpoint.html)
- Container: [`takoform_serverless_container_service`](/docs/resources/serverless_container_service.html),
  [`takoform_container_revision`](/docs/resources/container_revision.html),
  [`takoform_container_traffic`](/docs/resources/container_traffic.html),
  [`takoform_container_endpoint`](/docs/resources/container_endpoint.html),
  [`takoform_container_custom_domain`](/docs/resources/container_custom_domain.html)
- Table and pull queue: [`takoform_table`](/docs/resources/table.html),
  [`takoform_pull_queue`](/docs/resources/pull_queue.html)
- Topic: [`takoform_topic`](/docs/resources/topic.html),
  [`takoform_topic_subscription`](/docs/resources/topic_subscription.html)
- Schedule and vector: [`takoform_message_schedule`](/docs/resources/message_schedule.html),
  [`takoform_dense_vector_index`](/docs/resources/dense_vector_index.html)

Keeping these links preserves immutable historical source without presenting
Function, Container, Table, Queue, Topic, Schedule, or Vector as current.

## Historical compatibility

Withdrawn Host API epochs, Specification receipts, and Provider 1/2 releases
remain available under exact pins for recovery and migration. Read the
[historical release records](/release/) and the
[v2 to v3 migration boundary](/release/migrations/v2-to-v3.html) for those
identities; neither is a current version lane.

<StatusNote />
