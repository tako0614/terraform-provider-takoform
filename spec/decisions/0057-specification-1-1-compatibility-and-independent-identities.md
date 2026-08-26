# 0057 — Specification 1.1 freezes compatibility and independent identities

## Status

Accepted for the W09 Specification 1.1 candidate. This decision supersedes the
numbered-release wording in decisions 0052, 0053, and 0055 where those records
refer to Specification 1.0 or require implementation evidence. Those records
remain historical decisions; the current machine authority is
[`../publication-evidence.json`](../publication-evidence.json),
[`../../release/specification-releases.json`](../../release/specification-releases.json).
The generated
[`../../release/specification-compatibility.json`](../../release/specification-compatibility.json)
is a separate compatibility report and is not release authority.

## Decision

The first numbered Specification release is **1.1**. Identity `1.0` was never
published; it is withdrawn and retained as history and MUST never be reused. A
future W10 Core owner may become the writer for future
releases, but MUST NOT reissue, rewrite, or retag Specification 1.1.

The current repository remains the authority for this candidate:
`https://github.com/tako0614/terraform-provider-takoform.git`. The transfer of
future writer authority is a later operation. C1 does not create an SDK tag or
publish the future module, whose eventual Go coordinate is
`github.com/tako0614/takoform`.

### Compatibility and identity

Specification 1.1 has no Host API, Form-package, or Provider effect:
`hostApiEffect`, `formPublicationEffect`, and `providerEffect` are all `none`.
The literal Host API v1 bytes remain a separate unpublished candidate. Every
current FormRef, package digest, family identity, Interface, Binding, artifact
transport identity, standard-service slot, and Provider identity remain
unchanged. Publishing Specification 1.1 does not publish or promote Host API
v1. There is no `/v1.1` Host lane, v2 schema, v2 tag, or v2 receipt.

The generated compatibility manifest is exactly five classes:

1. Form and Package;
2. Host API lifecycle;
3. Form Family and Host Support;
4. Interface, Binding, artifact transport, and standard service; and
5. trust, revocation, lifecycle, version, and release identities.

Every entry has a status (`retained`, `new-independent`,
`unpublished-candidate`, or `withdrawn-retained`), raw source-byte digests,
an owning ledger, and an explicit migration disposition. The manifest
byte-pins the literal Host API v1 source set. Current candidate Forms,
candidate packages, and the literal Host API v1 remain
`unpublished-candidate`. Withdrawn document and schema lanes remain
`withdrawn-retained`; every published Provider identity from 1.0.1 through
3.0.0 remains `retained`. A manifest is compatibility evidence, not
publication evidence, a release asset, a prerequisite, or a publication
receipt.

### Publisher, trust, and provenance

Official and external publishers are equal at the Form and Package contract
boundary. “Official” is a publisher/source designation, not a stronger FormRef
or schema rule. An operator explicitly selects the trusted source, issuer,
signature/revocation policy, and host support policy. Takoform does not infer
trust from a product name, a deployment, a Terraform Provider, or a catalog.

Provenance is required for admission and publication, but it is deliberately
outside FormRef equality. A FormRef is exactly its group, kind,
definitionVersion, and schemaDigest; publisher/source/issuer/provenance facts
MUST NOT be smuggled into that equality key.

One closed data-only Form Package contains exactly one Form Definition and its
content-addressed package files. It does not contain executable code, a host
implementation, a Provider, a generic resource, or a second Form. Package
install does not itself grant Host Support, activation, or a Service Offering.

### Revocation and lifecycle

Package revocation is append-only and applies to the exact package identity.
After a valid revocation is observed, create, update, and activation MUST stop.
Observe/read, delete, and an explicit operator evacuation path remain
available so an operator can inspect and safely remove retained state. A
revocation does not rewrite package bytes or erase a Resource.

Authored, verified, packaged, published, installed, supported, activated,
provisioned, client-supported, and offered are independent lifecycle facts.
Closing one fact MUST NOT imply another. In particular, Specification release
does not publish a package, support a Form, activate a Resource, or establish a
commercial offering.

### Authoring, Provider, and evolution

An external author follows the same public authoring path as an official author
and MUST import only public Core contracts; internal Provider or repository
packages are not part of the path. The future Core module is
`github.com/tako0614/takoform`, but no SDK or CLI publication is prepared in
C1.

Provider 3 remains a typed, non-normative implementation projection. It may
support only explicitly compiled typed Forms and MUST NOT add an opaque generic
JSON resource as a substitute for typed schema/state contracts.

Form and package releases evolve independently of the numbered Specification
and Provider releases. Decision 0041's Provider-embedded package rule is
historical and superseded for current releases; a package publisher owns its
own publication decision and cadence. An existing Form/package evolution
mechanism (new definitionVersion and digest, with explicit migration and
retained identity rules) is sufficient; changing a Form does not require a
Specification or Provider bump when the Host API and package contract remain
compatible. No fixed package cadence has been adopted.

Release cadence is a **Proposal** only. No annual/major cadence, independent
Form cadence, or Provider/Host schedule is a publication prerequisite until a
later decision records owner, evidence, and migration rules.

## Consequences

The W09 release ledger has one 1.1 candidate and an empty `releases` array.
The sole prerequisite is `specification-source-snapshot`; publication evidence
for Forms, the reference Host, and Provider 3 remains independently visible
and unprepared. C1 freezes the normative source and executable tooling while
the evidence record is empty; C2 records only the exact source-snapshot
evidence; C3 appends the authoritative publication receipt and its projections.
The compatibility report is checked separately and never enters those records.
A create-only release surface may prepare and publish 1.1 only from one exact
reviewed W09 owner commit and must fail closed on a pre-existing 1.1 identity.
C1 does not execute that surface. W10 may own future releases but must never
reissue, rewrite, or retag 1.1.
