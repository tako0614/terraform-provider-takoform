# 0045 — An external standard service is a sealed slot with a protocol tag

- Status: Accepted
- Date: 2026-08-23

## Context

[Decision 0043](0043-forms-target-popular-vendor-locked-primitives.md) split
the world in two: vendor-locked categories become Form Families, and
categories with a de-facto standard API are integrated. It promised the
integration contract as its own change without fixing its shape.

Three shapes were available. A **resource shape** — an `ExternalService`
Resource holding the endpoint — puts a URL in portable desired state and
walks straight into the authority boundary: endpoints imply accounts,
accounts imply credentials, and the repository has spent its whole life
keeping both out of portable state. A **binding shape** — reusing the typed
Binding contract — misstates the relationship: a Binding targets an in-space
Resource providing a Takoform Interface, pins its uid and exact FormRef, and
participates in relation lifecycle; an external Postgres has none of those
and pretending otherwise would hollow out what a Binding's pin means. The
**sealed-slot shape** already exists: `requiredSensitiveVars` declares, in
portable state, only the NAME of a value the host supplies at runtime.

An external standard service is exactly that, plus one fact the name alone
cannot carry: which protocol the supplied material must speak.

## Decision

**An external standard service is declared as a sealed slot with a protocol
tag**: portable state carries `{name, service: {apiVersion, protocol}}` and
nothing else; the host or operator satisfies the slot with connection
material for a service speaking that protocol; the projection into the
consumer's runtime namespace is fixed per protocol so consumer code stays
portable. The normative contract is
[`../standard-services/README.md`](../standard-services/README.md) with the
closed vocabulary in `standard-service-ref-v1alpha1.schema.json`
(`s3-compatible`, `postgresql`, `redis`, `smtp`).

The contract deliberately grants no lifecycle authority over the external
service and never verifies the wired service's protocol conformance — the
host vouches for what it wires. Widening the protocol vocabulary is a
reviewed spec change held to decision 0043's test.

The published v1beta1 schemas are untouched: they are frozen identities, so
families minted from now on carry the declaration from birth, and the Edge
family adopts it when a graduation mints its next identities
([decision 0037](0037-immutability-begins-at-stable.md)).

## Consequences

A container revision can reach the team's Postgres and an S3-compatible
bucket without either appearing in state, without Takoform growing a database
Form it swore off, and without a Binding pretending an external service has a
uid. The first family that embeds the declaration brings the executable
conformance for it; until then the contract's own document says no host can
claim measured support.
