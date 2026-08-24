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

## Specification 1.0 candidate / Host API v1

Takoform は Experimental specification project です。provider の人間向け
SemVer、ホスト protocol、Form の identity を別々の軸として示し、一つの version
を別の maturity と取り違えないようにします。

| 軸                    | 現在の identity                        | 意味と利用可能性                                                                                                       |
| --------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Specification         | **Takoform 1.0 candidate**             | normative `spec/` tree の exact committed snapshot が1つ記録されるまで open。                                      |
| Host API              | **`forms.takoform.com/v1`**            | discovery、exact Form availability、operation、fence、error の stable contract。                                        |
| Form corpus           | **8 families / 31 Forms**              | exact current `0.x` FormRefs。すべて Experimental のまま。                                                              |
| Form Package envelope | `packages.forms.takoform.com/v1alpha5` | package artifact は unpublished。                                                                                       |
| Provider              | **3.0.0、Registry 公開済み**           | current 31 Forms の independent non-normative reference implementation。Provider 2.1.1 は retained history。             |

Specification release、Form maturity、Package publication、Provider release は
独立しています。Specification 1.0 は current Form を `1.0.0` に昇格させず、
Provider 3 は Specification を block できません。

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = ">= 3.0.0"
    }
  }
}
```

## Edge reference family (31 current Forms のうち 16)

Host API v1 の上で versionless Form Families が動きます。Edge family の
16 個の exact `0.x` Form はすべて Experimental です。worker は identity、module
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
| [`takoform_sqlite_database`](/docs/resources/sqlite_database.html)                           | identity   | SQLite 意味論の serverless database                                     |
| [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)                 | revision   | 順序と checksum を固定した SQL migration set                            |
| [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html) | attachment | 1つの database へ未適用 suffix だけを適用                               |
| [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)                   | identity   | acknowledgement と retry を持つ at-least-once 配信                      |
| [`takoform_queue_consumer`](/docs/resources/queue_consumer.html)                             | attachment | batch・retry・dead-letter policy を1つの worker へ向ける                |
| [`takoform_durable_workflow`](/docs/resources/durable_workflow.html)                         | identity   | deployment が供給する class として多段の durable 実行を持つ            |
| [`takoform_actor_namespace`](/docs/resources/actor_namespace.html)                           | identity   | 実行文脈1つ・専用ストレージ・alarm 1つを持つ addressable actor         |

Registry 公開済み Provider 3 は、ほかの current Forms 15 個も mapping します。

- Function: [`takoform_function`](/docs/resources/function.html)、[`takoform_function_version`](/docs/resources/function_version.html)、[`takoform_function_deployment`](/docs/resources/function_deployment.html)、[`takoform_function_endpoint`](/docs/resources/function_endpoint.html)
- Container: [`takoform_serverless_container_service`](/docs/resources/serverless_container_service.html)、[`takoform_container_revision`](/docs/resources/container_revision.html)、[`takoform_container_traffic`](/docs/resources/container_traffic.html)、[`takoform_container_endpoint`](/docs/resources/container_endpoint.html)、[`takoform_container_custom_domain`](/docs/resources/container_custom_domain.html)
- Table / queue: [`takoform_table`](/docs/resources/table.html)、[`takoform_pull_queue`](/docs/resources/pull_queue.html)
- Topic: [`takoform_topic`](/docs/resources/topic.html)、[`takoform_topic_subscription`](/docs/resources/topic_subscription.html)
- Schedule / vector: [`takoform_message_schedule`](/docs/resources/message_schedule.html)、[`takoform_dense_vector_index`](/docs/resources/dense_vector_index.html)

::: warning Provider distribution boundary
この resource 名は independent Provider 3 implementation の metadata で、normative
ではありません。Provider 3.0.0 は Registry 公開済みですが、31 Form Package は
unpublished のままです。
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

## 撤回された epoch と公開済み履歴

現在のスタックの前に 2 つの pre-Beta epoch があり、撤回されました
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html))。
それらを載せて公開された provider release は不変の Registry 履歴として残ります。
**Provider 2.0.0**（`forms.takoform.com/v1alpha2` compatibility client）と
**Provider 1.0.3**（`forms.takoform.com/v1alpha1` Legacy client）は既存 state の
維持・recovery・移行のために exact pin で今もインストールできますが、その
resource に後継はなく、このサイトはもう文書化しません。このリポジトリから
次に公開される release は major の `3.0.0` です。撤回された resource の利用者は
[v2 から v3 への移行境界](/release/migrations/v2-to-v3.html) に従ってください。

## どう動くか

1. **宣言する** — 必要なサービス形状のポータブルなフィールドだけを書きます。
   bundle、handlers、vars、sensitive slots、typed bindings、retention。
2. **ホストが実装する** — provider は versioned な経路でホストを探し、
   validate/prepare/apply、observe、delete を UID・generation・revision の fence
   付きで実行します。実装・配置・容量・資格情報・ルーティングはホストが決めます。
3. **束ねる** — revision が typed binding で能力を持ち、attachment が外界を
   routing します。ホストは対応範囲を Host Support Profile で公開します。

<StatusNote />
