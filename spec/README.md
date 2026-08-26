# Takoform portable specification

This directory is the portable specification surface for Takoform. It contains
the **Specification 1.1 candidate** and the literal Host API v1 source
candidate. Host API v1 is a separate, unpublished protocol identity; publishing
Specification 1.1 does not publish or promote it. The numbered release remains
open until its exact committed normative source snapshot closes. Takoform defines a small desired-state
boundary between infrastructure-as-code clients and resource hosts. It is not
an industry standards body, certification authority, or guarantee of backend
portability.

Requirement keywords, conformance classes, and what a passing check does and
does not prove are defined in [`conformance.md`](conformance.md). How the API
group, Forms, packages, and the provider are versioned is in
[`versioning.md`](versioning.md). Current project positioning and the
Proposal → Experimental → Stable → Legacy lifecycle are defined in
[`project-lifecycle.md`](project-lifecycle.md). The exact boundary between
portable workload semantics and host/profile/operator concerns is
[`portability-boundary.md`](portability-boundary.md).

## Product contract map

Takoform has five public contract interfaces:

1. **Exact Form and Package data.**
   [`form-definition/`](form-definition/) defines immutable `FormRef` and the
   desired/observed/output shape. [`form-package/`](form-package/) binds one
   exact definition and its data-only fixtures into immutable package bytes
   under the current `packages.forms.takoform.com/v1alpha5` envelope.
   [`core/`](core/) defines the family-neutral compilation of admitted package
   semantic closures into one immutable exact-identity Snapshot.
2. **Desired Resource lifecycle.**
   [`host-api/`](host-api/) defines discovery, exact Form availability,
   preview/apply, read/import/observe/refresh/delete, fencing, and portable
   errors on the `forms.takoform.com/v1` wire, reached through
   `/.well-known/takoform/v1`. The host chooses implementation and
   placement.
3. **Form Families and their contract surfaces.**
   [`form-families.md`](form-families.md) defines namespaced Form Family
   groups; current families use versionless groups such as
   `edge.forms.takoform.com`.
   [`host-api/v1.md`](host-api/v1.md) defines the Host API channel
   with UID/generation/revision resource identity, long-running Operations,
   and Host Support Profiles.
4. **Interface, Binding, and artifact contracts.**
   [`interface-contract/`](interface-contract/) defines exact digest-bound
   Interface contracts, [`binding-contract/`](binding-contract/) defines typed
   Binding contracts, [`artifact-transport/`](artifact-transport/) defines
   content-addressed artifact upload, and
   [`standard-services/`](standard-services/) defines Host-resolved sealed-slot
   access by opaque `standards.takoform.com/v1` reverse-DNS protocol identity.
5. **Trust, lifecycle, version, and release identity.**
   [`trust/`](trust/) defines immutable publisher evidence and revocation;
   [`project-lifecycle.md`](project-lifecycle.md) separates Form maturity from
   Host Support and availability; [`versioning.md`](versioning.md) keeps
   provider, API, Form, and package compatibility independent.
   [`release/`](../release/README.md) binds artifacts to those exact identities
   without changing the contracts above.

[`schemas/`](schemas/), [`conformance.md`](conformance.md), and
[`decisions/`](decisions/) support those interfaces with structural minima,
executable evidence language, and decision rationale. They are not additional
product interfaces. The generated current inventory is
[`../forms/README.md`](../forms/README.md), host discovery validation is
[`schemas/host-discovery-v1.schema.json`](schemas/host-discovery-v1.schema.json),
and the local evidence map is
[`../conformance/README.md`](../conformance/README.md).

## Current status

Specification 1.1 is the first numbered release. Identity 1.0 was never
published, is withdrawn, and is never reused. The separately generated
five-class compatibility report at
[`../release/specification-compatibility.json`](../release/specification-compatibility.json)
binds raw source bytes to owning ledgers and migration dispositions and
byte-pins the literal Host API v1 candidate. It is compatibility evidence only,
not publication evidence, a release asset, or a prerequisite. Current Forms,
packages, and Host API v1 remain unpublished candidates; no `/v1.1` or v2
lane/schema/tag/receipt is created.

## C1, C2, and C3 release boundaries

The W09 workflow keeps three commits distinct:

- **C1 — normative freeze:** the normative `spec/` tree and executable
  validation tooling are frozen; every publication-evidence field remains
  `null`.
- **C2 — source evidence:** an evidence-only change records one exact committed
  Specification source snapshot and its static projection. Candidate corpus,
  reference conformance, Provider, Host, and compatibility-report data remain
  outside this evidence record.
- **C3 — publication receipt:** a later authoritative receipt appends one
  immutable Specification 1.1 entry to the numbered ledger and its projections;
  it carries only the source-snapshot prerequisite and live publication
  readback.

The compatibility report is generated and checked separately. It is never a C2
or C3 asset, prerequisite, or release identity.

The FormRef, Form Definition, package-index, revocation, and cumulative
revocation-checkpoint schemas, the RFC 8785/I-JSON library, the closed local
verifier, the positive/negative corpus, the protected keyless Sigstore release
lane, and the signed append-only checkpoint delivery lane are implemented.

The neutral Core tracer compiles zero families, synthetic external groups, and
the generated current package set through artifact bytes without importing an
official family. It remains an internal pre-extraction implementation; it is
not a public SDK or independent module release.

Current Form design work uses eight versionless namespaced Form Family groups
([`form-families.md`](form-families.md)) and 31 exact Experimental `0.x`
FormRefs. Edge contains 16 and has no current `ObjectBucket`; packages use
`packages.forms.takoform.com/v1alpha5`. Package publication, Form maturity,
Host API version, Specification release, and Provider release are independent.
A repository implementation or local passing gate is not Form publication,
Host Support, activation, or live availability.

Two pre-Beta epochs preceded this one and were **withdrawn** while Takoform is
pre-Stable ([decision
0042](decisions/0042-the-pre-beta-epochs-are-withdrawn.md)). Decision
[`0004`](decisions/0004-takoform-is-an-experimental-specification.md) made the
previously published `forms.takoform.com/v1alpha1` line Legacy after it was
labelled `standard` without sufficient independent implementation and
operational evidence; decision
[`0006`](decisions/0006-v1alpha2-restarts-form-lines.md) restarted selected
kinds in the distinct v1alpha2 epoch. Both epochs' served identities — wire
lanes, schema addresses, corpus and candidate documents — are recorded as
retired in [`../release/published-document-lanes.json`](../release/published-document-lanes.json)
and [`../release/public-schema-identities.json`](../release/public-schema-identities.json),
so a withdrawn address can never quietly answer again meaning something else.
Their bytes stay verifiable in this repository's git history and release tags;
the [`formpackage`](../formpackage/) verifier deliberately keeps every epoch's
schema for that purpose. Historical `standard` and `portable-standard` fields
in those immutable bytes do not define a current approved subset, and nothing
derives current approval or admission from that history.

The literal Host API v1 candidate uses wire `forms.takoform.com/v1`, reached
through `/.well-known/takoform/v1` with API root
`/apis/forms.takoform.com/v1`. The Specification release assertion remains
open while its committed evidence pointers are null. The Host API group is a
protocol compatibility identity independent of every Form group and Form
maturity; Specification 1.1 publication does not publish or promote this Host
API candidate. The current package envelope is
`packages.forms.takoform.com/v1alpha5`; Interface and Binding refs remain
`interfaces.takoform.com/v1alpha1` and `bindings.takoform.com/v1alpha2`. The Terraform provider identity is
`registry.terraform.io/tako0614/takoform`; current Provider `v3.0.0` is
Registry-published for the 31 current Forms, while `v2.1.1` on retained Host
API `forms.takoform.com/v1beta1` is immutable history. Both are independent
from all of these current API identities. The
`release/version.json` descriptor remains `candidate-only` metadata by design
after owner publication.

Provider 3 is a separate Registry-published, non-normative reference
implementation. It may implement the exact current `0.x` Forms without
defining their semantics or blocking Specification 1.1. Publishing
Specification 1.1 does not promote those Forms to `1.0.0`; publishing the
separate Host API v1 candidate would not do so either. A future Stable Form
requires an explicit per-Form decision.

## Normative consistency audit

`go test ./spec` is the cross-specification contradiction gate. It does not
repeat the Form Package verifier, provider schema tests, or portable-host
runner. Instead, it joins their machine-readable inputs and fails when:

- host operations, mutation fences, idempotency, or the stable error taxonomy
  disagree with the portable-host conformance contract;
- the portable API identity, provider candidate version, or canonical provider
  FQN diverges between release, schema, trust, and conformance locks; or
- a normative active Form, package, schema, or host contract leaks a concrete
  backend vocabulary such as Cloudflare/Workers configuration.

The complete repository gate, `bun run check`, runs this audit together with
the deeper package-byte, provider-schema, and generated-surface verifiers.
Passing it remains local evidence only; it does not prove Registry
publication, Host Support, Form maturity, production activation, or
interoperability.
