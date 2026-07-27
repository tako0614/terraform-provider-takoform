---
page_title: "takoform_key_value_store Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable key/value state with an optional consistency preference.
---

# takoform_key_value_store

Portable key/value state with an optional consistency preference.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_key_value_store/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `consistency` (String, optional) — Optional consistency preference. One of `eventual`, `strong`.
- `default_ttl_seconds` (Number, optional) — Optional default entry lifetime in seconds. At least 0.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `keyvalue.store@1` — Portable key/value operations. Operations: `delete`, `get`, `list`, `put`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_key_value_store.example NAME
terraform import takoform_key_value_store.example SPACE/NAME
```
