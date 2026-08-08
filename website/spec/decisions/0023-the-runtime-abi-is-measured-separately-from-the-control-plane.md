# 0023 — The runtime ABI is measured separately from the control plane

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

[Decision 0019](0019-the-module-worker-abi-is-an-exact-contract.md) made the ES
Module Worker ABI an exact Interface contract, `worker.runtime@1.0.0`, and put
three required checks in the Host API v1alpha3 lane. That lane is a Host API
runner: it drives desired state over HTTP, so it can prove what a host
ADVERTISES and what it REFUSES to store, and nothing else.
[`../host-api/v1alpha3.md`](../host-api/v1alpha3.md#what-the-lane-proves-and-what-stays-a-host-obligation)
already says so, and lists what it leaves unproven: the default export's shape
and the three-argument handler signatures, `handler_not_exported` for arbitrary
bytes, streaming request and response bodies, `env` carrying exactly the
declared names, `ctx.waitUntil`, the exception outcomes, and the `globals`
floor.

Publication blocker V3-008 exists because of that gap. Its rationale was
already correct — the lane does not prove that an isolate behaves as the
contract says — but nothing in the repository could have measured a runtime
even if one had volunteered. There was no corpus, no probe module, no runner,
and no report format. "A real runtime is proven against the ABI contract" named
an outcome with no artifact behind it, which is the state where a blocker
quietly becomes a wish.

Two further facts shaped the design. First, the ABI's `loadModule` outcomes —
`module_not_found`, `unsupported_media_type`, `module_syntax_error`,
`handler_not_exported` — are decided BEFORE any traffic arrives, so no request
to a running worker can observe them. Second, the ABI declares a `tail` handler
that no resource in the Edge Platform Family can activate: the three
inward-activation attachments are `WorkerCustomDomain` (`fetch`),
`WorkerCronTrigger` (`scheduled`) and `QueueConsumer` (`queue`).

## Decision

### The runtime is measured by its own corpus against a deployed worker

`conformance/runtime-abi-v1` is a new digest-pinned corpus, and
`internal/runtimeconformance` its runner, separate from
`conformance/portable-host-v3` and `internal/portableconformancev3` in exactly
the way their subjects are separate. The host corpus measures a control plane
and takes a host endpoint with three tenant/principal credentials; this one
measures a runtime and takes the base URL of a worker deployed from the corpus
bundle. `cmd/runtime-conformance` is a separate command for the same reason: a
report has to be unambiguous about which subject it describes.

The contract states, as data, the exact `worker.runtime` InterfaceRef it
measures — including the `schemaDigest`, because two definitions with different
canonical bytes are different Interfaces — the closed handler vocabulary, the
closed loadable media-type set, the portable globals floor, the exact
deployment an operator must reproduce, the byte-pinned bundles, and for each
required check its name, what it proves, the bundle it needs, and the
request/expected-observation pairs that decide it.

The bundles carry module bytes and the handlers those bytes genuinely export.
The corpus loader recomputes every stated outcome from the bytes and refuses a
corpus that states one they do not produce, in both directions: a bundle that
claims an export its module does not have is refused, and so is a check
expecting a failure the bytes cannot cause. This is the honesty rule the host
corpus learned in decision 0019 — a required check no correct implementation
can pass is worse than a missing one, and its mirror image, a check no
incorrect implementation can fail, proves nothing.

The observation surface is a probe module that holds no expectation. Every
route reports what the RUNTIME supplied: the arguments of the invocation, the
own property names of `env`, which of the names the runner asked about exist as
globals, what a binding returned, and what the host later delivered to the
`scheduled` and `queue` handlers. The globals check sends the floor and asks
about those names, so the module cannot flatter a runtime that is missing one.

`loadModule` is measured through a disposable adapter over the runtime's own
module loader, declared in the contract as `takoform.runtime-abi-loader@v1`.
The adapter is not part of the ABI and must never be exposed by a production
deployment — the same discipline the host lane applies to its probe headers.
A run without one reports the load lane `unmeasured` and itself `partial`,
rather than counting an unmeasurable half as evidence.

### An observation is evidence only for the run that caused it

Three checks read back something the runtime stored for them: the `edge.kv`
round trip, the queue delivery, and the `ctx.waitUntil` marker. The subject
outlives the run — the `edge.kv` namespace an operator deploys is still there
the next morning — so a correlation value pinned as a constant makes the second
measurement of one deployment read the first one's observations. That is the
disqualifying pair above, both halves of it at once: a runtime whose `put`
resolves without persisting anything and whose queue never delivers passes on
the leftovers, and a conforming runtime fails because the `waitUntil` marker was
written before the task it just registered had run.

A per-run value cannot be a pinned constant and a pinned constant cannot
correlate a run, so what the corpus pins is the TEMPLATE. `runCorrelation`
states the placeholder and the token width; each correlated check states a
template carrying the placeholder exactly once; the runner mints one
unpredictable token per run, substitutes it for the placeholder in the value it
sends, and derives the observation it expects by that same substitution. The
corpus stays byte-identical — the token is never in it — and the loader refuses
a correlated check whose template is a constant exactly as it refuses a bundle
claiming an export its bytes do not have. The report states the token the run
used, so a failed run is diagnosable from the report and the corpus and nothing
else.

The probe mints nothing. It stores each observation under the value the RUNNER
sent and reports that value back, for the same reason it holds no expectations:
a probe that correlated itself would say nothing about which run stored what.

### A self-test proves the corpus and never a runtime

`SelfTest` runs the whole matrix against an in-process stand-in shipped with
this repository, so every `bun run check` exercises the corpus. This repository
has no JavaScript engine, so the stand-in reimplements the probe module's
behaviour in Go rather than executing its bytes. What keeps it honest is what
it is allowed to see: it is constructed from the deployment description and the
module bytes and NOTHING else — it never receives the contract, a check, or an
expected observation, and the package boundary enforces that rather than a
convention. Its loader answers from the bytes it is handed on the wire; it
derives `env` from the declaration; it states its own globals surface; it
streams through real incremental reads and writes; and it holds deferred work
in a goroutine that genuinely outlives the response.

### A report says which run it was

Every report carries `classification`
(`in-process-fake-runtime-self-test` or `deployed-runtime-conformance-run`), a
`proves` sentence in its own words, a `completeness` of `complete` or `partial`,
and `publicationReady: false` always — the same discipline as the host reports.
A self-test report says, in the document itself, that it proves the corpus and
nothing about any runtime.

### What closes V3-008

A passed `deployed-runtime-conformance-run`, with the load lane measured,
against a runtime this repository does not own. The corpus existing is not the
blocker closing, and the ledger entry now names the run rather than the
artifact. The report repeats that sentence in a `blockerEvidence` member, so a
reader holding only a report knows what it is and is not.

### `tail` is declared unmeasured rather than left unchecked

The corpus carries `tail` as an explicit `unmeasured` entry stating why it
cannot be observed, what would close it, and which blocker records it. Leaving
it silently unchecked would repeat exactly the defect
`compatibilityDate` had: a member of a published-shaped contract that no check
ever reaches, so no divergence between two hosts is detectable.

The handler is not merely hard to measure; it is unreachable. Removing it from
`worker.runtime@1.0.0` is the recommended fix, and this decision records that
recommendation rather than performing it: the Interface bytes are the input to
the generated Form chain, and moving them is a change to be made with that
regeneration rather than beside it. New blocker V3-011 holds the decision, and
[`../host-api/v1alpha3.md`](../host-api/v1alpha3.md) now lists `tail` among the
obligations the Host API lane does not prove, marked unmeasurable rather than
merely unproven.

## Consequences

- The seven obligations v1alpha3 lists as unproven now have an artifact that
  measures them. Whether a host meets them is a question with an answer.
- A runtime conformance run has a deployment prerequisite: the operator
  deploys the corpus bundle with the declared vars, sensitive variable,
  `edge.kv` and `edge.queue` bindings, a cron trigger, and a queue consumer.
  That is not an accident of the corpus — `scheduled` and `queue` are only
  observable when a host has been asked to invoke them.
- Three checks are timing-sensitive by nature: streaming is a claim about when
  bytes arrive, and `waitUntil` is a claim about work outliving a response.
  Their bounds are contract data, and each states the bound as a maximum a
  conforming runtime stays inside — including the wait for the streamed
  request's response headers, which is the wait a host that buffers the body
  until EOF never ends. A run waits the bound the corpus declares, not whatever
  timeout the caller's HTTP client happens to carry.
- The request-body check proves an ordering, not a framing. A `ReadableStream`
  read is not a transport frame, so the runner accumulates the worker's answers
  until they account for the bytes it has actually sent, and a conforming stack
  that splits one write across several reads passes. A host that buffers the
  body still fails, because it answers for nothing while the second chunk is
  unsent.
- Repeat measurement of one deployment is measurement rather than archaeology.
  A run against a deployment measured ten times behaves exactly like a run
  against a fresh one, which is the property the run token buys and the reason
  the correlated checks state templates rather than values.
- The corpus pins the ABI's `schemaDigest` as data, and a test recomputes it
  from the committed candidate definition. A change to the ABI's bytes fails
  that test by name and asks for a re-pin, instead of leaving a corpus quietly
  measuring a contract that no longer exists.
- The contract does not fix the JavaScript form of a queue message `body`; it
  states the wire shape only. The probe normalises the plausible forms and the
  gap is recorded here rather than papered over, because a runtime that hands
  the handler a different shape from its neighbour is exactly the divergence
  this project exists to prevent.
- `publicationReady` is false in every report this lane can produce, so nothing
  here weakens the publication freeze.

## Rejected alternatives

- **Extend `conformance/portable-host-v3` to cover runtime behaviour.**
  Rejected because the control plane cannot observe it. That runner speaks the
  Host API: it can ask a host what it advertises and watch it refuse a
  `WorkerVersion`, and every one of its observations is a desired-state
  document. No sequence of `validate`, `prepare`, `apply`, `import`, `observe`
  and `delete` can tell you whether `env` carried a name the version did not
  declare, whether a response body was buffered, or whether a rejected
  `waitUntil` task changed a response that was already sent — those facts exist
  only inside an isolate that is running code, which the lane never causes to
  happen. Bolting a worker-invocation lane onto that runner would also give one
  passing report two meanings, and the report is what a reader uses to decide
  whether a host runs their code correctly.
- **Trust the host's Host Support Profile declaration.** Rejected because a
  declaration is a claim, not a measurement. `worker.runtime@1.0.0` at an exact
  digest is precisely what a host must say; the whole value of saying it is
  that someone can check it, and a host that advertises the digest while its
  isolate buffers response bodies passes every check the v1alpha3 lane has.
  Treating the declaration as evidence would make the exact-contract model
  self-certifying, which is the shape decision 0019 rejected when it refused
  the compatibility date: a token whose meaning nothing verifies.
- **Ship the corpus and call V3-008 closed.** Rejected because the blocker is
  about a runtime, not about an artifact. The ledger validates that evidence
  paths exist, so a corpus path would have satisfied the mechanism while
  leaving the invariant unproven — the exact failure the ledger's
  "closing a blocker requires the evidence it names, not a passing local gate"
  rule exists to prevent.
- **Execute the probe module in-process with an embedded JavaScript engine.**
  Rejected because it buys a self-test that looks like a runtime run without
  being one. An embedded engine is a third runtime with its own conformance
  question, it would pull an executable dependency into a repository whose
  Forms and packages are deliberately data-only, and it would make a green
  local gate read as ABI evidence. The stand-in is deliberately obvious about
  being a stand-in instead.
- **Drop `tail` from the ABI in this change.** Rejected here for sequencing,
  not on the merits: the Interface bytes feed the generated Form chain, so the
  removal belongs to the change that regenerates it. The decision is recorded
  and the corpus refuses to pretend the handler is measured meanwhile.
