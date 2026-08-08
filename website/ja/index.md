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
---

## コード例

```hcl
provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}

resource "takoform_edge_kv_namespace" "cache" {
  name = "cache"
}

resource "takoform_at_least_once_queue" "jobs" {
  name                      = "jobs"
  message_retention_seconds = 345600
}

resource "takoform_worker_custom_domain" "api" {
  name     = "api-domain"
  worker   = takoform_module_worker.api.name
  hostname = "api.example.com"
}

resource "takoform_worker_cron_trigger" "cleanup" {
  name   = "cleanup"
  worker = takoform_module_worker.api.name
  cron   = "0 6 * * *"
}
```

資格情報・配置・価格は書きません。これらはホストが決めることで、state には
入りません。上の Edge Platform Family リソースには provider `v2.1.0`
(未公開の source candidate、source からビルド)が必要です。provider
`v2.0.0` は現在の公開済み client で保持される v2 リソースを提供し、`v1.0.3`
は公開済みの Legacy client です。[docs](/ja/docs/) で使い方を確認してください。

## 形状を保存する Form

Takoform は複数クラウドの最小公倍数へ薄めた汎用リソースを定義しません。
各 Form は実績あるサービスプリミティブのアプリケーションから見える意味論 —
実行 ABI・整合性・配信保証・更新単位 — を完全に固定し、ベンダーの名前・
アカウント・配置・商務だけを契約の外に置きます。意味論が異なる実装は別の
Form であり、交換可能なのはホストであって意味ではありません
([decision 0008](/spec/decisions/0008-forms-preserve-service-shape.html))。

## Edge Platform Family (v1alpha3 lane)

現在の設計レーンは namespaced な `edge.forms.takoform.com/v1alpha1`
family ([Form Families](/spec/form-families.html)) で、UID/generation/revision
識別・long-running operation・content-addressed artifact upload を備えた
`forms.takoform.com/v1alpha3` Host API 上で動きます。

※ 各リソースの詳細ページは英語のみです。

| Resource | Role | 宣言するもの |
| --- | --- | --- |
| [`takoform_module_worker`](/docs/resources/module_worker.html) | identity | JavaScript module worker アプリの論理 identity |
| [`takoform_worker_bundle`](/docs/resources/worker_bundle.html) | revision | アップロード済みの不変コードバンドル |
| [`takoform_worker_version`](/docs/resources/worker_version.html) | revision | bundle・compatibility date・handlers・typed bindings の不変 snapshot |
| [`takoform_worker_deployment`](/docs/resources/worker_deployment.html) | deployment | どの version へどれだけ配信するか (basis points) |
| [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html) | attachment | worker 自身を origin とする hostname |
| [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html) | attachment | scheduled handler を起動する UTC cron |
| [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html) | identity | eventually consistent な edge KV namespace |
| [`takoform_edge_object_bucket`](/docs/resources/edge_object_bucket.html) | identity | 強整合な object bucket |
| [`takoform_sqlite_database`](/docs/resources/sqlite_database.html) | identity | SQLite 意味論の serverless database |
| [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html) | identity | acknowledgement と retry を持つ at-least-once 配信 |
| [`takoform_queue_consumer`](/docs/resources/queue_consumer.html) | attachment | batch・retry・dead-letter policy を1つの worker へ向ける |

worker からの能力利用は typed binding (`kv_bindings`、`bucket_bindings`、
`sqlite_bindings`、`queue_producer_bindings`、`service_bindings`) で、exact な
[Interface contracts](/spec/interface-contract/) と
[Binding contracts](/spec/binding-contract/) に裏付けられます。外からの起動
(route・domain・cron・consumption) は常に別の attachment リソースです。
v1alpha3 lane はこの typed リソースだけで、provider が組み込んでいない Form を
運ぶ汎用リソースはありません。組み込んでいない FormRef を client が検証する
手段がこの lane には無いためです
([decision 0021](/spec/decisions/0021-third-party-forms-and-contract-distribution.html))。

## 保持される v2 リソース

`forms.takoform.com/v1alpha2` の 9 種 `0.1.0` 候補は、公開済み provider-v2
の互換面としてそのまま保持されます
([decision 0013](/spec/decisions/0013-v1alpha3-lane-ships-in-provider-v2-1.html))。

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

## どう動くか

1. **宣言する** — 必要なサービス形状のポータブルなフィールドだけを書きます。
   bundle、compatibility date、handlers、typed bindings、retention。
2. **ホストが実装する** — provider は versioned な経路でホストを探し、
   validate/prepare/apply、observe、refresh、delete を UID・generation・
   revision の fence 付きで実行します。実装・配置・容量・資格情報・
   ルーティングはホストが決めます。
3. **束ねる** — revision が typed binding で能力を持ち、attachment が外界を
   routing します。ホストは対応範囲を Host Support Profile で公開します。

<StatusNote />
