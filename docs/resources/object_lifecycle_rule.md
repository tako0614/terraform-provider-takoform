---
page_title: "takoform_object_lifecycle_rule Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable retention and transition rule applied to one connected object store.
---

# takoform_object_lifecycle_rule

Portable retention and transition rule applied to one connected object store.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_object_lifecycle_rule/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `prefix` (String, optional) — Optional key prefix the rule applies to.
- `expire_after_days` (Number, optional) — Optional age in days after which matching objects are deleted. At least 1.
- `transition_after_days` (Number, optional) — Optional age in days after which matching objects change storage class. At least 1.
- `transition_storage_class` (String, optional) — Storage class matching objects transition into. One of `infrequent_access`, `archive`.
- `connections` (List of Object, required) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Import

```console
terraform import takoform_object_lifecycle_rule.example NAME
terraform import takoform_object_lifecycle_rule.example SPACE/NAME
```
