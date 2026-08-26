# Specification, Provider, and publication evidence policy

Takoform has independent release and publication axes. Evidence from one axis
MUST NOT be treated as authority for another.

## Specification 1.1

The normative Specification 1.1 track freezes the portable specification
source. The repository also carries the unchanged literal Host API v1 source
candidate at `forms.takoform.com/v1`, but that is a separate unpublished
protocol identity. Publishing Specification 1.1 does not publish or promote
Host API v1. The Specification closes when one complete snapshot of the
normative `spec/` tree is committed and exact. Candidate Forms, packages,
conformance corpora, and reference implementations remain quality and adoption
evidence; they are not Specification release prerequisites.

Identity `1.0` was never published, is withdrawn, and is never reused. The
separately generated five-class [compatibility report](../release/specification-compatibility.json)
records raw source digests, owning ledgers, migration dispositions, and the
byte pin for the Host API v1 candidate. It is compatibility evidence only, not
publication evidence, a release asset, or a prerequisite. Specification 1.1
has no Host API, Form publication, or Provider effect and does not mint a
`/v1.1` or v2 lane/schema/tag/receipt.

The canonical machine policy is
[`publication-evidence.json`](publication-evidence.json), and the numbered
release candidate/ledger is
[`../release/specification-releases.json`](../release/specification-releases.json).
Decision [0057](decisions/0057-specification-1-1-compatibility-and-independent-identities.md)
amends decisions [0052](decisions/0052-the-specification-is-released-on-its-own-line.md)
and
[0053](decisions/0053-specification-and-provider-release-evidence.md), and
[0055](decisions/0055-specification-release-needs-only-normative-source.md)
define the boundary.

## C1, C2, and C3

The W09 workflow keeps the freeze, evidence, and receipt boundaries separate:

- **C1 — normative freeze and executable tooling:** the normative `spec/` tree
  and its validation tooling are frozen, while every field in the publication
  evidence record remains `null`.
- **C2 — evidence-only source snapshot:** one exact committed Specification
  source snapshot is recorded, with only the allowed evidence projections;
  candidate corpus, reference conformance, Provider, Host, and compatibility
  report data remain outside the release evidence record.
- **C3 — authoritative publication receipt:** one immutable Specification 1.1
  receipt is appended to the numbered ledger and its static projections. The
  receipt carries only the source-snapshot prerequisite and publication
  readback.

The compatibility report is generated and checked independently and is never a
C2/C3 asset, prerequisite, or release identity.

The current generator index contains eight versionless families and 31 exact
Experimental `0.x` FormRefs. Edge contains 16 and has no current
`ObjectBucket`, `edge.objects`, or `module-worker.object-bucket`. The stable
standard-service contract is `standards.takoform.com/v1`; protocol identifiers
are opaque reverse-DNS strings, not a Takoform-owned enum.

Publishing Specification 1.1 does not publish or promote the separate Host API
v1 candidate, and it does not:

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

Provider 3.0.0 is a Registry-published but non-normative reference
implementation. Its evidence track binds
the exact current Form projection, Provider identity ledger, retained codecs,
state continuity, and migration behavior. Its publication cannot close the
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
Form Package/public-service obligation set. It is not a Specification 1.1 or
Provider 3 gate.

## Fail-closed checks

- `bun run check:publication-evidence` validates both independent track shapes
  and reports their open/ready state.
- `bun run check:specification-releases` validates the numbered candidate and
  protects committed release entries append-only.
- `bun run check:specification-1-1-release` asserts only the normative source
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
