---
page_title: "takoform_metric_sink Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable destination that retains numeric time series.
---

# takoform_metric_sink

Portable destination that retains numeric time series.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_metric_sink/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `retention_days` (Number, optional) — Optional series retention in days. At least 1.
- `resolution_seconds` (Number, optional) — Optional smallest retained sample interval in seconds. At least 1.
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

- `metric.ingest@1` — Portable metric ingest and read operations. Operations: `query`, `write`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_metric_sink.example NAME
terraform import takoform_metric_sink.example SPACE/NAME
```
