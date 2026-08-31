# ContainerEndpoint — `takoform_container_endpoint`

> Historical/deferred candidate (English-only). This non-Edge Form is not in
> the current official Edge16 corpus or Current navigation.

## Workload and consumer

A team wants their service reachable, now, and does not yet own a domain —
or owns one and does not want the first request to wait on delegation and
verification. External clients reach the service's active traffic split over
HTTPS at an address the host assigns and publishes; TLS termination and the
naming scheme are the host's obligation, never portable state.

## Role

`attachment`. Inward activation is an attachment resource, never a binding
(decision 0010). Deleting the attachment removes the address and never
deletes the service.

## Observable semantics

`service` is the only desired member, and it is immutable: changing it
replaces the attachment. Requests at the assigned address are delivered to
instances of the revisions the service's ACTIVE TRAFFIC resource selects, so
promotion and rollback move what answers without the endpoint being
re-applied and without the address changing; each request is served entirely
by one revision.

The address is published as outputs: `hostname` is the assigned DNS name,
and `url` is exactly `https://` + that hostname + `/`. A portable author may
rely on a value being returned, on the scheme being HTTPS, and on the
address routing through the active traffic split — and on nothing about its
shape: they must not parse it, assert a suffix, or reconstruct it from the
resource name
([decision 0024](../../spec/decisions/0024-a-worker-is-reachable-at-a-host-assigned-address.md),
[decision 0025](../../spec/decisions/0025-declared-outputs-are-a-typed-contract.md)).

A service has at most one endpoint. A host that cannot assign an address
refuses the attachment with `unsupported_capability` rather than answering
with an address it did not assign.

## Why this is one Form

Reachability without a name of one's own is one complete observable fact:
the service answers at an address, over HTTPS, at the path root. Nothing
about it is a variant of serving an author-owned hostname — the desired
states are disjoint — so merging the two would need a selector token between
two different requests.

## What would require a separate Form

A name the author owns is
[ContainerCustomDomain](container-custom-domain.md). An address with a
chosen label or a regional affinity would put host placement into desired
state and is not a Form at all.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

The assigned address must be stable for the life of one incarnation: a
consumer holding the URL must not find it moved by a traffic change it did
not make. A second endpoint against one service must be refused
deterministically, by the service's UID rather than its name. Attaching
while the service has no traffic resource must be refused, per the family
aggregate. Deleting the service while the endpoint exists must fail with
`dependency_in_use`. Import must recover the service reference and the
assigned address exactly, without minting a new one.

## Prior art

The host-assigned service URL of a proven serverless container platform,
with the naming scheme kept host-side and the address published as contract
rather than inferred.
