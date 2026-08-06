# 0008 — Forms preserve proven service shape

- Status: accepted
- Date: 2026-08-06
- Owners: Takoform maintainers

## Context

The nine v1alpha2 candidates abstract by resource category: one `EdgeWorker`,
one `RelationalDatabase`, one `KeyValueStore`, one `Queue`. Category-level
generalization forced high-freedom value tokens — `runtime`, `engine`,
`consistency`, `ordering`, `persistence`, `permissions`, `projection` — to
absorb every difference between real services. Two conforming hosts reading
the same desired document could legitimately produce observably different
systems: a Postgres and a SQLite database, an eventually consistent and a
linearizable store, a FIFO and an at-least-once queue. That is not
abstraction; it is hollowing out the contract.

What Takoform actually needs to make exchangeable is not the meaning of a
resource but the business entity implementing it. The value of a proven
service primitive — its execution model, client API, consistency, delivery
guarantees, binding surface, update units, and failure behavior — is exactly
what an application depends on and exactly what a portable contract must
preserve.

## Decision

Takoform does not define least-common-denominator resources reduced from
multiple clouds. A current Form fixes, completely, the application-visible
semantics of one proven service primitive:

- execution ABI and client/runtime API;
- data model, consistency, and persistence;
- delivery guarantees, retry, and duplicate behavior;
- transaction boundaries and addressing;
- update, replace, delete, and migration units;
- error semantics and the capabilities exposed through Bindings.

Only provider- or host-specific facts stay outside the contract: accounts,
regions, SKUs, prices, credentials, native IDs, and internal implementation.

Implementations with different semantics are separate Forms; they are never
selected by a free token inside one Form. A host supports a Form by
implementing that shape with the same meaning; no host is obligated to
implement every Form.

A Form is the smallest **semantically complete** desired-state contract that
independent hosts can implement with the same application-visible behavior. A
field or semantic rule must not be removed merely because one host can choose
it privately. If two conforming hosts could make observably different choices
after a field or rule is omitted, the Form is incomplete.

Where a difference appears determines where it lives:

| Difference | Placement |
| --- | --- |
| ABI, client API, consistency, delivery semantics | separate Form |
| lifecycle or identity unit | separate Form or separate Resource |
| inter-resource connection API | exact Binding contract |
| algorithm choice within identical semantics | closed enum |
| host capability ceilings | Host Support Profile |
| capacity, price, region, quota | Service Offering |

Free semantic tokens are prohibited in current Forms. A closed enum is
permitted only when the API shape, storage model, and lifecycle are identical
and the value selects an internal algorithm (for example a vector distance
metric). Namespaced non-portable extensions must be explicitly marked and
excluded from the portable core.

## Consequences

- The v1alpha2 category-shaped candidate line is superseded for new design
  work. Its bytes, identities, and generator remain retained provider-v2
  preview source and are not overwritten.
- New Forms are authored per service shape: a JavaScript Module Worker, an
  eventually consistent edge KV namespace, a strongly consistent object
  bucket, a SQLite database, an at-least-once queue are each one Form; a WASI
  function, a linearizable KV store, a Postgres database, a FIFO queue are
  each a different Form.
- The `portability-boundary.md` "smallest contract" rule is restated as
  smallest **complete** contract.
- Host Support Profiles, not desired fields, express what a host can accept.

## Rejected alternatives

- **Keep open tokens with a central token registry.** Rejected because a
  registry of tokens still lets one Form mean many things; the contract stays
  unverifiable and hosts still diverge on everything the token does not say.
- **Model categories with optional feature flags.** Rejected because optional
  semantics make every consumer program against the intersection, reproducing
  the least common denominator this decision rejects.
- **One Form per provider product name.** Rejected because product identity is
  a distribution fact; the Form fixes semantics and deliberately omits the
  vendor's name, account, and commercial surface.
