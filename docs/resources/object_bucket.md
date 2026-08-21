---
page_title: "takoform_object_bucket Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable object storage namespace.
---

# takoform_object_bucket

Portable object storage namespace.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](https://takoform.com/examples/resources/takoform_object_bucket/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `versioning` (Bool, optional) — When present, requires retention of non-current object versions when true and disables creation of new non-current versions when false; omission states no portable version-retention requirement.
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

- `object.storage@1` — Portable object storage operations. Operations: `delete`, `get`, `list`, `put`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_object_bucket.example NAME
terraform import takoform_object_bucket.example SPACE/NAME
```
