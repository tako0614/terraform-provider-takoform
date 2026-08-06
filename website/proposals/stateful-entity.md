# Form Proposal: StatefulEntity

Status: active Proposal; intended first v1alpha2 release `0.1.0`. A local
candidate package exists under `forms/candidates/v1alpha2/`, but no released
FormRef or public release identity exists yet.

## Need and boundary

Takosumi Cloud runs a namespace of addressable durable entities from immutable
application code. The candidate Form describes namespace identity,
digest-bound code, a generic entrypoint, runtime and persistence capability
requirements, non-secret configuration, connections, and an entity invocation
Interface. Runtime implementation, storage engine, migrations, placement,
replication, credentials, routing, and price remain host-owned.

## Substrate-neutrality review

Addressable entity namespaces can be implemented by Durable Objects-like
isolates, an actor runtime, or a transactional database-backed host. The Form
uses a generic entrypoint and open persistence requirement; class deployment,
migration tags, shard placement, replication, routing, alarms, and account
bindings are excluded as substrate operation.

## Lifecycle and security risks

Namespace or storage-model changes need explicit migration and may not permit
replacement. Delete can destroy durable entity state and requires evacuation
or retention policy. Entity data, storage credentials, and invocation secrets
never enter portable state.

## Prior art and gap

OCCI Resource/Link/Action, TOSCA stateful components, Kubernetes StatefulSet
and operator patterns, and Terraform durable-object/actor resources are
applicable; CIMI does not model addressable entity namespaces directly. The
gap is a namespace-level contract without standardizing the host actor runtime
or storage engine.
