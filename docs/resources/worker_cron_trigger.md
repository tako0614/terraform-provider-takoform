---
page_title: "takoform_worker_cron_trigger Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Worker Cron Trigger (edge.forms.takoform.com/v1alpha1, role attachment).
---

# takoform_worker_cron_trigger

Attaches one cron schedule to a Module Worker, invoking its scheduled handler at each match. Schedules are interpreted in UTC only; there is no timezone field, so two hosts can never fire the same trigger at different instants and no schedule ever skips or repeats an hour for a daylight-saving transition. The grammar is five fields separated by single spaces — minute 0-59, hour 0-23, day-of-month 1-31, month 1-12, day-of-week 0-6 with 0 Sunday — and each field is a comma-separated list of `*`, a literal, a range `low-high`, `*/step`, or `low-high/step`. Names and a step on a bare literal are not accepted, and neither is any value outside its field's own range, an inverted range, or a step outside 1..span. When day-of-month and day-of-week are BOTH restricted the trigger fires on a day either of them selects; when only one is restricted only that one constrains the day. A missed run is not made up: a host that could not fire a match — because it was unavailable, or because the previous invocation was still running — skips it rather than firing late, so a schedule never produces a backlog. At-least-once delivery applies to each match: a handler may be invoked more than once for one matched minute, and it must be idempotent. An uncaught exception in the handler is a failed invocation reported to host diagnostics; it is not retried within the matched minute and it never becomes an HTTP response.

This is an `attachment` resource: it connects a parent to inward activation (routes, domains, schedules, queue consumption). Deleting the attachment never deletes the parent.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_worker_cron_trigger/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `worker` (String, required, forces replacement) — Module Worker whose scheduled handler this trigger invokes. Set the name of the target `ModuleWorker` resource.
- `cron` (String, required) — UTC cron schedule. Five fields separated by single spaces — minute `0`-`59`, hour `0`-`23`, day-of-month `1`-`31`, month `1`-`12`, and day-of-week `0`-`6` with `0` Sunday — where each field is a comma-separated list of `*`, a literal, a range `low-high`, `*/step`, or `low-high/step`. Month and day names, and a step on a bare literal such as `5/10`, are not accepted. The provider parses the expression at plan time exactly as the host does, so a value outside its field's range, an inverted range such as `5-1`, or a step outside `1`..span is a plan-time error rather than a failed apply. When day-of-month and day-of-week are both restricted the schedule fires on a day either selects.
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `create_timeout` / `update_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `20m` / `30m`).

## Read-only attributes

- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.
- `generation` — desired-state generation; increments only when the portable desired spec changes. Updates fence on it.
- `revision` — representation revision; increments whenever the representation changes — a spec-changing update, new status, or new outputs. Deletes fence on it via `If-Match`.
- `conditions` — the complete status condition list the host reports, in its order. Each entry carries
  `type` (the closed `Ready` / `Reconciling` / `Degraded` / `Drifted` / `Blocked` / `Deleting` vocabulary),
  `status` (`True` / `False` / `Unknown`), the closed portable `reason`, an optional `message`, an optional
  non-portable `host_reason` naming exactly what is wrong, the `observed_generation` the status reflects,
  and `last_transition_time`. Conditions are host-rendered state: they change when this resource changes
  AND when a resource it depends on changes, with no desired spec changing anywhere, so they are read-only
  and a configuration must not assert them.
- `ready` — derived convenience: true when `conditions` carries the closed `Ready` condition with status
  `True`. Read `conditions` for the reason it is not.
- `outputs_json` — the WHOLE `status.outputs` document, JSON-serialized. This Form declares no `outputSchema`, so a conforming host omits `status.outputs` entirely and this
  attribute is `"{}"`. It stays declared because a host may publish a value no contract describes, and
  an undescribed value must still be reachable rather than silently dropped.
- `form_api_version`, `form_kind`, `form_definition_version`, `form_schema_digest` — the exact immutable FormRef this state is bound to; reads dispatch on it.
- `form_package_digest` — audit-only package provenance; never part of resource identity, queries, or fences.
- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes an in-place re-apply of the same desired state, which is all a host needs to re-resolve and re-pin every reference. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.
- `pending_operation_id` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.

## State continuity

- **Reads dispatch on the recorded FormRef.** `WorkerCronTrigger` state is addressed under the
  exact `form_*` identity it records, not under this build's default create ref, so a
  resource created before the Form line advanced stays addressable as itself. An identity
  this provider build carries no codec for is a hard error naming that identity and the
  ones the build does carry; the provider never substitutes another exact FormRef, because
  a substituted query's "not found" is indistinguishable from deletion.
- **A changed `uid` is an error, and state is kept.** When the host serves a different
  `uid` under the recorded name, the resource this state was applied against is gone and
  something re-used its name. The provider reports a hard error naming both uids and keeps
  the resource in state. It does not re-bind — that would adopt a resource you never
  applied — and it does not remove state, which would make the next apply fail against the
  resource that does exist, with no plan left to repair it. Resolve it by importing the new
  incarnation explicitly, restoring the prior one, or deleting the host-side replacement.
- **An unfinished mutation is resumed, not re-created.** When `pending_operation_id` is
  set, a refresh asks the host about that operation before it reads the resource. While the
  operation is still running the resource may legitimately not exist yet, so its absence is
  not treated as deletion and the marker survives; a terminal success is verified against
  the exact identity and settles state; a terminal failure or an expired operation record
  defers to an exact read of the resource, which decides. Refresh again once the host
  settles.

## Import

```console
terraform import takoform_worker_cron_trigger.example NAME
terraform import takoform_worker_cron_trigger.example SPACE/NAME
```

Both short forms bind state to this provider build's default create
FormRef. To adopt a resource created under an EARLIER definition version of
this Form, name the exact identity instead. The import ID is then one JSON
object — not a delimiter-joined string, because a SpaceID is opaque UTF-8
whose only forbidden character is `/`, so no separator can escape it safely:

```console
terraform import takoform_worker_cron_trigger.example \
  '{"space":"prod","apiVersion":"edge.forms.takoform.com/v1alpha1","kind":"WorkerCronTrigger","definitionVersion":"0.1.0","schemaDigest":"sha256:…","name":"…"}'
```

`space` is optional and falls back to the provider default; the four FormRef
members are all-or-nothing. An identity this provider build carries no codec
for is refused, naming the identities it does carry — it is never silently
rebound to the default.
