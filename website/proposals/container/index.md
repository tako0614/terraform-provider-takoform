# Serverless Container Family proposals

The Serverless Container Family, `container.forms.takoform.com`, is
one of the eight families of the v1 lineup
([decision 0043](../../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md)).
Its members fix, completely, the application-visible semantics of the
request-driven serverless container model — immutable revisions minted from
digest-pinned OCI images, request-driven autoscaling with a concurrency
target, scale-to-zero, and basis-point traffic splitting across revisions —
that every major cloud offers and no de-facto standard API covers, without
naming any vendor
([decision 0008](../../spec/decisions/0008-forms-preserve-service-shape.md)).

## Authoring policy: shape-preserving contracts

Every member Form preserves one service shape end to end: process contract,
instance lifecycle, scaling semantics, update and delete units, error
semantics, and the capabilities a revision may reach. No free semantic token
is admitted; a difference in semantics is a different Form, never a selector
value. Outward capability use belongs to the revision resource; inward
activation is an attachment resource
([decision 0010](../../spec/decisions/0010-exact-interface-and-binding-contracts.md)).
Desired schemas carry no `name` or envelope plumbing: the resource envelope
owns identity and status
([decision 0011](../../spec/decisions/0011-resource-identity-generation-and-revision.md)).

The family is minted after
[decision 0045](../../spec/decisions/0045-external-standard-services-are-sealed-slots.md),
so its revision Form carries the external standard-service declaration from
birth ([spec/standard-services](../../spec/standard-services/index.md)): a
Container Revision reaches PostgreSQL, Redis, S3-compatible storage, or SMTP
through sealed slots with opaque reverse-DNS protocol identifiers, never through endpoints or
credentials in portable state — which is also how the family serves stateful
applications without growing the database Forms decision 0043 swore off.

These prose documents accompany five generated Experimental `0.x` candidates
under `forms/candidates/container.forms.takoform.com`. The exact candidate set,
Definitions, Interface digest, and packages — not proposal prose — own their
current identities. Editing this directory reserves or changes no identity.

## The service aggregate

A Container Service has at most ONE ContainerTraffic; that traffic resource
selects Container Revisions of its own identity with basis-point weights
summing to exactly 10000; every attachment is admitted against it and
refused when it is absent; and the identity reports itself Ready only while
its traffic resource actually serves — the same aggregate statement the Edge
family makes in
[decision 0016](../../spec/decisions/0016-the-worker-aggregate-has-one-active-deployment.md).

## Current members

| Form | Role | One-line semantics | Separate-Form boundary |
| --- | --- | --- | --- |
| [ContainerService](container-service.md) | identity | Logical identity of one request-driven container service; the runtime contract is fixed by identity. | A run-to-completion job or an event-invoked function is a different Form. |
| [ContainerRevision](container-revision.md) | revision | Immutable serving snapshot: OCI image by digest, command/args, vars, sensitive slots, external services, resources, concurrency, instance bounds. | Mutable in-place config or a host-built image is not a revision of this Form. |
| [ContainerTraffic](container-traffic.md) | deployment | Basis-point traffic split across up to eight Container Revisions of one service. | Tagged revision URLs or per-request routing are separate work. |
| [ContainerEndpoint](container-endpoint.md) | attachment | Makes the service reachable over HTTPS at an address the host assigns and publishes. | A name the author owns is `ContainerCustomDomain`. |
| [ContainerCustomDomain](container-custom-domain.md) | attachment | Serves one DNS hostname from the service's active traffic split over HTTPS. | Path-pattern routing is a separate attachment Form. |

Designs that differ in these semantics — run-to-completion jobs, addressable
per-instance actors, host-built source deploys — belong to other families or
later members and get their own proposals when that work starts, per
[spec/form-families.md](../../spec/form-families.md).
