# Host API v1alpha3

`forms.takoform.com/v1alpha3` is the current Host API lane
([decision 0013](../decisions/0013-v1alpha3-lane-ships-in-provider-v2-1.md)).
It carries namespaced FormRef groups, UID/generation/revision resource
identity, long-running Operations, content-addressed artifact upload, and
Host Support Profiles. The retained v1alpha2 contract in
[`README.md`](index.md) continues to govern the frozen provider-v2 lane;
rules stated there and not changed here (space identity, I-JSON raw-first
decoding, idempotency-key namespacing, replay fingerprinting, same-origin
endpoint negotiation) apply to this lane unchanged.

The wire schema is
[`../schemas/host-api-wire-v1alpha3.schema.json`](/schemas/v1alpha3/host-api-wire.schema.json);
the machine operation table is [`operations-v1alpha3.json`](operations-v1alpha3.json).

## Discovery

`GET /.well-known/takoform/v1alpha3` returns a document validating against
[`../schemas/host-discovery-v1alpha3.schema.json`](/schemas/v1alpha3/host-discovery.schema.json):
`api_versions` is exactly `["forms.takoform.com/v1alpha3"]`; the features
`service_forms`, `exact_form_ref`, `optimistic_concurrency`,
`idempotent_lifecycle`, `operations`, `artifact_upload`, and
`support_profiles` are all required and true; `endpoints.api` is same-origin
with path `/apis/forms.takoform.com/v1alpha3`. Each lane has its own
discovery path; a v1alpha2 client can never select this lane accidentally.

Every advertised endpoint path is compared in its escaped form and MUST carry
no percent-encoding at all. A client rejects the discovery document otherwise
([decision 0018](../decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)):
an escaped path describes a shape this lane does not have, and comparing the
decoded path instead would let `%2Fv1alpha3` pass as `/v1alpha3`.

A client negotiates each lane independently, under a deadline of its own that
is short and separate from its resource-operation deadline. Nothing about one
lane's outcome may make another lane's resources unusable, and each resource
type reports its own lane's negotiation error.

## Path shape

A namespaced Form group travels as **two ordinary path segments** — the group
name, then the group version — wherever a URL template names a group:

```
{api}/resources/{formGroup}/{formVersion}/{kind}/{name}
{api}/form-definitions/{formGroup}/{formVersion}/{kind}
{api}/support/forms/{formGroup}/{formVersion}/{kind}/{definitionVersion}
```

So `edge.forms.takoform.com/v1alpha1` travels as
`edge.forms.takoform.com/v1alpha1`. **No path segment ever percent-encodes a
slash.** Proxies, gateways, and web frameworks disagree about whether `%2F`
inside a path segment is passed through, decoded, rejected, or normalized, so a
lane that required it could not be placed behind ordinary infrastructure at all
([decision 0018](../decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)).
A host rejoins the two segments into the exact apiVersion; the FormRef
`apiVersion` string is unchanged everywhere else — request bodies, responses,
and the `group` query key still carry `edge.forms.takoform.com/v1alpha1`
verbatim. This is the required conformance check
`namespaced-group-travels-as-two-path-segments`.

## Resource identity

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
- A host that renders any part of a representation from OTHER resources MUST
  advance that resource's `metadata.revision` when the rendering changes, and
  MUST NOT move its `metadata.generation`: no desired spec changed. This lane
  has two such renderings — relation drift and Worker readiness — so creating,
  re-weighting, or deleting a `WorkerDeployment` moves the revision of the
  `ModuleWorker` whose readiness follows it, and a target that is deleted or
  replaced moves the revision of every source pinned to it. Serving the changed
  representation under the old revision would make the ETag a validator that
  reports "unchanged" about a change, and `If-Match` a fence on a
  representation the client never saw. After an accepted mutation a host
  recomputes the rendering of every resource that mutation can affect and
  advances only those that actually changed, so an idempotent re-apply moves
  nothing anywhere. This is a required conformance check
  (`dependent-revision-advances-with-rendering`), and it is decided by
  [decision 0016](../decisions/0016-the-worker-aggregate-has-one-active-deployment.md).
- `status.observedGeneration` names the desired generation the status
  reflects; `status.conditions` uses the closed types `Ready`, `Reconciling`,
  `Degraded`, `Drifted`, `Blocked`, `Deleting` with closed portable reasons
  and an optional non-portable `hostReason`. A client surfaces the WHOLE
  condition list, not a boolean derived from it: the reason is what says why a
  resource is not ready, and a client that discards it forces its operator to
  leave the tool to find out
  ([decision 0018](../decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)).
  Conditions are host-rendered state that changes without any desired spec
  changing, so a client MUST NOT carry a previous value forward as if it were
  still true.
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

## Lifecycle

Endpoints under `/apis/forms.takoform.com/v1alpha3`, keyed by group and
kind so one kind name can exist in many groups. `{formGroup}/{formVersion}` is
the two-segment group of [Path shape](#path-shape):

```
GET    {api}/forms?space&group&kind&definitionVersion&schemaDigest
GET    {api}/form-definitions/{formGroup}/{formVersion}/{kind}?…
POST   {api}/resources/validate         diagnostics only, no mutation, no digest
POST   {api}/resources/prepare          short-lived prepare digest for one exact spec
PUT    {api}/resources/{formGroup}/{formVersion}/{kind}/{name}    apply; carries review.prepareDigest
GET    {api}/resources/{formGroup}/{formVersion}/{kind}/{name}
POST   {api}/resources/{formGroup}/{formVersion}/{kind}/{name}/import
POST   {api}/resources/{formGroup}/{formVersion}/{kind}/{name}/observe
DELETE {api}/resources/{formGroup}/{formVersion}/{kind}/{name}
```

`observe` is the lane's only fenced read-only re-observation. There is no
`refresh` operation: v1alpha2 carried both under one contract, which meant two
spellings of one behavior and therefore two ways for hosts to differ. A
v1alpha3 Form never declares the `refresh` capability and a v1alpha3 host
serves no `/refresh` route.

`validate` reports diagnostics without mutating and without minting a
digest; a client MUST NOT describe provider apply-time preparation as
"reviewed in plan". `prepare` binds the exact spec, identity, and fences to
a `prepareDigest` the way v1alpha2 `preview` binds a plan digest;
substitution after prepare fails `invalid_argument` before mutation.

Role rules are wire-enforced: an update to a `revision`-role resource fails
`invalid_argument`; deleting a resource any live relation references fails
`dependency_in_use` (409); deleting a `deployment`-role resource any live
dependent needs fails the same way
([decision 0016](../decisions/0016-the-worker-aggregate-has-one-active-deployment.md));
a resource protected by policy fails `deletion_protected` (409).

## Cross-resource relations

The rules of this section are decided by
[decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md).

A **relation** is one reference from a resource's desired spec to another
resource. Its wire shape is the closed three-member object

```json
{ "apiVersion": "edge.forms.takoform.com/v1alpha1", "kind": "EdgeKVNamespace", "name": "cache" }
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
closed object that requires exactly `apiVersion`, `kind`, and `name` with the
first two `const` is a relation, identified by its JSON Pointer with `*`
standing for an array element (`/worker`, `/versions/*/workerVersion`,
`/kvBindings/*/resource`). A binding-list property additionally carries the
`x-takoform-binding` annotation naming the Binding contract that governs the
references inside it; the exact digest-bound BindingRef is the Definition's own
`acceptedBindings` entry of that name.

### Resolution and UID pinning

On `apply` and `import`, for every derived relation present in the materialized
spec, a host MUST, before any mutation
([decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md)):

- resolve `(space, apiVersion, kind, name)` to a stored resource. Absent fails
  `resource_not_found` (404) and the message MUST name the relation pointer.
  Cross-space references are unrepresentable — a reference carries no space.
- verify that the resolved resource's Form has exactly the referenced group and
  kind, failing `invalid_argument` (400) on mismatch. A well-formed spec cannot
  reach this, because the schema pins both constants; a host that resolved by
  name alone can, and must refuse rather than bind the wrong Form.
- verify the binding contract when the relation is a binding (below).
- record the resolution as
  `{pointer, relation, targetAPIVersion, targetKind, targetName, targetUID, bindingRef?}`
  alongside the resource, where `pointer` is the concrete instance pointer and
  `relation` is the derived pointer it came from.

A host MUST store the **target UID**, not only the name. A name is a label the
client chose and can reuse; the UID is the identity of one incarnation.

### Incarnation change

When a resource is read or observed and a stored relation's target now resolves
to a *different* UID, or to nothing, the source reports `Ready=False` with

- `ExternalChange` — the target name resolves to a different incarnation;
- `DependencyMissing` — the target no longer exists;

and a `hostReason` naming the relation pointer and both UIDs. A host MUST NOT
re-bind the relation automatically: re-resolving the name would make a delete
and re-create of the target invisible and silently point the source at a
resource its author never named. The source stays pinned until it is re-applied,
and re-reading it does not heal the condition.

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
commit time, so a resource that acquired a holder while the operation was
pending survives.

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

## The Worker aggregate

The rules of this section are decided by
[decision 0016](../decisions/0016-the-worker-aggregate-has-one-active-deployment.md).
They are semantics a desired-state schema cannot express — a schema cannot count
the deployments pointing at one worker, read the `/worker` relation of a version
it does not contain, add weights, know which handlers a referenced version
exports, or reach across sibling properties — so they live in the host under
[decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)
and are proven by required conformance checks.

A **Worker aggregate** is one `ModuleWorker` incarnation, the Worker Versions
pinned to it, the one Worker Deployment governing its traffic, and everything
activated against it. Every rule below is decided against the UID the reference
RESOLVED to, never against the worker name a spec spells.

### One active deployment

A `WorkerDeployment` whose resolved `/worker` UID already has one fails
`invalid_argument` (400) before any mutation, on `apply` and on `import` alike.
Re-applying a worker's own deployment is not a second one. Traffic moves by
re-weighting the deployment a worker already has, which is what makes rollback a
re-weighting rather than a mutation of a revision.

### Deployment integrity

Before any mutation a host MUST refuse a deployment whose `versions[]`

- weights a `WorkerVersion` whose stored `/worker` relation targets a different
  worker UID;
- names one `WorkerVersion` twice, by resolved UID — `uniqueItems` rejects a
  duplicated whole entry, so one version split across two different weights is
  schema-valid and still says two things about one revision;
- carries weights that do not sum to exactly 10000 basis points;
- weights a version that is not Ready, or that an accepted delete is already
  removing.

Each is `invalid_argument` (400): the request is well formed but states
something untrue about what will run.

### Attachment gate

Inward activation is admitted against the worker's ACTIVE DEPLOYMENT, not
against any stored version:

| Attachment | Required module handler |
| --- | --- |
| `WorkerCustomDomain` | `fetch` |
| `WorkerCronTrigger` | `scheduled` |
| `QueueConsumer` | `queue` |

EVERY version the deployment weights MUST export that handler, because a request
served by any weighted version has to find it. An absent deployment, or a
weighted version that does not export the handler, fails
`unsupported_capability` (422) before any mutation and the message names what is
missing. A stored version is a history entry, not a running one: gating on it
would admit a cron trigger against code no deployment selects.

### Reverse validation and deletion

The gate holds in both directions.

- An apply that would leave a live dependent unserved fails
  `unsupported_capability` (422) before any mutation. The dependents are a live
  `WorkerCustomDomain` (`fetch`), a live `WorkerCronTrigger` (`scheduled`), a
  live `QueueConsumer` (`queue`), and a live INBOUND service binding — another
  Form's `serviceBindings` entry targeting this worker — which requires `fetch`.
- Deleting a `WorkerDeployment` while any of those four lives fails
  `dependency_in_use` (409). Nothing REFERENCES a deployment, so this is not the
  relation rule above; it is the same statement about a different edge. A host
  fails closed rather than degrading the dependents, and an accepted (202)
  delete re-runs the scan at commit time.

### Worker readiness and inbound service bindings

`worker.service` is provided by the `ModuleWorker` identity and answered by
whatever its active deployment selects, so readiness is a claim about SERVICE:

- `Ready=True` / `Available` only when the worker has an active deployment whose
  every weighted version exports `fetch`;
- `Ready=False` / `Provisioning` when it has no deployment;
- `Ready=False` / `UnsupportedCapability` when its deployment serves no `fetch`.

Both false cases carry a `hostReason` naming the worker and what is missing. A
`module-worker.service` binding to a worker in either state is refused at BIND
time with `unsupported_capability` (422), rather than stored and reported
not-Ready: a stored binding that projects nothing is a declared capability no
host can keep.

The worker is not addressed by any of the mutations that move it between these
three states, so this is a representation rendered from another resource: a host
MUST advance the worker's `metadata.revision`, and MUST NOT move its
`metadata.generation`, when a deployment change flips the condition (see
[Resource identity](#resource-identity)).

### The environment namespace is single

Within one `WorkerVersion`, `vars` keys, `requiredSensitiveVars` entries, and
every binding `name` across every binding list are projected into ONE runtime
environment object, so their union MUST be unique. A collision fails
`invalid_argument` (400) before any mutation, and a client SHOULD refuse it at
plan time so the author sees it without a round trip. The schema cannot state
it: `uniqueItems` compares whole objects, so two bindings agreeing only on
`name` are distinct, and no keyword relates a property's keys to a sibling
array's element member. A host discovers the binding lists from the
`x-takoform-binding` annotation the desired schema already carries.

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

## Long-running operations

Create, update, delete, import, and artifact commit MAY
return `202 Accepted` with an Operation envelope
([`../schemas/operation-v1alpha1.schema.json`](/schemas/operations/v1alpha1/operation.schema.json))
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
exactly one of `result` or `error`. Cancel is honored only for safely stoppable
operations; an already-terminal operation replays its terminal state.
`operation_not_found` (404) and `operation_cancelled` (409) are the closed
outcomes; `deadline_exceeded` (504) reports a host-side deadline.

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

The desired state of a bundle-shaped revision resource is the **manifest
digest and nothing else**: the committed artifact manifest describes the
bytes, so the manifest and the desired spec are never two spellings of the
same facts. A host MUST therefore resolve the referenced manifest before it
mutates anything, on apply and on import alike, and fail closed when

- the digest names no committed manifest the caller's tenant holds —
  `artifact_missing` (404). Resolution is the same per-tenant question the
  artifact read surfaces ask, so a manifest another tenant committed is answered
  exactly as an uncommitted digest is;
- the stored manifest's RFC 8785 canonical digest differs from the referenced
  digest, its document is undecodable, or its `kind` is not the kind the Form
  requires — `artifact_invalid` (400);
- the manifest violates any rule of
  [`../artifact-transport/`](../artifact-transport/index.md), including its
  per-kind exclusivity, its closed media types, and the host's published
  `limits` — `artifact_invalid` (400).

A committed manifest and its blobs MUST remain readable while any resource
references the manifest. Abandoning an unrelated upload session, or
garbage-collecting staged blobs, MUST NOT make a referenced artifact
unresolvable.

### Upload sessions are owned

An upload id is a handle bound to the tenant and principal that started the
session, exactly as an operation id is. Continuing it
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
```

Responses validate against
[`../schemas/host-support-profile-v1alpha1.schema.json`](/schemas/support/v1alpha1/host-support-profile.schema.json).
A profile declares supported exact refs, closed capability subsets
(`supportedEnums`), inclusive ranges (`supportedRanges`), supported binding
contracts, and numeric limits. Price, SKU, region, quota, and commercial
policy MUST NOT appear; those remain Service Offering data outside this API.

## Errors

The closed taxonomy extends v1alpha2 with `rate_limited` (429),
`deadline_exceeded` (504), `operation_cancelled` (409),
`operation_not_found` (404), `dependency_in_use` (409),
`deletion_protected` (409), `artifact_missing` (404), `artifact_invalid`
(400), `unsupported_capability` (422), `migration_required` (409),
`uid_mismatch` (409), `revision_conflict` (412), and `generation_conflict`
(412). Only `resource_busy`, `backend_unavailable`, `rate_limited`, and
`deadline_exceeded` may be `retryable: true`; `Retry-After` is honored when
present. The v1alpha2 codes `resource_version_conflict`,
`interface_identity_ambiguous`, and `interface_instance_ambiguous` do not
exist in this lane: version conflicts split into the revision/generation
pair, and the v1alpha2 interface projection is replaced by exact Interface
contracts ([`../interface-contract/`](../interface-contract/index.md)).
