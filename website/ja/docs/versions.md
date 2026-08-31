# バージョンと互換性

Takoform の current version stream は Host API、各 Form definition、Core/library
software、Provider software の4つです。各軸は独立しており、Form Package の API
identifier は wire/envelope format であって product の5つ目の release stream ではありません。

## Current publisher corpus

現在の official corpus は versionless family `edge.forms.takoform.com` 一つです。exact な
Experimental `0.x` Form は16個、Interface は7個、Binding は6個です。正本は
[`takoform-forms` commit `3a395e4`（英語のみ）](https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e)
に固定されています。

| 軸 | 現在の identity | 意味 |
| --- | --- | --- |
| Host API | **`forms.takoform.com/v1`** | discovery、exact Form availability、operation、fence、error の stable wire contract。 |
| Form | 各 Form の **`definitionVersion`** | 独立した exact contract。公式 corpus は Edge16。 |
| Core/library | 独立した software SemVer | SDK、verifier、compiler、CLI の release。 |
| Provider | 独立した software SemVer | 明示的に対応する official Form の typed tooling。 |
| Form Package | **`packages.forms.takoform.com/v1alpha5`** | data envelope / wire format。product version 軸ではない。 |

canonical Registry Provider は official-Forms-only です。第三者 package は同じ Host API
path と package/verification contract を使い、自分の namespace の明示的な Provider
mapping で配布します。module は official Takoform Provider と他の Takoform / industry-standard
Provider を組み合わせられます。

## Provider release と publication boundary

Provider `3.0.0` は Registry 公開済み実装ですが、8 family / 31 resource projection は
その release に固定された履歴であり、current publisher corpus ではありません。Edge16 の
official-only mapping は次 major candidate（未公開）です。`>=3.1.0` の例も
candidate / unpublished であり、release record が変わるまで install を主張しません。

| Distribution | Host API | Form projection | 状態 |
| --- | --- | --- | --- |
| Provider 3.0.0 | Host API v1 | 履歴の 8 family / 31 resource projection | 公開済み Registry artifact。current roster ではない。 |
| Future official-only Provider | Host API v1 | current Edge16 publisher corpus | 次 major candidate。未公開・install 不可。 |
| Provider 2.1.1 | Host API v1beta1 | retained Edge v1beta1 identities | 不変の履歴 client。exact pin のみ。 |
| Provider 2.0.0 / 1.0.3 | 撤回済み pre-Beta epoch | retired identity | recovery / migration 用の履歴。 |

## Current Edge Form links（詳細は英語のみ）

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

## Specification receipt と撤回 lane

Specification 1.0/1.1 receipt は archive evidence であり、current version lane では
ありません。API v1.1、Form `1.0.0`、Provider release を mint しません。撤回された
Host API epoch と Provider client は exact pin による recovery / migration のためにのみ
履歴として残ります。[履歴 release](/release/) と [v2 から v3 への移行境界](/release/migrations/v2-to-v3.html)
（英語のみ）を参照してください。

<StatusNote />
