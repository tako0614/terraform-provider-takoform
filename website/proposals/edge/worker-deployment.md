# WorkerDeployment — `takoform_worker_deployment`

## Workload and consumer

An operator moves traffic between Worker Versions of one Module Worker:
canary a new version at a small share, promote it, or roll back — all by
re-weighting, never by mutating a revision.

## Role

`deployment`. This is the only mutable path for traffic movement.

## Observable semantics

`versions` lists one to eight entries of `workerVersion` plus `weight` in
basis points. Weights must sum to exactly 10000 across entries; the sum is
host-validated semantics because a JSON schema cannot add weights. Each
request is served entirely by one selected version; a client never observes a
blend of two versions within one request.

## Why this is one Form

Version selection is a single atomic fact per worker. Splitting weights
across resources would allow interleavings in which the observable sum is
temporarily wrong.

## What would require a separate Form

Per-request routing rules, header- or cookie-pinned sessions, and geographic
steering carry different selection semantics and belong to separate Forms.

## Provided Interfaces

None.

## Accepted Bindings

None; the deployment selects revisions, it does not use capabilities.

## Lifecycle risks

A deployment referencing a deleted version must fail closed. Rollback is
re-weighting to a previous version and must not require recreating it.
Deleting the deployment stops traffic selection but never deletes the worker
or its versions.

## Prior art

The gradual-rollout deployment model of a proven edge platform, reduced to
its portable core: exact basis-point weights over immutable versions.
