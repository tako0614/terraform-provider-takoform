# ドキュメント

このページは現在の design target — Provider 2.1.1、Host API v1beta1、Edge Form
Family v1beta1 — から始まります。互換性・移行・履歴は下の collapsed section に
置き、旧 contract も到達可能なまま current stack と混同しないようにします。

## Current design target — Provider 2.1.1 / Host API v1beta1

| Axis                  | Current identity                       | 意味と利用可能性                                                                                                       |
| --------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Provider              | **Provider 2.1.1**                     | Registry 公開済みの stable client distribution。repository descriptor は owner publication 後も設計上 `candidate-only` metadata として残ります。 |
| Host API              | **Host API v1beta1**                   | discovery、exact Form availability、operation、fence、error の Beta protocol。                                         |
| Form Family           | **Edge Form Family v1beta1**           | Beta family。現在の 15 個の Form definition は Experimental です。                                                      |
| Form definition       | **definition 0.1.0**                   | 現在の Form ごとの exact definition version。                                                                          |
| Form Package envelope | `packages.forms.takoform.com/v1alpha4` | provider や Host API と独立した package/distribution schema identifier。package artifact は unpublished。              |

provider target、Host API の maturity、Form family の maturity、package publication は
別の事実です。Provider 2.1.1 を Beta と呼びません。Beta は Host API の
protocol、15 Form は Experimental です。Registry readback が Provider の公開状態を
示し、repository descriptor は owner publication 後も設計上 `candidate-only`
metadata として残ります。

Takoform is an Experimental specification project. Provider 2.1.1 is the
Registry-published stable client distribution; its descriptor remains
`candidate-only` metadata after owner publication. The 34 published Form
Package identities belong to immutable Legacy history. No current central
Takoform-wide approval or admission is implied by that historical publication set.

## Current design target — Edge Form Family v1beta1 (15 Experimental Forms) {#beta-edge-platform-family}

Family は Host API v1beta1 を話し、`/.well-known/takoform/v1beta1` で discovery します。
UID/generation/revision identity、long-running operation、content-addressed artifact
upload を備えます。

worker が到達可能になるまでは 1 resource ではなく連鎖です。identity、module bytes の
不変 bundle、その bytes が export する handler を宣言する不変 version、traffic を
送る deployment、host が address を与える attachment で構成します。active deployment
のない endpoint は Ready になりません。

```hcl
# Provider 2.1.1 is Registry-published; release/version.json remains
# candidate-only metadata by design after owner publication.

terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.1.1"
    }
  }
}

provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}

resource "takoform_worker_bundle" "api" {
  name        = "api-bundle"
  main_module = "worker.mjs"

  modules = [
    {
      name         = "worker.mjs"
      content_type = "application/javascript+module"
      content_file = "${path.module}/dist/worker.mjs"
    },
  ]
}

resource "takoform_worker_version" "api" {
  name      = "api-v1"
  worker    = takoform_module_worker.api.name
  bundle    = takoform_worker_bundle.api.name
  handlers  = ["fetch"]
  vars_json = jsonencode({ "LOG_LEVEL" = "info" })
}

resource "takoform_worker_deployment" "api" {
  name   = "api"
  worker = takoform_module_worker.api.name

  versions = [
    {
      worker_version = takoform_worker_version.api.name
      weight         = 10000
    },
  ]
}

resource "takoform_worker_endpoint" "api" {
  name   = "api"
  worker = takoform_module_worker.api.name
}
```

各 resource の個別ページには同じ source-candidate pin と境界があります。version
への capability は typed binding で追加し、custom domain・cron trigger・queue
consumer など外からの activation は別の attachment resource にします。

## Current resource reference {#resource-reference}

現在の Edge Form Family の exact な 15 typed resource です。すべて definition 0.1.0
の Experimental Form です（詳細ページは英語のみ）。

- [module_worker](/docs/resources/module_worker.html)
- [worker_bundle](/docs/resources/worker_bundle.html)
- [static_asset_bundle](/docs/resources/static_asset_bundle.html)
- [worker_version](/docs/resources/worker_version.html)
- [worker_deployment](/docs/resources/worker_deployment.html)
- [worker_custom_domain](/docs/resources/worker_custom_domain.html)
- [worker_endpoint](/docs/resources/worker_endpoint.html)
- [worker_cron_trigger](/docs/resources/worker_cron_trigger.html)
- [edge_kv_namespace](/docs/resources/edge_kv_namespace.html)
- [edge_object_bucket](/docs/resources/edge_object_bucket.html)
- [sqlite_database](/docs/resources/sqlite_database.html)
- [sqlite_migration_set](/docs/resources/sqlite_migration_set.html)
- [sqlite_migration_application](/docs/resources/sqlite_migration_application.html)
- [at_least_once_queue](/docs/resources/at_least_once_queue.html)
- [queue_consumer](/docs/resources/queue_consumer.html)

provider が compile していない Form を運ぶ generic carrier はありません。typed
surface が client に検証できる exact FormRef だけを提供するためです
([decision 0021](/spec/decisions/0021-third-party-forms-and-contract-distribution.html))。

## Published compatibility / Migration / History {#lanes}

<details>
<summary>Published compatibility: Provider 2.0.0、保持される v1alpha2、Legacy Provider 1.0.3</summary>

### Provider 2.0.0 / Host API v1alpha2 の quick start

Provider 2.0.0 は retained Host API boundary
`forms.takoform.com/v1alpha2` 上の Registry 公開済み compatibility client です。
保持される provider-v2 の 9 resource を提供します。provider と 9 種類をまとめて検証するには
repository の conformance matrix を使います。

```sh
bun run check:current-form-candidates
go run ./cmd/provider-lifecycle-conformance matrix \
  --opentofu tofu --terraform terraform
```

matrix は実ホストに触れず exact v1alpha2 contract の preview/apply/observe/refresh/delete
を検証します。実ホストでは、先に `/.well-known/takoform/v1alpha2` で exact FormRef を
公開していることを確認してください。

#### 公開済み Provider 2.0.0 の pin

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.0.0"
    }
  }
}
```

実ホストを動かすには、host から API `endpoint`、対象 `space`、bearer `token` を取得
します。provider 設定か `TAKOFORM_ENDPOINT`、`TAKOFORM_SPACE`、`TAKOFORM_TOKEN` に
渡してください。mutation 前に host が exact v1alpha2 identity を提供できなければ
fail closed します。この repository は hosted service の live availability を主張
しません。

### Provider 2.0.0 の retained resource

次の 9 resource URL は compatibility surface として保持されます。

- [edge_worker](/docs/resources/edge_worker.html)
- [relational_database](/docs/resources/relational_database.html)
- [object_bucket](/docs/resources/object_bucket.html)
- [key_value_store](/docs/resources/key_value_store.html)
- [queue](/docs/resources/queue.html)
- [schedule](/docs/resources/schedule.html)
- [container_service](/docs/resources/container_service.html)
- [stateful_entity](/docs/resources/stateful_entity.html)
- [vector_index](/docs/resources/vector_index.html)
- [interface data source](/docs/data-sources/interface.html)

retained package envelope は `packages.forms.takoform.com/v1alpha3` です。

### Provider 1.0.3 / Host API v1alpha1

既存 v1 state には公開済み Legacy Provider 1.0.3 を pin してください。この provider は
state を v2 semantics に自動変換しません。v1 Host API boundary は
`forms.takoform.com/v1alpha1`、discovery は `/.well-known/takoform/v1alpha1`、公開済み
v1 Form Package identity は不変の履歴です。

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      # 公開済み Legacy Provider 1.0.3（既存 v1 state 用）。
      version = "= 1.0.3"
    }
  }
}
```

### Provider 1.0.3 からの migration

移行は自動 state rewrite ではなく明示的な create/import です。

1. Provider 1.0.3 を pin して Legacy resource を refresh する。
2. secret ではない desired configuration と必要な public output を記録する。
3. Provider 2.0.0 で exact v1alpha2 FormRef の下に create するか、host conformance
   の証明がある場合だけ import する。
4. consumer を切り替えて observe し、rollback が不要になってから Provider 1.0.3
   で Legacy を delete する。

[Provider 1 から 2 への migration guide](/release/migrations/v1-to-v2.html) も参照して
ください。

</details>

## その他の project surface

- [Form Proposals](/proposals/) — 公開 FormRef をまだ得ていない Form の設計資料
- [Form inventory](/forms/) — 現在の 15 Form と retained compatibility identity
- [Conformance evidence](/conformance/) — compatibility の証明方法
- [Release](/release/) — provider publication boundary、Form Package、migration
- [Glossary](/docs/glossary.html) — この documentation の用語

## Host boundary

Takoform は workload semantics、schema、exact identity、package、conformance を所有
します。capability support、配置、routing、scaling、資格情報、recovery、managed
service の live catalog、billing、quota、SLA は host が所有します。

<StatusNote />
