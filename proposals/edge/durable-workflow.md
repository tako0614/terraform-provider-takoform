# DurableWorkflow — `takoform_durable_workflow`

## Workload and consumer

An application team runs multi-step, long-lived operations — provisioning
sequences, payment pipelines, human-in-the-loop approvals — as durable code: a
workflow class exported by a worker's module, whose progress survives process
death. Workers, including the workflow's own worker, create, query, and signal
workflow INSTANCES through `module-worker.workflow` bindings; instances are
runtime data, never Resources. The Form enters the family at the v1beta2
generation as a `0.1.0` Experimental member
([decision 0043](../../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md),
[decision 0046](../../spec/decisions/0046-stable-arrives-through-a-stable-grade-beta-2.md)).

## Role

`identity`, a member of the worker aggregate. Desired state is a `worker`
reference to one [ModuleWorker](module-worker.md) plus `className`, a class
export name in the JavaScript identifier grammar; the reference pins the exact
`ModuleWorker` contract
([decision 0022](../../spec/decisions/0022-relations-pin-the-target-contract.md)).
The Form carries no implementation snapshot: which code answers is whatever
the worker's ACTIVE DEPLOYMENT selects
([decision 0016](../../spec/decisions/0016-the-worker-aggregate-has-one-active-deployment.md)),
so behavior upgrades and rollback ride [WorkerDeployment](worker-deployment.md).
One worker UID carries at most one DurableWorkflow per class name; a second
fails `invalid_argument` (400) before any mutation — a count no schema can
state, exactly like one-deployment-per-worker.

## Observable semantics

Exactly the `worker.workflow@1.0.0` contract, declared the way `ModuleWorker`
declares `worker.runtime@1.0.0`
([decision 0019](../../spec/decisions/0019-the-module-worker-abi-is-an-exact-contract.md)):
the identity is what a host implements; the class the serving version exports
is the code that fills it. One contract carries both the entrypoint ABI and
the instance surface, because what `create` accepts is what `run` receives and
what `sendEvent` sends is what `waitForEvent` resolves with.

The entrypoint ABI: the module every weighted version of the active deployment
selects exports `className`; the host constructs it and invokes
`run(event, step)` once per instance, where `event` carries the instance id
and the data-only JSON `params` the creator passed. `step.do(name, fn,
retryPolicy)` runs `fn` and durably records its JSON result under `name`.
`step.sleep(name, seconds)` parks the instance — no execution context while
sleeping — and the host wakes it at-least-once, never early.
`step.waitForEvent(name, type, timeoutSeconds)` parks the instance until a
matching sent event resolves it, or fails the step at the timeout. The timeout
is REQUIRED: with bounded sleeps, retries, and waits, every instance's
lifetime is bounded by construction, which keeps the delete refusal below a
delay rather than a deadlock. The contract's `limits` fix the portable
ceilings — sleep length, wait timeout, params size, retention — so a bound is
a contract fact, not host tuning.

Execution is at-least-once per step with memoized replay. An attempt that dies
after its effect but before its record commits re-executes, so `fn` must be
idempotent — the obligation [AtLeastOnceQueue](at-least-once-queue.md) already
places on consumers. A COMPLETED step's recorded result is returned on every
later execution without running `fn` again; the memo is keyed by step name, so
step names are unique per instance history by construction — a recurring name
IS the same step, and distinct work under one name is unobservable. The retry
policy is per `step.do` call, in code: maximum attempts, initial delay,
constant or exponential backoff, maximum delay. Exhausted retries fail the
step; an uncaught throw from `run` moves the instance to `errored`.

The contract seals step RESULTS, not arbitrary code effects: code between
steps re-executes on every replay, so it must be side-effect-free and
deterministic against its own recorded history. Every execution context an
instance gets is created from the deployment's then-current selection, exactly
like a request, so a promotion mid-instance changes the code the history
replays under; authors evolve workflow code compatibly with in-flight
instances, as they evolve any at-least-once consumer. The host can verify
neither obligation; the contract states both honestly.

Instances have unbounded cardinality and runtime addressing — like a queue's
messages, nothing about them enters desired state, the resource envelope, or a
plan: no uid, no generation, nothing for a provider to manage. The
`module-worker.workflow` binding states the surface a consumer calls:
`env.NAME.create({id?, params?}) -> Promise<WorkflowInstance>` admits an
author-chosen instance id, mints one when absent, and rejects an id a retained
instance already holds; `env.NAME.get(id)` rejects when no instance holds the
id. A `WorkflowInstance` carries `id`; `status() -> Promise<{status, output?,
error?}>` over the closed vocabulary `queued`, `running`, `sleeping`,
`waiting`, `complete`, `errored`, `terminated`; `sendEvent({type, payload}) ->
Promise<void>`, resolving when the event is durably retained — held until a
matching `waitForEvent` or a terminal state — and rejecting against a terminal
instance; and `terminate() -> Promise<void>`. A rejection carries the
contract's closed per-operation error code; a call rejects only when the
operation could not be performed.

Readiness follows the aggregate: `Ready=True` only while the active deployment
exists and EVERY weighted version exports `className` satisfying the ABI;
otherwise `Ready=False` with `Provisioning` (no deployment) or
`UnsupportedCapability` and a `hostReason` naming the class — a stable
readiness failure, never a silent fallback, rendered from other resources and
so moving `metadata.revision`, never `metadata.generation` (decision 0016).
Creating the Form while a live deployment visibly lacks the class fails
`unsupported_capability` (422) before any mutation; before any deployment it
stores `Provisioning`, and `create` on an unserved workflow rejects. Host
capability is decided at plan time
([decision 0031](../../spec/decisions/0031-host-capability-is-decided-at-plan-time.md)),
and a client SHOULD refuse a bundle that demonstrably lacks the export.

## Why this is one Form

Memoized replay is the contract application code is written against: which
effects are sealed and which re-execute decide how a workflow must be
authored, as delivery guarantees decide how a consumer must be. An
execution-model selector would make one desired document mean different
durability promises on different hosts
([decision 0008](../../spec/decisions/0008-forms-preserve-service-shape.md)).
Worker, class, and instance namespace travel together because results persist
under the identity while code arrives through the worker's deployment;
splitting them would let two hosts attach one instance history to observably
different code.

## What would require a separate Form

A declarative state-machine workflow — a state language the host interprets —
is the `workflow` family candidate decision 0043 records: a different
authoring model, not a variant. Exactly-once step execution, an
instance-pins-its-creating-revision lifecycle, and a control surface for
cross-instance query or batch signalling each change the promises above.

## Provided Interfaces

`worker.workflow@1.0.0` — the entrypoint ABI a conforming host provides to the
workflow class and the instance surface the binding projects. It is held by
the IDENTITY because the identity is what a host implements; the class a
Worker Version exports is the code that fills it.

## Accepted Bindings

None; it is a binding target (`module-worker.workflow`). The class's own code
runs inside its worker's environment, so every capability it uses is a binding
the serving `WorkerVersion` declares
([decision 0010](../../spec/decisions/0010-exact-interface-and-binding-contracts.md));
the v1beta2 `WorkerVersion` carries `workflowBindings` as one more annotated
list, whose names join the single environment namespace with no host edit.
Unlike `module-worker.service`, the binding does not require its target
`Ready` at bind time: a workflow's readiness follows its own worker's
deployment, which cannot exist before the version it weights, so a Ready gate
would make the ordinary self-bound wiring unconstructible in one apply. The
binding requires existence and the exact Interface; the deployment that lands
next is what makes the workflow serve.

## Lifecycle risks

Deleting a DurableWorkflow while any instance is live (`queued`, `running`,
`sleeping`, `waiting`) fails `dependency_in_use` (409), naming the count: an
instance is an execution the host promised to finish, and the required wait
timeout bounds the refusal to a delay. Terminal history is destroyed with the
Form, like a queue's retained messages. Deleting the referenced worker, or
the Form while any live `workflowBindings` entry pins it, fails
`dependency_in_use` under the ordinary relation rule. The Form joins the
deployment fence of decision 0016: an apply that would leave a live
DurableWorkflow's class unexported, and a deployment delete while one lives,
are refused — the fence is what funds "instances survive process death",
because a live instance always has code to replay into. Teardown is ordered
and discoverable from the refusals.

## Prior art

The code-defined durable-execution product of a proven edge platform, retained
without its vendor, account, or commercial surface. Decision 0043's survey
places the durable-execution category on the vendor-locked side — no de-facto
standard API exists — and this Form fixes that platform's shape of it.
