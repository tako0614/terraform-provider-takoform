---
page_title: "takoform_model_endpoint Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable inference endpoint serving one declared model for one declared task.
---

# takoform_model_endpoint

Portable inference endpoint serving one declared model for one declared task.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_model_endpoint/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `model` (String, required) — Immutable model reference the endpoint serves.
- `task` (String, required) — Open task capability token, for example text_generation or embedding.
- `max_concurrency` (Number, optional) — Optional concurrent-inference preference. At least 1.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `model.invoke@1` — Portable model inference operations. Operations: `invoke`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_model_endpoint.example NAME
terraform import takoform_model_endpoint.example SPACE/NAME
```
