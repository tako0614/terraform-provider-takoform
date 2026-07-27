---
page_title: "takoform_queue Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable asynchronous delivery with at-least-once semantics.
---

# takoform_queue

Portable asynchronous delivery with at-least-once semantics.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_queue/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `max_retries` (Number, optional) — Optional delivery retry preference. At least 0.
- `max_batch_size` (Number, optional) — Optional consumer batch size preference. At least 1.
- `visibility_timeout_seconds` (Number, optional) — Optional time a received message stays invisible to other consumers. At least 0.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `queue.messages@1` — Portable queue delivery operations. Operations: `acknowledge`, `receive`, `send`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_queue.example NAME
terraform import takoform_queue.example SPACE/NAME
```
