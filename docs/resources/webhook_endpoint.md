---
page_title: "takoform_webhook_endpoint Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable inbound HTTP endpoint that forwards received requests to one connected Resource.
---

# takoform_webhook_endpoint

Portable inbound HTTP endpoint that forwards received requests to one connected Resource.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_webhook_endpoint/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `path` (String, optional) — Absolute path the endpoint accepts requests on. Defaults to `/`.
- `allowed_methods` (Set of String, optional) — HTTP methods the endpoint accepts. One of `DELETE`, `GET`, `PATCH`, `POST`, `PUT`.
- `connections` (List of Object, exactly one) — One declared Resource reference with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
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

- `http.request@1` — Portable HTTP request surface exposed by a webhook endpoint. Operations: `request`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_webhook_endpoint.example NAME
terraform import takoform_webhook_endpoint.example SPACE/NAME
```
