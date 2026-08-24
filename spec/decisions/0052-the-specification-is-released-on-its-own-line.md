# 0052 — The Specification is released on its own line

- Status: Accepted
- Date: 2026-08-23

## Context

Takoform has several independent compatibility and maturity axes. The Host API
lane, each exact FormRef, the Form Package envelope, Interface and Binding
identities, and the Terraform Provider all version for different reasons. None
of those numbers identifies the complete normative document set a reader
implemented.

Earlier policy also made implementation adoption a precondition for the
Specification itself: two Hosts, production traffic, a compatibility window,
third-party publication, and Provider release were treated as inputs to the v1
decision. Those facts are useful adoption evidence, but their owners are not
Takoform specification authorities. Takoserver, Takosumi, another backend, a
production operator, or the official Provider must not be able to block or
define the normative protocol.

The source now has a literal Host API v1 suite. Its closed generator-owned
index names eight versionless Form families and 31 exact current FormRefs. The
Edge family contains 16 Forms. `ObjectBucket`, `edge.objects`, and
`module-worker.object-bucket` are not current identities. The 31 Forms keep
their own current `0.x` Definition versions and Experimental maturity.

Provider 2.1.1, Host v1beta1, its versioned 15-Form Edge family, and their
published bytes remain immutable history. The official Provider 3 work is a
separate, non-normative sample implementation.

## Decision

**Takoform Specification 1.0 is a numbered normative document release with the
literal Host API identity `forms.takoform.com/v1`. It releases only after three
Takoform-owned prerequisites close.**

The three prerequisites, in order, are:

1. `specification-source-snapshot`: one reachable full commit and the exact
   path set and digest of every normative Specification source byte;
2. `candidate-form-corpus`: the exact committed multi-family index reopened
   through every package, Definition, Interface, Binding, suite corpus, runner
   input, and transitively referenced artifact; and
3. `reference-conformance`: execution of the exact runner argv declared by the
   committed suite manifest, producing its exact class-specific report.

No external Host, backend, runtime, deployment, production consumer,
Takoserver, Takosumi, signer, or operator fact may replace, add to, or close one
of those rows. They remain independently recordable adoption evidence.

The release has no implicit effect on another axis:

- the 31 current FormRefs are not relabelled or promoted to Form `1.0.0`;
- a future Stable Form begins at `1.0.0` only through an explicit per-Form
  decision, with its own compatibility and maturity evidence;
- no Form Package is published by the Specification release;
- no Interface or Binding identity changes merely because v1 releases;
- Provider 3 may implement the current exact `0.x` FormRefs, but its completion
  is neither necessary nor sufficient for Specification 1.0; and
- the retained Provider 2.1.1/v1beta1 identities are never rewritten.

## Release record

[`../../release/specification-releases.json`](../../release/specification-releases.json)
is the append-only numbered-release ledger. Its `candidate` object states the
intended 1.0 identity, canonical index and suite paths, and no-promotion
effects. An entry moves into `releases` only after all three exact publication
evidence objects are committed and closed. Once committed, a numbered release
entry cannot be edited, removed, or reordered.

The release record binds:

- the full source commit;
- `forms/candidates/current-family-index.json` and its exact digest;
- `conformance/takoform-v1/manifest.json` and its exact digest;
- canonical digests of the source-snapshot, candidate/corpus, and reference
  conformance evidence objects; and
- the explicit statements that Form maturity is unchanged and Provider 3 is
  independent.

The release ledger is excluded from the source snapshot it records, avoiding a
self-referential commit digest. Publication evidence is committed separately
and must equal the record read from `HEAD` before a prerequisite can close.

## Current state

The repository currently records a **Specification 1.0 candidate**, not a
completed release. The evidence tuple remains absent in
[`../publication-evidence.json`](../publication-evidence.json), and the three
Specification evidence objects are `null`. This is intentional: a dirty or
uncommitted worktree cannot be the exact source snapshot and must remain red.

The v1 source generator and suite may be locally green without making this
release claim true. Exact baseline commit and digests are recorded only after
the integrated source bytes are committed. Provider 3 can remain open or close
independently without changing that result.

## Enforcement

- `bun run check:publication-evidence` validates both independent track shapes
  and reports open rows without requiring a release.
- `bun run check:specification-releases` validates the candidate and protects
  every committed numbered release append-only.
- `bun run check:specification-v1-release` asserts only the three Specification
  prerequisites and fails while they are open.
- `bun run check:provider-v3-release` asserts only the non-normative Provider
  milestone.
- `bun run check:takoform-milestones` is an optional project checkpoint that
  asserts both tracks; it is not the Specification release gate.

There is no `stable-mint` alias. A command that asserts Specification 1.0 must
not imply that every Form or package becomes Stable.

None of these commands commits, tags, publishes, deploys, signs, enrolls a key,
or changes production state.

## Consequences

Readers can cite one exact Takoform-owned Specification 1.0 document set and
literal Host API v1 contract without delegating normative authority to a
product adoption schedule. The price is explicit separation: protocol release,
Form maturity, Provider compatibility, package publication, Host support, and
production availability must each state their own evidence and must never be
inferred from another axis.
