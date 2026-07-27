---
page_title: "takoform_analytics_dataset Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable append-oriented dataset queried for analysis rather than transactions.
---

# takoform_analytics_dataset

Portable append-oriented dataset queried for analysis rather than transactions.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_analytics_dataset/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `partition_field` (String, optional, forces replacement) — Optional field the dataset partitions on. Changing it replaces the dataset.
- `retention_days` (Number, optional) — Optional record retention in days. At least 1.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `analytics.query@1` — Portable analytics dataset operations. Operations: `append`, `query`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_analytics_dataset.example NAME
terraform import takoform_analytics_dataset.example SPACE/NAME
```
