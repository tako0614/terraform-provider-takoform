# 0026 — An attachment's claim is decided on canonical, resolved identity

- Status: accepted; amended 2026-08-09 — the tenant half of the claim scope is
  now MEASURED, and the acyclic half of the dead-letter rule is measured for a
  shape other than one chain
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

Two Edge Platform Family attachments claim something outside the resource that
declares them, and a host could decide both claims wrongly while passing every
check the lane had. Neither claim is visible to a desired-state schema: both
documents are valid on their own, and what makes one wrong is the store.

**A hostname was compared as written.** `WorkerCustomDomain.hostname` is a DNS
name, and DNS is case-insensitive with a trailing dot meaning the fully
qualified form of the same name. `API.Example.com`, `api.example.com.` and
`api.example.com` are one hostname to DNS and were three to the host. Worse,
nothing compared them at all: the lane had no uniqueness rule for `hostname`, so
two attachments could serve one name and the host would hold two answers to one
request with no rule choosing between them — the incompleteness
[decision 0008](0008-forms-preserve-service-shape.md) forbids, at the one place
where the world outside the host decides who is right.

**A dead-letter reference could point home.** `QueueConsumer.deadLetterQueue`
could name the consumer's own queue, or close a longer cycle through another
consumer's. [Decision 0020](0020-the-edge-interfaces-state-their-data-and-delivery-model.md)
says a message that exhausts its retries arrives at the dead-letter queue as a
NEW message, with a new identity and its attempt count starting again at 1.
That is the right rule, and it is exactly what makes a cycle unbounded:
`maxRetries` bounds one message's deliveries and nothing bounds the loop. The
platform would build an infinite redelivery on the author's behalf and report
success.

The two are one failure. A host was deciding an attachment's claim on the bytes
a client wrote — a string, and one reference read one hop deep — instead of on
the identity those bytes name.

## Decision

The rules below are normative; the operative text lives in
[`../host-api/v1alpha3.md`](../host-api/v1alpha3.md#an-attachment-s-claim-is-decided-on-canonical-resolved-identity).
Both fail `invalid_argument` (400) before any mutation, on `apply` and on
`import` alike, and are **re-verified when an accepted `202` commits** — the
precedent of [decision 0016](0016-the-worker-aggregate-has-one-active-deployment.md)
rule 9 and [decision 0015](0015-cross-resource-references-are-uid-pinned-relations.md)
rule 10: a claim is a statement about the store, and the store moves between
accept and commit.

### 1. A hostname is canonicalized before it is compared and before it is stored

The canonical form of `WorkerCustomDomain.hostname` is the trailing root dot
removed and every ASCII letter lowercased. Canonicalization happens at the
host's single materialization entry point — the same place declared defaults
are applied, before validation, before the spec digest, before storage and echo
— so three spellings of one name produce byte-identical desired state, the same
`specDigest`, and the same `generation`. That placement is the rule, not an
implementation detail: canonicalizing at the comparison instead would leave the
store holding a spelling, and every later question would be answered about the
spelling rather than about the name.

An internationalized name travels as its **A-label**. This is a refusal, not a
conversion: performing IDNA/UTS-46 mapping inside the host would make the host's
Unicode table version part of the portable contract, so two conforming hosts on
different tables could canonicalize one U-label to two different names — the
same class of privately-decided meaning decision 0019 rejected the compatibility
date for. The Form's `hostname` pattern admits no non-ASCII byte, so a U-label
is refused by the Form's own grammar and the conversion belongs in the author's
tooling, where its Unicode version is the author's choice.

The pattern DOES admit an uppercase letter and a trailing dot, because refusing
a legitimate DNS spelling is a different rule from agreeing on one identity for
it, and only the second is what the lane needs.

### 2. One canonical hostname is claimed by at most one attachment, per tenant

A `WorkerCustomDomain` whose canonical hostname a live `WorkerCustomDomain`
already serves is refused. Releasing the holder makes the claim representable.

Scope is the **tenant**, not the space. Spaces partition one tenant's
resources; DNS does not partition with them, so two spaces claiming one hostname
is the same collision as two resources in one space. Scope is not wider than the
tenant either: what one tenant may claim is a question about who controls the
name, which is authority a Form cannot answer and this contract does not
pretend to.

The tenant is the **authenticated tenant of the request**, and it has to be,
because nothing in the document names one: a reference carries
`{apiVersion, kind, name}`, metadata carries a space, and neither is a boundary.
A host compares the proposed hostname against the live claims of that tenant in
every one of its spaces and against no others — including at commit, where the
tenant is the one the accepted mutation was admitted from rather than whoever
polls the operation.

That the scope must be SUPPLIED is what separates this rule from every other
cross-resource rule in the lane. One deployment per worker, one consumer per
queue, and the dead-letter walk below are all decided on a host-issued uid, and
a uid already names one resource inside one boundary, so those scans cannot
reach past it however the store is arranged. A hostname is a name DNS owns: the
comparison carries no boundary of its own, so a host that answered it out of an
unpartitioned store would enforce the rule host-wide and refuse a name this
decision says another tenant may hold — while naming, in the refusal, a resource
the caller may not read.

### 3. A dead-letter destination never leads back

A `QueueConsumer` whose `deadLetterQueue` resolves to the UID of the queue it
drains is refused, and so is one closing a cycle of any length through the
dead-letter graph.

The graph is over QUEUES: the edge `Q -> D` exists when the consumer of `Q`
declares `D` as its dead-letter destination. Because a queue has at most one
consumer (decision 0020), a queue has at most one outgoing edge, so a host
follows a single path from the proposed destination and asks whether it returns
to the origin. **The walk terminates on any graph shape** for two independent
reasons: it admits each queue UID at most once, and the number of stored
consumers is finite and does not grow while it runs. A cycle a laxer earlier
state left behind therefore ends the walk instead of running it forever.

"Any length" is the load-bearing half. A host that asked only whether the
DESTINATION's consumer points back at the origin refuses the self-reference and
`A -> B -> A` and admits `A -> B -> C -> A`, which is the same infinite
circulation reached one hop later — and it is the shape an author actually
builds, one consumer at a time. Nothing short of following the path decides
this, so the required check drives a three-queue cycle: a corpus that stopped at
two would be passed by the one-hop test.

### The error code

Both refusals are `invalid_argument` (400), and deliberately not a 409.

The lane's closed taxonomy has no `already_exists`, and every 409 in it names a
different fact: `resource_busy` is a concurrent mutation, `import_conflict` is
adoption, `form_identity_conflict` is Form identity, `dependency_in_use` and
`deletion_protected` are deletion edges, `uid_mismatch` is a fence. Reusing one
would make a code mean two things, which is the one thing a closed taxonomy
must not permit.

`invalid_argument` is also the right answer on its own terms, and the lane
already gives it to exactly this shape: one active deployment per worker
(decision 0016 rule 1) and one consumer per queue (decision 0020) are both
"this request is well formed and states something untrue about what will run",
and both are 400. A hostname another attachment serves and a dead-letter queue
that leads home are the same sentence. The message names the holder, or the
cycle, so the author can act on it without a second request.

## Consequences

- A `WorkerCustomDomain` is now a claim on a name rather than a record about
  one. Applying the same hostname twice fails where it used to succeed, which
  is a behavior change for any configuration that was already broken.
- The Terraform provider REFUSES a non-canonical hostname at plan time rather
  than rewriting it. A plan modifier cannot replace a Required attribute's
  configured value, and a Terraform attribute holds the literal string an author
  wrote, so a configuration the host would rewrite would plan one value and read
  back another forever. The diagnostic names the canonical spelling. This is the
  same three-places split decision 0016 rule 6 already uses: the client refuses
  what it cannot represent, the host canonicalizes because it is the only party
  that sees every client.
- Hostname canonicalization is the lane's first host-side rewrite of desired
  state other than declared defaults, and it reuses that machinery exactly, so
  the "effective spec IS the wire spec" property already stated for defaults
  extends to it unchanged.
- A dead-letter chain is now a DAG a host can walk, which is what makes
  "exhausted messages come to rest" a checkable claim rather than an intention.
  Building a two-queue mutual dead-letter pair is unrepresentable; a chain
  A -> B -> C is not.
- Four required conformance checks join the v1alpha3 runner list:
  `custom-domain-hostname-canonicalized`, `custom-domain-hostname-claim-unique`,
  `custom-domain-hostname-claim-stops-at-the-tenant`, and
  `dead-letter-cycle-rejected`. Each drives a configuration a laxer host accepts
  and proves nothing was stored, and each also drives the accepting case so a
  host cannot pass by refusing everything. Three of them drive the SCOPE of the
  rule and not only the rule: the claim check builds a second space with its own
  worker, version and deployment and collides from there while the first space's
  holder is live, in both directions, so a host enforcing uniqueness inside one
  space fails; the tenant check does the opposite and is described below; the
  cycle check closes a three-queue cycle, so a host testing the destination for
  an immediate back edge fails.
- **The tenant half of the claim scope is measured too** *(amended
  2026-08-09)*. The first version of this record proved the SPACE half and
  declined the other, on the reasoning that requiring a host to ACCEPT a
  hostname another tenant serves would decide who controls that name. That
  reasoning was wrong about its own rule. Rule 2 above does not say a host may
  choose; it says the scope "is not wider than the tenant" and that a host
  answering the claim out of an unpartitioned store "would refuse a name this
  decision says another tenant may hold". Leaving the only normative half a
  corpus cannot see to each host's own tests is how a boundary stops being one —
  and this boundary's failure is not an internal detail, it is one tenant
  denying another a DNS name it has every right to claim, on nothing but who
  asked first.
  Nor does measuring it decide who controls a name. Every hostname this corpus
  writes is under `.invalid`, which RFC 2606 reserves precisely so that no
  registry, host, or tenant has any claim to it. There is no control question
  for a host to be answering about such a name, so the only thing a refusal can
  be is a claim scan that reached past the tenant. A host with a control policy
  of its own — domain verification, an allow list — answers a different question
  about different names and is untouched.
  The check therefore drives BOTH polarities against one live pair, because the
  accepting half alone would be satisfied by a host with no claim rule at all: a
  second tenant's claim on the name the first tenant serves must SUCCEED, both
  claims must read back live under two host-issued uids, releasing one must
  leave the other exactly where it was — and a THIRD claim inside the second
  tenant must still be refused. It also reads the refusal, because the message
  names the holder: a host-wide scan would name a resource in another tenant,
  which is the membership oracle the tenant-isolation checks of
  [decision 0028](0028-the-resource-plane-is-tenant-isolated.md) spend a whole
  check removing. So the refusal must name the caller's own holder and must not
  name anyone else's.
- **The acyclic half of the dead-letter rule is measured as a graph, not as a
  chain** *(amended 2026-08-09)*. The cycle check drove three refusals and one
  accepted path, and every destination it ever accepted had in-degree zero and
  no consumer. A host that never walks the graph — refusing only the
  self-reference and any destination that already has a consumer — passed the
  whole check, including the three-queue cycle the check exists for, and would
  then refuse legitimate configurations this decision permits. In-degree is
  unbounded here: a queue has at most one CONSUMER and therefore at most one
  outgoing edge (decision 0020), and nothing bounds how many chains end at one
  queue. The check now closes a diamond as well, so two acyclic chains meeting
  is accepted and the shortcut fails.

## Rejected alternatives

- **Refuse a non-canonical hostname at the host too.** Simpler, and it makes
  uniqueness trivially safe — but it refuses spellings DNS calls correct, so an
  author who pasted a fully-qualified name from a zone file gets an error about
  a dot. Canonicalizing costs one function and answers the same question.
- **Compare hostnames case-insensitively without storing the canonical form.**
  Rejected because the stored value would still be a spelling: two clients
  reading the resource would see different names for one attachment depending
  on who applied last, and `generation` would move on a re-apply that changed
  nothing.
- **Convert U-labels to A-labels in the host.** Rejected above: it imports a
  Unicode table version into a portable contract. It is also strictly larger
  than the problem, since the conversion is deterministic in the author's
  tooling and the result is what the Form's grammar already admits.
- **Make hostname uniqueness per space.** Rejected because it is a rule about
  the host's own bookkeeping rather than about the name. Two spaces are one
  tenant's; DNS answers one of them.
- **Refuse only the self-referencing dead-letter queue.** Rejected because it
  is the case an author is least likely to write and most likely to notice. The
  loop that actually happens is built one consumer at a time, by two people or
  on two days, and only a graph walk sees it.
- **Bound redelivery with a hop counter instead of forbidding cycles.** Rejected
  because it would put a number nobody chose into the delivery model, and
  because a message that legitimately traverses a long chain and one caught in a
  loop would be cut off by the same rule. Refusing the cycle keeps the delivery
  model exactly as decision 0020 states it.
