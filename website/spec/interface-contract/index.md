# Exact Interface contracts (interfaces.takoform.com/v1alpha1)

An Interface is a digest-bound contract for one portable operation surface,
distributed with this repository. It replaces the open `(name, version,
operations)` descriptors of the retained v1alpha2 lane for all new-lane Forms
([decision 0010](../decisions/0010-exact-interface-and-binding-contracts.md)).
It is not an independently published artifact: see
[Interface distribution](#interface-distribution) below for exactly what an
InterfaceRef does and does not assure
([decision 0021](../decisions/0021-third-party-forms-and-contract-distribution.md)).
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
  (max 128). Segments are lowercase alphanumeric; a hyphen is invalid here, so
  an Interface name never mirrors a hyphenated Binding name such as
  `module-worker.service`. The Edge Platform Family's worker-scoped contracts
  are therefore `worker.service` and `worker.runtime`.
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

Rules that are behavioral rather than structural — what an uncaught exception
does, whether a body streams, what keeps an isolate alive — are stated as
normative sentences in the definition's own `description` and per-operation
`description`, and proven by fixtures wherever a fixture can prove them. That is
the placement rule of
[decision 0014](../decisions/0014-published-schemas-are-structural-minima.md):
the published meta-schema is a structural minimum, so a contract expresses
everything it can within what that schema already admits rather than minting a
new identity for each new kind of rule. `worker.runtime@1.0.0` is the worked
example
([decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).

## Runtime ABIs are Interfaces too

An Interface is not only a capability a resource offers outward. The exact
runtime a host provides to code it runs is the same kind of contract, and is
declared the same way: `ModuleWorker` lists `worker.runtime@1.0.0` in its
`providedInterfaces`, a host that supports that Form implements the contract at
that exact digest, and a runtime revision is a new contract version rather than
a new value of some field. A Form MUST NOT state a runtime by a version token,
date, or flag whose meaning depends on a registry the specification does not
publish.

A runtime contract is verified in two places, and neither substitutes for the
other. The Host API conformance lane drives desired state, so it proves what a
host SAYS and what it REFUSES: the exact advertised digest, the closed handler
vocabulary, and that a Worker Version declaring a handler the ABI does not
define — or one the module it references does not export — is refused before
anything is stored. Everything the contract fixes about invocation is proven by
its behavior fixtures against a real isolate: handler signatures, streaming
bodies, `env`'s exact property set, `waitUntil`, exception outcomes, and the
`globals` floor. The exact split for `worker.runtime@1.0.0` is written down in
[`../host-api/v1alpha3.md`](../host-api/v1alpha3.md#what-the-lane-proves-and-what-stays-a-host-obligation),
because a reader deciding whether a passing conformance report means "this host
runs my code correctly" is entitled to know which half it covers.

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

Interface Definitions are distributed **with this repository**, as digest-bound
documents under `interfaces/candidates/v1alpha1`. They are NOT independently
installable third-party artifacts, and there is no Interface Package envelope:
no such identity is specified, published, or planned for this generation
([decision 0021](../decisions/0021-third-party-forms-and-contract-distribution.md)).

What an exact InterfaceRef therefore assures, and what it does not:

- `schemaDigest` binds the RFC 8785 canonical Definition bytes, so a holder of
  those bytes MUST be able to confirm they are the document the ref names, and
  two different canonical byte strings are two different Interfaces;
- the ref says nothing about where the bytes came from. There is no package
  digest, no publisher identity, no signature, no transparency proof, no
  revocation statement, and no closed payload inventory, because there is no
  package. The Form Package trust lane
  ([`../trust/`](../trust/index.md)) covers Form Packages only.

A host obtains these contracts the way it obtains any other part of this
specification. An operator who installs an Interface Definition from any other
source has exactly the assurance they arranged for it themselves; this
specification provides none and MUST NOT be read as providing any.

Building the envelope means minting a `packages.interfaces.takoform.com` index
identity, which cannot be done by reusing
[`../schemas/package-index-v1alpha4.schema.json`](/schemas/v1alpha4/package-index.schema.json):
that index is closed, fixes `kind` to `FormPackage`, and requires a `formRef`.
The mint therefore waits for the next coherent generation of schema identities
([decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)).

## Relationship to Forms and Bindings

- A Form Definition lists the Interfaces its resources provide as exact
  `providedInterfaces[]` refs.
- A Binding contract names the Interface its target must provide as an exact
  ref; a host resolving a binding MUST verify the target Form provides that
  Interface.
- Hosts advertise which Interfaces they implement, and within which limits,
  in their Host Support Profile — never inside the Interface Definition.
