---
page_title: "takoform_sql_database Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares a portable bounded indexed database.
---

# takoform_sql_database

Declares a portable bounded indexed database. See the [complete example](../../examples/resources/takoform_sql_database/resource.tf).

Configuring `tables` selects the exact `SQLDatabase@2.0.0` Form and its
required `data.indexed@1` Interface. This interface exposes primary-key and
declared-index operations without accepting caller-provided SQL or DDL. It
pins closed request/response schemas, ascending byte/numeric/boolean ordering,
and 15-minute tamper-evident live-keyset cursors.

Existing `SQLDatabase@1.0.1` state remains readable, deletable, and importable.
Its historical `engine` and `migrations_path` fields are retained for that
compatibility path; they are not sent for a `tables` resource. A plain import
continues to select the historical identity because an import ID does not
contain a Form version.

`SQLDatabase@2.0.0` requires the versioned Form host API. The provider rejects
its create, read, update, and delete operations before network I/O when
historical `compatibility_fallback` is enabled. Historical 1.x resources retain
their existing fallback behavior.

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `tables` (List, optional, forces replacement) — One to 16 declared tables.
  Setting it selects `SQLDatabase@2.0.0`; changing the declared schema replaces
  the Resource.
  - `name` (String, required) — Portable identifier.
  - `columns` (List, required) — One to 32 columns with `name`, `type`, and
    optional `nullable` (default `false`). Types are `string`, `integer`,
    `number`, or `boolean`.
  - `primary_key` (List, required) — One to four ordered column names.
  - `indexes` (List, optional) — Up to eight indexes with `name`, one to four
    ordered `columns`, and optional `unique` (default `false`).
- `engine` (String, optional) — Historical `SQLDatabase@1.x` engine capability
  token; defaults to `sqlite`. The default-compatible value is omitted when
  `tables` is configured; any other engine cannot be combined with `tables`.
- `migrations_path` (String, optional) — Historical `SQLDatabase@1.x`
  runner-local migration directory. It cannot be combined with `tables`.
- `space` (String, optional, forces replacement) — Overrides the provider default.

Primary-key and index columns must be declared, non-null, and typed as
`string`, `integer`, or `boolean`; floating-point `number` columns cannot be
keys. Table, column, and index names must be unique within their scope. The
entire declared `tables` schema is immutable for this version; there is no
portable in-place schema migration API in `data.indexed@1`.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource fence, native observation result, and sanitized public
host results. `schema_version` is `1` for the `SQLDatabase@2.0.0` table schema
and absent for historical state. Backend placement is not provider state.

The exact `data.indexed@1` operation and limit contract is documented in
[`spec/data-indexed`](../../spec/data-indexed/README.md). Interface records,
OAuth audience/resource URI, consumer authorization, routing, and lifecycle
remain host-owned.
