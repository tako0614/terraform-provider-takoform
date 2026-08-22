# Versioning and compatibility

Takoform versions protocol, Forms, packages, and the Terraform/OpenTofu
provider for different reasons. Their numbers MUST NOT be aligned to imply a
shared release or maturity level. Form maturity and the complete lifecycle are
defined in [`project-lifecycle.md`](project-lifecycle.md).

## Version axes

| Concern      | Identifier                                      | Meaning                                                                                    |
| ------------ | ----------------------------------------------- | ------------------------------------------------------------------------------------------ |
| Host API     | API group such as current `forms.takoform.com/v1beta1` | Protocol envelope, discovery, and lifecycle compatibility                              |
| Form group   | DNS-like API group inside an exact `FormRef`    | Namespace boundary between Form Families and withdrawn epochs; versioned per family         |
| Form         | SemVer inside an exact `FormRef`                | Compatibility of one portable desired-state contract within its group                      |
| Form Package | Exact package identity plus content digest      | Immutable distribution of one exact Form and its fixtures                                  |
| Interface / Binding contract | Exact ref plus schema digest       | Immutable operation-surface and typed-capability contracts referenced by Forms             |
| Provider     | Provider SemVer                                 | Terraform/OpenTofu protocol, typed surface, persisted state, and host-client compatibility |

### A name is absolute or it rots

The axes above are independent by design, so their numbers disagree, and a
reader who expects them to agree cannot tell which axis a `v1beta1` or a `v3`
belongs to. What separates a version word that stays true from one that goes
quietly false is not the axis, though. It is whether the name is **absolute**
or **relative**.

An **absolute** name states which thing it names: `forms.takoform.com/v1beta1`
means that lane and no other, on the day it was written and afterwards. A
**relative** name states a position — `current`, or the Nth of a sequence — and
is true only until the thing it points at moves. Nothing announces the move, so
a relative name has to be rewritten by hand, and the rewrite is what gets
missed.

It has been missed. When the current lane became `forms.takoform.com/v1beta1`
the absolute names were minted correctly — [`host-api/v1beta1.md`](host-api/v1beta1.md)
beside the then-retained `host-api/v1alpha3.md` — while
`conformance/portable-host-v3`, named for its place in a sequence, was rewritten
in place instead. One published address then answered about a different contract
than it had answered about the day before, which the retention rule of
[decision 0035](decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)
forbids. The corpus now lives at `conformance/portable-host-v1beta1`, named for
its lane, and two checks in `bun run check` refuse both the symptom and the
cause. The v1alpha3 corpus was restored to its own address first and then
withdrawn with the family it measured
([decision 0037](decisions/0037-immutability-begins-at-stable.md)), and both
pre-Beta epochs later followed it out
([decision 0042](decisions/0042-the-pre-beta-epochs-are-withdrawn.md)).

So: **a new artifact whose name carries a version word names the lane it
describes, never its place in a sequence.** Already-published relative names are
retained history and stay as they are — the rule binds what is minted next.

### `current` is one word again

Two things in this repository used to be called current, two generations
apart: the current Beta lane, and the retained central Form epoch
(`forms.takoform.com/v1alpha2`) that `formpackage.CurrentFormAPIVersion` and
the `internal/currentform*` packages meant. The withdrawal of the pre-Beta
epochs ([decision 0042](decisions/0042-the-pre-beta-epochs-are-withdrawn.md))
removed the second referent: current now names the
`forms.takoform.com/v1beta1` lane, the `edge.forms.takoform.com/v1beta1`
family, and nothing else. A future generation move re-creates the ambiguity
only if a new name reuses the word instead of stating its lane.

### What freezes each value

An axis says what a number means; it does not say whether the number may move.
That is decided by what has already published it.

| Value | Frozen by |
| --- | --- |
| Lane `forms.takoform.com/v1beta1`, family `edge.forms.takoform.com/v1beta1`, the 15 exact FormRefs and their digests | Registry-published provider `v2.1.1`, recorded append-only in [`../release/provider-form-identities.json`](../release/provider-form-identities.json) |
| Every `spec/schemas/` filename and `$id` | [`../release/public-schema-identities.json`](../release/public-schema-identities.json), enforced append-only across the whole committed history of that ledger |
| `packages.forms.takoform.com/v1alpha4` | it is inside published schema bytes |
| The lane each published document declares | [`../release/published-document-lanes.json`](../release/published-document-lanes.json) |
| `forms.takoform.com/v1alpha1`, `/v1alpha2`, `/v1alpha3`, `edge.forms.takoform.com/v1alpha1`, their schemas, corpora, and served documents | **withdrawn** under [decisions 0037](decisions/0037-immutability-begins-at-stable.md)/[0042](decisions/0042-the-pre-beta-epochs-are-withdrawn.md); recorded as retired in [`../release/published-document-lanes.json`](../release/published-document-lanes.json) and [`../release/public-schema-identities.json`](../release/public-schema-identities.json), bytes in git history and release tags |
| Internal package, directory, and script names | nothing; they are free to be made absolute |

Since [decision 0009](decisions/0009-form-families-and-namespaced-api-versions.md)
the FormRef group is a namespaced DNS-like identifier. Official families use
subdomains of `forms.takoform.com` (the first is
`edge.forms.takoform.com/v1beta1`); third-party groups are valid FormRefs
under their own domains. The withdrawn `forms.takoform.com/v1alpha1` and
`forms.takoform.com/v1alpha2` groups are retired identities; new families
never reuse them. Each family group versions independently.

Host Support, Form Activation, and Service Offering records refer to exact
identities but are not version streams. Updating one of those facts MUST NOT
change a Form or provider version.

Current family packages use the `packages.forms.takoform.com/v1alpha4`
envelope, which carries namespaced FormRef groups; the three earlier envelope
identities are retired with their epochs.

**The envelope is a manifest format, not a version axis**
([decision 0040](decisions/0040-the-package-envelope-is-a-format-not-an-axis.md)).
Its four published values record one real format change — `v1alpha1` to
`v1alpha2` introduced content addressing — and two re-mints that followed the
FormRef grammar they wrap; normalising version words, the `v1alpha2`,
`v1alpha3` and `v1alpha4` schemas are structurally identical. A new envelope is
minted only when the manifest format itself changes structurally, never because
a Form generation moved: `v1alpha4` already admits any namespaced group and has
carried two family generations without moving, which is the proof it never
needed to track them. Both halves are checked the same way the Host API lane's
minting reasons are.

**Takoform's own Form Packages publish with the provider release that embeds
them** ([decision 0041](decisions/0041-form-packages-publish-with-the-provider-release.md)):
one release train, with each release's exact package digests locked append-only
in [`../release/provider-form-identities.json`](../release/provider-form-identities.json).
The identity stays content-addressed and provider-free, because a third-party
publisher — which the Stable criteria require — publishes under its own cadence. Content-addressed packages have no independent
SemVer. Their exact package digest produces the publication artifact ID
`sha256-<hex>` and therefore the source path and tag. The withdrawn epochs'
`packageVersion` values and package profiles are retired identities; their
bytes stay in git history and `forms/*` release tags, where the `formpackage`
verifier still verifies paths, tags, signatures, and bytes without
interpreting a package profile as Form maturity. The content-addressed locator decision is recorded in
[`decisions/0005`](decisions/0005-current-form-packages-use-content-addressed-locators.md),
the v1alpha3 envelope required by the Form epoch reset is recorded in
[`decisions/0006`](decisions/0006-v1alpha2-restarts-form-lines.md), and the
family-carrying v1alpha4 envelope was minted by the now-superseded
[`decision 0013`](decisions/0013-v1alpha3-lane-ships-in-provider-v2-1.md), and
its use by the current Beta family is fixed by
[`decision 0035`](decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)
(the namespaced family groups it carries are decided in
[`decisions/0009`](decisions/0009-form-families-and-namespaced-api-versions.md)).

## Provider versions are independent

A provider release version describes only:

- Terraform/OpenTofu protocol compatibility;
- resource and data-source schema compatibility;
- persisted provider state and state-upgrade behavior;
- host API client behavior and supported protocol capabilities.

It does not describe Form maturity, package publication order, Host Support,
Form Activation, a Service Offering, or a historical admission generation.

Within a stable provider major, a release MUST NOT remove an existing resource
type, make an existing valid configuration invalid, silently reinterpret
persisted state as a different FormRef, or discard a supported state migration.
Such a change requires a new provider major. Compatible optional fields, new
resource types, expanded exact-Form compatibility, and bug fixes MAY remain in
the current major when they preserve those promises.

A provider release MAY support a mixed set of Form versions and maturity
states. Changing the provider major MUST NOT reset, renumber, promote, or
deprecate a Form. Changing a Form MUST NOT require a provider release when the
provider can already carry that Form's data and exact identity correctly.

Provider `v2.1.1` is therefore a Registry-published stable provider version
that targets the Beta Host API `forms.takoform.com/v1beta1` and 15 exact
Experimental FormRefs in `edge.forms.takoform.com/v1beta1`. The release
descriptor remains `candidate-only` metadata by design after owner publication;
that descriptor status does not make the SemVer a prerelease. The exact Beta
FormRefs and definition and package digests embedded by this provider release
are immutable provider compatibility data even while their package artifacts
remain unpublished.

A later Stable `1.0.0` Form is a new exact identity. Existing Beta state remains
bound to its Beta FormRef and codec for read, refresh, update, and delete; a
provider MUST NOT silently select a future stable default during refresh or
rewrite that state as the stable identity.

The provider compatibility decision is recorded in
[`decisions/0001-provider-v1-keeps-form-versions-independent.md`](decisions/0001-provider-v1-keeps-form-versions-independent.md).
Current provider release facts belong to [`../release/`](../release/README.md),
not this compatibility policy.

## Form versions

### Proposal

A Form Proposal has no public version. Proposal edits MAY be breaking and a
Proposal MAY be withdrawn without reserving a FormRef.

### Experimental `0.x`

The first reproducible public version of a new Form line is `0.1.0`.

- A breaking semantic or schema change increments the minor version and resets
  the patch version.
- A compatible addition increments the minor version.
- A compatible correction that does not change the accepted desired contract
  increments the patch version.

A `0.x` identity is never OVERWRITTEN: while it is served it means exactly what
it meant when it was published, and a breaking correction mints a new identity
rather than editing an occupied one. That rule has no exceptions and is what
makes an exact FormRef worth recording in state.

What a `0.x` identity does NOT carry is a promise to be served forever. It MAY
be **withdrawn** while Takoform is pre-Stable
([decision 0037](decisions/0037-immutability-begins-at-stable.md)); permanence
begins at Stable, with the identity that earns it. Withdrawal is a recorded act
and never a silence: the identity moves to the `retired` list of the ledger that
published it — `release/public-schema-identities.json` for a schema,
`release/published-document-lanes.json` for a served document — keeping the
bytes and the lane it was published with, so the ledger still answers what that
address meant after it stops answering. A writer will not drop a published
address for you, a withdrawn address may not be reused for something else, and
none of this weakens the rule above: an identity that is still served must still
match, byte for byte.

Retiring an identity does not destroy it. The bytes stay in this repository's
history and under its immutable tags, and a released provider embeds the schemas
it needs rather than fetching them, so a withdrawal changes what this project
serves and not what an installed client can do.

The 15 current Beta family Forms are exactly `0.1.0` and Experimental. A
breaking correction to the Beta protocol or family contract mints
`forms.takoform.com/v1beta2` and/or a new
`edge.forms.takoform.com/v1beta2` FormRef as applicable; it never edits the
occupied v1beta1 identity or its published-schema bytes.

### Stable `1.x+`

A Form MAY begin a stable major only after satisfying the Stable criteria in
[`project-lifecycle.md`](project-lifecycle.md). The initial earned stable line
begins at `1.0.0` for a new kind.

- A patch MUST preserve the desired schema and portable semantics. It MAY fix
  documentation, fixtures, or non-semantic metadata only when the package
  identity changes without rewriting prior bytes.
- A minor MAY add optional data or relax a constraint. Every previously valid
  desired document MUST remain valid with the same portable meaning.
- A major MAY remove data, tighten a constraint, change meaning, or require
  replacement or explicit state migration.

Schema compatibility checks are conservative. When tooling cannot prove that a
change is compatible, the change MUST be treated as breaking or remain a
Proposal until the proof is improved.

### What a Form version change costs a client

A client that persists state binds each resource to the exact FormRef it was
applied under, and it addresses that resource under that identity for the rest of
its life ([decision 0017](decisions/0017-provider-state-survives-form-evolution-and-interruption.md)).
A Form line that advances therefore leaves state behind at the older identity,
and a client MUST be able to keep serving it.

The provider carries one **codec** per supported exact FormRef — the field set
that definition declared, used both to decode the state written under it and to
encode the spec sent for it. Which codec a definition needs follows directly from
the compatibility rules above:

- an **additive minor** MAY share one codec with the definitions before it, because
  every previously valid desired document remains valid with the same portable
  meaning, and the added properties are simply absent from an older spec;
- a **breaking major** MUST have its own codec, because a removed, retyped, or
  re-meant property cannot be encoded or decoded by the other definition's
  declarations.

A client that holds no codec for an identity recorded in its state MUST fail
closed, naming that identity and the identities it does carry. It MUST NOT read,
update, or delete the resource under a different exact FormRef: substituting one
reinterprets state written against one contract as another, and the substituted
query's `resource_not_found` is then indistinguishable from deletion. Removing an
exact FormRef from a client's supported set is therefore a compatibility change
in that client, governed by its own versioning — for the Terraform/OpenTofu
provider, by "Provider versions are independent" above, which already forbids
silently reinterpreting persisted state as a different FormRef within a major.

A group rename asks the same question of every member at once, and answers it
for none of them: the group string is inside the digested bytes, so renaming a
family re-identifies all of its Forms whether or not their contracts moved.
When `edge.forms.takoform.com/v1alpha1` became `/v1beta1`, three contracts
changed and twelve did not, and all fifteen got new `schemaDigest` values.
Which was which is recorded, derived rather than authored, in
[`../release/form-contract-continuity.json`](../release/form-contract-continuity.json)
— the answer a client holding a recorded FormRef actually needs.

### What a Form version change costs a Terraform resource type

A codec absorbs a definition change invisibly. A Terraform resource SCHEMA
cannot: there is exactly one `takoform_worker_version` schema in a provider
build, every resource of that type decodes through it, and a configuration
written against it is source a user maintains. The rule is therefore about the
schema rather than about the codec
([decision 0030](decisions/0030-a-form-line-moves-a-terraform-resource-type-may-not.md)).

The SAME Terraform resource type is kept when every existing attribute keeps
exactly its meaning and the change is one of

- adding an Optional attribute,
- adding a Computed attribute, a declared output included,
- relaxing validation, or
- adding an enum value that breaks neither an existing host nor existing state.

A NEW Terraform resource type is required for removing an attribute, changing an
attribute's type, making an attribute required, changing a declared output's
type, changing the Form's lifecycle role, changing the identity or the
replacement unit, or any other semantic break. A Form that breaks
`takoform_worker_version`'s schema becomes `takoform_worker_version_v2`, or a
different Form kind; both types then exist in one build, the old one serving the
state written under it through its own codec. Removing the old type is a
provider major under "Provider versions are independent" above.

Every v1beta1-lane resource carries a schema version and registers a state
upgrader for each earlier version, so a resource type can outlive a change to
its own persisted layout without minting a new type for it.

### Existing identities

An occupied FormRef MUST never be reused for different bytes. An existing kind
whose public version is already `1.x` or later MUST NOT be renumbered to `0.x`
or presented as Stable merely because its number is greater than zero.

The Forms and admission documents published before decision
[`0004`](decisions/0004-takoform-is-an-experimental-specification.md) were a
Legacy line, since withdrawn with its epoch
([decision 0042](decisions/0042-the-pre-beta-epochs-are-withdrawn.md)). Their
original version numbers and document fields remain intact in git history and
release tags; nothing reinterprets or renumbers them, and their identities are
never reused.

## Form and package identity

A Form's identity is its exact `FormRef`. The `schemaDigest` binds the canonical
Form Definition bytes. Two definitions with different canonical bytes are
different identities even if their display names match.

A package binds one exact FormRef to a closed byte inventory, fixtures,
metadata, provenance, and digest. Published package bytes MUST NOT be changed,
re-signed as a replacement, or served under an occupied identity.

Changing only package metadata or fixtures does not by itself change Form
maturity. Different closed package bytes produce a different package digest
and therefore a different current publication locator. They do not require a
new Form SemVer unless the Form contract itself changed.

## Host API group

The current Host API wire is `forms.takoform.com/v1beta1`, discovered at
`/.well-known/takoform/v1beta1` with API base
`/apis/forms.takoform.com/v1beta1`. It carries namespaced FormRef groups,
UID/generation/revision identity, long-running Operations, content-addressed
artifact upload, and Host Support Profiles
([decision 0035](decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)).

The three pre-Beta wires — `forms.takoform.com/v1alpha1` at the unversioned
`/.well-known/takoform`, `/v1alpha2` at `/.well-known/takoform/v1alpha2`, and
the `/v1alpha3` identities the Beta lane carried forward — are withdrawn
retired identities ([decision 0042](decisions/0042-the-pre-beta-epochs-are-withdrawn.md)).
Each lane has its own discovery path and API base; a Host
wire version never implies Form maturity. Breaking protocol changes require a
new Host API group identity, starting with `v1beta2` for a breaking Beta fix.

### A lane is minted for one of exactly two reasons

Two questions were being answered as one, and they are separate
([decision 0039](decisions/0039-a-lane-is-minted-for-one-of-two-reasons.md)):

- **What may cause a lane to move.** Only two things. The wire contract changed,
  or the lane ITSELF advanced a maturity channel on the evidence named below.
  A lane MUST NOT be minted because a Form Family moved, because Forms changed,
  or because a provider was released. That prohibition is the paragraph after
  this one, and it is what a generation move most easily violates: everything
  moves together and no reason is stated for the lane in particular.
- **What a graduation looks like when it does happen.** A new exact identity,
  never a relabelling of an occupied one. A served identity means what it meant
  when it was published
  ([decision 0037](decisions/0037-immutability-begins-at-stable.md)), and a lane
  published at a Beta channel asserted that channel; graduating it in place
  would make an address a reader already holds quietly mean something stronger.

Both halves are checked. A lane minted for a protocol change must have a wire
contract structurally different from every other protocol lane's, compared with
version words normalised so a rename cannot present itself as a contract. A lane
minted for a graduation must say which of the prerequisites below it satisfied,
because that is not provable from bytes. `forms.takoform.com/v1beta1` is
recorded as a graduation whose lane-specific evidence was never stated — it was
minted with the family channel move and carries v1alpha3's wire contract
unchanged ([decision 0038](decisions/0038-a-generation-move-is-measured-not-assumed.md)).
It is frozen into Registry-published provider `v2.1.1`; the record stands as
history rather than as a precedent.

The API group MUST NOT graduate based on a Form count, package publication,
provider major, historical admission, or one host's conformance report. A
future graduation decision requires, at minimum:

1. two independently operated hosts exercising the same lifecycle semantics;
2. a documented compatibility window with no breaking operation change;
3. end-to-end materialization of each optional surface the lane declares;
4. cross-publisher package installation and lifecycle evidence;
5. a real deprecation/removal exercise and production consumption of the
   revocation chain.

Takoform's lifecycle authority owns promotion; a provider release or host
milestone is not a maturity decision. Only Host API and Form contracts that
have satisfied the applicable qualification above MAY mint stable identities
and Form `1.0.0` lines; every other contract remains Beta or Experimental. A
Takosumi deployment may be one independent adopter, but a Takosumi product GA
is neither required nor sufficient evidence. Any graduation is a separate ADR
and public migration plan. Until then, the project and API MUST NOT be
described as stable or GA.

## Deprecation, Legacy, and revocation

Deprecation announces a migration contract; Legacy is the retained lifecycle
state after the current line is no longer recommended for new work. Neither
operation deletes public bytes.

Security revocation is separate, append-only, and described in
[`trust/`](trust/). It may block new creation, update, or activation while
retaining the referenced bytes for safe observation, deletion, recovery, or an
explicit operator evacuation path.
