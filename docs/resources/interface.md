---
page_title: "takoform_interface Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares one generic runtime interface exposed by a managed Resource.
---

# takoform_interface

Declares an application-authored, data-only runtime interface. The resource
contains no protocol-specific schema: write the complete document in
`document_json` and deterministic value mappings in `inputs_json`.

## Arguments

- `name` (String, required) — author-defined interface name.
- `version` (String, required) — exact author-defined version.
- `resource_kind` (String, required) — kind of the exposing Resource.
- `resource_name` (String, required) — name of the exposing Resource.
- `space` (String, optional) — defaults to provider config.
- `document_json` (String, required) — exact non-secret JSON object.
- `document_schema_json` (String, optional) — optional Draft 2020-12 schema.
- `inputs_json` (String, optional) — JSON array of generic `literal`, `output`,
  `resource_uri`, or explicitly host-namespaced input declarations. Defaults
  to `[]`.
- `resource_uri_input` (String, optional) — name of the single
  `resource_uri` input.

## Read-only attributes

- `id` — portable compound address, not a host record id.
- `resource_version` — opaque optimistic-concurrency fence.
- `values_json` — resolved public values.
- `resource_uri` — resolved credential-free HTTPS URI when declared.

The provider `endpoint` is the host control-plane origin. Runtime code does not
invoke an Interface through the provider. It discovers the declaration from
the host, obtains host-governed authorization when needed, and calls the
resolved endpoint directly.
