# SQLiteMigrationSet — `takoform_sqlite_migration_set`

## Workload and consumer

An application records an ordered, reviewable sequence of SQL files that moves
one SQLite database forward.

## Role

`revision`. The whole desired state is one committed `manifestDigest`; any
change to a migration path, digest, or order is a new set.

## Observable semantics

The referenced artifact manifest has kind `MigrationBundle`. It carries an
ordered non-empty `files` list, every entry uses `application/sql`, and it
carries no module members. Order is semantic migration order. SQL bytes live in
the content-addressed blob store and never in Form desired state or provider
state
([decision 0033](../../spec/decisions/0033-edge-app-assets-and-sqlite-migrations-are-content-addressed.md)).

## Why this is one Form

The sequence is reviewed and applied as one immutable history target. Separate
file resources would make ordering an external convention and admit a partially
declared set.

## What would require a separate Form

Another database dialect, reversible migration language, or schema declarator
is a different contract rather than another field.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

Rewriting an applied file is not an update; the application refuses any set
whose ordered path+digest sequence no longer extends the database ledger.
Deleting the set is blocked while an application references it.

## Prior art

Ordered file-based SQLite migration bundles with checksum history, made
content-addressed so a filename alone can never hide changed SQL.
