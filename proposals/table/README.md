# Table Family proposals

::: warning Historical / deferred candidate family

`table.forms.takoform.com` is retained source from the earlier Provider
projection. It is not part of the current official corpus (the single Edge
family with 16 Forms), and this family is outside Current navigation. These
pages are English-only historical/deferred proposals.

:::

The Table Family, `table.forms.takoform.com`, fixes the document/KV
table shape — items addressed by a declared partition key — selected by
[decision 0043](../../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md).
Its member preserves, completely, the application-visible semantics of that
proven shape without naming its vendor
([decision 0008](../../spec/decisions/0008-forms-preserve-service-shape.md)).

## Why a family, not an integration

A category with a de-facto standard API is never respecified as a Form;
Takoform reaches it through a sealed standard-service slot instead
([spec/standard-services](../../spec/standard-services/README.md)). The
key-addressed table has no such standard: every major cloud offers one, each
speaking only its own provisioning and data API, which is exactly the
category decision 0043 assigns to a Form Family — the place where a
host-neutral contract is the only portability there is. The document store
that DOES have a standard wire, the Mongo-flavored one, sits on the
integrate side of that decision's survey and is not this family. This family
replaces the withdrawn v1alpha2 `KeyValueStore` and the key-addressed half
of `StatefulEntity` (the addressable-actor half became the Edge family's
`ActorNamespace` addition).

## Authoring policy: shape-preserving contracts

Every member Form preserves one service shape end to end: client API, data
model, consistency, update and delete units, error semantics, and the
capabilities exposed through typed Bindings. No free semantic token is
admitted; a difference in semantics is a different Form, never a selector
value. Outward capability use is a digest-bound Binding held by a revision
resource; inward activation is an attachment resource
([decision 0010](../../spec/decisions/0010-exact-interface-and-binding-contracts.md)).
Desired schemas carry no `name` or envelope plumbing: the resource envelope
owns identity and status
([decision 0011](../../spec/decisions/0011-resource-identity-generation-and-revision.md)).

This prose accompanies the generated Experimental `Table` candidate under
`forms/candidates/table.forms.takoform.com` and its exact `table.document`
Interface. The generated candidate bytes are retained historical/deferred
source, not the current publisher identity. Family membership grants no
maturity.

## Retained candidate member (historical/deferred)

| Form | Role | One-line semantics | Separate-Form boundary |
| --- | --- | --- | --- |
| [Table](table.md) | identity | Key-addressed document table: declared partition/sort keys, atomic conditional writes, consistent single-item reads, key-ordered partition queries, declared secondary indexes, lazy TTL. | A cross-partition scan, an eventually-consistent-by-default table, or a query-language document store is not this Form. |

The family deliberately holds ONE Form. The key schema, secondary indexes,
and TTL attribute are desired state of the `Table` because each declares
which attributes address data — the identity's own addressing declaration —
not an independently meaningful attachment or operating policy. A scan
surface, a change feed, or a backup policy would be separate proposals when
that work starts, per
[spec/form-families.md](../../spec/form-families.md).
