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
- `artifact_path` / `artifact_url` / `artifact_ref` (String, optional) — Exactly one immutable artifact source. `artifact_url` and `artifact_ref` require `artifact_sha256`.
- `artifact_sha256` (String, optional) — Expected artifact digest.
- `index_document` (String, optional) — Document served for a directory request. Defaults to `index.html`.
- `error_document` (String, optional) — Optional document served for a not-found request.
- `single_page_app` (Bool, optional) — Whether unmatched paths should serve the index document.
- `cache_control_seconds` (Number, optional) — Optional freshness lifetime advertised for served assets, in seconds. At least 0.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `http.request@1` — Portable HTTP request surface exposed by a static site. Operations: `request`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_static_site.example NAME
terraform import takoform_static_site.example SPACE/NAME
```
