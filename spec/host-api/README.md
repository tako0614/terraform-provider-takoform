# Portable Form Host APIs

## Beta 2 design target: Host API v1beta2

`forms.takoform.com/v1beta2` ([`v1beta4.md`](v1beta4.md)) is the Beta 2
generation's lane, minted for protocol reasons out of the hardening review
(decisions [0039](../decisions/0039-a-lane-is-minted-for-one-of-two-reasons.md),
[0046](../decisions/0046-stable-arrives-through-a-stable-grade-beta-2.md)):

- discovery: `GET /.well-known/takoform/v1beta2`;
- API root: `/apis/forms.takoform.com/v1beta2`;
- wire schema: [`host-api-wire-v1beta4.schema.json`](../schemas/host-api-wire-v1beta4.schema.json);
- operation table: [`operations-v1beta4.json`](operations-v1beta4.json).

No released provider speaks it yet; the 3.0.0 release will. Until then the
published current lane below keeps its meaning unchanged.

## Current published lane: Host API v1beta1

The lane published provider `v2.1.1` speaks is the literal
`forms.takoform.com/v1beta1` **Host API** ([`v1beta1.md`](v1beta1.md)), a Beta
protocol channel with its own discovery and operation documents:

- discovery: `GET /.well-known/takoform/v1beta1`;
- API root: `/apis/forms.takoform.com/v1beta1`;
- wire schema: [`host-api-wire-v1beta1.schema.json`](../schemas/host-api-wire-v1beta1.schema.json);
- operation table: [`operations-v1beta1.json`](operations-v1beta1.json).

Human Provider 2.1.1 is the Registry-published stable-distribution SemVer that
carries this protocol. `release/version.json` intentionally remains
`publicationStatus: candidate-only` descriptor metadata after owner
publication; the retained signed Registry readback is the Provider availability
evidence. The Beta label here belongs to the Host API, not to Provider 2.1.1
([decision 0035](../decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)).

The current protocol serves the literal Edge Form Family
`edge.forms.takoform.com/v1beta1`: 15 individual `0.1.0` Form definitions,
each Experimental. Their current Form Package envelope is the literal
`packages.forms.takoform.com/v1alpha4`; package artifacts remain unpublished.
The nested `interfaces.takoform.com/v1alpha1` and
`bindings.takoform.com/v1alpha1` identifiers are independent contracts, not
Host API maturity labels.

For the complete current contract, read [`v1beta1.md`](v1beta1.md).
Requirement keywords are used as described in
[`../conformance.md`](../conformance.md), and the digest-pinned conformance
input for any neutral black-box host runner is
[`conformance/portable-host-v1beta1/contract.json`](../../conformance/portable-host-v1beta1/contract.json).

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
