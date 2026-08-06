# Exact Interface contracts (interfaces.takoform.com/v1alpha1)

An Interface is an independently published, digest-bound contract for one
portable operation surface. It replaces the open `(name, version,
operations)` descriptors of the retained v1alpha2 lane for all new-lane Forms
([decision 0010](../decisions/0010-exact-interface-and-binding-contracts.md)).
The retained projection contract remains documented in
[`../interface-declaration/`](../interface-declaration/) for the frozen lanes.

## InterfaceRef

```json
{
  "apiVersion": "interfaces.takoform.com/v1alpha1",
  "name": "edge.kv",
  "version": "1.0.0",
  "schemaDigest": "sha256:..."
}
```

- `name` uses the dotted grammar `^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)*$`
  (max 128).
- `version` is SemVer.
- `schemaDigest` is the RFC 8785 canonical digest of the Interface
  Definition bytes. Two definitions with different canonical bytes are
  different Interfaces even under one display name.

The normative shape is
[`../schemas/interface-ref-v1alpha1.schema.json`](/schemas/interfaces/v1alpha1/interface-ref.schema.json).

## Interface Definition

The normative shape is
[`../schemas/interface-definition-v1alpha1.schema.json`](/schemas/interfaces/v1alpha1/interface-definition.schema.json).
A definition MUST fix, as data only:

- `operations[]`: name, input schema, output schema, and a closed
  `errors[]` vocabulary per operation;
- `semantics`: the consistency model and pagination contract the whole
  surface obeys — closed enums, not free tokens.

A definition SHOULD also declare, and MAY omit (both fields are OPTIONAL in
the schema, and the schema wins per [`../conformance.md`](../conformance.md)):

- `limits`: portable minimum limits every conforming host MUST accept when
  the definition declares them;
- `fixtures[]`: declarative behavior traces (see below).

A definition MUST NOT contain executable code, credentials, endpoints,
prices, or host identities; the Form Package data-only policy applies
unchanged.

## Behavior fixtures

Shape validation alone cannot distinguish a KV store from a queue, so an
Interface Definition SHOULD carry data-only traces:

```json
{
  "name": "put-then-get",
  "steps": [
    { "operation": "put", "input": { "key": "a", "value": "AQID" } },
    { "operation": "get", "input": { "key": "a" },
      "expected": { "status": "found" } }
  ]
}
```

A conforming implementation executes each trace in order against one fresh
scope and MUST produce outputs matching every `expected` clause. Properties
that are not deterministically observable from one client (for example
cross-location convergence of an eventually consistent store) are out of
fixture scope and belong to distributed conformance runs, not packages.

## Interface distribution

Interface candidates are currently distributed as digest-bound Interface
Definition documents in the repository's `interfaces/candidates/v1alpha1`
directory; the exact
InterfaceRef with its `schemaDigest` is the immutable reference. A dedicated
Interface Package envelope — one definition per canonical, digest-bound
package with data-only payloads and fixture closure, mirroring Form
Packages — is future work; no such package envelope identity is published
yet.

## Relationship to Forms and Bindings

- A Form Definition lists the Interfaces its resources provide as exact
  `providedInterfaces[]` refs.
- A Binding contract names the Interface its target must provide as an exact
  ref; a host resolving a binding MUST verify the target Form provides that
  Interface.
- Hosts advertise which Interfaces they implement, and within which limits,
  in their Host Support Profile — never inside the Interface Definition.
