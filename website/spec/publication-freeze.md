# Specification, Provider, and publication evidence policy

Takoform has independent release and publication axes. Evidence from one axis
MUST NOT be treated as authority for another.

## Specification 1.0

The normative Specification 1.0 track defines the literal stable Host API lane
`forms.takoform.com/v1`. It closes when one complete snapshot of the normative
`spec/` tree is committed and exact. Candidate Forms, packages, conformance
corpora, and reference implementations remain quality and adoption evidence;
they are not Specification release prerequisites.

The canonical machine policy is
[`publication-evidence.json`](publication-evidence.json), and the numbered
release candidate/ledger is
[`../release/specification-releases.json`](../release/specification-releases.json).
Decisions [0052](decisions/0052-the-specification-is-released-on-its-own-line.md)
and
[0053](decisions/0053-specification-and-provider-release-evidence.md), and
[0055](decisions/0055-specification-release-needs-only-normative-source.md)
define the boundary.

The current generator index contains eight versionless families and 31 exact
Experimental `0.x` FormRefs. Edge contains 16 and has no current
`ObjectBucket`, `edge.objects`, or `module-worker.object-bucket`. The stable
standard-service contract is `standards.takoform.com/v1`; protocol identifiers
are opaque reverse-DNS strings, not a Takoform-owned enum.

Releasing Specification/Host API v1 does not:

- relabel any current Form as `1.0.0` or Stable;
- publish a Form Package, Interface, or Binding;
- release Provider 3;
- prove any Host, backend, runtime, or product supports the contract; or
- authorize a commit, tag, publication, deployment, signing action, or
  production change.

A future Stable Form begins at `1.0.0` only through an explicit per-Form
decision. Form maturity remains independent from the Host API and numbered
Specification.

## Official Provider

The Terraform Provider is an independent implementation stream. Provider
2.1.1 and its exact Host v1beta1/versioned Edge 15-Form identities remain
immutable Registry history.

Provider 3 is an official but non-normative sample. Its evidence track may bind
the exact current Form projection, Provider identity ledger, retained codecs,
state continuity, and migration behavior. It can remain open while
Specification 1.0 closes, and a green Provider milestone cannot close the
Specification track.

## Adoption evidence

Independent Hosts, Takoserver, Takosumi, other backends, runtime ABI reports,
production consumers, compatibility windows, package publishers, signers, and
operators may record adoption evidence in their owning repositories. Those
facts are useful for support, implementation confidence, and product
readiness. They are not Takoform release authorities and can neither add a
Specification prerequisite nor substitute for one.

Decisions 0044 and 0046 are retained as adoption-program history. Decision
0053 supersedes them as Specification and Provider release gates.

## Retained publication history

[`publication-blockers.json`](publication-blockers.json) records an earlier
v1beta1 Form Package/public-service attempt. Its bytes and open statuses remain
immutable historical truth. The new Specification assertion does not rewrite
or require closure of those rows.

Likewise:

- published schema identities remain append-only in
  [`../release/public-schema-identities.json`](../release/public-schema-identities.json);
- occupied document lanes remain recorded in
  [`../release/published-document-lanes.json`](../release/published-document-lanes.json);
- Provider release and FormRef identities remain append-only in the release
  ledgers; and
- retired or retained bytes are never republished under a different meaning.

The historical `assert:publishable` command remains the assertion for that old
Form Package/public-service obligation set. It is not a Specification 1.0 or
Provider 3 gate.

## Fail-closed checks

- `bun run check:publication-evidence` validates both independent track shapes
  and reports their open/ready state.
- `bun run check:specification-releases` validates the numbered candidate and
  protects committed release entries append-only.
- `bun run check:specification-v1-release` asserts only the normative source
  snapshot.
- `bun run check:provider-v3-release` asserts only the non-normative Provider
  milestone.
- `bun run check:takoform-milestones` asserts both as an optional project
  checkpoint.

The Specification source evidence remains `null` until one clean, reachable
commit contains the normative bytes. Candidate and reference evidence may stay
`null` without blocking it. A dirty, staged, unreachable, or digest-mismatched
normative snapshot fails.

There is no `stable-mint` alias. The readiness assertion does not silently
promote Forms or packages.
