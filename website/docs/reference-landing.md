---
title: Reference landing
description: Find the current Takoform Provider resources by Form family.
---

# Reference landing

Use this index to find the Provider resource that matches an exact Form. The
resource pages are generated from the Provider schema and are the authority
for Terraform fields, defaults, import, and lifecycle details.

The current Provider `3.0.0` maps 31 Experimental Forms across eight
versionless families. The resource name is Provider metadata; it is not a Form
identity or a new version stream.

## Edge family

- [module worker](/docs/resources/module_worker.html)
- [worker bundle](/docs/resources/worker_bundle.html)
- [static asset bundle](/docs/resources/static_asset_bundle.html)
- [worker version](/docs/resources/worker_version.html)
- [worker deployment](/docs/resources/worker_deployment.html)
- [worker custom domain](/docs/resources/worker_custom_domain.html)
- [worker endpoint](/docs/resources/worker_endpoint.html)
- [worker cron trigger](/docs/resources/worker_cron_trigger.html)
- [edge KV namespace](/docs/resources/edge_kv_namespace.html)
- [SQLite database](/docs/resources/sqlite_database.html)
- [SQLite migration set](/docs/resources/sqlite_migration_set.html)
- [SQLite migration application](/docs/resources/sqlite_migration_application.html)
- [at-least-once queue](/docs/resources/at_least_once_queue.html)
- [queue consumer](/docs/resources/queue_consumer.html)
- [durable workflow](/docs/resources/durable_workflow.html)
- [actor namespace](/docs/resources/actor_namespace.html)

## Function family

- [function](/docs/resources/function.html)
- [function version](/docs/resources/function_version.html)
- [function deployment](/docs/resources/function_deployment.html)
- [function endpoint](/docs/resources/function_endpoint.html)

## Container family

- [serverless container service](/docs/resources/serverless_container_service.html)
- [container revision](/docs/resources/container_revision.html)
- [container traffic](/docs/resources/container_traffic.html)
- [container endpoint](/docs/resources/container_endpoint.html)
- [container custom domain](/docs/resources/container_custom_domain.html)

## Table, queue, topic, schedule, and vector families

- [table](/docs/resources/table.html)
- [pull queue](/docs/resources/pull_queue.html)
- [topic](/docs/resources/topic.html)
- [topic subscription](/docs/resources/topic_subscription.html)
- [message schedule](/docs/resources/message_schedule.html)
- [dense vector index](/docs/resources/dense_vector_index.html)

## Read a resource page

1. Confirm the Form's `formId` and `definitionVersion` in the host support profile.
2. Read the resource's shape and lifecycle before filling in fields.
3. Keep host-owned endpoint, space, credentials, and placement outside the Form declaration.
4. Pin the Provider SemVer and inspect the plan before applying.

For the compatibility boundary, see the [version model](/docs/versions.html).
For ownership, see [who decides what](/docs/ownership.html).

<StatusNote />
