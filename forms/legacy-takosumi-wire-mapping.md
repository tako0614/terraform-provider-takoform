# Historical Takosumi wire mapping for `0.0.0-legacy.1`

This note records the compatibility mapping used to extract the ten
`0.0.0-legacy.1` Form Packages. It is migration evidence, not a normative
Takoform dependency on Takosumi. Takoform owns the provider-neutral Form
Definition and package identities; a host owns its resource envelope,
placement, credentials, policy, lifecycle execution, and observed state.

The current Takoform provider already uses the
`forms.takoform.com/v1alpha1` resource envelope. A bridge to the historical
Takosumi `takosumi.dev/v1alpha1` envelope is therefore a host adapter, not a
change to a Form Package or its digest.

Provider behavior in this note is pinned by
[`../conformance/compatibility-candidate-v1/manifest.json`](../conformance/compatibility-candidate-v1/manifest.json)
and the executable provider parity tests. The host-side mapping was inspected
on 2026-07-16 in `takosumi/contract/resource-shape.ts`,
`takosumi/core/domains/resource-shape/planner.ts`, and
`takosumi/core/api/resource_routes.ts`. That Takosumi working tree had base HEAD
`eb802d658e0109f8f0fc8eb9ffab09b7861be4e9` plus unrelated uncommitted work, so
this is explicitly not an immutable Takosumi source snapshot or a conformance
claim about every deployment.

## Envelope mapping

| Takoform package value | Historical Takosumi resource value | Authority |
| --- | --- | --- |
| exact `FormRef` | registered schema selection for the resource `kind` | package/catalog chooses identity; host verifies support |
| desired fixture/object | `Resource.spec` | Form defines portable validation; host materializes it |
| `FormRef.kind` | `Resource.kind` | values must match |
| desired `name` | `metadata.name` and route name | bridge must require one equal value |
| none | `apiVersion: takosumi.dev/v1alpha1` | host envelope |
| none | `metadata.space`, `project`, `environment`, `owner`, `labels`, `managedBy` | host envelope |
| none | target-pool, policy, provider connection, credential, capacity, or billing selection | host control plane |
| none | `status`, resolution, selected implementation, target, conditions, native identifiers, and outputs | host observation |

The historical Takosumi route parser derives `kind`, `name`, `space`, and
`spec` but does not use the request body's `apiVersion` as Form identity.
Consequently a bridge must verify the exact FormRef and package digest before
constructing a host request; host route acceptance is not package
conformance.

`lifecycleCapabilities` describe which operations an implementation may
support. They do not map to Takosumi's `lifecyclePolicy`, which is host policy
and is intentionally absent from these packages. Likewise, `/name` is the only
immutability proven by the characterized provider. It means replacement
semantics for portable desired data, not ownership of the host envelope.

## Kind coverage

The historical Takosumi contract bundles schemas for these six kinds:

- `EdgeWorker`
- `ObjectBucket`
- `KVStore`
- `Queue`
- `SQLDatabase`
- `ContainerService`

The other four packages are still valid Takoform compatibility candidates,
but a Takosumi host must explicitly register a schema and adapter/plugin before
it can execute them:

- `VectorIndex`
- `DurableWorkflow`
- `StatefulActorNamespace`
- `Schedule`

A matching kind token alone never proves host support.

## Known compatibility gaps

The legacy packages are deliberately stricter than the historical host
parsers. Every desired object uses `additionalProperties: false` (or the
reviewed typed-map escape), while bundled Takosumi parsers reconstruct known
fields and can discard unknown input fields. A bridge must validate against
the exact package first and must never interpret silent field loss as a
successful conversion.

`ObjectBucket.storageClass` is the concrete known divergence. It is present in
the characterized Takoform provider and defaults there to `standard`, but it
is not present in the historical bundled Takosumi `ObjectBucket` contract and
is discarded by that parser. A bridge must fail closed or use an explicitly
registered implementation that preserves the field; it cannot claim the
bundled mapping is lossless.

The Form schemas annotate these characterized defaults:

- `ObjectBucket.storageClass`: `standard`
- `SQLDatabase.engine`: `sqlite`
- `VectorIndex.metric`: `cosine`
- `StatefulActorNamespace.storageProfile`: `durable_sqlite`
- `Schedule.timezone`: `UTC`

JSON Schema `default` is an annotation and does not mutate desired JSON. The
checked-in positive fixtures spell out every value above. A bridge or provider
that applies a default owns that behavior and must preserve the resulting
explicit desired value.

`VectorIndex.dimensions` is required and positive, but it is not listed as
immutable: the characterized Terraform resource has no replacement plan
modifier for it. Documentation language that calls dimensions fixed does not
override executable evidence.

Finally, the empty exact `observedSchema` in every package is intentional. The
historical host's status, resolution, target, implementation, output, and
native-resource evidence are host authority and are not backfilled into a
portable Form Definition.

## Trust boundary

[`legacy-package-set.json`](legacy-package-set.json) pins the ten exact
FormRefs and package digests and points at their executable conformance cases.
It also records `signatureStatus: unsigned` and `publicationReady: false`.
This mapping does not sign, publish, activate, or standardize the packages.
