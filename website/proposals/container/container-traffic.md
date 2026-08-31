# ContainerTraffic — `takoform_container_traffic`

> Historical/deferred candidate (English-only). This non-Edge Form is not in
> the current official Edge16 corpus or Current navigation.

## Workload and consumer

An operator moves traffic between Container Revisions of one service: canary
a new revision at a small share, promote it, or roll back — all by
re-weighting, never by mutating a revision.

## Role

`deployment`. This is the only mutable path for traffic movement.

## Observable semantics

`service` is the immutable reference naming the ContainerService this traffic
resource governs, and every listed revision MUST belong to it: an entry whose
revision resolves to a different service's aggregate is `invalid_argument`
before any mutation, decided against resolved UIDs. At most one traffic
resource governs one service incarnation.

`revisions` lists one to eight entries of `containerRevision` plus `weight`
in basis points. Weights are positive and must sum to exactly 10000 across
entries — the same host-validated rule as the Edge family's
`WorkerDeployment`, because a JSON schema cannot add weights. Each request
is served entirely by one selected revision; a client never observes a blend
of two revisions within one request. A revision not listed receives no
traffic, and its instance bounds are not in force.

A Container Service has at most one ContainerTraffic. Attachments are
admitted against it and refused when it is absent; deleting it while an
attachment lives is refused; and the identity reports Ready only while its
traffic resource serves — the family's aggregate statement, mirroring
[decision 0016](../../spec/decisions/0016-the-worker-aggregate-has-one-active-deployment.md).

## Why this is one Form

Revision selection is a single atomic fact per service. Splitting weights
across resources would allow interleavings in which the observable sum is
temporarily wrong.

## What would require a separate Form

Named revision URLs — a tagged revision addressable outside the split —
carry a second activation surface and are a separate attachment Form.
Header- or cookie-pinned sessions, per-request routing rules, and automated
progressive-rollout policies carry different selection semantics and belong
to separate Forms.

## Provided Interfaces

None.

## Accepted Bindings

None; the traffic resource selects revisions, it does not use capabilities.

## Lifecycle risks

A traffic resource referencing a deleted or absent revision must fail closed
(`resource_not_found`, and dependency protection keeps a weighted revision
undeletable). Rollback is re-weighting to a previous revision and must not
require recreating it. Deleting the traffic resource stops selection but
never deletes the service or its revisions, and is refused with
`dependency_in_use` while an attachment lives.

## Prior art

The revision traffic split of a proven serverless container platform,
reduced to its portable core: exact basis-point weights over immutable
revisions, one active selection per service.
