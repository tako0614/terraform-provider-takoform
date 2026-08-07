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

## Import

```console
terraform import takoform_worker_bundle.example NAME
terraform import takoform_worker_bundle.example SPACE/NAME
```

An imported bundle restores `manifest_digest` from the host and leaves
`main_module` and `modules` null: those are local authoring facts the wire
never echoes. The resource is fully manageable afterwards — a configuration
that states the same `manifest_digest` plans empty, and adopting the local
files that commit exactly that manifest is not a change either, because the
bundle's identity is the digest.
