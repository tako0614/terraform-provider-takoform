---
page_title: "takoform_model_endpoint Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable inference endpoint serving digest-bound model bytes for one declared task.
---

# takoform_model_endpoint

Portable inference endpoint serving digest-bound model bytes for one declared task.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_model_endpoint/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `artifact_url` (String, required) — Absolute credential-free HTTPS location any conforming host can fetch; userinfo, query, and fragment are forbidden because this value persists in nonsensitive state.
- `artifact_sha256` (String, required) — Digest binding the URL to exact immutable bytes.
- `artifact_media_type` (String, required) — Lowercase type/subtype describing how the bytes are interpreted.
- `task` (String, required) — Open task capability token, for example text_generation or embedding.
- `max_concurrency` (Number, optional) — Optional concurrent-inference preference. At least 1.
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

- `model.invoke@1` — Portable model inference operations. Operations: `invoke`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_model_endpoint.example NAME
terraform import takoform_model_endpoint.example SPACE/NAME
```
