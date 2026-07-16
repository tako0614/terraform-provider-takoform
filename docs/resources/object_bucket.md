---
page_title: "takoform_object_bucket Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares portable object storage.
---

# takoform_object_bucket

Declares portable object storage. See the [complete example](../../examples/resources/takoform_object_bucket/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `storage_class` (String, optional) — `standard` or `infrequent_access`; defaults to `standard`.
- `interfaces` (Set of String, optional) — Requested interfaces such as `s3_api`, `signed_url`, or `object_events`.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `selected_implementation`, `target`, `locked`, `portability`, and `outputs` report sanitized host results.
