# 0043 — A Form Family targets a popular primitive whose APIs are vendor-locked; de-facto standards are integrated, not respecified

- Status: Accepted
- Date: 2026-08-23

## Context

One family exists, and its shapes all come from one platform: the Edge
Platform Family fixes the Workers model. The withdrawn v1alpha2 epoch had
covered more ground — relational databases, container services, key/value
stores, vector indexes — but as least-common-denominator contracts, which
[decision 0008](0008-forms-preserve-service-shape.md) rejected: a Form fixes
the complete shape of one proven primitive, and what is exchangeable is the
host, never the meaning.

Widening the lineup again therefore needs a selection rule, or the same drift
recurs: either every cloud product grows a Form (unbounded, and most would be
LCD contracts again), or Forms accrete wherever the maintainer happens to
work (which is how one platform's shapes became the whole catalog).

The rule falls out of asking where a host-neutral desired-state contract adds
value at all. A service category can be popular and portable already — object
storage has the S3-compatible API, relational databases have the PostgreSQL
wire protocol, caches have the Redis protocol, mail submission has SMTP. There
a portability layer already exists, an ecosystem of clients speaks it, and a
Takoform respecification would compete with a standard instead of adding one.
The categories that actually lock applications in are the popular managed
services whose provisioning and service APIs are vendor-specific: serverless
containers, document/KV tables, schedulers, vector indexes, workflow engines —
offered by every major cloud, portable to none of them.

## Decision

**A Form Family is minted only for a service category that is (a) popularly
offered as a managed service across major providers and (b) has no de-facto
standard API — the category where a host-neutral contract is the only
portability there is.**

**A category with a de-facto standard API is never respecified as a Form.
Takoform integrates with it instead**: a Form's runtime reaches an
S3-compatible bucket, a PostgreSQL database, a Redis cache, or an SMTP
submission endpoint as an **external standard-service binding** — the standard
protocol is the contract, and the host resolves the endpoint and credential
the way it already resolves sensitive slots, so neither ever enters portable
desired state. That binding contract is its own reviewed spec change; this
record fixes the direction.

Under this rule the v1 lineup is:

| Family group | Fixed shape | Replaces (withdrawn) |
| --- | --- | --- |
| `edge.forms.takoform.com` | the Workers platform model (existing, 15 Forms) | — |
| `container.forms.takoform.com` | OCI-image service with immutable revisions and traffic splitting (the Cloud Run/Knative shape) | `ContainerService` |
| `table.forms.takoform.com` | document/KV table with declared keys and secondary indexes (the DynamoDB shape) | `KeyValueStore`, `StatefulEntity` in part |
| `schedule.forms.takoform.com` | standalone cron invoking a declared target with a retry policy (the Cloud Scheduler shape) | `Schedule` |
| `vector.forms.takoform.com` | fixed-dimension vector index with declared metric and namespaces | `VectorIndex` |

`workflow` (the Step Functions category) satisfies the rule and is recorded as
a candidate; its state-language surface is large enough to be its own design
effort. An `actor` family is deliberately not minted now: its proven shape is
one vendor's, and the Edge family's SQLite/queue primitives cover most of its
current use.

What is deliberately **excluded** by the rule: object storage beyond the Edge
family's own bucket (S3-compatible is the standard), relational databases
(PostgreSQL is the standard — the earlier `postgres` family idea dies here),
caches (Redis), mail (SMTP). Each is an integration target, not a Form.

The same principle is adopted ecosystem-wide: takoserver builds on de-facto
standards where they exist and defines contracts only where the category is
vendor-locked. That obligation lives with takoserver; this record states the
shared principle.

## Enforcement

Minting a family is a reviewed spec change, and this record is what the review
holds it against: a proposed family must name the popular category, show the
absence of a de-facto standard API, and name the one proven shape it fixes
(decision 0008). The external standard-service binding contract arrives as its
own spec change with its own conformance surface; until it lands, nothing
claims it exists.

## Consequences

The lineup grows by four families and a recorded candidate, every one of them
justified by the same sentence, and the catalog stops being one platform's
silhouette. Where the industry already solved portability, Takoform rides the
standard instead of fighting it — which is also what keeps the spec small
enough to graduate.
