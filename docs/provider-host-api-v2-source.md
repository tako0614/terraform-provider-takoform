# Provider 5 consumer reference

Provider 5.0.0 is a fresh-install Provider for Host API
`forms.takoform.com/v2`. It maps thirteen exact published Edge Form URLs from
the `takoform-forms` publisher to Terraform/OpenTofu resources and never
negotiates or falls back to v1. Check the live Registry listing and exact
version readback before installing or pinning 5.0.0; this reference does not
state current Registry availability. Provider 4.1.0 remains the published v1
client. A compatible Host must independently advertise support for each exact
Form before use.

The Host API identity and discovery path are defined in the Provider's
[v2 protocol identifiers](https://github.com/tako0614/terraform-provider-takoform/blob/main/internal/hostv2/types.go)
and [HTTP client](https://github.com/tako0614/terraform-provider-takoform/blob/main/internal/hostv2/client.go).
These links describe the client implementation; they are not a claim that a
particular Host implements or supports the API.

The mounted Terraform/OpenTofu resource type names are
`takoform_module_worker`, `takoform_worker_bundle`,
`takoform_worker_version`, `takoform_worker_deployment`,
`takoform_worker_endpoint`, `takoform_worker_cron_trigger`,
`takoform_edge_kv_namespace`, `takoform_edge_object_bucket`,
`takoform_sqlite_database`, `takoform_sqlite_migration_set`,
`takoform_sqlite_migration_application`, `takoform_at_least_once_queue`,
and `takoform_queue_consumer`. Their HCL `spec` object is typed per exact Form;
the four empty-spec Forms omit that attribute. Artifact Forms carry typed
`artifact.url` and `artifact.sha256` inputs. This Provider forwards those
identities; it neither publishes artifact bytes nor grants the Host permission
to fetch arbitrary sources. WorkerVersion's arbitrary public `vars` value is
authored as `spec.vars_json`, validated as strict JSON by the exact Form codec.

| Terraform resource | Exact publisher Form URL |
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

### Example: Worker identity and deployment relationships

Resource references use exact Host-assigned UIDs, not name lookup. These
references let Terraform order the resources and pass those UIDs to the
Provider:

```hcl
resource "takoform_module_worker" "api" {
  name = "api"
}

resource "takoform_worker_bundle" "api" {
  name = "api-bundle"
  spec = {
    artifact = {
      url    = var.bundle_url
      sha256 = var.bundle_sha256
    }
  }
}

resource "takoform_worker_version" "api" {
  name = "api-v1"
  spec = {
    worker   = { resource_uid = takoform_module_worker.api.uid }
    bundle   = { resource_uid = takoform_worker_bundle.api.uid }
    handlers = ["fetch"]
  }
}

resource "takoform_worker_deployment" "api" {
  name = "api-live"
  spec = {
    worker = { resource_uid = takoform_module_worker.api.uid }
    versions = [{
      worker_version = { resource_uid = takoform_worker_version.api.uid }
      weight         = 10000
    }]
  }
}

resource "takoform_worker_endpoint" "api" {
  name = "api-endpoint"
  spec = {
    worker = { resource_uid = takoform_module_worker.api.uid }
  }
}
```

`bundle_url` and `bundle_sha256` must identify the same artifact; the example
does not publish or upload it. The Host assigns and returns the worker UID.
The deployment references the worker and version, while the endpoint references
the worker; the Host controls resulting endpoint and runtime behavior.

For a queue relationship, `takoform_queue_consumer.spec.queue` and `.worker`
also take exact UIDs:

```hcl
resource "takoform_at_least_once_queue" "events" {
  name = "events"
  spec = { message_retention_seconds = 86400 }
}

resource "takoform_queue_consumer" "events" {
  name = "events-consumer"
  spec = {
    queue                     = { resource_uid = takoform_at_least_once_queue.events.uid }
    worker                    = { resource_uid = takoform_module_worker.api.uid }
    max_batch_size            = 10
    max_batch_timeout_seconds = 5
    max_concurrency           = 1
    max_retries               = 3
    retry_delay_seconds       = 10
  }
}
```

## Local custody before configure

A caller must explicitly initialize one private, absolute-path intent store
for one fixed management scope and exact Host API root. The store is an
execution-owner durability obligation; do not put it in an ephemeral directory
that disappears between plans, applies, refreshes, or process restarts.

```sh
terraform-provider-takoform intent-store-init \
  --path "$INTENT_STORE_PATH" \
  --store-id "$CALLER_SCOPE_ID" \
  --host-api "https://host.example/apis/forms.takoform.com/v2"
```

A locally built Provider binary exposes this subcommand, so initializing a
store does not require the Go source tree. From a source checkout,
`go run ./cmd/intent-store-init` accepts the same flags. Neither invocation
installs the Provider through the Terraform Registry.

The tool refuses mismatched or nonempty unbound stores. `store-id` is a fixed
local label, **not** authentication or proof that a rotated bearer token has
the same Host principal. The execution owner must retain the store and the
actual Host principal/authorization continuity. Loss, corruption, a changed
path/root/ID, or an unresolved claim stops mutation; it does not silently
create a replacement store.

```hcl
provider "takoform" {
  endpoint      = "https://host.example"
  host_api_root = "https://host.example/apis/forms.takoform.com/v2"
  token         = var.host_token
  store_path    = var.absolute_private_store_path
  store_id      = var.fixed_caller_scope_id
  space         = "prod"
}

resource "takoform_worker_endpoint" "api" {
  name = "api"
  spec = {
    worker = { resource_uid = var.existing_worker_uid }
  }
}
```

Configure validates static values and opens the already initialized store
locally; it sends no HTTP request. Each resource validates state and custody
before v2 discovery. Discovery's `baseUrl` must equal `host_api_root` exactly.
The Host's authorized exact Form support and Offering policy are checked for
a new Create. If Offerings are advertised, set both `offering_id` and
`offering_revision`; otherwise omit both. There is no default Offering.

## Recovery and private inputs

Each Create/Update/Delete durably records the original public request,
idempotency key, first-send time, and advertised replay window before its
first Host mutation. The store excludes bearer credentials and private-input
values. Once an Operation is acknowledged, the Provider follows that exact
Operation and reads the exact Resource UID before settling state. An accepted
ACK whose Terraform state was lost can be reconstructed by those two reads
without a second mutation. A pre-ACK claim with no Operation ID is deliberately
halted: neither a new key nor automatic replay or name-based adoption is
authorized. A failed accepted Resource keeps its UID and name reservation;
recovery is an explicit same-UID Update or Delete. Delete releases state only
after terminal success and exact UID 404 evidence.

The v2 state schema is version 2. v0/v1 state upgrade is an explicit refusal
before HTTP. A fresh UID import uses the configured Space and requires an
authorized exact-UID Resource GET with matching Host root, Form, Space, UID,
name, and generation before the store binds it. Import does not synthesize a
Create, Operation, or prior private input. Do not use import as an automatic
orphan-adoption or legacy-state migration path.

For WorkerVersion `required_sensitive_vars`, the caller supplies the exact
Space/name/input-name map only in the Provider process at apply or same-Operation
continuation time through `TAKOFORM_V2_PRIVATE_INPUTS_JSON`. The source shape
and constraints are in [the private-input source note](private-input-source-v2.md).
Values do not enter `spec`, HCL resource attributes, Terraform state, the
intent store, or diagnostics. A fresh Create requires the full map. An explicit
Update may omit `privateInputs` to retain the Host's prior values; a selected
map must still match the declaration exactly. A Host `waiting_input` Operation
requires the entire original map on that same Operation, never a replacement
Create/Update.

This source and its local fixture tests do not establish a normal Takosumi
Runner's store initialization, persistent filesystem custody, private-input
delivery, live Host conformance, artifact availability, Provider release, or
GA readiness. Those require their separate owning integrations and evidence.
