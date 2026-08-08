---
page_title: "takoform_worker_version Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Worker Version (edge.forms.takoform.com/v1alpha1, role revision).
---

# takoform_worker_version

Immutable executable snapshot of one Module Worker: a bundle, a runtime compatibility date, declared handlers, non-secret vars, and the typed capability bindings the code may use. A change is a new Worker Version; traffic moves only through Worker Deployments.

This is a `revision` resource: an immutable snapshot. It is create-only — every desired attribute forces replacement, and rollback means pointing a deployment at an earlier revision, never editing this one.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_worker_version/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `worker` (String, required, forces replacement) — Module Worker identity this version belongs to. Set the name of the target `ModuleWorker` resource.
- `bundle` (String, required, forces replacement) — Worker Bundle carrying the exact module bytes this version executes. Set the name of the target `WorkerBundle` resource.
- `compatibility_date` (String, required, forces replacement) — Runtime compatibility date fixing default runtime behavior for this version.
- `compatibility_flags` (Set of String, optional, forces replacement) — Closed runtime compatibility flags enabled for this version. Omitting it enables no flag. One of `nodejs_compat`. Defaults to the empty list `[]`.
- `handlers` (Set of String, required, forces replacement) — Module event handlers this version exports. A host rejects an attachment whose event kind is not declared here. One of `fetch`, `scheduled`, `queue`, `tail`.
- `vars_json` (String, optional, forces replacement) — Non-secret configuration values projected into the module environment. Sensitive material never enters portable state. Omitting it projects no variable. Authored as one JSON object string (for example `jsonencode({...})`); the provider sends the parsed object. Defaults to the empty object `{}`.
- `kv_bindings` (List of Object, optional, forces replacement) — Typed module-worker.edge-kv bindings projecting the edge.kv API under JavaScript identifier names. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `EdgeKVNamespace` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `bucket_bindings` (List of Object, optional, forces replacement) — Typed module-worker.object-bucket bindings projecting the edge.objects API. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `ObjectBucket` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `sqlite_bindings` (List of Object, optional, forces replacement) — Typed module-worker.sqlite bindings projecting the edge.sql API. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `SQLiteDatabase` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `queue_producer_bindings` (List of Object, optional, forces replacement) — Typed module-worker.queue-producer bindings projecting only send and sendBatch. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `AtLeastOnceQueue` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `service_bindings` (List of Object, optional, forces replacement) — Typed module-worker.service bindings projecting worker.service fetch toward another Module Worker. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `ModuleWorker` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `required_sensitive_vars` (Set of String, optional, forces replacement) — Names of sensitive values this version requires the host to supply out-of-band. Only the names are portable state; values travel through each host's own sealed path. Omitting it requires no sensitive value. Defaults to the empty list `[]`.
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

## Accepted bindings

- `module-worker.edge-kv@1.0.0`
- `module-worker.object-bucket@1.0.0`
- `module-worker.sqlite@1.0.0`
- `module-worker.queue-producer@1.0.0`
- `module-worker.service@1.0.0`

Outward capability use is a typed binding held by this revision; inward
activation (routes, domains, cron, queue consumption) is a separate
attachment resource. The two are never merged.

## Import

```console
terraform import takoform_worker_version.example NAME
terraform import takoform_worker_version.example SPACE/NAME
```
