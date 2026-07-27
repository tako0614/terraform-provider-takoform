---
page_title: "takoform_rate_limit_policy Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable request budget applied to one connected Resource.
---

# takoform_rate_limit_policy

Portable request budget applied to one connected Resource.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_rate_limit_policy/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `requests_per_minute` (Number, required) — Sustained request budget per minute. At least 1.
- `burst` (Number, optional) — Optional additional requests tolerated above the sustained budget. At least 0.
- `scope` (String, optional) — Whether the budget is counted per calling client or across the whole target. One of `client`, `route`. Defaults to `client`.
- `connections` (List of Object, required) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Import

```console
terraform import takoform_rate_limit_policy.example NAME
terraform import takoform_rate_limit_policy.example SPACE/NAME
```
