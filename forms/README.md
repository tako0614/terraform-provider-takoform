# Service Form compatibility inventory

The Phase 0 inventory is exactly these ten kinds and their corresponding statically compiled provider resources:

| Kind | Provider resource | Status |
| --- | --- | --- |
| `EdgeWorker` | `takoform_edge_worker` | compatibility candidate |
| `ObjectBucket` | `takoform_object_bucket` | compatibility candidate |
| `KVStore` | `takoform_kv_store` | compatibility candidate |
| `Queue` | `takoform_queue` | compatibility candidate |
| `SQLDatabase` | `takoform_sql_database` | compatibility candidate |
| `ContainerService` | `takoform_container_service` | compatibility candidate |
| `VectorIndex` | `takoform_vector_index` | compatibility candidate |
| `DurableWorkflow` | `takoform_durable_workflow` | compatibility candidate |
| `StatefulActorNamespace` | `takoform_stateful_actor_namespace` | compatibility candidate |
| `Schedule` | `takoform_schedule` | compatibility candidate |

This inventory freezes the extracted provider surface; it does not assert that
every current field has passed provider-neutral semantic review. Each kind now
has one independent data-only compatibility package under
[`../conformance/form-package-v1/positive/legacy/`](../conformance/form-package-v1/positive/legacy/),
with `definitionVersion` and `packageVersion` set to `0.0.0-legacy.1`.
Every package is strict-verifier and positive-fixture evidence only, remains
`compatibility-candidate`, and contains no host envelope, credential, target,
capacity, provider, billing, or observed-authority fields.

[`legacy-package-set.json`](legacy-package-set.json) is the machine-readable
backfill inventory. It pins the exact ten `(FormRef, packageDigest)` pairs and
their conformance cases. The set is explicitly unsigned and non-publishable;
it is not a multi-form package, catalog release, signature bundle, or host
activation record. The historical host conversion and known loss boundaries
are documented separately in
[`legacy-takosumi-wire-mapping.md`](legacy-takosumi-wire-mapping.md).

Only `/name` is asserted immutable across these definitions. In particular,
`VectorIndex.dimensions` is not marked immutable because the characterized
provider has no replacement plan modifier for it. Unknown host extension fields
are rejected rather than preserved implicitly.

Target-pool, verified-domain, AI-gateway, and every other operator/admin object are outside the inventory.
