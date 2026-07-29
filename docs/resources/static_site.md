---
page_title: "takoform_static_site Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable static asset site served from a prebuilt immutable artifact.
---

# takoform_static_site

Portable static asset site served from a prebuilt immutable artifact.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_static_site/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `artifact_url` (String, required) — Absolute credential-free HTTPS location any conforming host can fetch; userinfo, query, and fragment are forbidden because this value persists in nonsensitive state.
- `artifact_sha256` (String, required) — Digest binding the URL to exact immutable bytes.
- `artifact_media_type` (String, required) — Lowercase type/subtype describing how the bytes are interpreted.
- `index_document` (String, optional) — Document served for a directory request. Defaults to `index.html`.
- `error_document` (String, optional) — Optional document served for a not-found request.
- `single_page_app` (Bool, optional) — Whether unmatched paths should serve the index document.
- `cache_control_seconds` (Number, optional) — Optional freshness lifetime advertised for served assets, in seconds. At least 0.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`form_api_version`, `form_kind`, `form_definition_version`, `form_schema_digest`, and
`form_package_digest` bind state to the exact immutable Form identity.
`id` is the provider-synthesized `Kind/name` identity and `resource_version` is
the host generation fence. `drift_status`, `portability`, and `outputs` are written
only after the host's observed and output documents satisfy this exact Form's
closed schemas, identities, and generation. Undeclared host keys are rejected;
backend placement is never provider state.

## Declared runtime interfaces

- `http.request@1` — Portable HTTP request surface exposed by a static site. Operations: `request`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_static_site.example NAME
terraform import takoform_static_site.example SPACE/NAME
```
