---
page_title: "takoform_queue_consumer Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Queue Consumer (edge.forms.takoform.com/v1alpha1, role attachment).
---

# takoform_queue_consumer

Attaches one Module Worker as the batch consumer of one At-Least-Once Queue, invoking its queue handler with message batches and redelivering failed batches. Consumption is inward activation and therefore an attachment, never a binding.

This is an `attachment` resource: it connects a parent to inward activation (routes, domains, schedules, queue consumption). Deleting the attachment never deletes the parent.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_queue_consumer/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `queue` (String, required, forces replacement) — Queue this consumer drains. Changing it replaces the attachment. Set the name of the target `AtLeastOnceQueue` resource.
- `worker` (String, required, forces replacement) — Module Worker whose queue handler receives the batches. Changing it replaces the attachment. Set the name of the target `ModuleWorker` resource.
- `max_batch_size` (Number, required) — Largest number of messages delivered in one batch. Between 1 and 100.
- `max_batch_timeout_seconds` (Number, required) — Longest time the host waits to fill a batch before delivering it, in seconds. Between 0 and 60.
- `max_retries` (Number, required) — How many times a failed batch is redelivered before its messages go to the dead-letter queue or are dropped. Between 0 and 100.
- `retry_delay_seconds` (Number, required) — Delay before a failed batch becomes deliverable again, in seconds. Between 0 and 43200.
- `dead_letter_queue` (String, optional) — Queue receiving messages that exhausted their retries. Without it, exhausted messages are dropped. Set the name of the target `AtLeastOnceQueue` resource.
- `max_concurrency` (Number, required) — Largest number of concurrent batch invocations. Between 1 and 250.
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `create_timeout` / `update_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `20m` / `30m`).

## Read-only attributes

- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.
- `generation` — desired-state generation; increments only when the portable desired spec changes. Updates fence on it.
- `revision` — representation revision; increments whenever the representation changes — a spec-changing update, new status, or new outputs. Deletes fence on it via `If-Match`.
- `ready` — true when the closed `Ready` condition reports `True`.
- `outputs_json` — JSON-serialized `status.outputs` document (`"{}"` when empty).
- `form_api_version`, `form_kind`, `form_definition_version`, `form_schema_digest` — the exact immutable FormRef this state is bound to; reads dispatch on it.
- `form_package_digest` — audit-only package provenance; never part of resource identity, queries, or fences.

## Import

```console
terraform import takoform_queue_consumer.example NAME
terraform import takoform_queue_consumer.example SPACE/NAME
```
