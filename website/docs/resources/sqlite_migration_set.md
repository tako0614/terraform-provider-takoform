---
page_title: "takoform_sqlite_migration_set Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  SQLite Migration Set (edge.forms.takoform.com/v1beta2, role revision).
---

# takoform_sqlite_migration_set

Immutable ordered SQLite migration history backed by one committed artifacts.takoform.com/v1alpha1 MigrationBundle manifest. The manifest files array is the migration order; every entry has a unique portable path, media type application/sql, exact size, and sha256 digest. SQL bytes travel only through the content-addressed artifact upload and never enter desired state. A new set may append entries, but an application refuses any set whose prefix rewrites, reorders, or removes an already applied path+digest pair.

This is a `revision` resource: an immutable snapshot. It is create-only — every desired attribute forces replacement, and rollback means pointing a deployment at an earlier revision, never editing this one.

This Experimental Form speaks the Host API v1beta1 lane and requires provider v2.1.1 or
later. Provider v2.1.1 is Registry-published; release/version.json retains
candidate-only descriptor metadata after owner publication. The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](https://takoform.com/examples/resources/takoform_sqlite_migration_set/resource.tf).

## Arguments

- `name` (String, optional, computed, forces replacement) — Portable resource name (`metadata.name`). Omit it and set `revision_owner` instead: this Form is an immutable revision, so the provider derives `sqlite-migration-set-<content digest prefix>-<owner digest prefix>` from this revision's own content and its declared owner. Changed content is then a NEW revision created beside the old one, which is the only way a code change applies at all — a host refuses every update to a revision, and replacing one under a name it still holds completes in neither apply order. Setting it pins the name, which an imported revision needs; the provider then refuses at plan time any change that would replace this revision under it.
- `revision_owner` (String, optional, forces replacement) — Stable name of whatever owns this revision; the `takoform_module_worker` it belongs to is the usual answer. Required whenever `name` is omitted. Two independent resources built from identical content derive identical content digests, so without an owner they would derive one name and two Terraform resources would manage one host address — where a destroy of either breaks the other. It is provider-side authoring input: no wire member carries it, the host never sees it, and it enters only the derived name. The official [`worker-app` module](https://github.com/tako0614/terraform-provider-takoform/tree/main/modules/worker-app) sets it for you.
- `manifest_digest` (String, required, forces replacement) — Immutable identity of the committed MigrationBundle artifact manifest. Before mutation a host resolves it within the caller's tenant and verifies its canonical digest, MigrationBundle kind, non-empty ordered files inventory, unique paths, application/sql media type, sizes, and blob digests.
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

- **Reads dispatch on the recorded FormRef.** `SQLiteMigrationSet` state is addressed under the
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
terraform import takoform_sqlite_migration_set.example NAME
terraform import takoform_sqlite_migration_set.example SPACE/NAME
```

Both short forms bind state to this provider build's default create
FormRef. To adopt a resource created under an EARLIER definition version of
this Form, name the exact identity instead. The import ID is then one JSON
object — not a delimiter-joined string, because a SpaceID is opaque UTF-8
whose only forbidden character is `/`, so no separator can escape it safely:

```console
terraform import takoform_sqlite_migration_set.example \
  '{"space":"prod","apiVersion":"edge.forms.takoform.com/v1beta2","kind":"SQLiteMigrationSet","definitionVersion":"0.1.0","schemaDigest":"sha256:…","name":"…"}'
```

`space` is optional and falls back to the provider default; the four FormRef
members are all-or-nothing. An identity this provider build carries no codec
for is refused, naming the identities it does carry — it is never silently
rebound to the default.
