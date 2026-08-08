# WorkerEndpoint — `takoform_worker_endpoint`

## Workload and consumer

A team wants their worker reachable, now, and does not yet own a domain — or
owns one and does not want the first request to wait on delegation and
verification. External clients reach the worker's active deployment over HTTPS
at an address the host assigns and publishes; TLS termination and the naming
scheme are the host's obligation, never portable state.

## Role

`attachment`. Inward activation is an attachment resource, never a binding
(decision 0010). Deleting the attachment removes the address and never deletes
the worker.

## Observable semantics

`worker` is the only desired member, and it is immutable: changing it replaces
the attachment. Requests at the assigned address invoke the worker's `fetch`
handler, resolved through its ACTIVE DEPLOYMENT, so promotion and rollback move
what answers without the endpoint being re-applied and without the address
changing.

The address is published as outputs: `hostname` is the assigned DNS name, and
`url` is exactly `https://` + that hostname + `/`. A portable author may rely on
a value being returned, on the scheme being HTTPS, and on the address routing to
the active deployment. They may rely on nothing about its shape — which
subdomain, which apex, how long the label is — and must not parse it, assert a
suffix, or reconstruct it from the resource name.

A worker has at most one endpoint. A host that cannot assign an address refuses
the attachment with `unsupported_capability` rather than answering with an
address it did not assign.

## Why this is one Form

Reachability without a name of one's own is one complete observable fact: the
worker answers at an address, over HTTPS, at the path root. Nothing about it is
a variant of serving a customer-owned hostname — the desired states are
disjoint, one carrying a hostname and the other carrying none — so merging the
two would need a selector token between two different requests.

## What would require a separate Form

A name the author owns is `WorkerCustomDomain`. Path-pattern or zone-scoped
routes carry matching semantics beyond a whole address and are a separate
attachment Form (`WorkerRoute` in the family plan,
[spec/form-families.md](../../spec/form-families.md)). An address with a chosen
label, a reserved name, or a regional affinity would put host placement into
desired state and is not a Form at all.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

The assigned address must be stable for the life of one incarnation: a consumer
holding the URL must not find it moved by a deployment change it did not make.
A second endpoint against one worker must be refused deterministically, by the
worker's UID rather than its name. Deleting the worker while the endpoint exists
must fail with `dependency_in_use`. Import must recover the worker reference and
the assigned address exactly, without minting a new one.

## Prior art

The vendor-subdomain endpoint of a proven edge platform, with the subdomain
scheme kept host-side and the address published as contract rather than
inferred.
