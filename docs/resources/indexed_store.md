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

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `data.indexed@1` — Portable bounded key and declared-index operations. Operations: `delete`, `get`, `put`, `query`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_indexed_store.example NAME
terraform import takoform_indexed_store.example SPACE/NAME
```
