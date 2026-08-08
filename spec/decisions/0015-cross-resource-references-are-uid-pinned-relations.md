# 0015 — Cross-resource references are UID-pinned relations

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

A v1alpha3 host recognised a cross-resource reference by finding an object under
a member literally named `resource`, which is the shape a typed Binding instance
happens to have. Every other reference a Form declares was therefore invisible:
a Worker Version could name a worker or a bundle that does not exist, a
Deployment could weight a Version belonging to a different worker, a Consumer
could target a missing queue, and deleting any of those targets was not blocked.

The reference itself carried only `{kind, name}`. A bare kind forces a host to
guess which installed Form a name means once two Form Families declare the same
kind, and a name alone cannot distinguish one resource from a different resource
that later reused the name — which is exactly what happens when a target is
deleted and re-created out of band.

Decision [0014](0014-published-schemas-are-structural-minima.md) settled where a
new invariant may live: data the published schema already admits, then
Definition content, then code plus conformance. It authorises *deriving*
relation metadata from the desired schema instead of minting a new Definition
member. It does not decide what a reference is, how it is resolved, what a host
stores, what protects a referenced resource from deletion, what happens when a
target's identity changes underneath a source, or which stable errors any of
that produces. Those are normative contract decisions, and this repository's
`AGENTS.md` requires a decision record for them.

## Decision

A **relation** is one reference from a resource's desired spec to another
resource in the same space. The rules below are normative; the operative text
lives in [`../host-api/v1alpha3.md`](../host-api/v1alpha3.md) and
[`../binding-contract/README.md`](../binding-contract/README.md).

1. **Shape.** A reference MUST be the closed three-member object
   `{apiVersion, kind, name}`. `apiVersion` and `kind` MUST be `const` in the
   declaring Form's `desiredSchema`; `name` follows the portable resource-name
   grammar. All three members are required. A reference carries no space member,
   so a cross-space reference is unrepresentable rather than refused.
2. **Derivation.** Relations MUST be derived from the desired schema and MUST
   NOT be declared beside it. Every closed object requiring exactly those three
   members with the first two `const` is a relation, identified by its JSON
   Pointer with `*` standing for an array element. A binding list additionally
   carries the `x-takoform-binding` annotation naming its contract; the exact
   digest-bound BindingRef stays the Definition's own `acceptedBindings` entry,
   so the digest keeps one source of truth (decision 0014).
3. **Resolution before mutation.** On `apply` and on `import`, before ANY
   mutation, a host MUST resolve every relation present in the materialized
   spec and MUST verify that the resolved resource's Form has exactly the
   referenced group and kind.
4. **UID pinning.** A host MUST store the resolved target's `metadata.uid`
   alongside the source, not only the name. A name is a label the client chose
   and can reuse; the UID is the identity of one incarnation.
5. **Absent target.** A relation naming a resource that does not exist MUST fail
   `resource_not_found` (404) before any mutation, and the message MUST name the
   relation pointer.
6. **Deletion protection.** Deleting a resource that any stored relation
   references by UID MUST fail `dependency_in_use` (409). This covers every
   relation, not only typed bindings. A self-reference does not block a
   resource's own deletion, and an accepted (202) delete MUST re-run the scan at
   commit time.
7. **Incarnation change.** When a stored relation's target resolves to a
   different UID, the source MUST report `Ready=False` with `ExternalChange`;
   when it resolves to nothing, with `DependencyMissing`. Both MUST carry a
   `hostReason` naming the relation pointer and both UIDs. A host MUST NOT
   re-bind automatically, and a read MUST NOT heal the condition.
8. **Binding verification order.** A binding relation MUST be verified before
   any mutation, in this order: (1) the source Definition's `acceptedBindings`
   carries the annotated contract, else `invalid_argument` (400); (2) the host
   has installed that contract at exactly the accepted `schemaDigest`, else
   `unsupported_capability` (422); (3) the source Form's role equals the
   Binding's `sourceRole`, else `invalid_argument`; (4) the resolved target's
   exact Form appears in `allowedTargetForms`, else `invalid_argument`; (5) the
   target Form declares the Binding's `targetInterface` in `providedInterfaces`,
   else `invalid_argument`. Rule 2 is about what this host can do at all, which
   is why it alone is a capability failure.
9. **Recovery.** Because a host never re-binds, the remedy MUST be an apply, and
   that apply MUST stay reachable. Every accepted mutation re-resolves and
   re-pins every relation, including one whose spec is byte-identical to the
   stored spec. Re-pinning MUST NOT move `metadata.generation`, because desired
   state did not change, and it moves `metadata.revision`, because the
   representation did. A client MUST report the broken relation without failing
   its refresh — the plan that repairs the resource is computed from the
   refreshed state — and MUST offer an apply: a spec-identical one for a Form
   that declares `update`, and a REPLACEMENT for a Form that declares none,
   since a host refuses every apply to an existing resource of such a Form.

Semantics a schema cannot state stay in the host under decision 0014, and are
proven by the required conformance checks `relation-target-missing-rejected`,
`relation-target-deletion-blocked`, `relation-incarnation-change-detected`,
`relation-reapply-repins`, and `binding-contract-verified`.

## Consequences

- Fourteen relations across the Edge Platform Family are validated by one rule
  instead of one special case per binding list. Enforcing that rule surfaced a
  contract inconsistency no schema could: the service binding allowed a
  `ModuleWorker` target while `worker.service` was declared on `WorkerVersion`,
  so no service binding could ever have been verified.
- A host keeps a reverse index from target UID to holders. That index, not a
  binding scan, is what `dependency_in_use` is computed from.
- The desired spec still carries the NAME. Desired state stays what an author
  wrote; the UID lives in host-owned records, so a spec is portable between
  hosts that issue different UIDs.
- A resource pinned to a replaced target is visibly broken and recoverable, not
  quietly wrong. The provider records the reported reason in a computed
  `relation_drift_reason` attribute so a plan can propose the repairing apply;
  that attribute is provider-side recovery bookkeeping and is deliberately
  absent from the portable wire spec.
- A `revision`-role resource recovers only by replacement. That follows from the
  role — a revision is immutable by definition — and clients must plan it rather
  than attempt an update the host will refuse.

## Rejected alternatives

- **Declare relations as a new Form Definition member.** Rejected twice over.
  The published Form Definition schema admits no such member and its bytes are
  immutable (decision 0014), so it would require minting a replacement identity;
  and it would make the declared list and the desired schema two sources of
  truth for the same facts, free to disagree, with the schema still deciding
  what a valid spec looks like.
- **Pin relations by name only.** Rejected because a name is reusable. A target
  deleted and re-created under the same name would silently adopt every source
  that referenced the old one, and no client would ever learn that the resource
  its author named is gone. Storing the UID is what makes the difference
  observable at all.
- **Re-bind automatically when the target's UID changes.** Rejected because it
  turns a destroyed backend resource into a silent adoption: the source would
  start pointing at a resource its author never named, inheriting whatever state
  that resource holds — another tenant's queue, an empty replacement of a
  populated namespace — while reporting itself healthy. The failure it would
  hide is precisely the one worth reporting.
- **Fail the read while a relation is broken.** Rejected because a read is
  Terraform's refresh, and a failed refresh aborts the plan that would repair
  the resource. It is a remedy that removes its own path.
- **Scan only binding-shaped members, as before.** Rejected because the
  reference shape, not the member name, is what a Form declares; the narrower
  rule protected typed bindings and left every plain reference unresolved,
  unprotected, and undetected.
