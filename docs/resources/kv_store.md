---
page_title: "takoform_kv_store Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares portable key/value storage.
---

# takoform_kv_store

Declares portable key/value storage. See the [complete example](../../examples/resources/takoform_kv_store/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `consistency` (String, optional) — `eventual` or `strong`.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `selected_implementation`, `target`, `locked`, `portability`, and `outputs` report sanitized host results.
