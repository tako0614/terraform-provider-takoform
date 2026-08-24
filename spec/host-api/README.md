# Portable Form Host APIs

## Current Specification contract: Host API v1

Takoform Specification 1.0 defines the literal
`forms.takoform.com/v1` Host API ([`v1.md`](v1.md)):

- discovery: `GET /.well-known/takoform/v1`;
- API root: `/apis/forms.takoform.com/v1`;
- wire schema: [`host-api-wire-v1.schema.json`](../schemas/host-api-wire-v1.schema.json);
- operation table: [`operations-v1.json`](operations-v1.json).

The Specification release candidate remains open until one exact committed
snapshot of the normative `spec/` tree is recorded. Candidate Forms, packages,
reference conformance, Providers, Hosts, products, deployments, and adoption
evidence cannot block or authorize that release. The lane's stable name does
not promote any current `0.x` Form to Form `1.0.0` and does not release Provider
3 ([decision 0055](../decisions/0055-specification-release-needs-only-normative-source.md)).

## Retained pre-v1 design snapshot: Host API v1beta4

[`v1beta4.md`](v1beta4.md) and decision
[0051](../decisions/0051-an-unserved-lane-is-withdrawn-not-stacked.md) are
retained pre-v1 design history. Snapshot-era uses of “current” in those frozen
bytes describe that document's point in time; they do not select today's Host
lane. Decisions [0052](../decisions/0052-the-specification-is-released-on-its-own-line.md)
and [0053](../decisions/0053-specification-and-provider-release-evidence.md)
supersede their lane-selection and release-authority conclusions. The v1beta4
document is not rewritten into v1 and is not a Specification 1.0 prerequisite.

## Retained Provider 2.1.1 lane: Host API v1beta1

Provider `v2.1.1` speaks the immutable
`forms.takoform.com/v1beta1` Host API ([`v1beta1.md`](v1beta1.md)):

- discovery: `GET /.well-known/takoform/v1beta1`;
- API root: `/apis/forms.takoform.com/v1beta1`;
- wire schema: [`host-api-wire-v1beta1.schema.json`](../schemas/host-api-wire-v1beta1.schema.json);
- operation table: [`operations-v1beta1.json`](operations-v1beta1.json).

Provider 2.1.1 is the Registry-published retained distribution that carries
this protocol. `release/version.json` intentionally remains
`publicationStatus: candidate-only` descriptor metadata after owner
publication; the retained signed Registry readback is the Provider availability
evidence. The Beta label here belongs to the Host API, not to Provider 2.1.1
([decision 0035](../decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)).

The retained protocol serves the literal Edge Form Family
`edge.forms.takoform.com/v1beta1`: 15 individual `0.1.0` Form definitions,
each Experimental. Their retained Form Package envelope is the literal
`packages.forms.takoform.com/v1alpha4`; package artifacts remain unpublished.
The nested `interfaces.takoform.com/v1alpha1` and
`bindings.takoform.com/v1alpha1` identifiers are independent contracts, not
Host API maturity labels.

For the current Specification contract, read [`v1.md`](v1.md). For the
retained Provider contract, read [`v1beta1.md`](v1beta1.md). Requirement
keywords are used as described in
[`../conformance.md`](../conformance.md), and the digest-pinned conformance
input for any neutral black-box host runner is
[`conformance/portable-host-v1beta1/contract.json`](../../conformance/portable-host-v1beta1/contract.json). The stable suite is rooted at
[`conformance/takoform-v1/manifest.json`](../../conformance/takoform-v1/manifest.json).

## Withdrawn pre-Beta lanes

Three earlier Host API lanes existed and were withdrawn with their epochs
([decision 0042](../decisions/0042-the-pre-beta-epochs-are-withdrawn.md)):

- `forms.takoform.com/v1alpha1`, the Legacy provider-v1 lane at the
  unversioned `/.well-known/takoform`;
- `forms.takoform.com/v1alpha2`, the provider-v2 lane at
  `/.well-known/takoform/v1alpha2`;
- `forms.takoform.com/v1alpha3`, whose wire the v1beta1 lane carried forward
  ([decision 0038](../decisions/0038-a-generation-move-is-measured-not-assumed.md)).

Their normative documents, operation tables, and wire schemas are recorded as
retired identities in
[`release/published-document-lanes.json`](../../release/published-document-lanes.json)
and remain readable in this repository's git history. A withdrawn lane
identifier is never reused for a different contract, and published provider
releases that speak those lanes (`v1.0.3`, `v2.0.0`) remain immutable Registry
history. A host has no obligation to keep serving a withdrawn lane; a client
that needs one pins the provider release that speaks it and the history that
defines it.
