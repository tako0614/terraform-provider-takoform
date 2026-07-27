---
page_title: "takoform_container_service Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable OCI container service pinned to an immutable image digest.
---

# takoform_container_service

Portable OCI container service pinned to an immutable image digest.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_container_service/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `image` (String, required) — Immutable OCI image reference pinned by sha256 digest.
- `ports` (Set of Number, optional) — Container ports requested by the service. Between 1 and 65535.
- `public_http` (Bool, optional) — Whether this container asks for public HTTP exposure.
- `cpu_millicores` (Number, optional) — Optional CPU request in millicores. At least 1.
- `memory_mib` (Number, optional) — Optional memory request in mebibytes. At least 1.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `http.request@1` — Portable HTTP request surface exposed by a container service. Operations: `request`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_container_service.example NAME
terraform import takoform_container_service.example SPACE/NAME
```
