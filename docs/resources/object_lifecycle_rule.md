---
page_title: "takoform_object_lifecycle_rule Resource - takoform"
subcategory: "Service Forms"
description: |-
  One portable expiration or storage-transition action applied to a connected object store.
---

# takoform_object_lifecycle_rule

One portable expiration or storage-transition action applied to a connected object store.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_object_lifecycle_rule/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `prefix` (String, optional) — Optional key prefix the rule applies to.
- `action` (String, required) — Single action performed when matching objects reach after_days. One of `expire`, `transition_infrequent_access`, `transition_archive`.
- `after_days` (Number, required) — Age in days at which the declared action is performed. At least 1.
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
terraform import takoform_object_lifecycle_rule.example NAME
terraform import takoform_object_lifecycle_rule.example SPACE/NAME
```
