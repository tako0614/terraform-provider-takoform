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

The identity fixes the ES Module Worker ABI: handlers are functions exported
from an ES module (`fetch`, `scheduled`, `queue`, `tail`), invoked with typed
events and an environment object carrying the revision's declared bindings.
An instance is stateless between events; durable state lives behind bindings.

## Why this is one Form

Every consumer of the family programs against exactly this ABI. Splitting the
identity from the ABI would let two hosts attach the same deployments to
observably different runtimes.

## What would require a separate Form

A WASI function, an OCI container service, or an isolate model with a
different handler contract each preserve a different shape and are separate
Forms in other families.

## Provided Interfaces

None on the identity; `worker.service` is provided by [WorkerVersion](worker-version.md),
whose snapshot actually answers invocations.

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
