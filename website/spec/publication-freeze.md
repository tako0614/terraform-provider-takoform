# Historical Specification, Provider, and publication evidence policy

Takoform has independent release and publication axes. Evidence from one axis
MUST NOT be treated as authority for another.

## Historical Specification 1.1 evidence

The Specification 1.1 track freezes the portable specification source and is
retained as historical evidence. The current Host API `forms.takoform.com/v1`
is a stable protocol identity independent of this receipt. One complete, exact
committed snapshot of the normative `spec/` tree satisfied the historical
receipt's readiness; the append-only numbered ledger records that publication
state. Candidate Forms, packages, conformance corpora, and reference
implementations remain quality and adoption evidence; they are not current
Host API or Provider authority.

Identity `1.0` was never published, is withdrawn, and is never reused. The
separately generated five-class [compatibility report](../release/specification-compatibility.json)
records raw source digests, owning ledgers, migration dispositions, and the
historical Host API source pin. It is compatibility evidence only, not
publication evidence, a release asset, or a prerequisite. The historical
Specification receipt has no Host API, Form publication, or Provider effect and
does not mint a `/v1.1` or v2 lane/schema/tag/receipt.

The canonical machine policy is
[`publication-evidence.json`](publication-evidence.json), and the numbered
release ledger is
[`../release/specification-releases.json`](../release/specification-releases.json).
Decision [0057](decisions/0057-specification-1-1-compatibility-and-independent-identities.md)
amends decisions [0052](decisions/0052-the-specification-is-released-on-its-own-line.md)
and
[0053](decisions/0053-specification-and-provider-release-evidence.md), and
[0055](decisions/0055-specification-release-needs-only-normative-source.md)
define the boundary.

## C1, C2, C3, and C4

The W09 workflow keeps the freeze, evidence, receipt, and derived-public
boundaries separate:

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
- **C4 — deterministic derived-public refresh:** the first ancestry commit
  after C3 is its direct single-parent child and changes only the explicit
  generated compatibility, site-status, README, and website outputs. C4 is not
  publication authority. It MUST NOT change `spec/**`, publication evidence,
  any ledger or ledger projection, release authority/tooling, Provider/Form/Host
  source, or an unrelated file. Later descendants are permitted after this
  fixed point.

  Concretely, that bounded write set is `README.md`; the canonical/static/public
  compatibility report; the static/public site status; every generated
  VitePress `website/public/**/*.html` page (including `404.html`); and only the
  content-hashed `website/public/assets/**` closure changed by the build.
  The immutable runtime-ABI fixture
  `website/public/conformance/runtime-abi-v1/bundles/unsupported-media-type/page.html`,
  `website/docs/reference.md`, and `website/public/hashmap.json` do not change in
  this transition and are not allowed C4 paths.

The compatibility report is generated and checked independently and is never a
C2/C3 asset, prerequisite, or release identity. C3 and C4 remain separate,
linear commits and are never squashed.

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
- `bun run check:specification-releases` validates the numbered ledger,
  protects committed release entries append-only, and enforces the exact
  C3-to-C4 history fixed point after a 1.1 receipt exists.
- `bun run check:specification-1-1-release` asserts only the normative source
  snapshot.
- `bun run check:provider-v3-release` asserts only the non-normative Provider
  milestone.
- `bun run check:takoform-milestones` asserts both as an optional project
  checkpoint.

When Specification source evidence is `null`, readiness remains open; when it
records one clean reachable commit, its exact normative bytes must match.
Candidate and reference evidence may stay `null` without blocking it. A dirty,
staged, unreachable, or digest-mismatched normative snapshot fails.

There is no `stable-mint` alias. The readiness assertion does not silently
promote Forms or packages.
