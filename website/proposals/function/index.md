# Regional Function Family proposals

The Regional Function Family, `function.forms.takoform.com/v1beta1`, is one
of the eight families of the v1 lineup
([decision 0043](../../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md)).
Its members fix, completely, the application-visible semantics of the
regional function-as-a-service model — an uploaded artifact, one declared
handler, event-driven per-invocation execution — that every major cloud
offers and no de-facto standard API covers, without naming any vendor
([decision 0008](../../spec/decisions/0008-forms-preserve-service-shape.md)).

## Authoring policy: shape-preserving contracts

Every member Form preserves one service shape end to end: execution ABI,
invocation lifecycle, update and delete units, error semantics, and the
capabilities a revision may reach. No free semantic token is admitted; a
difference in semantics is a different Form, never a selector value. Outward
capability use belongs to the revision resource; inward activation is an
attachment resource
([decision 0010](../../spec/decisions/0010-exact-interface-and-binding-contracts.md)).
Desired schemas carry no `name` or envelope plumbing: the resource envelope
owns identity and status
([decision 0011](../../spec/decisions/0011-resource-identity-generation-and-revision.md)).

The family is minted after
[decision 0045](../../spec/decisions/0045-external-standard-services-are-sealed-slots.md),
so its revision Form carries the external standard-service declaration from
birth ([spec/standard-services](../../spec/standard-services/index.md)): a
Function Version reaches PostgreSQL, Redis, S3-compatible storage, or SMTP
through sealed slots with protocol tags, never through endpoints or
credentials in portable state.

These documents are proposals only. No catalog declaration, candidate
package, Interface digest, or FormRef exists yet; a Form enters a family
only through its generated candidate set ([../README.md](../index.md)), so
nothing here reserves a public identity.

## The function aggregate

A Function has at most ONE FunctionDeployment; that deployment selects
Function Versions of its own identity with basis-point weights summing to
exactly 10000; every attachment is admitted against the deployment and
refused when it is absent; and the identity reports itself Ready only while
its deployment actually serves — the same aggregate statement the Edge
family makes in
[decision 0016](../../spec/decisions/0016-the-worker-aggregate-has-one-active-deployment.md).

Cross-family event sources are not members of this family. An attachment
that invokes a function from a queue, topic, bucket, or schedule is declared
in the family that owns the SOURCE, exactly as the Edge family's
`QueueConsumer` stays worker-targeted; scheduled invocation belongs to the
future `schedule.forms.takoform.com` family. The MVP gives the function its
own HTTP activation and nothing else.

## MVP members

| Form | Role | One-line semantics | Separate-Form boundary |
| --- | --- | --- | --- |
| [Function](function.md) | identity | Logical identity of one regional JavaScript function; the ABI is fixed by identity. | A different language runtime, a WASI function, or a container service is a different Form. |
| [FunctionVersion](function-version.md) | revision | Immutable executable snapshot: artifact digest, handler, vars, sensitive slots, external services, memory/timeout/concurrency. | Mutable in-place code or config is not a version of this Form. |
| [FunctionDeployment](function-deployment.md) | deployment | Basis-point traffic shift between at most two Function Versions of one function. | Multi-alias routing or per-request rules are separate work. |
| [FunctionEndpoint](function-endpoint.md) | attachment | Delivers HTTPS requests at a host-assigned address as `http` invocation events. | An author-owned hostname or an access policy is a separate Form. |

Designs that differ in these semantics — other event sources, other
runtimes, scheduled invocation — belong to other families or later members
and get their own proposals when that work starts, per
[spec/form-families.md](../../spec/form-families.md).
