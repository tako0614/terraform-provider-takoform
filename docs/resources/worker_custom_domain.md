---
page_title: "takoform_worker_custom_domain Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Worker Custom Domain (edge.forms.takoform.com/v1alpha1, role attachment).
---

# takoform_worker_custom_domain

Attaches one DNS hostname to a Module Worker so its active deployment serves that hostname over HTTPS. Inward activation is an attachment, never a binding; deleting the attachment detaches the hostname and never deletes the worker.

This is an `attachment` resource: it connects a parent to inward activation (routes, domains, schedules, queue consumption). Deleting the attachment never deletes the parent.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_worker_custom_domain/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `worker` (String, required, forces replacement) — Module Worker served on this hostname. Set the name of the target `ModuleWorker` resource.
- `hostname` (String, required, forces replacement) — Dotted DNS hostname this attachment serves. Changing it replaces the attachment.
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
- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes replacing this resource, because this Form declares no in-place update and a host refuses every apply to the existing one. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.

## Import

```console
terraform import takoform_worker_custom_domain.example NAME
terraform import takoform_worker_custom_domain.example SPACE/NAME
```
