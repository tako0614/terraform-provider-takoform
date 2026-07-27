---
page_title: "takoform_stream_topic Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable published event stream that many independent consumers can read.
---

# takoform_stream_topic

Portable published event stream that many independent consumers can read.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_stream_topic/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `partitions` (Number, optional, forces replacement) — Ordered partition count fixed for the stream lifecycle. At least 1.
- `retention_hours` (Number, optional) — Optional published-record retention in hours. At least 1.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `stream.publish@1` — Portable stream publish and subscribe operations. Operations: `publish`, `subscribe`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_stream_topic.example NAME
terraform import takoform_stream_topic.example SPACE/NAME
```
