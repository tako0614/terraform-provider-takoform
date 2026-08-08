# 0010 — Exact Interface contracts and typed Bindings

- Status: accepted; its Interface Package envelope and the word "published" in
  its description of an Interface are withdrawn by
  [0021](0021-third-party-forms-and-contract-distribution.md), which distributes
  Interface and Binding Definitions with this repository instead
- Date: 2026-08-06
- Owners: Takoform maintainers

## Context

Current Interface declarations are open `(name, version, operations)`
descriptors. They do not fix request/response types, errors, consistency,
pagination, or retry, so two hosts can expose the "same" interface with
incompatible behavior.

Resource-to-resource connections are generic:

```json
{
  "resource": "KeyValueStore/cache",
  "permissions": ["read", "write"],
  "projection": "keyvalue.binding.v1"
}
```

Free `permissions` and `projection` tokens delegate the meaning of the
connection to each host. The proven model — a binding that grants a
capability and a concrete runtime API together, without exposing credentials
— cannot be expressed, verified, or held stable this way.

## Decision

### Exact Interface contracts

An Interface becomes an independently published, digest-bound contract:

```json
{
  "apiVersion": "interfaces.takoform.com/v1alpha1",
  "name": "edge.kv",
  "version": "1.0.0",
  "schemaDigest": "sha256:..."
}
```

An Interface Definition fixes operations with input/output schemas, a closed
error vocabulary, consistency and pagination semantics, portable minimum
limits, and data-only behavior fixtures (conformance traces such as
put-then-get), not just a name and an operation list. Interface Packages
follow the Form Package rules: one package, one definition, exact digest,
data-only payloads, positive and negative fixtures.

### Typed Bindings

Generic connections, `permissions`, and `projection` are removed from the new
line. A Binding is its own digest-bound contract:

```json
{
  "apiVersion": "bindings.takoform.com/v1alpha1",
  "name": "module-worker.edge-kv",
  "version": "1.0.0",
  "schemaDigest": "sha256:..."
}
```

A Binding Definition fixes the source Form role, the target Interface, the
runtime API projected into the consumer, the allowed target Forms, optional
access modes, lifecycle, and the binding-name grammar. A binding grants
capability and API together and never exposes credentials or secret values to
the consumer. Worker-family binding names use the JavaScript identifier
grammar `^[A-Za-z_$][A-Za-z0-9_$]*$`.

Outward capability use (KV, buckets, databases, queues, services) is a
Binding held by a revision resource. Inward invocation (routes, custom
domains, cron, queue consumption) is an attachment resource, never a binding.

## Consequences

- Form Definitions reference Interfaces and Bindings by exact ref; the old
  `(name, version, operations)` projection stays only in the retained
  v1alpha2 lane docs.
- The provider exposes typed binding blocks (`kv_binding`, `bucket_binding`,
  `sqlite_binding`, `queue_producer_binding`, `service_binding`) instead of a
  generic `connections` map.
- New public schema identities are minted for interface-ref,
  interface-definition, binding-ref, and binding-definition documents.
- Behavior fixtures make interface conformance testable beyond shape
  validation; location-dependent properties (for example eventual-consistency
  convergence) are explicitly out of the deterministic fixture scope.

## Rejected alternatives

- **Harden the generic connection with more token vocabularies.** Rejected
  because closed token lists on a generic surface still cannot bind an actual
  runtime API or error contract; every new pair of resources would grow the
  central vocabulary.
- **Define bindings inside each Form Definition only.** Rejected because the
  same binding shape (worker → KV) must stay identical across families and
  hosts; an independent digest-bound contract keeps it single-sourced.
- **Executable conformance suites inside packages.** Rejected because
  packages are data-only by trust policy; behavior fixtures stay declarative
  traces executed by the verifier.
