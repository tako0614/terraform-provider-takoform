---
page_title: "takoform_indexed_store Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable bounded key/value item store with declared queryable attributes and no query language.
---

# takoform_indexed_store

Portable bounded key/value item store with declared queryable attributes and no query language.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_indexed_store/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `partition_key` (String, required, forces replacement) — Attribute that partitions stored items. Changing it replaces the store.
- `sort_key` (String, optional, forces replacement) — Optional attribute that orders items inside one partition.
- `indexed_attributes` (Set of String, optional) — Additional attributes the host must make queryable.
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

- `data.indexed@1` — Portable bounded key and declared-index operations. Operations: `delete`, `get`, `put`, `query`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_indexed_store.example NAME
terraform import takoform_indexed_store.example SPACE/NAME
```
