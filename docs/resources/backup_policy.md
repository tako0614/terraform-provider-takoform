---
page_title: "takoform_backup_policy Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable scheduled copy and retention rule for one connected Resource.
---

# takoform_backup_policy

Portable scheduled copy and retention rule for one connected Resource.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_backup_policy/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `cron` (String, required) — Portable five-field cron expression describing when copies are taken.
- `retention_days` (Number, required) — How long each copy is retained, in days. At least 1.
- `timezone` (String, optional) — Open timezone token the schedule is interpreted in. Defaults to `UTC`.
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
terraform import takoform_backup_policy.example NAME
terraform import takoform_backup_policy.example SPACE/NAME
```
