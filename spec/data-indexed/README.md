# `data.indexed@1`

`data.indexed@1` is the portable bounded data plane required by
`SQLDatabase@2.0.0`. It operates only on tables, primary keys, and indexes
declared in the Resource's exact Form desired state. It is not a general SQL
API.

## Interface declaration

The Form declares one required Interface with exact identity
`data.indexed@1` and one endpoint shape:

```text
POST /indexed/v1/operations
```

The request body validates against [`request.schema.json`](request.schema.json)
and every HTTP 200 or 409 body validates against
[`response.schema.json`](response.schema.json). The Interface descriptor pins
both files by repository `specPath` and RFC 8785 SHA-256 digest.

The request is a closed union selected by `operation`:

| Operation | Selection and effect |
| --- | --- |
| `get` | Fetch one row by the complete declared primary key. |
| `get_unique` | Fetch one row by the complete declared unique index key. |
| `page` | Page one declared index in its declared ascending order by an exact leading-key `prefix` and optional range on the next key column. Cursor pagination only. |
| `put` | Insert or replace one row by primary key, optionally fenced by integer `expectedRevision`. |
| `delete` | Delete one row by complete primary key, optionally fenced by integer `expectedRevision`. |
| `batch` | Apply one to 25 `put`/`delete` mutations atomically within the Resource. |

Every successful operation returns HTTP 200 and one of these closed shapes:

| Operation | HTTP 200 response |
| --- | --- |
| `get` | `{"operation":"get","item":null}` or an `item` containing `row` and `revision`. |
| `get_unique` | `{"operation":"get_unique","item":null}` or an `item` containing `row` and `revision`. |
| `page` | `{"operation":"page","items":[...],"nextCursor":null|string}` with at most 100 row/revision items. |
| `put` | `{"operation":"put","item":{"row":...,"revision":...}}`. |
| `delete` | `{"operation":"delete","deleted":boolean}`. |
| `batch` | `{"operation":"batch","results":[...]}` with one operation-tagged `put` or `delete` result for each input mutation, in input order. |

Returned rows contain exactly the declared columns. An omitted nullable input
column is returned as explicit JSON `null`; key columns and declared non-null
columns cannot be omitted. A row result always carries its positive safe
integer `revision`. Request-side `expectedRevision` separately permits zero as
a create-if-absent fence.

Whole-operation optimistic-concurrency and uniqueness conflicts return HTTP
409. They do not return partial batch results:

```json
{
  "operation": "put",
  "conflict": {
    "reason": "revision_conflict",
    "table": "records",
    "key": { "id": "r1" }
  }
}
```

```json
{
  "operation": "batch",
  "conflict": {
    "reason": "unique_conflict",
    "table": "records",
    "index": "by_tenant_created"
  }
}
```

`revision_conflict` is valid for `put`, `delete`, and `batch`, and its `key` is
the normalized complete primary key. `unique_conflict` is valid for `put` and
`batch`. The response schema covers only operation success (200) and these
whole-operation conflicts (409). Validation, authentication, authorization,
rate-limit, and runtime-error envelopes remain host-owned.

The host validates table, column, key, prefix, range, and index references
against the declared schema. A range may choose at most one lower bound (`gt`
or `gte`) and one upper bound (`lt` or `lte`).

Caller-provided SQL, DDL, arbitrary filters, arbitrary ordering, and offset
pagination are outside this contract and are rejected. There is no portable
escape hatch for them.

## Canonical ordering

`page` has exactly one portable order: ascending. A caller cannot request
descending or arbitrary order. Hosts compare declared key values as follows:

- `string`: unsigned lexicographic comparison of the original UTF-8 bytes,
  with no Unicode normalization, locale collation, or case folding;
- `integer`: signed numeric comparison;
- `boolean`: `false` before `true`;
- composite index: lexicographic comparison in declared index-column order,
  followed by primary-key columns not already present in the index, in declared
  primary-key order.

The appended primary-key values make the full sort tuple immutable and unique.
Primary-key and index columns are required and non-null; null or missing key
values have no portable order and must fail closed. `prefix` must name an exact
contiguous leading set of index columns. A range, when present, must name only
the immediately following index column.

`nextCursor` is `null` at the end of a page chain. Otherwise it is an opaque,
tamper-evident token no larger than 64 KiB and valid for at most 900 seconds.
It is bound to the Resource identity and generation, exact Form and declared
schema, exact query/filter/order, and the last complete canonical sort tuple.
The host must reject an invalid, mismatched, or expired cursor without falling
back to a broader query. HMAC or equivalent key material and rotation remain
host-private.

Continuation is exclusive live keyset pagination. With an unchanged dataset,
following cursors produces no duplicate or omitted rows. The contract does not
promise snapshot isolation: concurrent writes may change membership in later
pages. Offset scans remain forbidden.

## Declared schema bounds

| Bound | Maximum |
| --- | ---: |
| Tables per Resource | 16 |
| Columns per table | 32 |
| Indexes per table | 8 |
| Columns per primary key or index | 4 |
| Page rows | 100 |
| Batch mutations | 25 |
| Encoded row | 8 KiB |
| UTF-8 string value | 4 KiB |
| Encoded request | 1 MiB |
| Encoded result | 1 MiB |
| Opaque cursor (UTF-8) | 64 KiB |
| Cursor lifetime | 900 seconds |
| Revision | 9,007,199,254,740,991 |

Portable column types are `string`, `integer`, `number`, and `boolean`.
Primary-key and index columns must be non-null `string`, `integer`, or
`boolean` columns. `number` is intentionally excluded from keys so hosts do
not need to normalize floating-point ordering or equality. Every `integer` or
`number` column value—and every integer key, prefix, and range bound—is limited to
`-9,007,199,254,740,991..9,007,199,254,740,991` so JSON implementations agree
on the portable numeric range and exact integer identity.

The complete `tables` declaration is immutable in `SQLDatabase@2.0.0`.
Changing it replaces the Resource. This version does not define an in-place
schema migration operation.

JSON Schema provides the structural/cardinality checks. The host must also
enforce UTF-8 byte sizes, encoded row/request/result sizes, declared-key
semantics, the 64 KiB cursor byte bound, cursor integrity, revision fencing,
and batch atomicity. JSON Schema's cursor `maxLength` counts characters, so it
does not replace the host's UTF-8 byte check. Revisions are non-negative JSON
integers capped at the largest integer exactly representable in all common
JSON number implementations. A limit
failure is not permission to truncate a write or silently broaden a read.

## Host boundary

The descriptor maps public Form outputs for Resource identity, name,
generation, `schemaVersion`, and tables. Its `resourceUriInput` names the
portable `resource_uri` input: the host supplies the canonical
credential-free HTTPS OAuth resource URI used as an audience fence. That
non-secret URI grants no access.

Takoform owns this descriptor, request schema, fixtures, and conformance data.
The host owns the Interface record and endpoint, token issuance,
InterfaceBinding/consumer authorization, tenant fencing, routing, runtime
implementation, lifecycle, Target selection, credentials, quota, and billing.
