# Table — `takoform_table`

> Historical/deferred candidate (English-only). This non-Edge Form is not in
> the current official Edge16 corpus or Current navigation.

## Workload and consumer

An application keeps key-addressed records — profiles, orders, sessions,
counters, per-tenant settings — as items in a document table where every
access names a declared partition key. Workers will consume it through
`module-worker.table` bindings; management is the ordinary resource
lifecycle.

## Role

`identity`. Its desired fields declare which attributes ADDRESS data — the
key schema, the secondary indexes, the TTL attribute — and nothing about how
the table behaves: the data-plane semantics are fixed entirely by the
`table.document` Interface, the way `SQLiteDatabase`'s are by `edge.sql`.

`partitionKey` and optional `sortKey` are each `{name, type}` with type from
the closed set `string | number | bytes`, fixed at creation: a change to
either is refused before mutation and plans as replacement.
`secondaryIndexes` is a closed list of `{name, partitionKey, sortKey?}` over
top-level attributes, index names in the portable resource-name grammar;
adding or removing an entry is an in-place update the host materializes
(readiness below), and an index is redefined only by removing it in one
update and re-adding it later. A host MUST accept at least 20 declared
indexes; a higher ceiling is Host Support Profile material. `ttlAttribute`
optionally names one top-level number attribute, set or cleared in place.
Attribute names are UTF-8, 1 through 255 bytes.

## Observable semantics

Exactly the `table.document@1.0.0` contract: `get`, `put`, `delete`, and
`query`, with consistent-by-default reads on the base table, per-item
last-writer-wins, atomic conditional writes, key-ordered queries within one
partition, cursor pagination, closed errors, and portable minimum limits.

An item is a closed document value: null, boolean, finite binary64 number,
UTF-8 string, the family's encoded-bytes object, a list, or a string-keyed
map, to a declared depth (portable minimum 32). The encoded-bytes shape is
RESERVED: a map whose member set is exactly `{encoding, data}` with
`encoding` equal to `base64` is a bytes value, so the document model cannot
express a plain map of that exact shape — stated here rather than
discovered. `maxItemBytes` (portable minimum 409600) bounds the UTF-8
encoding of the item's canonical JSON. Key attributes are top-level and
typed as declared: string and bytes keys are bounded at 2048 bytes
(partition) and 1024 bytes (sort), measured decoded, ordered by unsigned
byte order of their encoding; a number in KEY position must be an integer
inside the safe corridor (magnitude at most 2^53 − 1), because a key is an
identity and must compare and order identically across hosts and storage
engines, while a non-key number is any finite binary64. An item missing a
key attribute, or carrying the wrong type there, fails `invalid_key`; a
document outside the value model fails `invalid_value`; an oversized item
fails `value_too_large`.

`get` reads one item by full primary key, and a get after a resolved put or
delete observes it — single-item reads are consistent by default, and there
is no weaker read to select. An absent item fails `not_found`. `put`
replaces the whole item at its key; `delete` removes it and succeeds when it
is already absent. Either write MAY carry a condition — `exists` (whether
the item exists) and/or `equals` (exact top-level scalar attribute values),
conjoined — evaluated and applied as one atomic step. A failed condition
changes nothing and fails `precondition_failed`, the code the retained
v1beta1 `edge.objects` Interface used for the same historical meaning: a
caller-stated guard did not hold and the write did not happen. `edge.objects`
is not a current Interface and supplies no current dependency here. A retried conditional write that already
applied may therefore fail its own condition; conditional writes are not
blindly retryable.

`query` names one partition-key value, an optional sort-key condition from
the closed set `eq | lt | lte | gt | gte | between | beginsWith`
(`beginsWith` on string and bytes sort keys only), a direction, a limit, and
a cursor; it returns items in sort-key order and a cursor when
`maxQueryPageItems` (1000) or `maxResultBytesPerQuery` (1048576) ends the
page. A malformed cursor fails `invalid_cursor`. A table without a sort key
holds at most one item per partition-key value. A query naming `indexName`
runs against a declared secondary index instead: index queries are
eventually consistent — an acknowledged base write may be missing, and a
deleted item still present, until the index converges — and an item lacking
an index's key attributes simply does not appear there (sparse indexes).
Indexes project the full item. Observed state reports each index as
`backfilling` or `ready`; a query against a backfilling index fails
`resource_busy`, chosen from the existing taxonomy because the index exists
and the condition is transient, expected, and retryable — `not_found` would
deny a declared index, and `backend_unavailable` would blame infrastructure
for a defined lifecycle state. A query naming an undeclared index fails
`not_found`.

When `ttlAttribute` is set, an item whose named attribute holds a number is
eligible for deletion once that epoch-second time passes. Expiry is lazy: an
expired item MAY remain visible to every read — and satisfies `exists`
conditions — until the host reaps it, which it must eventually do; reaping
removes it from the base table and every index with no observable event. An
item whose TTL attribute is absent or not a number never expires.

## Why this is one Form

Key-addressed items, atomic conditional writes, consistent single-item
reads, and key-ordered partition queries are one inseparable model that
application code is written against; a consistency selector or an open query
language would hollow the contract out (decision 0008). The secondary
indexes live in the table's desired state because the proven shape declares
them there and an index changes what THIS table can answer; a separate index
resource would carry a uid and relation lifecycle while meaning nothing
apart from its table.

## What would require a separate Form

A table scan or any cross-partition query is a different operational
contract — unbounded read amplification with its own pagination and
consistency story — and is deliberately absent from the MVP; it arrives, if
ever, as a versioned Interface addition or a separate Form. An
eventually-consistent-by-default table is a different Form. An
update-expression mutation surface, multi-item transactions, and a change
feed are future versioned Interface additions or attachment work, never
selectors on this Form. A document store addressed by a query language is
not this category at all: the Mongo-flavored shape has a de-facto standard
wire and is an integration, never a Form (decision 0043).

## Provided Interfaces

`table.document@1.0.0`.

## Accepted Bindings

None; it is a binding target (`module-worker.table`, a future binding Form
mirroring `module-worker.edge-kv`). Consumers outside the worker family
reach the same Interface only when the cross-family binding projection
exists; that realization is its own work and is not defined here.

## Lifecycle risks

Deleting a table bound by any consumer revision must fail with
`dependency_in_use` (refuse_while_bound). Delete destroys every item and
index; there is no portable restore. Removing a secondary index discards its
materialization immediately; re-adding the name later starts a fresh
backfill.

## Prior art

The DynamoDB-like document/KV table category of the major clouds — offered
by every one of them, portable to none (decision 0043). The withdrawn
v1alpha2 `KeyValueStore`, whose open `consistency` enum this Form replaces
by fixing one shape, and the key-addressed half of the withdrawn
`StatefulEntity`; its addressable-actor half became the Edge family's
`ActorNamespace` addition.
