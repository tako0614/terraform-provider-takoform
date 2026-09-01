# Specification history

> Historical source: this page is retained for the immutable Specification
> 1.0/1.1 receipts and withdrawn compatibility evidence. Numbered
> Specification is not a current version stream. The current contract is the
> stable Host API `forms.takoform.com/v1`; see the
> [current version model](/docs/versions.html) for today’s boundary.

## Identity

An exact **FormRef** is the join of an API group, a kind, a definition version,
and a schema digest. Compatibility is never inferred from a kind name alone:
the same kind in a different epoch is a different contract. Form groups are
namespaced into [Form Families](/spec/form-families.html); the Host API group
is a protocol compatibility identity independent of any nested Form group.

| Surface | Identity |
| --- | --- |
| Historical Specification receipt | Takoform 1.1; first numbered release identity recorded by the append-only numbered ledger. It is historical evidence, not a current API, Form, or Provider version stream (identity 1.0 was never published, is withdrawn, and may not be reused) |
| Current Form corpus | `forms/candidates/current-family-index.json` (8 versionless families; 31 exact Experimental `0.x` Forms) |
| Current Host API | `forms.takoform.com/v1`, discovered at `/.well-known/takoform/v1` (stable wire contract) |
| Current package envelope | `packages.forms.takoform.com/v1alpha5` (unpublished) |
| Provider distribution | Independent: **Provider 3.0.0** is the current Registry-published non-normative reference implementation; Provider 2.1.1 is retained history |

The historical Specification receipt does not alter or promote the stable Host
API v1, relabel any current Form as `1.0.0`, publish a Form Package, mint a
`/v1.1` or v2 lane, or release Provider 3. External Hosts, products,
deployments, signers, and operators are optional adoption evidence rather than
release authority.

The separately generated five-class compatibility report is compatibility
evidence only. It is not part of the publication-evidence document, a release
asset, or a prerequisite. W09 keeps C1 (normative freeze and executable
tooling), C2 (evidence-only source snapshot), C3 (authoritative append-only
publication receipt), and its direct-child C4 (bounded deterministic public
refresh) as distinct linear boundaries. C4 cannot change normative source,
evidence, ledgers, release tooling, Provider/Form/Host source, or unrelated
files. W10 may write only future releases and may never reissue Specification
1.1.

The pre-Beta epochs (`forms.takoform.com/v1alpha1` and `/v1alpha2`) were
withdrawn ([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html));
their identities are retired in the published ledgers and their bytes stay in
repository history. The provider's SemVer is independent of every API
identity, and no current central approval or admission derives from anything
the withdrawn epochs published.

## Current-lane contracts

- [Form Families](/spec/form-families.html) — namespaced Form groups and the
  Edge Platform Family
- [Host API v1](/spec/host-api/v1.html) — uid/generation/revision
  identity, long-running operations, fencing
- [Interface contracts](/spec/interface-contract/) — exact capability
  contracts a Form's service exposes
- [Binding contracts](/spec/binding-contract/) — typed outward capability use
  held by revisions
- [Artifact transport](/spec/artifact-transport/) — content-addressed
  artifact upload and manifests

## Normative schemas

The stable Host/API schema identities are reserved at
`forms.takoform.com/schemas/...` by the append-only local contract lock:

- [form-ref v1](/schemas/v1/form-ref.schema.json)
- [form-definition v1](/schemas/v1/form-definition.schema.json)
- [host-api-wire v1](/schemas/v1/host-api-wire.schema.json)
- [host discovery v1](/schemas/v1/host-discovery.schema.json)
- [package-index v1alpha5](/schemas/v1alpha5/package-index.schema.json)

The withdrawn epochs' schema identities are recorded as retired in
[`release/public-schema-identities.json`](/release/public-schema-identities.json)
and are never reused.

## Lifecycle

A Form moves Proposal → Experimental → Stable → Legacy independently from the
Specification. A future stable Form begins at `1.0.0` only through an explicit
per-Form decision and its own evidence.

## More surfaces

- [Proposals](/proposals/) — mutable design material for Forms that have not
  earned a public FormRef
- [Form inventory](/forms/) — generated candidate and family inventory
- [Conformance](/conformance/) — executable compatibility evidence
- [Release](/release/) — independent Specification and Provider evidence tracks
- [Glossary](/docs/glossary.html) — terms used across the documentation

<StatusNote />
