# 0048 — The protocol states mechanisms; a family instantiates them

- Status: Accepted
- Date: 2026-08-23

## Context

[Decision 0047](0047-the-host-api-is-the-substrate-a-form-declares-against.md)
fixed one direction of a dependency the maintainer had named: a Form never said
which Host API it needed, so the two could only move together. The maintainer
then named the other direction (2026-08-23): *the API is still updated whenever
the Forms are; make the API declare an extensible substrate, and let each
Form's specification define the details on top of it, so API changes are
avoided.*

Measured, the complaint was exact. `spec/host-api/v1beta2.md` — a protocol
document — named Edge Form kinds **66 times across 16 kinds**, concentrated in
the attachment gate, the worker aggregate's cardinality rules, the endpoint's
assigned address, reverse validation, and the SQLite migration ledger. That is
why adding two Forms in the Beta 2 generation forced edits to the protocol: the
protocol contained the family.

The reference host had the same shape. Four separate hand-written functions
enforced four cardinality rules — one active deployment per worker, one
consumer per queue, one live migration application per database, one class
holder per worker and class — each of which was also a paragraph naming a Form
kind in the protocol document.

## Decision

**The protocol states MECHANISMS. A family instantiates them in its Form
Definitions. The protocol document names no Form kind of any family.**

The criterion is mechanical rather than editorial: a gate collects every kind
from every family's candidate set and fails if one appears in a lane document
that declares itself Form-neutral. A lane either carries that property and is
checked for it, or does not claim it.

`forms.takoform.com/v1beta3` is minted for this, which is
[decision 0039](0039-a-lane-is-minted-for-one-of-two-reasons.md)'s first
reason: the wire contract changes, because what a Definition may declare and
what a host must read out of it both change.

The mechanisms this lane declares, each carried by the Definition and read by
the host:

| Mechanism | What it replaced |
| --- | --- |
| `x-takoform-exclusive` | four hand-written cardinality rules, and four paragraphs naming Form kinds |
| `x-takoform-sum` | the sentence about one Form's traffic weights totalling 10000 |
| `x-takoform-claim` | the section about one Form's hostname claim |
| `x-takoform-host-assigned` | the section about one Form's assigned address |
| `x-takoform-required-entrypoint` | the closed table of which attachment needs which handler |

What is NOT a protocol mechanism moves to where it belongs rather than being
generalized for its own sake: one family's asset-fallback order and another's
migration ledger are that family's rulebook, and they are stated in
[`../form-families.md`](../form-families.md).

## Enforcement

The no-Form-names gate is the criterion, and it is tied to a property a lane
declares rather than to a lane number, so a future lane that does not claim
Form-neutrality is not silently held to it — and the same flag is what lets the
mint-reason gate accept a lane whose wire bytes are otherwise unchanged.

Every mechanism is measured by a check that was verified to FAIL against a
reference host with that mechanism disabled. A mechanism a corpus does not
exercise is prose, which is the defect this record exists to remove rather than
relocate.

## Consequences

A family may add a Form, or a rule of a shape the protocol already knows,
without the protocol changing. What it may not do is add a rule of a NEW shape
without a reviewed protocol change — which is correct, because a new shape is a
new thing a host must be able to enforce.

The honest cost is that three of the five mechanisms have exactly one
instantiation each. Their generality is asserted rather than demonstrated, and
the second instance is what would prove it. `x-takoform-exclusive`, at five, is
the one that earned its keep — and it is also the measure by which the others
are still provisional.
