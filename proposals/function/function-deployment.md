# FunctionDeployment — `takoform_function_deployment`

> Historical/deferred candidate (English-only). This non-Edge Form is not in
> the current official Edge16 corpus or Current navigation.

## Workload and consumer

An operator moves traffic between Function Versions of one Function: shift a
share of invocations to a candidate version, promote it, or roll back — all
by re-weighting, never by mutating a revision.

## Role

`deployment`. This is the only mutable path for traffic movement.

## Observable semantics

`function` is the immutable reference naming the Function this deployment
governs, and every listed version MUST belong to it: an entry whose version
resolves to a different function's aggregate is `invalid_argument` before any
mutation, decided against resolved UIDs, never names — the Edge family's
deployment-integrity rule verbatim. At most one deployment governs one
function incarnation.

`versions` lists one or two entries of `functionVersion` plus `weight` in
basis points. Every weight is positive — a zero-weight entry is not a smaller
split but a version the deployment does not select, and the Edge family's
`WorkerDeployment` already refuses it — and weights must sum to exactly 10000
across entries; the sum is host-validated semantics because a JSON schema
cannot add weights. A single-version deployment is one entry at 10000. Each
invocation is served entirely by one selected version; a caller never
observes a blend of two versions within one invocation.

Two entries, not eight: the proven regional function service shifts traffic
between one stable version and one candidate, and this family preserves that
shape rather than importing the Edge family's eight-way split. A wider split
would be a new exact version of this Form, argued from the shape it
preserves.

A Function has at most one FunctionDeployment. Attachments are admitted
against the deployment and refused when it is absent; deleting the
deployment while an attachment lives is refused; and the identity reports
Ready only while its deployment serves — the family's aggregate statement,
mirroring
[decision 0016](../../spec/decisions/0016-the-worker-aggregate-has-one-active-deployment.md).

## Why this is one Form

Version selection is a single atomic fact per function. Splitting weights
across resources would allow interleavings in which the observable sum is
temporarily wrong.

## What would require a separate Form

Named multi-alias routing — several concurrently addressable version mixes
per function — is a different shape: this family's attachments resolve
through THE active deployment. Per-request routing rules and session pinning
carry different selection semantics and are separate work.

## Provided Interfaces

None.

## Accepted Bindings

None; the deployment selects revisions, it does not use capabilities.

## Lifecycle risks

A deployment referencing a deleted or absent version must fail closed
(`resource_not_found`, and dependency protection keeps a weighted version
undeletable). Rollback is re-weighting to a previous version and must not
require recreating it. Deleting the deployment stops traffic selection but
never deletes the function or its versions, and is refused with
`dependency_in_use` while an attachment lives.

## Prior art

The weighted alias of a proven cloud function service, reduced to its
portable core: exact basis-point weights over immutable versions, one active
selection per function.
