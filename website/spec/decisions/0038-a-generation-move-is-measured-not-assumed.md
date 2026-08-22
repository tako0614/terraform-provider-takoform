# 0038 — A generation move is measured, and what it costs is recorded

- Status: Accepted
- Date: 2026-08-22

## Context

The move that produced the current generation was measured after the fact,
because a reader could not tell from the identities what it had changed.

**The Host API lane moved and the protocol did not.**
`spec/schemas/host-api-wire-v1alpha3.schema.json` and `-v1beta1.schema.json`
are 590 lines each and, with version words normalised, structurally identical;
so are the two discovery schemas; and `spec/host-api/v1alpha3.md` and
`v1beta1.md` differ by exactly one line that is not a version word — a link to
[decision 0035](0035-beta-contracts-ship-in-stable-provider-v2-1.md). Comparing
all four lanes pairwise, the only structurally identical pair in six is
v1alpha3 against v1beta1.

**The Form family moved and most of its contracts did not.** The same fifteen
kinds, all still `0.1.0`, and twelve of the fifteen definitions structurally
unchanged. Three changed: `WorkerVersion`, `WorkerDeployment`,
`SQLiteMigrationApplication`.

**The axis that should have carried those three did not move at all.**
`definitionVersion` stayed `0.1.0` for every Form, while
[`../versioning.md`](../versioning.md) already says a breaking Experimental
change increments the minor version.

So one event moved three identity axes, and the axis built for it stayed still.
That is why the numbers read as coupled while the documents describe them as
independent, and why "which one is current" stopped being answerable by
inspection.

## The part this record does not decide

Whether the lane should have moved is genuinely open, because two normative
documents in this repository answer it differently.

[`../versioning.md`](../versioning.md) says the lane does not move for Form
reasons:

> a Host wire version never implies Form maturity.

> The API group MUST NOT graduate based on a Form count, package publication,
> provider major, historical admission, or one host's conformance report.

and names only one trigger — "Breaking protocol changes require a new Host API
group identity" — as a sufficiency, never as the only permitted reason.

[Decision 0035](0035-beta-contracts-ship-in-stable-provider-v2-1.md) and
[`../publication-freeze.md`](../publication-freeze.md) say a maturity
graduation arrives as a new identity:

> Stable `v1` and Form `1.0.0` are new exact identities

> Promotion adds identities and create defaults

Under the first reading the v1alpha3 → v1beta1 move is hard to justify; under
the second it is the design working. **Both are current normative text, and
this record does not pick one.** Deciding requires answering whether the
maturity word is part of what a lane identity MEANS — which runs directly into
[decision 0037](0037-immutability-begins-at-stable.md)'s rule that a served
identity means what it meant when it was published. If it is part of the
meaning, a graduation must mint; if it is not, minting on maturity is the empty
migration this record measured.

That question binds only when a graduation is real, and graduation is gated on
evidence this project cannot produce alone —
[`../project-lifecycle.md`](../project-lifecycle.md) requires two
independently maintained host implementations. `versioning.md` already reserves
the decision: "Any graduation is a separate ADR and public migration plan."
This is a **conformance gap** in the sense `AGENTS.md` names — a disagreement
between two authorities, recorded rather than silently resolved by whichever
one a reader happens to open first.

## Decision

**A generation move records what it changed, per Form.**
[`../../release/form-contract-continuity.json`](../../release/form-contract-continuity.json)
carries, for each move, which contracts changed and which were only
re-identified. The classification is DERIVED — definitions compared before and
after with version words normalised — so it cannot drift from the definitions
and cannot be asserted by hand. It answers the question a client holding a
recorded FormRef actually has: is this the same contract under a new name, or
one I must re-read.

**A graduation must be executable before it is decided.** Two places would have
failed the moment the family advanced past Beta, on a path this repository has
already committed to in `internal/currentformregistry/registry_v3_test.go` and
`internal/provider/v3_continuity_test.go`. The site-status derivation had a
bare-`/vN` branch for the Host API lane and none for the family, so a stable
family threw; and `formpackage` selected the family schema by matching
`/v1beta1$`, letting any other namespaced group fall through to the retained
v1alpha3 closure without an error. Both are fixed: the two derivations now read
the same way, the family channel is checked against the recognised set rather
than against today's value, and the retained families are named one at a time
so a new one gets the current schema by default.

**A maturity word is not checked by anything, and this record says so.** The
lane and the family each carry a channel word that no gate compares against the
evidence — not against `spec/publication-blockers.json`, not against the
lifecycle records. Where a channel is derived from a name, it reports what the
name says, never whether the name is still true. That is the same class as
[decision 0036](0036-artifact-names-state-the-lane-they-describe.md)'s relative
names, and it is unresolved here deliberately: making the word verifiable and
deciding when it may move are one question, and it is the question deferred
above.

## Consequences

The cost of the last generation move is now legible: three contracts changed,
twelve identities were spent on the rename. A future move can be judged against
that record rather than described.

Nothing about the current identities changes. `forms.takoform.com/v1beta1` and
`edge.forms.takoform.com/v1beta1` are embedded in Registry-published provider
`v2.1.1` and are not movable regardless of how the deferred question is
answered.

What a later graduation ADR inherits from this one is a measurement and two
working code paths, rather than an argument and a latent crash.
