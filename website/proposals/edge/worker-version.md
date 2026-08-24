# WorkerVersion — `takoform_worker_version`

## Workload and consumer

A team ships one executable snapshot of a worker: which bundle runs, which
handlers it exports, its non-secret vars, and the typed capability bindings its
code may use. Deployments select among versions; nothing ever edits a version in
place.

## Role

`revision`. Every field is immutable; a change is a new Worker Version.

## Observable semantics

The runtime a version runs on is not a field of this Form. It is the exact
`worker.runtime@1.1.0` contract the [ModuleWorker](module-worker.md) identity
provides, so there is no `compatibilityDate` and no `compatibilityFlags`
([decision 0019](../../spec/decisions/0019-the-module-worker-abi-is-an-exact-contract.md)):
a date names no behavior without a registry saying what each date changes, and
two hosts reading the same date could legitimately run different runtimes.

`handlers` closes the event surface a host may attach to, and its vocabulary IS
the handler set that contract defines — a host refuses a handler the contract
does not define, before any mutation. `vars` is a bounded data-only JSON map
projected into the module environment. Each current binding list projects one exact
Binding contract — edge KV, SQLite, queue producer, service, workflow, or actor —
under a JavaScript identifier name, and each of those contracts states the
JavaScript surface it projects, not just the operations it grants.
`requiredSensitiveVars` declares only the names of sealed values the host must
supply; values never enter portable state.

`externalServices` is not another typed Binding list. It carries sealed
`standards.takoform.com/v1` slots identified by opaque reverse-DNS protocol
strings; the Host resolves them without a target Resource or central protocol
enum. This is how current worker code may consume an externally managed
S3-compatible service. It does not revive the retained `ObjectBucket` Form.

`assets` is optional and absence is semantic: without it the host performs no
asset lookup. When present it is one closed object containing an exact
`StaticAssetBundle` reference, `runWorkerFirst`, and the closed
`notFoundHandling` value `none` or `single_page_application`. Asset-first and
worker-first request order, the 404 handoff, and the required `index.html` for
SPA fallback are fixed by
[decision 0033](../../spec/decisions/0033-edge-app-assets-and-sqlite-migrations-are-content-addressed.md).

The field is named `requiredSensitiveVars`, not `secretRequirements`, because
the Form Package data-only policy forbids the token `secret` in any field
name (`formpackage` rejects the whole definition rather than the value), so
the declaration states the same fact in permitted vocabulary.

## Deferred: a runtime revision

A future runtime revision is a new exact `worker.runtime` version, published at
its own digest. If it changes what this Form desires it is also a new definition
version of this Form. It is never a new value of a date field, and never a flag.

## Why this is one Form

Code, runtime behavior, and granted capabilities must travel as one immutable
unit, or rollback cannot be exact: re-activating an old version must restore
exactly the bindings and configuration it was verified with.

## What would require a separate Form

Mutable in-place configuration, a per-environment overlay model, or bindings
resolved at request time each break the immutable-snapshot shape.

## Provided Interfaces

None. Both of the worker's exact contracts — `worker.runtime@1.1.0` and
`worker.service@1.0.0` — belong to the [ModuleWorker](module-worker.md)
identity: a `module-worker.service` binding addresses a worker by logical
identity, and the runtime ABI is what a host implements rather than what one
snapshot ships.

## Accepted Bindings

`module-worker.actor`, `module-worker.edge-kv`, `module-worker.sqlite`,
`module-worker.queue-producer`, `module-worker.service`, and
`module-worker.workflow`, each at 1.0.0 with its exact schema digest. The
retained v1beta1 `module-worker.object-bucket` contract is not current.

## Lifecycle risks

Creating a version whose binding targets are absent must fail; deleting a
bound target must fail with `dependency_in_use` (refuse_while_bound).
Deleting a version still weighted by a deployment must fail. Import must
reproduce the exact snapshot, including binding names.

## Prior art

The versioned worker snapshot of a proven edge platform, with its binding
environment made an exact digest-bound contract per decision 0010. The
sensitive-value declaration mirrors that platform's sealed secret path while
keeping only names portable.
