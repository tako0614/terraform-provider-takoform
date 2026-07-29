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

## Import

```console
terraform import takoform_rate_limit_policy.example NAME
terraform import takoform_rate_limit_policy.example SPACE/NAME
```
