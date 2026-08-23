# ContainerRevision — `takoform_container_revision`

## Workload and consumer

A team ships one immutable revision of a container service: which image
runs, how its process starts, its environment, its sealed slots, and the
resource and scaling bounds it serves under. Traffic selects among
revisions; nothing ever edits a revision in place.

## Role

`revision`. Every field is immutable; a change is a new Container Revision.

## Observable semantics

`image` is one OCI image reference and MUST be digest-pinned
(`name@sha256:…`). A mutable tag is refused with `invalid_argument` before
any mutation: the digest is the revision's content identity, in the spirit
of decision 0005's content-addressed locators, and a tag would let the same
desired document mean different bytes on different days. The repository part
of the reference says where the bytes resolve; registry choice, mirroring,
and pull authorization are host and operator obligations that never enter
portable state. A multi-platform index digest resolves, on each host, to a
platform that host can run.

`command` and `args` override the image's entrypoint and default arguments;
absent, the image's own configuration applies, and absence is semantic
rather than a default the host fills in. `vars` is a bounded data-only JSON
map projected as environment members. `requiredSensitiveVars` declares only
the names of sealed values the host must supply, under the same permitted
vocabulary the Edge family records (the Form Package data-only policy
forbids the token `secret` in field names).

`externalServices` is carried from birth
([decision 0045](../../spec/decisions/0045-external-standard-services-are-sealed-slots.md),
[spec/standard-services](../../spec/standard-services/index.md)): each
entry is exactly
`{"name": "DB", "service": {"apiVersion": "standards.takoform.com/v1alpha1", "protocol": "postgresql"}}`,
with the closed protocol vocabulary `s3-compatible`, `postgresql`, `redis`,
`smtp`. The host satisfies a slot with connection material; endpoints and
credentials never enter portable state. The revision projects vars,
sensitive names, and slot projections into ONE process environment, and
because slot projections are derived member names (`DB_URL`,
`MEDIA_ENDPOINT`, …), uniqueness is enforced over the PROJECTED names, not
only the declared union; a host refuses the collision before mutation.

`resources` fixes `memoryMiB`, the bound of usable memory — exceeding it
fails the instance observably — and `cpu`, the compute allocation one
instance receives in vCPU. `concurrencyTarget` is the maximum concurrent
requests one instance receives; the host adds instances to hold it.
`minInstances` and `maxInstances` bound the instance count while the active
traffic resource weights this revision: a minimum of zero permits
scale-to-zero, a positive minimum holds warm capacity, and at the maximum
under saturation requests wait no longer than `timeoutSeconds` — the
request's wall-clock budget — and then fail observably. All of these change
what an application and its callers observe and are therefore desired state;
the admitted ranges are Host Support Profile facts.

## Why this is one Form

Image, process invocation, environment, sealed slots, and serving bounds
must travel as one immutable unit, or rollback cannot be exact: re-weighting
to an old revision must restore exactly the behavior it was verified with.

## What would require a separate Form

Mutable in-place configuration or a per-environment overlay breaks the
immutable-revision shape. A source tree the host builds into an image
changes the trust model and is a separate Form. A run-to-completion job is a
different lifecycle.

## Provided Interfaces

None. The runtime contract belongs to the
[ContainerService](container-service.md) identity.

## Accepted Bindings

None in the MVP, and the absence is honest rather than an oversight. A typed
binding today projects a JavaScript surface into `worker.runtime`'s
environment; an arbitrary OCI process has no such namespace to project into,
so realizing `edge.kv` or `edge.sql` inside a container requires a
wire-level realization of the Interface that the binding contracts do not
define. The MVP reaches external state through the standard-service slots
above; typed cross-family bindings arrive when that realization is specified
as its own reviewed contract. This proposal does not invent it.

## Lifecycle risks

A host must be able to start instances of a revision for as long as it
exists: it resolves and verifies the digest before any mutation, and a
registry that later drops the digest must not break a serving revision —
retention is the host's obligation from acceptance onward. Deleting a
revision still weighted by a traffic resource must fail with
`dependency_in_use`. A REQUIRED external-service slot a host cannot satisfy
is refused at plan time with `unsupported_capability`, and an unsatisfied
slot makes the resource not Ready. Import must reproduce the exact snapshot,
digest included.

## Prior art

The immutable revision of a proven serverless container platform: image
digest, environment, resources, concurrency, and instance bounds frozen
together. The withdrawn v1alpha2 `ContainerService` candidate, whose mutable
service document and open value tokens this Form replaces with digest-pinned
revisions.
