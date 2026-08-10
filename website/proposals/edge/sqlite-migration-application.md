# SQLiteMigrationApplication — `takoform_sqlite_migration_application`

## Workload and consumer

An operator attaches one exact ordered SQLite migration set to one exact
database and asks the host to apply only its unapplied suffix.

## Role

`attachment`. Both `database` and `migrationSet` are immutable exact-Form
relations; changing either creates another application.

## Observable semantics

The database carries a durable ordered ledger of migration path and blob
digest. Before execution the host proves that ledger is an exact prefix of the
referenced `MigrationBundle`. Removal, reorder, or checksum change fails
`migration_required` and mutates nothing. Each SQL file and its ledger record
commit in one SQLite transaction; retry resumes at the first unapplied entry.
Applications serialize per database and Ready means ledger equals the exact
set
([decision 0033](../../spec/decisions/0033-edge-app-assets-and-sqlite-migrations-are-content-addressed.md)).

Deleting the attachment stops managing the relationship only. It never runs
down SQL, erases ledger entries, changes schema, or deletes the database.

This administrative path is deliberately separate from `edge.sql`. Runtime
`execute`, `query`, and `transaction` reject durable schema changes,
multi-statement input, and transaction-control SQL; only this attachment may
advance schema through the exact committed migration history
([decision 0034](../../spec/decisions/0034-edge-sql-uses-safe-wire-values-and-rollback-only-queries.md)).

## Why this is one Form

The database, desired history, and act of applying that history have distinct
lifecycles. The attachment makes their relation explicit and deletion safe.

## What would require a separate Form

Rollback, destructive reset, baseline adoption, or repair of a checksum
mismatch requires explicit authority and semantics; none is inferred here.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

Wrong-kind or unavailable manifests, relation drift, concurrent applications,
failed SQL, and rewritten history all fail closed. Database and migration-set
deletion are blocked while the attachment lives. Removal does not make the
database's retained data recoverable or disposable; that remains database
policy.

## Prior art

Database-local migration journals that pair an ordered filename with its exact
checksum and apply only append-only suffixes.
