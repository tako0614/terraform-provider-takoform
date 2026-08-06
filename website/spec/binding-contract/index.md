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
[`../schemas/binding-ref-v1alpha1.schema.json`](/schemas/bindings/v1alpha1/binding-ref.schema.json)
and
[`../schemas/binding-definition-v1alpha1.schema.json`](/schemas/bindings/v1alpha1/binding-definition.schema.json).

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
  "resource": { "kind": "EdgeKVNamespace", "name": "cache" }
}
```

- `resource` is an exact in-space reference (`apiVersion` defaulted by the
  Form's family, `kind` fixed by the binding field, `name` chosen by the
  author). After resolution the host pins the target UID in host-owned
  records; desired state keeps the name reference.
- Sensitive material never appears: a binding projects an API, not a
  credential. Sensitive-variable requirements are declared as named slots
  (`requiredSensitiveVars` on a worker version) and satisfied by
  host/operator-owned bindings.
- Cross-space targets are unrepresentable, as in the retained lanes.

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
