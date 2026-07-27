---
page_title: "takoform_stateful_entity Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable namespace of individually addressable, individually persistent entities.
---

# takoform_stateful_entity

Portable namespace of individually addressable, individually persistent entities.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_stateful_entity/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `entity_class` (String, required) — Runtime class identifier owning entity behaviour inside this namespace.
- `persistence` (String, optional) — Open persistence capability token requested for entity state.
- `migration_tag` (String, optional) — Optional namespace migration tag. It never identifies one entity instance.
- `configuration` (Map of String, optional) — Non-secret configuration passed to the running service. Secret material is never portable state: a host injects it through its own credential path.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `entity.invoke@1` — Portable stateful entity invocation operations. Operations: `invoke`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_stateful_entity.example NAME
terraform import takoform_stateful_entity.example SPACE/NAME
```
