---
page_title: "takoform_worker_bundle Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Worker Bundle (edge.forms.takoform.com/v1alpha1, role revision).
---

# takoform_worker_bundle

Immutable content-addressed module bundle of one worker build: a main module plus additional modules, each pinned by size and digest and resolved through the content-addressed artifact upload API (decision 0012). Different bytes are a different bundle.

This is a `revision` resource: an immutable snapshot. It is create-only — every desired attribute forces replacement, and rollback means pointing a deployment at an earlier revision, never editing this one.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_worker_bundle/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `main_module` (String, required, forces replacement) — Relative path of the ES module the runtime instantiates first; it must name one declared module.
- `modules` (List of Object, required, forces replacement) — Every module of the bundle. Each entry declares `name`, `content_type` (one of the five closed media types), and `content_file` (a local file path). The provider reads each file, computes its exact `size` and sha256 `digest` (both computed attributes), uploads the bytes through the content-addressed artifact API, and pins the module by digest. File paths stay in state; file bytes never do. At every plan against existing state the provider re-reads and re-hashes each `content_file`: changed bytes at an unchanged path change the planned digest and force replacement.
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `create_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `30m`). Changing only these provider-side timeouts is applied in place without any host call.

## Read-only attributes

- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.
- `generation` — desired-state generation; increments only when the portable desired spec changes; a revision has no spec-changing update — every desired attribute forces replacement instead.
- `revision` — representation revision; increments whenever the representation changes — a spec-changing update, new status, or new outputs. Deletes fence on it via `If-Match`.
- `ready` — true when the closed `Ready` condition reports `True`.
- `outputs_json` — JSON-serialized `status.outputs` document (`"{}"` when empty).
- `form_api_version`, `form_kind`, `form_definition_version`, `form_schema_digest` — the exact immutable FormRef this state is bound to; reads dispatch on it.
- `form_package_digest` — audit-only package provenance; never part of resource identity, queries, or fences.

## Import

```console
terraform import takoform_worker_bundle.example NAME
terraform import takoform_worker_bundle.example SPACE/NAME
```

Import is supported for adoption, but an imported bundle cannot converge
with local `content_file` authoring: the wire does not echo local file
paths, so imported state carries no authored modules and the first plan
after import proposes replacement. The replacement re-uploads your authored
files; byte-identical files pin the same content-addressed digests.
