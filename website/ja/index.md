---
layout: home
title: Takoform
description: Terraform と OpenTofu のための移植可能なリソース契約と、4 つの独立した version stream。

hero:
  name: Takoform
  text: 実際のインフラを運ぶ契約。
  tagline: Terraform / OpenTofu 用の型付き Provider。実装、配置、資格情報、routing は host が担います。
  actions:
    - theme: brand
      text: 使い始める
      link: /ja/docs/
    - theme: alt
      text: バージョンモデルを見る
      link: /ja/docs/versions.html
---

## 4 つの独立した version stream

Takoform は wire contract、Form の定義、Core ライブラリ、Provider の
release を混同しません。最初に次の表を確認してください。

| stream          | 現在の形                       | 識別するもの                                         |
| --------------- | ------------------------------ | ---------------------------------------------------- |
| Host API        | `forms.takoform.com/v1`        | discovery と operation の literal lane。             |
| Form            | 各 Form の `definitionVersion` | host が実装する具体的な service shape。              |
| Core ライブラリ | `v1.1.0`                       | 独立して release される Core module / library。      |
| Provider        | `3.0.0`                        | この repository の Registry 公開済み typed mapping。 |

Form Package の envelope、schema ID、content digest、family label は artifact
の識別子です。公開状態は unpublished のままで、追加の version stream ではありません。

## Provider を使う

Registry から Provider を導入し、Host Support Profile を公開する host に
接続します。Provider の release は implementation の metadata であり、host
の能力や Form の安定性を暗黙に保証しません。

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 3.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}
```

[5 分で設定する](/ja/docs/)か、[リファレンスの入口](/ja/docs/reference.html)
から resource の一覧を開いてください。

## 形状を保つ Form

各 Form は、実行 ABI、整合性、配信保証、更新単位、lifecycle を含む一つの
service shape を定義します。配置、容量、資格情報、routing、recovery などの
運用は host が担当します。意味論の異なる実装は別の Form であり、交換できる
のは host です。

現在の Provider は、8 つの versionless family に属する Experimental Form
31 個を mapping します。[リファレンスの入口](/ja/docs/reference.html)から
family を選び、各 resource のページ（英語）で exact field を確認できます。

<details>
<summary>現行 resource 名</summary>

`takoform_serverless_container_service`、`takoform_container_revision`、
`takoform_container_traffic`、`takoform_container_endpoint`、
`takoform_container_custom_domain`、`takoform_module_worker`、
`takoform_worker_bundle`、`takoform_static_asset_bundle`、
`takoform_worker_version`、`takoform_worker_deployment`、
`takoform_worker_custom_domain`、`takoform_worker_endpoint`、
`takoform_worker_cron_trigger`、`takoform_edge_kv_namespace`、
`takoform_sqlite_database`、`takoform_sqlite_migration_set`、
`takoform_sqlite_migration_application`、`takoform_at_least_once_queue`、
`takoform_queue_consumer`、`takoform_durable_workflow`、
`takoform_actor_namespace`、`takoform_function`、`takoform_function_version`、
`takoform_function_deployment`、`takoform_function_endpoint`、
`takoform_pull_queue`、`takoform_message_schedule`、`takoform_table`、
`takoform_topic`、`takoform_topic_subscription`、`takoform_dense_vector_index`

</details>

## 現在のモデルから分離した履歴資料

番号付きの Specification 1.1 receipt と、撤回された 1.0 identity は不変の
履歴資料として残ります。現行の release train、API lane、5 つ目の stream では
ありません。[仕様資料（履歴）](/ja/spec/)と[リリース資料](/ja/docs/history.html)
には案内を付け、読み方を説明しています。

<StatusNote />
