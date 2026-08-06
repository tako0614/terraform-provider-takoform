# WorkerVersion — `takoform_worker_version`

## Workload and consumer

A team ships one executable snapshot of a worker: which bundle runs, under
which runtime compatibility date and flags, which handlers it exports, its
non-secret vars, and the typed capability bindings its code may use.
Deployments select among versions; nothing ever edits a version in place.

## Role

`revision`. Every field is immutable; a change is a new Worker Version.

## Observable semantics

`compatibilityDate` fixes default runtime behavior; `compatibilityFlags` is a
closed set (`nodejs_compat`). `handlers` closes the event surface a host may
attach to. `vars` is a bounded data-only JSON map projected into the module
environment. Each binding list projects one exact Binding contract —
edge KV, object bucket, SQLite, queue producer, service — under a JavaScript
identifier name. `requiredSensitiveVars` declares only the names of sealed
values the host must supply; values never enter portable state.

The field is named `requiredSensitiveVars`, not `secretRequirements`, because
the Form Package data-only policy forbids the token `secret` in any field
name (`formpackage` rejects the whole definition rather than the value), so
the declaration states the same fact in permitted vocabulary.

## Deferred: static assets

This milestone has no `assets` field. Static assets served alongside a worker
are the separate `StaticAssetBundle` Form of a later milestone
(`spec/form-families.md`); `WorkerVersion` gains an `assets` reference to it
when that Form lands, as one new optional field on a new definition version.
Until then this Form cannot express an asset-serving worker, and no host
should infer one.

## Why this is one Form

Code, runtime behavior, and granted capabilities must travel as one immutable
unit, or rollback cannot be exact: re-activating an old version must restore
exactly the bindings and configuration it was verified with.

## What would require a separate Form

Mutable in-place configuration, a per-environment overlay model, or bindings
resolved at request time each break the immutable-snapshot shape.

## Provided Interfaces

`worker.service@1.0.0` — the version answers worker-to-worker `fetch`
invocations projected by `module-worker.service` bindings.

## Accepted Bindings

`module-worker.edge-kv`, `module-worker.object-bucket`,
`module-worker.sqlite`, `module-worker.queue-producer`,
`module-worker.service`, each at 1.0.0 with its exact schema digest.

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
