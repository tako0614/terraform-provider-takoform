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
parameters, SQLite's SQL dialect and dynamic typing, serializable
transactions applying atomically over a single consistent snapshot, `busy`
under write contention, and closed errors.

Values are TAGGED by storage class — null, integer, real, text, blob — so a
64-bit INTEGER (canonical decimal text) and a BLOB (base64) round-trip
losslessly instead of being flattened into a JSON scalar that cannot hold
them. Every statement reports the same result whether it ran alone or inside
a transaction, so a `SELECT` inside a transaction returns its rows
(decision 0020).

## Why this is one Form

SQL dialect, typing, and isolation are what application code is written
against. An `engine` token selecting SQLite versus other engines would make
one desired document mean different systems on different hosts — exactly
what decision 0008 prohibits.

## What would require a separate Form

A Postgres or MySQL database is a different Form in a managed-database
family. Migration sets and their application are separate revision-shaped
resources in the family plan (`SQLiteMigrationSet`,
[spec/form-families.md](../../spec/form-families.md)).

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
