---
page_title: "takoform_module_worker Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Module Worker (edge.forms.takoform.com/v1alpha1, role identity).
---

# takoform_module_worker

Long-lived logical identity of one ES Module Worker application. The Form fixes the ES module worker ABI by identity: handlers are exported module functions receiving typed events and a binding environment. Code, configuration, and bindings live on Worker Version revisions; traffic selection lives on Worker Deployments.

This is an `identity` resource: a long-lived logical identity with a stable name, updated in place.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_module_worker/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `create_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `30m`). There is no `update_timeout`: this Form declares no update capability. Changing only these provider-side timeouts is applied in place without any host call.

## Read-only attributes

- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.
- `generation` — desired-state generation; increments only when the portable desired spec changes; this Form declares no update capability — every desired attribute forces replacement instead.
- `revision` — representation revision; increments whenever the representation changes — a spec-changing update, new status, or new outputs. Deletes fence on it via `If-Match`.
- `ready` — true when the closed `Ready` condition reports `True`.
- `outputs_json` — JSON-serialized `status.outputs` document (`"{}"` when empty).
- `form_api_version`, `form_kind`, `form_definition_version`, `form_schema_digest` — the exact immutable FormRef this state is bound to; reads dispatch on it.
- `form_package_digest` — audit-only package provenance; never part of resource identity, queries, or fences.

## Import

```console
terraform import takoform_module_worker.example NAME
terraform import takoform_module_worker.example SPACE/NAME
```
