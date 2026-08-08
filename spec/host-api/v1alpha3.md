# Host API v1alpha3

`forms.takoform.com/v1alpha3` is the current Host API lane
([decision 0013](../decisions/0013-v1alpha3-lane-ships-in-provider-v2-1.md)).
It carries namespaced FormRef groups, UID/generation/revision resource
identity, long-running Operations, content-addressed artifact upload, and
Host Support Profiles. The retained v1alpha2 contract in
[`README.md`](README.md) continues to govern the frozen provider-v2 lane;
rules stated there and not changed here (space identity, I-JSON raw-first
decoding, idempotency-key namespacing, replay fingerprinting, same-origin
endpoint negotiation) apply to this lane unchanged.

The wire schema is
[`../schemas/host-api-wire-v1alpha3.schema.json`](../schemas/host-api-wire-v1alpha3.schema.json);
the machine operation table is [`operations-v1alpha3.json`](operations-v1alpha3.json).

## Discovery

`GET /.well-known/takoform/v1alpha3` returns a document validating against
[`../schemas/host-discovery-v1alpha3.schema.json`](../schemas/host-discovery-v1alpha3.schema.json):
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
- A resource RECORDS the exact FormRef it was created under. That ref is written
  at create, carried forward unchanged by every update and import, and is the
  only identity the resource is answered about. A `read`, `observe`, `apply`,
  `import`, or `delete` naming any other exact ref addresses no resource and
  fails `resource_not_found` (404) — the Form may well be installed; what is
  absent is a resource of that name under that contract. A response MUST NOT
  rewrite an older resource's recorded ref to a newer one. This is the required
  conformance check `resource-answers-only-under-its-recorded-form-ref`.
- A resource NAME stays unique within one space, group, and kind. The definition
  version decides what a request is answered about, never where a resource
  lives: a reference is `{apiVersion, kind, name}` and carries no definition
  version, so two same-named resources of one kind under different contracts
  would leave every reference to that name unresolvable. A create therefore still
  conflicts with a name taken under another contract of the same kind.

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
  { "apiVersion": "edge.forms.takoform.com/v1alpha1", "kind": "WorkerBundle",
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

Which one is correct is decided by the dependency, not by preference.
`x-takoform-target-formrefs` states the requirement when the source — or the host
acting for it — reads a member of the target's desired spec, or enforces a rule
stated over the target Form itself: a `WorkerVersion`'s `/bundle`, whose
`manifestDigest` a host resolves to learn what the version runs; a
`WorkerDeployment`'s `/versions/*/workerVersion`, whose `handlers` and `/worker`
relation decide what the deployment serves and owns; and a `WorkerDeployment`'s
`/worker`, the one reference that WRITES to its target, because a deployment is
what renders a Module Worker's readiness under
[decision 0016](../decisions/0016-the-worker-aggregate-has-one-active-deployment.md).
`x-takoform-required-interface` states it everywhere else, including every typed
binding and every inward-activation attachment, because what those need is the
behavior a contract fixes and any Form providing it would serve.

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
| `WorkerEndpoint` | `fetch` |
| `WorkerCronTrigger` | `scheduled` |
| `QueueConsumer` | `queue` |

EVERY version the deployment weights MUST export that handler, because a request
served by any weighted version has to find it. An absent deployment, or a
weighted version that does not export the handler, fails
`unsupported_capability` (422) before any mutation and the message names what is
missing. A stored version is a history entry, not a running one: gating on it
would admit a cron trigger against code no deployment selects.

Two attachments carry a further rule the desired schema cannot state
([decision 0020](../decisions/0020-the-edge-interfaces-state-their-data-and-delivery-model.md)).

A `WorkerCronTrigger`'s `cron` MUST be parsed, not merely matched against the
pattern the Form Definition carries. That pattern is the structural minimum a
host holding only the Definition can enforce, and it admits shapes that name no
schedule — `0 24 * * *`, `5-1 * * * *`, `*/0 * * * *`, `0 0 32 * *`. A host MUST
refuse each of them before any mutation, on `validate`, `prepare`, and `apply`
alike, with `invalid_argument` (400), and MUST accept the sub-hourly schedules
the grammar exists for, `*/5 * * * *` and `0 * * * *` among them. This is the
required conformance check `cron-grammar-enforced`.

An `AtLeastOnceQueue` has AT MOST ONE `QueueConsumer`. A second consumer against
the same queue incarnation fails `invalid_argument` (400) before any mutation:
the consumer's `maxRetries`, `retryDelaySeconds`, `maxConcurrency`, and
dead-letter destination are properties of how that queue is drained, so two
consumers would give one queue two of each with no rule deciding which message
got which. The rule is over the queue's UID rather than its name, exactly like
the deployment rule above, and removing the first consumer makes the second
representable. This is the required conformance check
`queue-single-consumer-enforced`.

### An attachment's claim is decided on canonical, resolved identity

Two further attachment rules are about what an attachment CLAIMS rather than
about the worker it activates
([decision 0026](../decisions/0026-attachment-claims-are-canonical-and-acyclic.md)).
Both fail `invalid_argument` (400) before any mutation, on `apply` and on
`import` alike, and are re-raised when an accepted `202` commits: each is a
statement about the store, and the store moves between accept and commit.

A `WorkerCustomDomain`'s `hostname` is **canonicalized before it is compared and
before it is stored**: the trailing root dot removed and every ASCII letter
lowercased. Canonicalization happens at the same entry point as declared
defaults — before validation, before the spec digest, before storage and echo —
so `API.Example.com`, `api.example.com.` and `api.example.com` produce
byte-identical desired state, the same `specDigest`, and the same `generation`.
An internationalized name travels as its **A-label**; the Form's `hostname`
pattern admits no non-ASCII byte, so a host performs no IDNA mapping of its own
and two hosts on different Unicode tables cannot canonicalize one name two ways.
The canonical hostname is then unique **per tenant**: a second
`WorkerCustomDomain` claiming a hostname a live one already serves — in that
space or in any other space of the same tenant — fails `invalid_argument` (400),
and releasing the holder makes the claim representable. Spaces partition one
tenant's resources; DNS does not partition with them, and one hostname has one
answer. The tenant is the AUTHENTICATED tenant of the request, because no
reference and no metadata field names one, and at commit it is the tenant the
accepted mutation was admitted from. A hostname a DIFFERENT tenant serves is
outside the comparison: who controls a name is authority this contract does not
answer. These are the required conformance checks
`custom-domain-hostname-canonicalized` and
`custom-domain-hostname-claim-unique`; the second collides from a second space
of the tenant, against an aggregate of its own, in both directions.

A `QueueConsumer`'s `deadLetterQueue` MUST NOT lead back to the queue the
consumer drains. A destination resolving to the same queue UID is refused, and
so is one closing a cycle through the dead-letter graph of any length: the graph
is over queues, where the edge `Q -> D` exists when the consumer of `Q` declares
`D`. An exhausted message arrives at its dead-letter queue as a NEW message with
its attempt count starting again at 1, so a cycle is a loop `maxRetries` cannot
bound — the platform would build an infinite redelivery for the author. Because
a queue has at most one consumer it has at most one outgoing edge, so a host
follows a single path; the walk admits each queue UID once, so it terminates on
any graph shape, including a cycle a laxer state left behind. Any length means
any: a host that asks only whether the destination's own consumer points back
admits `A -> B -> C -> A`, so the required conformance check
`dead-letter-cycle-rejected` closes a three-queue cycle and accepts a
four-queue chain.

### The host-assigned endpoint

The rules of this section are decided by
[decision 0024](../decisions/0024-a-worker-is-reachable-at-a-host-assigned-address.md).

A `WorkerEndpoint` makes one worker reachable over HTTPS without a
customer-owned domain. Its desired state is the worker reference and NOTHING
else: the author asks for reachability, and the address is the host's decision,
in the same class as an account, a region, and a vendor subdomain
([decision 0008](../decisions/0008-forms-preserve-service-shape.md)).

- A host returns `status.outputs` carrying `hostname`, the DNS name it assigned,
  and `url`, which is exactly `https://` + that hostname + `/`. The scheme is
  HTTPS and TLS is not optional; there is no port and no deeper path. The
  assigned `hostname` is in CANONICAL form — lowercase, no trailing root dot —
  because a name a host produced has no earlier spelling to preserve; the two
  members are held to one grammar, so a hostname no `url` could be built from is
  not representable.
- A portable author may rely on three things and no others: that a value comes
  back, that it is HTTPS, and that it routes to the worker's ACTIVE DEPLOYMENT.
  The SHAPE of the address — which label, which subdomain, which apex, how long,
  whether it resembles the resource name — is host detail, so a configuration
  MUST NOT parse the hostname, assert a suffix, or reconstruct either value from
  anything else it knows. Nothing measures the shape either: an address that
  resembles the resource name is conforming, because the endpoint's desired
  state carries no address for a host to have echoed.
- Two endpoints on two workers carry two DIFFERENT addresses. It follows from
  the third guarantee rather than from any rule about the shape: one address at
  one path root cannot invoke the active deployments of two workers, so a host
  answering both with the same value has made the guarantee false for one of
  them.
- The endpoint holds no version reference, so promotion and rollback move what
  answers without the endpoint being re-applied and without its address
  changing. "Active deployment" is the one this document already defines.
- A worker has AT MOST ONE endpoint. A second `WorkerEndpoint` whose resolved
  `/worker` UID already has one fails `invalid_argument` (400) before any
  mutation, on `apply` and `import` alike. The rule is over the worker's UID and
  lives in the host because a desired schema cannot count the endpoints pointing
  at one worker, exactly like the one-deployment and one-consumer rules above.
- A host that supports the Form but cannot offer a host-assigned hostname MUST
  fail `unsupported_capability` (422) before any mutation. It MUST NOT store the
  endpoint, and it MUST NOT answer with an address it did not assign: the whole
  point of the Form is that the returned address is reachable.
- Deleting the endpoint never deletes the worker (the `attachment` role rule),
  and deleting the worker while a live endpoint pins it fails
  `dependency_in_use` (409) like every other relation. An endpoint therefore
  never outlives its worker, by refusal and ordering rather than by cascade.

These are the required conformance checks
`worker-endpoint-address-is-host-assigned`, `worker-endpoint-single-per-worker`,
and `worker-endpoint-follows-the-active-deployment`.

### Reverse validation and deletion

The gate holds in both directions.

- An apply that would leave a live dependent unserved fails
  `unsupported_capability` (422) before any mutation. The dependents are a live
  `WorkerCustomDomain` (`fetch`), a live `WorkerEndpoint` (`fetch`), a live
  `WorkerCronTrigger` (`scheduled`), a live `QueueConsumer` (`queue`), and a
  live INBOUND service binding — another Form's `serviceBindings` entry
  targeting this worker — which requires `fetch`.
- Deleting a `WorkerDeployment` while any of those lives fails
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
`desiredSchema`, and its bytes are immutable
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
([`../schemas/operation-v1alpha1.schema.json`](../schemas/operation-v1alpha1.schema.json))
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

A `202` accepts a mutation to **one resource**, and the commit is bound to that
one ([decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md)).
A host records the target's exact `formRef` and `metadata.uid` beside the fence
the mutation was accepted under, and at commit time resolves through that record
rather than re-deriving a target from the name the request addressed. A name is
unique per kind and reusable, and a re-created resource starts at revision 1, so
a target removed out of band and re-created — under the same contract or under
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
[`../artifact-transport/`](../artifact-transport/README.md). The artifact
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
  [`../artifact-transport/`](../artifact-transport/README.md), including its
  per-kind exclusivity, its closed media types, and the host's published
  `limits` — `artifact_invalid` (400).

A committed manifest and its blobs MUST remain readable while any resource
references the manifest. Abandoning an unrelated upload session, or
garbage-collecting staged blobs, MUST NOT make a referenced artifact
unresolvable.

### A bundle's modules are what the runtime can load

The module media types a `WorkerBundle` manifest admits are exactly the
LOADABLE set `worker.runtime@1.0.0` imports —
`application/javascript+module`, `text/plain`, `application/octet-stream`,
`application/wasm` — plus the AUXILIARY set a bundle carries and the graph
never imports, today `application/source-map+json` alone. `application/json`
is in neither: this ABI version loads none, and the published manifest enum
never admitted one
([decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).

A host MUST refuse a `WorkerBundle` whose `mainModule` names an auxiliary
module, before commit and before any mutation that references the manifest,
with `artifact_invalid` (400) — and MUST NOT refuse a bundle merely for
CARRYING one. The published manifest schema states the union in one enum and
cannot relate `mainModule` to the media type of the module it names, so the
split is host-enforced under
[decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)
and proved by the required conformance check `bundle-main-module-is-loadable`.
The corresponding runtime obligation — an import resolving to an auxiliary
module fails `unsupported_media_type` — is behavior no desired-state runner
observes, and stays a host obligation with the rest of the ABI.

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
[`../schemas/host-support-profile-v1alpha1.schema.json`](../schemas/host-support-profile-v1alpha1.schema.json).
A profile declares supported exact refs, closed capability subsets
(`supportedEnums`), inclusive ranges (`supportedRanges`), supported binding
contracts, and numeric limits. Price, SKU, region, quota, and commercial
policy MUST NOT appear; those remain Service Offering data outside this API.

A host that supports the Edge Platform Family's `ModuleWorker` MUST advertise
the ES Module Worker runtime ABI contract `worker.runtime@1.0.0` at the exact
`schemaDigest` that Form's `providedInterfaces` names, and MUST advertise the
`WorkerVersion` `handlers` enum as exactly the handler vocabulary that contract
defines. It MUST NOT advertise a `compatibilityDate` range or a
`compatibilityFlags` enum: runtime behavior is stated by implementing an exact
contract, not by a token no registry interprets
([decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).
A `WorkerVersion` declaring a handler the runtime contract does not define MUST
be refused before any mutation, and so MUST a `WorkerVersion` declaring a
handler the main module of the `WorkerBundle` it references does not export:
`loadModule` fails that version with `handler_not_exported` before any traffic
arrives, so storing it would leave the attachment gate above admitting a cron
trigger or a queue consumer against a handler that does not exist. The first is
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
  and supports the exact `ModuleWorker` Form line that provides it;
- the `WorkerVersion` `handlers` enum it advertises is exactly that contract's
  vocabulary, and it advertises no `compatibilityDate` range or
  `compatibilityFlags` enum;
- a version declaring a handler outside the vocabulary is refused, and one
  declaring only defined handlers is still accepted;
- a version declaring a handler its referenced module does not export is
  refused against a corpus-pinned bundle, and one declaring only exported
  handlers is still accepted;
- every inward-activation attachment is gated on the handler its events invoke,
  in both directions;
- a `WorkerEndpoint` is answered with a complete HTTPS address the host
  assigned, that address survives a promotion of the worker it serves, and a
  second endpoint against one worker is refused.

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
- that a request to a `WorkerEndpoint`'s published address actually ARRIVES at
  the worker. The lane drives desired state and sends no traffic, so what it
  proves is that a host answers with a complete HTTPS address it assigned and
  keeps that address stable across a promotion — not that anything is listening
  on it. Nor can it prove the refusal branch: a black-box runner cannot take the
  address-assignment capability away from the host under test, so
  `unsupported_capability` (422) for a host that cannot assign one is stated
  normatively and left to that host to honor.

One further obligation is not merely unproven here but UNMEASURABLE anywhere
today, and is listed rather than left implied:

- the `tail` handler. The contract declares it, and no resource in this family
  activates one: the three inward-activation attachments above are gated on
  `fetch`, `scheduled`, and `queue`. No deployment a portable author can write
  causes a host to invoke `tail`, so no conformance run can observe it. The
  recommended remedy is to remove it from the ABI and re-add it with the
  attachment that makes it observable; until then the runtime corpus carries it
  as an explicitly unmeasured entry and blocker **V3-011** records the decision
  ([decision 0023](../decisions/0023-the-runtime-abi-is-measured-separately-from-the-control-plane.md)).

Those are stated normatively in the contract's own descriptions and proven by
its behavior fixtures, which a runtime conformance run executes against a real
isolate ([`../interface-contract/`](../interface-contract/README.md)). That run
is [`../../conformance/runtime-abi-v1/`](../../conformance/runtime-abi-v1/contract.json),
whose runner drives a worker deployed from its own byte-pinned bundle and a
disposable adapter over the runtime's module loader; it is a separate corpus,
runner, and command from this lane precisely because the subjects differ
([decision 0023](../decisions/0023-the-runtime-abi-is-measured-separately-from-the-control-plane.md)).
A host that passes this lane has not thereby proven it implements the ABI; it
has proven it says which ABI it implements and holds desired state to it.

The same split covers the four data-plane contracts —  `edge.kv`,
`edge.objects`, `edge.sql`, and `edge.queue` — because this lane drives desired
state and never moves a byte of application data
([decision 0020](../decisions/0020-the-edge-interfaces-state-their-data-and-delivery-model.md)).

Proven by required checks:

- the host advertises each of the four contracts at the EXACT pinned
  `schemaDigest`, which is what turns "this host has a KV store" into a
  statement about a consistency model, a value model, an error vocabulary, and a
  set of limits (`edge-interface-contracts-advertised`);
- a `WorkerCronTrigger` whose expression is a shape rather than a schedule is
  refused, and the sub-hourly schedules the grammar exists for are accepted
  (`cron-grammar-enforced`);
- a second `QueueConsumer` against one queue is refused, and the same consumer
  is accepted once the first is gone (`queue-single-consumer-enforced`);
- a `WorkerCustomDomain` written in any spelling of one DNS name is stored under
  the canonical one, and a second attachment claiming it — in either spelling —
  is refused while the first lives and accepted once it is gone
  (`custom-domain-hostname-canonicalized`,
  `custom-domain-hostname-claim-unique`);
- a `QueueConsumer` whose dead-letter destination resolves to its own queue, or
  closes a cycle through another consumer's, is refused, and an acyclic
  destination is accepted (`dead-letter-cycle-rejected`).

Obligations a conforming host MUST meet that this lane does NOT prove, because
proving them means exercising the data plane rather than driving the Host API:

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
- **`edge.sql` losslessness and atomicity.** That a 64-bit INTEGER and a BLOB
  round-trip unchanged, that an out-of-range integer is refused, that a writing
  statement submitted through `query` is refused, and that a failed transaction
  applies nothing.
- **`edge.queue` delivery.** That a `messageId` is stable across redeliveries,
  that `attempts` counts them, that the first delivery does not count toward
  `maxRetries`, that an uncaught handler exception retries every message not
  already acknowledged, and that a dead-lettered message arrives as a new
  message with `attempts` starting again at 1. All of it needs a consumer and
  the passage of time, neither of which a desired-state runner has.

Those are stated normatively in each contract's own descriptions and proven by
its behavior fixtures wherever a fixture can prove them.

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
contracts ([`../interface-contract/`](../interface-contract/README.md)).
