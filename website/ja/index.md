---
layout: home

hero:
  name: Takoform
  text: ポータブルな契約。複数の provider。
  tagline: ホスト中立なリソース契約のための Terraform / OpenTofu tooling
  actions:
    - theme: brand
      text: 現在の境界を読む
      link: /ja/docs/
    - theme: alt
      text: 履歴資料を見る
      link: /ja/spec/
---

## Stable Host API v1 と独立した version 軸

Takoform の Host API `forms.takoform.com/v1` は、discovery、exact Form
availability、lifecycle operation、fence、error を定める stable な wire
contract です。Specification 1.0/1.1 の receipt は履歴資料であり、API
v1.1、Form の昇格、Provider release を作るものではありません。

現在の official Form corpus は versionless な一つの family、
`edge.forms.takoform.com` です。exact な Experimental `0.x` Form は16個、
Interface は7個、Binding は6個です。正本は [`takoform-forms` commit
`3a395e4`（英語のみ）](https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e)
に固定されています。

Takoform の current version stream は Host API、各 Form definition、Core/library
software、Provider software の4つです。Form Package API identifier
(`packages.forms.takoform.com/v1alpha5`) は wire/envelope format であり、5つ目の
product version 軸ではありません。

| 軸 | 現在の identity | 意味 |
| --- | --- | --- |
| Host API | **`forms.takoform.com/v1`** | stable wire contract。 |
| Form | 各 Form の **`definitionVersion`** | 独立した exact contract。公式 corpus は Edge 16 Forms。 |
| Core/library | 独立した software SemVer | SDK、verifier、compiler、CLI の release。 |
| Provider | 独立した software SemVer | 明示的に対応する official Form の typed tooling。 |

canonical Registry の Takoform Provider は official-Forms-only の tool です。第三者
の Form package は同じ Host API path と package/verification contract を使い、自分の
namespace と明示的な Provider mapping で配布します。generic carrier や universal
infrastructure provider ではありません。一つの Terraform / OpenTofu module で official
Takoform Provider と、他の Takoform または industry-standard Provider を組み合わせられます。

Provider `3.0.0` は Registry 公開済みの実装ですが、その不変の 8 family / 31 resource
projection は Provider release の履歴です。current publisher corpus ではありません。
Edge16 の official-only Provider mapping は次 major の candidate で未公開です。このページ
は将来 version の install 可否を主張しません。`>=3.1.0` を要求する例も release record
が公開を示すまでは candidate / unpublished です。

## Current Edge family（16 Experimental Forms）

詳細ページは現時点ではすべて英語です。リンク名に **（英語のみ）** と明記します。

- [`takoform_module_worker`（英語のみ）](/docs/resources/module_worker.html)
- [`takoform_worker_bundle`（英語のみ）](/docs/resources/worker_bundle.html)
- [`takoform_static_asset_bundle`（英語のみ）](/docs/resources/static_asset_bundle.html)
- [`takoform_worker_version`（英語のみ）](/docs/resources/worker_version.html)
- [`takoform_worker_deployment`（英語のみ）](/docs/resources/worker_deployment.html)
- [`takoform_worker_custom_domain`（英語のみ）](/docs/resources/worker_custom_domain.html)
- [`takoform_worker_endpoint`（英語のみ）](/docs/resources/worker_endpoint.html)
- [`takoform_worker_cron_trigger`（英語のみ）](/docs/resources/worker_cron_trigger.html)
- [`takoform_edge_kv_namespace`（英語のみ）](/docs/resources/edge_kv_namespace.html)
- [`takoform_sqlite_database`（英語のみ）](/docs/resources/sqlite_database.html)
- [`takoform_sqlite_migration_set`（英語のみ）](/docs/resources/sqlite_migration_set.html)
- [`takoform_sqlite_migration_application`（英語のみ）](/docs/resources/sqlite_migration_application.html)
- [`takoform_at_least_once_queue`（英語のみ）](/docs/resources/at_least_once_queue.html)
- [`takoform_queue_consumer`（英語のみ）](/docs/resources/queue_consumer.html)
- [`takoform_durable_workflow`（英語のみ）](/docs/resources/durable_workflow.html)
- [`takoform_actor_namespace`（英語のみ）](/docs/resources/actor_namespace.html)

## Deferred candidate source（履歴・保留、英語のみ）

Function、Container、Table、Pull Queue、Topic、Schedule、Vector の candidate は
current ではありません。旧 Provider projection としての source を残していますが、
Current navigation から外し、履歴・保留として扱います。

- Function: `takoform_function`, `takoform_function_version`, `takoform_function_deployment`, `takoform_function_endpoint`
- Container: `takoform_serverless_container_service`, `takoform_container_revision`, `takoform_container_traffic`, `takoform_container_endpoint`, `takoform_container_custom_domain`
- Table / Pull Queue: `takoform_table`, `takoform_pull_queue`
- Topic: `takoform_topic`, `takoform_topic_subscription`
- Schedule / Vector: `takoform_message_schedule`, `takoform_dense_vector_index`

これらのページは英語のみの履歴資料であり、削除せず source history として保持します。

## 履歴資料

撤回された Host API epoch、Specification receipt、Provider 1/2 release は exact pin
による recovery / migration のための履歴です。現在の version lane ではありません。
[履歴 release](/release/) と
[v2 から v3 への移行境界](/release/migrations/v2-to-v3.html)（英語のみ）を参照してください。

<StatusNote />
