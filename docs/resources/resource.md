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
carry per-field validation, a typed plan, and import support. This carrier
trades all three for reach.

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
- `ready` — true when the closed `Ready` condition reports `True`.
- `outputs_json` — JSON-serialized `status.outputs` document (`"{}"` when empty).

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
shows the drift. When the host serves a different `uid` for the same name, the
provider warns that the resource was replaced out of band and removes it from
state so the next plan proposes re-creating it.

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

`terraform import` is not supported for `takoform_resource`. An import ID
carries only `NAME` or `SPACE/NAME`, which cannot supply the four exact FormRef
fields this carrier requires, and the provider will not guess them. Adopt an
existing resource through the typed family resource for its Form where one
exists; otherwise the resource must be created through this provider, because
create fences on `If-None-Match: *` and fails instead of silently adopting an
existing host resource.
