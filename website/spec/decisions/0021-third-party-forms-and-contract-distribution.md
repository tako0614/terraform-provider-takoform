# 0021 — Third-party Forms and contract distribution wait for a trust model

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

Two surfaces of the v1alpha3 lane promised third-party reach that nothing in
the lane could back.

The provider's generic carrier, `takoform_resource`, accepted any exact FormRef
as four configured strings plus one JSON `spec_json`, and applied it. It
validated the FormRef grammar and that `spec_json` parsed as an object. It
fetched no Form Definition, compiled no desired schema, materialized no
portable defaults, learned no role, and never checked that the Definition a
host serves for that ref hashes to the `schemaDigest` the configuration pinned.
"Carry any third-party Form by exact reference" therefore meant "accept an
exact reference as a string": the first real feedback arrived from the host at
apply time, and the digest — the whole point of an exact ref — was a value
nobody compared anything to.

Earning that promise turns out to be impossible against the lane as published,
and the reason is not effort. `schemaDigest` binds the RFC 8785 canonical
**Form Definition** bytes ([`../versioning.md`](../versioning.md)), and a Form
Definition's required members include `title`, `role`, and
`lifecycleCapabilities`
(`../schemas/form-definition-v1alpha3.schema.json`).
The only surface a host exposes for a Definition is the `form-definition`
operation, whose response is
`host-api-wire-v1alpha3.schema.json#/$defs/formDefinitionResponse`: an
`additionalProperties: false` envelope carrying exactly `identity`,
`displayName`, `description`, and `desiredSchema`. `role` is not in it, and
neither is anything else the digest covers. No other v1alpha3 surface carries a
role either — a Host Support Profile carries `formRef`, `operations`, supported
enums, ranges, bindings, and limits, and no role.

So a client that does not already hold the Definition can never reconstruct the
bytes the pinned digest names, and can never read the Form's role. Both wire
documents are published and immutable
([`../publication-freeze.md`](../publication-freeze.md),
[decision 0014](0014-published-schemas-are-structural-minima.md)); adding a
member to either is a byte change the append-only ledger rejects, and the only
alternative is minting a replacement identity, which 0014 spends its whole
argument against.

What remains implementable is compiling the served `desiredSchema` and
validating `spec_json` against it, materializing that schema's `default`s, and
calling the host's `validate` at plan time. Every one of those rests on a
document whose correspondence to the pinned digest is unverifiable, served by
the same host that will later apply the spec. A carrier doing them would report
schema errors in the plan and look like it had verified the Form — which is
exactly the failure mode where a partial trust model is worse than none.

Interface and Binding contracts have the mirror-image problem, one layer up.
They are distributed as bare Definition documents under
`interfaces/candidates/v1alpha1/` and `bindings/candidates/v1alpha1/`, with no
package envelope: no canonical package digest, no publisher signature, no
revocation feed, no fixture closure, none of the properties
[decision 0010](0010-exact-interface-and-binding-contracts.md) promised when it
said "Interface Packages follow the Form Package rules". Meanwhile 0010's own
opening sentence, this repository's glossary, and
[`../interface-contract/`](../interface-contract/index.md) all describe an
Interface as "independently published" — a claim no artifact, verifier, or
release lane supports.

An envelope cannot be added here either, and again the obstacle is an identity
rather than the work. A Form Package index is
[`../schemas/package-index-v1alpha4.schema.json`](/schemas/v1alpha4/package-index.schema.json):
`additionalProperties: false`, `kind` fixed to `FormPackage`, and `formRef`
required. It cannot carry an InterfaceRef or a BindingRef, so an Interface or
Binding Package needs its own index identity. That is a mint, and this decision
does not make one.

These are MUST-level provider and specification semantics, so this
repository's `AGENTS.md` requires a decision record.

## Decision

Takoform ships what it can verify and says plainly what it cannot. Two rules,
one per surface.

### 1. The v1alpha3 provider lane exposes only typed Form resources

`takoform_resource` is withdrawn from the provider v2.1 surface. It never
shipped in a released provider: v2.1.0 is an unpublished source candidate under
the [publication freeze](../publication-freeze.md), so nothing that exists is
broken by its removal.

The lane's registered resource set is now exactly the typed Edge Platform
Family resources, one per catalog Form. Each compiles its Form's declarations
into the build: the desired schema as typed attributes, the portable defaults
as framework defaults, the role as its replacement and update semantics, and
the exact FormRef as a compiled-in constant its state is bound to. That is what
the removed carrier could not have, and the exactness of the set is itself
enforced — by `internal/standardforms`, by the counting tests in
`internal/provider`, and by the exact-set gates in
`scripts/check-public-surfaces.mjs`.

Supporting a third-party Form means adding it to a build, not naming it in a
configuration. The provider MUST NOT expose a resource whose Form identity is
supplied by configuration until a client can verify that identity, which
requires at minimum that a host be able to serve the canonical Form Definition
bytes the `schemaDigest` covers. That is a new wire surface and therefore a new
schema identity; it belongs to the next coherent generation of identities that
0014 describes, not to this change set.

Nothing else about the lane moves. The exact-identity JSON import ID, the
UID-mismatch rule, and pending-operation resumption of
[decision 0017](0017-provider-state-survives-form-evolution-and-interruption.md)
are unchanged for the typed resources; only 0017's carrier-specific clause
lapses with the carrier.

### 2. Interface and Binding contracts are repository-distributed documents

Interface Definitions and Binding Definitions are digest-bound documents
distributed **with this repository**, under `interfaces/candidates/v1alpha1/`
and `bindings/candidates/v1alpha1/`. They are NOT independently installable
third-party artifacts, and no Interface Package or Binding Package envelope
identity exists, is specified, or is published.

What the exact ref does and does not carry follows directly, and the owning
specifications now say it in those words:

- an `InterfaceRef` or `BindingRef` `schemaDigest` binds the canonical
  Definition bytes, so a document that reaches a consumer intact is
  verifiable **as bytes**;
- it carries no statement about where those bytes came from. There is no
  package digest, no publisher identity, no signature, no revocation feed, and
  no fixture closure over payload files, because there is no package;
- a host therefore obtains these contracts the way it obtains any part of this
  specification, and an operator who installs one from anywhere else has
  exactly the assurance they arranged themselves.

`spec/interface-contract/README.md` and `spec/binding-contract/README.md` state
this as their distribution rule, and the word "published" is removed from the
descriptions of what an Interface is, in those documents and in `CONTEXT.md`.

When an envelope is built, it mints `packages.interfaces.takoform.com` and
`packages.bindings.takoform.com` index identities together with whatever else
the next generation needs, and it satisfies the existing Form Package verifier
rules — one package, one definition, canonical index digest, data-only payload,
closed file inventory, fixture closure — rather than a parallel set.

## Consequences

- A user with a third-party v1alpha3 Form has no provider path today. That is
  the honest state: the previous path did not verify the Form, so what it
  offered was the ability to type an identity, not to rely on one.
- The provider surface is smaller and every resource in it is derived from a
  Form. `internal/standardforms` no longer hand-authors any resource document
  or example, so "every published surface comes from a catalog declaration" is
  now true without exception.
- One conformance obligation is retired, not weakened: the lane's required
  checks are unchanged, because the reason the carrier cannot verify a Form is
  a published wire schema's closed shape, not a behavior any host could get
  right or wrong. Adding a check would test the JSON Schema, not the host.
- The specification stops claiming Interface publication it cannot perform.
  A reader who wants the trust properties of a Form Package now learns from the
  Interface and Binding documents themselves that those properties do not exist
  yet, instead of inferring them from the word "published".
- Restoring third-party reach has a named prerequisite — a host surface serving
  the canonical Form Definition bytes — so the next generation of schema
  identities has one more member to carry, and the work is a design decision
  rather than an implementation backlog item.

## Rejected alternatives

- **Earn the carrier's promise: fetch the Definition, verify its digest against
  the pinned `schemaDigest`, compile the desired schema locally, materialize
  the Form's defaults, read its role, and call `validate` at plan time.**
  Rejected because three of those six are unimplementable against the published
  lane and the other three are misleading without them. The Form Definition
  response is closed to `identity`, `displayName`, `description`, and
  `desiredSchema`, so the canonical Definition bytes are unavailable, the
  digest comparison has no left-hand side, and the role has no source. What
  would have shipped is plan-time validation against an unverified schema plus
  defaults materialized from it — a plan that renders per-field diagnostics and
  a resource that appears to have checked its Form, over a document the pinned
  digest never covered. The cost of doing it anyway is the worst kind: a trust
  model that reports success.
- **Keep the carrier exactly as it is and document the gap.** This is the
  status quo, and its own reference document already carried three
  disclaimers — no local schema compilation, write the Form's defaults
  explicitly, nothing about the trust model changes on import. Rejected because
  a resource that requires an exact `schemaDigest`, forces replacement on it,
  and records it in state states by construction that the digest means
  something. Documentation does not undo an argument the surface makes.
- **Add the missing members to `formDefinitionResponse`, or mint a replacement
  wire schema.** Rejected on 0014's terms. The v1alpha3 wire schema is
  published and immutable, the append-only ledger and the deploy no-overwrite
  guard both reject a byte change, and minting one identity per blocker leaves
  a trail of superseded documents served forever. The surface this needs is
  real and is recorded above as a prerequisite for the next generation.
- **Ship an Interface Package envelope now.** Rejected because it cannot be
  built without minting an index schema identity: `package-index-v1alpha4` is
  `additionalProperties: false` with `kind` const `FormPackage` and `formRef`
  required, so it cannot carry an InterfaceRef, and no unpublished index
  identity exists to extend. Reusing the Form Package index by pretending an
  Interface is a Form would put a non-Form document behind a `FormPackage`
  claim, which is worse than having no envelope.
- **Leave the Interface and Binding documents saying "independently
  published".** Rejected because it is the same defect as the carrier at the
  specification layer: a word that promises a trust property no artifact
  carries. A reader deciding whether to install a third-party Interface is
  entitled to know that installing one is not a thing this project has defined.
