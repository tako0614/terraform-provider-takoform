# 0011 — Resource identity: UID, generation, and revision

- Status: accepted; the DELETE fence was amended on 2026-08-09 (see "A delete
  fences on the generation" below), and a replay record's LIFETIME was stated the
  same day (see "A replay record does not outlive the incarnation it reports"),
  because decision
  [0016](0016-the-worker-aggregate-has-one-active-deployment.md) rule 9 made a
  derived rendering move the revision and a teardown moves its own, and with it
  the `Idempotency-Key` composition (see "An operation's Idempotency-Key names
  the incarnation"), because neither counter ever named an incarnation; and
  ADOPTION was given an identity of its own the same day (see "An adoption
  claims the native identity it names"), because until then nothing in this lane
  distinguished an import from a create
- Date: 2026-08-06
- Owners: Takoform maintainers

## Context

The v1alpha2 wire duplicates naming and versioning concerns: `metadata.name`,
`spec.name`, a provider `name` attribute, `output.name`, `observed.id`, and
`output.id` coexist; `resourceVersion` acts both as the desired-state
generation and as the strong ETag for the whole representation, even though
observe/refresh legitimately change status without changing desired state.
Form Definitions also each re-declare envelope facts (`observed.ready`,
`observed.generation`, `output.id`, portability markers) that belong to the
protocol, not to any one Form. Finally, provider state keys resources by
`packageDigest` in addition to FormRef, so the same resource read through a
different — equally valid — package of the same Form becomes unaddressable.

## Decision

The v1alpha3 resource envelope owns identity and versioning; Form schemas own
only portable desired fields.

- `metadata.name` is the only name. `spec.name` and per-Form envelope fields
  (`/name`, `observed.id`, `observed.generation`, `observed.ready`,
  `output.id`, `output.name`, and similar) are removed from Form Definitions.
- `metadata.uid` is a host-issued immutable identity. Deleting and recreating
  a same-named resource yields a different UID. Mutations may carry an
  expected UID and are rejected on mismatch.
- `metadata.generation` (canonical decimal string) increments only when the
  portable desired spec changes.
- `metadata.revision` (canonical decimal string) increments whenever the full
  representation — including status and outputs — changes. The HTTP strong
  ETag is the quoted revision.
- `status.observedGeneration` states which desired generation the status
  reflects. `status.conditions` uses the closed set `Ready`, `Reconciling`,
  `Degraded`, `Drifted`, `Blocked`, `Deleting` with closed portable reasons;
  host-internal codes go to a separate `hostReason`.
- Desired-state mutations fence on the expected generation; representation
  fences use `If-Match` on the revision. Stale fences fail with
  `generation_conflict` (412) or `revision_conflict` (412).

### A delete fences on the generation

*Amended 2026-08-09.* A **delete** is a desired-state mutation — it withdraws
the desired state entirely — so it fences on the expected generation like every
other one, and a stale fence fails `generation_conflict` (412). The fence is
REQUIRED: an unfenced delete names no incarnation and is refused
`invalid_argument` (400). `If-Match` on the revision stays available to a client
that means "remove exactly the representation I read", and a host that is given
one MUST honor it — but a host MUST NOT require it, and MUST NOT refuse a delete
because the representation moved when no `If-Match` was sent.

The lane originally fenced a delete on the revision alone, which was defensible
while the revision moved only for the resource's own reasons. Decision 0016
rule 9 made it move for OTHER resources' reasons too, and that turned the
defensible rule into a broken one: removing an aggregate means removing the
dependents first, and removing a `WorkerDeployment` re-renders the
`ModuleWorker` whose readiness follows it. By the time the worker's own delete
is issued, its revision has moved — *because of this client's own teardown*,
after the plan that computed the teardown read it. `terraform destroy` of the
official `worker-app` module therefore failed with `revision_conflict`, telling
the author their destroy was stale about a change the destroy itself caused.

What the fence is FOR survives intact. It exists so a client cannot remove an
incarnation it never saw, and the generation says exactly that about desired
state: it moves only when some client changed the spec, which is the only kind
of "it changed under me" a deleter can act on. Identity is `expectedUid`'s job
and always was — a re-created resource starts at generation 1 exactly as it
starts at revision 1, so neither counter ever distinguished incarnations, which
is why the accepted-delete commit path resolves through the recorded uid first.

Two required conformance checks hold a host to it: `delete-generation-fence`
drives all three verdicts, including a delete carrying nothing but a current
generation, which a host demanding `If-Match` fails; and
`delete-fence-survives-derived-rendering` tears down a live Worker aggregate and
proves in one exchange both that the revision genuinely moved and that the
generation fence read before the teardown is still honored at the end.

### An operation's Idempotency-Key names the incarnation

*Amended 2026-08-09, with the fence above.* A client derives the
`Idempotency-Key` of a resource operation from the operation, the Form group and
kind, the name, the space, the resource's `metadata.uid`, the fence, and the
request body digest. The **uid** is the addition, and it is required of a delete
and of an observe, permitted of an update, and empty for exactly the two
operations that address a free name rather than an incarnation — a create and a
new-adoption import.

This is the sentence above, applied to the key rather than to the commit: a
re-created resource starts at generation 1 exactly as it starts at revision 1,
so neither counter ever distinguished incarnations. Moving the delete fence from
one counter to the other therefore moved the flaw rather than introducing it,
and where it landed it was reachable: a key built from a name and a counter is
the same key, with the same request fingerprint, for two resources that only
ever shared a name. Nothing about that is exotic. Delete a name, re-create it,
delete it again, and the second delete is answered from the first one's record
— by design, because a host replays before it resolves the resource a request
addresses, which is what an idempotency record is for. The client is told 204
and the resource is still running. For a `terraform destroy` that is the worst
shape available: the state drops the resource and the host keeps it.

The composition is the general form of what [decision
0015](0015-cross-resource-references-are-uid-pinned-relations.md) rule 10
already requires of a host — an accepted mutation is bound to the incarnation it
was accepted for — asked of the client at the moment it builds the request. It
satisfies the three things a key must do at once. It distinguishes incarnations,
because the uid is host-issued and a re-creation yields a new one. It still
replays, because a uid does not move for one incarnation: the SAME delete
retried after a lost response derives the same key and is answered with the same
terminal result rather than executed twice. And it is derivable from state the
client already holds — a generation is only ever read off a verified
representation and such a representation always carries a uid, so no caller can
name the fence without being able to name the incarnation.

The uid stays out of the request. It is a key component, not a wire fence:
`expectedUid` remains the MAY this record left it, no host behaviour changes,
and the published error taxonomy is untouched.

A **create** cannot carry one, and this is a property of a create rather than an
omission. No incarnation exists when the request is built, and the prepare
binding of such a request pins the create markers — no uid, generation `0`. The
determinism the key does have is load-bearing there: the required check
`apply-idempotency-replay` measures it, and what it measures is the only answer
a client has to a create whose response was lost, since `If-None-Match: *`
alone turns the retry into `already_exists` and hands the operator an import.
What that leaves is a create replayed across an incarnation boundary, and no
client can close it: the discriminator would have to be the deletion the host
performed in between, which is not part of the client's request and not
something the client knows. It closes at the host, and the next section is that
host rule.

### A replay record does not outlive the incarnation it reports

*Amended 2026-08-09.* A host MUST NOT answer a request from a recorded replay
once the incarnation that recording reports as LIVE no longer exists. Such a
record is **retired**: it stops answering, and the request is executed as the new
request it is.

A record is **bound** to one incarnation when the answer it would replay reports
a resource that exists — the `metadata.uid` in the recorded document. A create,
an update, an adoption, and an observe all report one. A record whose recorded
answer reports NO live incarnation is bound to nothing and is retired by nothing
here: that is exactly a successful **delete**, whose `204` reports the
incarnation gone, and every refusal, which created nothing and claims nothing
exists. An accepted `202` carries no uid at accept time, so it is bound to its
**Operation** and follows that operation's outcome: a commit that leaves a
resource live binds the record to that uid, and a commit that removed one, or
failed, leaves it bound to nothing. Nothing else retires a record. Retirement is
by INCARNATION and not by name — a later resource taking the same name retires
nothing — and a host MAY expire records by age or capacity as it always could,
which is a different question.

Both halves are load-bearing and both are wrong on their own. Without
retirement, `terraform destroy` followed by `terraform apply` of an unchanged
configuration is answered the old `201`, reports success, creates nothing, and
never converges: the next read 404s, the plan proposes the create again, and it
replays again. Retire too much — bind a delete's record to what it removed, or
key retirement on the name — and the retried delete of a lost response is
EXECUTED instead of replayed, against whichever incarnation holds the name now.
That is the same destruction the uid-in-the-key composition above closes,
reintroduced by the fix for its sibling.

This changes no wire shape, adds no code to the closed taxonomy, and asks
nothing of a client ([decision 0014](0014-published-schemas-are-structural-minima.md),
rung 3: the host enforces it and a required conformance check proves a laxer
host fails). The required check is
`replay-record-retires-with-its-incarnation`, and it drives all three edges: the
lost-response retry still replays while the resource is present, the re-create
after a delete is a NEW create with a new uid that reads back, the first
incarnation's delete still replays its `204` and leaves the replacement alive,
and the accepted `202` of a byte-identical async create is not the settled
operation again. An Operation's own record is a different thing with a lifetime
of its own ([decision 0017](0017-provider-state-survives-form-evolution-and-interruption.md));
this rule is about the idempotency record and says nothing about polling an id a
client already holds. Publication blocker **V3-013** records the item as P0 for
every Form.

### An adoption claims the native identity it names

*Amended 2026-08-09.* A resource this host ADOPTED has a second identity beside
its uid: the `nativeId` the adoption named — the host's own name for a backend
object that already existed. A host MUST record it on the resource, and MUST
hold at most one live resource per native identity within one tenant.

- An `import` naming a native identity another live resource of the caller's
  tenant already holds fails `import_conflict` (409) before any mutation, and
  stores nothing.
- A recorded claim is IMMUTABLE. An `import` addressing an existing resource of
  the caller under a different `nativeId` fails `import_conflict` (409) the same
  way; re-importing that resource under the identity it already holds is the
  ordinary fenced import and answers the same incarnation. A resource this host
  created holds no claim, so the FIRST import naming it records one — that
  import is the ordinary `terraform import` onto an address a configuration
  already manages, and it is a recording rather than a change.
- The claim is released with its holder and by nothing else: once the resource
  is deleted the identity is adoptable again, and an ordinary update withdraws
  nothing.
- It spans every space of one tenant and every FORM KIND, and STOPS at the
  tenant, which is the same scope, for the same reasons, as the hostname claim
  of decision
  [0026](0026-attachment-claims-are-canonical-and-acyclic.md). Spaces partition
  one tenant's resources and a backend object does not partition with them.
  Neither does a Form kind: an object has one identity whatever Form was pointed
  at it, so a claim keyed by `(tenant, kind, nativeId)` — the shape that falls
  out of keeping the claim in the per-kind table the resource already lives in —
  lets a queue and a KV namespace manage one object between them, which is the
  duplication this rule exists to prevent wearing a second Form. And a host-wide
  claim would answer "somebody already manages that" to a caller who cannot see
  the holder — a membership oracle over every identifier a stranger can guess,
  which decision
  [0028](0028-the-resource-plane-is-tenant-isolated.md) forbids a refusal to
  disclose. Whose account an object lives in is authority this lane does not
  model.
- Every tenant is a caller. The claim binds inside the plane of whichever tenant
  makes the request, so a host that holds it in one tenant and not the others
  has not scoped the rule — it has stopped applying it everywhere the corpus was
  not looking.

Why this is an identity rule and not a nicety: `nativeId` was on the wire, was
REQUIRED, and meant nothing. Everything else the lane asked of `import` — a
minted uid, generation `1`, revision `1`, the full validation gauntlet, the
attachment-claim scan — a plain create also satisfies, so a host whose `/import`
handler was an alias for create passed all 112 required checks. What that host
does to a practitioner is not subtle: `terraform import` mints a NEW backend
object, the object being adopted is orphaned and still running, and the state
file now describes something the operator never provisioned. A second import of
the same object mints a third. None of it is visible until the bill or the
outage.

What makes it observable without breaking the portability boundary is that the
identifier itself is never observed. A native id is host detail
([`../portability-boundary.md`](../portability-boundary.md)) and the published
wire documents are closed (decision 0014), so no response carries one and no
portable author is made to know one — the rules above add no wire member and are
rung 3 of the 0014 ladder. What a client and a corpus CAN see is what a host
holding the claim can no longer do: adopt one object twice, or move a managed
resource onto another object. Those are refusals in the published closed
taxonomy, and `import_conflict` is the code that already meant exactly this and
until now had no producer anywhere in the corpus except the error probe.

This is deliberately weaker than "adoption adopts". The lane cannot prove that
the object named existed before the call, or that the host did not also mint a
fresh one beside it: no portable surface reports either, and a black-box runner
cannot name an object it knows a host already holds without depending on that
host's identifier format — which is the boundary this rule is built to respect.
A smaller true claim beats a larger unverifiable one, and the remainder is
recorded as publication blocker **V3-014**, closing on a real backend rather
than on any check.

The required conformance checks are `import-claims-its-native-identity` and
`import-records-its-native-identity`; the tenant edge is a leg of
`resource-import-is-tenant-isolated`, where the rest of that boundary already
lives. Each is driven at the scope the rule above is written at, because a check
narrower than its rule is passed by a host that implements only the check:
holder and rival are different Form kinds, the release is measured by the kind
that did not hold the claim, both paths onto a first claim are driven — the
resource this host created and the resource adopted into being — and the tenant
leg measures the second tenant taking the identity AND then holding it against
its own next adoption.
- The Form semantic identity of a resource is its exact FormRef. The package
  digest used at creation may be recorded as audit evidence but never enters
  resource identity, queries, or update/delete fences. A host that installed
  the same FormRef from a different legitimate package must read and delete
  the same resource.

## Consequences

- Provider v2.1 family-resource state identity is `space`, `apiVersion`, `kind`, `uid`; a
  JSON-serialized `status.outputs` document (`outputs_json`) replaces the
  string-map outputs; the ETag/generation split removes
  the global client-side mutation mutex.
- Conformance gains UID-stability, delete/recreate UID change,
  generation-only-on-spec-change, revision-on-status-change, stale-fence
  rejection, and package-digest-substitution checks.
- Form authoring gets simpler: definitions stop repeating envelope plumbing.
- *(2026-08-09)* The provider sends no `If-Match` on a delete and renders the
  generation it sent in the failure diagnostic instead of the revision. The
  `revision_conflict` code stays in the closed taxonomy and stays reachable,
  because a client that supplies the optional representation fence is still
  answered with it; what changed is that no ordinary teardown supplies one.
- *(2026-08-09)* A delete and an observe require the recorded `metadata.uid`
  alongside the generation, because the key is derived from it. The provider
  reads both out of the same state entry, so nothing new is asked of an author,
  and a fenced import requires the uid of the incarnation it adopts for the same
  reason. Nothing is added to any request: the uid is a component of the
  `Idempotency-Key` the client already sent.
- *(2026-08-09)* A host keeps one more field per RESOURCE — the native identity
  it was adopted onto — and one tenant-scoped scan over it before an adoption
  mutates anything. A host with a real datastore would more naturally hold it as
  a unique index on `(tenant, nativeId)` — never on `(tenant, kind, nativeId)`,
  which is the natural index and the wrong one, because the row it would key is
  the resource and the thing being claimed is the object; the reference host
  scans, because what the lane measures is the answer. That field also survives
  every update of its resource, which is not a rule of its own so much as the
  reading of "released with its holder" that a host rebuilding its stored record
  from the request document gets wrong for free. The corpus grows to 114
  required checks, and `import_conflict` gains its first organic producer.
- *(2026-08-09)* A host that records replays keeps one more field per record —
  the incarnation the recorded answer reports, or the Operation it handed back —
  and consults it before replaying. The reference host stores it as
  `replayBinding` and resolves the uid against its own store; a host with a real
  datastore would more naturally cascade the retirement from the deletion. Either
  is conforming, because what the lane measures is the answer.

## Rejected alternatives

- **Keep one `resourceVersion` for both fences.** Rejected because
  observe/refresh then either lies (status changes invisible to ETag) or
  breaks desired-state fencing (every observe invalidates client fences).
- **Key state by `space/name` only.** Rejected because delete/recreate would
  silently rebind old state to a new resource; UID makes that visible.
- **Keep `packageDigest` in identity for provenance.** Rejected because
  provenance is evidence, not identity; it is retained as an audit field.

Rejected in the 2026-08-09 amendment:

- **Keep the revision fence and have the client re-read and retry on 412.**
  Rejected because it does not satisfy the fence, it waives it. A client that
  answers every `revision_conflict` by fetching the current revision and
  deleting under it has built a delete that ignores the precondition with extra
  steps, and it would do so in exactly the case the fence exists for — a
  resource somebody else really did change. It also does not terminate cheaply:
  each remaining dependent moves the parent's revision again, so a teardown of
  an aggregate pays a re-read per delete and races every other client in the
  space. And retry is a CLIENT decision, so two clients would differ on how
  hard a delete tries, which is the divergence a closed contract exists to
  remove.
- **Have the host say WHY the revision moved, and let the client decide.** A
  412 could carry "this moved only because a rendering changed", and a client
  could then retry. Rejected on two counts. The distinction is not the host's
  to draw usefully: a rendering change and a status change are both "the
  representation moved without a desired-state mutation", and a delete has no
  reason to care about either — so the honest version of the distinction is
  exactly the generation, which the lane already has and already serves. And it
  puts the decision in the client anyway, which is the previous alternative with
  a hint attached.
- **Retire a replay record when the NAME it addressed changes hands.** Cheaper
  to implement, because it needs no uid on the record — and it breaks the delete.
  A delete's record is the one that must survive its resource, since the resource
  being gone is what the record says; keying on the name retires it the moment
  anything takes the name back, and the next retry of that delete is executed
  against a resource it was never issued against.
- **Have the client discriminate instead.** Rejected because it cannot. The
  discriminator is the deletion the host performed between the two creates, which
  is not in the request and not something the client knows; the only thing a
  client could add is a nonce, and a nonce is exactly the determinism
  `apply-idempotency-replay` requires it not to add.
- **Require `expectedUid` on a delete instead of any counter.** Attractive,
  because uid is what actually names an incarnation, and rejected as the wrong
  scope for this record: `expectedUid` is a MAY across every mutation of the
  lane, and making it a MUST on one of them is a separate contract change with
  its own consequences for import and for clients that hold a name but not a
  uid. The generation fence is required, the uid fence stays available beside
  it, and the accepted-delete commit path already resolves through the recorded
  uid before it looks at any fence.
- **Echo the `nativeId` back on every representation, so adoption is directly
  observable.** Rejected twice over. It puts a vendor's identifier format in
  front of every portable author and every client that renders a resource, which
  the portability boundary excludes from the Form contract by name; and the
  published `resourceResponse` is closed with `unevaluatedProperties: false`, so
  it is rung 4 of the decision 0014 ladder — a new schema generation minted to
  expose exactly the fact that must not travel. The refusals prove the host holds
  the identity without ever handing it out.
- **Require an `import` of an UNKNOWN native identity to fail.** This is the
  rule that would prove adoption really adopts, and it is not drivable black
  box. The runner would have to name an object it knows the host already has,
  which means either the corpus pinning a host's identifier format — the
  boundary violation above — or the lane defining a provisioning side channel it
  has no business owning. Stated as an obligation and left to V3-014 instead.
- **Scope the claim host-wide rather than per tenant.** Rejected because it is
  the membership oracle decision 0028 exists to close: a stranger who guesses an
  identifier learns whether anyone on the host manages it. The duplication the
  rule prevents is duplication WITHIN one payer's account, which is exactly
  where the claim is scoped.
- **Scope the claim per space.** Rejected because a backend object is not
  partitioned by space. Two spaces of one tenant adopting one object is the same
  double management as two resources in one space, and it is the likelier
  accident: a second workspace importing what the first already manages.
- **Scope the claim per Form kind.** Rejected for the same reason and with less
  excuse, because nothing about a native identity was ever qualified by kind: an
  object has one identity, and which Form a practitioner pointed at it is a fact
  about the configuration rather than about the object. It is nonetheless the
  index a host writes without thinking — the claim goes in the table the
  resource lives in, and that table is per kind — and it is invisible to any
  check that drives holder and rival through one probe, which is why the
  conformance leg drives them through two.
- **Refuse a re-import that names the identity a resource already holds.** It
  would fall out of a host that kept a bare set of identifiers it had seen, and
  it breaks the one thing a client can retry: an import re-run has nothing else
  to send. The rule is about which resource holds an identity, not about how
  many times the identity is mentioned.
