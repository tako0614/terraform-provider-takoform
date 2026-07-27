---
page_title: "takoform_container_registry Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable OCI artifact registry namespace.
---

# takoform_container_registry

Portable OCI artifact registry namespace.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_container_registry/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `visibility` (String, optional) — Whether pulls require an authenticated principal. One of `private`, `public`. Defaults to `private`.
- `immutable_tags` (Bool, optional) — Whether an existing tag may be repointed at different bytes.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `registry.images@1` — Portable registry artifact operations. Operations: `list`, `pull`, `push`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_container_registry.example NAME
terraform import takoform_container_registry.example SPACE/NAME
```
