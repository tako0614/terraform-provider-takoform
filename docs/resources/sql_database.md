---
page_title: "takoform_sql_database Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares a portable SQL database.
---

# takoform_sql_database

Declares a portable SQL database. See the [complete example](../../examples/resources/takoform_sql_database/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `engine` (String, optional) — SQL engine capability token; defaults to `sqlite`.
- `migrations_path` (String, optional) — Runner-local migration directory.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource fence, native observation result, and sanitized public
host results. Backend placement is not provider state.
