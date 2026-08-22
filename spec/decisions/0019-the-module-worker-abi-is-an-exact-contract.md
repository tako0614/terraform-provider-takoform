# 0019 — The Module Worker ABI is an exact contract, not a date

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

`ModuleWorker` claimed to fix "the ES Module Worker ABI" by identity, and
nothing in the repository said what that ABI was. A conforming host had no
statement of the module's default-export shape, the handler signatures, the
request and response types, how bindings appear in `env`, what `waitUntil`
means, what an uncaught exception does, whether bodies stream, which Web APIs
must exist, or which module media types load. [Decision
0008](0008-forms-preserve-service-shape.md) says a Form fixes the complete
application-visible semantics of one proven service primitive; for a worker the
ABI *is* those semantics, and it was missing. Two hosts could implement
`ModuleWorker` and run observably different runtimes while both passing every
check the lane had.

`WorkerVersion` made this worse rather than better. It carried
`compatibilityDate` (a required calendar date) and `compatibilityFlags` (a
closed set whose only member was `nodejs_compat`). A compatibility date selects
default runtime behavior only against a registry that states which behavior each
date changes. This project publishes no such registry and has no plan to own
one. The field therefore promised portability it could not deliver: two
conforming hosts reading `2026-01-01` could legitimately apply different
defaults, which is precisely the incompleteness
[`../portability-boundary.md`](../portability-boundary.md) forbids and precisely
the "free semantic token" shape decision 0008 prohibits. The repository already
treated the same spelling as a leaked backend token elsewhere: the v1alpha2
vendor-token guard in `spec/contracts_test.go` lists `compatibility_date` and
`compatibility_flags` alongside `cloudflare` and `wrangler`.

The five typed Bindings had a smaller version of the same gap. Each named the
Interface operations it granted but never said what the consumer's code
actually calls — no method names, argument types, return types, failure shape,
or streaming behavior. A capability without a callable surface is not a runtime
API.

## Decision

### The ABI is an exact Interface contract owned by `ModuleWorker`

`worker.runtime@1.0.0` is a new exact Interface contract in the
`interfaces.takoform.com/v1alpha1` lane, authored in
`internal/edgeformcatalog` beside the other Interface definitions and
distributed as a candidate under `interfaces/candidates/v1alpha1`. It fixes, as
data:

- **`loadModule`** — the module's default export is a plain object; each
  handler is an optional own property of it; a handler a version DECLARES must
  exist there or the version fails before traffic arrives. It also fixes the
  **loadable** media types (`application/javascript+module`, `text/plain`,
  `application/octet-stream`, `application/wasm`) and states that a WASM
  module's default export is a compiled `WebAssembly.Module`, never an
  instance, so imports stay the application's choice. Its `declaredHandlers`
  enum IS the closed handler vocabulary of the ABI, and it is the single source
  of truth for it: the `WorkerVersion` `handlers` enum is read back out of this
  contract rather than written a second time. Its `modules` and
  `auxiliaryModules` media-type enums are the single source of truth for the
  two module classes in the same way (see "The loadable set is the manifest's
  set, minus what is never imported" below).
- **`fetch`** — `fetch(request, env, ctx) -> Response | Promise<Response>`,
  with the request/response types, `bodyStream`, and `handlerOutcome`.
- **`scheduled`** — `scheduled(event, env, ctx) -> void | Promise<void>`; the
  event carries `scheduledTime` (UTC milliseconds) and the matched five-field
  `cron` expression.
- **`queue`** — `queue(batch, env, ctx) -> void | Promise<void>`; the batch
  carries the queue name and ordered messages with `id`, `timestamp`, `body`,
  and `attempts`.
- **`environment`** — `env`'s own enumerable properties are exactly the union of
  the version's `vars` keys, `requiredSensitiveVars` names, and binding names,
  and nothing else portable; a sensitive-variable slot appears as its
  host-supplied value.
- **`waitUntil`** — the host keeps the isolate alive until the registered
  promise settles; a rejection reaches host diagnostics only, never changes an
  already-sent response, and never fails a successful invocation.
- **`globals`** — the portable minimum Web API surface: `Request`, `Response`,
  `Headers`, `URL`, `URLSearchParams`, `TextEncoder`/`TextDecoder`,
  `crypto.getRandomValues`, `crypto.subtle.digest`, `structuredClone`,
  `AbortController`/`AbortSignal`, `ReadableStream`/`WritableStream`, the four
  timers, and `fetch`.

Exception handling is fixed across the surface: an uncaught throw or unhandled
rejection in `fetch` is a host-generated 500 response, never a hung request; in
`scheduled` and `queue` it is a failed invocation, and a failed `queue`
invocation redelivers the batch under the at-least-once delivery `edge.queue`
already states and the retry policy of the invoking `QueueConsumer`.

The contract is **provided by `ModuleWorker`**, the identity, because the
identity is what a host implements; a `WorkerVersion` is the code that fills it.
The published Interface name grammar admits no hyphen, so the contract is named
`worker.runtime`, matching the family's other worker-scoped Interface
`worker.service`; the hyphenated `module-worker.` prefix stays in the Binding
namespace, where the grammar allows it.

Rules that are behavioral rather than structural are stated as normative
sentences in the definition's descriptions and proven by fixtures wherever a
fixture can prove them — the placement model of [decision
0014](0014-published-schemas-are-structural-minima.md). Nothing about this
contract required a new schema identity.

### The loadable set is the manifest's set, minus what is never imported

The ABI's first statement of its loadable media types was written beside, not
against, the artifact manifest's — and the two did not agree. The manifest
admitted `application/source-map+json`, which no runtime loads; the ABI claimed
`application/json`, which the published manifest enum never admitted. A bundle
could therefore be accepted by one and unusable by the other, and nothing
discovered it until deploy: a Worker built out of modules the runtime refuses.

The reconciled set is one statement in two classes:

- **loadable** — `application/javascript+module`, `text/plain`,
  `application/octet-stream`, `application/wasm`. What the module graph may
  import, and the only class a bundle's `mainModule` may name.
- **auxiliary** — `application/source-map+json`. What a bundle may carry and
  the graph never imports. Refused as `mainModule`, refused as an import target
  (`unsupported_media_type`), and never an error merely for being present.

`application/json` is not supported in this ABI version. Removing it is the
narrowing direction decision 0014 permits: the published manifest enum is the
union of the two classes above, so the code narrows onto the published bytes
rather than past them, and a widening later would be a contract change with a
new digest like any other.

The set has ONE source of truth — `internal/currentformmodel` — which the
runtime contract's two enums, the host's manifest validator, and the provider's
authoring allowlist all read rather than restate. A drift gate holds it to the
published manifest enum, and the required conformance check
`bundle-main-module-is-loadable` proves the half a schema cannot state: the
published enum lists all five media types in one place and cannot relate
`mainModule` to the media type of the module it names, so only a host stops a
bundle whose first module is evidence rather than code
([decision 0014](0014-published-schemas-are-structural-minima.md), and
[decision 0012](0012-artifacts-use-content-addressed-upload.md) for the
manifest side).

### `tail` is removed, and every declared handler is measured

The vocabulary above no longer contains `tail`. It was declared, and nothing in
the Edge Platform Family could activate it: the inward-activation attachments
are `WorkerCustomDomain` (`fetch`), `WorkerCronTrigger` (`scheduled`),
`QueueConsumer` (`queue`), and `WorkerEndpoint` (`fetch`). No portable
deployment could cause a host to invoke it, so no run could observe it and two
hosts could implement it differently with nothing able to detect the
divergence — which is the `compatibilityDate` defect in a different member.
[Decision 0023](0023-the-runtime-abi-is-measured-separately-from-the-control-plane.md)
recorded the removal as the recommended fix and deferred it on sequencing,
because the Interface bytes are the input to the generated Form chain; this is
that change. Blocker **V3-011** closes with it.

Adding a `WorkerTailConsumer` instead would have made the handler observable and
is the wrong move now: it widens the family by a Form nothing else asks for, and
it decides the trace model — what an event carries, what sampling means, what a
failed consumer does to the traced invocation — inside a change whose subject is
the ABI. **`tail` returns together with the attachment that makes it observable
and a NEW exact `worker.runtime` version, never as a bare handler.** Under the
rule this decision already states, a runtime revision IS a new exact contract
version, so re-adding it is a version bump with a new digest every consumer
sees, not a quiet widening.

What replaces the handler is a property: **every handler the ABI declares is
measured.** That is a claim worth nothing as prose, so the runtime corpus
ENFORCES it. Its loader refuses a corpus in which a member of
`handlerVocabulary` — which is read out of this contract's own
`declaredHandlers` enum — has no check whose `operation` names it, and an
`unmeasured` entry does not discharge one, because recording a handler as
unmeasured is exactly what the corpus did for `tail`. Widening the enum without
measuring the addition therefore fails corpus verification by name rather than
waiting for review. The `unmeasured` mechanism itself stays: the load lane still
reports itself unmeasured when an operator supplies no module-loader adapter,
and a future surface that genuinely cannot be observed can still say so — what
it can no longer do is stand in for a handler.

### `compatibilityDate` and `compatibilityFlags` are removed

Both fields, and the `nodejs_compat` enum, are deleted from `WorkerVersion` in
the catalog, the generated Form Definition and fixtures, the provider schema,
the generated docs and examples, the conformance probe, and the Host Support
Profile (`supportedRanges.compatibilityDate`,
`supportedEnums.compatibilityFlags`). A conforming host MUST NOT advertise
either.

**A runtime revision is a new exact Interface version, and — if it changes what
a Form desires — a new Form version. It is never a date and never a flag.** This
is stated normatively in [`../form-families.md`](../form-families.md), in
`../host-api/v1alpha3.md`, and in
[`../interface-contract/`](../interface-contract/README.md).

### Bindings state the surface the caller calls

Every Binding Definition's `description` now states the JavaScript surface it
projects: the method names on `env.NAME`, argument and return types, how each
Interface error code appears in JavaScript, and whether bodies stream.
`module-worker.service` projects `env.NAME.fetch(request) -> Promise<Response>`,
streaming in both directions, resolving with the callee's host-generated 500
when the callee's handler throws — the promise rejects only when the call could
not be made at all. The meta-schema's `runtimeProjection` member admits
operation names only, so the projection prose lives in `description`, again per
decision 0014.

### Conformance

Four required checks join the v1alpha3 runner list:

- `module-worker-runtime-contract-advertised` — the host serves the runtime
  contract's support profile at the exact pinned `schemaDigest` and supports the
  exact `ModuleWorker` Form line. Because the ModuleWorker Form Definition's own
  digest covers its `providedInterfaces`, a host serving a definition that omits
  or moves the runtime ref already fails `form-definition-exact`.
- `undeclared-runtime-handler-rejected` — a `WorkerVersion` declaring a handler
  the runtime contract does not define is refused at both pre-mutation gates
  (`validate` and `prepare`) and stores nothing, while a version declaring only
  defined handlers is still accepted.
- `declared-handler-not-exported-rejected` — a `WorkerVersion` declaring a
  handler the ABI *does* define, against a bundle whose main module does not
  export it, is refused before any mutation and stores nothing, while a version
  declaring only exported handlers is still accepted. `loadModule` fails such a
  version with `handler_not_exported` before traffic arrives, so a host that
  stored it would go on gating attachments against a handler that does not
  exist. Unlike the check above, this one is not decidable from the spec: it
  reads the bundle relation, the committed manifest, and the module's content
  address, so it lives on the mutation path beside the other Worker aggregate
  rules.
- `bundle-main-module-is-loadable` — a `WorkerBundle` manifest whose
  `mainModule` names an auxiliary module is refused, while the same module set
  with a loadable `mainModule` commits with the auxiliary module still in it.
  One module set drives both directions, so the only difference between the
  refusal and the acceptance is which module `mainModule` names.

`support-profiles-present` additionally now fails a host that advertises a
`compatibilityDate` range or a `compatibilityFlags` enum, or whose `handlers`
enum is not exactly the contract's vocabulary.

The corpus states what each pinned module exports, because a check pairing a
declaration with a bundle that cannot answer it would fail exactly the hosts
that implement the contract — a required check no correct host can pass is worse
than a missing one. `conformance/portable-host-v3` therefore carries two
bundles: the probe bundle, whose module exports the whole vocabulary and which
every positive control is driven against, and one fetch-only bundle that exists
so the refusal above can be driven at all. Loading the corpus fails if either
stops being what the lane needs it to be.

The lane is a Host API runner, so it proves what a host advertises and what it
refuses, never what an isolate does. `../host-api/v1alpha3.md`
states the split explicitly: handler signatures, streaming bodies, the exact
`env` property set, `waitUntil`, exception outcomes, the `globals` floor, and
`handler_not_exported` for arbitrary bytes remain obligations proven by the
contract's behavior fixtures against a real runtime.

## Consequences

- `ModuleWorker` and `WorkerVersion` Form Definition bytes change, so their
  `schemaDigest` and package digests move, and the whole generated chain —
  candidates, provider v3 registry, conformance corpus pins, docs, examples,
  website mirrors — is regenerated. No published artifact changes: the family
  lane is unpublished.
- A host can no longer claim "runs module workers" without saying which runtime.
  The claim is one digest, and the conformance lane reads it back.
- The `handlers` vocabulary has exactly one source of truth. Widening it is a
  contract change with a new digest, which every consumer sees, rather than a
  quiet host decision.
- So does the module media-type set. Four surfaces state it and none of them
  owns a list: the contract, the manifest schema gate, the host validator, and
  the authoring allowlist all resolve to one declaration, so a bundle a host
  commits is a bundle this runtime can load, by construction rather than by
  review.
- An asset-serving worker and a sub-hourly cron remain unexpressible; those are
  separate blockers with their own decisions. Nothing here widens them.
- `WorkerVersion` loses its only `date-string` field. The authoring model keeps
  the kind, because it describes a shape a future Form may legitimately need;
  what it must never again describe is a runtime selector.
- The handler vocabulary is three, not four. `WorkerVersion`'s `handlers` enum,
  the runtime corpus's `handlerVocabulary`, the Host Support Profile a host
  advertises, and the reference module every positive control is driven against
  all follow from the one declaration, so none of them had to be edited twice —
  and the corpus's own reference module stopped exporting a handler the ABI no
  longer defines, because a module claiming an export the contract does not
  declare is refused at load.
- `conformance/runtime-abi-v1` goes from seventeen checks to eighteen: `tail`'s
  unmeasured entry leaves, and two worker-to-worker streaming checks arrive
  ([decision 0020](0020-the-edge-interfaces-state-their-data-and-delivery-model.md)).
  Its self-test is now `complete` rather than `partial` whenever the load lane is
  measured, which is the observable difference between a matrix with a hole in it
  and one without.

## Rejected alternatives

- **Keep `compatibilityDate` and build a behavior registry.** This is the only
  way the field could have meant something: a published, versioned, digest-bound
  document mapping each date to the exact set of behavior changes it turns on,
  plus a rule for what a host does with a date it does not know, plus
  conformance proving two hosts apply the same date identically. That is a
  second normative product line — its own schema identity, its own publication
  and freeze policy, its own conformance corpus — owned by a project that
  deliberately does not own runtime implementations. It is also strictly larger
  than the problem: dates exist to let ONE vendor evolve ONE runtime without
  breaking deployed code, whereas Takoform's job is to make independent hosts
  agree. Rejected for now, not forever: if the family ever ships a runtime
  revision that must coexist with its predecessor inside one host, this is the
  design to revisit, and the exact-contract model composes with it — a registry
  would map dates to contract versions rather than replacing them.
- **Keep the date as an opaque host hint.** Rejected because the v1alpha3 lane
  forbids open tokens outright (decision 0008): a field whose meaning each host
  decides privately is the exact thing that makes one Form mean many things. An
  "advisory" token is worse than an absent one, because authors would write it,
  hosts would honor it differently, and the divergence would be invisible to
  every check in the lane.
- **Describe the ABI in prose only** — in the proposal, the Form description, or
  a specification page. Rejected because prose is not held to a digest. A host
  cannot advertise conformance to a paragraph, `form-definition-exact` cannot
  detect a host that read it differently, and the two required checks above
  would have nothing to compare. The ABI is the contract, so it has to be a
  contract.
- **Put the ABI on `WorkerVersion` instead of `ModuleWorker`.** Rejected because
  a version is code, not an implementation of the runtime: a host implements the
  ABI once, for the identity every deployment and attachment addresses. Declaring
  it per revision would also make the ABI something a Terraform author appears to
  choose.
- **Mint a `runtimes.takoform.com` document kind for the ABI.** Rejected under
  decision 0014: the published interface-definition meta-schema already admits
  operations with input/output schemas, closed errors, semantics, limits,
  fixtures, and descriptions, which is enough to express every rule above. A new
  identity would have bought vocabulary, not closure.
