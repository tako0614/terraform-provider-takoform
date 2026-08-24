# バージョンと互換性

Takoform では、Specification、Provider、Host API、Form Family、Form definition、
Form Package を独立した軸として扱います。一つの軸の version は、別の軸の
preview や成熟度を意味しません。

## Current design target

| 軸                    | 現在の identity                            | 意味と利用可能性                                                                                                                                                               |
| --------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Specification         | **Takoform 1.0 candidate**                 | exact committed source、exact candidate/corpus、reference conformance が閉じるまで open。                                                                                       |
| Host API              | **`forms.takoform.com/v1`**                | stable Specification contract。                                                                                                                                                 |
| Form corpus           | **8 families / 31 Forms**                  | exact current `0.x` Forms。すべて Experimental。                                                                                                                                |
| Form Package envelope | **`packages.forms.takoform.com/v1alpha5`** | package artifact は unpublished。                                                                                                                                               |
| Provider              | **independent**                            | Provider 2.1.1 は retained history、Provider 3 は non-normative sample。                                                                                                       |

Provider の配布状態は独立した軸です。**Provider 2.1.1** は retained Registry
公開済み client です。**Provider 2.0.0** と **Provider 1.0.3** は撤回された
pre-Beta epoch のための不変の Registry 履歴として残ります
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html))。
このリポジトリから次に公開される release は major の **3.0.0** です
([v2 から v3 への移行境界](/release/migrations/v2-to-v3.html))。

## Published compatibility mapping

| Client / distribution       | Host API          | Form と definition                                                    | 状態 / 用途                                                           |
| --------------------------- | ----------------- | --------------------------------------------------------------------- | --------------------------------------------------------------------- |
| Provider 2.1.1 distribution | Host API v1beta1  | Edge Form Family v1beta1 の immutable historical Form 15 個 | Registry 公開済みの retained client。descriptor は設計上 `candidate-only` metadata。 |
| Provider 2.0.0 distribution | Host API v1alpha2（撤回済み epoch） | 撤回された 9 個の v1alpha2 Form | 不変の Registry 履歴。exact pin のみ、後継なし。 |
| Provider 1.0.3 Legacy       | Host API v1alpha1（撤回済み epoch） | 撤回された v1 Form Package identity | 不変の Registry 履歴。recovery / migration のみ。 |

Host API v1beta1 は wire protocol、Edge Form Family v1beta1 はその上で動く
Form の group です。definition 0.1.0 は Form の identity であり、Provider
2.1.1 の SemVer を変更しません。

## Current Edge reference family (16 Experimental Forms)

現在の versionless Edge family は次の 16 個の exact Experimental `0.x` Form
です（詳細ページは英語のみ）。

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

independent Provider 3 sample の resource type 名は次の version floor を使います。

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

repository descriptor は owner publication 後も設計上 `candidate-only`
metadata として残ります。この metadata は Registry client の公開状態を変更しません。

## 撤回された epoch

pre-Beta の epoch とその文書は撤回されました
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html))。
撤回された resource ページはこのサイトに存在しません。identity は公開台帳に
retired として記録され、バイト列は git 履歴と release tag に残ります。撤回
された resource の既存利用者は exact pin（`= 2.0.0` / `= 2.1.1`、v1 state は
`= 1.0.3`）を維持するか、
[v2 から v3 への移行境界](/release/migrations/v2-to-v3.html) に従ってください。
自動 migration はなく、9 resource のいずれかを state に残したまま撤回を越えて
upgrade すると、lifecycle request の前に fail closed します。

<StatusNote />
