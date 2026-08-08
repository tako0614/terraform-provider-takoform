# 0022 — A host answers for an exact Form, and a relation pins its target's contract

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

Two rules in this lane were written as if a Form line held one contract at a
time, and a resource named one thing forever. Neither is true, and the lane had
no way to notice.

**The exact ref stopped at the provider.** Decision
[0011](0011-resource-identity-generation-and-revision.md) settled that a
resource's Form semantic identity is its exact FormRef, and decision
[0017](0017-provider-state-survives-form-evolution-and-interruption.md) reworked
the provider registry around that: a `Supported` map keyed by the whole
`{apiVersion, kind, definitionVersion, schemaDigest}` tuple, separate from the
one `DefaultCreates` entry a NEW resource is created under. The host side of the
same promise was never built. The reference host's catalog was keyed by group
and kind, so `ModuleWorker@0.1.0` and `ModuleWorker@0.2.0` could not be
installed at once, and a stored resource carried a group and a kind rather than
the ref it was created under. `exact-form-ref-fails-closed-on-unknown-definition`
proved the host refuses a definition version it does not have — the only case a
one-contract-per-kind host CAN get right. The case the provider actually
depends on, a host holding two contracts of one line and answering each request
about the right one, was unrepresentable in the lane, so no check could prove it
and a host that matched by kind would have passed.

**A relation pinned an incarnation but not a contract.** Decision
[0015](0015-cross-resource-references-are-uid-pinned-relations.md) made a
reference the closed object `{apiVersion, kind, name}` and made a host store the
resolved target's `metadata.uid`, so a target deleted and re-created is reported
rather than silently adopted. What a host verifies about the target, though, is
only that its Form has the referenced group and kind. Nothing says what that
target must still SATISFY. A `WorkerDeployment` reads its weighted version's
`handlers`; a `WorkerVersion`'s bundle reference is resolved through the
bundle's `manifestDigest`; a `WorkerVersion`'s worker reference is only
meaningful against the ES Module Worker ABI that identity fixes
([decision 0019](0019-the-module-worker-abi-is-an-exact-contract.md)). A target
whose Definition moved to a version that renamed, retyped, or withdrew any of
that would keep satisfying every reference to it, and every source would keep
reporting itself healthy.

Decision [0014](0014-published-schemas-are-structural-minima.md) settled where a
new invariant may live: data the published schema already admits, then
Definition content, then code plus conformance. It authorises putting the
missing statement on the reference node itself, exactly as `x-takoform-binding`
already names the Binding contract governing a binding list. It does not decide
what a host is keyed by, what a relation requires of its target, what a host
stores, or which stable errors either produces. Those are normative contract
decisions, and this repository's `AGENTS.md` requires a decision record for
them.

## Decision

A host answers for one EXACT Form identity at a time, and a relation states —
and a host verifies — what contract its target must satisfy. The rules below are
normative; the operative text lives in
[`../host-api/v1alpha3.md`](../host-api/v1alpha3.md) and
[`../binding-contract/README.md`](../binding-contract/README.md).

1. **The installed catalog is keyed by the whole identity.** A host's installed
   set MUST be keyed by `{apiVersion, kind, definitionVersion, schemaDigest}`.
   The package digest stays out of the key: it is distribution provenance and
   never identity (decision 0011). Two definition versions of one group and kind
   MUST be installable at once and MUST answer independently on availability,
   the Form Definition surface, and the support profile. A host MUST NOT install
   two Definitions that agree on group, kind, and `definitionVersion` while
   differing on `schemaDigest`; a definition version names one set of bytes.

2. **Every lookup resolves by that key, and fails closed.** Availability, Form
   Definition, `validate`, `prepare`, `apply`, `read`, `import`, `observe`,
   `delete`, and the support profile MUST resolve the whole identity or fail
   `form_unknown` (404). A host MUST NOT resolve the group and kind first and
   then compare the rest; it MUST NOT fall back to another definition of the
   same kind. A `definitionVersion` from one contract combined with a
   `schemaDigest` from another names no installed Definition and is
   `form_unknown` like any other.

3. **A resource records the exact ref it was created under.** That ref is
   written at create, carried forward unchanged by every update and import, and
   is the ONLY identity the resource is answered about. A request naming any
   other exact ref addresses no resource: `resource_not_found` (404), never a
   response about the stored one. A response MUST NOT rewrite an older
   resource's recorded ref to a newer one — that would be the host asserting, in
   a representation, that two contracts are the same, which is precisely what
   decision 0017 forbids a provider to assert on its own state.

4. **A resource NAME stays unique per space, group, and kind.** The definition
   version decides what a request is ANSWERED about, never where a resource
   lives. A reference is `{apiVersion, kind, name}` and carries no definition
   version, so a host holding two same-named resources of one kind under
   different contracts could resolve a reference to neither. A create therefore
   still conflicts with a name taken under another contract of the same kind.

5. **Every reference-shaped node states one target contract.** Each reference in
   a Form's `desiredSchema` MUST carry exactly one of

   ```json
   "x-takoform-target-formrefs": [ { "apiVersion": …, "kind": …, "definitionVersion": …, "schemaDigest": … } ]
   ```

   when the relation depends on the target's exact desired contract, or

   ```json
   "x-takoform-required-interface": { "apiVersion": "interfaces.takoform.com/v1alpha1", "name": …, "version": …, "schemaDigest": … }
   ```

   when it depends only on an Interface the target provides. Both are data the
   published Form Definition schema already admits, like `x-takoform-binding`
   (decision 0014). Neither is a new Definition member. A reference carrying
   both, or neither, is refused at derivation: a host that cannot say what a
   relation requires cannot verify it.

   The choice is a statement about the dependency. `x-takoform-target-formrefs`
   is correct when the source — or the host acting for it — reads a member of the
   target's desired spec, or enforces a rule stated over the target Form itself.
   `x-takoform-required-interface` is correct when what the source needs is
   behavior a contract fixes and any Form providing it would serve; pinning the
   Form there refuses a legitimate target for a requirement the source does not
   have.

6. **A binding's annotation is its contract's `targetInterface`.** A binding-list
   reference MUST require exactly the Interface its Binding Definition names as
   `targetInterface`. The Binding Definition stays the authority and the
   annotation is its projection onto the reference, so the two cannot become two
   sources of truth (decision 0014). A binding relation is verified against its
   Binding Definition first, in the published order of decision 0015 rule 8, and
   against the annotation second.

7. **Resolution verifies the annotation before mutation, and pins both.** On
   `apply` and on `import`, before ANY mutation, a host MUST verify the resolved
   target against the annotation: recorded under one of the listed exact
   identities, or declaring the required Interface in its Definition's
   `providedInterfaces` at the exact digest. A target that does not satisfy it
   fails `invalid_argument` (400) — the request is well formed and states
   something untrue about the resource it points at, exactly like the binding
   rules 3 through 5 — and the message MUST name the relation pointer and what
   was required. The stored relation record MUST then carry BOTH the target's
   `metadata.uid` and the target's exact FormRef. The UID pins which incarnation;
   the ref pins what contract that incarnation satisfies.

8. **A moved contract is reported the way a moved incarnation already is.** When
   a stored relation's target no longer matches its recorded ref, the source
   reports `Ready=False` with `ExternalChange` and a `hostReason` naming the
   relation pointer, the recorded ref, and the current one — the same condition,
   the same closed reason, and the same remedy as a changed UID (decision 0015
   rule 7). A host MUST NOT re-bind, and a read MUST NOT heal it. There is no
   second drift mechanism and no new error code or condition reason: both
   published enums are closed.

These are proven by the required conformance checks
`two-definition-versions-answer-independently`,
`resource-answers-only-under-its-recorded-form-ref`,
`relation-target-form-ref-verified`, `relation-target-interface-verified`, and
`relation-pin-records-target-form-ref`, against a corpus that pins a SECOND Form
Definition of one kind as exact bytes.

## Consequences

- The conformance corpus grows to 84 required checks and pins one more artifact:
  a byte-digested second Form Definition of the `ModuleWorker` line. It differs
  from the first in exactly two places, and both are load-bearing — the
  `definitionVersion`, which makes it a second contract at all, and the absent
  `providedInterfaces`, which gives a required-interface relation a live target
  of exactly the right group and kind that truthfully does not satisfy it.
  Inventing that second contract at runtime would have proven only that the host
  agrees with whatever the runner made up.
- The Edge Platform Family's fourteen relations split eleven Interface
  requirements to three exact-Form ones. The three are the relations that read
  their target: a Worker Version's `/bundle` (the host resolves the bundle's
  `manifestDigest` to learn what the version runs), a deployment's
  `/versions/*/workerVersion` (the host reads the version's `handlers` and its
  own `/worker` relation), and a deployment's `/worker` — the one reference in
  the family that WRITES to its target, because a deployment is what renders a
  Module Worker's readiness and the aggregate rules of decision 0016 are stated
  over that Form rather than over any Interface it provides.
- A host may now advance a Form line without a migration on its own side, which
  is what makes decision 0017's promise real from both ends: the provider keeps
  every recorded ref addressable, and the host keeps answering for each of them.
- The host's "which runtime ABI do I implement" question can no longer be
  answered by looking up a kind. It is the one contract every installed Form
  providing `worker.runtime` agrees on; zero providers, or two different ones,
  fail closed rather than pick.
- `formpackage`'s sensitive-field-name policy gains a closed four-entry
  exemption for the reviewed `x-takoform-*` annotations, because
  `x-takoform-target-formrefs` uses `target` in its portable sense — the resource
  a relation points at — while the policy exists to keep author-declared fields
  from naming backend placement authority.

## Rejected alternatives

- **Key the host catalog by group and kind.** This is what the reference host
  did, and its cost is the whole gap. One Form line holds one contract, so the
  day a definition version moves, every resource already recorded under the
  previous one becomes unaddressable at once — and worse, a host that answered
  such a request by kind would answer it SUCCESSFULLY, describing the resource
  under a contract it was never applied under, with a spec validated against a
  schema it was never written for and a client's state unable to detect any of
  it. The lane could not even express the failure: with one contract per kind
  installed, "answers for the exact ref" and "answers for the kind" are the same
  behavior, so the corpus was structurally incapable of failing a host that got
  this wrong. That is the same defect decision 0017 rejected on the provider
  side, left standing on the host side.
- **Pin only the target UID.** This is what decision 0015 stored, and it catches
  exactly one thing: a target that was deleted and re-created. A target that
  MOVED — same resource, same incarnation, same UID, a Definition that advanced
  to a version with a different field set — passes every check, because group and
  kind still match and the UID never changed. The source keeps reporting itself
  healthy while the host reads a member of the target's spec that may no longer
  mean what it did, or projects an Interface the target no longer provides. The
  UID answers "is this the same thing"; nothing answered "is it still the thing I
  needed".
- **Declare target contracts as a new Form Definition member.** Blocked and
  wrong. The published Form Definition schema admits no such member and its
  bytes are immutable (decision 0014), so it would require minting a replacement
  identity for a correction to unpublished Forms. And it would put the declared
  list and the desired schema in disagreement-capable positions for the same
  facts: the schema would still decide what a valid reference looks like, while a
  sibling member decided what its target must be, with nothing keeping a
  reference and its entry aligned. The annotation lives ON the reference it
  describes, so there is exactly one place to look and no way to describe a
  reference that is not there.
- **Let a name hold two resources, one per definition version.** Rejected
  because a reference is `{apiVersion, kind, name}` (decision 0015) and carries
  no definition version, so a host holding both could resolve a reference to
  neither. Ambiguity would have to become a new failure mode on the one path that
  must stay decidable, in exchange for a store shape nothing needs.
- **Report a moved target contract with its own condition or error code.**
  Rejected because the closed condition-reason and error-code vocabularies are
  published, and because the fact is the same one `ExternalChange` already names:
  the source is pinned to something that is no longer there, and the remedy is
  the same re-apply. A parallel mechanism would make a client that already
  renders relation drift wrong by omission.
- **Verify the annotation after the mutation, on read.** Rejected for the reason
  every pre-mutation rule in this lane exists: a host that stored the relation
  first would have written a record whose every later dependency and drift
  decision is unsound, and the remedy would be to delete something the client
  believed it had created.
