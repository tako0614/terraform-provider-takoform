---
page_title: "takoform_load_balancer Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable listener that distributes connections across connected backends.
---

# takoform_load_balancer

Portable listener that distributes connections across connected backends.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_load_balancer/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `protocol` (String, required) — Listener protocol. One of `tcp`, `udp`, `http`, `https`.
- `listen_port` (Number, required) — Port the listener accepts connections on. Between 1 and 65535.
- `health_check_path` (String, optional) — Optional HTTP path polled to decide backend health.
- `connections` (List of Object, required) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `network.endpoint@1` — Portable network endpoint status operations. Operations: `status`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_load_balancer.example NAME
terraform import takoform_load_balancer.example SPACE/NAME
```
