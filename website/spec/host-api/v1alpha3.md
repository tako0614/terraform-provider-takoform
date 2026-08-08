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
- `status.observedGeneration` names the desired generation the status
  reflects; `status.conditions` uses the closed types `Ready`, `Reconciling`,
  `Degraded`, `Drifted`, `Blocked`, `Deleting` with closed portable reasons
  and an optional non-portable `hostReason`.
- The Form semantic identity is the exact FormRef. The package digest used
  at installation is audit evidence (`form.packageDigest`, optional in
  responses); it never enters queries, fences, or state identity, and a host
  that installed the same FormRef from a different legitimate package MUST
  read and delete the same resources.

## Lifecycle

Endpoints under `/apis/forms.takoform.com/v1alpha3`, keyed by group and
kind so one kind name can exist in many groups:

```
GET    {api}/forms?space&group&kind&definitionVersion&schemaDigest
GET    {api}/form-definitions/{group}/{kind}?…
POST   {api}/resources/validate         diagnostics only, no mutation, no digest
POST   {api}/resources/prepare          short-lived prepare digest for one exact spec
PUT    {api}/resources/{group}/{kind}/{name}    apply; carries review.prepareDigest
GET    {api}/resources/{group}/{kind}/{name}
POST   {api}/resources/{group}/{kind}/{name}/import
POST   {api}/resources/{group}/{kind}/{name}/observe
DELETE {api}/resources/{group}/{kind}/{name}
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
`dependency_in_use` (409); a resource protected by policy fails
`deletion_protected` (409).

## Cross-resource relations

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
spec, a host MUST, before any mutation:

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
replays its terminal state instead of re-executing. A
client can resume across its own restart only if it persists the ID first; no
shipped Takoform client persists one today, so a restart during a long
operation currently means re-reading the resource, not resuming the
operation. A terminal Operation carries exactly one of `result` or `error`. Cancel is honored only for safely stoppable
operations; an already-terminal operation replays its terminal state.
`operation_not_found` (404) and `operation_cancelled` (409) are the closed
outcomes; `deadline_exceeded` (504) reports a host-side deadline.

## Artifacts

The content-addressed upload API is
[`../artifact-transport/`](../artifact-transport/index.md). The artifact
endpoints share the lane's auth, idempotency, and error taxonomy.

The desired state of a bundle-shaped revision resource is the **manifest
digest and nothing else**: the committed artifact manifest describes the
bytes, so the manifest and the desired spec are never two spellings of the
same facts. A host MUST therefore resolve the referenced manifest before it
mutates anything, on apply and on import alike, and fail closed when

- the digest names no committed manifest — `artifact_missing` (404);
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

## Support profiles

```
GET {api}/support/forms
GET {api}/support/forms/{group}/{kind}/{definitionVersion}
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
