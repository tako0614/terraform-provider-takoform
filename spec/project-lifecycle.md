# Project and Form lifecycle

Takoform is an **Experimental specification and tooling project** for portable
desired-state contracts between infrastructure-as-code clients and resource
hosts. It is not currently an industry standard, a certification authority, a
universal cloud API, or a promise that an existing Resource can move between
backends without migration.

This document is the authority for current project positioning, Form maturity,
and the evidence required to change maturity. Exact version compatibility is
defined in [`versioning.md`](versioning.md); package and implementation
conformance are defined in [`conformance.md`](conformance.md).

Machine authority is scoped by identity family. The retained central epochs
and their proposals are recorded in
[`../forms/lifecycle.json`](../forms/lifecycle.json). Its repository-local authoring schema is
[`../forms/lifecycle.schema.json`](../forms/lifecycle.schema.json), and
`standard-form-conformance verify` is the fail-closed semantic validator. The
JSON Schema documents shape; the validator additionally verifies referenced
files, exact package bytes, transition order, owner continuity, independent
maintainers, and the pinned Legacy inventory. The authoring schema has no
public `$id` and is not a published Form contract. Current family maturity and
exact identity are recorded in the generated family candidate set; for the
Beta Edge family that is
[`../forms/candidates/edge/v1beta1/candidate-set.json`](../forms/candidates/edge/v1beta1/candidate-set.json).
The provider compatibility copy is independently locked in
[`../release/provider-form-identities.json`](../release/provider-form-identities.json).

## Independent facts

The following facts MUST remain independent:

| Fact | Authority | Meaning |
| --- | --- | --- |
| Form maturity | Takoform lifecycle record | Confidence in one portable contract |
| Package publication | Takoform publisher evidence | Exact bytes can be retrieved and authenticated |
| Provider compatibility | Provider release and compatibility data | A provider can represent an exact FormRef and preserve its own state contract |
| Host Support | Named host evidence | One host implements the lifecycle for one exact FormRef |
| Form Activation | Host/operator policy | A supported FormRef is usable in a particular scope |
| Service Offering | Commercial platform | Capacity, price, availability, and support for an exact supported FormRef |

No package, provider release, generated catalog entry, host report, activation,
or Service Offering MAY by itself promote Form maturity. A host implementation
MUST NOT describe its support decision as Takoform approval or certification.

The Beta Host API and provider v2.1 path follow the scoped release policy in
[`publication-freeze.md`](publication-freeze.md). Open independent-host,
third-party-ecosystem, and Cloud-GA evidence remains required for later
Stable/GA claims and Form Package/public-service publication, but does not
block a stable provider release that embeds the exact Beta identities.

## Form Families

Forms are grouped into named Form Families with namespaced API groups
([`form-families.md`](form-families.md),
[decision 0009](decisions/0009-form-families-and-namespaced-api-versions.md)).
A family is a catalog and namespace fact only; its API channel does not confer
Form maturity. Each member still has an explicit maturity classification. The
current `edge.forms.takoform.com/v1beta1` candidate set contains exactly 15
`0.1.0` Forms and classifies all of them Experimental. Its unpublished
`packages.forms.takoform.com/v1alpha4` artifacts are a separate publication
fact. Adding a Form to a family, or publishing one family member, promotes
nothing else. The retained
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

An Experimental Form MUST NOT become Stable until:

- at least two independently maintained host implementations exercise the same
  exact semantics, or equivalent evidence demonstrates that the contract is
  implementable without relying on one host's private model;
- real consumers have operated the Form across a documented compatibility
  window;
- immutable fields, replacement, import, drift, delete, and recovery behavior
  agree across implementations;
- interoperability has been exercised using packages published by a party
  other than at least one consuming host;
- migration from the latest Experimental line is documented and tested;
- deprecation and security-revocation procedures have been exercised on a real
  release line;
- known limitations do not contradict the claimed portable semantics.

Stable commits Takoform to the stable SemVer rules in
[`versioning.md`](versioning.md). It does not guarantee that every host supports
the Form or that a commercial platform offers it.

Takosumi GA is a qualification checkpoint, not an automatic transition. At
that checkpoint only contracts with the required evidence above may mint a
Stable `1.0.0` identity; all others remain Experimental/Beta without being
renamed or overwritten.

The machine record requires the exact transition history `proposal →
experimental → stable`, two distinct host subjects with two independently
named maintainers, real-consumer evidence, lifecycle agreement,
interoperability, independent publication, migration, deprecation-exercise,
and security-revocation-exercise files. A direct Proposal-to-Stable record is
invalid even if other evidence is present.

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

## Deprecation and security revocation

Deprecation is a consumer migration contract. It identifies the reason,
successor, timing for rejecting new create/apply, retained lifecycle behavior,
and migration instructions.

Security revocation is separate and append-only as defined by
[`trust/`](trust/). It may block new creation, update, or activation while the
referenced bytes remain available for observation, deletion, recovery, or an
explicit operator evacuation path.
