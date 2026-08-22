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
wire protocol, caches have the Redis protocol. There a portability layer
already exists, an ecosystem of clients speaks it, and a Takoform
respecification would compete with a standard instead of adding one. The
categories that actually lock applications in are the popular managed
services whose provisioning and service APIs are vendor-specific — offered by
every major cloud, portable to none of them.

## The landscape, partitioned

The rule is only as good as the survey behind it, so the survey is recorded.
Sweeping the popular managed-service categories of the major clouds and
splitting them by "does a de-facto standard API exist":

| Category | De-facto standard | Side |
| --- | --- | --- |
| Object storage | S3-compatible API | integrate |
| Relational database | PostgreSQL / MySQL wire | integrate |
| Cache | Redis protocol | integrate |
| Message broker | Kafka protocol, AMQP | integrate |
| Document store (Mongo-flavored) | MongoDB wire (Cosmos/DocumentDB emulate it) | integrate |
| Search | Elasticsearch API (OpenSearch emulates it) | integrate |
| Mail submission | SMTP | integrate |
| Container orchestration | Kubernetes API | integrate |
| Auth flow | OIDC / OAuth2 | integrate |
| Telemetry export | OTLP (OpenTelemetry) | integrate |
| LLM inference serving | OpenAI-compatible API (standardizing) | integrate |
| Browser push | RFC 8030 Web Push | integrate |
| TLS certificate issuance | ACME | integrate (a host duty behind domain attachments; no slot protocol needed) |
| Regional FaaS (Lambda, Cloud Functions) | none | **Form Family** |
| Serverless containers (Cloud Run, Fargate, Container Apps) | none | **Form Family** |
| Document/KV table (DynamoDB, Firestore, Cosmos native) | none | **Form Family** |
| Pull queue (SQS, Cloud Tasks) | none | **Form Family** |
| Pub/sub fanout (SNS, EventGrid) | none | **Form Family** |
| Scheduler (Cloud Scheduler, EventBridge Scheduler) | none | **Form Family** |
| Vector index (Pinecone, Vertex, serverless vector stores) | none | **Form Family** |
| Durable code-defined execution (Workflows, Temporal, Durable Functions, Inngest) | none (Temporal is open source, not a de-facto standard) | **Form** (edge family addition) |
| Declarative state machines (Step Functions ASL, Cloud Workflows) | none | family candidate |
| Addressable actors (Durable Objects) | none | **Form** (edge family addition) |
| Event bus/router (EventBridge) | none | family candidate |
| Realtime client push (managed WebSocket) | none | family candidate |
| Identity pool management (Cognito, Firebase Auth admin) | none | family candidate |
| Batch compute | none | family candidate |
| Data warehouse (BigQuery, Redshift, Snowflake) | none (SQL dialects diverge) | family candidate |
| Media transform/streaming (Images, Cloudinary, Mux) | none | family candidate |
| Notification delivery (SMS, mobile push) | none for SMS; FCM/APNs are their platforms' de-facto channels | family candidate |
| Secrets manager | none | skip — sealed slots are already this project's secret interface (decision 0045); a secret-container Form would duplicate the doctrine |
| DNS zone/record management | the data plane is DNS itself | skip — native provider tooling is ubiquitous and the app-side need is covered by domain-attachment Forms |
| CI/CD and build pipelines | none | skip — tooling, not runtime desired state |
| Inbound mail routing | none (webhook shapes diverge) | skip — niche |
| Custom metrics/analytics (Analytics Engine) | OTLP covers export; the query side is niche | skip |
| Trace/diagnostic taps (Tail Workers) | single-vendor diagnostics | skip |
| Managed browser rendering | single-vendor, niche | skip |
| Edge workers (Workers, Fastly Compute) | none | **Form Family** (edge, existing) |

The split is not "clouds have no originality" — it is an asymmetry: **data
planes have largely standardized while the serverless control planes have
not**, and the unstandardized half is exactly where a host-neutral contract
is the only portability there is. One platform sits almost entirely on the
locked side — the Workers platform speaks essentially no external standard
except R2's S3 compatibility — which is why the first family filled itself
with one platform's shapes and why the catalog must not stop there.

Virtual machines and other substrate stay out on separate grounds: decision
[0007](0007-current-forms-exclude-substrate-operation.md) excludes substrate
operation from portable desired state regardless of standards.

## Decision

**A Form Family is minted only for a service category that is (a) popularly
offered as a managed service across major providers and (b) has no de-facto
standard API — the category where a host-neutral contract is the only
portability there is.**

**A category with a de-facto standard API is never respecified as a Form.
Takoform integrates with it instead**: a Form's runtime reaches the standard
service as an external standard-service binding
([decision 0045](0045-external-standard-services-are-sealed-slots.md)) — the
standard protocol is the contract, and the host resolves the endpoint and
credential the way it already resolves sensitive slots, so neither ever
enters portable desired state. The protocol vocabulary starts at
`s3-compatible`, `postgresql`, `redis`, `smtp`; the survey above is its
growth roadmap (`kafka`, `amqp`, `mysql`, `mongodb`, `elasticsearch-compatible`,
`openai-compatible`, `otlp` are the recorded candidates), and every widening
is a reviewed change held to this record's test.

Under this rule the v1 lineup is **eight families**:

| Family group | Fixed shape | Replaces (withdrawn) |
| --- | --- | --- |
| `edge.forms.takoform.com` | the Workers platform model (existing, 15 Forms) | — |
| `function.forms.takoform.com` | regional FaaS: artifact + handler + event invocation, concurrency, timeout (the Lambda shape) | `EdgeWorker` in part |
| `container.forms.takoform.com` | OCI-image service with immutable revisions and traffic splitting (the Cloud Run/Knative shape) | `ContainerService` |
| `table.forms.takoform.com` | document/KV table with declared keys and secondary indexes (the DynamoDB shape) | `KeyValueStore`, `StatefulEntity` in part |
| `queue.forms.takoform.com` | pull-based at-least-once queue with visibility timeout and dead-lettering (the SQS shape) | `Queue` |
| `topic.forms.takoform.com` | fanout topic with push subscriptions and filter policies (the SNS shape) | — |
| `schedule.forms.takoform.com` | standalone cron invoking a declared target with a retry policy (the Cloud Scheduler shape) | `Schedule` |
| `vector.forms.takoform.com` | fixed-dimension vector index with declared metric and namespaces | `VectorIndex` |

The Edge family's `AtLeastOnceQueue` is not the same Form as `queue`'s: one is
push-delivery into a worker consumer, the other is pull with visibility
semantics — two proven shapes, two contracts, per decision 0008.

**The Edge family also closes its own gaps.** Its platform grew two core
primitives after the family's shapes were fixed, and both sit squarely on the
locked side of the survey, in the family's own model:

- `DurableWorkflow` — the code-defined durable-execution shape (a workflow is
  a class on a worker with `step.do`/`step.sleep`/`waitForEvent`, step results
  persisted, at-least-once execution with memoized replay); instances are
  runtime data reached through a `workflow` binding, not Resources.
- `ActorNamespace` — the addressable-actor shape (a class on a worker,
  single-threaded per object, addressed by name/id, per-object storage and
  alarms), reached through an `actor` binding.

Both enter as new `0.1.0` Experimental identities with their own ABI and
binding contracts; the published v1beta1 definitions are frozen and unchanged
(decision 0037), and the existing Edge Forms adopt the decision-0045
external-service declaration only when a graduation mints their next
identities. The standalone `actor` family candidate is retired into this
addition — it returns only if a second platform's actor shape warrants a
family of its own. What the platform has that stays out is recorded in the
survey: analytics, tail taps, and browser rendering, each with its reason.

`workflow` (the declarative state-machine shape — the durable code-defined
shape lands in the Edge family instead), `eventbus`, `realtime`, `identity`,
`batch`, `warehouse`, `media`, and `notify` are recorded as family
candidates: each satisfies the rule, and each is deferred for its own reason
(state-language size, coupling to `workflow`, protocol churn, security
surface, narrower demand, analytics surface area, breadth of the media
category, delivery-network dependence) rather than by the rule. Every
withdrawn v1alpha2 kind has a recorded successor or integration: the lineup
table's last column accounts for seven, `StatefulEntity` splits between the
Edge `ActorNamespace` and `table`, and `RelationalDatabase` became the
`postgresql` integration.

What is deliberately **excluded**: every integrate-side row above. The
earlier `postgres`-family idea dies here — PostgreSQL is a standard, so it is
an integration target.

This record binds only Takoform: it decides which categories this project
specifies and which it reaches through decision-0045 sealed slots. The
maintainer has directed the complementary half for takoserver — serving the
de-facto standard APIs themselves (an S3-compatible endpoint, an
OpenAI-compatible Responses API endpoint) while implementing the Takoform
contracts as a host for the vendor-locked remainder — and that direction is
recorded and coordinated where cross-repository scope lives, the takos-control
task ledger (TASK-0032); takoserver's own decision record adopts it there.
What this ADR contributes to that picture is the boundary itself: Takoform
exists to specify exactly the non-standard remainder.

## Enforcement

Minting a family is a reviewed spec change, and this record is what the
review holds it against: a proposed family must name the popular category,
show the absence of a de-facto standard API, and name the one proven shape it
fixes (decision 0008). Widening the standard-service protocol vocabulary is
held to the same table. The external standard-service binding contract is
[decision 0045](0045-external-standard-services-are-sealed-slots.md) with its
own conformance obligations.

## Consequences

The lineup grows to eight families plus two Edge-family additions, with eight
recorded candidates, every one justified by the same sentence, and the
catalog stops being one platform's silhouette. Where the industry already solved portability, Takoform rides the
standard instead of fighting it — which is also what keeps the spec small
enough to graduate.
