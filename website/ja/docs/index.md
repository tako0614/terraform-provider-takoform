# ドキュメント

このページは Specification 1.0 open candidate、Host API v1、exact 8-family /
31-Form corpus から始まります。Provider と historical lane は独立です。

## Specification 1.0 candidate / Host API v1

| Axis                  | Current identity                       | 意味と利用可能性                                                                                                       |
| --------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Specification         | **Takoform 1.0 candidate**             | 3つの local prerequisite が閉じるまで open。                                                                           |
| Host API              | **`forms.takoform.com/v1`**            | stable contract。                                                                                                       |
| Form corpus           | **8 families / 31 Forms**              | exact `0.x` FormRefs。すべて Experimental。                                                                             |
| Form Package envelope | `packages.forms.takoform.com/v1alpha5` | package artifact は unpublished。                                                                                       |
| Provider              | **3.0.0、Registry 公開済み**           | independent non-normative reference implementation。Provider 2.1.1 は retained history。                               |

Specification release、Form maturity、package publication、Provider release は
別の事実です。Specification 1.0 は current Form を `1.0.0` に昇格させません。
Provider 2.1.1 の Registry history と Provider 3.0.0 implementation は
Specification authority ではありません。

## Edge reference family (16 Experimental Forms) {#beta-edge-platform-family}

Family は Host API v1 を話し、`/.well-known/takoform/v1` で discovery します。
UID/generation/revision identity、long-running operation、content-addressed artifact
upload を備えます。

worker が到達可能になるまでは 1 resource ではなく連鎖です。identity、module bytes の
不変 bundle、その bytes が export する handler を宣言する不変 version、traffic を
送る deployment、host が address を与える attachment で構成します。active deployment
のない endpoint は Ready になりません。

```hcl
# Provider 3.0.0 is Registry-published but remains non-normative.

terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = ">= 3.0.0"
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

## Current Provider 3 resource reference {#resource-reference}

独立した Registry 公開済み Provider 3 は current Experimental `0.x` Forms 31 個すべてを
mapping します。resource 名は非 normative な Provider metadata であり、Provider
が公開済みだという主張ではありません（詳細ページは英語のみ）。

### Edge family

- [module_worker](/docs/resources/module_worker.html)
- [worker_bundle](/docs/resources/worker_bundle.html)
- [static_asset_bundle](/docs/resources/static_asset_bundle.html)
- [worker_version](/docs/resources/worker_version.html)
- [worker_deployment](/docs/resources/worker_deployment.html)
- [worker_custom_domain](/docs/resources/worker_custom_domain.html)
- [worker_endpoint](/docs/resources/worker_endpoint.html)
- [worker_cron_trigger](/docs/resources/worker_cron_trigger.html)
- [edge_kv_namespace](/docs/resources/edge_kv_namespace.html)
- [sqlite_database](/docs/resources/sqlite_database.html)
- [sqlite_migration_set](/docs/resources/sqlite_migration_set.html)
- [sqlite_migration_application](/docs/resources/sqlite_migration_application.html)
- [at_least_once_queue](/docs/resources/at_least_once_queue.html)
- [queue_consumer](/docs/resources/queue_consumer.html)
- [durable_workflow](/docs/resources/durable_workflow.html)
- [actor_namespace](/docs/resources/actor_namespace.html)

### Function family

- [function](/docs/resources/function.html)
- [function_version](/docs/resources/function_version.html)
- [function_deployment](/docs/resources/function_deployment.html)
- [function_endpoint](/docs/resources/function_endpoint.html)

### Container family

- [serverless_container_service](/docs/resources/serverless_container_service.html)
- [container_revision](/docs/resources/container_revision.html)
- [container_traffic](/docs/resources/container_traffic.html)
- [container_endpoint](/docs/resources/container_endpoint.html)
- [container_custom_domain](/docs/resources/container_custom_domain.html)

### Table、queue、topic、schedule、vector families

- [table](/docs/resources/table.html)
- [pull_queue](/docs/resources/pull_queue.html)
- [topic](/docs/resources/topic.html)
- [topic_subscription](/docs/resources/topic_subscription.html)
- [message_schedule](/docs/resources/message_schedule.html)
- [dense_vector_index](/docs/resources/dense_vector_index.html)

provider が compile していない Form を運ぶ generic carrier はありません。typed
surface が client に検証できる exact FormRef だけを提供するためです
([decision 0021](/spec/decisions/0021-third-party-forms-and-contract-distribution.html))。

## 撤回された epoch {#lanes}

pre-Beta の 2 epoch（`forms.takoform.com/v1alpha1` Legacy と
`forms.takoform.com/v1alpha2`）は撤回されました
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html))。
それらを運んだ **Provider 2.0.0** と **Provider 1.0.3** は不変の Registry
履歴として exact pin で残りますが、resource に後継はなく、このサイトはもう
文書化しません。既存 state の扱いは
[v2 から v3 への移行境界](/release/migrations/v2-to-v3.html) を参照してください。

## その他の project surface

- [Form Proposals](/proposals/) — 公開 FormRef をまだ得ていない Form の設計資料
- [Form inventory](/forms/) — 現在の 31 Form と retained compatibility identity
- [Conformance evidence](/conformance/) — compatibility の証明方法
- [Release](/release/) — provider publication boundary、Form Package、migration
- [Glossary](/docs/glossary.html) — この documentation の用語

## Host boundary

Takoform は workload semantics、schema、exact identity、package、conformance を所有
します。capability support、配置、routing、scaling、資格情報、recovery、managed
service の live catalog、billing、quota、SLA は host が所有します。

<StatusNote />
