---
page_title: "takoform_feature_flag Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable named runtime switch expressed as one complete enabled percentage.
---

# takoform_feature_flag

Portable named runtime switch expressed as one complete enabled percentage.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_feature_flag/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `flag_key` (String, required, forces replacement) — Stable key applications evaluate. Changing it replaces the flag.
- `enabled_percentage` (Number, required) — Share of stable evaluation subjects receiving true; 0 is always false and 100 is always true. Between 0 and 100.
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

- `flag.evaluate@1` — Portable feature flag evaluation operations. Operations: `evaluate`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_feature_flag.example NAME
terraform import takoform_feature_flag.example SPACE/NAME
```
