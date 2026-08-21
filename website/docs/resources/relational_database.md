---
page_title: "takoform_relational_database Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable relational database identified by an open engine capability token.
---

# takoform_relational_database

Portable relational database identified by an open engine capability token.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](https://takoform.com/examples/resources/takoform_relational_database/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `engine` (String, required, forces replacement) — Open relational engine token. Changing it replaces the database.
- `engine_version` (String, optional) — Optional engine compatibility version required by the workload.
- `database_name` (String, optional, forces replacement) — Initial logical database name. Changing it replaces the resource.
- `schema_url` (String, optional) — Optional immutable schema or migration bundle applied in document order.
- `schema_sha256` (String, optional) — Canonical lowercase digest binding schemaUrl to exact immutable bytes.
- `schema_format` (String, optional) — Open token naming the schema bundle format.
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

- `sql.query@1` — Portable SQL query and transaction operations. Operations: `execute`, `query`, `transaction`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_relational_database.example NAME
terraform import takoform_relational_database.example SPACE/NAME
```
