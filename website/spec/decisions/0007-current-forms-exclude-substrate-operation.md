# 0007 — Current Forms exclude substrate operation

- Status: accepted
- Date: 2026-08-05
- Owners: Takoform maintainers

## Context

The first v1alpha2 candidate generator copied each retained v1alpha1 Definition
and fixture tree, then changed its API version, Form version, status, and
description. This mechanically carried fields such as concurrency, replicas,
CPU, memory, size class, high availability, routing, health checks, storage
class, consumer retry policy, and provider compatibility into the reset line.

Several current concepts resemble Cloudflare products because Takosumi Cloud
is the first real workload and host. That resemblance is not itself a defect:
edge execution, key/value storage, queues, schedules, and addressable stateful
entities are useful general abstractions. The defect is allowing one
substrate's operating model to determine the portable desired schema.

## Decision

The normative ownership boundary is
[`../portability-boundary.md`](../portability-boundary.md).

Current Forms own independently implementable workload semantics only. Host
support profiles own capability availability. Adapters own compatibility with
external protocols or ecosystems. Operators own placement, routing, scaling,
health, capacity, and retry policy. Service Offerings own quota, availability,
price, and support.

Cloudflare-like general resource concepts are allowed. Cloudflare-specific
product identity, configuration, compatibility surfaces, account topology,
limits, and commercial behavior are not portable fields.

The current catalog is independently authored. Candidate generation may reuse
the shared schema type system, but may not read or copy a Legacy Definition or
fixture tree. Exact current field sets and forbidden substrate/operation fields
are executable tests.

## Consequences

- `EdgeWorker` retains artifact, entrypoint, runtime capability,
  configuration, connections, and an HTTP Interface; assets policy, request
  timeout, and concurrency move out of the Form.
- `ContainerService` retains an immutable image, configuration, connections,
  and an HTTP Interface; ports, public routing, health probes, CPU, memory, and
  replicas move out of the Form.
- `RelationalDatabase` retains engine/schema semantics; storage size, size
  class, and high availability move out of the Form.
- `ObjectBucket` retains object lifecycle semantics; storage class and access
  protocol compatibility move to host/adapter layers.
- `Queue` retains delivery order and message-retention semantics; consumer
  batching, retry, visibility, delay, and capacity limits move out of the Form.
- `StatefulEntity` uses a generic artifact entrypoint and persistence
  capability; provider-shaped class and migration controls do not enter the
  Form.
- Takosumi Cloud can continue to implement these Forms with Cloudflare or
  another substrate without making that substrate part of Takoform.

## Rejected alternatives

- **Rename every Cloudflare-like concept.** Rejected because neutral semantics,
  not unfamiliar names, provide portability.
- **Keep all fields as open capability tokens.** Rejected because an open token
  can still smuggle placement, pricing, or one provider's control plane into
  the portable contract.
- **Standardize compatibility APIs in each Form.** Rejected because protocol
  compatibility is adapter/Interface material and varies independently from
  Resource lifecycle.
- **Filter Legacy fields only at documentation time.** Rejected because the
  provider schema, fixtures, digests, and host contract would remain coupled to
  the old substrate-shaped source.
