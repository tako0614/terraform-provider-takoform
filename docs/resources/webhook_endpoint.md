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
- `connections` (List of Object, required) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `http.request@1` — Portable HTTP request surface exposed by a webhook endpoint. Operations: `request`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_webhook_endpoint.example NAME
terraform import takoform_webhook_endpoint.example SPACE/NAME
```
