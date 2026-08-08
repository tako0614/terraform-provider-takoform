# 0020 — The edge Interfaces state their data and delivery model

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

[Decision 0010](0010-exact-interface-and-binding-contracts.md) made an Interface
an exact, digest-bound contract, and
[decision 0019](0019-the-module-worker-abi-is-an-exact-contract.md) showed what
that costs to do properly by writing the runtime ABI out in full. The four
data-plane contracts of the Edge Platform Family were not held to the same
standard, and each of them left something an application depends on either
unsaid or said twice, differently.

**`edge.kv` contradicted itself.** Its `semantics` declared `consistency:
eventual` while its behavior fixtures required a put to be visible to the very
next get and a delete to make the next get miss. Both cannot be normative: the
fixtures are exactly the trace a correct eventually consistent store is allowed
to fail, so the deterministic set demanded read-your-writes from a contract that
promised none. Its value model measured two quantities at once — `maxValueBytes`
counts bytes, a JSON Schema `maxLength` counts string length — so no host could
tell which one bounded a value, and the answers diverge for every value outside
ASCII.

**`edge.objects` promised an API it did not have.** The description promised
range reads while `get`'s input took only a key, there were no conditional
requests despite the description fencing on etags, there was no multipart path,
and the body was a JSON string while `maxObjectBytes` was 5 GiB. No host can
produce a 5 GiB JSON string and no client can consume one.

**`edge.sql` could not carry SQLite.** Its value model was
`boolean|null|number|string`. A 64-bit INTEGER does not survive an IEEE-754
double — `9223372036854775807` arrives as `9223372036854775808` — and a BLOB has
nowhere to go at all. `lastInsertRowId` was a JSON integer with the same defect,
and a transaction result could report only a write count, so a `SELECT` inside a
transaction had nowhere to put its rows: the atomic path was strictly weaker
than the non-atomic one.

**`edge.queue` left the message model open.** Nothing said what a message body
was, what a message id meant across redeliveries, what the timestamp measured,
how an attempt was counted, how a message was settled, what an uncaught handler
exception did to the rest of the batch, whether the first delivery counted
toward `maxRetries`, what the attempt count was after a dead-letter transfer, or
whether one queue could have two consumers. Every one of those is a thing two
conforming hosts could answer differently.

**The cron grammar rejected the schedules the Form exists for.** The enforced
pattern was five single-value fields, so `* * * * *`, `0 * * * *`, and
`*/5 * * * *` were all refused and the most frequent representable schedule was
once a day. A Form whose entire purpose is periodic invocation could not express
an hourly one.

All five are corrections to unpublished Forms and unpublished Interface
candidates, so they are made under
[decision 0014](0014-published-schemas-are-structural-minima.md): everything
below is expressed within what the already-published Interface Definition
meta-schema admits — operations with input and output schemas, closed error
vocabularies, `semantics`, `limits`, fixtures, and normative descriptions — and
no schema identity is minted.

## Decision

### One encoded-bytes shape, and `edge.kv` values are bytes

A value that is bytes travels as `{"encoding": "base64", "data": "…"}`, with
`data` RFC 4648 §4 base64, padded and unwrapped. The declared byte limit bounds
the DECODED length; the `maxLength` on `data` is the structural ceiling of the
encoding and is not the limit. The shape is the same in `edge.kv` values and
`edge.queue` message bodies, so a value moving from a queue into a namespace
does not change representation on the way. A key stays a UTF-8 string, and
`maxKeyBytes` bounds its UTF-8 encoded length, which is stated rather than
implied.

Because the values are bytes, the name `edge.kv` continues to describe what the
contract is. No rename is needed.

### `edge.kv` stays eventually consistent, and says so where it matters

Consistency remains `eventual`, and the contract now states the consequence
outright: a read that follows a write may return the previous value, or
`not_found`, until replication converges — including a read the same client
issues immediately after its own write, on the same connection, from the same
location. There is no read-your-writes guarantee, no session in which one holds,
and no bound on convergence time; a host states its own convergence target in
its Host Support Profile.

The immediate-visibility fixtures are removed from the deterministic set. What
remains asserts only facts that no write has to converge for: a put is accepted,
a never-written key is `not_found`, deleting an absent key succeeds, and the
listing of an empty scope is complete. Convergence becomes a host obligation,
listed as one in
[`../host-api/v1alpha3.md`](../host-api/v1alpha3.md#what-the-lane-proves-and-what-stays-a-host-obligation).

### `edge.objects` becomes a real object API with streaming bodies

`head`, ranged and conditional `get`, conditional `put`, `delete`, `list` with
delimiter roll-up, and `createMultipartUpload` / `uploadPart` /
`completeMultipartUpload` / `abortMultipartUpload`. Bodies are STREAMS: `get`
and `put` declare `bodyStream` and `contentLength`, and the bytes move beside
the operation document rather than inside it, which is what makes the 5 GiB
ceiling meaningful. Consistency stays `read_after_write` and is stated exactly:
a `get`, `head`, or `list` issued after a `put` or `delete` that already
resolved observes it; writes to one key are last-writer-wins; there is no
cross-key atomicity and no versioning.

The fixtures are consistent with that model and prove what a single client can:
put-then-head, a ranged get returning the declared subrange, a range past the
end refused, a conditional put refused against an existing key, delete-then-head
missing, and a listing truncating at the requested limit. A trace declaring
`bodyStream` sends exactly `contentLength` bytes whose value at index *i* is
*i* mod 256, so every trace is byte-for-byte reproducible. Multipart assembly is
a host obligation: its steps depend on part etags the host mints during the
trace, which a static trace cannot carry.

### `edge.sql` values are tagged by storage class

Every bound parameter and every returned column value is exactly one of
`{"type":"null"}`, `{"type":"integer","value":"<decimal>"}`,
`{"type":"real","value":<number>}`, `{"type":"text","value":"<string>"}`, and
`{"type":"blob","base64":"<base64>"}` — SQLite's five storage classes and
nothing else. There is no boolean member, because SQLite has no boolean storage
class. An INTEGER travels as canonical decimal text, and so does
`lastInsertRowId`; a host refuses one outside the 64-bit two's-complement range
with `sql_error`. Every statement, alone or inside a transaction, reports the
same result — `rows`, `rowsWritten`, and `lastInsertRowId` when it inserted —
so a `SELECT` inside a transaction returns its rows. `query` is declared
idempotent, so a host refuses a writing statement submitted through it.

### `edge.queue` states its message model, and a queue has one consumer

A body is opaque bytes in the shared encoded-bytes shape. A `messageId` is
host-issued, opaque, unique within the queue, and stable across redeliveries.
`timestampMillis` is the UTC instant of acceptance and does not change across
redeliveries. `attempts` is 1 on first delivery and increments on each
redelivery. Settlement is explicit: `acknowledge` and `retry` per message,
`acknowledgeAll` and `retryAll` per batch, all scoped by the `batchId` the
delivered batch carried. A handler that returns without settling anything
acknowledges the whole batch; a handler that throws retries every message it had
not already acknowledged. `maxRetries` counts REDELIVERIES only, so a message is
delivered at most `1 + maxRetries` times and `maxRetries: 0` means one delivery.
A message that exhausts its retries becomes a NEW message on the dead-letter
queue — new id, new acceptance timestamp, `attempts` starting again at 1 — or is
dropped when no dead-letter queue is declared.

One queue has AT MOST ONE consumer. `maxRetries`, `retryDelaySeconds`,
`maxConcurrency`, and the dead-letter destination are properties of how that
queue is drained; two consumers would give one queue two of each with no rule
deciding which message got which, which leaves the queue's own behavior
unstatable.

The queue-producer binding still projects only `send` and `sendBatch`: the
settlement operations exist on the batch the module's `queue` handler receives,
never on a binding, because settling a message you were not delivered is not an
operation. `worker.runtime`'s `queue` operation carries the same `batchId`,
`timestampMillis`, and encoded-bytes body, so the two contracts describe one
message.

### The cron grammar is parsed, not matched

The Worker Cron Trigger accepts five fields where each field is a
comma-separated list of `*`, a literal, a range `low-high`, `*/step`, or
`low-high/step`. Names and a step on a bare literal are refused. The regular
expression carried inside the Form Definition is the STRUCTURAL half — it is
what a host with only the Definition can enforce — and a parser is the semantic
half: it builds an AST, holds each field to its own domain, refuses an inverted
range and a step outside `1..span`, and produces a canonical form so two
spellings of one schedule are comparable. The provider runs that same parser at
plan time, so a configuration that plans is one that applies.

Schedules are UTC only. When both day-of-month and day-of-week are restricted,
a match on EITHER fires the trigger; when one is restricted, only that one
constrains the day. A missed run is not made up. Delivery of a match is
at-least-once, so a handler may run twice for one matched minute and must be
idempotent. An uncaught handler exception is a failed invocation reported to
host diagnostics, not retried within the matched minute.

## Consequences

- Every Interface Definition, the six Forms that reference them, and the four
  binding descriptions that project them change bytes, so the whole digest chain
  is regenerated: Interface digests, Form Definition digests, package digests,
  the provider v3 registry, and the conformance corpus pins.
- Three required conformance checks are added — `edge-interface-contracts-advertised`,
  `cron-grammar-enforced`, and `queue-single-consumer-enforced` — bringing the
  closed list to 79. They are exactly what a black-box Host API runner can
  prove; everything else these contracts state is listed as a host obligation.
- The catalog gains an authoring-time proof for EVERY Interface, not only the
  runtime ABI: operation names are unique and well formed, fixtures exercise
  declared operations, and a fixture expecting a failure names an error that
  operation declares. A fixture expecting an error outside the closed vocabulary
  is a trace no conforming host could pass, which is the same defect class as a
  required check no correct host can complete.
- `KindDateString` lost its last user when `compatibilityDate` was removed in
  decision 0019, and its validator only ever checked the `YYYY-MM-DD` shape, so
  `2026-99-99` passed. Rather than leave a dead field kind with a weak
  validator, the kind and its pattern are removed; a future calendar-date field
  mints one with a real calendar check.
- The Host API lane's "what the lane proves, and what stays a host obligation"
  section grows a second half covering the data-plane contracts, so a reader of
  a passing report still knows which half it covers.

## Rejected alternatives

- **Keep the immediate-visibility fixtures under an eventual claim.** Rejected
  because they are the one trace a correct eventually consistent store is
  allowed to fail: the deterministic set would have excluded exactly the systems
  the contract describes.
- **Promise read-your-writes within a session.** Rejected because it would need
  "session" defined precisely — a connection, an isolate, a location, a
  principal — and no definition of it is true of a globally replicated store
  served from the nearest replica. Stating a guarantee no implementation keeps
  is worse than stating none.
- **Bound the KV value with `maxLength` alone and drop `maxValueBytes`.**
  Rejected because a host stores bytes, bills bytes, and rejects on bytes; a
  code-unit ceiling would be a limit nobody enforces.
- **JSON-string object bodies.** Rejected because the object ceiling is 5 GiB.
  Either the body model or the ceiling had to go, and shrinking an object store
  to what fits in a JSON string makes it not an object store.
- **Narrow `edge.objects` to a small-text store and rename it.** Rejected
  because decision 0008 names the strongly consistent object bucket as one of
  the family's proven service shapes; narrowing it would remove the shape rather
  than fix the contract, and every application that needs an object store would
  be pushed off the family.
- **Untagged SQL values, with the integer widened to a JSON number.** Rejected
  because IEEE-754 double loses INTEGER precision above 2^53 and there is no
  JSON scalar that carries a BLOB at all; a value model that silently changes
  what was stored is worse than one that is verbose.
- **A single `bytes` scalar instead of a tagged union for SQL.** Rejected
  because SQLite's storage class is observable — `typeof()`, comparison order,
  and affinity all depend on it — so collapsing five classes into two would make
  the same query answer differently on two conforming hosts.
- **Let a queue have several consumers and say the split is unspecified.**
  Rejected because "unspecified" is exactly the free-token failure decision 0008
  forbids: retry counts, delays, and dead-letter destinations would differ per
  message with no rule an author could predict.
- **Count the first delivery toward `maxRetries`.** Rejected because
  `maxRetries: 0` would then mean "never deliver", which is not a queue.
- **A daily-only cron under a general name.** Rejected because the Form's whole
  purpose is periodic invocation, and a name that says "cron" while refusing
  `*/5 * * * *` misleads every author who has ever written a crontab. Renaming
  it to something honest about daily-only granularity would have preserved the
  defect under better labelling.
- **Widen the cron regular expression and stop there.** Rejected because a regex
  that admits lists, ranges, and steps also admits `0 24 * * *`, `5-1 * * * *`,
  and `*/0 * * * *`, which name no schedule; a host would store a trigger it can
  never fire, and two hosts would disagree about which ones they accepted.
- **Canonicalise the stored cron expression.** Rejected because the desired
  state is the bytes an author wrote: rewriting them would make every plan drift
  against the host's copy. The canonical form is a comparison form only.
