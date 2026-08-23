# 0047 — The Host API is a substrate a Form declares against, and the two version apart

- Status: Accepted
- Date: 2026-08-23

## Context

Three versions exist in this specification and each has its own line: the Host
API lane (`forms.takoform.com/vN`), a Form Family group
(`edge.forms.takoform.com/vN`), and a Form's own `definitionVersion`. They are
separate on purpose — [decision 0039](0039-a-lane-is-minted-for-one-of-two-reasons.md)
restricts when a lane may move at all, and
[decision 0038](0038-a-generation-move-is-measured-not-assumed.md) measures what a family
move actually re-identified.

They have nevertheless moved together every time. The Beta 2 generation moved
the lane and the family in one change, as the Beta 1 generation had before it.
The maintainer named the smell (2026-08-23): *why, when each specification has
its own version, is this being done all at once?*

The answer is that **nothing in a Form's contract says which Host API it
needs**, so nothing ever licensed moving one without the other. A Definition
declares the exact Interfaces it PROVIDES, the exact Bindings it ACCEPTS, and
the exact contracts its relations REQUIRE of their targets — every dependency
it has on another contract is exact and stated, except the one every Form has:
the protocol a host must speak to serve it at all. That one was carried by
convention, and convention has no version, so the only safe move was to move
everything.

The same review found the family's published surface carrying something that
is not the family's: `candidate-set.json`, served at takoform.com, names a
`resourceType` — `takoform_module_worker` — for each Form. That is the
Terraform provider's authoring name. A Form fixes the shape of a cloud
primitive ([decision 0008](0008-forms-preserve-service-shape.md)); how an
author WRITES it is the province of whichever client renders it, and a second
client rendering the same Form differently is not a conformance question. The
published Definition itself was already clean — no HCL names, no resource type,
wire names throughout. Only the index beside it was not.

## Decision

**The Host API lane is a SUBSTRATE. A Form Definition declares the minimum
lane it requires, and the lane and the family version apart.**

1. **A Definition carries `requiresHostApi`**: the exact identity of the
   earliest Host API lane whose rules its contract needs. A host installing a
   Form whose requirement its own lane does not satisfy refuses it with
   `unsupported_capability`, at install time, naming both identities. A Form
   that needs nothing newer than the first Beta lane says so and runs on every
   host that serves it, whatever generation its family is on.

2. **The requirement is a LOWER BOUND, not a pin**, and the asymmetry with
   [decision 0022](0022-relations-pin-the-target-contract.md) is deliberate.
   A relation pins its target's exact contract because the target is another
   resource whose Definition may move underneath it and whose meaning is
   nobody's obligation to preserve. A lane is not another resource: it is the
   protocol the host serves, its compatibility across generations is the
   LANE's own obligation, and decision 0039 already forbids minting one for
   any reason but a protocol change or an evidenced graduation. Pinning it
   would say that every Form must be re-minted whenever the protocol moves —
   which is the lockstep this record exists to end, wearing a different name.

3. **A family generation and a lane generation are separate events.** A family
   moves when its shapes change; a lane moves for decision 0039's two reasons.
   Neither implies the other, and the continuity ledger measures the family's
   move alone. Members of one generation MAY declare different lane
   requirements: what a Form needs from the substrate is a property of that
   Form's contract, not of the group it was minted in.

4. **A family's published surface carries no client's authoring vocabulary.**
   `resourceType` leaves the candidate set. A provider derives its own resource
   names from `kind`, and two providers rendering one Form differently are both
   conforming.

## Enforcement

The Definition profile requires `requiresHostApi`, so a Form minted without
one cannot be published. A host's install path compares it against the lane it
serves and refuses a Form it cannot honour; the conformance corpus drives that
refusal, because an unenforced requirement would be a comment. The generation
gates stop treating a family move and a lane move as one event: each is checked
against its own record.

## Consequences

The next family generation can move without touching the lane, and the next
protocol change can move without re-minting seventeen Forms. The cost is one
more member on every Definition and one more thing a host must check —
which is exactly what the previous arrangement was hiding.

This record does not renumber anything already minted. The Beta 2 identities
keep the numbers they have; what changes is that the NEXT move is not required
to be simultaneous.
