---
layout: home

hero:
  name: Takoform
  text: どの provider にも依存しない、1つの provider
  tagline: ポータブルでホスト中立な、Terraform / OpenTofu 用のリソース契約
  actions:
    - theme: brand
      text: はじめる
      link: /ja/docs/
    - theme: alt
      text: 仕様を読む
      link: /ja/spec/

features:
  - title: ホスト中立
    details: 必要なものだけを宣言します — worker、database、queue、bucket。配置・資格情報・容量・価格はホストのもの。
  - title: 9 種類のリソース
    details: EdgeWorker、RelationalDatabase、ObjectBucket、KeyValueStore、Queue、Schedule、ContainerService、StatefulEntity、VectorIndex。
  - title: バージョン管理された契約
    details: すべての Form は不変の FormRef — API group・kind・version・schema digest。互換性は名前から推測しません。
  - title: 実在するホスト
    details: Takosumi Cloud が 9 種類すべてを実装し、本番フィードバックを設計に返しています。
---

## コード例

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}

resource "takoform_edge_worker" "example" {
  name                = "edge-worker"
  artifact_media_type = "application/vnd.takoform.edge-worker+tar"
  artifact_sha256     = "sha256:0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37"
  artifact_url        = "https://artifacts.portable-conformance.invalid/edge-worker.tar"
  entrypoint          = "worker.mjs"
  runtime             = "javascript"
  runtime_version     = "2026.1"
  configuration       = { "LOG_LEVEL" = "info" }
}
```

資格情報・配置・価格は書きません。これらはホストが決めることで、state には
入りません。provider `v2.0.0` は現在の公開済み client で、`v1.0.3` は公開済みの
Legacy client です。[docs](/ja/docs/) で使い方を確認してください。

## リソース

9 種類はすべて `forms.takoform.com/v1alpha2` の `0.1.0` 候補です。

| Resource | 宣言するもの |
| --- | --- |
| [`takoform_edge_worker`](/docs/resources/edge_worker.html) | ダイジェストを固定した artifact から動く request/event アプリ |
| [`takoform_relational_database`](/docs/resources/relational_database.html) | open engine token で識別する relational database |
| [`takoform_object_bucket`](/docs/resources/object_bucket.html) | オブジェクトストレージ |
| [`takoform_key_value_store`](/docs/resources/key_value_store.html) | key/value ストア |
| [`takoform_queue`](/docs/resources/queue.html) | at-least-once でメッセージを配信するキュー |
| [`takoform_schedule`](/docs/resources/schedule.html) | 接続したリソースを1つ呼び出す cron |
| [`takoform_container_service`](/docs/resources/container_service.html) | OCI イメージのダイジェストから動くサービス |
| [`takoform_stateful_entity`](/docs/resources/stateful_entity.html) | 参照可能な永続エンティティ |
| [`takoform_vector_index`](/docs/resources/vector_index.html) | 次元を固定したベクターインデックス |

各リソースは read-only の interface (`http.request@1`、`sql.query@1`、
`object.storage@1`、`queue.messages@1` など) を公開し、他のリソースから
名前と permission で接続できます。[resource reference](/ja/docs/#resource-reference) を参照してください。

## どう動くか

1. **宣言する** — ポータブルなフィールドだけを書きます。name、artifact digest、
   entrypoint、runtime、configuration、connections。
2. **ホストが実装する** — provider は versioned な経路でホストを探し、
   preview/apply、observe、refresh、delete を実行します。実装・配置・容量・
   資格情報・ルーティングはホストが決めます。
3. **接続する** — ホストが宣言済みの interface を公開し、他のリソースが
   名前と permission を指定して接続します。

> **ステータス:** Takoform は **Experimental specification project** です。現行の
> FormRef は `forms.takoform.com/v1alpha2`、現行の package envelope は
> `packages.forms.takoform.com/v1alpha3` です。provider `v1.0.3` は公開済みの
> Legacy client、provider `v2.0.0` は現在の公開済み client です。
> `forms.takoform.com/v1alpha1` の公開済み Form Package identity 34件は、不変の
> Legacy 証跡です。現在、中央による承認や admission はありません。
