---
layout: home

hero:
  name: Takoform
  text: どの provider にも依存しない、1つの provider
  tagline: ポータブルでホスト中立な、Terraform / OpenTofu 用のリソース契約
  actions:
    - theme: brand
      text: 現在の stack を見る
      link: /ja/docs/
    - theme: alt
      text: 仕様を読む
      link: /ja/spec/
---

## Current design target — Provider 2.1.1 / Host API v1beta1

Takoform は Experimental specification project です。provider の人間向け
SemVer、ホスト protocol、Form の identity を別々の軸として示し、一つの version
を別の maturity と取り違えないようにします。

| 軸                    | 現在の identity                        | 意味と利用可能性                                                                                                       |
| --------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Provider              | **Provider 2.1.1**                     | Registry 公開済みの stable client distribution。repository descriptor は owner publication 後も設計上 `candidate-only` metadata として残ります。 |
| Host API              | **Host API v1beta1**                   | discovery、exact Form availability、operation、fence、error の Beta protocol。                                         |
| Form Family           | **Edge Form Family v1beta1**           | Beta family。現在の 15 個の Form definition は Experimental です。                                                      |
| Form definition       | **definition 0.1.0**                   | 現在の Form ごとの exact definition version。                                                                          |
| Form Package envelope | `packages.forms.takoform.com/v1alpha4` | provider や Host API と独立した package/distribution schema identifier。package artifact は unpublished。              |

Provider 2.1.1 は Registry 公開済みの current client distribution です。
repository の `release/version.json` descriptor は owner publication 後も設計上
`candidate-only` metadata として残ります。この metadata は公開済み client を取り消しません。
Provider の distribution availability は下の compatibility section と
[機械可読なステータス](/.well-known/takoform-site.json)で別に示します。
[現在の quick start](/ja/docs/) には適用できる構成があります。

Provider 2.1.1 is Registry-published; its descriptor remains `candidate-only`
metadata after owner publication. The 34 published Form Package identities
belong to immutable Legacy history. No current central Takoform-wide approval
or admission is implied by that historical publication set.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.1.1"
    }
  }
}
```

## Current design target — Edge Form Family v1beta1 (15 Experimental Forms)

Host API v1beta1 の上で Edge Form Family v1beta1
([Form Families](/spec/form-families.html)) が動きます。15 個すべてが
Experimental Form で、definition 0.1.0 を使います。worker は identity、module
bytes、handler を宣言する version、traffic deployment、host が割り当てる address
への attachment という不変 resource の連鎖で到達可能になります。

※ 各リソースの詳細ページは英語のみです。

| Resource                                                                                     | Role       | 宣言するもの                                                            |
| -------------------------------------------------------------------------------------------- | ---------- | ----------------------------------------------------------------------- |
| [`takoform_module_worker`](/docs/resources/module_worker.html)                               | identity   | JavaScript module worker アプリの論理 identity                          |
| [`takoform_worker_bundle`](/docs/resources/worker_bundle.html)                               | revision   | アップロード済みの不変コードバンドル                                    |
| [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html)                   | revision   | アップロード済みの不変 static file inventory                            |
| [`takoform_worker_version`](/docs/resources/worker_version.html)                             | revision   | bundle・handlers・vars・sensitive slots・typed bindings の不変 snapshot |
| [`takoform_worker_deployment`](/docs/resources/worker_deployment.html)                       | deployment | どの version へどれだけ配信するか (basis points)                        |
| [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html)                 | attachment | worker 自身を origin とする hostname                                    |
| [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html)                           | attachment | host が割り当てるアドレスでの HTTPS 到達性                              |
| [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)                   | attachment | scheduled handler を起動する UTC cron                                   |
| [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html)                       | identity   | eventually consistent な edge KV namespace                              |
| [`takoform_edge_object_bucket`](/docs/resources/edge_object_bucket.html)                     | identity   | 強整合な object bucket                                                  |
| [`takoform_sqlite_database`](/docs/resources/sqlite_database.html)                           | identity   | SQLite 意味論の serverless database                                     |
| [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)                 | revision   | 順序と checksum を固定した SQL migration set                            |
| [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html) | attachment | 1つの database へ未適用 suffix だけを適用                               |
| [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)                   | identity   | acknowledgement と retry を持つ at-least-once 配信                      |
| [`takoform_queue_consumer`](/docs/resources/queue_consumer.html)                             | attachment | batch・retry・dead-letter policy を1つの worker へ向ける                |

::: warning Provider distribution boundary
Provider 2.1.1 は Registry 公開済みの stable distribution です。
repository descriptor は owner publication 後も設計上 `candidate-only` metadata として
残り、15 Form Package artifact は unpublished です。target の SemVer、Host API の
Beta protocol、Form family の Beta maturity、15 Form definition の Experimental
maturity は別々の事実です。
[release-policy obligations](/spec/publication-freeze.html)は別の公開義務です。
:::

worker の capability は exact な
[Interface contracts](/spec/interface-contract/) と
[Binding contracts](/spec/binding-contract/) に裏付けられた typed binding で利用します。
外からの activation は別の attachment resource です
([decision 0021](/spec/decisions/0021-third-party-forms-and-contract-distribution.html))。

## 形状を保存する Form

Takoform は複数クラウドの最小公倍数へ薄めた汎用リソースを定義しません。
各 Form は実績あるサービスプリミティブのアプリケーションから見える意味論 —
実行 ABI・整合性・配信保証・更新単位 — を完全に固定し、ベンダーの名前・
アカウント・配置・商務だけを契約の外に置きます。意味論が異なる実装は別の
Form であり、交換可能なのはホストであって意味ではありません
([decision 0008](/spec/decisions/0008-forms-preserve-service-shape.html))。

## Published compatibility / Migration / History

<details>
<summary>Published compatibility: Provider 2.0.0、保持される v1alpha2、Legacy Provider 1.0.3</summary>

### Provider 2.0.0 / Host API v1alpha2

Provider 2.0.0 は retained `forms.takoform.com/v1alpha2` surface の Registry
公開済み compatibility client です。次の 9 resource を保持します
([decision 0035](/spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.html))。

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

resource "takoform_key_value_store" "cache" {
  name                = "cache"
  consistency         = "eventual"
  default_ttl_seconds = 3600
}

resource "takoform_queue" "jobs" {
  name                      = "jobs"
  message_retention_seconds = 345600
  ordering                  = "best_effort"
}

resource "takoform_edge_worker" "api" {
  name                = "api"
  artifact_media_type = "application/vnd.takoform.edge-worker+tar"
  artifact_sha256     = "sha256:0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37"
  artifact_url        = "https://artifacts.example.com/api.tar"
  entrypoint          = "worker.mjs"
  runtime             = "javascript"
  runtime_version     = "2026.1"
  configuration       = { "LOG_LEVEL" = "info" }

  connections = [
    {
      name        = "data"
      resource    = "KeyValueStore/cache"
      permissions = ["read", "write"]
      projection  = "keyvalue.binding.v1"
    },
  ]
}
```

Provider 2.0.0 は `/.well-known/takoform/v1alpha2` で exact な retained FormRef
を discovery し、package index は `packages.forms.takoform.com/v1alpha3` です
([decision 0035](/spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.html))。

| Resource                                                                   | 宣言するもの                                                  |
| -------------------------------------------------------------------------- | ------------------------------------------------------------- |
| [`takoform_edge_worker`](/docs/resources/edge_worker.html)                 | ダイジェストを固定した artifact から動く request/event アプリ |
| [`takoform_relational_database`](/docs/resources/relational_database.html) | open engine token で識別する relational database              |
| [`takoform_object_bucket`](/docs/resources/object_bucket.html)             | オブジェクトストレージ                                        |
| [`takoform_key_value_store`](/docs/resources/key_value_store.html)         | key/value ストア                                              |
| [`takoform_queue`](/docs/resources/queue.html)                             | at-least-once でメッセージを配信するキュー                    |
| [`takoform_schedule`](/docs/resources/schedule.html)                       | 接続したリソースを1つ呼び出す cron                            |
| [`takoform_container_service`](/docs/resources/container_service.html)     | OCI イメージのダイジェストから動くサービス                    |
| [`takoform_stateful_entity`](/docs/resources/stateful_entity.html)         | 参照可能な永続エンティティ                                    |
| [`takoform_vector_index`](/docs/resources/vector_index.html)               | 次元を固定したベクターインデックス                            |

### Provider 1.0.3 / Host API v1alpha1

Provider 1.0.3 は既存 v1 state のための公開済み Legacy client です。refresh、
delete、recovery、v1 wire が残る移行手順では pin してください。discovery 境界は
`forms.takoform.com/v1alpha1` の `/.well-known/takoform/v1alpha1` で、v1 Form
Package identity は不変の履歴です。

### 移行

移行は明示的な create/import で行います。自動 state rewrite はありません。

1. Provider 1.0.3 を pin して Legacy resource を refresh する。
2. secret ではない desired configuration と必要な public output を記録する。
3. Provider 2.0.0 で exact な v1alpha2 FormRef の下に create するか、host
   conformance の証明がある場合だけ import する。
4. consumer を切り替えて observe し、rollback が不要になってから Legacy を
   delete する。

[Provider 1 から 2 への migration guide](/release/migrations/v1-to-v2.html) も参照して
ください。

</details>

## どう動くか

1. **宣言する** — 必要なサービス形状のポータブルなフィールドだけを書きます。
   bundle、handlers、vars、sensitive slots、typed bindings、retention。
2. **ホストが実装する** — provider は versioned な経路でホストを探し、
   validate/prepare/apply、observe、delete を UID・generation・revision の fence
   付きで実行します。実装・配置・容量・資格情報・ルーティングはホストが決めます。
3. **束ねる** — revision が typed binding で能力を持ち、attachment が外界を
   routing します。ホストは対応範囲を Host Support Profile で公開します。

<StatusNote />
