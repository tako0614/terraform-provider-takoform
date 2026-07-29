---
page_title: "takoform_search_index Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable full-text index over a declared set of document fields.
---

# takoform_search_index

Portable full-text index over a declared set of document fields.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_search_index/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `fields` (Set of String, required) — Document fields the index must make searchable.
- `language` (String, optional) — Optional analysis language token.
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

- `search.query@1` — Portable search index operations. Operations: `delete`, `index`, `query`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_search_index.example NAME
terraform import takoform_search_index.example SPACE/NAME
```
