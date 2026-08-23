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

Provider 2.1.1 は Registry 公開済みで、descriptor は owner 公開後も設計として
`candidate-only` metadata のままです。pre-Beta の 2 epoch は撤回されました
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html))。
その identity は retired として台帳に記録され、バイト列はリポジトリ履歴に
残ります。過去の公開集合から現在の承認や admission は一切導かれません。

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
| [`takoform_durable_workflow`](/docs/resources/durable_workflow.html)                         | identity   | deployment が供給する class として多段の durable 実行を持つ            |
| [`takoform_actor_namespace`](/docs/resources/actor_namespace.html)                           | identity   | 実行文脈1つ・専用ストレージ・alarm 1つを持つ addressable actor         |

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
