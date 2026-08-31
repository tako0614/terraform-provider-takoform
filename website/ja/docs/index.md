# ドキュメント

現在の official Form corpus は `edge.forms.takoform.com` 一つだけです。exact な
Experimental `0.x` Form 16個、Interface 7個、Binding 6個を [`takoform-forms`
commit `3a395e4`（英語のみ）](https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e)
で固定しています。

## 現在の normative contract

現在の導線は番号付き Specification receipt ではなく、Host API v1 と Form/Core
contract です（以下の詳細ページは **英語のみ**）。

- [Host API v1（英語のみ）](/spec/host-api/v1.html)
- [Form Definition（英語のみ）](/spec/form-definition/)
- [Form Package（英語のみ）](/spec/form-package/)
- [Interface contracts（英語のみ）](/spec/interface-contract/)
- [Binding contracts（英語のみ）](/spec/binding-contract/)
- [Core contracts（英語のみ）](/spec/core/)

Specification 1.0/1.1 receipt は履歴 archive です。API v1.1 や新しい Form / Provider
release の current lane ではありません。

## Provider boundary と version stream

Takoform には Host API、各 Form definition、Core/library software、Provider software
の4つの独立した version stream があります。Form Package API identifier
`packages.forms.takoform.com/v1alpha5` は data wire/envelope format であり、product の
5つ目の version 軸ではありません。

canonical Registry Provider は official-Forms-only の typed tool です。第三者 Form
package は同じ Host API path と package/verification contract を使い、自分の namespace
で明示的な Provider mapping を公開します。generic carrier や universal infrastructure
provider ではありません。一つの Terraform / OpenTofu module で official Takoform Provider
と他の Takoform / industry-standard Provider を組み合わせられます。

Provider `3.0.0` は公開済み実装ですが、8 family / 31 resource の不変 projection は
Provider release の履歴であり current publisher corpus ではありません。Edge16 の
official-only mapping は次 major candidate（未公開）です。`>=3.1.0` の例も
candidate / unpublished として release record で確認されるまで install を主張しません。

## Current Edge family（16 Forms）

worker は identity、bundle、version、deployment、attachment の不変 resource chain で
構成します。各 resource の詳細は **英語のみ** です。

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

## 非 Edge candidate（履歴・保留、英語のみ）

Function、Container、Table、Pull Queue、Topic、Schedule、Vector の Form は現在の
official corpus ではありません。旧 Provider projection の履歴 source を保持し、Current
navigation から外しています。

- Function: [`takoform_function`（英語のみ・保留）](/docs/resources/function.html)、[`takoform_function_version`（英語のみ・保留）](/docs/resources/function_version.html)、[`takoform_function_deployment`（英語のみ・保留）](/docs/resources/function_deployment.html)、[`takoform_function_endpoint`（英語のみ・保留）](/docs/resources/function_endpoint.html)
- Container: [`takoform_serverless_container_service`（英語のみ・保留）](/docs/resources/serverless_container_service.html)、[`takoform_container_revision`（英語のみ・保留）](/docs/resources/container_revision.html)、[`takoform_container_traffic`（英語のみ・保留）](/docs/resources/container_traffic.html)、[`takoform_container_endpoint`（英語のみ・保留）](/docs/resources/container_endpoint.html)、[`takoform_container_custom_domain`（英語のみ・保留）](/docs/resources/container_custom_domain.html)
- Table / Pull Queue: [`takoform_table`（英語のみ・保留）](/docs/resources/table.html)、[`takoform_pull_queue`（英語のみ・保留）](/docs/resources/pull_queue.html)
- Topic: [`takoform_topic`（英語のみ・保留）](/docs/resources/topic.html)、[`takoform_topic_subscription`（英語のみ・保留）](/docs/resources/topic_subscription.html)
- Schedule / Vector: [`takoform_message_schedule`（英語のみ・保留）](/docs/resources/message_schedule.html)、[`takoform_dense_vector_index`（英語のみ・保留）](/docs/resources/dense_vector_index.html)

source history は削除せず、履歴・保留ページとしてのみ参照できます。

## その他の project surface

- [Provider 3.0 inventory](/forms/)（英語のみ・履歴） — released Provider projection
- [Current Edge16 publisher roster（英語のみ）](https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e)
- [Form Proposals](/proposals/)（英語のみ） — Form 設計資料
- [Conformance evidence](/conformance/)（英語のみ）
- [Release](/release/)（英語のみ） — publication boundary と履歴
- [Glossary](/docs/glossary.html)（英語のみ）

<StatusNote />
