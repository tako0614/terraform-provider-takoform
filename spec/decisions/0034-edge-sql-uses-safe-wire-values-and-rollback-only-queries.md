# 0034 — edge.sql uses safe wire values and rollback-only queries

- Status: accepted
- Date: 2026-08-10
- Owners: Takoform maintainers
- Supersedes: the `edge.sql`-specific context, decision, fixtures, and
  consequences of
  [decision 0020](0020-the-edge-interfaces-state-their-data-and-delivery-model.md)
  only

## Context

Decision 0020 corrected an earlier SQL value model by projecting SQLite's five
storage classes as tagged objects. That preserved every 64-bit SQLite INTEGER,
but it made SQLite's private storage choice part of the application API, made a
normal JavaScript number and string invalid unless wrapped, invented a second
binary envelope beside the family-wide encoded-bytes shape, and retained
`lastInsertRowId` as special statement metadata.

That is the wrong portability boundary. A Module Worker is authored against
JavaScript values. JavaScript has one binary64 `Number`, not separately
observable INTEGER and REAL value types, and `BigInt` does not cross the JSON
operation boundary. Pretending otherwise makes every host preserve a
distinction consumers cannot use while still requiring special handling for a
64-bit value they cannot represent safely.

The old `query` rule had a second defect. It promised idempotency by requiring a
host to classify SQL as reading before executing it. SQLite statements can hide
effects behind common-table expressions, functions, pragmas, extensions, and
future syntax. A text classifier is therefore part of the database semantics
without being SQLite, and two hosts can disagree about the same statement.

The Edge Platform Family is publication-frozen and every affected Form,
Interface, and Binding candidate remains unpublished. This correction can
therefore keep the human-facing `1.0.0` and `0.1.0` version strings while
replacing their exact digests. No published identity is rewritten.

This decision supersedes only the SQL portions of decision 0020: its paragraph
explaining why the earlier untagged model could not carry SQLite, its section
"`edge.sql` values are tagged by storage class", its SQL fixture model, and the
SQL-specific digest consequences derived from them. Decision 0020 remains the
record for `edge.kv`, `edge.objects`, `edge.queue`, `worker.service`, the cron
grammar, and the common encoded-bytes representation.

## Decision

### EdgeSqlValue is one portable wire union

Every bound parameter and returned column is exactly one of:

```text
null
finite binary64 number with abs(value) <= 9007199254740991
UTF-8 string
{"encoding":"base64","data":"..."}
```

The object is the common encoded-bytes shape already used by the family. Its
`data` is canonical RFC 4648 section 4 base64: padded, unwrapped, and decoding
to the BLOB bytes. A SQL-only `{type: ...}` tag, boolean, `BigInt`,
`ArrayBuffer`, typed array, and raw byte string are not EdgeSqlValue wire
values. A host adapter may use a backend-native byte type internally, but it
decodes on ingress and emits the canonical object on egress.

All numbers are binary64. SQLite may store one as INTEGER or REAL according to
its own affinity and expression rules; the wire does not expose that storage
class and a host must not tag, stringify, or otherwise preserve it as a second
portable type. The SQL program can still call SQLite functions such as
`typeof()`; what is absent is a storage-class tag in the returned value.

Non-finite numbers and numbers whose absolute value exceeds
`Number.MAX_SAFE_INTEGER` fail `numeric_out_of_range`, on both input and
output. A host never rounds, clamps, stringifies, substitutes null, or commits a
write before discovering that its output cannot be represented. A malformed
value or malformed base64 is `sql_error`. The Interface Definition meta-schema
admits lower-snake-case operation error codes including
`numeric_out_of_range`, so the exact code is declared by all three operations
rather than collapsed into `sql_error`.

`lastInsertRowId` is removed. It is not defined for every insert, is ambiguous
with triggers and multi-row statements, does not apply to `WITHOUT ROWID`
tables, and can exceed the portable number corridor. Applications that need an
inserted identity use SQL `RETURNING`, whose value follows the same EdgeSqlValue
rules as every other result column.

### Limits are part of the exact contract

The existing limits remain:

- `maxStatementBytes = 100000`;
- `maxBoundParameters = 100`;
- `maxStatementsPerTransaction = 100`;
- `maxRowsPerStatement = 10000`;
- `maxColumnsPerRow = 100`.

The contract additionally fixes:

- `maxColumnNameBytes = 128`;
- `maxTextBytesPerValue = 1000000`;
- `maxBlobBytesPerValue = 1000000`;
- `maxRowBytes = 2000000`;
- `maxResultBytesPerCall = 8388608`.

SQL text, column names, and TEXT use their UTF-8 byte length. BLOB uses decoded
byte length. The JSON Schema `maxLength` values are structural ceilings; the
byte limits remain normative for multi-byte text. `maxRowBytes` is the UTF-8
length of the RFC 8785 canonical JSON representation of one returned row, and
`maxResultBytesPerCall` is the same measurement over the complete operation
output, including every transaction result. This makes the budget independent
of host object ordering and number spelling.

An input over a structural or byte limit is refused before SQL executes. A
row, column, value, number, or combined output over a limit fails the operation
before its write transaction commits. The host never truncates rows, columns,
text, BLOBs, or a transaction's result list to fit.

### Runtime SQL is one statement and cannot own migrations

`execute` and `query` accept one statement. Each member of `transaction` is one
statement. A host uses SQLite's parser boundary: after the prepared statement,
only whitespace and comments may remain. A second statement is `sql_error`;
splitting on semicolons is not the contract.

Every runtime operation refuses transaction-control SQL — `BEGIN`, `COMMIT`,
`END`, `ROLLBACK`, `SAVEPOINT`, and `RELEASE` — with `sql_error`. The operation
owns its transaction boundary, so SQL inside it cannot open, close, or nest a
different one.

Every runtime operation also refuses a statement that would change the durable
schema, database attachment set, or migration ledger. Durable schema change is
owned only by the `SQLiteMigrationApplication` administrative path, which pins
an exact `SQLiteMigrationSet`, validates its append-only path-and-digest prefix,
and commits each migration with its ledger entry. The admin path may execute a
migration file according to that contract; it is not an invocation of
`edge.sql` and does not weaken the runtime restrictions.

### The three operation boundaries are exact

`execute` runs one statement and is effectful and non-idempotent. It returns
exactly `{rows, rowsWritten}`. It materializes and validates the complete result
before the statement's implicit transaction commits, so a numeric or output
limit failure applies nothing. A transport failure after an effect committed
is still an unknown outcome the caller must resolve; the contract does not make
an unsafe retry safe.

`query` runs one statement inside a rollback-only transaction. It fully
materializes and validates the result, always rolls the transaction back, then
returns exactly `{rows, rowsWritten: 0}`. A statement may transiently write
while producing rows; the API promises zero persistent effects without
pre-classifying it as read-only. SQL failure, cancellation, output conversion,
or a limit failure takes the same rollback path. This executed boundary, not a
text classifier, is what makes `query` idempotent.

`transaction` accepts 1 through 100 statements and runs them in order under
serializable isolation over one consistent snapshot. It returns exactly one
`{rows, rowsWritten}` result per statement, in order. Every row and result is
materialized and validated before commit. Only then do all effects commit
together. Any SQL, numeric, materialization, busy, cancellation, or limit
failure rolls everything back and returns no partial result list.

## Consequences

- `edge.sql@1.0.0`, `module-worker.sqlite@1.0.0`, `SQLiteDatabase@0.1.0`,
  and every Form that embeds one of those exact refs receive new digests while
  retaining their unpublished version strings.
- The dependent closure includes at least `SQLiteMigrationApplication`,
  `WorkerVersion`, and `WorkerDeployment`; their exact Form and package pins,
  the provider v3 registry, the portable-host-v3 corpus, and the website
  projections are regenerated together.
- Interface fixtures use ordinary values, safe-number boundaries, canonical
  encoded BLOBs, multi-statement refusal, transaction-control refusal, and the
  admin-only schema boundary. They no longer create schema through runtime SQL
  or assert `lastInsertRowId`.
- The Host API conformance lane still proves only that a host advertises the
  exact Interface digest. A deployed data-plane run must prove rollback-only
  query effects, transaction atomicity, output bounds, and adapter conversion.
  The publication blocker for a real SQLite backend remains open.
- None of this authorizes publication, provider release, tag creation, push,
  deploy, or a production migration.
