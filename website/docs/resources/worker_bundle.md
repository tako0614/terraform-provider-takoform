---
page_title: "takoform_worker_bundle Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Worker Bundle (edge.forms.takoform.com/v1alpha1, role revision).
---

# takoform_worker_bundle

Immutable content-addressed module bundle of one worker build, named by the digest of the artifact manifest committed through the content-addressed upload API (decision 0012). The manifest, not this Form, describes the main module and every additional module with its closed media type, exact size, and sha256 digest, so the bundle keeps exactly one source of truth for its bytes. Different bytes commit a different manifest, which is a different bundle.

This is a `revision` resource: an immutable snapshot. It is create-only — every desired attribute forces replacement, and rollback means pointing a deployment at an earlier revision, never editing this one.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_worker_bundle/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `manifest_digest` (String, optional, computed, forces replacement) — Immutable digest of the committed artifact manifest this bundle is. It is the whole portable desired state: the manifest, not this resource, describes the modules. Declare exactly one of the two authoring modes — reference a manifest already committed to the host by setting this digest, or leave it unset and author the bundle locally with the two arguments below. Writing it alongside local authoring is accepted only when the authored bytes commit exactly that manifest; a disagreement is refused before any host call.
- `main_module` (String, optional, forces replacement) — Local authoring only: relative path of the ES module the runtime instantiates first; it must name one declared module. It is not portable desired state; it describes the artifact manifest the provider commits.
- `modules` (List of Object, optional, forces replacement) — Local authoring only: every module of the bundle. Each entry declares `name`, `content_type` (one of the five closed media types), and `content_file` (a local file path). The provider reads each file, computes its exact `size` and sha256 `digest` (both computed attributes), commits the artifact manifest through the content-addressed artifact API, and records the returned `manifest_digest`. File paths stay in state; file bytes never do. At every plan against existing state the provider re-reads and re-hashes each `content_file`: changed bytes at an unchanged path change the planned manifest digest and force replacement.
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
- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes replacing this resource, because this Form declares no in-place update and a host refuses every apply to the existing one. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.
- `pending_operation_id` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.

## State continuity

- **Reads dispatch on the recorded FormRef.** `WorkerBundle` state is addressed under the
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
terraform import takoform_worker_bundle.example NAME
terraform import takoform_worker_bundle.example SPACE/NAME
```

Both short forms bind state to this provider build's default create
FormRef. To adopt a resource created under an EARLIER definition version of
this Form, name the exact identity instead. The import ID is then one JSON
object — not a delimiter-joined string, because a SpaceID is opaque UTF-8
whose only forbidden character is `/`, so no separator can escape it safely:

```console
terraform import takoform_worker_bundle.example \
  '{"space":"prod","apiVersion":"edge.forms.takoform.com/v1alpha1","kind":"WorkerBundle","definitionVersion":"0.1.0","schemaDigest":"sha256:…","name":"…"}'
```

`space` is optional and falls back to the provider default; the four FormRef
members are all-or-nothing. An identity this provider build carries no codec
for is refused, naming the identities it does carry — it is never silently
rebound to the default.

An imported bundle restores `manifest_digest` from the host and leaves
`main_module` and `modules` null: those are local authoring facts the wire
never echoes. The resource is fully manageable afterwards — a configuration
that states the same `manifest_digest` plans empty, and adopting the local
files that commit exactly that manifest is not a change either, because the
bundle's identity is the digest.
