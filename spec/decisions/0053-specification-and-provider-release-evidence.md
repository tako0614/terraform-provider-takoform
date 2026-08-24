# 0053 — Specification and Provider evidence are separate release authorities

- Status: Accepted
- Date: 2026-08-23
- Supersedes: decisions 0044 and 0046 as Takoform Specification/Provider release authorities
- Stacks after: decision 0052 (`0052-the-specification-is-released-on-its-own-line.md`)

## Context

Takoform owns a normative specification and an official Terraform Provider. It
does not own every Host, backend, deployment, or consumer that adopts the
specification. Making Takoserver, Takosumi, a production consumer, a live
backend, or an operator signature a Takoform release prerequisite would give a
different product authority over Takoform. Making the Provider a prerequisite
would instead let a sample implementation define protocol semantics.

The old `spec/publication-blockers.json` records an earlier v1beta1 publication
attempt. Its bytes and historical open statuses remain immutable. They are not
rewritten, and the current release assertion does not require those historical
statuses to change.

The current specification is also no longer a single Edge-family checklist.
One Host lane composes multiple independently versioned Form families, shared
Interface and Binding identities, generic Host behavior, family semantics, and
cross-family resolution. A literal total count or a caller-supplied list can
silently omit a new family. Publication therefore needs one generated closed
index and one conformance-owned suite manifest, followed transitively to the
bytes they name.

The literal stable Host lane is `forms.takoform.com/v1`. The occupied
v1beta1/Provider 2.1.1 identities and the distinct pre-v1 documents and corpora
remain their own history; a publication check does not mutate or relabel those
bytes. Decisions 0044 and 0046 may still describe useful adoption exercises,
but they no longer grant another Host, product, Provider, or production
operator authority over the Specification release.

## Decision

Takoform records two independent release tracks in
[`../publication-evidence.json`](../publication-evidence.json).

### Normative Specification 1.0

Specification 1.0 has exactly three prerequisites, in this order:

1. `specification-source-snapshot` binds one full reachable Takoform commit and
   the complete normative document set by exact path-set and content digests;
2. `candidate-form-corpus` reopens the committed multi-family index, every
   package and Definition, the aggregate Interface and Binding sets, the suite
   manifest, every corpus, every runner input, and every transitively referenced
   artifact; and
3. `reference-conformance` executes the exact runner argv declared by that
   committed suite manifest and parses its class-specific report for the exact
   generic, per-family, composition, check-name, and FormRef tuple.

Those are the whole Specification authority. An independent Host, deployed
runtime, backend receipt, production consumer, Takoserver, Takosumi, Terraform
Provider, or operator signature can neither close nor block the track.

The indexed Forms keep their exact current `0.x` Definition versions and
Experimental maturity. Specification/Host API v1 does not create Form `1.0.0`
identities. Each future Stable Form requires an explicit per-Form decision;
that maturity evidence is independent from this release track.

#### Closed current-family index

The generator owns the non-authoritative index at
`forms/candidates/current-family-index.json`. Its exact format is
`takoform.current-family-index@v1`:

```json
{
  "format": "takoform.current-family-index@v1",
  "families": [
    {
      "group": "edge.forms.takoform.com",
      "candidateSet": "forms/candidates/edge.forms.takoform.com/candidate-set.json",
      "sha256": "...",
      "formCount": 16
    }
  ],
  "interfaceCandidateSet": { "path": "...", "sha256": "..." },
  "bindingCandidateSet": { "path": "...", "sha256": "..." }
}
```

`families` is closed, unique, and ordered by group and candidate-set path. The
index carries no maturity, release, Provider, Terraform `resourceType`, or
global Form-count field. `formCount` is only a per-family generator assertion:
the publication verifier reopens that candidate set and derives the count.
It follows every package index, recomputes raw file hashes, canonical package
digests, Definition schema digests, exact versionless FormRefs, Host-lane
requirements, and lifecycle capabilities. It likewise reopens the aggregate
Interface and Binding sets and their Definitions. No directory scan or caller
list substitutes for the fixed index.

The current Edge family is exactly 16 Forms and its semantic corpus is exactly
16/16 with no missing FormRefs. There is no current `ObjectBucket` Form,
`edge.objects` Interface, `module-worker.object-bucket` Binding, or
same-document bucket Resource link. The exact Provider 2.1.1/versioned
v1beta1 ObjectBucket bytes remain retained publication history.

The current generator closure contains eight families and 31 exact Forms. The
verifier derives both values by reopening the fixed index and candidate sets;
neither number is a hand-written release prerequisite or a permanent central
enum.

Worker code may still declare a sealed Host-resolved standard-service slot.
The stable slot is `standards.takoform.com/v1` and carries only
`{name, service:{apiVersion,protocol}, required?}`. Its protocol is an opaque
lowercase reverse-DNS identity with an owner namespace plus protocol segment;
Takoform does not enumerate protocols. The Edge corpus includes the exact
opaque `com.amazonaws.s3` slot as an S3-compatible adoption case, but equality
with that string is not protocol-compliance certification. Endpoints,
credentials, and a portable Resource identity never enter the Form document;
the Host resolves and projects the sealed runtime capability. The old
`standards.takoform.com/v1alpha1` closed enum remains immutable history and is
not extended. The protocol schema admits only the exact type, maximum length,
and reverse-DNS pattern plus non-constraining annotations: another enum,
`allOf`, `not`, minimum length, or other narrowing keyword fails publication.
The Edge corpus carries structured support-profile readbacks, not a string
label: the exact v1 `com.amazonaws.s3` ServiceRef is satisfiable and another
valid opaque ServiceRef is refused.

#### Conformance-owned suite

The conformance generator owns
`conformance/takoform-v1/manifest.json`, with exact format
`takoform.conformance-suite@v1`:

```json
{
  "format": "takoform.conformance-suite@v1",
  "hostApiLane": "forms.takoform.com/v1",
  "generic": { "path": "...", "sha256": "...", "requiredChecks": ["..."] },
  "families": [
    {
      "group": "edge.forms.takoform.com",
      "path": "...",
      "sha256": "...",
      "requiredChecks": ["..."],
      "dependencyGroups": []
    }
  ],
  "composition": { "path": "...", "sha256": "...", "requiredChecks": ["..."] },
  "runner": { "command": ["..."], "reportFormat": "..." }
}
```

The suite family groups exactly equal the index groups and appear in the same
order. Dependencies are sorted, unique, and name other indexed groups. The generic corpus owns
Host-wide behavior, each family corpus owns its complete Form lifecycle and
semantic probes, and the composition corpus owns all-family resolution smoke.
Every exact check name is recorded, not only a hand count. Every referenced
artifact is committed and hashed transitively. Generic, family, and composition
corpora use distinct formats and carry one ordered concrete input/expected
scenario for every required check. Composition scenarios repeat the complete
indexed family group set, so check labels alone cannot stand in for all-family
coverage.

The publication layer does not invent a self-test CLI and does not accept a
caller-supplied report. The manifest owns shell-free argv whose executable is
`go` or `bun` and whose source is a committed repository-relative `cmd/` or
`scripts/` path. The verifier checks the implementation and inputs against the
source commit across the whole repository (excluding only the evidence record
and the later self-referential Specification release ledger), rejects staged or
untracked substitutions anywhere in that closure, runs that exact argv, and
requires the declared report format to enumerate every generic, family, and
composition check as passed plus the exact runner FormRefs. A generic
`{"result":"pass"}`, a relabeled class, or a handwritten count cannot close a
prerequisite.

### Non-normative official Provider 3.0

Provider 3.0 is an independent official implementation milestone with three
different prerequisites:

1. `provider-v3-exact-conformance` binds Provider schema/client/codec source
   and executable lifecycle matrices to every indexed exact FormRef;
2. `provider-v3-identity-lock` binds the Provider descriptor and append-only
   FormRef-to-Terraform-resource-type ledger; and
3. `provider-v3-compatibility-migration-lock` binds retained Provider releases,
   per-FormRef codecs, state continuity, and migration evidence.

The Provider is non-normative. Its Terraform mapping remains Provider-owned
and never enters a Form Definition, candidate identity, package digest, Host
API rule, or Specification prerequisite. The two versions are not lockstep;
either track may remain open while the other closes.

### Repository authority and evidence classes

All subjects name the canonical Takoform repository, normalized to
`https://github.com/tako0614/terraform-provider-takoform.git`, and a full commit
reachable from `origin/main`, a `v*` tag, or a `specification/*` tag. A missing
origin, different origin URL, present-but-unreachable object, caller repository
map, working-tree substitution, or unhashed transitive file fails closed.

This is an identity and misrouting check under an explicit
`trusted-fetched-checkout-precondition`, not cryptographic remote provenance.
Local Git configuration and remote-tracking refs are mutable; therefore the
operator or CI substrate must authenticate and freshly fetch the canonical
checkout before running a release assertion. A standalone offline verifier
cannot prove that GitHub advertised a local ref without a remote exchange or a
trusted signature. Neither is smuggled into this portable gate, and the machine
policy states the precondition instead of claiming that local object presence
is authorization.

The required result formats are distinct:

| Prerequisite | Result format |
| --- | --- |
| Specification source | `takoform.specification-source-snapshot@v2` |
| candidate/corpus closure | `takoform.multi-family-candidate-corpus@v1` |
| reference suite | `takoform.reference-conformance-suite-evidence@v1` |
| Provider conformance | `takoform.provider-v3-exact-conformance@v2` |
| Provider identity | `takoform.provider-v3-identity-lock@v2` |
| Provider compatibility | `takoform.provider-v3-compatibility-migration-lock@v2` |

No detached signer is configured or required. The required Specification facts
are committed Takoform-owned bytes and a deterministic committed reference
runner, not live operator assertions. A deployed Host may define signed
operational evidence in its own repository, outside this release record.

## Enforcement

- `bun run check:publication-evidence` validates policy and reports both tracks
  without requiring either to be complete.
- `bun run check:specification-v1-release` asserts only the three normative
  Specification prerequisites.
- `bun run check:provider-v3-release` asserts only the independent Provider
  milestone.
- `bun run check:takoform-milestones` asserts both tracks as a project
  checkpoint.
- No `stable-mint` alias exists: the Specification assertion must not imply
  that current Forms or packages are promoted.

The current evidence record intentionally stores `null` for both the fixed
family-index pointer and suite pointer. Their absence keeps the Specification
track open rather than falling back to the old single-Edge corpus. One pointer
without the other, a noncanonical path, or an open new prerequisite fails
closed. A synthetic committed multi-family fixture proves that all three exact
Specification artifacts can close without changing the frozen v1beta1 ledger
or supplying any Provider, external-Host, production, or signer artifact.

No command in this decision commits, tags, publishes, deploys, enrolls a key,
or changes production state.

## Consequences

Specification 1.0 can release on Takoform-owned normative and reference bytes,
while every current family must be present and composition-tested. Adding a
family changes the generator index and therefore demands its own exact semantic
corpus and all-family result; no global number needs manual maintenance.
Provider and external-adoption progress stay visible without becoming hidden
Specification authority. Form maturity remains a separate per-Form decision,
so releasing Specification/Host API v1 does not silently convert the 31 current
`0.x` FormRefs into Stable identities.
