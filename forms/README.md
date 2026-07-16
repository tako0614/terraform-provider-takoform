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

This inventory freezes the extracted provider surface; it does not assert that every current field has passed provider-neutral semantic review. The generic FormRef, Form Definition, package-index, canonicalization, and local-verification contracts now exist, but individual versioned definitions for these ten kinds are intentionally absent until each kind passes lifecycle, portability, security, and conformance review. Package signing and publication also remain pending.

Target-pool, verified-domain, AI-gateway, and every other operator/admin object are outside the inventory.
