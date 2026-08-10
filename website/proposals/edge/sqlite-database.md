# SQLiteDatabase — `takoform_sqlite_database`

## Workload and consumer

A worker keeps relational application state — accounts, jobs, inventories —
in one embedded SQLite database. Workers consume it through
`module-worker.sqlite` bindings.

## Role

`identity`. The database has no desired fields; SQLite semantics are the
identity, not a configuration.

## Observable semantics

Exactly the `edge.sql@1.0.0` contract: execute/query/transaction with bound
parameters, SQLite's SQL dialect and dynamic typing, rollback-only queries,
serializable transactions applying atomically over a single consistent
snapshot, `busy` under write contention, and closed errors.

`EdgeSqlValue` is null, a finite binary64 number inside
`Number.MAX_SAFE_INTEGER`, a UTF-8 string, or the common canonical
`{"encoding":"base64","data":"..."}` bytes object. Boolean, bigint, and
SQL-only tagged objects are not values. SQLite may store a number as INTEGER or
REAL, but the binding does not expose that storage-class distinction. Unsafe
input or output is `numeric_out_of_range`; a host never rounds or stringifies
it. A result is exactly `rows` plus `rowsWritten`; inserted identities use SQL
`RETURNING`, not `lastInsertRowId`.

`execute` runs one effectful, non-idempotent statement. `query` runs one
statement in a rollback-only transaction, materializes its result, always
rolls back, returns `rowsWritten: 0`, and leaves no persistent effect without
trying to classify the SQL as read-only. `transaction` runs 1 through 100
statements under serializable all-or-none isolation and materializes every
result before commit. Runtime calls reject multiple statements,
transaction-control SQL, and durable schema changes. Schema history is applied
only through `SQLiteMigrationApplication`
([decision 0034](../../spec/decisions/0034-edge-sql-uses-safe-wire-values-and-rollback-only-queries.md)).

## Why this is one Form

SQL dialect, typing, and isolation are what application code is written
against. An `engine` token selecting SQLite versus other engines would make
one desired document mean different systems on different hosts — exactly
what decision 0008 prohibits.

## What would require a separate Form

A Postgres or MySQL database is a different Form in a managed-database
family. Migration bytes and applying their ordered history are separate
`SQLiteMigrationSet` and `SQLiteMigrationApplication` resources; the database
identity does not absorb schema rollout policy, and the runtime Interface
cannot bypass their append-only ledger.

## Provided Interfaces

`edge.sql@1.0.0`.

## Accepted Bindings

None; it is a binding target (`module-worker.sqlite`).

## Lifecycle risks

Deleting a database bound by any Worker Version must fail with
`dependency_in_use`. Delete destroys the data; portable state never carries
backups or connection material.

## Prior art

The embedded per-application SQLite database of a proven edge platform. The
retained v1alpha2 `RelationalDatabase` candidate, whose open `engine` token
this Form replaces by fixing SQLite as the shape.
