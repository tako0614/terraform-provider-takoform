---
page_title: "takoform_sqlite_database Resource - takoform"
subcategory: "tako0614 Forms"
description: |-
  SQLite Database (edge.forms.takoform.com, role identity).
---

# takoform_sqlite_database

Embedded SQLite database with bounded EdgeSqlValue values, rollback-only queries, and serializable all-or-none transactions, exactly as fixed by the edge.sql Interface. SQLite semantics are the identity: a database with different SQL, typing, or isolation behavior is a different Form, never an engine token. Numbers stay finite binary64 values inside Number.MAX_SAFE_INTEGER and do not expose the INTEGER/REAL storage-class distinction; BLOB uses the common canonical encoded-bytes object. Runtime SQL cannot migrate schema; SQLiteMigrationApplication owns that administrative path (decision 0034).

This is an `identity` resource: a long-lived logical identity with a stable name, updated in place.

This page documents the publisher-set next-major Provider mapping; its exact FormRef `edge.forms.takoform.com/SQLiteDatabase` is recorded below.
The resource type is Provider metadata. The published Edge Form Definition is maintained in the [takoform-forms source](https://github.com/tako0614/takoform-forms/blob/3231633605b737ce5279d7fc020b4780568e7091/forms/candidates/edge.forms.takoform.com/sqlite-database/definition.json).
See the complete exact identity and the [complete example](https://takoform.com/examples/resources/takoform_sqlite_database/resource.tf).

## Exact FormRef

This Provider mapping carries the following exact four-field FormRef:

```json
{
  "apiVersion": "edge.forms.takoform.com",
  "kind": "SQLiteDatabase",
  "definitionVersion": "0.1.0",
  "schemaDigest": "sha256:c72eeb66ef96c4679b5c724fa1219d71c89bb7eeb9e543d73d868ec41bddddfe"
}
```

`packageDigest` — Form Package digest (separate from FormRef; embedded Provider provenance): `sha256:0d3cce162550bb956ed19ce72b5eb1aeca52d78e7ca80e868d646fd580deef84`.

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `create_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `30m`). There is no `update_timeout`: this Form declares no update capability. Changing only these provider-side timeouts is applied in place without any host call.

## Read-only attributes

- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.
- `generation` — desired-state generation; increments only when the portable desired spec changes; this Form declares no update capability — every desired attribute forces replacement instead. It is also the DELETE fence, because a delete withdraws desired state like any other desired-state mutation.
- `revision` — representation revision; increments whenever the representation changes — a spec-changing update, new status, new outputs, or a change to another resource this one is rendered from. It is the strong ETag, and it is deliberately NOT the delete fence: a teardown removes dependents first and would otherwise be refused by a revision it moved itself.
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
- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes replacing this resource, because this Form declares no in-place update and a host refuses every apply to the existing one. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.
- `pending_operation_id` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.

## State continuity

- **Reads dispatch on the recorded FormRef.** `SQLiteDatabase` state is addressed under the
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

## Provided interfaces

- `edge.sql@1.0.0` — the exact Interface contract this Form's service exposes.

## Import

```console
terraform import takoform_sqlite_database.example NAME
terraform import takoform_sqlite_database.example SPACE/NAME
```

Both short forms bind state to this provider build's default create
FormRef. To adopt a resource created under an EARLIER definition version of
this Form, name the exact identity instead. The import ID is then one JSON
object — not a delimiter-joined string, because a SpaceID is opaque UTF-8
whose only forbidden character is `/`, so no separator can escape it safely:

```console
terraform import takoform_sqlite_database.example \
  '{"space":"prod","apiVersion":"edge.forms.takoform.com","kind":"SQLiteDatabase","definitionVersion":"0.1.0","schemaDigest":"sha256:…","name":"…"}'
```

`space` is optional and falls back to the provider default; the four FormRef
members are all-or-nothing. An identity this provider build carries no codec
for is refused, naming the identities it does carry — it is never silently
rebound to the default.
