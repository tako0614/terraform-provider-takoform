# Exact Interface contracts (interfaces.takoform.com/v1alpha1)

An Interface is a repository-distributed, digest-bound contract for one
portable operation surface. It replaces the open `(name, version,
operations)` descriptors of the withdrawn v1alpha2 lane
([decision 0010](../decisions/0010-exact-interface-and-binding-contracts.md));
that lane's projection contract was withdrawn with its epoch and stays in
this repository's git history (decision 0042).

Official and external authors use this same public contract path. An external
author MUST import only the eventual public Core module and MUST NOT depend on
internal Provider or repository packages. Specification 1.1 records this
authoring boundary without publishing a new Interface lane or changing any
existing Interface identity.

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
new identity for each new kind of rule. `worker.runtime@1.1.0` is the current worked
example
([decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).

## Runtime ABIs are Interfaces too

An Interface is not only a capability a resource offers outward. The exact
runtime a host provides to code it runs is the same kind of contract, and is
declared the same way: current `ModuleWorker` lists `worker.runtime@1.1.0` in its
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
`globals` floor. The exact Host half is exercised by the manifest-owned
[`takoform-v1` suite](../../conformance/takoform-v1/manifest.json), while the
runtime half is exercised by the separate corpus below. A reader deciding
whether a passing conformance report means "this host runs my code correctly"
is entitled to know which subject that report measured.

The second half has its own corpus, runner, and command —
[`../../conformance/runtime-abi-v1/`](../../conformance/runtime-abi-v1/contract.json),
driven against a worker deployed from its own byte-pinned bundle rather than
against a Host API
([decision 0023](../decisions/0023-the-runtime-abi-is-measured-separately-from-the-control-plane.md)).
Its reports state which subject was measured: a run against the repository's
in-process stand-in proves the corpus, only a run against a deployed worker
proves a runtime, and neither is Specification publication evidence. Every handler
`worker.runtime@1.1.0` declares is measured there, and the corpus ENFORCES that
rather than stating it: a declared handler with no check naming it is refused at
load, and an explicitly unmeasured entry does not discharge one. A handler
nothing can invoke is therefore removed from the contract and returns with the
attachment that makes it observable, in a new exact version
([decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).
An operation that genuinely cannot be observed may still be recorded as
unmeasured with what would close it, which is what the load lane reports when a
run has no module-loader adapter.

## Behavior fixtures

Shape validation alone cannot distinguish a KV store from a queue, so an
Interface Definition SHOULD carry data-only traces.

### Retained beta-only example: `edge.objects`

The current Interface candidate set contains no `edge.objects`; current
`ObjectBucket` and `module-worker.object-bucket` identities do not exist. The
following retained beta example is preserved only to show how a declared
`read_after_write` consistency model makes the second step's expectation
normative. It is not a current Interface declaration:

```json
{
  "name": "put-then-head-observes-the-write",
  "steps": [
    { "operation": "put",
      "input": { "key": "a", "bodyStream": true, "contentLength": 8 } },
    { "operation": "head", "input": { "key": "a" },
      "expected": { "size": 8 } }
  ]
}
```

A conforming implementation executes each trace in order against one fresh
scope and MUST produce outputs matching every `expected` clause. Properties
that are not deterministically observable from one client (for example
cross-location convergence of an eventually consistent store) are out of
fixture scope and belong to distributed conformance runs, not packages.

A fixture set MUST NOT contradict the `semantics` of the definition that
carries it. The rule is easy to break in exactly one direction — writing the
trace an implementation "obviously" satisfies — and `edge.kv` broke it: it
declared `consistency: eventual` while requiring a put to be visible to the
very next get, which is the one trace a correct eventually consistent store is
allowed to fail. A trace no conforming implementation can pass is the same
defect as a required conformance check no correct host can complete, and the
fix is the same both times: keep the semantics, remove the trace, and state the
unprovable half as a host obligation
([decision 0020](../decisions/0020-the-edge-interfaces-state-their-data-and-delivery-model.md)).
A fixture whose `expectedError` is outside the closed `errors` vocabulary of
the operation it exercises is unpassable for the same reason, and the family
catalog refuses to render one.

Where a value is bytes, the contract carries it in one shared encoded-bytes
shape rather than as a bare string, so that a declared byte limit and a
structural `maxLength` do not measure two different quantities. Where a payload
streams at all — for example, a worker-to-worker request or response — the
operation declares that it streams and states what is KNOWN of its length
instead of carrying the bytes, and the bytes travel beside the document. What
is known is sometimes nothing: a `worker.service` call completes at the response
head, so a body generated as it is written has no byte count to state there, and
a contract that demanded one would have bought it by buffering the body — the
defect streaming exists to avoid. Its `contentLength` is therefore nullable in
both directions, an integer being an exact count the writer knows and `null` a
count it does not have. A contract that streams also has to say what a caller
can OBSERVE about the stream: whether an absent body differs from an empty one
and from one of unknown length, what a declared count obliges when the bytes
disagree with it, when the stream starts, how backpressure and cancellation
behave, and how a truncation on each side is reported.
`worker.service@1.0.0` is the worked example, and its first version is the
counter-example — it declared bodies as JSON strings while the binding that
projects it promised streaming, so the two halves of one contract disagreed.
All of it is decided in decision 0020 and stated in each definition's own
descriptions, because the published meta-schema has a member for none of it.

`edge.sql` uses the same encoded-bytes object for BLOBs but does not expose
SQLite storage-class tags. Its exact value is null, a finite binary64 number,
UTF-8 text, or canonical encoded bytes. Fractional finite numbers are valid;
integer-valued numbers outside `Number.MAX_SAFE_INTEGER` and non-finite values
are rejected.
`query` earns idempotency by executing inside a rollback-only transaction and
always rolling it back, not by classifying SQL text as read-only; runtime SQL
cannot own schema migration. That correction supersedes only decision 0020's
SQL portions
([decision 0034](../decisions/0034-edge-sql-uses-safe-wire-values-and-rollback-only-queries.md)).

## Interface distribution

Interface candidates are distributed as digest-bound Interface
Definition documents in the repository's `interfaces/candidates/v1alpha1`
directory; the exact
InterfaceRef with its `schemaDigest` is the immutable reference. They are not
independently installable third-party artifacts, and no Interface Package
envelope identity exists, is specified, or is published: the envelope was
withdrawn, not deferred
([decision 0021](../decisions/0021-third-party-forms-and-contract-distribution.md)).
A `schemaDigest` therefore binds the canonical Definition bytes and says
nothing about where they came from — no package digest, no publisher identity,
no signature, no revocation feed — so a host obtains these contracts the way it
obtains any other part of this specification.

## Relationship to Forms and Bindings

- A Form Definition lists the Interfaces its resources provide as exact
  `providedInterfaces[]` refs.
- A Binding contract names the Interface its target must provide as an exact
  ref; a host resolving a binding MUST verify the target Form provides that
  Interface.
- Hosts advertise which Interfaces they implement, and within which limits,
  in their Host Support Profile — never inside the Interface Definition.
