# ModuleWorker — `takoform_module_worker`

## Workload and consumer

An application team runs an HTTP/event application as one ES Module Worker on
a proven edge platform. Consumers address the worker by its logical identity:
deployments, domains, triggers, consumers, and service bindings all point at
this resource, never at a specific build.

## Role

`identity`. The Form carries no desired fields: everything that can change —
code, configuration, bindings, traffic — lives on revision and deployment
resources around it.

## Observable semantics

The identity fixes the ES Module Worker ABI, and says exactly what that ABI is:
the exact Interface contract `worker.runtime@1.1.0`
([decision 0019](../../spec/decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).
That contract fixes the module's default-export shape and that a declared
handler must exist; the `fetch`, `scheduled`, and `queue` signatures and
what each event carries; the `env` object (every declared binding, var, and
sensitive-variable slot, and nothing else portable); `ctx.waitUntil`; exception
handling; request and response body streaming; the minimum Web API surface; and
which module media types load, which a bundle merely carries without ever
importing them, and how a WASM module is instantiated. An instance
is stateless between events; durable state lives behind bindings.

There is no compatibility date and no compatibility flag. A date selects
behavior only against a registry that says which behavior each date changes,
this project has none, and a Form field that promises portability it cannot
deliver is the incompleteness `portability-boundary.md` forbids. A runtime
revision is a new exact contract version — and, if it changes what this Form
desires, a new Form version.

## Why this is one Form

Every consumer of the family programs against exactly this ABI. Splitting the
identity from the ABI would let two hosts attach the same deployments to
observably different runtimes.

## What would require a separate Form

A WASI function, an OCI container service, or an isolate model with a
different handler contract each preserve a different shape and are separate
Forms in other families.

## Provided Interfaces

`worker.runtime@1.1.0` — the runtime ABI a conforming host provides to this
worker's code. It is held by the IDENTITY because the identity is what a host
implements; a Worker Version is the code that fills it.

`worker.service@1.0.0` — worker-to-worker `fetch` invocation. It is held by the
identity too: a `module-worker.service` binding addresses another worker by
logical identity, and whichever versions that worker's active deployment selects
are what answer.

## Accepted Bindings

None. Capability bindings belong to revision resources (decision 0010).

## Lifecycle risks

Deleting the identity while versions, deployments, or attachments reference
it must fail with `dependency_in_use`. Delete/recreate mints a new UID
(decision 0011); stale references never rebind silently.

## Prior art

The module-worker execution model of a proven edge platform, retained here
without its vendor, account, or commercial surface. The retained v1alpha2
`EdgeWorker` candidate is prior art that this family supersedes for new
design work.
