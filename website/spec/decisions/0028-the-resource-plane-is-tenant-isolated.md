# 0028 — The resource plane is tenant-isolated, by address

- Status: accepted
- Date: 2026-08-09
- Owners: Takoform maintainers

## Context

Every other boundary in the v1alpha3 lane is stated and measured. An Operation
id and an upload session are handles bound to the tenant and principal that
created them, and a caller who is not the owner is answered as if they did not
exist. A content address is a name and not an entitlement: a manifest or a blob
is readable only by a tenant that holds it, and a referenced manifest is resolved
against the mutating caller's tenant on apply, on import, and again when an
accepted `202` commits ([decision 0018](0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)).
A hostname claim is unique per tenant and deliberately stops there
([decision 0026](0026-attachment-claims-are-canonical-and-acyclic.md)).

The resource plane itself was not. The reference host's internal key was
`space + group + kind + name`, with no tenant in it. Each record remembered the
tenant that created it, but as a field the hostname scan read — not as part of
the address. The host said so in its own comment: a real host partitions by
tenant, and this corpus does not measure it.

So two tenants naming one resource in one space addressed one record. The second
tenant's `If-None-Match: *` create collided with a resource it could not read; a
`read`, a fenced update, and a fenced delete all reached it; a reference from one
tenant resolved to another tenant's resource whenever the names agreed, and
pinned its uid into stored state; a `prepareDigest` minted by one tenant was
byte-identical to the other's and spendable by it; and the derived-rendering pass
that advances revisions after a mutation ran across every tenant in the space.

Nothing in the lane forbade a host from shipping exactly that. An independent
host is about to be written against this contract, and a boundary that is
documented but unmeasured is not a boundary — it is a thing the first
implementer discovers, or does not.

## Decision

**A host's internal resource address includes the tenant, the space, the
`apiVersion`, the `kind`, and the `name`.** The tenant is the authenticated
tenant of the request, recorded at create and carried forward by every update and
import, exactly as the recorded exact FormRef is
([decision 0011](0011-resource-identity-generation-and-revision.md),
[decision 0022](0022-relations-pin-the-target-contract.md)).

1. Two tenants may create one `{space, apiVersion, kind, name}` and get two
   resources with two host-issued uids. A name is taken within one tenant.
2. Every resource surface addresses the caller's own tenant. A request naming
   another tenant's resource fails `resource_not_found` (404), and is
   indistinguishable from one naming a resource that never existed.
3. Relations resolve only within the same tenant. A name only another tenant
   holds is an absent target.
4. A `prepareDigest` binds the minting tenant and is not spendable outside it.
   An apply presenting a foreign review fails `invalid_argument` (400).
5. An `Idempotency-Key` names one operation of one tenant. The same key from two
   tenants is two operations.
6. A pass that renders one resource from others, and advances revisions, is
   scoped to the mutating tenant.
7. **One exception, unchanged:** a `WorkerCustomDomain`'s canonical hostname is
   unique across every space of one tenant, because DNS does not partition with
   spaces. That rule drops the space and keeps the tenant. Nothing reaches past
   the tenant.

The error code is the closed taxonomy's `resource_not_found` for every
addressing refusal, and `permission_denied` is forbidden for them. A foreign
tenant's resource must be indistinguishable from one that does not exist, and a
403 is a disclosure: it says "a resource of that name exists on this host and is
not yours" to anyone who can guess a name, which is precisely the membership fact
the boundary withholds. This is the same reasoning decision 0018 already applied
to Operation ids, upload sessions, and content addresses, applied to the plane
those three surround. The taxonomy is closed and no code is added
([decision 0014](0014-published-schemas-are-structural-minima.md), rung 3: the
host enforces it and required conformance checks prove a laxer host fails).

Seven required conformance checks measure it, all black box:
`resource-address-is-tenant-scoped`, `resource-read-is-tenant-isolated`,
`resource-update-is-tenant-isolated`, `resource-delete-is-tenant-isolated`,
`relation-resolution-is-tenant-scoped`, `prepare-is-tenant-scoped`, and
`idempotency-is-tenant-scoped`. The lane's runner therefore REQUIRES an
alternate-tenant credential: a runner that can authenticate as one tenant can
measure none of this.

## Consequences

- The reference host keys `resources` by tenant first, and every store lookup,
  every store-wide scan, and the derived-rendering pass take a `resourceScope`
  value rather than a bare space. A key cannot be built without a tenant, so a
  host-wide question is not one this code can ask by accident — which is exactly
  how the previous boundary became unmeasured.
- `cross-principal-idempotency-isolation` changes what it expects from the
  alternate tenant. It used to expect a `generation_conflict`, which was the
  second tenant colliding with the first tenant's record — the defect, recorded
  as evidence of isolation. It now expects the two legs to differ: the
  same-tenant principal still collides, and the second tenant is stopped by the
  review it carries rather than by a resource it does not have.
- The required check list grows from 93 to 100.
- Two obligations stay unproven by the lane and are stated as such in
  [`../host-api/v1alpha3.md`](../host-api/v1alpha3.md): that the tenant is in the
  address rather than in a late filter, and that an unscoped derived-rendering
  pass would cross the tenant. Neither is reachable black box — the second
  because relation resolution refuses to build the case. Both are held by
  host-side tests against the reference host.
- Publication blocker **V3-012** records the item as P0 for every Form.

## Rejected alternatives

- **Answer `permission_denied` (403) for another tenant's resource.** Rejected
  because it is a membership oracle over every name on the host, and because it
  contradicts the answer decision 0018 already fixed for the three handle-shaped
  surfaces. Two codes for one class of fact would also let two hosts differ on
  which they return.
- **Keep the tenant as a field and filter at each call site.** Rejected because
  that is the state this decision corrects. The tenant WAS a field, and exactly
  one scan read it; every other lookup was correct only by the accident of a uid
  being unguessable. A filter is something a future edit forgets, and the failure
  is silent.
- **Scope only the resource key, and leave the prepare binding global.** Rejected
  because a create binding pins no uid and no generation: two tenants preparing
  one name in one space would hold one digest, so the review would be a bearer
  token over another tenant's plane. It is also the cheapest thing to get wrong,
  because everything else keeps working.
- **Put the tenant on the wire, in `metadata` or as a query key.** Rejected
  because a client that can name a tenant can name someone else's, which turns
  authentication into a suggestion; and because it would make the tenant part of
  the portable resource identity, which it is not — the same desired state must
  apply unchanged in any tenant of any host
  ([decision 0003](0003-space-id-is-an-opaque-portable-scope-identity.md)).
- **Scope the hostname claim to the tenant AND the space, for uniformity.**
  Rejected because it would make the rule wrong. Two spaces of one tenant
  claiming one DNS name is one collision with two answers, and decision 0026
  decided it; uniformity is not a reason to give a host two answers for one
  hostname.
