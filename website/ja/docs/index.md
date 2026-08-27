# Takoform Provider

Takoform Provider は、Terraform / OpenTofu の型付きリソースを、互換 Host が
公開する Form contract に対応させます。resource identity と desired state は
Terraform state に保持され、実際のリソースは Host が実行します。現在の
API/Core は **`v1.0.1`** で、`forms.takoform.com/v1` を使います。

## Install と configure

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 3.0.0"
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

`endpoint`、`space`、bearer `token` は `TAKOFORM_ENDPOINT`、
`TAKOFORM_SPACE`、`TAKOFORM_TOKEN` からも設定できます。

Provider 3 は 8 family、31 リソースを収録します。リソース名は Provider の
metadata であり、contract の意味は各ページからリンクする Form Definition と
[Core v1.0.1 の仕様](https://github.com/tako0614/takoform/tree/v1.0.1/spec)
で定義されます。

## Resource reference {#resource-reference}

生成された reference は Provider 3 の全 mapping を収録します。

### Edge (16)

- [`takoform_module_worker`](/docs/resources/module_worker.html)、[`takoform_worker_bundle`](/docs/resources/worker_bundle.html)、[`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html)、[`takoform_worker_version`](/docs/resources/worker_version.html)
- [`takoform_worker_deployment`](/docs/resources/worker_deployment.html)、[`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html)、[`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html)、[`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)
- [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html)、[`takoform_sqlite_database`](/docs/resources/sqlite_database.html)、[`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)、[`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html)
- [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)、[`takoform_queue_consumer`](/docs/resources/queue_consumer.html)、[`takoform_durable_workflow`](/docs/resources/durable_workflow.html)、[`takoform_actor_namespace`](/docs/resources/actor_namespace.html)

### Function (4)

- [`takoform_function`](/docs/resources/function.html)、[`takoform_function_version`](/docs/resources/function_version.html)、[`takoform_function_deployment`](/docs/resources/function_deployment.html)、[`takoform_function_endpoint`](/docs/resources/function_endpoint.html)

### Container (5)

- [`takoform_serverless_container_service`](/docs/resources/serverless_container_service.html)、[`takoform_container_revision`](/docs/resources/container_revision.html)、[`takoform_container_traffic`](/docs/resources/container_traffic.html)、[`takoform_container_endpoint`](/docs/resources/container_endpoint.html)、[`takoform_container_custom_domain`](/docs/resources/container_custom_domain.html)

### Queue、schedule、table、topic、vector (6)

- [`takoform_pull_queue`](/docs/resources/pull_queue.html)、[`takoform_message_schedule`](/docs/resources/message_schedule.html)、[`takoform_table`](/docs/resources/table.html)、[`takoform_topic`](/docs/resources/topic.html)、[`takoform_topic_subscription`](/docs/resources/topic_subscription.html)、[`takoform_dense_vector_index`](/docs/resources/dense_vector_index.html)

各リソースページには、4 フィールドすべてを含む FormRef、別管理の package
digest、引数、state、import の仕様、参照元の Form が載っています。
[mapping inventory](/forms/) はリソース一覧と各 `definitionVersion` を、
[identity ledger](/release/provider-form-identities.json) はリリース時の正確な
identity を記録します。

## History と migration

現在の互換性と過去のリリースは [バージョンと互換性](/ja/docs/versions.html)
にまとめています。古い Provider state は
[v2-to-v3 migration](/release/migrations/v2-to-v3.html) を確認してください。

実行可能な互換性検証は [Conformance](/conformance/) にまとめています。

## Apply 前に確認

Apply 前に、plan に含まれる各 FormRef を Host がサポートしていることを
確認してください。
