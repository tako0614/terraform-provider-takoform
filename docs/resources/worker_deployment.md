---
page_title: "takoform_worker_deployment Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Worker Deployment (edge.forms.takoform.com/v1alpha1, role deployment).
---

# takoform_worker_deployment

Selects which Worker Versions of one Module Worker serve traffic and in what proportion. Weights are basis points and must sum to exactly 10000 across entries; the sum is host-validated semantics because a schema cannot add weights. Rollback is re-weighting, never mutating a revision.

This is a `deployment` resource: the only mutable path for traffic movement and rollback. It selects which revisions are active.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_worker_deployment/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `worker` (String, required, forces replacement) — Module Worker identity whose traffic this deployment governs. Set the name of the target `ModuleWorker` resource.
- `versions` (List of Object, required) — Active Worker Versions and their traffic weights in basis points. Weights must sum to exactly 10000. Each entry declares `worker_version`, `weight` (between 1 and 10000). The list must declare between 1 and 8 entries.
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
- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes an in-place re-apply of the same desired state, which is all a host needs to re-resolve and re-pin every reference. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.

## Import

```console
terraform import takoform_worker_deployment.example NAME
terraform import takoform_worker_deployment.example SPACE/NAME
```
