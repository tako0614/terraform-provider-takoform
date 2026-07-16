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

`id`, `selected_implementation`, `target`, `locked`, `portability`, and `outputs` report sanitized host results.
