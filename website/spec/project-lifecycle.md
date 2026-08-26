# Project and Form lifecycle

Takoform is a portable desired-state specification and tooling project. The
repository defines **Specification 1.1** and carries a literal Host API v1
source candidate. Host API v1 is a separate, unpublished protocol identity;
publishing Specification 1.1 does not publish or promote it. Specification
publication state is derived from the append-only numbered release ledger, not
hard-coded in this lifecycle document. Takoform is not an industry
standards body, certification authority, universal cloud API, or a promise that
an existing Resource can move between backends without migration.

This document is the authority for current project positioning, Form maturity,
and the evidence required to change maturity. Exact version compatibility is
defined in [`versioning.md`](versioning.md); package and implementation
conformance are defined in [`conformance.md`](conformance.md).

Machine authority is scoped by identity family. The withdrawn central epochs'
lifecycle ledger went with them (decision 0042) and stays in git history. The
closed current inventory is generated at
[`../forms/candidates/current-family-index.json`](../forms/candidates/current-family-index.json):
eight versionless families and 31 exact Experimental FormRefs, including 16 in
Edge and no current `ObjectBucket`. Each family candidate set records its exact
maturity and identities. The retained Provider 2.1.1 compatibility copy is
independently locked in
[`../release/provider-form-identities.json`](../release/provider-form-identities.json).

## Independent facts

The following facts MUST remain independent:

| Fact | Authority | Meaning |
| --- | --- | --- |
| Specification release | Takoform publication evidence and numbered release ledger | One exact committed snapshot of the normative `spec/` tree |
| Form maturity | Takoform lifecycle record | Confidence in one portable contract |
| Package publication | Takoform publisher evidence | Exact bytes can be retrieved and authenticated |
| Provider compatibility | Provider release and compatibility data | A provider can represent an exact FormRef and preserve its own state contract |
| Host Support | Named host evidence | One host implements the lifecycle for one exact FormRef |
| Form Activation | Host/operator policy | A supported FormRef is usable in a particular scope |
| Service Offering | Commercial platform | Capacity, price, availability, and support for an exact supported FormRef |

No Specification release, package, provider release, generated catalog entry,
host report, activation, or Service Offering MAY by itself promote Form
maturity. A host implementation MUST NOT describe its support decision as
Takoform approval or certification.

Specification 1.1 follows decision
[`0057`](decisions/0057-specification-1-1-compatibility-and-independent-identities.md),
which amends decisions
[`0052`](decisions/0052-the-specification-is-released-on-its-own-line.md) and
[`0053`](decisions/0053-specification-and-provider-release-evidence.md), as
amended by [`0055`](decisions/0055-specification-release-needs-only-normative-source.md):
only the exact committed normative source snapshot satisfies release readiness,
and the numbered ledger records publication. Candidate
Forms, reference conformance, Provider 3, and external Host,
backend, runtime, production, signer, operator, Takoserver, and Takosumi facts
are independent adoption evidence. Provider 2.1.1 and the v1beta1 identities
remain immutable history.

## Form Families

Forms are grouped into named Form Families with namespaced API groups
([`form-families.md`](form-families.md),
[decision 0009](decisions/0009-form-families-and-namespaced-api-versions.md)).
A family is a catalog and namespace fact only; its group does not confer Form
maturity. Each member still has an explicit maturity classification. The
current versionless family index contains exactly eight families and 31
Experimental Forms; Edge contains 16 and no `ObjectBucket`. Current packages
use `packages.forms.takoform.com/v1alpha5`, and package publication is a
separate fact. Adding a Form to a family, publishing Specification 1.1, or
publishing the separate Host API v1 candidate or one family member promotes
nothing else. The retained Provider
2.1.1 projection remains the versioned v1beta1 Edge family, 15 exact FormRefs,
and `packages.forms.takoform.com/v1alpha4`. The
`forms.takoform.com/v1alpha2` candidates are superseded provider-v2 preview
source ([decision 0035](decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md));
they are not a published current Form line and are not the basis for new
specification work.

## Form lifecycle

### Proposal

A Form Proposal is mutable, unversioned design material. It has no public
FormRef, package release, compatibility promise, or maturity claim. A Proposal
MAY change incompatibly or be removed without deprecation.

A Proposal MUST name:

- the real workload and consumer that need it;
- the maintainer responsible for its portable semantics;
- at least one intended host implementation;
- the desired-state boundary and the decisions left to the host;
- lifecycle risks, including replacement, data loss, delete, import, and drift;
- the credential, network, artifact, and secret boundary;
- relevant prior art and the concrete reason an existing abstraction is not
  sufficient.

A Proposal MUST also classify every desired field using the portable boundary
in [`portability-boundary.md`](portability-boundary.md). A first host or
commercial offering is workload evidence, not permission to copy its product
model into a Form.

The prior-art review MUST include OCCI, CIMI, TOSCA where applicable,
Kubernetes/Crossplane APIs, and established Terraform/OpenTofu provider APIs
for the represented capability. The review is a design obligation, not a claim
of compliance.

Every Proposal registry entry records all five prior-art families explicitly
and marks each `applicable` or `not-applicable` with a finding. A Proposal entry
cannot contain a FormRef, package digest, version, maturity state, or release
identity: strict decoding rejects those fields. Its ID is only an internal link
from later lifecycle evidence and is not a public compatibility identity.

Creating or revising a Proposal does not require release authority. Publication
does.

The authoring workspace and copyable review outline are
[`../proposals/`](../proposals/). A Proposal exists in current authority only
when its document and matching registry entry are both present. The semantic
validator compiles the repository-local authoring schema on every verification;
an uncompiled or drifting schema is a failing gate, not advisory documentation.

### Experimental

An Experimental Form is a reproducible public contract on a `0.x` version
line. Its released bytes are immutable, but its semantics may still change
under the `0.x` policy in [`versioning.md`](versioning.md).

A Proposal MAY become Experimental only when all of the following are present:

- one exact canonical Form Definition and FormRef;
- positive and negative fixtures covering the portable semantics;
- a host implementation that exercises create/read/update/delete and the
  applicable observe, refresh, import, replacement, and recovery paths;
- known limitations and unresolved portability questions;
- compatibility classification and a migration or rollback note;
- security-boundary review;
- generated reference and narrative documentation that agree;
- immutable FormRef and canonical Definition identity;
- package provenance, signature, and public readback plan before package
  publication.

Passing these checks prepares a publication. It does not authorize one.

An immutable Definition contains no maturity field. The scoped lifecycle or
family record is the authority for Experimental, Stable, and later Legacy
transitions, so a maturity change never requires rewriting or duplicating a
fact inside Definition or package bytes. A provider-first release may lock an
Experimental FormRef and Definition/package digest before the package artifact
is published; that lock is still immutable and package publication remains a
separate operation.

### Stable

A Stable Form is an evidence-earned portable contract. Stable is not a central
approval applied to a preferred subset.

An Experimental Form becomes Stable only through an explicit per-Form decision
that mints its own `1.0.0` identity. That decision MUST bind:

- the exact predecessor FormRef and a compatibility analysis proving what is
  preserved or deliberately changed;
- complete positive, negative, lifecycle, relation, constraint, output, and
  failure fixtures for the Form's portable semantics;
- the exact family conformance corpus and reference result that exercise the
  proposed stable Definition;
- migration or explicit no-migration guidance from the latest Experimental
  identity;
- immutable-field, replacement, import, drift, delete, recovery, and security
  boundary review; and
- known limitations that do not contradict the claimed portable semantics.

Independent implementations, production consumers, compatibility windows,
cross-publisher installation, deprecation exercises, and revocation exercises
SHOULD be recorded when they exist. They strengthen adoption confidence but do
not give a Host, product, Provider, backend, signer, or operator normative
authority over Specification 1.1 or another Form. Decisions 0044 and 0046 are
retained as the history of that adoption-evidence program and are superseded
by decision 0053 as release authorities.

Stable commits Takoform to the stable SemVer rules in
[`versioning.md`](versioning.md). It does not guarantee that every host supports
the Form or that a commercial platform offers it.

No Takosumi or other product GA milestone triggers a transition. Publishing
Specification 1.1 likewise leaves every current `0.x` Form
Experimental; publishing the separate Host API v1 candidate has the same
non-effect. The machine record requires the exact transition history
`proposal → experimental → stable`, the per-Form decision and bound contract
evidence above, and an explicit `1.0.0` FormRef. A direct Proposal-to-Stable
record is invalid even if adoption evidence is present.

### Legacy

A Legacy Form is an immutable published identity retained for compatibility,
recovery, and explicit migration but no longer used as the basis for new
specification work. Legacy status MUST identify the reason, exact affected
FormRefs, successor or alternative when one exists, new-create policy, and the
retained read/observe/delete/recovery behavior.

Moving a Form to Legacy MUST NOT delete or overwrite its definition, package,
tag, signature, revocation evidence, or migration material.

## Existing public line

The public line created before decision
[`0004`](decisions/0004-takoform-is-an-experimental-specification.md) is a
retained **Legacy line**. Its exact artifacts and historical `standard` and
`portable-standard` fields remain verifiable facts about those documents, but
they are not current maturity states and do not define an approved subset.

In particular:

- the generated 34-Form `standard` catalog is not evidence that 34 Forms have
  satisfied the Stable criteria above;
- the retained ten-Form `portable-standard` admission closure is historical
  signed lifecycle evidence for exact identities, not a normative ranking;
- a Takosumi or Takosumi Cloud implementation count is Host Support or Service
  Offering data, not Form maturity;
- existing Resources keep their exact pins and MUST retain safe observation,
  deletion, recovery, and explicit migration paths.

New lifecycle data MUST NOT rewrite the immutable legacy Form Definitions merely
to replace their historical status field. Reader-facing tools MUST project the
current Legacy classification alongside the original document truth.

The top-level `legacy` object pins that pre-reset line by a canonical digest of
every exact public release identity. `currentForms` is scoped to the retained
central post-reset epoch; current namespaced families use their own generated
candidate-set maturity record. This separation prevents the old generated
catalog or admission subset from being silently reinterpreted as new maturity
evidence.

## Change authority

Every public Form change MUST update, in the same reviewed change set:

- canonical definition and fixtures;
- compatibility classification and migration effect;
- lifecycle and security risks;
- generated reference documentation;
- the applicable scoped maturity record;
- provider compatibility only when provider behavior changes;
- Host Support only when the named host's evidence changes.

Machine checks MUST fail closed on an unknown lifecycle state, a missing owner,
a changed published byte, a maturity claim inferred from provider/host/Cloud
data, or a direct Proposal-to-Stable transition.

The lifecycle facts authored, verified, packaged, published, installed,
supported, activated, provisioned, client-supported, and offered are
independent. A Specification release, package installation, Provider mapping,
Host Support report, or Service Offering MUST NOT imply any other fact.

One closed data-only Form Package contains one Form Definition and one exact
FormRef; a catalog or compatibility set is an external mapping, never a
multi-Form package. Official and external publishers use the same authoring,
verification, and admission path. External authors import only public Core
contracts and never internal Provider packages. The eventual public module is
`github.com/tako0614/takoform`; C1 publishes no SDK from that coordinate.

## Deprecation and security revocation

Deprecation is a consumer migration contract. It identifies the reason,
successor, timing for rejecting new create/apply, retained lifecycle behavior,
and migration instructions.

Security revocation is separate and append-only as defined by
[`trust/`](trust/). It may block new creation, update, or activation while the
referenced bytes remain available for observation, deletion, recovery, or an
explicit operator evacuation path.
