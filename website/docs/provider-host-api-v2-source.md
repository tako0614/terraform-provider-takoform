# Provider 5 consumer reference

Provider 5.0.0 is a fresh-install Provider for Host API
`forms.takoform.com/v2`; this page does not establish current Registry
availability or that a particular Host supports the API or any listed Form.
Check the live Registry listing and exact-version readback before installing
or pinning 5.0.0. Until then, evaluate from source or a local build. The
published Provider 4.1.0 remains the v1 client. **Provider 4 state is not
automatically translated to Provider 5.**

The exact v2 API identity and discovery path are in the Provider's [protocol
identifiers](https://github.com/tako0614/terraform-provider-takoform/blob/main/internal/hostv2/types.go)
and [HTTP client](https://github.com/tako0614/terraform-provider-takoform/blob/main/internal/hostv2/client.go).
The 13 mapped publisher Forms are:

| Terraform resource | Exact Form URL |
| --- | --- |
| `takoform_module_worker` | [ModuleWorker 0.3.0](https://edge.forms.takoform.com/forms/ModuleWorker/0.3.0/) |
| `takoform_worker_bundle` | [WorkerBundle 0.2.0](https://edge.forms.takoform.com/forms/WorkerBundle/0.2.0/) |
| `takoform_worker_version` | [WorkerVersion 0.5.0](https://edge.forms.takoform.com/forms/WorkerVersion/0.5.0/) |
| `takoform_worker_deployment` | [WorkerDeployment 0.4.0](https://edge.forms.takoform.com/forms/WorkerDeployment/0.4.0/) |
| `takoform_worker_endpoint` | [WorkerEndpoint 0.3.0](https://edge.forms.takoform.com/forms/WorkerEndpoint/0.3.0/) |
| `takoform_worker_cron_trigger` | [WorkerCronTrigger 0.3.0](https://edge.forms.takoform.com/forms/WorkerCronTrigger/0.3.0/) |
| `takoform_edge_kv_namespace` | [EdgeKVNamespace 0.2.0](https://edge.forms.takoform.com/forms/EdgeKVNamespace/0.2.0/) |
| `takoform_edge_object_bucket` | [ObjectBucket 0.2.0](https://edge.forms.takoform.com/forms/ObjectBucket/0.2.0/) |
| `takoform_sqlite_database` | [SQLiteDatabase 0.2.0](https://edge.forms.takoform.com/forms/SQLiteDatabase/0.2.0/) |
| `takoform_sqlite_migration_set` | [SQLiteMigrationSet 0.2.0](https://edge.forms.takoform.com/forms/SQLiteMigrationSet/0.2.0/) |
| `takoform_sqlite_migration_application` | [SQLiteMigrationApplication 0.2.0](https://edge.forms.takoform.com/forms/SQLiteMigrationApplication/0.2.0/) |
| `takoform_at_least_once_queue` | [AtLeastOnceQueue 0.2.0](https://edge.forms.takoform.com/forms/AtLeastOnceQueue/0.2.0/) |
| `takoform_queue_consumer` | [QueueConsumer 0.3.0](https://edge.forms.takoform.com/forms/QueueConsumer/0.3.0/) |

The resource `spec` values are typed to their exact Forms. Four Forms have an
empty spec. Artifact Forms accept `artifact.url` and `artifact.sha256`; the
Provider does not publish the bytes or authorize arbitrary artifact fetching.
WorkerVersion's public `vars` object is configured as strict-JSON
`spec.vars_json`.

## Source evaluation and Registry installation

For source development, initialize one private, absolute-path intent store
for the fixed local management scope and exact Host API root. The source
checkout command is:

```sh
go run ./cmd/intent-store-init \
  --path "$INTENT_STORE_PATH" \
  --store-id "$CALLER_SCOPE_ID" \
  --host-api "https://host.example/apis/forms.takoform.com/v2"
```

The built Provider binary exposes the same initializer as
`terraform-provider-takoform intent-store-init --path "$INTENT_STORE_PATH"
--store-id "$CALLER_SCOPE_ID" --host-api
"https://host.example/apis/forms.takoform.com/v2"`. Neither command installs
the Provider through the Registry. Once the exact version appears in the
Registry listing, verify it separately. When the `v5.0.0` tag resolves to the
intended reviewed source, the source verification command is:

```sh
git checkout --detach v5.0.0
```

A source checkout or tag alone is not Registry publication evidence.

`store_id` is a local label, not authentication or proof that a rotated bearer
token still represents the same Host principal. The selected store is bound to
the exact Host API root. With local `store_path` custody, Configure opens the
already initialized store locally and sends no HTTP request. Keep it available
across plans, applies, refreshes, and restarts. Loss, corruption, changed
path/root/ID, or unresolved intent stops mutation rather than silently creating
a replacement.

Alternatively, configure exactly one `intent_store_slot` instead of
`store_path` when the Runner/OpenTofu environment explicitly grants generic
private-file access for that opaque slot. The Provider does not provision this
capability; no default Cloud mount or ambient authentication is implied. For
example, set `intent_store_slot = var.runner_private_slot` only when that exact
slot is granted. During Configure, before resource planning, remote mode may
initialize an empty slot with a compare-and-swap snapshot bound to the same
`store_id` and exact Host API root. This private-store operation is not a Host
API request, and a mismatch or unresolved commit outcome stops before Host
mutation. Before each Host mutation, an exact acknowledged durable journal
commit is required; a lost or unknown commit acknowledgement is accepted only
after an exact receipt is resolved. Unresolved outcomes refuse mutation.

For example, with `runner_private_slot` granted by the execution environment:

```hcl
provider "takoform" {
  endpoint          = "https://host.example"
  host_api_root     = "https://host.example/apis/forms.takoform.com/v2"
  token             = var.host_token
  intent_store_slot = var.runner_private_slot
  store_id          = var.fixed_caller_scope_id
  space             = "prod"
}
```

```hcl
provider "takoform" {
  endpoint      = "https://host.example"
  host_api_root = "https://host.example/apis/forms.takoform.com/v2"
  token         = var.host_token
  store_path    = var.absolute_private_store_path
  store_id      = var.fixed_caller_scope_id
  space         = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}

resource "takoform_worker_endpoint" "api" {
  name = "api-endpoint"
  spec = {
    worker = { resource_uid = takoform_module_worker.api.uid }
  }
}
```

References pass exact Host-assigned UIDs. WorkerVersion refers to a worker and
optionally a bundle; WorkerDeployment refers to its worker and weighted
versions; WorkerEndpoint and WorkerCronTrigger refer to a worker. QueueConsumer
refers to a queue and worker, and may refer to a dead-letter queue. These are
relations between resources, not name-based adoption.

The Host must advertise the exact v2 root and authorize every exact Form used.
If Offerings are advertised, Configure both `offering_id` and
`offering_revision`; otherwise omit both. There is no default Offering.

## Private inputs and recovery boundary

WorkerVersion private input values are supplied only to the Provider process at
apply or same-Operation continuation time through
`TAKOFORM_V2_PRIVATE_INPUTS_JSON`. They are not HCL attributes, public `spec`,
Terraform state, the intent store, command arguments, or diagnostics. The
environment input must be controlled by the process owner; this is not a
Runner delivery guarantee. An accepted `waiting_input` Operation requires the
entire original input map on that same Operation; if unavailable, stop and
reconcile it rather than creating a replacement operation.

Create, Update, and Delete claims persist the original public request and
idempotency key before the first mutation. After acknowledgement, recovery
follows that exact Operation and reads the exact Resource UID. If a claim has
no Operation ID, the unknown outcome is an operator refusal: no new key,
automatic replay, or name-based adoption. Failed accepted resources retain
their UID and name reservation until an explicit same-UID Update or Delete.

Provider 5 uses fresh v2 state. v0/v1 upgrades are refused before HTTP. Import
is by one exact UID and requires an authorized Resource read matching Host
root, Form, Space, UID, name, and generation before binding the store. Import
does not synthesize a Create or migrate legacy state.
