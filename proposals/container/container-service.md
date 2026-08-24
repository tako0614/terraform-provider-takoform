# ContainerService — `takoform_serverless_container_service`

The Provider 3 resource type is `takoform_serverless_container_service`.
The withdrawn v1alpha2 lane used `takoform_container_service`; that name is
never reoccupied by this different lifecycle contract (decision 0030).

## Workload and consumer

An application team runs an HTTP service as containers on a proven
serverless container platform: hand the platform an immutable image and let
it start instances when requests arrive and stop them when they cease.
Consumers address the service by its logical identity: traffic, endpoints,
and domains all point at this resource, never at a specific revision.

## Role

`identity`. The Form carries no desired fields: everything that can change —
image, environment, resources, scaling bounds, traffic — lives on revision
and deployment resources around it.

## Observable semantics

The identity fixes the serverless container contract, and says exactly what
that contract is: the exact Interface contract `container.runtime@1.0.0`,
authored the way `worker.runtime@1.1.0` fixes the ModuleWorker ABI
([decision 0019](../../spec/decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).
It fixes:

- An instance is one Linux process group started from the revision's
  digest-pinned OCI image, with the image's entrypoint or the revision's
  `command`/`args`.
- The environment the process receives is `PORT` plus the revision's vars,
  sensitive slots, and standard-service projections, and nothing else
  portable. The process must accept HTTP/1.1 connections on `0.0.0.0:$PORT`;
  an instance is ready when it accepts them.
- Requests are delivered only to ready instances, at most the revision's
  concurrency target each. Instances are started on demand and may be
  stopped whenever the revision's minimum allows; with a minimum of zero the
  service scales to zero and the next request bears the start latency.
- Stopping is SIGTERM, then at least ten seconds, then SIGKILL — the floor
  an application may rely on for draining.
- The filesystem is instance-local and ephemeral; durable state lives behind
  the revision's external services. `stdout` and `stderr` are captured as
  host diagnostics.
- Instances have no identity and no address; how requests reach instances is
  host placement, and nothing is promised about caller proximity.

A host that cannot run the image's declared platform refuses with
`unsupported_capability` rather than starting something else. There is no
compatibility date and no compatibility flag; a runtime revision is a new
exact Interface version — and, if it changes what a Form desires, a new Form
version.

## Why this is one Form

Every revision of the family runs under exactly this contract. Splitting the
identity from the contract — the withdrawn epoch's open tokens — would let
two hosts run the same revisions with observably different lifecycles.

## What would require a separate Form

A run-to-completion job or batch container preserves a different lifecycle
and is a different Form. Addressable per-instance state is the Edge family's
actor direction. A function invoked per event is the function family. A
non-HTTP protocol server — raw TCP, gRPC-only — changes the readiness and
delivery contract and is separate work.

## Provided Interfaces

`container.runtime@1.0.0` — the contract a conforming host provides to this
service's instances. It is held by the identity because the identity is what
a host implements; a Container Revision is the image that fills it.

## Accepted Bindings

None. Capability use belongs to revision resources (decision 0010), and
[ContainerRevision](container-revision.md) states why the MVP revision
accepts none either.

## Lifecycle risks

Deleting the identity while revisions, traffic, or attachments reference it
must fail with `dependency_in_use`. Delete/recreate mints a new UID
(decision 0011); stale references never rebind silently.

## Prior art

The request-driven serverless container model a proven serverless container
platform popularized and every major cloud now offers. The withdrawn
v1alpha2 `ContainerService` candidate is prior art that this family
supersedes for new design work; its open value tokens are the defect this
identity replaces by fixing one exact contract.
