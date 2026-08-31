---
title: はじめる
description: Takoform Provider を導入し、host に接続して最初の resource を宣言します。
---

# Takoform をはじめる

空の Terraform / OpenTofu module から、型付き Takoform resource を host に
接続するまでの最短手順です。現行のバージョンモデルは 4 つの独立した流れを
持ちますが、このページでは Provider と host の接続だけを扱います。

## 1. Provider を導入する

Registry の source と利用する major を指定します。

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 3.0"
    }
  }
}
```

`terraform init`（または `tofu init`）を実行して、exact な Provider release
を選びます。Provider の SemVer は Host API lane、各 Form の
`definitionVersion`、Core ライブラリの SemVer から独立しています。

## 2. host を指定する

Provider には endpoint と host 所有の space が必要です。資格情報、配置、容量、
routing、recovery は host の責任です。

```hcl
provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}
```

host は Host Support Profile で対応 Form を知らせます。現行の wire lane は
literal な `forms.takoform.com/v1` で、`/v1.1` lane はありません。

## 3. shape を宣言する

[リファレンスの入口](/ja/docs/reference.html)から resource を選び、その Form
に属するポータブルな field だけを指定します。たとえば worker endpoint は
worker identity に対する attachment です。

```hcl
resource "takoform_module_worker" "api" {
  name = "api"
}

resource "takoform_worker_endpoint" "api" {
  name   = "api"
  worker = takoform_module_worker.api.name
}
```

各 Form のページ（英語）に exact field、lifecycle、attachment の規則があります。
Provider が compile していない Form を運ぶ generic carrier はありません。
Form Package の公開状態は unpublished であり、追加の version stream ではありません。

## 4. plan を確認する

`terraform plan`（または `tofu plan`）を実行し、apply 前に host の応答を確認します。
未対応の Form、古い generation、無効な attachment は host が拒否します。これは
contract boundary の証拠なので、version label を変更せず宣言か host support を
修正してください。

## 次に読むページ

<details>
<summary>resource 一覧（現行 Form 31 個）</summary>

- [`takoform_serverless_container_service`](/docs/resources/serverless_container_service.html)
- [`takoform_container_revision`](/docs/resources/container_revision.html)
- [`takoform_container_traffic`](/docs/resources/container_traffic.html)
- [`takoform_container_endpoint`](/docs/resources/container_endpoint.html)
- [`takoform_container_custom_domain`](/docs/resources/container_custom_domain.html)
- [`takoform_module_worker`](/docs/resources/module_worker.html)
- [`takoform_worker_bundle`](/docs/resources/worker_bundle.html)
- [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html)
- [`takoform_worker_version`](/docs/resources/worker_version.html)
- [`takoform_worker_deployment`](/docs/resources/worker_deployment.html)
- [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html)
- [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html)
- [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)
- [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html)
- [`takoform_sqlite_database`](/docs/resources/sqlite_database.html)
- [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)
- [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html)
- [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)
- [`takoform_queue_consumer`](/docs/resources/queue_consumer.html)
- [`takoform_durable_workflow`](/docs/resources/durable_workflow.html)
- [`takoform_actor_namespace`](/docs/resources/actor_namespace.html)
- [`takoform_function`](/docs/resources/function.html)
- [`takoform_function_version`](/docs/resources/function_version.html)
- [`takoform_function_deployment`](/docs/resources/function_deployment.html)
- [`takoform_function_endpoint`](/docs/resources/function_endpoint.html)
- [`takoform_pull_queue`](/docs/resources/pull_queue.html)
- [`takoform_message_schedule`](/docs/resources/message_schedule.html)
- [`takoform_table`](/docs/resources/table.html)
- [`takoform_topic`](/docs/resources/topic.html)
- [`takoform_topic_subscription`](/docs/resources/topic_subscription.html)
- [`takoform_dense_vector_index`](/docs/resources/dense_vector_index.html)

</details>

- [バージョンモデル](/ja/docs/versions.html) — 4 つの流れと、流れではない artifact identity。
- [概念](/ja/docs/concepts.html) — portability、Form identity、lifecycle。
- [所有範囲](/ja/docs/ownership.html) — 各判断を所有する repository / runtime。
- [リファレンスの入口](/ja/docs/reference.html) — 現行 family と resource のリンク。
- [履歴](/ja/docs/history.html) — Specification receipt と撤回された provider epoch。

<StatusNote />
