# EdgeKVNamespace — `takoform_edge_kv_namespace`

## Workload and consumer

A worker keeps small, read-heavy state — sessions, feature data, rendered
fragments — in a key/value namespace replicated close to every point of
presence. Workers consume it exclusively through `module-worker.edge-kv`
bindings.

## Role

`identity`. The namespace has no desired fields: its semantics are entirely
fixed by the `edge.kv` Interface.

## Observable semantics

Exactly the `edge.kv@1.0.0` contract: get/getWithMetadata/put/delete/list,
eventual consistency (a read after a write may return the previous value
until replication converges), last-writer-wins per key, cursor pagination in
lexicographic key order, closed errors, and portable minimum limits.

## Why this is one Form

Eventual consistency is the Form's semantics, not an option. Every consumer
program is written against the convergence model; a consistency selector
would hollow the contract out (decision 0008).

## What would require a separate Form

A per-key linearizable store, a cache with eviction, or a TTL-defaulted
store each change the model a consumer can rely on and are different Forms.

## Provided Interfaces

`edge.kv@1.0.0`.

## Accepted Bindings

None; it is a binding target (`module-worker.edge-kv`).

## Lifecycle risks

Deleting a namespace bound by any Worker Version must fail with
`dependency_in_use` (refuse_while_bound). Delete destroys all keys; there is
no portable restore.

## Prior art

The eventually consistent edge KV store of a proven edge platform. The
retained v1alpha2 `KeyValueStore` candidate, whose open `consistency` enum
this Form replaces by fixing one shape.
