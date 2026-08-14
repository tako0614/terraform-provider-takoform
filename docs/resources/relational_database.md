---
page_title: "takoform_relational_database Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable relational database addressed through an open engine capability token.
---

# takoform_relational_database

Portable relational database addressed through an open engine capability token.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_relational_database/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `engine` (String, required, forces replacement) — Open engine capability token. Changing it replaces the database.
- `engine_version` (String, optional) — Optional engine version requested from the host.
- `storage_gib` (Number, optional) — Optional storage request in gibibytes. At least 1.
- `size_class` (String, optional) — Open capability token describing the requested compute size.
- `database_name` (String, optional, forces replacement) — Initial logical database created inside the instance. Changing it replaces the database.
- `high_availability` (Bool, optional) — Whether the host should keep a standby able to take over.
- `schema_url` (String, optional) — Optional immutable migration bundle the host applies in order.
- `schema_sha256` (String, optional) — Digest binding schema_url to exact immutable bytes.
- `schema_format` (String, optional) — Open capability token naming how the bundle is interpreted.
- `form_transition` (String, optional) — Closed explicit same-resource transition request. The only value is `relational-database-v2-to-v3`; it is inert for fresh DB3 creates and state already proved as DB3. See the [recorded-Form transition guide](../../release/migrations/v1.0.2-to-v1.0.4-recorded-form-transition.md).
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
