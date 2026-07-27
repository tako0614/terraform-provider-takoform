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
- `connections` (List of Object, required) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Import

```console
terraform import takoform_backup_policy.example NAME
terraform import takoform_backup_policy.example SPACE/NAME
```
