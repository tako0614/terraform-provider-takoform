---
page_title: "takoform_cache_cluster Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable in-memory cache sized by an open capability token.
---

# takoform_cache_cluster

Portable in-memory cache sized by an open capability token.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_cache_cluster/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `size_class` (String, required) — Open capability token describing the requested cache size.
- `eviction_policy` (String, optional) — Optional eviction preference when the cache is full. One of `least_recently_used`, `least_frequently_used`, `time_to_live`.
- `default_ttl_seconds` (Number, optional) — Optional default entry lifetime in seconds. At least 0.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `cache.store@1` — Portable cache operations. Operations: `delete`, `get`, `put`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_cache_cluster.example NAME
terraform import takoform_cache_cluster.example SPACE/NAME
```
