# 0042 — The pre-Beta epochs are withdrawn

- Status: Accepted
- Date: 2026-08-22

## Context

The repository carried three generations at once: the Legacy
`forms.takoform.com/v1alpha1` epoch (34 published Form Package identities, the
admission evidence trees, the provider-v1 lane), the
`forms.takoform.com/v1alpha2` epoch (nine retained candidates, the provider-v2
lane, the v1alpha3 identities its provider carried), and the current Beta
generation. The first two were retained as frozen history under the rule that
published identities are never overwritten.

Retention is not free. The retained generations were most of the repository's
surface — candidate trees, release inventories, admission ledgers, lifecycle
registries, conformance corpora, schemas, provider code paths, publication
gates — and every one of those surfaces had to keep meaning what it meant,
which the gates could only partially check. Measured in this change set: two
retained documents had already changed meaning undetected, the word `current`
named two different generations depending on which file pinned it, and the
retained machinery (admission checkpoints, lifecycle registry, publication
truth, Registry readback tooling, the Legacy package release train) existed
only to keep asserting facts about identities no new work may use.

[Decision 0037](0037-immutability-begins-at-stable.md) settled the authority
question: **immutability begins at Stable.** A pre-Stable identity may be
withdrawn, provided the withdrawal is recorded — never silently, and never by
reusing the address for something else. Takoform is Experimental; both
pre-Beta epochs are below that line.

## Decision

**Both pre-Beta epochs are withdrawn.** The Legacy v1alpha1 epoch, the
v1alpha2 epoch, the v1alpha3 identities that epoch's provider served, and
every tree that existed only to retain them: candidate sets, release
inventories and plans, the lifecycle registry, admission evidence and its
identity ledger, the withdrawn lanes' schemas, operation tables, normative
documents, conformance corpora, provider resources and data sources, and the
publication-truth machinery derived from the admission evidence.

**What a withdrawal is.** Every served identity moves to the `retired` list of
the ledger that owns it — document addresses in
[`../../release/published-document-lanes.json`](../../release/published-document-lanes.json),
schema `$id`s in
[`../../release/public-schema-identities.json`](../../release/public-schema-identities.json) —
with the bytes it had and the reason. A retired identity is never reused: the
ledgers fail closed when a retired address answers again or a retired `$id`
reappears under different bytes. The bytes themselves stay in this
repository's git history and in the immutable `forms/*` release tags.

**What is deliberately kept:**

- The `formpackage` verifier keeps embedded copies of
  every epoch's schemas, so the package bytes retained in history and tags
  stay verifiable forever. A withdrawal removes service, not the ability to
  read what was served.
- The revocation delivery lane
  (`.github/workflows/form-package-revocation.yml` and its deploy phases)
  stays standing: published bytes remain revocable after their epoch is
  withdrawn.
- The published provider releases `v1.0.3`, `v2.0.0`, and `v2.1.1` are
  immutable Registry history and are not touched. `v2.1.1` remains the
  current published provider; the nine v1alpha2 resources it carries are
  simply without successors in this repository.
- The keyless package-publisher identity
  (`.github/workflows/form-package-release.yml@refs/heads/main`) stays pinned
  in the trust profile. The workflow file itself is removed — its plan source
  was withdrawn, and [decision 0041](0041-form-packages-publish-with-the-provider-release.md)
  moved package publication onto the provider release train — but the
  identity is path-addressed, so the rebuilt publication step recreates it at
  the same path and historical signatures keep verifying.

**What this forces next.** The provider built from this repository exposes
only the 15 Family resources, so the next published release MUST be a major,
`3.0.0`; [`../../release/migrations/v2-to-v3.md`](../../release/migrations/v2-to-v3.md)
is the migration contract for users of the nine. The signed Registry readback
lane was epoch-bound tooling and is retired with it; the `v2.1.1` readback
closure is retained in `release/provider-release-identities.json`, and the
`3.0.0` release must bring a readback lane matched to its own surface before
it can be called Registry-verified.

## Enforcement

The withdrawal is recorded where machines look, not only where people read:

- both ledgers hold the retired identities and refuse their reuse, and the
  ledger checks walk committed history so an entry cannot be deleted from
  both the tree and the ledger in one edit;
- the lane and envelope tables in `scripts/check-public-surfaces.mjs` keep a
  row per withdrawn identity, require each row to state why it was withdrawn,
  and fail when a withdrawn identity still names bytes on disk;
- the current-lane residue check keeps rejecting withdrawn-lane identifiers in
  current implementation code, so the withdrawal cannot quietly reverse.

## Consequences

One generation remains: `forms.takoform.com/v1beta1`,
`edge.forms.takoform.com/v1beta1`, `packages.forms.takoform.com/v1alpha4`,
provider `2.1.1` published and `3.0.0` next. The word `current` has one
referent again. The cost is that recovering a withdrawn epoch's contract now
requires git history rather than the working tree — which is the point: the
tree states what is, history states what was.
