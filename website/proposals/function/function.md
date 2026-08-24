# Function — `takoform_function`

## Workload and consumer

An application team runs event-invoked code as one regional function on a
proven cloud function service: upload an artifact, declare a handler, and
the platform runs one handler invocation per event. Consumers address the
function by its logical identity: deployments, endpoints, and future
source-family attachments all point at this resource, never at a specific
build.

## Role

`identity`. The Form carries no desired fields: everything that can change —
code, configuration, resource bounds, traffic — lives on revision and
deployment resources around it.

## Observable semantics

The identity fixes the regional function ABI, and says exactly what that ABI
is: the exact Interface contract `function.runtime@1.0.0`, authored the way
`worker.runtime@1.1.0` fixes the ModuleWorker ABI
([decision 0019](../../spec/decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).
It is a JavaScript ES-module function runtime, deliberately distinct from
`worker.runtime`:

- The unit of execution is one invocation. The main module's declared
  handler is a named export invoked as `handler(event, context)`; an
  execution environment handles ONE invocation at a time, so concurrency
  comes only from parallel environments, never from interleaving within one.
- The event vocabulary is closed and every event names its kind. This
  version defines exactly the `http` event, delivered by
  [FunctionEndpoint](function-endpoint.md); an event kind no attachment can
  activate does not enter the contract until the attachment ships beside it,
  in a new exact version — the same rule decision 0019 fixes for the worker.
- The wall-clock budget is the invoking version's declared timeout, a
  regional budget measured in minutes rather than the colocated-request
  budget of an edge fetch event; `context` reports the invocation id and the
  remaining budget.
- Nothing is promised about proximity. The contract carries no colocation
  guarantee between caller and execution — the edge family's model — and no
  region field: placement is a host decision, not portable state.
- Work the handler does not await is not guaranteed to run. There is no
  `waitUntil`; the environment may be suspended the moment the handler
  settles.
- Environment reuse across invocations is permitted and never promised;
  durable state lives behind the revision's external services.
- The contract also fixes the environment projection (vars, sensitive slots,
  and per-protocol standard-service members, nothing else portable),
  exception handling, the minimum JavaScript and Web API surface, and module
  loading.

There is no compatibility date and no compatibility flag, for the reason the
Edge family records: a date names no behavior without a registry saying what
each date changes. A runtime revision is a new exact Interface version —
and, if it changes what a Form desires, a new Form version. A different
language runtime is not a version of this contract at all: it is a different
Form line, exactly as a WASI function is to
[ModuleWorker](../edge/module-worker.md).

## Why this is one Form

Every consumer of the family programs against exactly this ABI. Splitting
the identity from the ABI — the withdrawn epoch's open `runtime` token —
would let two hosts attach the same deployments to observably different
runtimes.

## What would require a separate Form

A different language runtime is a different Form line. Cross-family event
sources — queue-, topic-, or schedule-driven invocation — are attachments
declared in the source's own family, never members here; the Edge family's
`QueueConsumer` stays worker-targeted, and scheduled invocation belongs to the
separate current Schedule family. A source-specific object event would require
its own future contract; there is no current `ObjectBucket` Form.
Function-to-function
invocation would be its own `function.invoke` Interface and binding
contract, proposed when that work starts.

## Provided Interfaces

`function.runtime@1.0.0` — the runtime ABI a conforming host provides to
this function's code. It is held by the identity because the identity is
what a host implements; a Function Version is the code that fills it.

## Accepted Bindings

None. Capability bindings belong to revision resources (decision 0010), and
[FunctionVersion](function-version.md) states why the MVP revision accepts
none either.

## Lifecycle risks

Deleting the identity while versions, deployments, or attachments reference
it must fail with `dependency_in_use`. Delete/recreate mints a new UID
(decision 0011); stale references never rebind silently.

## Prior art

The regional function service every major cloud offers — uploaded artifact,
declared handler, per-invocation lifecycle — retained without vendor,
account, or commercial surface. The withdrawn v1alpha2 `EdgeWorker`
candidate is prior art in part; its open `runtime` token is the defect this
identity replaces by fixing one exact runtime contract per Form line.
