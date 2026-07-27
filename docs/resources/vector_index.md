---
page_title: "takoform_vector_index Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable vector index with dimensions fixed for the index lifecycle.
---

# takoform_vector_index

Portable vector index with dimensions fixed for the index lifecycle.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_vector_index/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `dimensions` (Number, required, forces replacement) — Positive vector dimensions fixed for the index lifecycle. At least 1.
- `metric` (String, optional) — Open similarity metric capability token. Defaults to `cosine`.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `vector.query@1` — Portable vector index operations. Operations: `delete`, `query`, `upsert`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_vector_index.example NAME
terraform import takoform_vector_index.example SPACE/NAME
```
