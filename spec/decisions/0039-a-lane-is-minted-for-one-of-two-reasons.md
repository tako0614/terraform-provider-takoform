# 0039 — A Host API lane is minted for one of exactly two reasons

- Status: Accepted
- Date: 2026-08-22

## Context

[Decision 0038](0038-a-generation-move-is-measured-not-assumed.md) recorded a
question and declined to answer it: two normative documents in this repository
appeared to disagree about whether a maturity graduation may mint a new Host API
lane.

[`../versioning.md`](../versioning.md) says the lane does not move for Form
reasons — *"a Host wire version never implies Form maturity"*, and *"The API
group MUST NOT graduate based on a Form count, package publication, provider
major, historical admission, or one host's conformance report."*

[Decision 0035](0035-beta-contracts-ship-in-stable-provider-v2-1.md) and
[`../publication-freeze.md`](../publication-freeze.md) say a graduation arrives
as a new identity — *"Stable `v1` and Form `1.0.0` are new exact identities"*,
*"Promotion adds identities and create defaults."*

They do not disagree. They answer **different questions**, and reading them as
one produced the deadlock:

- `versioning.md` answers **what may cause a lane to move**.
- `0035` answers **what a move looks like when one is warranted**.

A rule that satisfies both is available, and this record adopts it. What forced
the earlier deferral was reading "the lane must not graduate on Form grounds"
as "the lane must not graduate", which is not what it says.

## Decision

**A lane is minted for the wire contract changing, or for the lane itself
advancing a maturity channel on the evidence `versioning.md` names. Nothing
else.** Not because a Form Family moved, not because Forms changed, not because
a provider was released.

**A graduation mints a new exact identity; it never relabels an occupied one.**
[Decision 0037](0037-immutability-begins-at-stable.md) settles this half: a
served identity means what it meant when it was published. A lane published at
a Beta channel asserted that channel — the site derives and publishes it from
that word — so graduating it in place would make an address a reader already
holds quietly mean something stronger than it did. That is the failure this
repository spent a whole change set removing, and it is not worth re-introducing
to save one migration.

The migration a graduation costs is bounded and final. `alpha → beta → stable`
has no further step, so the move to a stable lane is the last maturity-driven
lane move that can ever happen. A permanent false name is the worse trade.

**Both halves are enforced rather than described.** Every lane is recorded with
the reason it was minted. A `protocol` lane's wire contract must differ
structurally from every other `protocol` lane's, with version words normalised
so a rename cannot present itself as a contract; a graduation lane is exempt
from that comparison precisely because carrying an existing contract under a new
channel is what it is for. A `graduation` lane must state which prerequisite it
satisfied, because no comparison of bytes can establish it.

## What this says about the lane that exists

`forms.takoform.com/v1beta1` is recorded as a graduation whose lane-specific
evidence was never stated. It was minted with the Edge family channel move, and
its wire contract is structurally identical to `v1alpha3`'s — 590 lines each,
indistinguishable once version words are normalised, with the normative document
differing by a single line that is a link. Under the rule above it would have
had to say what it graduated on, and it did not.

It is frozen into Registry-published provider `v2.1.1` and cannot be withdrawn.
The record therefore stands as history rather than as a precedent, and the gate
reads it as the exception it is. What the rule prevents is the next one.

## Consequences

The deadlock 0038 recorded is resolved without either document being wrong, and
without changing any identity that exists.

A future stable lane is still permitted, still requires the five prerequisites
in `versioning.md`, and now also requires saying which of them it met. A future
protocol lane is permitted and must prove it carries a different contract.

What is no longer permitted is the thing that happened: a lane moving because
everything else did, with no reason recorded for the lane in particular. The
family and the Forms may move on their own axes without dragging the wire
identity with them, which is what `versioning.md` said all along.
