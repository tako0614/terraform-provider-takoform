---
page_title: "takoform_container_service Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares a portable OCI container service.
---

# takoform_container_service

Declares a portable OCI container service. See the [complete example](../../examples/resources/takoform_container_service/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `image` (String, required) — OCI image reference.
- `ports` (Set of Number, optional) — Requested container ports.
- `public_http` (Boolean, optional) — Requests public HTTP exposure.
- `connections` (List of Object, optional) — Non-secret resource connections with `name`, `resource`, `permissions`, and `projection`.
- `space` (String, optional, forces replacement) — Overrides the provider default.

Runtime configuration and credentials are host-owned projections; this resource never stores arbitrary environment values.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource fence, native observation result, and sanitized public
host results. Backend placement is not provider state.
