# 0024 — A worker is reachable at an address the host assigns

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

The Edge Platform Family can describe a complete running worker — an identity,
immutable versions, a deployment that weights them — and still not describe a
worker anyone can send a request to. The family's only inward activation for
`fetch` is `WorkerCustomDomain`, whose desired state requires a dotted DNS
hostname the author owns. Before the first request an author must therefore
have a domain, have delegated it, and have waited for the host to verify it.

That makes the shortest path to a working worker longer than the path to
abandoning it, which is publication blocker V3-007. It also has a quieter cost:
it puts the family's first observable success behind a step this specification
has no control over, so nothing in the lane demonstrates that a portable
desired document produces a portable *service*.

The obvious fix — let an author write a hostname the host will complete — is
the wrong one twice over. It puts the host's naming scheme into desired state,
where two conforming hosts would need the same subdomains for the same document
to work; and it makes the author responsible for a decision they cannot make
correctly, because which names are available is a fact only the host holds.

Decision [0008](0008-forms-preserve-service-shape.md) already says where such a
fact lives: account, region, SKU, credential, native ID, and placement stay
outside the contract. A vendor subdomain is that same class of fact. What is
NOT outside the contract is the address itself, because it is observable: an
author who cannot read the URL cannot use the worker at all. Decision
[0014](0014-published-schemas-are-structural-minima.md) settles where the
missing statements may live, and decision
[0016](0016-the-worker-aggregate-has-one-active-deployment.md) already decides
what "active deployment" means, which is what this Form must invoke rather than
restate. None of them decides the Form's shape, its cardinality, its outputs, or
which stable error a host that cannot serve it returns. Those are normative
contract decisions, and this repository's `AGENTS.md` requires a decision record
for them.

## Decision

The Edge Platform Family gains `WorkerEndpoint`, an `attachment`-role Form whose
desired state is one Module Worker reference and nothing else. The rules below
are normative; the operative text lives in
[`../host-api/v1alpha3.md`](../host-api/v1alpha3.md) and
[`../form-families.md`](../form-families.md).

1. **The author asks for reachability; the host decides the address.** The
   desired schema declares exactly `worker`. There is no hostname, no
   subdomain, no region, no plan, and no vendor label, because every one of them
   is either a host fact (decision 0008) or a request the author cannot state
   correctly.

2. **The address is observable and therefore published.** A host returns
   `status.outputs` carrying `hostname` — the DNS name it assigned — and `url`,
   the absolute address of the path root. `url` is exactly
   `https://` + `hostname` + `/`. The scheme is HTTPS and TLS is not optional; a
   plaintext address is a different promise from the one this Form makes. There
   is no port and no deeper path: a consumer composes onto `url` rather than
   reconstructing an origin.

3. **A portable author may rely on exactly three things**: that a value comes
   back, that it is HTTPS, and that it routes to the worker's ACTIVE DEPLOYMENT.
   The SHAPE of the address is host detail. Which label, which subdomain, which
   apex, how long, and whether it resembles the resource name are decisions the
   host makes, and a configuration that parses the hostname, asserts a suffix,
   or reconstructs the value from the resource name has written a fact about one
   host into a portable document. The conformance lane holds hosts to the three
   guarantees and deliberately asserts nothing about the shape — including the
   shape it might be tempted to forbid. That the address is the HOST'S is proved
   by the endpoint's desired state carrying no address at all: a host that
   answers with one has answered with something no request supplied, whatever
   it looks like. A host that allocates `endpoint-probe.tenant.example`
   conforms, and a lane that failed it would be enforcing its own taste in
   labels. What the lane does assert about the value follows from the third
   guarantee rather than from any shape: two endpoints on two workers publish
   two DIFFERENT addresses, because one address at one path root cannot invoke
   the active deployments of two workers.

4. **It invokes the active deployment.** The endpoint is an inward activation
   for `fetch`, gated exactly as the other three attachments are: every version
   the worker's active deployment weights must export `fetch`, and an absent
   deployment or a missing handler fails `unsupported_capability` (422) before
   any mutation. "Active deployment" is decision 0016's, not a second
   definition: the endpoint holds no version reference of its own, so promotion
   and rollback move what answers without the endpoint being re-applied and
   without its address changing.

5. **At most one per worker, enforced in the host.** A second `WorkerEndpoint`
   whose resolved `/worker` UID already has one fails `invalid_argument` (400)
   before any mutation. It is a host uniqueness rule rather than a Form-level
   cardinality statement because a desired schema cannot express it: one
   endpoint's document says nothing about any other, and counting the endpoints
   pointing at one worker is a query over the store. Placing it in the host is
   the only placement that cannot be bypassed — the same reasoning, and the same
   placement, as one-deployment-per-worker and one-consumer-per-queue.

6. **A host that cannot assign an address refuses the Form.** When a host
   supports `WorkerEndpoint` but cannot offer a host-assigned hostname, it MUST
   fail `unsupported_capability` (422) before any mutation and MUST NOT store
   the endpoint or answer with an address it did not assign. The code is the one
   the taxonomy already uses for exactly this: the request is well formed and
   states nothing untrue, and what is missing is a capability of the host.
   `invalid_argument` would blame the author for a document that is correct;
   `form_unavailable` describes an installed Form the host cannot currently
   serve at all, which is a different and larger claim; `backend_unavailable`
   would be retryable and this is not; and `internal_error` would report a
   permanent, stated limitation as a fault. Both published enums stay closed:
   no new error code is minted (decision 0014).

7. **The two delete directions are the lane's existing semantics, and neither
   is new.** Deleting the endpoint never deletes the worker — that is the
   `attachment` role rule of decision
   [0009](0009-form-families-and-namespaced-api-versions.md), enforced
   mechanically for every attachment. And an endpoint never outlives its worker,
   because deleting a `ModuleWorker` while a live `WorkerEndpoint` relation pins
   its UID fails `dependency_in_use` (409) under decision
   [0015](0015-cross-resource-references-are-uid-pinned-relations.md) rule 6.
   That refusal, not a cascade, is what this lane means by the second direction:
   removing a worker is an ordered sequence in which every step names its
   dependent (decision 0016), and the endpoint takes its place in that order.

These are proven by the required conformance checks
`worker-endpoint-address-is-host-assigned`, `worker-endpoint-single-per-worker`,
and `worker-endpoint-follows-the-active-deployment`, and by
`attachment-requires-active-deployment`, which now drives the endpoint probe
alongside the other three attachments.

## Consequences

- The Edge Platform Family has twelve members and fifteen relations. The
  conformance corpus grows to 89 required checks and pins one more probe, whose
  desired state is the worker reference alone — which is what makes an address
  in the response provably the host's rather than an echo of the request.
- The shortest path to a reachable worker becomes: a bundle, a version, a
  deployment, an endpoint. No DNS, no delegation, no verification wait, and
  nothing the specification does not control.
- `WorkerCustomDomain` is unchanged and remains the way to serve a name the
  author owns. The two coexist on one worker: a custom domain says which name
  reaches the worker, and an endpoint says the worker is reachable at all.
- A worker with an endpoint has one more dependent, so a deployment change that
  would stop serving `fetch` is refused while an endpoint lives, and so is
  deleting that deployment. Both follow from the endpoint being an inward
  activation for `fetch` rather than from any endpoint-specific rule.
- Because the address is `status` rather than `spec`, a host that reassigns it
  moves the resource's `metadata.revision` and not its `metadata.generation`,
  like every other representation change (decision 0011).
- The refusal branch of rule 6 is stated normatively and is NOT proven by the
  lane, because a black-box runner cannot take a capability away from the host
  under test. What the lane does close is the failure that branch exists to
  prevent: a host that accepts the endpoint and then publishes an incomplete,
  plaintext, or shared address fails a required check.

## Rejected alternatives

- **Let the author write the hostname and have the host complete it.** Rejected
  because it puts one host's naming scheme into portable desired state. Two
  conforming hosts would have to agree on subdomains for the same document to
  apply, which is exactly the incompleteness
  [`portability-boundary.md`](../portability-boundary.md) forbids — and the
  author would be stating a fact only the host can know.
- **Add an `enabled` boolean to `ModuleWorker` instead of a Form.** Rejected
  because inward activation is an attachment, not a field of the thing being
  activated (decision 0010). A boolean on the identity would also have nowhere
  to publish the address: an identity's outputs would then describe a service
  that its deployment, not it, provides, and deleting the "endpoint" would be a
  spec change to the worker rather than the removal of an attachment.
- **Publish only the hostname, and let consumers build the URL.** Rejected
  because building the URL means concatenating a scheme and a path root onto a
  host-owned value — precisely the reconstruction rule 3 forbids. Every consumer
  would carry the same three lines, and any host that ever needed a different
  composition would break all of them at once.
- **Publish only the URL, and let consumers parse the hostname.** Rejected for
  the mirror-image reason. A consumer that needs the bare name for a DNS record
  or a certificate would parse the URL, which is parsing host detail; publishing
  both costs one member and removes the need.
- **Allow several endpoints per worker.** Rejected because it leaves one service
  with several addresses and nothing saying which is canonical, so every
  consumer picks one by a rule of its own. The author who genuinely wants a
  second address is asking for a name they control, which is
  `WorkerCustomDomain`.
- **State the cardinality in the desired schema.** Blocked and wrong. No
  keyword relates one document to the other documents that reference the same
  target, so a schema could only be made to look like it enforced this while
  enforcing nothing.
- **Cascade the endpoint's deletion when the worker is deleted.** Rejected
  because it converts a refusal that names a dependent into the silent
  destruction of a resource a client holds in state. The lane's whole deletion
  story is that every step is refused with its dependent named, so the order is
  discoverable from the errors (decision 0016); one Form cascading would make
  that story false exactly where an author is least likely to be watching.
- **Report a host that cannot assign an address as not-Ready instead of
  refusing.** Rejected for the reason every pre-mutation rule in this lane
  exists. A stored endpoint that never had an address is a resource whose whole
  purpose is unfulfillable, and the remedy would be deleting something the
  client believed it had created.
