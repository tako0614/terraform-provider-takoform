# Versioning and compatibility

Takoform versions protocol, Forms, packages, and the Terraform/OpenTofu
provider for different reasons. Their numbers MUST NOT be aligned to imply a
shared release or maturity level. Form maturity and the complete lifecycle are
defined in [`project-lifecycle.md`](project-lifecycle.md).

## Version axes

| Concern      | Identifier                                      | Meaning                                                                                    |
| ------------ | ----------------------------------------------- | ------------------------------------------------------------------------------------------ |
| Host API     | API group such as current `forms.takoform.com/v1alpha3` | Protocol envelope, discovery, and lifecycle compatibility                             |
| Form group   | DNS-like API group inside an exact `FormRef`    | Namespace boundary between Form Families and retained epochs; versioned per family          |
| Form         | SemVer inside an exact `FormRef`                | Compatibility of one portable desired-state contract within its group                      |
| Form Package | Exact package identity plus content digest      | Immutable distribution of one exact Form and its fixtures                                  |
| Interface / Binding contract | Exact ref plus schema digest       | Immutable operation-surface and typed-capability contracts referenced by Forms             |
| Provider     | Provider SemVer                                 | Terraform/OpenTofu protocol, typed surface, persisted state, and host-client compatibility |

Since [decision 0009](decisions/0009-form-families-and-namespaced-api-versions.md)
the FormRef group is a namespaced DNS-like identifier. Official families use
subdomains of `forms.takoform.com` (the first is
`edge.forms.takoform.com/v1alpha1`); third-party groups are valid FormRefs
under their own domains. `forms.takoform.com/v1alpha1` (Legacy) and
`forms.takoform.com/v1alpha2` (retained provider-v2 preview) are frozen
groups; new families never reuse them. Each family group versions
independently.

Host Support, Form Activation, and Service Offering records refer to exact
identities but are not version streams. Updating one of those facts MUST NOT
change a Form or provider version.

Current family packages use the `packages.forms.takoform.com/v1alpha4`
envelope, which carries namespaced FormRef groups; the
`packages.forms.takoform.com/v1alpha3` envelope remains the retained
provider-v2 candidate profile. Content-addressed packages have no independent
SemVer. Their exact package digest produces the publication artifact ID
`sha256-<hex>` and therefore the source path and tag. Existing v1alpha1
`packageVersion` values and the published content-addressed v1alpha2 package
profile remain immutable Legacy identities; tooling MUST preserve and verify
their paths, tags, signatures, and bytes without interpreting a package profile
as Form maturity. The content-addressed locator decision is recorded in
[`decisions/0005`](decisions/0005-current-form-packages-use-content-addressed-locators.md),
the v1alpha3 envelope required by the Form epoch reset is recorded in
[`decisions/0006`](decisions/0006-v1alpha2-restarts-form-lines.md), and the
family-carrying v1alpha4 envelope is minted by
[`decisions/0013`](decisions/0013-v1alpha3-lane-ships-in-provider-v2-1.md)
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

The provider compatibility decision is recorded in
[`decisions/0001-provider-v1-keeps-form-versions-independent.md`](decisions/0001-provider-v1-keeps-form-versions-independent.md).
Current provider release facts belong to [`../release/`](../release/index.md),
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

Every released `0.x` identity remains immutable. Experimental means the next
release may break according to this policy; it never permits overwriting the
current release.

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

### Existing identities

An occupied FormRef MUST never be reused for different bytes. An existing kind
whose public version is already `1.x` or later MUST NOT be renumbered to `0.x`
or presented as Stable merely because its number is greater than zero.

The Forms and admission documents published before decision
[`0004`](decisions/0004-takoform-is-an-experimental-specification.md) are a
Legacy line. Their original version numbers and document fields remain intact;
current lifecycle projections describe them as Legacy without changing the
published definitions.

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

The current Host API wire is `forms.takoform.com/v1alpha3`, discovered at
`/.well-known/takoform/v1alpha3` with API base
`/apis/forms.takoform.com/v1alpha3`. It carries namespaced FormRef groups,
UID/generation/revision identity, long-running Operations, content-addressed
artifact upload, and Host Support Profiles
([decisions 0011–0013](decisions/0011-resource-identity-generation-and-revision.md)).

The `forms.takoform.com/v1alpha2` wire remains the retained provider-v2 lane
at `/.well-known/takoform/v1alpha2`, and the frozen
`forms.takoform.com/v1alpha1` Host API and Form epoch remain a closed
provider-v1 compatibility lane at `/.well-known/takoform`. Each lane has its
own discovery path and API base; a Host wire version never implies Form
maturity. Breaking protocol changes require a new Host API group identity.

The API group MUST NOT graduate based on a Form count, package publication,
provider major, historical admission, or one host's conformance report. A
future graduation decision requires, at minimum:

1. two independently operated hosts exercising the same lifecycle semantics;
2. a documented compatibility window with no breaking operation change;
3. end-to-end materialization of each retained optional interface surface;
4. cross-publisher package installation and lifecycle evidence;
5. a real deprecation/removal exercise and production consumption of the
   revocation chain.

Any graduation is a separate ADR and public migration plan. Until then, the
project and API MUST NOT be described as stable.

## Deprecation, Legacy, and revocation

Deprecation announces a migration contract; Legacy is the retained lifecycle
state after the current line is no longer recommended for new work. Neither
operation deletes public bytes.

Security revocation is separate, append-only, and described in
[`trust/`](trust/). It may block new creation, update, or activation while
retaining the referenced bytes for safe observation, deletion, recovery, or an
explicit operator evacuation path.
