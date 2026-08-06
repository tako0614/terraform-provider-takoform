# Portable Form boundary

This document defines what may enter a current Takoform Form. A Form owns the
smallest desired-state contract and observable outcomes that independent hosts
can implement with the same meaning. It does not model a cloud product,
deployment platform, operator policy, or commercial service catalogue.

Cloudflare-like concepts such as an edge application, key/value namespace,
queue, scheduled invocation, or namespace of stateful entities are valid when
their semantics stand on their own. Cloudflare product names, compatibility
APIs, account topology, limits, placement controls, billing tiers, and runtime
configuration are not portable Form state.

## Ownership layers

| Layer | Owns | Examples |
| --- | --- | --- |
| Form | workload semantics and exact portable lifecycle | immutable artifact or image, entrypoint, data semantics, declared Interface |
| Host support profile | whether and how a host supports an exact FormRef | supported runtime tokens, engine versions, metrics, timezone behavior |
| Adapter or compatibility profile | translation to an external ecosystem | S3-compatible access, Workers compatibility, Kubernetes projection |
| Operator policy | realized placement and operating policy | region, route, replicas, CPU, memory, health checks, retry and scaling policy |
| Service Offering | capacity, availability, price, and support | quotas, size classes, HA tiers, SLA, billing |

A field belongs in a Form only when all of the following are true:

1. a consumer needs the value to distinguish workload meaning, not merely a
   preferred implementation;
2. two independent hosts can give the value the same observable semantics;
3. drift, update, replacement, import, and delete behavior can be tested from
   the portable lifecycle;
4. the field does not select a vendor product, account, target, credential,
   price, placement, or operator-private policy;
5. absence of the field cannot be silently interpreted as one particular
   provider's default.

Open tokens such as `runtime`, `engine`, `metric`, or `persistence` are
capability requirements, not a central allowlist. A host must advertise
support for the exact token or fail closed. A token does not authorize a host
to expose backend configuration through the Form.

Interfaces describe a portable operation surface. Endpoints, credentials,
authorization, provider bindings, and protocol compatibility remain
host-owned. For example, `object.storage@1` can be implemented through S3, R2,
GCS, a filesystem, or another backend; requesting an S3-compatible endpoint is
an adapter/profile concern rather than an `ObjectBucket` desired field.

## Current-candidate authoring rule

The v1alpha1 Legacy catalog is prior art only. A current candidate MUST be
authored from its Proposal and this boundary. A generator MUST NOT copy a
Legacy Definition or fixture tree and change only API version, status, or
SemVer. Reusing the shared schema vocabulary and retaining a semantic field
after explicit review are allowed; inheriting the old field set is not.

Current candidate generation starts from
[`internal/currentformcatalog`] and produces
the exact packages under [`forms/candidates/v1alpha2`](/forms/candidates/v1alpha2/candidate-set.json).
The catalog test pins every reviewed field set and rejects known substrate and
operator fields. Reproducibility checks prove the tracked package bytes still
come from that source.
