# Host API v1beta3

`forms.takoform.com/v1beta3` is the current Host API lane. It was minted for
protocol reasons ([decision 0039](../decisions/0039-a-lane-is-minted-for-one-of-two-reasons.md)),
and the reason is stated in
[decision 0048](../decisions/0048-the-protocol-states-mechanisms-not-forms.md):
**this document states MECHANISMS and names no Form kind of any family.**

Its predecessor named Edge Form kinds sixty-six times, in sections about one
family's cardinality rules, its assigned address, and its migration ledger —
which is why a family could not gain a Form without the protocol changing. Here
a Definition declares which mechanism it instantiates and a host reads that,
so a family adds a Form, or a rule of a shape this lane already knows, without
this document moving. A rule of a NEW shape still needs a reviewed protocol
change, because a new shape is a new thing every host must be able to enforce.

The wire is otherwise its predecessor's, which is what makes the mechanisms the
whole of the difference. The v1beta2 and v1beta1 lanes remain served
history at its own discovery path; nothing here restates what its documents
meant.

The lane carries namespaced FormRef groups, UID/generation/revision resource
identity, long-running Operations, content-addressed artifact upload, and
Host Support Profiles. This document is normatively self-contained: every
error code it may answer, every fence it may demand, and every query key it
may read is defined here or in the machine tables it names.

The wire schema is
[`../schemas/host-api-wire-v1beta2.schema.json`](/schemas/v1beta2/host-api-wire.schema.json);
the machine operation table is [`operations-v1beta2.json`](operations-v1beta2.json).

## Discovery

`GET /.well-known/takoform/v1beta3` returns a document validating against
[`../schemas/host-discovery-v1beta2.schema.json`](/schemas/v1beta2/host-discovery.schema.json):
`api_versions` is exactly `["forms.takoform.com/v1beta3"]`; the features
`service_forms`, `exact_form_ref`, `optimistic_concurrency`,
`idempotent_lifecycle`, `operations`, `artifact_upload`, and
`support_profiles` are all required and true; `endpoints.api` is same-origin
with path `/apis/forms.takoform.com/v1beta3`. Each lane has its own
discovery path; a v1alpha2 client can never select this lane accidentally.

Every advertised endpoint path is compared in its escaped form and MUST carry
no percent-encoding at all. A client rejects the discovery document otherwise
([decision 0018](../decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)):
an escaped path describes a shape this lane does not have, and comparing the
decoded path instead would let `%2Fv1beta3` pass as `/v1beta3`.

A client negotiates each lane independently, under a deadline of its own that
is short and separate from its resource-operation deadline. Nothing about one
lane's outcome may make another lane's resources unusable, and each resource
type reports its own lane's negotiation error.

## Shared wire rules

These rules predate this lane and are restated here so the lane's contract is
closed under this document alone.

**Same-origin endpoints.** JSON Schema cannot compare URL origins. After
schema validation, an implementation MUST parse and normalize every advertised
URL and MUST reject any endpoint whose scheme, host, and effective port do not
equal those of `endpoints.api`. Userinfo, query, and fragment components are
forbidden. Plain HTTP is allowed only for a loopback development origin. A
provider sends bearer credentials only to same-origin advertised URLs.

**Space identity.** `SpaceID` is the portable scope identity used by Resource
bodies, query parameters, idempotency scope, provider configuration, and
provider state. It is an opaque, case-sensitive string, not a `PatternName`. A
valid `SpaceID` is valid UTF-8 containing 1 through 255 Unicode code points;
does not start or end with a Unicode `White_Space` code point or `U+FEFF`;
contains no Unicode `Cc` control code point (which includes `U+007F`); and
contains no `/`. The normative
code-point sets are encoded explicitly in
[`../schemas/host-api-wire-v1beta2.schema.json`](/schemas/v1beta2/host-api-wire.schema.json)
under `$defs.spaceId`. Embedded non-control whitespace is data and is valid.
Every participant MUST preserve the exact decoded value: no trimming, Unicode
normalization, or case folding. URL percent-encoding MAY represent that value
on the wire, but decoding MUST recover the same code-point sequence. Import
uses either `NAME` with a configured default or `SPACE/NAME`; forbidding `/`
in `SpaceID` makes that split unambiguous.

**I-JSON raw-first decoding.** Every Host API request and response document
MUST be UTF-8 I-JSON. Before typed decoding, an implementation MUST validate
the complete raw document, including nested objects, and reject invalid UTF-8,
duplicate object member names, invalid Unicode, non-I-JSON numbers, excessive
nesting, and trailing data. A request with a duplicate member or an unknown
envelope/metadata field fails as `invalid_argument` / HTTP 400 before
mutation. The wire schema's `x-takoform-equals` annotations (the envelope
`apiVersion` and `kind` mirroring `/form/formRef/*`) are enforced at the same
typed-validation stage, before any fence: a mirrored member that disagrees is
`invalid_argument` naming both pointers. A response with duplicate members is protocol-invalid before any
typed or stable-error semantics are applied. The same raw-first rule applies
when decoding this repository's digest-pinned conformance contract; a
duplicate member is verification failure, never last-member-wins.

**Idempotency-key namespacing and replay fingerprinting.** Every MUTATING
fenced request carries a deterministic `Idempotency-Key` (observe, the fenced
non-mutation, carries none), and a retry reuses
the same key. The key is not a bearer capability and never a global cache
address: a host MUST namespace a replay record by its authenticated
tenant/security domain, authenticated principal, exact Space, and
`Idempotency-Key`, and MUST authenticate and authorize the current request
before consulting or returning that record. A same-key request from another
principal or tenant is an independent request, and a principal whose
credential or permission has since been revoked receives the current
`unauthenticated`, `permission_denied`, or `policy_denied` result rather than
a cached success. The replay fingerprint also binds the method, exact request
target, generation preconditions, and exact request body bytes; reusing a key
for different request bytes fails as `invalid_argument`. A key is 1 through
255 visible ASCII characters (0x21–0x7E); anything else is
`invalid_argument`. A same-key, same-fingerprint request that arrives while
the first is STILL EXECUTING is not replayed and not run twice: the host
answers `resource_busy` (retryable) until the first execution records its
outcome. A replay record survives at least as long as the incarnation it
reports and, for an accepted 202, at least as long as its operation record;
a host MAY expire older records and says nothing when it does — an expired
key simply executes as new.

## Path shape

A namespaced Form group travels as **two ordinary path segments** — the group
name, then the group version — wherever a URL template names a group:

```
{api}/resources/{formGroup}/{formVersion}/{kind}/{name}
{api}/form-definitions/{formGroup}/{formVersion}/{kind}
{api}/support/forms/{formGroup}/{formVersion}/{kind}/{definitionVersion}
```

So `edge.forms.takoform.com/v1beta3` travels as
`edge.forms.takoform.com/v1beta3`. **No path segment ever percent-encodes a
slash.** Proxies, gateways, and web frameworks disagree about whether `%2F`
inside a path segment is passed through, decoded, rejected, or normalized, so a
lane that required it could not be placed behind ordinary infrastructure at all
([decision 0018](../decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)).
A host rejoins the two segments into the exact apiVersion; the FormRef
`apiVersion` string is unchanged everywhere else — request bodies, responses,
and the `group` query key still carry `edge.forms.takoform.com/v1beta3`
verbatim. This is the required conformance check
`namespaced-group-travels-as-two-path-segments`.

## Addressing a resource

The lane's resources are addressed by five facts: the authenticated tenant,
the Space, and the exact FormRef's group, kind, and name. Three of them travel
as path segments (`{formGroup}/{formVersion}/{kind}/{name}`); the remaining
exactness travels differently by direction, and the split is normative:

- **Requests with a body** (`validate`, `prepare`, `apply`, `import`) carry
  the Space as `metadata.space` and the whole exact FormRef as `form.formRef`
  inside the document. Nothing exact travels in the query.
- **Requests without a body** (`read`, `observe`, `delete`, and the
  form-definition read) carry `space`, `definitionVersion`, and
  `schemaDigest` as query parameters, exactly as
  [`operations-v1beta2.json`](operations-v1beta2.json) lists per operation.
  A listed required parameter that is absent or malformed is
  `invalid_argument`; a well-formed exact query naming a ref the resource is
  not bound to is `resource_not_found`, indistinguishably from a name nobody
  created. `observe` carries an empty body; only its fence header and query
  address it.
- `definitionVersion` values never carry SemVer build metadata (an installed
  definition version is forbidden to have any: the digest already
  distinguishes builds), and a `+` that does appear in a query value MUST
  travel percent-encoded as `%2B`; a host decodes the query before matching.

The machine table's `exactFormQuery` flag marks every operation addressed
this way, and `exactFormQueryMeaning` in the same file is its definition.

## Resource identity

A portable resource name matches `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`: 1–63
lowercase characters, starting with a letter, never ending in a hyphen — so
every name is also a valid DNS label, and the hostname grammars of this
family never diverge from the names beneath them. Names are stable wire
identities; nobody trims, folds, or normalizes one.

- `metadata.name` is the only name; Form desired schemas do not carry a
  `name` field.
- `metadata.uid` is host-issued and immutable. Delete followed by re-create
  of the same name yields a new UID. A client MAY fence any mutation with
  `expectedUid`; mismatch fails `uid_mismatch` (409).
- `metadata.generation` increments only when the portable desired spec
  changes. Desired-state mutations fence on the expected generation
  (`Takoform-Expected-Generation` header or `expectedGeneration` body field);
  a stale fence fails `generation_conflict` (412).
- `metadata.revision` increments whenever the representation — including
  status, conditions, and outputs — changes. The strong ETag is the quoted
  revision; `If-Match` fences on it and a stale value fails
  `revision_conflict` (412).
- **A `delete` fences on the expected generation, and MUST NOT be required to
  fence on the revision.** A delete withdraws desired state, so it is a
  desired-state mutation and carries the same fence every other one carries: the
  `Takoform-Expected-Generation` header, REQUIRED, with a stale value failing
  `generation_conflict` (412) and an absent one `invalid_argument` (400). A
  client MAY additionally send `If-Match`, and a host that is given one MUST
  honor it and answer `revision_conflict` (412) when the representation moved —
  but a host MUST NOT require it, and MUST NOT refuse an otherwise valid delete
  because the revision moved. The revision moves for reasons the deleting client
  did not cause: a host-side status change, and the derived rendering below.
  Removing an aggregate means removing its dependents first, so a client's own
  teardown moves the revision of the resource it is about to delete next, after
  the plan that computed the teardown read it — and requiring the representation
  fence would refuse a destroy on a change the destroy itself made, with no
  repair available, because the next dependent moves it again. The generation
  moves only when a client changes a desired spec, which is the only "it changed
  under me" a deleter can act on; the incarnation question is `expectedUid`'s,
  and an accepted delete resolves through the recorded uid before it evaluates
  any fence. These are the required conformance checks `delete-generation-fence`
  and `delete-fence-survives-derived-rendering`, and they are decided by
  [decision 0011](../decisions/0011-resource-identity-generation-and-revision.md).
- A host that renders any part of a representation from OTHER resources MUST
  advance that resource's `metadata.revision` when the rendering changes, and
  MUST NOT move its `metadata.generation`: no desired spec changed. This lane
  has two such renderings — relation drift and derived readiness — so creating,
  re-weighting, or deleting a resource whose selection another resource's
  readiness follows moves that resource's revision, and a target that is
  deleted or replaced moves the revision of every source pinned to it. Serving the changed
  representation under the old revision would make the ETag a validator that
  reports "unchanged" about a change, and `If-Match` a fence on a
  representation the client never saw. After an accepted mutation a host
  recomputes the rendering of every resource that mutation can affect and
  advances only those that actually changed, so an idempotent re-apply moves
  nothing anywhere. This is a required conformance check
  (`dependent-revision-advances-with-rendering`), and it is decided by
  [decision 0016](../decisions/0016-the-worker-aggregate-has-one-active-deployment.md).
  It is also exactly why a delete does not fence on the revision, above: the
  rendering a teardown moves is the rendering of what the teardown deletes next.
- `status.observedGeneration` names the desired generation the status
  reflects; `status.conditions` uses the closed types `Ready`, `Reconciling`,
  `Degraded`, `Drifted`, `Blocked`, `Deleting` with closed portable reasons
  and an optional non-portable `hostReason`. The condition model is exact:
  `Ready` is present in EVERY status; every other type appears only while its
  state holds; a type appears at most once per status; and the valid
  type-reason pairs are closed —

  | Type | Valid reasons | Present when |
  | --- | --- | --- |
  | `Ready` | `Available`, `Provisioning`, `UnsupportedCapability`, `ExternalChange`, `DependencyMissing` | always; status True/False/Unknown says whether the resource serves |
  | `Reconciling` | `Provisioning` | the host is actively converging this resource toward its desired generation |
  | `Degraded` | `ExternalChange`, `DependencyMissing` | the resource serves, worse than desired |
  | `Drifted` | `ExternalChange` | observed backend state departed from what the host materialized |
  | `Blocked` | `UnsupportedCapability`, `DependencyMissing` | convergence cannot proceed without an outside change |
  | `Deleting` | `Provisioning` | an accepted delete has not reached its terminal state |

  A reason outside its type's row, a missing `Ready`, or a duplicated type is
  protocol-invalid. A client surfaces the WHOLE
  condition list, not a boolean derived from it: the reason is what says why a
  resource is not ready, and a client that discards it forces its operator to
  leave the tool to find out
  ([decision 0018](../decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)).
  Conditions are host-rendered state that changes without any desired spec
  changing, so a client MUST NOT carry a previous value forward as if it were
  still true.
- **A replay record does not outlive the incarnation it reports.** A host MUST
  NOT answer a request from a recorded idempotency replay once the incarnation
  that recording reports as LIVE no longer exists; such a record is RETIRED and
  the request is executed as the new request it is. A record is bound to the
  `metadata.uid` its recorded answer carries — a create, an update, an adoption,
  and an observe all report one. A record whose answer reports no live
  incarnation is bound to nothing and MUST keep replaying: that is a successful
  delete, whose `204` reports the incarnation gone, and every refusal. An
  accepted `202` is bound to its Operation and follows that operation's outcome.
  Retirement is by INCARNATION, never by name: a later resource taking the same
  name retires nothing, and a host MAY still expire records by age or capacity.
  Both halves matter. Without retirement a `destroy` followed by an `apply` of an
  unchanged configuration is answered the old `201` forever and never converges,
  because a create's prepare binds the create markers so the re-create derives a
  byte-identical key and fingerprint; retire a delete's own record and the
  retried delete of a lost response is EXECUTED against whichever incarnation
  holds the name now. These are the required conformance checks
  `apply-idempotency-replay` and
  `replay-record-retires-with-its-incarnation`, and they are decided by
  [decision 0011](../decisions/0011-resource-identity-generation-and-revision.md).
- The Form semantic identity is the exact FormRef. The package digest used
  at installation is audit evidence (`form.packageDigest`, optional in
  responses); it never enters queries, fences, or state identity, and a host
  that installed the same FormRef from a different legitimate package MUST
  read and delete the same resources.
- An exact FormRef query is answered about the WHOLE identity or not at all. A
  host MUST fail `form_unknown` (404) when the `definitionVersion` or
  `schemaDigest` it was given names a definition it does not have, on
  `{api}/forms`, on `{api}/form-definitions/…`, and on every resource route —
  it MUST NOT fall back to matching the group and kind. A client dispatches on
  the exact FormRef recorded in its state precisely so that a resource created
  under an earlier definition version stays addressable as itself
  ([decision 0017](../decisions/0017-provider-state-survives-form-evolution-and-interruption.md)),
  and a host that matched by kind would answer every such request about a
  different contract, successfully, with nothing downstream able to detect it.
  This is the required conformance check
  `exact-form-ref-fails-closed-on-unknown-definition`.

### An adoption claims the native identity it names

The rules of this section are decided by
[decision 0011](../decisions/0011-resource-identity-generation-and-revision.md).

`import` carries a `nativeId` — compared byte-exact, with no trimming, case
folding, or Unicode normalization anywhere in the claim index — the host's
own name for a backend object that
already exists. It is a CLAIM, not a hint.

- **A host RECORDS the `nativeId` of every adoption on the resource it adopts,
  and holds it exclusively within the caller's tenant.** At most one live
  resource of one tenant holds one native identity. An adoption naming an
  identity another live resource of that tenant already holds fails
  `import_conflict` (409) before any mutation and stores nothing.
- **A recorded claim is immutable.** An `import` addressing an EXISTING resource
  of the caller under a different `nativeId` fails `import_conflict` (409)
  before any mutation. Re-importing that resource under the identity it already
  holds is not a conflict: it is the ordinary fenced import and answers the same
  incarnation. A resource this host created holds no claim, so the FIRST
  `import` naming it records one rather than changing one — and that import is
  the ordinary one, `terraform import` onto an address a configuration already
  manages.
- **The claim is released with its holder, and by nothing else.** Once the
  resource holding an identity is deleted, that identity is adoptable again —
  otherwise a `destroy` would leave a backend object permanently unimportable.
  An ordinary `apply` on the holder withdraws nothing: a resource that survives
  an update keeps the object it was adopted onto.
- **The claim spans every space of one tenant, and every Form kind, and stops at
  the tenant.** Spaces partition one tenant's resources and a backend object does
  not partition with them, so two spaces adopting one object is the same
  duplication as two resources in one space. Neither is an object partitioned by
  the Form that adopted it: it has one identity, so two resources of one tenant
  may not both name it whatever their kinds, and a host keying the claim by
  `(tenant, kind, nativeId)` is holding a narrower claim than this one.
  It stops at the tenant for the reason every refusal in
  [Tenant isolation](#tenant-isolation) is shaped the way it is: a host-wide
  claim would answer "somebody already manages that" to a caller who cannot see
  the holder, which is a membership oracle over every identifier a stranger can
  guess. Whose account an object lives in is authority this lane does not model.
  Inside each tenant the claim binds in full, for every tenant.

These are the required conformance checks `import-claims-its-native-identity`
and `import-records-its-native-identity`, with the tenant edge driven by
`resource-import-is-tenant-isolated`. They exist because every other thing this
lane asks of `import` — a minted uid, generation `1`, the full validation
gauntlet, the claim scan of [decision
0026](../decisions/0026-attachment-claims-are-canonical-and-acyclic.md) — a plain
create also satisfies. A host whose `/import` created would pass the rest of the
corpus while `terraform import` against it minted a NEW backend object and
orphaned the one being adopted. Each is measured at the scope written above and
not at a convenient narrowing of it: the rival that may not adopt is a different
Form kind from the holder, the release is measured by that other kind, and the
first claim is driven onto a resource the host created as well as onto one
adoption brought into being.

**The `nativeId` never appears in a response, and no rule above asks a portable
author to know one.** A native identifier is host detail, outside the Form
contract ([portability boundary](../portability-boundary.md)), and the published
wire documents are closed
([decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)),
so what is observable is not the identifier but what a host holding it can no
longer do. What this lane therefore does NOT prove is that the object named
existed before the call, or that the host did not also mint a fresh one beside
it: no portable surface reports either, and a black-box runner cannot name an
object it knows a host already has without depending on that host's identifier
format. Publication blocker **V3-014** records that remainder.

### What the availability booleans mean

The `/forms` answer's members are truth-conditioned, not vibes:

- `definitionKnown` — the host holds the exact Definition bytes the
  `schemaDigest` names and can serve them from the form-definition route.
- `installed` — code compiled against that exact contract exists in this
  host. Implies `definitionKnown`.
- `executable` — that code can actually run here now (its runtime present,
  its own dependencies loadable). Implies `installed`.
- `activated` — the host's operator policy allows NEW resources of this exact
  Form for this tenant. Never implied by the previous three; deactivation
  refuses creates while every existing resource keeps its whole lifecycle.
- `availableToPrincipal` — the requesting principal may address this exact
  Form at all; false hides nothing else in the row.

`form_not_installed` is `definitionKnown && !installed` at a lifecycle route;
`form_unavailable` is `installed` with transiently absent capacity. There is
no `deprecated` member in this lane: a deprecation surface arrives with the
deprecation exercise, as its own reviewed change, with a source of truth.

### The installed set is keyed by the exact identity

The rules of this section are decided by
[decision 0022](../decisions/0022-relations-pin-the-target-contract.md).

- A host's installed set is keyed by the whole
  `{apiVersion, kind, definitionVersion, schemaDigest}` tuple, never by group and
  kind. Two definition versions of one group and kind MUST be installable at the
  same time and MUST answer independently on `{api}/forms`, on
  `{api}/form-definitions/…`, and on `{api}/support/forms/…`. A host MUST NOT
  install two Definitions that agree on group, kind, and `definitionVersion` and
  differ on `schemaDigest`: a definition version names one set of bytes, and the
  support path resolves an exact identity from that version alone. A
  `definitionVersion` from one contract combined with a `schemaDigest` from
  another names no installed Definition and fails `form_unknown` (404) like any
  other unknown identity. This is the required conformance check
  `two-definition-versions-answer-independently`.
- **The Form Definition surface answers with the Definition's own bytes.** For
  an exact FormRef, `desiredSchema` MUST be the `desiredSchema` of the installed
  Definition whose canonical bytes that `schemaDigest` addresses — every
  declared default, bound, pattern, and enum, unchanged. Echoing the requested
  identity is not enough: a host that answered a pinned FormRef with a schema of
  its own would hand every client a different portable default, and a client
  that materialized it would compute a `specDigest` no other client computes,
  with `prepare` refusing what the author actually wrote and nothing on either
  side able to say why. A client CANNOT re-derive the served document from the
  pinned digest — the digest covers the whole Definition and this surface serves
  a subset of it — so the conformance corpus pins the desired schema of every
  Form it drives as bytes, and `form-definition-exact` compares what a host
  serves against them. The runner materializes its probe specs from the PINNED
  schema, never from the served one; measuring a host against defaults that host
  supplied measures nothing.
- A resource RECORDS the exact FormRef it was created under. That ref is written
  at create, carried forward unchanged by every update and import, and is the
  only identity the resource is answered about. A `read`, `observe`, `apply`,
  `import`, or `delete` naming any other exact ref addresses no resource and
  fails `resource_not_found` (404) — the Form may well be installed; what is
  absent is a resource of that name under that contract. A response MUST NOT
  rewrite an older resource's recorded ref to a newer one. This is the required
  conformance check `resource-answers-only-under-its-recorded-form-ref`.
- A resource NAME stays unique within one tenant, space, group, and kind. The
  definition version decides what a request is answered about, never where a
  resource lives: a reference is `{apiVersion, kind, name}` and carries no
  definition version, so two same-named resources of one kind under different
  contracts would leave every reference to that name unresolvable. A create
  therefore still conflicts with a name taken under another contract of the same
  kind — and never with a name taken by another TENANT, which is a different
  address entirely ([Tenant isolation](#tenant-isolation)).

## Tenant isolation

The rules of this section are decided by
[decision 0028](../decisions/0028-the-resource-plane-is-tenant-isolated.md).

A host's **internal resource address MUST include the tenant, the space, the
`apiVersion`, the `kind`, and the `name`.** The tenant is the AUTHENTICATED
tenant of the request: no path segment, query key, reference, or `metadata`
member names one, so it comes from the credential and from nowhere else. It is
recorded when a resource is created and carried forward by every update and
import, exactly like the recorded exact FormRef.

A request that carries no credential, or one naming nobody, therefore has no
tenant and MUST be refused `unauthenticated` (401) — never
`permission_denied` (403), because the caller has not been identified and there
is nothing yet to deny. This is the required conformance check
`unauthenticated-request-refused`, driven with an ABSENT `Authorization` header
and with a well-formed bearer credential naming nobody, on a read surface and on
a mutating one. Everything below is downstream of it: a host whose credential
lookup fails open has picked a tenant the caller never named, and every rule in
this section is then about a boundary that is not there.

Everything else follows from the address.

- Two tenants MAY create one `{space, apiVersion, kind, name}`. They get two
  resources with two host-issued uids, two generations, and two revisions.
  Neither create conflicts with the other, because a name is taken **within one
  tenant** — a host that let one tenant's name choice deny another's would make
  the whole name space a shared, first-come resource and a membership oracle
  besides.
- Every surface that takes a resource name addresses the caller's own tenant:
  `read`, `observe`, `prepare`, `apply`, `import`, and `delete`. A request
  naming another tenant's resource is answered exactly as one naming a resource
  that was never created. Where that answer is a refusal it is
  `resource_not_found` (404), and it MUST be **indistinguishable** from the
  refusal for a name nobody created — the same code, the same status, and a
  message that discloses nothing about the other tenant. `permission_denied`
  (403) is the wrong answer and is forbidden here: it would confirm that a
  resource of that name exists somewhere on the host, which is the fact the
  boundary exists to withhold, to any caller who can guess a name.
- **"Its own tenant" is a permission as well as a limit.** Every tenant MUST be
  able to create, read, observe, update, import, and delete resources in its own
  plane, and a resource MUST stay operable by its holder after a caller from
  another tenant has been refused. A host that is create-and-read-only outside
  one privileged tenant, and a host that quarantines a record a stranger reached
  for, both refuse everything the paragraph above requires them to refuse — and
  both have taken a resource from the tenant that owns it. Required conformance
  checks: `each-tenant-mutates-its-own-plane`, and the holder's own leg of
  `resource-update-is-tenant-isolated` and
  `resource-delete-is-tenant-isolated`.
- `import` is the one surface whose absent answer is not a refusal, so the rule
  is stated for it exactly. An adoption under `If-None-Match: *` of a name the
  caller does not hold MINTS a resource at generation 1, and a name held only by
  another tenant is not held by the caller — so that adoption MUST succeed, and
  MUST answer what the same adoption of a name nobody holds anywhere answers:
  the same status, the same ETag, and a document differing in nothing but the
  host-issued uid and the name. An adoption carrying an update generation fence
  names an existing resource instead, so against a name only another tenant
  holds it fails `resource_not_found` (404) like any other absent target. A host
  that refused the create-intent adoption with `generation_conflict` would be
  answering "that name is taken" about a tenant the caller cannot see; one that
  took the update path would write the caller's desired state over a stranger's
  live resource, which — followed by a delete — destroys it through nothing but
  a name.
- `validate` carries a name and resolves none: it answers diagnostics about the
  document it was handed and reads no stored resource, so it has no
  tenant-dependent answer to give.
- **Relations resolve only within the same tenant, and the stored pin is the uid
  they resolved to.** A reference is
  `{apiVersion, kind, name}` and carries neither a tenant nor a space, so it is
  resolved inside the referring resource's own scope. A name that only another
  tenant holds is an absent target and fails `resource_not_found` (404) like any
  other. Resolving correctly and PINNING the other tenant's uid is the same
  defect arriving one step later, and it is the step everything downstream reads:
  deletion protection, drift, and every Worker aggregate rule are computed from
  the pin, so a foreign pin protects one tenant's resource from its own owner and
  makes another tenant's source follow a resource its author never named. A host
  that resolved a reference across tenants would bind one tenant's
  desired state to another tenant's resource and pin its uid into stored state,
  which is the substitution
  [decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md)
  closes for incarnations, committed across a security boundary rather than a
  naming one. Deletion protection, drift, and every Worker aggregate rule read
  those pinned relations, so all of them stop at the tenant too.
- A `prepareDigest` is minted for one tenant and is spendable by that tenant
  alone. An apply presenting a review minted in another tenant fails
  `invalid_argument` (400) before any mutation — the ordinary prepare-binding
  refusal, because the resource being addressed is the caller's own and what is
  untrue is the review. A create binding pins no uid and no generation, so
  without the tenant two tenants preparing one name in one space would hold the
  same digest, and a review — a value clients log, print in plans, and pass
  between processes — would be a bearer token over another tenant's plane. The
  boundary is the TENANT and not the principal: a second principal of the minting
  tenant MUST be able to spend the same review, because planning under one
  identity and applying under another is ordinary operation and a host enforcing
  the rule more tightly than it is written is a host two deployments differ on.
- An `Idempotency-Key` names one operation **of one tenant**. The same key from
  two tenants is two operations, and each tenant replays only its own recorded
  response. Answering a second tenant with the first tenant's recorded success
  would hand it another tenant's uid, generation, and ETag while performing no
  mutation at all — and so would answering it a fresh-looking `201` that stores
  nothing, which is why the check reads the second tenant's resource back.
- Anything a host renders from OTHER resources — the relation drift and Worker
  readiness of [Resource identity](#resource-identity) — is computed from, and
  advances the revision of, resources of the mutating tenant only. That pass
  writes, so a host-wide one would move another tenant's ETag.

There is **one deliberate exception**, and it is the one
[decision 0026](../decisions/0026-attachment-claims-are-canonical-and-acyclic.md)
already decided: a claimed value is unique across
every space **of one tenant**, because spaces partition one tenant's resources
and a claimed namespace does not partition with them
([Claimed values](#claimed-values)).
That rule drops the space and keeps the tenant. It does not reach past the
tenant, and no rule in this lane does.

Ten required conformance checks measure this, all of them black box, all of
them driven with two configured tenants' credentials:
`resource-address-is-tenant-scoped`, `resource-read-is-tenant-isolated`,
`resource-observe-is-tenant-isolated`, `resource-update-is-tenant-isolated`,
`resource-import-is-tenant-isolated`, `resource-delete-is-tenant-isolated`,
`relation-resolution-is-tenant-scoped`, `prepare-is-tenant-scoped`,
`idempotency-is-tenant-scoped`, and `each-tenant-mutates-its-own-plane`. A
runner that cannot authenticate as two tenants
can measure none of them, which is why the lane's runner REQUIRES an
alternate-tenant credential alongside the same-tenant alternate principal rather
than treating it as optional.

Nine of the ten are enumerated by SURFACE and not by intent. Every route of
[Lifecycle](#lifecycle) that takes a resource name is listed against the check
that measures it, and the list is bound to the published route block, to the
reference host's router, and to the required-check list by tests, so a
name-addressed endpoint cannot be added to this lane without one. An
enumeration by intent is what left `observe` and `import` out of the first
version of this section: both take a name, one returns a whole representation
and one mutates, and a host that scoped `read`, `apply` and `delete` while
resolving either host-wide satisfied every check there was. The tenth is not a
surface but the permissive half of all of them, so it is listed against `PUT` and
`DELETE` beside the two boundary checks it completes.

Two facts are host obligations this lane does NOT prove, because a black-box
runner cannot observe them:

- that the tenant is part of the ADDRESS rather than a filter applied somewhere
  late. The checks measure the consequences, which is what a third-party host can
  be held to; a host that reaches the same answers by other means passes, and is
  conforming.
- that a host-wide derived-rendering pass would cross the tenant. Constructing
  the case needs one tenant's resource to render from another's, and relation
  resolution refuses to build it — so there is no request sequence that
  distinguishes a scoped pass from an unscoped one.

Publication blocker **V3-012** records this item; the reference host's own
obligations are proved by host-side tests in
`internal/portableconformancev3/tenant_isolation_test.go`.

## Lifecycle

Endpoints under `/apis/forms.takoform.com/v1beta3`, keyed by group and
kind so one kind name can exist in many groups. `{formGroup}/{formVersion}` is
the two-segment group of [Path shape](#path-shape):

```
GET    {api}/forms                       enumeration; six optional filter keys
GET    {api}/form-definitions/{formGroup}/{formVersion}/{kind}?space&definitionVersion&schemaDigest
POST   {api}/resources/validate         diagnostics only, no mutation, no digest
POST   {api}/resources/prepare          short-lived prepare digest for one exact spec
PUT    {api}/resources/{formGroup}/{formVersion}/{kind}/{name}    apply; carries review.prepareDigest
GET    {api}/resources/{formGroup}/{formVersion}/{kind}/{name}
POST   {api}/resources/{formGroup}/{formVersion}/{kind}/{name}/import
POST   {api}/resources/{formGroup}/{formVersion}/{kind}/{name}/observe
DELETE {api}/resources/{formGroup}/{formVersion}/{kind}/{name}
```

`GET {api}/forms` enumerates every installed exact Form the principal may
see. Each of its six query keys (`group`, `version`, `kind`,
`definitionVersion`, `schemaDigest`, `space`) is optional and narrows the
answer; a malformed value is `invalid_argument`; all six present is the
exact-availability probe, whose answer carries zero entries or one. The
response never blends tenants and never lists a Form the principal could not
address ([Tenant isolation](#tenant-isolation)).

`observe` is the lane's only fenced read-only re-observation. There is no
`refresh` operation: v1alpha2 carried both under one contract, which meant two
spellings of one behavior and therefore two ways for hosts to differ. A
v1beta2 Form never declares the `refresh` capability and a v1beta2 host
serves no `/refresh` route.

`validate` reports diagnostics without mutating and without minting a
digest; a client MUST NOT describe provider apply-time preparation as
"reviewed in plan". `prepare` binds the exact spec, identity, and fences to
a `prepareDigest`; substitution after prepare fails `invalid_argument`
before mutation. A prepare digest is valid at least while the generation it
fenced is current: a host MUST NOT expire it earlier by clock, restart, or
capacity, and MUST refuse it once that generation moves. Expiry and
substitution answer the same `invalid_argument` with a message that says
which — a client repairs both the same way, by preparing again.

`delete` carries an empty body, an `Idempotency-Key`, and the expected
generation, which is its only required precondition ([Resource
identity](#resource-identity)). `observe` carries an empty body and NO
`Idempotency-Key`: it mutates nothing, so there is no operation to replay,
and a recorded answer would only be a stale representation the ETag rules
exist to prevent. Every observe executes.

### The fence matrix

One table governs how a generation fence travels, and
[`operations-v1beta2.json`](operations-v1beta2.json) carries it as data
(`fences`):

| Operation | Transport | Absent | Stale |
| --- | --- | --- | --- |
| update (apply on an existing resource) | `Takoform-Expected-Generation` header, or `expectedGeneration` in the body | `invalid_argument` | `generation_conflict` |
| prepare on an existing resource | header only — the prepare request shape carries no fence member | `invalid_argument` | `generation_conflict`; a fence naming an absent resource is `resource_not_found` |
| observe | header only | `invalid_argument` | `generation_conflict` |
| delete | header only | `invalid_argument` | `generation_conflict` |
| create (apply minting a resource) | `If-None-Match: *` | `invalid_argument` | an existing resource answers `generation_conflict` |

When an apply carries both the header and the body field they MUST be equal;
disagreement is `invalid_argument` before either is evaluated. A create
carrying `If-None-Match` together with a generation fence is
`invalid_argument`: the two assert contradictory beliefs about existence.

Role rules are wire-enforced: an update to a `revision`-role resource fails
`invalid_argument`; deleting a resource any live relation references fails
`dependency_in_use` (409); deleting a `deployment`-role resource any live
dependent needs fails the same way
([decision 0016](../decisions/0016-the-worker-aggregate-has-one-active-deployment.md)).
The `policy` role has no wire rules in this lane: the first policy-role Form
brings them, and until then no code claims a policy surface exists.

## Cross-resource relations

The rules of this section are decided by
[decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md).

A **relation** is one reference from a resource's desired spec to another
resource. Its wire shape is the closed three-member object

```json
{ "apiVersion": "example.forms.takoform.com/v1", "kind": "ExampleStore", "name": "cache" }
```

where `apiVersion` and `kind` are `const` in the referring Form's
`desiredSchema` and `name` follows the portable resource-name grammar. Both
constants are required. A reference carrying only `{kind, name}` cannot address
two Form Families at once and forces a host to guess which installed Form a bare
kind means; a group-qualified reference makes the target Form exact.

Relations are **derived from the Form Definition's `desiredSchema`, never
declared**. Every reference already states its target group and kind as schema
constants, so a separate relation list on the Definition would be a second
source of truth for facts the schema already carries — and the published Form
Definition schema admits no such member
([decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)).
A host derives the relation set from the same `desiredSchema` it serves: every
closed object that requires AT LEAST `apiVersion`, `kind`, and `name` with the
first two `const` is a relation, identified by its JSON Pointer with `*`
standing for an array element (`/owner`, `/versions/*/revision`,
`/bindings/*/resource`). A resource that selects a named export of its target
names that export as its OWN top-level property, beside the reference rather
than inside it: the export selects WITHIN the pinned target and never weakens
the pin, and a sibling property needs no annotation to be found. A closed
reference-shaped object carrying members the reference itself does not declare
is a Definition defect a package verifier refuses, never a silently un-derived
relation. A binding-list property additionally carries the
`x-takoform-binding` annotation naming the Binding contract that governs the
references inside it; the exact digest-bound BindingRef is the Definition's own
`acceptedBindings` entry of that name.

### What a relation requires of its target

The rules of this section are decided by
[decision 0022](../decisions/0022-relations-pin-the-target-contract.md).

A group and a kind say WHICH resource a reference names. They say nothing about
what that resource must still satisfy, so a target whose Definition later moved
to an incompatible version would keep satisfying every reference to it. Every
reference-shaped node in a `desiredSchema` therefore carries exactly one target
contract, as an annotation on the reference itself — data the published Form
Definition schema already admits, exactly like `x-takoform-binding`
([decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)):

```json
"x-takoform-target-formrefs": [
  { "apiVersion": "example.forms.takoform.com/v1", "kind": "ExampleArtifact",
    "definitionVersion": "0.1.0", "schemaDigest": "sha256:..." }
]
```

when the relation depends on the target's exact desired contract, or

```json
"x-takoform-required-interface": {
  "apiVersion": "interfaces.takoform.com/v1alpha1",
  "name": "worker.runtime", "version": "1.0.0", "schemaDigest": "sha256:..."
}
```

when it depends only on an Interface the target provides. A reference carrying
both, or neither, is refused when relations are derived: a host that cannot say
what a relation requires cannot verify it.

Which one is correct is decided by the dependency, not by preference, and by
nothing else: `x-takoform-target-formrefs` states the requirement exactly when
the source — or the host acting for it — reads a member of the target's
desired spec or enforces a rule stated over the target Form itself;
`x-takoform-required-interface` states it exactly when the source needs only
behavior a contract fixes, which any Form providing that Interface would
serve. No list of which references use which lives here: each Definition's
own annotations are the enumeration, and a reader asking "why this one" reads
the criterion against the reference's semantics.

A binding-list reference MUST require exactly the Interface its Binding
Definition names as `targetInterface`. The Binding Definition stays the
authority and the annotation is its projection onto the reference, so the two
cannot become two sources of truth; a binding relation is verified against its
Binding Definition first, in the order below, and against the annotation second.

### Resolution and UID pinning

On `apply` and `import`, for every derived relation present in the materialized
spec, a host MUST, before any mutation
([decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md)):

- resolve `(space, apiVersion, kind, name)` to a stored resource. Absent fails
  `resource_not_found` (404) and the message MUST name the relation pointer.
  Cross-space references are unrepresentable — a reference carries no space.
- verify that the resolved resource's Form has exactly the referenced group and
  kind, failing `invalid_argument` (400) on mismatch. The resolved resource's
  own RECORDED exact ref names that Form; a well-formed spec cannot reach this,
  because the schema pins both constants, but a host that resolved by name alone
  can, and must refuse rather than bind the wrong Form.
- verify the target contract the reference annotates
  ([decision 0022](../decisions/0022-relations-pin-the-target-contract.md)): the
  target's recorded exact ref is one of the listed `x-takoform-target-formrefs`,
  or the target Form's Definition declares the annotated
  `x-takoform-required-interface` in `providedInterfaces` at exactly that
  digest. A target that satisfies neither fails `invalid_argument` (400) — the
  request is well formed and states something untrue about the resource it
  points at — and the message MUST name the relation pointer and what was
  required. These are the required conformance checks
  `relation-target-form-ref-verified` and `relation-target-interface-verified`.
- verify the binding contract when the relation is a binding (below).
- record the resolution as
  `{pointer, relation, targetAPIVersion, targetKind, targetName, targetUID, targetFormRef, bindingRef?}`
  alongside the resource, where `pointer` is the concrete instance pointer and
  `relation` is the derived pointer it came from.

A host MUST store the **target UID**, not only the name. A name is a label the
client chose and can reuse; the UID is the identity of one incarnation. It MUST
also store the **target's exact FormRef**: the UID pins which incarnation the
source is bound to, and the ref pins what contract that incarnation satisfies.
This is the required conformance check `relation-pin-records-target-form-ref`.

### Incarnation change

When a resource is read or observed and a stored relation's target now resolves
to a *different* UID, to a different exact FormRef, or to nothing, the source
reports `Ready=False` with

- `ExternalChange` — the target name resolves to a different incarnation, or the
  same incarnation under a different exact contract;
- `DependencyMissing` — the target no longer exists;

and a `hostReason` naming the relation pointer, both UIDs, and both exact
FormRefs. A host MUST NOT re-bind the relation automatically: re-resolving the
name would make a delete and re-create of the target invisible and silently
point the source at a resource its author never named. The source stays pinned
until it is re-applied, and re-reading it does not heal the condition. A moved
CONTRACT is reported through this same condition rather than a parallel one: it
is the same fact — the source is pinned to something that is no longer there —
with the same remedy.

### Recovery

The remedy is an apply, and it MUST stay reachable
([decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md)).

- **What the host reports.** The condition above, on `read` and `observe`, for as
  long as the stored relation does not resolve to its pinned UID. Reporting it
  is not an error outcome: the resource is still readable, still deletable, and
  still carries its desired spec.
- **What re-pins.** Every ACCEPTED mutation re-resolves every relation and stores
  the UIDs it resolved to, including a `apply` whose spec is byte-identical to
  the stored one. Re-pinning is host-owned bookkeeping, not desired state: it
  MUST NOT move `metadata.generation`, and it moves `metadata.revision` exactly
  when it changes the representation the host serves — which it does, because
  the Ready condition stops reporting the drift. A second identical apply then
  moves nothing at all.
- **What a client must do.** Report the break without failing the read, and offer
  an apply. A client that fails its refresh on this condition removes its own
  remedy, because the plan that repairs the resource is computed from the
  refreshed state. A Form that declares `update` recovers with a spec-identical
  apply. A Form that declares none — every `revision`-role Form — has no such
  apply at all: a host refuses every apply to the existing resource, so its only
  recovery is REPLACEMENT, and a client MUST plan one. A `DependencyMissing`
  target must exist again before either apply can succeed.

A client sees the same fact — an incarnation changed — in two places, and the two
answers differ because the remedies do. A relation whose TARGET moved is a
warning with a proposed apply: the resource itself is still the one state names,
and an accepted apply re-pins the reference. A resource whose OWN `metadata.uid`
no longer matches what state records is an error the operator must resolve, and a
client MUST keep the resource in state while reporting it: no apply converts one
incarnation into another, and dropping the resource would make the next apply
fence on `If-None-Match: *` against the resource that does exist, with no plan
left that repairs anything
([decision 0017](../decisions/0017-provider-state-survives-form-evolution-and-interruption.md)).

### Dependency protection

Deleting a resource that any stored relation references by UID fails
`dependency_in_use` (409). This covers every relation, not only typed bindings:
a Worker Bundle a Worker Version executes, a Worker Version a Worker Deployment
weights, a Module Worker an attachment activates, a queue a consumer drains, and
a dead-letter queue are all live dependencies. A resource that references itself
does not block its own deletion. An accepted (202) delete re-runs the scan at
commit time against the incarnation it was accepted for, so a resource that
acquired a holder while the operation was pending survives, and a resource that
merely took the deleted one's name is never touched.

### Binding verification

A binding relation is verified, never assumed. Before any mutation a host MUST
check, in this order:

1. the source Form Definition's `acceptedBindings` carries the Binding contract
   the desired schema annotates — otherwise `invalid_argument` (400);
2. the host has installed that contract at exactly the accepted
   `schemaDigest` — otherwise `unsupported_capability` (422), because the
   capability itself is unavailable rather than the request malformed;
3. the source Form's `role` equals the Binding Definition's `sourceRole` —
   otherwise `invalid_argument` (400);
4. the resolved target's exact Form (group and kind) appears in the Binding
   Definition's `allowedTargetForms` — otherwise `invalid_argument` (400);
5. the target Form's Definition declares the Binding's `targetInterface` in
   `providedInterfaces` — otherwise `invalid_argument` (400). A binding projects
   an Interface, so a target that provides none cannot be bound;
6. source and target share a space, which the wire guarantees: a reference
   carries no space member.

Rules 3 through 5 are about the target or holder the client chose and are
therefore argument failures; rule 2 is about what this host can do at all.

## Declared cross-resource rules

Some rules a desired-state schema cannot express. A schema cannot count the
resources pointing at one target, read a relation of a resource it does not
contain, add a column of weights, know which entrypoints a referenced revision
exports, or reach across sibling properties. Those rules live in the host under
[decision 0014](../decisions/0014-published-schemas-are-structural-minima.md).

The previous lane wrote them out one Form at a time. It said that a worker
carries one active deployment, that a queue carries one consumer, that a
database carries one live migration application — each a paragraph naming a
Form kind, in a document that is supposed to be the protocol. Its own aggregate
chapter admitted the shape of the problem: *hosted here while that family is
this lane's only family*. The consequence was mechanical and it happened: a
family that gained two Forms could not gain them without editing the protocol.

**This lane states the MECHANISMS. A family states which of them apply to
which of its references, in its own Definitions.** A host reads a Definition
and enforces what it declares, without knowing what any Form is for. The lane
therefore names no Form kind, and a check enforces that it does not.

Every rule below is decided against the UID a reference RESOLVED to, never
against the name a spec spells.

### Exclusive holds

A reference may declare `x-takoform-exclusive`: **at most one LIVE resource of
this Form kind may hold the target this reference resolves to.** A second one
fails `invalid_argument` (400) before any mutation — the request is well formed
and what is untrue is what it says about the target it points at.

The annotation is an object. Its optional `keyedBy` member is a JSON Pointer to
a sibling property of the same desired spec, and it joins the target in the
key:

| `keyedBy` | The key is | Two holders conflict when |
| --- | --- | --- |
| absent | the resolved target | they resolve the same target |
| a pointer | the target and that property's value | they resolve the same target AND carry the same value |

A conflict is decided over LIVE resources only. A resource an accepted delete
has removed holds nothing, so the target it held is free — a rule that outlived
its holder would make a destroyed-and-recreated target permanently
unmanageable.

The rule is per exact Form kind, not per target: two resources of DIFFERENT
kinds resolving one target are not a conflict. That is what lets one target
carry an exclusive holder of one kind and any number of resources of another.

### Summed members

An object-list property may declare `x-takoform-sum`: the named integer member
of its elements MUST total exactly the stated value. A list that does not fails
`invalid_argument` (400) before any mutation.

A schema can bound each element and cannot add a column. This is the whole of
what the annotation adds, and it adds it as data rather than as a sentence
about one Form's traffic weights.

### Claimed values

A property may declare `x-takoform-claim`: its value is held by **at most one
live resource per tenant**, across every space, compared on the CANONICAL form
the property's own schema defines. A second claimant fails `invalid_argument`
(400) before any mutation, and releasing the holder makes the claim
representable again.

Canonicalization happens before comparison and before storage, so two spellings
of one value are one claim rather than two. Which spellings are equal is the
property's schema to say; that the comparison happens on the canonical form is
this lane's.

### Host-assigned outputs

A declared output member may carry `x-takoform-host-assigned`: the host mints
the value, it is immutable for the lifetime of the resource's UID, and no
desired property may state it. A consumer may store it; a portable
configuration never parses it, asserts a suffix of it, or reconstructs it from
a resource name.

### Activation entrypoints

A reference may declare `x-takoform-required-entrypoint`: the entrypoint of the
target's runtime Interface that this resource's inward activation invokes.
Activation is admitted against the target's ACTIVE selecting resource — not
against any stored revision — and EVERY revision that selection weights MUST
provide the annotated entrypoint, because an event served by any weighted
revision has to find it.

An absent selection, or a weighted revision that does not provide the
entrypoint, fails `unsupported_capability` (422) before any mutation and the
message names what is missing. A stored revision is a history entry, not a
running one: gating on it would admit an activation against code no selection
serves.

A resource that SELECTS a named export rather than activating one states the
same requirement through its own top-level property, and is gated on it
wherever a selection exists to contradict it. It differs in the absent-selection case, and the difference is
structural rather than a concession: a revision may declare a binding to the
selector its own identity serves, so identity, revision, and selection are
ordinarily created in ONE apply and no order exists in which the selection
comes first. Such a resource STORES with `Ready=False` / `Provisioning`, and
the selection that lands next is what makes it serve.

### Dependents and reverse validation

The dependents of a selecting resource are DERIVED, not listed: they are the
live resources whose annotated entrypoint that selection must keep serving, and
the live bindings that resolve to it. A change that would stop serving a
dependent's entrypoint, and a delete of a selection a dependent still needs,
both fail `dependency_in_use` (409) naming what depends on it.

Nothing here enumerates a Form kind. A family that adds an activation adds an
annotated reference, and its dependents are counted by the same walk that
counted every other one.

## External standard services

A revision-role Form MAY embed sealed external-service slots
([`../standard-services/README.md`](../standard-services/index.md),
[decision 0045](../decisions/0045-external-standard-services-are-sealed-slots.md)).
The lane adds the rules only a wire contract can carry:

- **Derivation.** The array property embedding the slots carries the
  `x-takoform-standard-services` annotation, exactly parallel to
  `x-takoform-binding`, so a host holding only the Definition derives every
  slot it must enforce. An un-annotated slot-shaped array is a Definition
  defect a package verifier refuses.
- **Slot shape.** `{name, service, required?}`: `name` matches
  `^[A-Z][A-Z0-9_]*$` (64 max) — the SCREAMING_SNAKE grammar
  `requiredSensitiveVars` already uses, because the projection mints
  `NAME_`-prefixed members and a lowercase or `$`-bearing name would make
  those unpronounceable; `service` validates against the StandardServiceRef
  schema; `required` is an optional boolean defaulting to true. An optional
  slot the host does not satisfy projects nothing and blocks nothing.
- **The namespace closure.** Uniqueness is enforced over the PROJECTED
  closure, not the declared names: the union of vars keys, sensitive-value
  names, binding names, and every member each slot projects
  (`MEDIA_ENDPOINT`, `MEDIA_ACCESS_KEY_ID`, …) must be collision-free, and a
  host refuses the collision before mutation exactly as it refuses the
  declared-name collision — a var named `MEDIA_ENDPOINT` beside an
  `s3-compatible` slot named `MEDIA` is `invalid_argument`.
- **Plan surface.** `validate` never depends on the tenant, so slot
  satisfiability is answered by the `StandardServiceSupport` profile and
  enforced at `prepare`: an unsatisfiable REQUIRED slot is
  `unsupported_capability` there, and again at `apply` where prepare could
  not know. An unsatisfied REQUIRED slot discovered later renders
  `Ready=False` / `DependencyMissing`.
- **Sealing.** Projected values never appear in desired state, observed
  state, outputs, diagnostics, or provider state; the observed document MAY
  state that a slot is satisfied and MUST NOT state with what.

Adopting a slot-projecting runtime is a runtime-contract revision: a family
whose runtime Interface pins its environment membership (the Edge family's
`worker.runtime` does) mints the next Interface version to admit the
projected members, per
[decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md).

## Lifecycle capabilities

`lifecycleCapabilities` is a claim about what a host can actually be asked to
do, so it is derived from the Form's own declared fields rather than from its
role. The base set every Form declares is exactly `create`, `read`, `delete`,
`import`, `observe`. `update` is added only when the Form has at least one
mutable desired field — a field that is not immutable, on a Form whose role is
not `revision`. A Form with no field at all, or whose every field is immutable,
therefore declares no `update`.

The rule is enforced on the wire in both directions. A host MUST refuse a
spec-CHANGING apply to an existing resource whose Form Definition omits
`update`, failing `invalid_argument` before any mutation; a client MUST plan a
replacement rather than an in-place change for such a Form.

## Portable defaults

An optional desired property carries its portable meaning in its own schema:
the Form Definition declares a JSON Schema `default` on that property inside
`desiredSchema`. That default is normative, not advisory.

A host MUST materialize every declared top-level default into the desired spec
at the entry point of `validate`, `prepare`, `apply`, and `import` — before the
spec is validated, digested, stored, or echoed. Materialization fills only
absent properties; a property that is present keeps its written value, even
when that value equals the default. It follows that:

- the effective spec IS the wire spec: there is no second, host-private
  document a client cannot see;
- `specDigest` is the digest of the MATERIALIZED spec;
- omitting a defaulted property and writing its default are one desired state:
  same `specDigest`, same `metadata.generation`, no update;
- a client that sends an unmaterialized spec MUST accept the materialized echo
  from `prepare`, `apply`, `import`, and `read` as its own desired state.

An optional property with no declared default is only legitimate when its
ABSENCE is itself the portable semantics, and the property's `description`
must then state what the absent case does.

## Declared outputs

The rules of this section are decided by
[decision 0025](../decisions/0025-declared-outputs-are-a-typed-contract.md).

A Form Definition MAY declare an `outputSchema`: the closed Draft 2020-12
contract of the host-computed values it publishes. Every declared member is
required and the object carries `additionalProperties: false`, because an output
a host may omit forces every consumer to invent a fallback, and an undeclared
member is a value the contract never described.

A host is held to it in both directions, which is the rule the wire schema
already states through `x-takoform-requiredWhen` and `x-takoform-omittedWhen`:

- for a Form that declares an `outputSchema`, `status.outputs` is PRESENT,
  validates against that schema, and carries exactly its members;
- for a Form that declares none, `status.outputs` is OMITTED — not an empty
  object.

This is the required conformance check `form-declared-outputs-are-exact`, driven
across a Form that declares outputs and Forms that declare none, because a host
returning an empty document everywhere would satisfy only the first half. Each
returned value is measured against the declared schema — its type, its anchored
pattern, its bounds — rather than against an assumption about it: a check that
only asked for a non-empty string would accept `hostname: "not a hostname"` and
reject an integer output the authoring model permits. The conformance corpus
pins the whole declared schema for that reason, since no wire surface serves
one.

An output is not desired state. It carries no default, no immutability, and no
cross-resource reference: those describe what an author asks for, and an output
is what the host answers. A change to an output moves `metadata.revision` and
never `metadata.generation`, like every other representation change.

The `outputSchema` is not served on the wire. The form-definition response is a
closed document carrying identity, display name, description, and
`desiredSchema`; `displayName` is exactly the installed Definition's `title`
and is always present — a host neither invents nor omits it. The bytes are
immutable
([decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)),
so a client learns a Form's output contract from the Form Package it installs.

The Terraform and OpenTofu provider projects each declared output as a typed
computed attribute, and retains `outputs_json` — carrying the WHOLE document,
unnarrowed — as the way to reach an output no schema describes. It writes null
for a declared output only where no representation exists — the state an
accepted-but-unfinished mutation leaves behind — and fails an ordinary create,
update, or refresh whose response omits a declared output or returns it with the
wrong type, naming the output and what was wrong.

## Long-running operations

Create, update, delete, import, and artifact commit MAY
return `202 Accepted` with an Operation envelope
([`../schemas/operation-v1alpha2.schema.json`](/schemas/operations/v1alpha2/operation.schema.json))
instead of the terminal representation:

```
GET  {api}/operations/{id}
POST {api}/operations/{id}/cancel
```

Clients poll with `Retry-After` when present, otherwise exponential backoff
with full jitter, under an overall deadline. The contract guarantees only
that an operation ID is stable, that the host keeps it addressable at
`GET {api}/operations/{id}` while the operation record exists (afterwards the
closed outcome is `operation_not_found`), and that a terminal operation
replays its terminal state instead of re-executing. A terminal Operation carries
exactly one of `result` or `error` — the schema enforces the exactly, so a
done record with neither, or both, is protocol-invalid.

Cancel has exactly three outcomes, and the machine table carries them as
data. The request body is empty. A safely stoppable running operation is
cancelled: the answer is `200` with the operation now terminal, `done: true`
and `error.code: operation_cancelled` — cancellation is a terminal FAILURE of
the operation, never a fourth state. An already-terminal operation replays
its terminal record unchanged, also `200`. An operation past its safe
stopping point answers HTTP `409` `operation_cancelled` with
`retryable: false` and keeps running to its own terminal state; nothing about
it changed. `operation_not_found` (404) covers an id the tenant does not
hold; `deadline_exceeded` (504) reports a host-side deadline on the
underlying work, never on the poll.

A `202` accepts a mutation to **one resource**, and the commit is bound to that
one ([decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md)).
A host records the target's exact `formRef` and `metadata.uid` beside the fence
the mutation was accepted under, and at commit time resolves through that record
rather than re-deriving a target from the name the request addressed. A name is
unique per kind and reusable, and a re-created resource starts at generation 1
and at revision 1, so whichever counter a fence names, a target removed out of
band and re-created — under the same contract or under
another definition version of it — presents a NEW incarnation behind the same
address that satisfies the fence the original was accepted under. When the name
is held by a different incarnation the operation terminates `uid_mismatch` (409);
when nothing holds it, `resource_not_found` (404). Neither commits anything, and
neither is retryable: no wait turns one incarnation back into another. A create
is fenced against the free name rather than against an incarnation, so it pins
none and `If-None-Match: *` decides at commit as it always did. This is the
required conformance check `async-commit-binds-the-accepted-identity`.

An operation id is a resumption handle, **not a capability**. A host records the
authenticated tenant and principal that the mutation was accepted from, and
answers `GET {api}/operations/{id}` and
`POST {api}/operations/{id}/cancel` from any other tenant or principal with
`operation_not_found` (404) — the same answer an id that never existed gets.
`permission_denied` would confirm that the id names a real operation, which is
the one fact a stranger holding a guessed or leaked id is trying to learn
([decision 0018](../decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)).
This is the required conformance check
`operation-bound-to-its-creating-principal`.

"While the record exists" is a retention window, not an ordering: a host MUST NOT
drop a terminal operation merely because the system moved on. As long as the
record is retained, the ID stays addressable and its terminal state replays
byte-identically after the resource it names has been mutated again and after
other operations have been accepted and settled. This is the required
conformance check `operation-resumable-after-settlement`, and it is what makes a
persisted operation ID a usable handle rather than one whose value depends on
timing.

A client that persists the ID resumes from it
([decision 0017](../decisions/0017-provider-state-survives-form-evolution-and-interruption.md)):
the Terraform/OpenTofu provider records it as `pending_operation_id` and consults
it BEFORE it reads the resource, because on a host that commits the resource only
when the operation commits, a `resource_not_found` during that window means "not
yet", not "deleted". A terminal success is verified against the exact identity
before anything is adopted, and an expired record defers to an exact resource
read rather than being read as failure.

## Artifacts

The content-addressed upload API is
[`../artifact-transport/`](../artifact-transport/index.md). The artifact
endpoints share the lane's auth, idempotency, and error taxonomy.

The desired state of an artifact-backed revision resource is the **manifest
digest and nothing else**: the committed artifact manifest describes the
bytes, so the manifest and the desired spec are never two spellings of the
same facts. Which manifest kind a given Form requires is that Form's family to
state ([`../form-families.md`](../form-families.md)); a host MUST resolve the
referenced manifest before
it mutates anything, on apply and on import alike, and fail closed when

- the digest names no committed manifest the caller's tenant holds —
  `artifact_missing` (404). Resolution is the same per-tenant question the
  artifact read surfaces ask, so a manifest another tenant committed is answered
  exactly as an uncommitted digest is;
- the stored manifest's RFC 8785 canonical digest differs from the referenced
  digest, its document is undecodable, or its `kind` is not the kind the Form
  requires — `artifact_invalid` (400);
- the manifest violates any rule of
  [`../artifact-transport/`](../artifact-transport/index.md), including its
  per-kind exclusivity, its media-type policy, entry-count or aggregate-byte
  ceilings, and the host's published `limits` — `artifact_invalid` (400).

A committed manifest and its blobs MUST remain readable while any resource
references the manifest. Abandoning an unrelated upload session, or
garbage-collecting staged blobs, MUST NOT make a referenced artifact
unresolvable.

### A bundle's modules are what the runtime can load

A manifest whose entries are a module graph splits its media types into a
LOADABLE set the runtime contract imports and an AUXILIARY set the bundle
carries and the graph never imports. WHICH types fall on each side is the
runtime contract's to state and the family's to reference
([decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md));
that the split exists, and what a host does with it, is this lane's.

A host MUST refuse such a manifest whose `mainModule` names an auxiliary
module, before commit and before any mutation that references the manifest,
with `artifact_invalid` (400) — and MUST NOT refuse a bundle merely for
CARRYING one. A published manifest schema states the union in one enum and
cannot relate `mainModule` to the media type of the module it names, so the
split is host-enforced under
[decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)
and proved by the required conformance check `bundle-main-module-is-loadable`.
The corresponding runtime obligation — an import resolving to an auxiliary
module fails `unsupported_media_type` — is behavior no desired-state runner
observes, and stays a host obligation with the rest of the ABI.

### A Form's own artifact semantics are its family's

What an artifact MEANS to the Form that references it — which manifest kind
it must be, what its entries may contain, what the host does with them, and
what makes the referencing resource Ready — is stated by the family that
defines that Form, not here. The previous lane wrote two such rulebooks into
the protocol document, and they were the clearest case of the problem this
lane exists to fix: a reader of the protocol had to scroll past one family's
asset-fallback order and another's migration ledger to reach the next
protocol rule.

What stays here is the obligation every artifact reference carries, whatever
it references: **a host MUST resolve the referenced manifest and hold it to
the contract the referring Form states, before it mutates anything, on apply
and on import alike.** A reference whose manifest does not resolve, or
resolves to something the Form does not accept, fails before mutation — never
after, and never as a readiness condition on a resource that was already
stored.

The Edge Platform Family's own artifact semantics are in
[`../form-families.md`](../form-families.md).

### Upload sessions are owned

An upload id is a handle bound to the tenant and principal that started the
session, exactly as an operation id is. Session identity is
per-(tenant, manifest digest): a start request for a manifest the tenant
already has an open session for answers that session's id again (200), and a
start for already-committed bytes answers the committed state rather than a
new session. An open session survives at least an hour of inactivity before a
host may reclaim it; a blob PUT into a session already committed is
`invalid_argument`. Continuing it
(`PUT {api}/artifacts/uploads/{uploadId}/blobs/{sha256}`), committing it, and
abandoning it from any other tenant or principal all fail `artifact_missing`
(404). Abandoning an id the caller does not hold fails the same way whether it
never existed or belongs to someone else: answering `204` for one and `404` for
the other would make the difference observable, which is the disclosure the rule
exists to prevent. This is the required conformance check
`upload-session-bound-to-its-creating-principal`.

### A content address is not a capability

A digest names bytes; it does not entitle a caller to them
([decision 0018](../decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)).
`GET {api}/artifacts/{manifestDigest}` and
`HEAD {api}/artifacts/blobs/{sha256}` answer only a caller whose TENANT already
holds that address — by having uploaded the bytes, or by having committed a
manifest that names them. Any other tenant is answered `artifact_missing` (404)
for a manifest and `404` for a blob, even for a digest that exists. It follows
that `missingBlobs` is answered per tenant and not per byte store: reporting a
blob as already present because a different tenant uploaded it would hand over
bytes on the strength of a digest.

A host MAY still deduplicate physically — one stored copy per content address,
however many tenants hold it. Two tenants that upload the same bytes and commit
the same manifest see the same immutable identity, and neither one's abandon or
garbage collection may take the bytes away from the other. This is the required
conformance check `artifact-digest-is-not-a-capability`.

The rule governs **using** an address as well as reading one. Wherever a host
resolves a manifest or a blob on behalf of a request — above all the referenced
manifest of a bundle-shaped desired state — it asks the same per-tenant holding
question, so a caller who merely learns a digest cannot apply or import their way
to another tenant's artifact. The refusal is the ordinary `artifact_missing`
(404), raised before any mutation on apply and import alike and re-raised when a
202 commits, and it MUST NOT distinguish "exists but is not yours" from "does not
exist": a distinguishable refusal is the existence oracle the read rule removes.
Holding is the tenant's, so a manifest one principal uploaded is referenceable by
another principal of the same tenant, and a tenant that supplies the same bytes
itself references the same immutable identity like any other holder. This is the
required conformance check `manifest-reference-is-not-a-capability`.

## Support profiles

```
GET {api}/support/forms
GET {api}/support/forms/{formGroup}/{formVersion}/{kind}/{definitionVersion}
GET {api}/support/interfaces/{name}/{version}
GET {api}/support/bindings/{name}/{version}
GET {api}/support/standard-services/{protocol}
```

Responses validate against
[`../schemas/host-support-profile-v1alpha2.schema.json`](/schemas/support/v1alpha2/host-support-profile.schema.json).
A profile declares supported exact refs, closed capability subsets
(`supportedEnums`), inclusive ranges (`supportedRanges`), supported binding
contracts, and numeric limits. A profile's `operations` member is the subset
of the Form's declared `lifecycleCapabilities` this host serves: narrowing is
lawful (a host serving no `import` refuses it `unsupported_capability` at
prepare, [decision 0031](../decisions/0031-host-capability-is-decided-at-plan-time.md));
widening is not — an operation the Definition does not declare never appears.
`supportedEnums`, `supportedRanges`, and `limits` keys are RFC 6901 JSON
Pointers into the desired document (`/assets/notFoundHandling`,
`/versions/-/weight` with `-` addressing every element), so nested and
array-element members are addressable; every `limits` value stays within
2^53−1 so the document remains I-JSON.

A `StandardServiceSupport` profile answers the standard-services route: its
`serviceRef` names the protocol and `satisfiable` says whether this host will
satisfy slots of that protocol for the requesting tenant — never with what.
Satisfiability is tenant wiring, so the answer is tenant-scoped, `validate`
never depends on it, and the plan-time refusal of an unsatisfiable REQUIRED
slot lives in `prepare` (`unsupported_capability`), the earliest surface that
knows the tenant ([`../standard-services/README.md`](../standard-services/index.md)). Artifact-backed Forms publish
`maximumBundleBytes` together with `maximumBundleFiles`, at the portable floors
their family states. Price, SKU, region, quota, and
commercial policy MUST NOT appear; those remain Service Offering data outside
this API.

A host that supports a Form providing a runtime ABI contract MUST advertise
that contract at the exact `schemaDigest` the Form's `providedInterfaces`
names, and MUST advertise any enum derived from it as exactly the vocabulary
that contract defines. It MUST NOT advertise a compatibility date range or a
compatibility flag enum: runtime behavior is stated by implementing an exact
contract, not by a token no registry interprets
([decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).
A revision declaring an entrypoint the runtime contract does not define MUST
be refused before any mutation, and so MUST a revision declaring an
entrypoint the artifact it references does not provide:
loading fails that revision before any traffic
arrives, so storing it would leave the activation gate above admitting an
activation against an entrypoint that does not exist. The first is
decidable from the spec alone and is refused on `validate`, `prepare`, and
`apply` alike; the second needs the bundle relation, the committed manifest, and
the module bytes, so it is refused before any mutation on `apply` and `import`
alike and re-raised when a 202 commits — `invalid_argument` (400) in both cases,
because the request is well formed and states something untrue about what will
run. These are the required conformance checks
`module-worker-runtime-contract-advertised`,
`undeclared-runtime-handler-rejected`, and
`declared-handler-not-exported-rejected`.

### What the lane proves, and what stays a host obligation

`worker.runtime@1.0.0` fixes far more than a black-box lifecycle runner can
observe, so the split is stated rather than implied.

Proven by required checks:

- the host advertises the runtime contract at the EXACT pinned `schemaDigest`,
  and supports the exact Form line that provides it;
- any entrypoint enum it advertises is exactly that contract's
  vocabulary, and it advertises no `compatibilityDate` range or
  `compatibilityFlags` enum;
- a version declaring a handler outside the vocabulary is refused, and one
  declaring only defined handlers is still accepted;
- a version declaring a handler its referenced module does not export is
  refused against a corpus-pinned bundle, and one declaring only exported
  handlers is still accepted;
- every inward-activation attachment is gated on the handler its events invoke,
  in both directions;
- a host-assigned output is answered with a complete address the host
  minted, in canonical form and with every published member built from one
  hostname; that address is still the same address, under the same UID, after a
  host-side status refresh, a promotion of the worker it serves, and a re-read;
  and a second endpoint against one worker is refused.

Obligations a conforming host MUST meet that this lane does NOT prove, because
proving them means executing the module rather than driving the Host API:

- the default export's shape, the three-argument handler signatures, and that a
  returned promise is awaited;
- `handler_not_exported` for an ARBITRARY module — the lane drives bundles whose
  exports the corpus pinned, so what it proves is that a host refuses the
  version when it knows, not that it derives the export set from any bytes an
  author uploads;
- request and response bodies streaming rather than buffering;
- `env` carrying exactly the declared names and nothing else portable;
- `ctx.waitUntil` holding the isolate open, and a rejected task never changing
  an already-sent response;
- an uncaught throw becoming a host-generated 500 in `fetch` and a failed
  invocation in `scheduled` and `queue`;
- the `globals` floor, the loadable module media types, and that an import
  resolving to an auxiliary module fails `unsupported_media_type` rather than
  linking source-map evidence into the graph;
- that a request to a host-assigned published address actually ARRIVES at
  the resource. The lane drives desired state and sends no traffic, so what it
  proves is that a host answers with a complete address it minted and
  keeps that address stable across a promotion — not that anything is listening
  on it. Nor can it prove the refusal branch: a black-box runner cannot take the
  address-assignment capability away from the host under test, so
  `unsupported_capability` (422) for a host that cannot assign one is stated
  normatively and left to that host to honor.

Nothing the ABI declares is unmeasurable any more. `worker.runtime@1.0.0` used
to declare a `tail` handler no resource in this family could activate, so no
run could observe it and two hosts could implement it differently with nothing
able to detect the divergence; the handler was removed rather than left in a
published-shaped contract nothing reaches, and the runtime corpus now REFUSES a
declared handler with no check measuring it, so the property is enforced rather
than described. `tail` returns with the attachment that makes it observable and
a new exact runtime Interface version, never as a bare handler
([decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).

Those are stated normatively in the contract's own descriptions and proven by
its behavior fixtures, which a runtime conformance run executes against a real
isolate ([`../interface-contract/`](../interface-contract/index.md)). That run
is [`../../conformance/runtime-abi-v1/`](../../conformance/runtime-abi-v1/contract.json),
whose runner drives a worker deployed from its own byte-pinned bundle and a
disposable adapter over the runtime's module loader; it is a separate corpus,
runner, and command from this lane precisely because the subjects differ
([decision 0023](../decisions/0023-the-runtime-abi-is-measured-separately-from-the-control-plane.md)).
A host that passes this lane has not thereby proven it implements the ABI; it
has proven it says which ABI it implements and holds desired state to it.

The same split covers the four data-plane contracts —  `edge.kv`,
`edge.objects`, `edge.sql`, and `edge.queue` — and `worker.service`, whose whole
delivery model is a claim about bytes moving between two workers: that neither
body is buffered, that an absent body is distinguishable from an empty one and
from one whose length the writer does not yet know, that a length the head DOES
declare is one the bytes keep, that backpressure and cancellation propagate,
that a request abort and a response abort are different observable outcomes,
that a callee exception arrives as a complete host-generated 500 rather than a
hung call, and that a call which could not be dispatched fails rather than
answering with a status. `edge.sql` separately fixes its safe binary64 value
corridor, canonical encoded BLOBs, rollback-only queries, one-statement runtime
boundary, admin-only schema migrations, and materialize-before-commit
transactions. This lane drives desired state and never moves a byte of
application data
([decision 0020](../decisions/0020-the-edge-interfaces-state-their-data-and-delivery-model.md),
[decision 0034](../decisions/0034-edge-sql-uses-safe-wire-values-and-rollback-only-queries.md)).

Proven by required checks:

- the host advertises each of the four contracts at the EXACT pinned
  `schemaDigest`, which is what turns "this host has a KV store" into a
  statement about a consistency model, a value model, an error vocabulary, and a
  set of limits (`edge-interface-contracts-advertised`);
- a schedule expression that is a shape rather than a schedule is
  refused, and the sub-hourly schedules the grammar exists for are accepted
  (`cron-grammar-enforced`);
- a second holder of an exclusively held target is refused, and the same holder
  is accepted once the first is gone (`queue-single-consumer-enforced`);
- a claimed value written in any spelling of one name is stored under
  the canonical one, and a second attachment claiming it — in either spelling —
  is refused while the first lives and accepted once it is gone
  (`custom-domain-hostname-canonicalized`,
  `custom-domain-hostname-claim-unique`);
- a hostname another TENANT serves is claimable, both claims stay live under two
  uids, releasing one leaves the other alone, and inside either tenant a second
  claim is still refused without naming anyone else's resource
  (`custom-domain-hostname-claim-stops-at-the-tenant`);
- a resource whose dead-letter destination resolves to its own target, or
  closes a cycle through another consumer's, is refused, and an acyclic
  destination is accepted, including one that shares a destination with another
  chain (`dead-letter-cycle-rejected`).

Obligations a conforming host MUST meet that this lane does NOT prove, because
proving them means exercising the data plane rather than driving the Host API:

- **Static-asset routing.** That request paths resolve to the exact bytes in the
  referenced asset manifest, that a family's declared stage order is honoured,
  and that a declared fallback serves those bytes rather than a host-owned
  document. The reference host proves the exact
  relation and refuses an SPA bundle without that path; it serves no HTTP
  application traffic.
- **SQLite migration execution.** That each SQL file and its `(path, digest)`
  ledger insertion commit in one real SQLite transaction, that a failed file
  leaves neither schema effects nor a ledger record, and that concurrent
  applications serialize on the same database. The reference host proves the
  ordered prefix/suffix state machine and never pretends its in-memory ledger
  executed SQL.
- **`edge.kv` convergence.** That a write eventually becomes visible at every
  location. One client cannot observe cross-location convergence at all, which
  is why the contract's deterministic fixtures assert only facts no write has to
  converge for. The obligation is that convergence happens, that a host's own
  convergence target is stated in its Host Support Profile, and that the
  contract's other reading — read-your-writes — is NOT provided and must not be
  relied on.
- **The encoded-bytes value model.** That `maxValueBytes` and
  `maxMessageBytes` bound the DECODED length, that `maxKeyBytes` bounds a key's
  UTF-8 encoding, and that undecodable base64 is refused.
- **`edge.objects` streaming, ranges, conditionals, and multipart.** That a body
  is never buffered into a JSON member, that a ranged `get` returns exactly the
  requested subrange of one object version, that a failed precondition changes
  nothing, and that a multipart upload assembles its parts atomically or not at
  all. The contract's fixtures state the first three as traces a runtime
  conformance run executes; multipart cannot be a static trace, because its
  steps depend on part etags the host mints while the trace runs.
- **`edge.sql` value and effect boundaries.** That safe finite binary64 numbers,
  UTF-8 text, null, and canonical encoded BLOBs round-trip unchanged; unsafe
  input and output fail `numeric_out_of_range` without rounding; a runtime call
  refuses multiple statements, transaction-control SQL, and schema migration;
  `query` materializes inside a transaction it always rolls back with
  `rowsWritten: 0`, even when the statement transiently writes; and a
  transaction materializes every bounded result before committing all effects
  or none. The Interface fixtures state the deterministic wire/refusal half;
  proving persistent effects requires a real SQLite data plane
  ([decision 0034](../decisions/0034-edge-sql-uses-safe-wire-values-and-rollback-only-queries.md)).
- **`edge.queue` delivery.** That a `messageId` is stable across redeliveries,
  that `attempts` counts them, that the first delivery does not count toward
  `maxRetries`, that an uncaught handler exception retries every message not
  already acknowledged, and that a dead-lettered message arrives as a new
  message with `attempts` starting again at 1. All of it needs a consumer and
  the passage of time, neither of which a desired-state runner has.

Those are stated normatively in each contract's own descriptions and proven by
its behavior fixtures wherever a fixture can prove them.

## Errors

The taxonomy is closed, and this document defines every code in it. Nothing
is inherited: a code either has its trigger stated here or it is not in the
lane. `retryable: true` is lawful only on `resource_busy`,
`backend_unavailable`, `rate_limited`, and `deadline_exceeded`;
`Retry-After` is honored when present; every other code carries
`retryable: false`.

| Code | HTTP | A host answers it exactly when |
| --- | ---: | --- |
| `invalid_argument` | 400 | the request fails validation before any fence or mutation: malformed or missing query/header/body members, duplicate JSON members, a fence pair that disagrees, a spec the exact Form's desired schema refuses, a review echo that does not match, or an expired/substituted prepare digest |
| `artifact_invalid` | 400 | uploaded or committed bytes violate the artifact manifest they were declared under |
| `unauthenticated` | 401 | the request carries no credential, or one that names nobody |
| `permission_denied` | 403 | the authenticated principal lacks authority for this operation on this resource |
| `policy_denied` | 403 | the principal has the authority, and a host-side policy refuses the act anyway; `hostCode`/`details` MAY name the policy, portable state never does |
| `form_unknown` | 404 | the exact FormRef names a group/kind/version this host has never installed |
| `resource_not_found` | 404 | the addressed resource does not exist under the exact ref for this tenant — indistinguishably from never-created, foreign-tenant, and wrong-exact-ref |
| `operation_not_found` | 404 | the operation id names no record the principal's tenant holds |
| `artifact_missing` | 404 | a referenced content address names bytes the caller's tenant has not committed |
| `form_not_installed` | 409 | the exact FormRef is known but not installed as executable code — `definitionKnown` true, `installed` false in the availability answer |
| `resource_busy` | 409 | the resource exists and cannot answer now because the host is mid-way through another mutation of IT (a concurrent apply, a commit in flight, an index backfilling); transient by definition, hence retryable |
| `import_conflict` | 409 | an adoption names a `nativeId` another live resource of the tenant already holds |
| `operation_cancelled` | 409 | a cancel arrived past the operation's safe stopping point (the HTTP refusal), or — inside a terminal operation record — the operation ended because it was cancelled |
| `dependency_in_use` | 409 | deletion is refused because a live relation, binding, dead-letter target, subscription, or dependent resource still references the target |
| `migration_required` | 409 | the request addresses state a newer contract wrote and this host will not silently reinterpret |
| `uid_mismatch` | 409 | a fence or commit named a uid and the name is now held by a different incarnation |
| `unsupported_capability` | 422 | the exact Form, operation, declared binding, external-service slot, or entrypoint requirement is one this host does not serve for this tenant; answered at the earliest surface that can know it (prepare, or apply where prepare could not) |
| `revision_conflict` | 412 | a supplied ETag/revision fence does not match the current revision |
| `generation_conflict` | 412 | a supplied generation fence does not match the current generation, or a create found the name occupied |
| `rate_limited` | 429 | the tenant exceeded a host-side rate policy; retryable, `Retry-After` states when |
| `backend_unavailable` | 503 | the host's backend cannot be reached; nothing about the request is wrong |
| `form_unavailable` | 503 | the exact Form is installed but its backing capacity is temporarily not serving — `installed` true, availability transiently false |
| `internal_error` | 500 | the host failed; the request may or may not have taken effect, so a mutation is never blindly retried |
| `deadline_exceeded` | 504 | the host gave up waiting on its own downstream work; completion state is unknown, so a mutation is never blindly retried |

The v1alpha2 codes `resource_version_conflict`, `interface_identity_ambiguous`,
and `interface_instance_ambiguous` do not exist in this lane: version
conflicts split into the revision/generation pair, and the v1alpha2 interface
projection is replaced by exact Interface contracts
([`../interface-contract/`](../interface-contract/index.md)). The v1beta1
codes `form_identity_conflict` and `deletion_protected` do not exist in this
lane either: the first was never given a trigger, and the second claimed a
policy surface no Form provides. A withdrawn code is never reused meaning
something else.

An error body always carries `code`, `message`, `requestId`, and `retryable`,
MAY carry `hostCode` and `details`, and never carries a credential, a price,
or another tenant's existence.
