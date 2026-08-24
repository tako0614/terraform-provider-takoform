# FunctionVersion — `takoform_function_version`

## Workload and consumer

A team ships one executable snapshot of a function: which artifact runs,
which export handles events, its non-secret vars, its sealed slots, and the
resource bounds it runs under. Deployments select among versions; nothing
ever edits a version in place.

## Role

`revision`. Every field is immutable; a change is a new Function Version.

## Observable semantics

`function` is the immutable reference pinning this version to its owning
[Function](function.md) identity, resolved and uid-pinned like every relation
and carrying `x-takoform-target-formrefs` — exactly the Edge family's
`WorkerVersion.worker` shape, and for the same reason: without the parent
reference a host cannot say which function's aggregate a version belongs to.

The runtime a version runs on is not a field of this Form. It is the exact
`function.runtime@1.0.0` contract the Function identity
provides, so there is no `compatibilityDate` and no `compatibilityFlags`
([decision 0019](../../spec/decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).

`artifact` is one committed manifest digest under the content-addressed
artifact transport
([decision 0012](../../spec/decisions/0012-artifacts-use-content-addressed-upload.md),
[spec/artifact-transport](../../spec/artifact-transport/README.md)). The
family adds a `FunctionBundle` manifest kind: a main module plus
digest-pinned modules, whose closed media-type policy is what
`function.runtime` loads plus the source maps a bundle carries without
importing. Before any mutation a host resolves the manifest and holds it to
the artifact contract: an uncommitted digest is `artifact_missing`, and a
manifest of another kind or shape is `artifact_invalid`. There is no
separate bundle Form: the manifest digest is already the build's whole
content identity and the transport deduplicates shared bytes, so a
standalone bundle resource would add a name without adding identity.

`handler` names the export of the main module the host invokes; the runtime
contract requires that the declared export exist and be callable. `vars` is
a bounded data-only JSON map projected as environment members.
`requiredSensitiveVars` declares only the names of sealed values the host
must supply, under the same permitted vocabulary the Edge family records
(the Form Package data-only policy forbids the token `secret` in field
names).

`externalServices` is carried from birth
([decision 0045](../../spec/decisions/0045-external-standard-services-are-sealed-slots.md),
[spec/standard-services](../../spec/standard-services/README.md)): each
entry is exactly
`{"name": "DB", "service": {"apiVersion": "standards.takoform.com/v1alpha1", "protocol": "postgresql"}}`,
with the closed protocol vocabulary `s3-compatible`, `postgresql`, `redis`,
`smtp`. The host satisfies a slot with connection material; endpoints and
credentials never enter portable state. The version projects vars, sensitive
names, and slot projections into ONE process-environment namespace, and
because slot projections are derived member names (`DB_URL`,
`MEDIA_ENDPOINT`, …), uniqueness is enforced over the PROJECTED names, not
only the declared union: a var literally named `DB_URL` collides with a
`postgresql` slot named `DB`, and a host refuses the collision before
mutation.

`memoryMiB` is the bound of usable memory: exceeding it fails the invocation
observably. `timeoutSeconds` is the wall-clock budget per invocation: on
expiry the invocation is terminated and reported failed. `maxConcurrency`
bounds simultaneous invocations of this version: excess invocations are
refused observably as throttling, never queued indefinitely. These bounds
change what an application and its callers observe and are therefore desired
state; the admitted ranges are Host Support Profile facts.

## Why this is one Form

Code, handler, environment, sealed slots, and resource bounds must travel as
one immutable unit, or rollback cannot be exact: re-activating an old
version must restore exactly the snapshot it was verified with.

## What would require a separate Form

Mutable in-place configuration or a per-environment overlay model breaks the
immutable-snapshot shape. A version whose artifact is an OCI image is the
container family's `ContainerRevision`, not a variant of this Form.

## Provided Interfaces

None. The runtime contract belongs to the [Function](function.md) identity.

## Accepted Bindings

None in the MVP, and the absence is honest rather than an oversight. Current
Edge Interfaces such as `edge.kv`, `edge.sql`, and `edge.queue` state the
JavaScript surface their `module-worker.*` bindings project into
`worker.runtime`'s environment. The retained v1beta1 `edge.objects` Interface
is historical and is not a current Function capability. Projecting any typed
binding into this family's runtime requires either a stated projection into
`function.runtime` or a wire-level realization of the Interface, and the
binding contracts define neither today. The MVP reaches external state
through the standard-service slots above; typed cross-family bindings
arrive when that realization is specified as its own reviewed contract.
This proposal does not invent it.

## Lifecycle risks

Creating a version whose manifest is uncommitted or invalid must fail
(`artifact_missing` / `artifact_invalid`) before mutation, and a referenced
manifest and its blobs stay readable for as long as the version exists.
Deleting a version still weighted by a deployment must fail with
`dependency_in_use`. A REQUIRED external-service slot a host cannot satisfy
is refused at plan time with `unsupported_capability`, and an unsatisfied
slot makes the resource not Ready. Import must reproduce the exact snapshot,
digest and handler included.

## Prior art

The published function version of a proven cloud function service —
artifact, handler string, environment, memory and timeout — with its sealed
slots and external services made explicit contract. The withdrawn v1alpha2
`EdgeWorker` candidate is prior art in part.
