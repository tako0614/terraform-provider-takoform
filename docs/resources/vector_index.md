---
page_title: "takoform_vector_index Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares a portable vector index.
---

# takoform_vector_index

Declares a portable vector index. See the [complete example](../../examples/resources/takoform_vector_index/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `dimensions` (Number, required, forces replacement) — Positive vector dimension count.
- `metric` (String, optional) — Similarity metric capability token; defaults to `cosine`.
- `connections` (List of Object, optional) — Non-secret resource connections with `name`, `resource`, `permissions`, and `projection`.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `selected_implementation`, `target`, `locked`, `portability`, and `outputs` report sanitized host results.
