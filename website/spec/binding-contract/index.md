# Typed Binding contracts (bindings.takoform.com/v1alpha2)

A Binding is a digest-bound contract that grants one consumer resource a
typed capability on one target resource: the runtime API and the permission
arrive together, and credentials never cross the boundary. Bindings replace
the generic `connections` / `permissions` / `projection` surface of the
withdrawn Host API v1alpha2 lane
([decision 0010](../decisions/0010-exact-interface-and-binding-contracts.md)).

## BindingRef

```json
{
  "apiVersion": "bindings.takoform.com/v1alpha2",
  "name": "module-worker.edge-kv",
  "version": "1.0.0",
  "schemaDigest": "sha256:..."
}
```

`name` uses the dotted grammar; `schemaDigest` binds the canonical Binding
Definition bytes. The normative shapes are
[`../schemas/binding-ref-v1alpha2.schema.json`](/schemas/bindings/v1alpha2/binding-ref.schema.json)
and
[`../schemas/binding-definition-v1alpha2.schema.json`](/schemas/bindings/v1alpha2/binding-definition.schema.json).

## Binding Definition

A Binding Definition fixes, as data only:

- `sourceRole`: the ONE Form role allowed to hold this binding. Enforcement is
  equality against the source Form's declared role, and nothing else: the
  Definition decides which role that is, and no rule elsewhere narrows the
  choice. Every binding of the current family happens to declare `revision`,
  because that family puts outward capability on revisions — a fact about that
  family's shapes, not a constraint this contract imposes;
- `targetInterface`: the exact InterfaceRef the target must provide;
- `allowedTargetForms[]`: exact Form kinds (by group and kind) this binding
  may point at;
- `runtimeProjection`: the API surface projected into the consumer at run
  time, described against the target Interface's operations, including
  which operations the binding exposes (`accessModes` when the binding
  supports restricted modes);
- `description`: the concrete surface the consumer's code actually calls. A
  list of operation names is not a runtime API — it does not say what the
  caller writes, what it passes, what it gets back, what a callee failure looks
  like, or whether a body streams. Every Binding Definition in the Edge Platform
  Family therefore states its projection in prose: `module-worker.service`
  projects `env.NAME.fetch(request) -> Promise<Response>`, streaming in both
  directions, resolving (not rejecting) with the callee's host-generated 500
  when the callee's handler throws, and rejecting only when the call could not
  be made; the current KV, SQLite, queue-producer, workflow, and actor bindings state their
  method names, argument and result types, and how each interface error code
  appears in JavaScript. The current set has no bucket projection; the retained
  v1beta1 ObjectBucket and `module-worker.object-bucket` definitions keep their
  exact historical descriptions.
  The meta-schema's `runtimeProjection` admits operation
  names only, so this belongs in `description`
  ([decision 0014](../decisions/0014-published-schemas-are-structural-minima.md));
- `bindingNameGrammar`: the grammar for instance names declared by the
  consumer. Worker-family bindings use the JavaScript identifier grammar
  `^[A-Za-z_$][A-Za-z0-9_$]*$`; `.` and `-` are invalid there;
- `lifecycle`: what happens to the binding when the target is deleted
  (`dependency_in_use` refusal is the default).

## Binding instances

A consumer resource declares instances as typed data. The wire shape is a
`name` plus an exact `resource` reference:

```json
{
  "name": "CACHE",
  "resource": {
    "apiVersion": "edge.forms.takoform.com",
    "kind": "EdgeKVNamespace",
    "name": "cache"
  }
}
```

- `resource` is an exact in-space reference. All three members are required:
  `apiVersion` and `kind` are `const` in the declaring Form's desired schema —
  the group is never "defaulted by the family", because a reference that omits
  it can address only one family and forces a host to guess which installed
  Form a bare kind means — and `name` is chosen by the author. After resolution
  the host pins the target UID *and the target's exact FormRef* in host-owned
  records; desired state keeps the name reference. A target deleted and
  re-created under the same name is a different resource, and the source is NOT
  re-bound to it; neither is a target whose Form has since moved to a different
  exact contract
  ([`../host-api/v1.md`](../host-api/v1.md),
  [decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md),
  [decision 0022](../decisions/0022-relations-pin-the-target-contract.md)).
  Re-applying the source is what re-pins it.
- Sensitive material never appears: a binding projects an API, not a
  credential. Sensitive-variable requirements are declared as named slots
  (`requiredSensitiveVars` on a worker version) and satisfied by
  host/operator-owned bindings.
- The object these names land in is fixed by the consumer's runtime ABI, not by
  the Binding: for the worker family, `worker.runtime@1.1.0` states that `env`
  carries exactly the declared binding names, vars keys, and sensitive-variable
  slots, and nothing else portable
  ([decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).
- Cross-space targets are unrepresentable, as in the retained lanes.

## Which Binding a list carries

A Form declares each binding list as a typed array property, and the property
itself states its contract through the `x-takoform-binding` annotation in the
Form's `desiredSchema`:

```json
"kvBindings": {
  "type": "array",
  "items": { "…": "one {name, resource} instance" },
  "x-takoform-binding": "module-worker.edge-kv"
}
```

The annotation names the contract; the exact digest-bound BindingRef is the
Form Definition's own `acceptedBindings` entry of that name, so the digest keeps
one source of truth. The annotation is what lets a host derive the binding
relations of a Form from the `desiredSchema` it already serves, instead of a
separate declared list the published Form Definition schema does not admit
([decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)).

The `resource` node INSIDE each instance carries a second annotation, stating
what the target must satisfy
([decision 0022](../decisions/0022-relations-pin-the-target-contract.md)). For a
binding it is always `x-takoform-required-interface`, naming exactly the Binding
Definition's own `targetInterface` at its exact digest:

```json
"resource": {
  "…": "the closed {apiVersion, kind, name} reference",
  "x-takoform-required-interface": {
    "apiVersion": "interfaces.takoform.com/v1alpha1",
    "name": "edge.kv", "version": "1.0.0", "schemaDigest": "sha256:..."
  }
}
```

A binding IS the projection of an Interface into the consumer's runtime, so the
exact contract the target provides is the whole requirement; pinning the
target's Form as well would refuse a different store implementing the same
contract for a reason the binding does not have. The Binding Definition remains
the authority for that Interface and the annotation is its projection onto the
reference, so the two cannot disagree — a Form whose binding list annotated any
other Interface does not render.

## Verification a host performs

A conforming host verifies a declared binding before it mutates anything.
Schema validity is never sufficient: a schema states the shape of a reference,
never whether the contract behind it can be honored. This order and its failure
codes are decided by
[decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md).
In order:

1. **Accepted by the source Form.** The source Form Definition's
   `acceptedBindings` must carry the annotated contract. Otherwise
   `invalid_argument` (400).
2. **Installed by the host at the exact digest.** The host must have the Binding
   Definition whose `schemaDigest` the source accepts. Otherwise
   `unsupported_capability` (422): the host cannot perform the capability at
   all, so this is not a malformed request.
3. **Source role equals `sourceRole`.** Otherwise `invalid_argument` (400).
   The comparison is equality with the role the Binding Definition declares;
   which role that is belongs to the Definition, so this check reads one value
   and never consults a table of roles that may or may not hold capability.
4. **Target Form is listed in `allowedTargetForms`.** The resolved target's
   exact group and kind must appear there. Otherwise `invalid_argument` (400).
5. **Target Form provides `targetInterface`.** The target Form Definition's
   `providedInterfaces` must declare the exact Interface the Binding requires.
   Otherwise `invalid_argument` (400) — a binding projects an Interface, so a
   target that provides none cannot be bound.
6. **Same space.** Guaranteed by the wire: a reference carries no space member,
   so a cross-space binding is unrepresentable rather than refused.
7. **The reference's own target contract.** The annotated
   `x-takoform-required-interface` is verified last, against the target's
   recorded exact ref and its Form's `providedInterfaces`
   ([decision 0022](../decisions/0022-relations-pin-the-target-contract.md)).
   For a binding it restates rule 5, because the annotation must be exactly the
   Binding's `targetInterface`; the order above is the one clients were told, so
   the binding-specific refusal is the one they get. Every NON-binding relation
   in a Form is verified by this rule alone.

Deleting a resource any live binding — or any other stored relation — references
fails `dependency_in_use` (409). The required conformance checks
`binding-contract-verified`, `relation-target-missing-rejected`,
`relation-target-deletion-blocked`, `relation-incarnation-change-detected`,
`relation-reapply-repins`, `relation-target-form-ref-verified`,
`relation-target-interface-verified`, and `relation-pin-records-target-form-ref`
prove a laxer host fails the lane.

## Binding distribution

Binding Definitions are distributed **with this repository**, as digest-bound
documents under `bindings/candidates/v1alpha2`. They are NOT independently
installable third-party artifacts, and there is no Binding Package envelope:
no such identity is specified, published, or planned for this generation
([decision 0021](../decisions/0021-third-party-forms-and-contract-distribution.md)).

The rule is the Interface distribution rule word for word
([`../interface-contract/`](../interface-contract/index.md)).
A `BindingRef`'s `schemaDigest` binds the canonical Definition bytes and
nothing else: no package digest, no publisher identity, no signature, no
revocation feed, and no closed payload inventory, because there is no package.
A host that advertises a Binding at an exact digest in its Host Support Profile
is stating which document it implements, not where it obtained it.

## Direction rule

Outward capability (this worker *uses* KV, a database, a queue, a worker
service, a workflow, or an actor) is a Binding held by the consumer's revision
resource. Inward activation (an HTTP route, custom domain, cron trigger, or queue consumer
*invokes* this worker) is an attachment resource pointing at the identity
resource. The two directions never share a mechanism.

An opaque external standard service is a third, deliberately separate case.
An `externalServices` slot carries `standards.takoform.com/v1` plus an opaque
reverse-DNS protocol identifier and has no target Form, Resource reference, or
Binding Definition. The Host resolves and seals that runtime-native service;
Takoform does not maintain a protocol enum or turn object storage into a
current `ObjectBucket` Form.

## Provider surface

The non-normative Provider 3 sample renders each current worker binding type as
its own typed list attribute — `actor_bindings`, `kv_bindings`,
`queue_producer_bindings`, `service_bindings`, `sqlite_bindings`, and
`workflow_bindings` — whose elements carry `name` and `target_name`. It has no
current `bucket_bindings` authoring surface. Retained Provider 2.1.1 codecs keep
their historical ObjectBucket mapping without defining the current contract.
There is no generic connection block in Host API v1. Provider mapping is
implementation evidence and cannot add to or block Specification 1.0.
