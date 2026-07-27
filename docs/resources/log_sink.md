---
page_title: "takoform_log_sink Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable destination that retains structured application logs.
---

# takoform_log_sink

Portable destination that retains structured application logs.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_log_sink/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `retention_days` (Number, optional) — Optional log retention in days. At least 1.
- `format` (String, optional) — Record format the sink accepts. One of `json`, `text`. Defaults to `json`.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `log.ingest@1` — Portable log ingest and read operations. Operations: `query`, `write`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_log_sink.example NAME
terraform import takoform_log_sink.example SPACE/NAME
```
