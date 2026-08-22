# 0044 — Graduation evidence is implementation independence, honestly attributed

- Status: Accepted
- Date: 2026-08-23

## Context

Two normative documents made a person a prerequisite. `versioning.md` required
"two independently operated hosts" for a lane graduation, and
`project-lifecycle.md` required "two distinct host subjects with two
independently named maintainers" for a Stable Form. In an ecosystem whose
every implementation is currently written and operated by one maintainer,
those words make v1 unreachable regardless of how much evidence the contracts
earn — not because the contract is unproven, but because of an organizational
fact the spec cannot change.

The criterion was never really about payroll. What two maintainers were a
proxy for is the failure mode that kills portability claims: a contract that
is only implementable by copying the reference implementation's private model.
Independence of *implementations* — distinct codebases, neither derived from
the other's host internals, both passing the same corpus, at least one
carrying real traffic — measures that directly. Independence of *people* was
the proxy, and a proxy that cannot be satisfied stops measuring anything.

The maintainer directed this realization explicitly (2026-08-23).

## Decision

**The personnel criterion is replaced by an implementation criterion, and the
personnel fact is disclosed instead of required.**

A graduation's host-evidence prerequisite is now: **two independent host
implementations from distinct codebases — neither derived from the other's
host internals — exercising the same lifecycle semantics through the same
conformance corpus, at least one of them operating in production for real
consumers.** When those implementations share a maintainer, the graduation ADR
MUST say so by name; the shared authorship is a recorded limitation of the
evidence, never a hidden one.

Everything else stands unchanged: the compatibility window, end-to-end
materialization of every optional surface the lane declares, cross-publisher
package installation (the package publisher must not be a consuming host —
a role separation this ecosystem can satisfy), the deprecation and revocation
exercises, and the rule that a graduation is its own ADR minting new exact
identities ([decision 0039](0039-a-lane-is-minted-for-one-of-two-reasons.md),
[decision 0037](0037-immutability-begins-at-stable.md)).

`versioning.md` and `project-lifecycle.md` are updated with this record, as
the ADR rule requires.

## What this does not do

It does not lower the bar to a declaration. A second implementation still has
to exist, be independent in code, pass the corpus, and one of the two still
has to carry production traffic. It does not remove the third-party dimension
from the ecosystem's ambitions — an external implementation remains the
strongest possible evidence and a graduation ADR should cite one when it
exists. It changes only what is *required*: the spec stops requiring an
organizational fact it cannot cause.

## Consequences

v1 becomes reachable by doing work this ecosystem can actually do: a second
host implementation (takosumi is the designated candidate) built against the
spec rather than extracted from the reference host, driven by
`portable-host-conformance` and the runtime corpus, with the reference host as
the other subject. The graduation ADR that eventually cites this rule will
carry the maintainer disclosure, and a reader evaluating the Stable claim gets
the true shape of its evidence.
