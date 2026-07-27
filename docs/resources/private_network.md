---
page_title: "takoform_private_network Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable private address space that other Resources can attach to.
---

# takoform_private_network

Portable private address space that other Resources can attach to.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_private_network/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `address_space` (String, required, forces replacement) — Private address block in CIDR notation.
- `public_egress` (Bool, optional) — Whether attached Resources may open outbound public connections.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `network.attach@1` — Portable private network attachment operations. Operations: `attach`, `detach`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_private_network.example NAME
terraform import takoform_private_network.example SPACE/NAME
```
