# 0011 — Resource identity: UID, generation, and revision

- Status: accepted
- Date: 2026-08-06
- Owners: Takoform maintainers

## Context

The v1alpha2 wire duplicates naming and versioning concerns: `metadata.name`,
`spec.name`, a provider `name` attribute, `output.name`, `observed.id`, and
`output.id` coexist; `resourceVersion` acts both as the desired-state
generation and as the strong ETag for the whole representation, even though
observe/refresh legitimately change status without changing desired state.
Form Definitions also each re-declare envelope facts (`observed.ready`,
`observed.generation`, `output.id`, portability markers) that belong to the
protocol, not to any one Form. Finally, provider state keys resources by
`packageDigest` in addition to FormRef, so the same resource read through a
different — equally valid — package of the same Form becomes unaddressable.

## Decision

The v1alpha3 resource envelope owns identity and versioning; Form schemas own
only portable desired fields.

- `metadata.name` is the only name. `spec.name` and per-Form envelope fields
  (`/name`, `observed.id`, `observed.generation`, `observed.ready`,
  `output.id`, `output.name`, and similar) are removed from Form Definitions.
- `metadata.uid` is a host-issued immutable identity. Deleting and recreating
  a same-named resource yields a different UID. Mutations may carry an
  expected UID and are rejected on mismatch.
- `metadata.generation` (canonical decimal string) increments only when the
  portable desired spec changes.
- `metadata.revision` (canonical decimal string) increments whenever the full
  representation — including status and outputs — changes. The HTTP strong
  ETag is the quoted revision.
- `status.observedGeneration` states which desired generation the status
  reflects. `status.conditions` uses the closed set `Ready`, `Reconciling`,
  `Degraded`, `Drifted`, `Blocked`, `Deleting` with closed portable reasons;
  host-internal codes go to a separate `hostReason`.
- Desired-state mutations fence on the expected generation; representation
  fences use `If-Match` on the revision. Stale fences fail with
  `generation_conflict` (412) or `revision_conflict` (412).
- The Form semantic identity of a resource is its exact FormRef. The package
  digest used at creation may be recorded as audit evidence but never enters
  resource identity, queries, or update/delete fences. A host that installed
  the same FormRef from a different legitimate package must read and delete
  the same resource.

## Consequences

- Provider v2.1 family-resource state identity is `space`, `apiVersion`, `kind`, `uid`; a
  JSON-serialized `status.outputs` document (`outputs_json`) replaces the
  string-map outputs; the ETag/generation split removes
  the global client-side mutation mutex.
- Conformance gains UID-stability, delete/recreate UID change,
  generation-only-on-spec-change, revision-on-status-change, stale-fence
  rejection, and package-digest-substitution checks.
- Form authoring gets simpler: definitions stop repeating envelope plumbing.

## Rejected alternatives

- **Keep one `resourceVersion` for both fences.** Rejected because
  observe/refresh then either lies (status changes invisible to ETag) or
  breaks desired-state fencing (every observe invalidates client fences).
- **Key state by `space/name` only.** Rejected because delete/recreate would
  silently rebind old state to a new resource; UID makes that visible.
- **Keep `packageDigest` in identity for provenance.** Rejected because
  provenance is evidence, not identity; it is retained as an audit field.
