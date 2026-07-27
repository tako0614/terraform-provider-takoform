---
page_title: "takoform_compute_instance Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable long-running machine instance built from an immutable image.
---

# takoform_compute_instance

Portable long-running machine instance built from an immutable image.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_compute_instance/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `machine_class` (String, required) — Open machine capability token describing the requested size class.
- `image` (String, required) — Immutable machine image reference.
- `boot_disk_gib` (Number, required) — Boot disk size in gibibytes. At least 1.
- `instance_count` (Number, optional) — Optional identical-instance count preference. At least 1.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Import

```console
terraform import takoform_compute_instance.example NAME
terraform import takoform_compute_instance.example SPACE/NAME
```
