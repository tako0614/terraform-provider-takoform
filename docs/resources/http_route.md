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
- `connections` (List of Object, exactly one) — One declared Resource reference with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`form_api_version`, `form_kind`, `form_definition_version`, `form_schema_digest`, and
`form_package_digest` bind state to the exact immutable Form identity.
`id` is the provider-synthesized `Kind/name` identity and `resource_version` is
the host generation fence. `drift_status`, `portability`, and `outputs` are written
only after the host's observed and output documents satisfy this exact Form's
closed schemas, identities, and generation. Undeclared host keys are rejected;
backend placement is never provider state.

## Import

```console
terraform import takoform_http_route.example NAME
terraform import takoform_http_route.example SPACE/NAME
```
