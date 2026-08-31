---
page_title: "takoform_table Resource - takoform"
subcategory: "Historical / deferred candidate Forms"
description: |-
  Table (table.forms.takoform.com, role identity).
---

# takoform_table

> Historical/deferred candidate. This non-Edge Form is retained for source
> inspection and exact historical state only; it is not in the current
> official Edge16 corpus or Current navigation.

Key-addressed document table with a declared partition key, optional sort key, mutable secondary indexes, and optional lazy TTL. The table.document Interface fixes document values, conditional writes, consistent single-item reads, and key-ordered partition queries; this identity carries only the table's addressing declaration.

This is an `identity` resource: a long-lived logical identity with a stable name, updated in place.

This page documents a non-normative historical Provider mapping for the
deferred Experimental candidate `table.forms.takoform.com/Table`.
The mapping name is provider metadata: it is absent from the Form Definition and cannot change
the Form's canonical bytes or digest. Provider publication and support are versioned separately.
The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](https://takoform.com/examples/resources/takoform_table/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `partition_key` (Object, required, forces replacement) — Immutable partition key declaration. Every item must carry this top-level attribute; changing it replaces the table. The object declares `name`, `type`; when the object is present, every member is required.
- `sort_key` (Object, optional, forces replacement) — Optional immutable sort key declaration. When absent, each partition key addresses at most one item; changing it replaces the table. The object declares `name`, `type`; when the object is present, every member is required.
- `secondary_indexes` (List of Object, optional) — Secondary indexes over top-level document attributes. Adding or removing an entry is an in-place table update; an index is redefined only by removing it and adding it later. Omitting this list declares no secondary indexes. Each entry declares `name`, `partition_key`, `sort_key`. Defaults to the empty list `[]`.
- `ttl_attribute` (String, optional) — Optional top-level number attribute used for lazy epoch-second expiry. When absent, the table has no TTL policy; setting or clearing it is an in-place update.
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `create_timeout` / `update_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `20m` / `30m`).

## Read-only attributes

- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.
- `generation` — desired-state generation; increments only when the portable desired spec changes. Updates fence on it. It is also the DELETE fence, because a delete withdraws desired state like any other desired-state mutation.
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
- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes an in-place re-apply of the same desired state, which is all a host needs to re-resolve and re-pin every reference. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.
- `pending_operation_id` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.

## State continuity

- **Reads dispatch on the recorded FormRef.** `Table` state is addressed under the
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

- `table.document@1.0.0` — the exact Interface contract this Form's service exposes.

## Import

```console
terraform import takoform_table.example NAME
terraform import takoform_table.example SPACE/NAME
```

Both short forms bind state to this provider build's default create
FormRef. To adopt a resource created under an EARLIER definition version of
this Form, name the exact identity instead. The import ID is then one JSON
object — not a delimiter-joined string, because a SpaceID is opaque UTF-8
whose only forbidden character is `/`, so no separator can escape it safely:

```console
terraform import takoform_table.example \
  '{"space":"prod","apiVersion":"table.forms.takoform.com","kind":"Table","definitionVersion":"0.1.0","schemaDigest":"sha256:…","name":"…"}'
```

`space` is optional and falls back to the provider default; the four FormRef
members are all-or-nothing. An identity this provider build carries no codec
for is refused, naming the identities it does carry — it is never silently
rebound to the default.
