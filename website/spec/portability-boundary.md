# Portable Form boundary

This document defines what may enter a current Takoform Form. The governing
rule, decided in
[decision 0008](decisions/0008-forms-preserve-service-shape.md):

> A Form is the smallest **semantically complete** desired-state contract that
> independent hosts can implement with the same application-visible behavior.
> A field or semantic rule must not be removed merely because one host can
> choose it privately. If two conforming hosts could make observably different
> choices after a field or rule is omitted, the Form is incomplete.

Takoform does not define generic resources reduced to the least common
denominator of several clouds. A Form fixes the application-visible semantics
of one proven service primitive — its execution ABI, client API, data model,
consistency, delivery guarantees, failure and retry behavior, update, delete,
and migration units, and runtime interface — completely. Only the provider- or
host-specific facts are outside the contract: account, region, SKU, price,
credentials, native IDs, and internal implementation.

Implementations with different semantics are separate Forms sharing a family,
never one Form with a selector token. A host supports a Form by implementing
that shape with the same meaning; no host must implement every Form.

## Ownership layers

| Layer | Owns | Examples |
| --- | --- | --- |
| Form | complete application-visible semantics of one service shape | module-worker ABI, KV consistency, queue delivery guarantee, SQLite semantics |
| Interface contract | exact operation surface a resource exposes | operations, input/output types, errors, pagination, consistency |
| Binding contract | typed capability projected into a consumer | worker KV binding: runtime API + permission, no credentials |
| Host Support Profile | whether and within what limits a host implements an exact ref | supported compatibility dates, flags, metrics, dimension ceilings |
| Adapter profile | translation to one concrete backend or ecosystem | Cloudflare API mapping, S3 compatibility, workerd hosting |
| Operator policy | realized placement and operating decisions | region, replicas, capacity assignment |
| Service Offering | commercial availability | quota, size classes, SLA, price, billing |

## Where a difference lives

| The difference changes… | Placement |
| --- | --- |
| execution ABI, client API, consistency, persistence, delivery guarantee | a separate Form |
| the unit of update, delete, migration, or replacement | a separate Form or a separate Resource |
| how one resource connects to another | a typed Binding contract |
| orthogonal inward activation (route, trigger, consumer, domain) | an attachment Resource |
| an immutable implementation snapshot (code, assets, migrations) | a revision Resource |
| independently updated operating rules (CORS, retention, lock, lifecycle) | a policy Resource |
| what a host can accept (versions, features, ceilings) | a Host Support Profile |
| minimum execution requirements (CPU, memory) | a Form or revision resource requirement |
| SKU, price, actual allocation, placement, vendor IDs | Service Offering / host policy |
| the concrete translation to one vendor API | an adapter profile |

Resources share one Form because the same program can use them under the same
assumptions — not because they belong to the same category. A JavaScript
module worker, a WASI function, and an OCI container are three Forms. An
eventually consistent global KV, a per-key linearizable KV, and a
Redis-compatible cache are three Forms. SQLite, PostgreSQL, and MySQL
databases are three Forms.

## Free tokens do not carry semantics

Open value tokens such as `runtime`, `engine`, `persistence`, `consistency`,
`ordering`, or `projection` are prohibited as semantic fields in current
Forms. A token a host may interpret freely turns one desired document into
different systems on different hosts.

- A token that changes API or ABI → a separate Form.
- A token that changes consistency or persistence → a separate Form.
- A token that changes a binding API → a typed Binding contract.
- An internal algorithm choice with identical API, storage model, and
  lifecycle (for example a vector distance metric) → a closed enum.
- Future extension values → explicit namespaced extensions, marked
  non-portable and excluded from the portable core.

## Field admission test

A field belongs in a Form only when all of the following are true:

1. a consumer needs the value to distinguish workload meaning, not merely a
   preferred implementation;
2. two independent hosts must give the value the same observable semantics;
3. drift, update, replacement, import, and delete behavior can be tested from
   the portable lifecycle;
4. the field does not select a vendor product, account, target, credential,
   price, placement, or operator-private policy;
5. absence of the field cannot be silently interpreted as one particular
   provider's default;
6. removing the field would let two conforming hosts choose observably
   different behavior.

Interfaces are exact contracts, not open descriptors: an Interface Definition
fixes operations, types, errors, consistency, and pagination, and is bound by
digest ([decision 0010](decisions/0010-exact-interface-and-binding-contracts.md)).
Endpoints, credentials, authorization, and protocol compatibility remain
host-owned; S3-compatible access to an `ObjectBucket` is adapter material,
not a desired field.

## Current-candidate authoring rule

The v1alpha1 Legacy catalog and the v1alpha2 provider-v2 candidate line are
prior art only. A current candidate MUST be authored from its Proposal and
this boundary. A generator MUST NOT copy a Legacy or retained Definition or
fixture tree and change only API version, status, or SemVer. Reusing the
shared schema vocabulary and retaining a semantic field after explicit review
are allowed; inheriting an old field set is not.

The retained v1alpha2 candidates under
[`forms/candidates/v1alpha2`](/forms/candidates/v1alpha2/candidate-set.json) remain frozen
provider-v2 preview source generated from
`internal/currentformcatalog`. New
family-based candidates are generated from their family catalog sources into
family-scoped candidate trees; the catalog tests pin every reviewed field set,
reject known substrate and operator fields, and reject open semantic tokens.
Reproducibility checks prove the tracked package bytes still come from that
source.
