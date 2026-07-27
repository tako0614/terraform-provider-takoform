---
page_title: "takoform_feature_flag Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable named runtime switch with an optional percentage rollout.
---

# takoform_feature_flag

Portable named runtime switch with an optional percentage rollout.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_feature_flag/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `flag_key` (String, required, forces replacement) — Stable key applications evaluate. Changing it replaces the flag.
- `enabled` (Bool, required) — Whether the flag evaluates true by default.
- `rollout_percentage` (Number, optional) — Optional share of evaluations that receive the enabled value. Between 0 and 100.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `flag.evaluate@1` — Portable feature flag evaluation operations. Operations: `evaluate`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_feature_flag.example NAME
terraform import takoform_feature_flag.example SPACE/NAME
```
