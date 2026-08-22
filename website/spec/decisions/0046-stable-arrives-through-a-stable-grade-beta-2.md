# 0046 — Stable arrives through a stable-grade Beta 2

- Status: Accepted
- Date: 2026-08-23

## Context

The maintainer directed the convergence (2026-08-23): perfect the
specification, take it to v1, and reach Stable through a Beta 2 built to
stable discipline — graduating when satisfied, not when a date arrives.

The graduation prerequisites
([`../versioning.md`](../versioning.md), realized by
[decision 0044](0044-graduation-evidence-is-implementation-independence.md))
need a vehicle: a compatibility window is measured ON some generation, two
host subjects implement SOME exact contract, packages are published FOR some
identities. If those are measured on a generation that keeps churning, the
evidence expires as fast as it accumulates. And a v1 minted directly from a
still-moving Beta would make Stable's first day the first day the contract
was ever held immutable — the highest-stakes moment and the least-rehearsed
one.

## Decision

**A rehearsal generation — Beta 2 — is built to stable discipline, the
graduation evidence is measured on it, and v1 is minted from it as a
relabeling of a contract already run the way Stable will run.**

What Beta 2 contains:

1. **The Edge family moves to `edge.forms.takoform.com/v1beta2`** carrying
   17 Forms: the 15 at additive `0.2.0` definitions embedding the
   decision-0045 external-service declaration where it is meaningful, plus
   `DurableWorkflow` and `ActorNamespace` at `0.1.0`
   ([decision 0043](0043-forms-target-popular-vendor-locked-primitives.md)),
   with their ABI and binding contracts. The definition grammar gains the
   external-service member as a new `form-definition-v1beta2` schema
   identity; the frozen v1beta1 schemas and served v1beta1 documents are
   retained unchanged — Beta 2 adds identities, it withdraws nothing.
2. **The seven new families are born** under their own groups at their own
   `v1beta1`, into the same discipline from birth. Their numbers do not align
   with the Edge family's, which is the standing rule, not an accident.
3. **The Host API lane moves only for its own reasons.** A structural wire
   change found by the hardening review mints `forms.takoform.com/v1beta2`
   as a protocol lane (decision 0039); if none is found, the lane stays
   `v1beta1` and graduates from there. The lane is never renumbered just so
   the generation's numbers rhyme.
4. **The hardening-review findings land here.** Every defect, ambiguity, and
   asymmetry found in the v1beta1 contracts is fixed in the Beta 2
   identities, so v1 inherits a reviewed contract rather than a familiar one.

What stable discipline means, starting at each Beta 2 identity's mint:

- **The withdrawal era is over for this generation.** Decision 0037 still
  permits pre-Stable withdrawal; Beta 2 elects not to use it. A published
  Beta 2 identity is treated as immutable from publication — a defect gets a
  successor identity, never a retirement.
- **Complete before minted.** A Beta 2 identity is not published until its
  conformance coverage exists: corpus entries, provider behavior, reference
  host support, and both generated and hand-written surfaces green.
- **The evidence clock runs on Beta 2.** The compatibility window, the two
  0044 host subjects, the optional-surface materialization, the
  cross-publisher package installation, and the deprecation and revocation
  exercises are all performed against Beta 2 identities.

**Graduation is the maintainer's act, on evidence.** When the prerequisites
hold and the maintainer is satisfied, a graduation ADR — carrying the
decision-0044 maintainer disclosure and naming which prerequisite each piece
of evidence satisfied (decision 0039) — mints `forms.takoform.com/v1` (from
whichever channel the lane then occupies), family `/v1` groups, and Form
`1.0.0` identities carrying the Beta 2 contracts unchanged. Provider `3.0.0`
ships them; its release is the same event
([decision 0041](0041-form-packages-publish-with-the-provider-release.md),
[`../../release/migrations/v2-to-v3.md`](../../release/migrations/v2-to-v3.md)).

## Enforcement

The continuity ledger derives and records what the Beta 1 → Beta 2 move
re-identified versus changed, as it did for the last generation move
(decision 0038). The mint-reason tables in `scripts/check-public-surfaces.mjs`
hold any new lane or envelope to decisions 0039/0040. The no-withdrawal
election is enforced socially by this record and mechanically by the ledgers'
standing refusal to reuse a retired identity: electing not to retire means
the `retired` lists simply stop growing for this generation, which is
auditable in review.

## Consequences

Stable's first day is boring: the identities change names, the bytes and the
discipline do not. The cost is carrying two Beta generations of the Edge
family (v1beta1 served as history, v1beta2 current) until graduation — the
exact cost the ledgers were built to make safe.
