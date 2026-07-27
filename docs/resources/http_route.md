---
page_title: "takoform_http_route Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable hostname and path binding that sends HTTP traffic to one connected Resource.
---

# takoform_http_route

Portable hostname and path binding that sends HTTP traffic to one connected Resource.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_http_route/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `hostname` (String, required) — Hostname this route answers.
- `path_prefix` (String, optional) — Absolute path prefix this route matches. Defaults to `/`.
- `strip_path_prefix` (Bool, optional) — Whether the matched prefix is removed before the request reaches the target.
- `connections` (List of Object, required) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Import

```console
terraform import takoform_http_route.example NAME
terraform import takoform_http_route.example SPACE/NAME
```
