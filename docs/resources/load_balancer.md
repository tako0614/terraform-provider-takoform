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
- `internal` (Bool, optional) — Whether the listener is reachable only from inside the host's private network.
- `idle_timeout_seconds` (Number, optional) — Optional time an idle connection is held open, in seconds. Between 1 and 4000.
- `connections` (List of Object, one or more) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. Names must be unique. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`form_api_version`, `form_kind`, `form_definition_version`, `form_schema_digest`, and
`form_package_digest` bind state to the exact immutable Form identity.
`id` is the provider-synthesized `Kind/name` identity and `resource_version` is
the host generation fence. `drift_status`, `portability`, and `outputs` are written
only after the host's observed and output documents satisfy this exact Form's
closed schemas, identities, and generation. Undeclared host keys are rejected;
backend placement is never provider state.

## Declared runtime interfaces

- `network.endpoint@1` — Portable network endpoint status operations. Operations: `status`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_load_balancer.example NAME
terraform import takoform_load_balancer.example SPACE/NAME
```
