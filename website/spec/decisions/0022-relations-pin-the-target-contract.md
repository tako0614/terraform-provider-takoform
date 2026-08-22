# 0022 — A host answers for an exact Form, and a relation pins its target's contract

- Status: accepted; rule 2 was amended on 2026-08-09 to say what a resolved
  lookup must ANSWER with (see "An exact identity answers with the Definition's
  own bytes" below), because resolving the right Definition and serving its
  contract turned out to be two promises and the lane only measured the first
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
`../host-api/v1alpha3.md` and
[`../binding-contract/README.md`](../binding-contract/index.md).

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

### An exact identity answers with the Definition's own bytes

*Amended 2026-08-09.* Rule 2 says every lookup RESOLVES the whole identity or
fails closed. It does not say what a resolved lookup answers with, and the two
are different promises. A host MUST serve, for an exact FormRef, the
`desiredSchema` of the installed Definition whose canonical bytes that
`schemaDigest` addresses — every declared default, bound, pattern, and enum,
unchanged.

The gap this closes is in the oracle rather than in any host. The conformance
runner read each Form Definition from the host under test and materialized every
probe spec against what it was served. A host publishing a default of its own
therefore agreed with itself about every byte the runner then sent, stored,
digested, and compared: self-consistent for an entire run, and passing all 112
required checks. The client this lane exists to protect materializes the
NORMATIVE default instead, computes a different `specDigest`, and has every
`prepare` bound to a spec that host does not recognise — a failure with no local
symptom, at the surface a practitioner cannot inspect. An oracle that adopts the
subject's answer as the standard it measures against is the defect this
repository keeps finding, and it was here in its purest form.

It cannot be closed by re-derivation. `schemaDigest` addresses the canonical
bytes of the WHOLE Definition, while the wire serves a subset of it — identity,
display name, description, and `desiredSchema` — so no party holding the digest
can reconstruct the served document and compare. That was read once as "so the
lane cannot check this", and it is the wrong conclusion: decision
[0025](0025-declared-outputs-are-a-typed-contract.md) had already answered the
same problem for the output contract by pinning it IN THE CORPUS, with a
repository test binding those bytes to the installed Definition at the exact
pinned FormRef. This does the same for the desired schema.

- The corpus pins the desired schema of every Form its probes materialize a spec
  against — the ten Edge Family probes — as byte-digested corpus files, and
  `TestCorpusPinsTheFormsOwnDesiredSchema` recomputes them from the installed
  Definition at that exact FormRef, so the pin cannot drift into a private
  contract of the corpus's own.
- `form-definition-exact` compares what a host serves against those bytes, for
  every one of them, and for the synthetic second definition version out of the
  Definition the corpus already pins whole. It previously drove two Forms and
  compared nothing but the echoed identity.
- The runner materializes probe specs from the PINNED schema. It is the half
  that matters: a comparison alone would name the divergence while the run
  continued to measure the host against its own defaults.

Exactly the Forms the runner materializes against are pinned, and no others. A
pin nothing uses is maintenance with no oracle value, and the two synthetic
Forms this lane installs — the second-group `EdgeKVNamespace` and the second
`ModuleWorker` definition version — are driven with a literal empty desired spec
the runner writes itself, so no served schema can influence what is sent for
them. The rule is the one that generalises: a Form whose spec the runner
materializes MUST have a pin, which contract validation requires of every probe,
so a probe added later cannot fall back to believing the subject.

These are proven by the required conformance checks
`two-definition-versions-answer-independently`,
`resource-answers-only-under-its-recorded-form-ref`,
`relation-target-form-ref-verified`, `relation-target-interface-verified`, and
`relation-pin-records-target-form-ref`, against a corpus that pins a SECOND Form
Definition of one kind as exact bytes.

## Consequences

- *(2026-08-09)* The corpus pins ten more artifacts — one desired schema per
  probe Form, byte-digested under `fixtures/` — and `form-definition-exact`
  becomes a comparison rather than an echo. Widening it to every probe also
  found a stale pin nothing else was looking at: the corpus's `WorkerEndpoint`
  `packageDigest` had drifted from the generated registry, because the test that
  binds probe identities to that registry enumerated eight of the ten probes by
  hand. Both the pin and the enumeration are fixed, and the enumeration is now
  the one list the contract keeps.
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

Rejected in the 2026-08-09 amendment:

- **Serve the desired schema's own digest on the Form Definition surface and
  have the client compare.** This is the shape that would need no corpus pin,
  and it is unavailable: the published form-definition response is a closed
  object and its bytes are immutable (decision 0014), so a `desiredSchemaDigest`
  member is rung 4 of that ladder — a new schema generation, minted for one
  comparison the corpus can make today.
- **Pin the WHOLE Form Definition per probe instead of its desired schema.**
  Attractive, because `schemaDigest` would then be re-derivable from the pinned
  bytes and the corpus would verify itself without a repository checkout. It is
  the right shape and the wrong change set: the corpus already pins each Form's
  `outputSchema` inline under decision 0025, and pinning the whole Definition
  beside it would state that contract twice with nothing keeping the two
  agreeing. Folding both into one pinned Definition is a coherent follow-up and
  belongs to its own change.
- **Pin a desired schema for every installed Form rather than every probed
  one.** Rejected as maintenance without oracle value. What a pin buys is a
  comparison and a materialization source; a Form no probe drives supplies
  neither, and the two synthetic Forms this lane installs are driven with a
  literal spec the runner writes itself. The invariant kept instead is the one
  that cannot rot: every Form the runner materializes against has a pin, checked
  when the contract loads.
- **Keep reading the served schema and merely warn on divergence.** Rejected
  because the run would continue measuring the host against defaults the host
  supplied. A divergence named in a report that still says `passed` is the same
  oracle with a footnote.
