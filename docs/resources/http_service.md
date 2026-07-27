---
page_title: "takoform_http_service Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable HTTP application served from a prebuilt immutable artifact.
---

# takoform_http_service

Portable HTTP application served from a prebuilt immutable artifact.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_http_service/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `artifact_path` / `artifact_url` / `artifact_ref` (String, optional) — Exactly one immutable artifact source. `artifact_url` and `artifact_ref` require `artifact_sha256`.
- `artifact_sha256` (String, optional) — Expected artifact digest.
- `runtime` (String, optional) — Open runtime capability token the artifact expects. The configured host decides support.
- `runtime_version` (String, optional) — Optional runtime version requested for the artifact.
- `request_timeout_seconds` (Number, optional) — Optional per-request timeout preference in seconds. Between 1 and 3600.
- `concurrency` (Number, optional) — Optional concurrent-request preference. At least 1.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `http.request@1` — Portable HTTP request surface exposed by an application. Operations: `request`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_http_service.example NAME
terraform import takoform_http_service.example SPACE/NAME
```
