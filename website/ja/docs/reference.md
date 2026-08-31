---
title: リファレンスの入口
description: Form family ごとに Takoform Provider の resource を探すための入口。
---

# リファレンスの入口

この一覧から、exact Form に対応する Provider resource を探せます。各 resource
ページ（英語）は Provider schema から生成され、Terraform の field、default、
import、lifecycle の正本です。

現在の Provider `3.0.0` は、8 つの versionless family に属する Experimental Form
31 個を mapping します。resource 名は Provider の metadata であり、Form identity
や新しい version stream ではありません。

## Edge family

- [module worker](/docs/resources/module_worker.html)
- [worker bundle](/docs/resources/worker_bundle.html)
- [static asset bundle](/docs/resources/static_asset_bundle.html)
- [worker version](/docs/resources/worker_version.html)
- [worker deployment](/docs/resources/worker_deployment.html)
- [worker custom domain](/docs/resources/worker_custom_domain.html)
- [worker endpoint](/docs/resources/worker_endpoint.html)
- [worker cron trigger](/docs/resources/worker_cron_trigger.html)
- [edge KV namespace](/docs/resources/edge_kv_namespace.html)
- [SQLite database](/docs/resources/sqlite_database.html)
- [SQLite migration set](/docs/resources/sqlite_migration_set.html)
- [SQLite migration application](/docs/resources/sqlite_migration_application.html)
- [at-least-once queue](/docs/resources/at_least_once_queue.html)
- [queue consumer](/docs/resources/queue_consumer.html)
- [durable workflow](/docs/resources/durable_workflow.html)
- [actor namespace](/docs/resources/actor_namespace.html)

## Function family

- [function](/docs/resources/function.html)
- [function version](/docs/resources/function_version.html)
- [function deployment](/docs/resources/function_deployment.html)
- [function endpoint](/docs/resources/function_endpoint.html)

## Container family

- [serverless container service](/docs/resources/serverless_container_service.html)
- [container revision](/docs/resources/container_revision.html)
- [container traffic](/docs/resources/container_traffic.html)
- [container endpoint](/docs/resources/container_endpoint.html)
- [container custom domain](/docs/resources/container_custom_domain.html)

## Table、queue、topic、schedule、vector family

- [table](/docs/resources/table.html)
- [pull queue](/docs/resources/pull_queue.html)
- [topic](/docs/resources/topic.html)
- [topic subscription](/docs/resources/topic_subscription.html)
- [message schedule](/docs/resources/message_schedule.html)
- [dense vector index](/docs/resources/dense_vector_index.html)

## resource page の読み方

1. Host Support Profile で Form の `formId` と `definitionVersion` を確認します。
2. field を埋める前に、resource の shape と lifecycle を読みます。
3. host が所有する endpoint、space、資格情報、配置は Form declaration の外に置きます。
4. Provider の SemVer を固定し、apply 前に plan を確認します。

互換性の境界は[バージョンモデル](/ja/docs/versions.html)、所有者の分担は[所有範囲](/ja/docs/ownership.html)
を参照してください。

<StatusNote />
