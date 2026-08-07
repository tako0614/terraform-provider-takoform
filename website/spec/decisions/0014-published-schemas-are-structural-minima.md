# 0014 — Published schemas are structural minima; closure lives in code and conformance

- Status: accepted
- Date: 2026-08-07
- Owners: Takoform maintainers

## Context

The v1alpha3 lane's schema documents were published on 2026-08-06, before the
lane's publication blockers were fixed. Publication is per artifact: no family
Form, Interface, or Binding is published, but the seven meta-schemas that
describe them are, and their bytes are immutable. The append-only ledger and
the deploy no-overwrite guard both reject a byte change, so `bun run check`
fails permanently on any attempt.

Several remaining blockers were designed as schema edits: closing the artifact
manifest per kind with `oneOf`, adding a defaults map to the Form Definition,
adding relation metadata, tightening Interface semantics. Each would now
require minting a replacement identity, and minting one per blocker would
leave a trail of superseded documents served forever.

## Decision

A published schema is the **structural minimum** for its document, not the
complete acceptance contract — the rule [`conformance.md`](../conformance.md)
already states. Closure that a frozen schema cannot express is therefore
placed in the semantic verifier, the host, and the conformance corpus, which
is where the project's other fail-closed rules already live.

When a contract needs a new invariant, prefer the first option that works:

1. **Data the published schema already admits.** A JSON Schema `default`
   inside a Form's desired schema carries portable defaults; the canonical
   definition bytes cover it, so the exact FormRef digest protects it.
2. **Form Definition content.** Definitions are unpublished, so their desired
   schema, required set, capability list, and referenced contracts may change
   freely; only the derived digests move.
3. **Code and conformance.** The host rejects the shape, the authoring model
   rejects the declaration, and a required conformance check proves a laxer
   host fails the lane.
4. **A new schema identity.** Only when the invariant is unexpressible by the
   first three. New identities are then minted **together, once**, as a
   coherent generation — never one per change set.

Two consequences follow. A conforming implementation MUST NOT treat schema
validity as sufficient; the semantic rules named in the owning specification
are equally normative. And the repository MUST NOT publish again until the
lane's contracts are settled, which the ledger enforces mechanically: a
schema byte change turns the gate red rather than reaching production.

## Consequences

- The artifact manifest's per-kind closure (a `WorkerBundle` manifest must not
  carry asset files; an asset or migration manifest must not carry modules) is
  enforced by the host and a required conformance check rather than by
  `oneOf`. The host already had to verify manifest-to-blob agreement, media
  types, and published limits in code, so the schema was never the whole
  contract for this document.
- Relation metadata is derived from the desired schema, where each reference
  already encodes its target kind, instead of being duplicated into a new
  Definition member. Deriving it removes a second source of truth as well as
  the mint.
- The seven published v1alpha3 meta-schemas stay served and retained. They
  describe the lane accurately; what they do not describe is stated in the
  owning specification and proven by conformance.
- Publishing a schema before its contract is settled is expensive. Future
  lanes SHOULD keep meta-schemas unpublished until their first Form is ready
  for publication.

## Rejected alternatives

- **Mint a replacement identity per blocker.** Rejected because it publishes a
  superseded document for every change set, and because the resulting version
  story teaches consumers to expect churn in exactly the surface that is meant
  to be immutable.
- **Edit the published bytes and record an exception.** Rejected because it is
  mechanically impossible without weakening the append-only ledger, and the
  ledger is the only thing making a published identity mean anything.
- **Freeze the design until a new generation is minted.** Rejected because the
  blockers are corrections to unpublished Forms; delaying them to batch a
  schema mint would publish the defects instead of the fixes.
