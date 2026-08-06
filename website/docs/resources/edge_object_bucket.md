---
page_title: "takoform_edge_object_bucket Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Object Bucket (edge.forms.takoform.com/v1alpha1, role identity).
---

# takoform_edge_object_bucket

Flat-namespace object store with read-after-write consistency, exactly as fixed by the edge.objects Interface. Operating rules such as CORS, lifecycle, and lock are separate policy resources, never desired fields of the bucket identity.

This is an `identity` resource: a long-lived logical identity with a stable name, updated in place.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_edge_object_bucket/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
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

- `edge.objects@1.0.0` — the exact Interface contract this Form's service exposes.

## Import

```console
terraform import takoform_edge_object_bucket.example NAME
terraform import takoform_edge_object_bucket.example SPACE/NAME
```
