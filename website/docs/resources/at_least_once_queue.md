---
page_title: "takoform_at_least_once_queue Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  At-Least-Once Queue (edge.forms.takoform.com/v1alpha1, role identity).
---

# takoform_at_least_once_queue

Message queue with at-least-once delivery and no ordering guarantee, exactly as fixed by the edge.queue Interface. There is no ordering field: a FIFO queue is a different Form.

This is an `identity` resource: a long-lived logical identity with a stable name, updated in place.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_at_least_once_queue/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `message_retention_seconds` (Number, required) — How long an undelivered message is retained before it is dropped, in seconds. Between 60 and 1209600.
- `delivery_delay_seconds` (Number, optional) — Default delay before a sent message becomes deliverable, in seconds. Omitting it delivers immediately. Between 0 and 43200. Defaults to `0`.
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

## Provided interfaces

- `edge.queue@1.0.0` — the exact Interface contract this Form's service exposes.

## Import

```console
terraform import takoform_at_least_once_queue.example NAME
terraform import takoform_at_least_once_queue.example SPACE/NAME
```
