# バージョンと互換性

Takoform では、Provider、Host API、Form Family、Form definition、Form Package
API/status の 5 つを独立した軸として扱います。一つの軸の version は、別の軸の
preview や成熟度を意味しません。

## Current design target

| 軸                    | 現在の identity                            | 意味と利用可能性                                                                                                                                                               |
| --------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Provider              | **Provider 2.1.1**                         | Registry 公開済みの stable client distribution。repository descriptor は owner publication 後も設計上 `candidate-only` metadata として残ります。 |
| Host API              | **Host API v1beta1**                       | Beta protocol。discovery、exact Form availability、operation、fence、error を定義します。                                                                                      |
| Form Family           | **Edge Form Family v1beta1**               | Beta family。現在の 15 個の Form definition は Experimental です。                                                                                                          |
| Form definition       | **definition 0.1.0**                       | 現在の 15 個の Experimental Form が使う exact definition version です。                                                                                                        |
| Form Package envelope | **`packages.forms.takoform.com/v1alpha4`** | provider や Host API とは独立した package/distribution schema identifier。package artifact は unpublished です。                                                               |

Provider の配布状態は独立した軸です。**Provider 2.1.1** は現在の Registry
readback client、**Provider 2.0.0** は retained compatibility surface の公開済み
predecessor、**Provider 1.0.3** は既存 v1 state 用の公開済み Legacy client です。

## Published compatibility mapping

| Client / distribution       | Host API          | Form と definition                                                    | 状態 / 用途                                                           |
| --------------------------- | ----------------- | --------------------------------------------------------------------- | --------------------------------------------------------------------- |
| Provider 2.1.1 distribution | Host API v1beta1  | Edge Form Family v1beta1、definition 0.1.0 の Experimental Form 15 個 | Registry 公開済みの current client。descriptor は設計上 `candidate-only` metadata。 |
| Provider 2.0.0 distribution | Host API v1alpha2 | retained provider-v2 Form の既存 definition identity                  | Registry 公開済みの compatibility client。                            |
| Provider 1.0.3 Legacy       | Host API v1alpha1 | 不変の v1 Form Package identity                                       | Registry 公開済みの migration / recovery client。                     |

Host API v1beta1 は wire protocol、Edge Form Family v1beta1 はその上で動く
Form の group です。definition 0.1.0 は Form の identity であり、Provider
2.1.1 の SemVer を変更しません。

## Current design target — Edge Form Family v1beta1 (15 Experimental Forms)

現在の stack は Host API v1beta1 と、次の 15 個の typed resource です。すべて
definition 0.1.0 の Experimental Form です（詳細ページは英語のみ）。

- [`takoform_module_worker`](/docs/resources/module_worker.html)
- [`takoform_worker_bundle`](/docs/resources/worker_bundle.html)
- [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html)
- [`takoform_worker_version`](/docs/resources/worker_version.html)
- [`takoform_worker_deployment`](/docs/resources/worker_deployment.html)
- [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html)
- [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html)
- [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)
- [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html)
- [`takoform_edge_object_bucket`](/docs/resources/edge_object_bucket.html)
- [`takoform_sqlite_database`](/docs/resources/sqlite_database.html)
- [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)
- [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html)
- [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)
- [`takoform_queue_consumer`](/docs/resources/queue_consumer.html)

current target は Registry から利用できます。

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

repository descriptor は owner publication 後も設計上 `candidate-only`
metadata として残ります。この metadata は Registry client の公開状態を変更しません。

## Published compatibility / Migration / History

<details>
<summary>保持される Provider 2.0.0 と Legacy Provider 1.0.3</summary>

### Provider 2.0.0 / Host API v1alpha2 {#provider-2-0-0}

既存の resource URL はそのまま利用できます。次の 9 リソースは現在の Edge Form
Family ではなく、互換性のための履歴として保持されます。

- [`takoform_edge_worker`](/docs/resources/edge_worker.html)
- [`takoform_relational_database`](/docs/resources/relational_database.html)
- [`takoform_object_bucket`](/docs/resources/object_bucket.html)
- [`takoform_key_value_store`](/docs/resources/key_value_store.html)
- [`takoform_queue`](/docs/resources/queue.html)
- [`takoform_schedule`](/docs/resources/schedule.html)
- [`takoform_container_service`](/docs/resources/container_service.html)
- [`takoform_stateful_entity`](/docs/resources/stateful_entity.html)
- [`takoform_vector_index`](/docs/resources/vector_index.html)
- [Interface data source](/docs/data-sources/interface.html)

Provider 2.0.0 は `/.well-known/takoform/v1alpha2` で exact な retained FormRef
を discovery します。retained package index は
`packages.forms.takoform.com/v1alpha3` であり、どちらの identity も現在の
v1beta1 stack へ暗黙に置き換えません。

### Provider 1.0.3 / Host API v1alpha1 {#provider-1-0-3}

Provider 1.0.3 は不変の Legacy client です。既存 v1 state、refresh/delete/recovery、
および v1 wire が必要な移行手順では pin してください。discovery 境界は
`/.well-known/takoform/v1alpha1` で、公開済み v1 Form Package identity は履歴の
証跡として残ります。

### 移行

移行は自動 state rewrite ではなく明示的に行います。

1. Provider 1.0.3 を pin して Legacy resource を refresh する。
2. secret ではない desired configuration と必要な public output を記録する。
3. Provider 2.0.0 で exact な Host API v1alpha2 FormRef の下に create するか、
   host conformance の証明がある場合だけ import する。
4. consumer を切り替えて observe し、rollback が不要になってから Legacy を
   delete する。

[Provider 1 から 2 への migration guide](/release/migrations/v1-to-v2.html) も参照して
ください。公開済み identity は不変です。互換性とは旧 URL と意味を保持することであり、
v1beta1 に名前を変えることではありません。

</details>

<StatusNote />
