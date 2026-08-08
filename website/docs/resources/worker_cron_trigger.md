---
page_title: "takoform_worker_cron_trigger Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Worker Cron Trigger (edge.forms.takoform.com/v1alpha1, role attachment).
---

# takoform_worker_cron_trigger

Attaches one cron schedule to a Module Worker, invoking its scheduled handler at each match. Schedules are interpreted in UTC only; there is no timezone field, so two hosts can never fire the same trigger at different instants. The accepted grammar is exactly five single-value fields separated by single spaces: minute is a literal 0-59 and hour a literal 0-23, day-of-month is `*` or 1-31, month is `*` or 1-12, and day-of-week is `*` or 0-6. Ranges, lists, steps such as `*/5`, names, and `*` in the minute or hour field are all rejected, so the most frequent representable schedule is once per day at one fixed UTC time. Hourly and sub-hourly schedules are not expressible and need a future grammar revision, which is a new definition version of this Form.

This is an `attachment` resource: it connects a parent to inward activation (routes, domains, schedules, queue consumption). Deleting the attachment never deletes the parent.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_worker_cron_trigger/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `worker` (String, required, forces replacement) — Module Worker whose scheduled handler this trigger invokes. Set the name of the target `ModuleWorker` resource.
- `cron` (String, required) — UTC cron schedule in the enforced closed grammar: exactly five single-value fields separated by single spaces — minute `0`-`59` and hour `0`-`23` are literal numbers (`*` is not accepted there), day-of-month is `*` or `1`-`31`, month is `*` or `1`-`12`, and day-of-week is `*` or `0`-`6`. Ranges, lists, steps (such as `*/5`), and names are not accepted.
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
terraform import takoform_worker_cron_trigger.example NAME
terraform import takoform_worker_cron_trigger.example SPACE/NAME
```
