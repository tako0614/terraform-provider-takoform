# Typed Binding contracts (bindings.takoform.com/v1alpha1)

A Binding is a digest-bound contract that grants one consumer resource a
typed capability on one target resource: the runtime API and the permission
arrive together, and credentials never cross the boundary. Bindings replace
the generic `connections` / `permissions` / `projection` surface of the
retained v1alpha2 lane
([decision 0010](../decisions/0010-exact-interface-and-binding-contracts.md)).

## BindingRef

```json
{
  "apiVersion": "bindings.takoform.com/v1alpha1",
  "name": "module-worker.edge-kv",
  "version": "1.0.0",
  "schemaDigest": "sha256:..."
}
```

`name` uses the dotted grammar; `schemaDigest` binds the canonical Binding
Definition bytes. The normative shapes are
[`../schemas/binding-ref-v1alpha1.schema.json`](../schemas/binding-ref-v1alpha1.schema.json)
and
[`../schemas/binding-definition-v1alpha1.schema.json`](../schemas/binding-definition-v1alpha1.schema.json).

## Binding Definition

A Binding Definition fixes, as data only:

- `sourceRole`: the Form role allowed to hold this binding (normally
  `revision`; a deployment or identity resource never holds capability
  bindings);
- `targetInterface`: the exact InterfaceRef the target must provide;
- `allowedTargetForms[]`: exact Form kinds (by group and kind) this binding
  may point at;
- `runtimeProjection`: the API surface projected into the consumer at run
  time, described against the target Interface's operations, including
  which operations the binding exposes (`accessModes` when the binding
  supports restricted modes);
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
    "apiVersion": "edge.forms.takoform.com/v1alpha1",
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
  the host pins the target UID in host-owned records; desired state keeps the
  name reference. A target deleted and re-created under the same name is a
  different resource, and the source is NOT re-bound to it
  ([`../host-api/v1alpha3.md`](../host-api/v1alpha3.md),
  [decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md)).
  Re-applying the source is what re-pins it.
- Sensitive material never appears: a binding projects an API, not a
  credential. Sensitive-variable requirements are declared as named slots
  (`requiredSensitiveVars` on a worker version) and satisfied by
  host/operator-owned bindings.
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
3. **Source role matches `sourceRole`.** Otherwise `invalid_argument` (400).
   Outward capability belongs to revision resources; an identity or deployment
   resource never holds one.
4. **Target Form is listed in `allowedTargetForms`.** The resolved target's
   exact group and kind must appear there. Otherwise `invalid_argument` (400).
5. **Target Form provides `targetInterface`.** The target Form Definition's
   `providedInterfaces` must declare the exact Interface the Binding requires.
   Otherwise `invalid_argument` (400) — a binding projects an Interface, so a
   target that provides none cannot be bound.
6. **Same space.** Guaranteed by the wire: a reference carries no space member,
   so a cross-space binding is unrepresentable rather than refused.

Deleting a resource any live binding — or any other stored relation — references
fails `dependency_in_use` (409). The required conformance checks
`binding-contract-verified`, `relation-target-missing-rejected`,
`relation-target-deletion-blocked`, `relation-incarnation-change-detected`, and
`relation-reapply-repins` prove a laxer host fails the lane.

## Direction rule

Outward capability (this worker *uses* KV, a bucket, a database, a queue, a
service) is a Binding held by the consumer's revision resource. Inward
activation (an HTTP route, custom domain, cron trigger, or queue consumer
*invokes* this worker) is an attachment resource pointing at the identity
resource. The two directions never share a mechanism.

## Provider surface

The official provider renders each binding type as its own typed list
attribute — `kv_bindings`, `bucket_bindings`, `sqlite_bindings`,
`queue_producer_bindings`, `service_bindings` — whose elements carry `name`
and `target_name`. There is no generic connection block in the v1alpha3
lane. (Decision 0010 names the block concept in the singular; the
implemented attribute names are these plural lists.)
