---
page_title: "takoform_resource Resource - takoform"
subcategory: "Host API v1alpha3"
description: |-
  Generic Host API v1alpha3 carrier for any Form, including third-party Forms, addressed by exact FormRef.
---

# takoform_resource

Carries any Host API v1alpha3 Form by exact FormRef, including Forms published
by third parties in their own API groups. It is not a Form and has no FormRef
of its own: the configuration names the exact reference plus one JSON desired
spec, so a Form this provider was never built against is still usable.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_resource/resource.tf).

Prefer a typed resource where one exists: the Edge Platform Family resources
carry per-field validation, a typed plan, and short import IDs. This carrier
trades all three for reach — it imports too, but only through the exact
identity it cannot infer.

## Arguments

The four `form_*` attributes are the exact FormRef. They, `name`, and `space`
all force replacement, so existing state is never rebound to another identity.

- `form_api_version` (String, required, forces replacement) — Exact namespaced Form group and version, for example `forms.example.com/v1alpha1`. The frozen `forms.takoform.com/v1alpha1` and `forms.takoform.com/v1alpha2` groups are rejected; retained-epoch Forms belong to the provider-v2 typed resources.
- `form_kind` (String, required, forces replacement) — Exact Form kind, matching `^[A-Z][A-Za-z0-9]{0,63}$`.
- `form_definition_version` (String, required, forces replacement) — Exact immutable Form definition version, a semantic version.
- `form_schema_digest` (String, required, forces replacement) — Exact immutable Form schema digest, `sha256:` followed by 64 lowercase hexadecimal characters.
- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`), matching `^[a-z][a-z0-9-]{0,62}$`.
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `spec_json` (String, optional) — The portable desired spec as one JSON object string (for example `jsonencode({...})`); the provider sends the parsed object as `spec`. Omitting it means the empty spec `{}`.
- `create_timeout` / `update_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `20m` / `30m`).

## Read-only attributes

- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.
- `generation` — desired-state generation; increments only when the portable desired spec changes. Updates fence on it together with `uid`.
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
- `outputs_json` — JSON-serialized `status.outputs` document (`"{}"` when empty).
- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes an in-place re-apply, which is all a host needs to re-resolve and re-pin every reference. A Form whose Definition omits `update` refuses that apply, naming the missing capability; replace the resource instead. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.
- `pending_operation_id` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.

State records the four FormRef fields and no package digest: the distribution
a host installed is audit evidence, never resource identity.

## No local schema compilation

The provider neither fetches nor compiles the Form's desired schema:

- at plan time it checks only that the four FormRef fields are wholly known and satisfy the FormRef grammar and that `spec_json` parses as one JSON object; it reads no Form Definition and makes no host call;
- before each apply the client requires `GET {api}/forms` to return that exact FormRef as installed, executable, activated, available to the principal, and supporting the requested operation, then calls `prepare` and `apply`;
- the host validates `spec` against that exact Form's `desiredSchema`, so a spec that violates the Form surfaces as a host `invalid_argument` error during apply rather than as a plan-time diagnostic.

Reads compare `spec_json` semantically rather than textually: your formatting
survives while the parsed document still equals the host's `spec`, and a real
out-of-band change adopts the host's canonical serialization so the next plan
shows the drift.

## State continuity

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
  defers to an exact read of the resource, which decides.

## Write the Form's defaults explicitly

A host materializes the portable defaults a Form declares before it validates,
digests, or stores your spec, so the `spec` it serves back can carry properties
your `spec_json` omitted. Because this carrier reads no Form Definition, it
cannot fill those defaults into the plan, and a read that adopts the host's
materialized document leaves a difference the next plan proposes again — each
apply is a host no-op that never advances `generation`, and the difference
returns. Write every defaulted property explicitly in `spec_json`, or use the
typed resource for that Form where one exists: typed resources carry each
declared default in the schema, so an omitted attribute plans as the value the
host will materialize and the second plan is empty.

## Import

The import ID is one JSON object naming the exact identity. It is not a
delimiter-joined string, because a SpaceID is opaque UTF-8 whose only forbidden
character is `/`, so no separator can escape it safely:

```console
terraform import takoform_resource.example \
  '{"space":"prod","apiVersion":"forms.example.com/v1alpha1","kind":"ExampleWidget","definitionVersion":"1.0.0","schemaDigest":"sha256:…","name":"example-widget"}'
```

`space` is optional and falls back to the provider default; every other member
is required. The `NAME` and `SPACE/NAME` short forms the typed family resources
accept are refused here: this carrier has no default create ref to resolve them
against, and guessing one would bind state to a Form the resource may not be.

Import writes exactly the identity you state and nothing else; the refresh that
follows is what verifies it against the host, and `spec_json` is adopted from
the host's materialized spec there. Nothing about the carrier's trust model
changes: no Form Definition is fetched and no schema is compiled.
