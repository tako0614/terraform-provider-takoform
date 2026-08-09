# 0011 — Resource identity: UID, generation, and revision

- Status: accepted; the DELETE fence was amended on 2026-08-09 (see "A delete
  fences on the generation" below), because decision
  [0016](0016-the-worker-aggregate-has-one-active-deployment.md) rule 9 made a
  derived rendering move the revision and a teardown moves its own, and with it
  the `Idempotency-Key` composition (see "An operation's Idempotency-Key names
  the incarnation"), because neither counter ever named an incarnation
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
something the client knows. It closes at the host — by retiring a record whose
incarnation is gone — and that is a host rule this lane has not stated and this
record does not state.
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
- **Require `expectedUid` on a delete instead of any counter.** Attractive,
  because uid is what actually names an incarnation, and rejected as the wrong
  scope for this record: `expectedUid` is a MAY across every mutation of the
  lane, and making it a MUST on one of them is a separate contract change with
  its own consequences for import and for clients that hold a name but not a
  uid. The generation fence is required, the uid fence stays available beside
  it, and the accepted-delete commit path already resolves through the recorded
  uid before it looks at any fence.
