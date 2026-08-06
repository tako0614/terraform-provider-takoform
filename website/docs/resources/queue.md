---
page_title: "takoform_queue Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable asynchronous at-least-once message delivery.
---

# takoform_queue

Portable asynchronous at-least-once message delivery.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_queue/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `message_retention_seconds` (Number, optional) — Optional minimum retention for an unacknowledged message, in seconds. At least 1.
- `ordering` (String, optional) — Delivery ordering requirement: best_effort gives no relative-order guarantee; strict follows host acceptance order within this queue, apart from duplicate redelivery allowed by at-least-once delivery. One of `best_effort`, `strict`.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
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

- `queue.messages@1` — Portable queue delivery operations. Operations: `acknowledge`, `receive`, `send`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_queue.example NAME
terraform import takoform_queue.example SPACE/NAME
```
