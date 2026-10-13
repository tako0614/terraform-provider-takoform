---
page_title: "Takoform provider"
description: "Publisher-specific Takoform Terraform/OpenTofu Provider resource reference"
---

# Takoform Provider

Takoform Provider maps Terraform/OpenTofu resources to exact Form contracts
served by a compatible Host. The Host runs the service; the Provider manages
typed configuration, state, and import.

| Use | Provider | Host API | Reference |
| --- | --- | --- | --- |
| Registry installation | 4.1.0 | `forms.takoform.com/v1` | [Getting started](getting-started.md) · [日本語](ja/getting-started.md) |
| Current source evaluation | 5.0.0 | `forms.takoform.com/v2` | [Provider 5 consumer reference](provider-host-api-v2-source.md) |

The resource pages below describe the published v4 mapping. The default
source-built plugin is v5 and has a different schema and thirteen exact Form
URLs. Provider SemVer, Form definitions, and Host support are independent.

## Published Provider 4.1.0 (Host API v1)

Provider 4.1.0 is published at
`registry.terraform.io/tako0614/takoform` and registers 17 exact Edge Forms
selected from `github.com/tako0614/takoform-forms`. Its Host API lane is
`forms.takoform.com/v1`. Provider 3.0.0 remains immutable 31-Form aggregate
history; it has a separate [migration boundary](../release/migrations/v3-to-v4.md).

This relationship is identified by publisher repository and exact FormRefs;
Takoform assigns it no privileged classification.

Point it at a compatible Host:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 4.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://forms.example.com"
  space    = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}
```

`endpoint`, `space`, and bearer `token` may instead come from
`TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.

Provider 4.1.0 adds `token_file` or
`TAKOFORM_TOKEN_FILE` for a run-local token rotated by atomic file replacement.
The provider opens it for each Host request. The file must be a regular file
owned by the provider user with no group/other access, symlink, parent
traversal, or newline. `token` / `TAKOFORM_TOKEN` and a token file are mutually
exclusive. Existing Provider 4.0.0 static-token installations remain valid;
verify the exact Registry version before changing a consumer pin.
The token-file mode is supported on Linux, macOS, FreeBSD, OpenBSD, and NetBSD;
Windows keeps the static-token option only.

Provider `4.1.0` is recorded as Registry-published in the
[Provider release identity ledger](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/provider-release-identities.json), whose entry carries the immutable
GitHub Release and the Registry readback for that version.
Availability is verified, not declared by this immutable documentation.
The exact removal and state boundary is documented in the
[v3-to-v4 migration guide](../release/migrations/v3-to-v4.md).

## Provider 5.0.0 (Host API v2)

Provider 5.0.0 is a fresh-install Provider for Host API
`forms.takoform.com/v2` and thirteen exact Edge Forms. Registry availability is
determined by the live listing and exact-version readback, not this reference.
Check both before installing or pinning 5.0.0. Until that version is listed,
evaluate the source checkout or a local build rather than attempting Registry
installation.
This reference does not establish that a Host supports the Forms. Provider
4.1.0 remains the published v1 Provider. **Provider 4 state is not automatically translated to Provider 5.** Keep existing v4 state with v4 unless an operator plans and verifies a separate migration.

The v2 wire namespace is `forms.takoform.com/v2`; it is separate from each
versioned Form URL. For example, the exact
[ModuleWorker 0.3.0 Form](https://edge.forms.takoform.com/forms/ModuleWorker/0.3.0/)
is one of the thirteen URLs mounted by Provider 5.0.0.

The v5 configuration and lifecycle boundaries are in the
[Provider 5 consumer reference](provider-host-api-v2-source.md). The HCL below shows the v5 schema and the Registry pin to use after
exact-version publication is verified. To evaluate before publication, build
and install a local plugin as described in that reference. Initialize the
private local store before configuring `store_path`, or use exactly one
`intent_store_slot` explicitly granted by the execution environment. The
Host must advertise and authorize the exact Forms before applying:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 5.0"
    }
  }
}

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
```

After the exact Provider version is listed in the Registry, verify that
version before changing a consumer pin. Source-tag verification is separate;
when the `v5.0.0` tag resolves to the intended reviewed source, the exact
checkout is:

```sh
git clone https://github.com/tako0614/terraform-provider-takoform.git
cd terraform-provider-takoform
git checkout --detach v5.0.0
```

A source tag or local build alone is not evidence of Registry publication.

## Resource reference

### Provider 4.1.0: Edge (17)

- [ModuleWorker](resources/module_worker.md), [WorkerBundle](resources/worker_bundle.md), [StaticAssetBundle](resources/static_asset_bundle.md), [WorkerVersion](resources/worker_version.md)
- [WorkerDeployment](resources/worker_deployment.md), [WorkerCustomDomain](resources/worker_custom_domain.md), [WorkerEndpoint](resources/worker_endpoint.md), [WorkerCronTrigger](resources/worker_cron_trigger.md)
- [EdgeKVNamespace](resources/edge_kv_namespace.md), [ObjectBucket](resources/edge_object_bucket.md), [SQLiteDatabase](resources/sqlite_database.md), [SQLiteMigrationSet](resources/sqlite_migration_set.md), [SQLiteMigrationApplication](resources/sqlite_migration_application.md)
- [AtLeastOnceQueue](resources/at_least_once_queue.md), [QueueConsumer](resources/queue_consumer.md), [DurableWorkflow](resources/durable_workflow.md), [ActorNamespace](resources/actor_namespace.md)

Each generated resource page shows the full four-field FormRef, its separate
package digest, typed arguments, state behavior, import contract, and source
Form. The [mapping inventory](../forms/) lists the roster and each
`definitionVersion`; the [identity ledger](../release/provider-form-identities.json)
retains the exact release identities.

## Verify the release

```console
curl -fsS https://registry.terraform.io/v1/providers/tako0614/takoform/versions
git clone https://github.com/tako0614/terraform-provider-takoform.git
cd terraform-provider-takoform
git checkout --detach v4.1.0
```

A source tag, documentation page, or local build alone is not
Registry publication or installation evidence.

Version and migration history are kept in [Versions and compatibility](../website/docs/versions.md), the
[v2-to-v3 guide](../release/migrations/v2-to-v3.md), and the
[publisher-set v3-to-v4 guide](../release/migrations/v3-to-v4.md).

## Native composition with other providers

AWS, Cloudflare, Kubernetes, PostgreSQL, and other providers are peers, not
Takoform implementation details. A module declares any number of provider
sources in `required_providers` and connects their resources through the native
OpenTofu graph. Takoform does not create wrapper Forms, proxy their credentials,
or maintain a central provider catalog. See the
[composition example](../examples/native-provider-composition/main.tf).

## Host requirements

Before applying, confirm that the configured Host advertises support for every
exact FormRef (v4) or Form URL (v5) in the plan. Executable compatibility checks are listed in
[Conformance](../conformance/README.md).
