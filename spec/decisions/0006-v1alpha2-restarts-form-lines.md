# 0006 — v1alpha2 restarts Form lines without reusing v1alpha1 identities

- Status: accepted; the v1alpha2 epoch it creates is retained provider-v2
  preview source since
  [0013](0013-v1alpha3-lane-ships-in-provider-v2-1.md)
- Date: 2026-08-05
- Owners: Takoform maintainers
- Supersedes: the rule in [0004](0004-takoform-is-an-experimental-specification.md)
  that an existing Kind can never restart at `0.x`

## Context

Classifying the pre-reset catalog as Legacy removed its false maturity claims,
but it left those historical version numbers attached to the only visible Form
line. `EdgeWorker@4.0.0` therefore still looked current even though no current
Experimental EdgeWorker existed. Continuing the same version line would make
new design work inherit compatibility expectations from specifications that
the project explicitly stopped recommending.

An exact FormRef includes `apiVersion`, Kind, definition version, and schema
digest. A new API epoch can therefore reuse a useful Kind name and restart its
Form SemVer without overwriting or ambiguously reassigning any published
identity.

## Decision

`forms.takoform.com/v1alpha1` is the frozen Legacy specification epoch.
`forms.takoform.com/v1alpha2` is the current Specification epoch. The epoch is
not itself a maturity state: each definition remains a Proposal until its own
lifecycle record transitions to Experimental.

Package API versions are envelopes, not Form maturity. The already published
`packages.forms.takoform.com/v1alpha2` package schema refers to a v1alpha1
FormRef and is therefore retained unchanged. Current v1alpha2 Forms use the new
`packages.forms.takoform.com/v1alpha3` content-addressed package envelope. This
extra package-profile increment is required by immutable public schema
identity; it does not advance any Form version.

Current v1alpha2 candidates may reuse v1alpha1 Kind names. Every new v1alpha2
Form begins as an unversioned Proposal; a reproducible `0.1.0` source package
may be reviewed before publication, but it becomes an Experimental Form only
after the explicit lifecycle transition and public release. No v1alpha1
version number, maturity label, package locator, provider mapping, or admission
result is inherited by the v1alpha2 Form.

The v1alpha2 Form Definition omits the v1alpha1 document-local `status` field.
Proposal, Experimental, Stable, and Legacy are mutable lifecycle facts owned
only by `forms/lifecycle.json`; duplicating one inside immutable Definition
bytes would create a second authority that becomes stale after a transition.
Published v1alpha1 Definitions retain their exact historical field and bytes.

The 34 v1alpha1 Forms are not mechanically cloned into v1alpha2. Each Kind must
again establish a real workload, owner, host implementation, prior-art review,
portable boundary, fixtures, migration behavior, and security analysis.

The initial design set is the nine Form-backed Resources evaluated against a
dated Takosumi-hosted preview during this reset: EdgeWorker,
RelationalDatabase, ObjectBucket, KeyValueStore, Queue, Schedule,
ContainerService, StatefulEntity, and VectorIndex. That evaluation is
historical provenance only; it does not claim current implementation or offer,
first-host status, or maturity. An independent host implementation or workload
— including a Takosumi deployment — may contribute conformance/adoption
evidence, while Takoform owns maturity and publication authority. The related
`VerifiedDomain` and `AIGateway` capabilities remain separate non-Form
services. Absence from v1alpha2 remains the default for every other former Kind.

Provider SemVer remains independent. Provider v1 is the frozen v1alpha1
compatibility client. Provider v2 is the v1alpha2 client and fails closed on
provider-v1 state instead of pretending it is a state upgrade.

## Consequences

- `EdgeWorker@4.0.0` remains an exact Legacy identity, while
  `forms.takoform.com/v1alpha2 / EdgeWorker@0.1.0` can become a distinct current
  identity after its Proposal earns publication.
- current docs and provider surfaces expose only the nine Proposal-derived
  v1alpha2 source candidates and state that none has yet passed Experimental
  admission;
- a v1alpha2 Definition cannot claim or cache its own maturity; readers join
  its exact FormRef with the lifecycle authority when they need that fact;
- old provider binaries, tags, releases, Resource pins, revocations, and
  recovery paths remain valid and immutable;
- v1alpha1 and v1alpha2 package envelopes remain readable for Legacy Forms,
  while current Forms use the v1alpha3 envelope;
- a host may retain v1alpha1 lifecycle compatibility, but it must advertise and
  activate that Legacy lane explicitly;
- provider users pin v1 for Legacy state or migrate explicitly to provider v2.

## Rejected alternatives

- **Continue EdgeWorker at 5.0.0.** Rejected because it represents the reset as
  another compatibility change inside the specification line being retired.
- **Publish EdgeWorker@0.1.0 in v1alpha1.** Rejected because SemVer resolution
  and human readers would see two conflicting starts in one API epoch.
- **Rename EdgeWorker.** Rejected because the portable concept remains useful;
  the API epoch already provides the identity boundary.
- **Republish all 34 Forms at 0.1.0.** Rejected because it repeats premature
  standardization with smaller numbers.
