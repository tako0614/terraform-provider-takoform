# Takoform Provider

Takoform Provider は、Terraform / OpenTofu の型付きリソースを、互換 Host が
公開する Form contract に対応させます。resource identity と desired state は
Terraform state に保持され、実際のリソースは Host が実行します。現在の
API/Core は **`v1.0.1`** で、`forms.takoform.com/v1` を使います。

## Publisher-specific next major

この source checkout は既存の `registry.terraform.io/tako0614/takoform`
address を維持し、`github.com/tako0614/takoform-forms` から選んだ exact Form 17 種だけを
登録します。Provider `3.0.0` の 31 resource aggregate は immutable history
として残します。次 major の Registry 公開はまだ主張しません。

この関係は publisher repository と exact FormRefs で識別します。Takoform は
特権的な分類を付けません。

次 major の公開後は次のように設定します。

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 4.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://forms.example.com"
  space    = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}
```

`endpoint`、`space`、bearer `token` は `TAKOFORM_ENDPOINT`、
`TAKOFORM_SPACE`、`TAKOFORM_TOKEN` からも設定できます。

リソース名は Provider の metadata であり、contract の意味は各ページから
リンクする publisher Form Definition と
[Core v1.0.1 の仕様](https://github.com/tako0614/takoform/tree/v1.0.1/spec)
で定義されます。

## Resource reference {#resource-reference}

生成された reference は、この Provider が選んだ全17 mappingを収録します。

### Edge (17)

- [`takoform_module_worker`](/docs/resources/module_worker.html)、[`takoform_worker_bundle`](/docs/resources/worker_bundle.html)、[`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html)、[`takoform_worker_version`](/docs/resources/worker_version.html)
- [`takoform_worker_deployment`](/docs/resources/worker_deployment.html)、[`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html)、[`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html)、[`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)
- [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html)、[`takoform_edge_object_bucket`](/docs/resources/edge_object_bucket.html)、[`takoform_sqlite_database`](/docs/resources/sqlite_database.html)、[`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)、[`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html)
- [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)、[`takoform_queue_consumer`](/docs/resources/queue_consumer.html)、[`takoform_durable_workflow`](/docs/resources/durable_workflow.html)、[`takoform_actor_namespace`](/docs/resources/actor_namespace.html)

各リソースページには、4 フィールドすべてを含む FormRef、別管理の package
digest、引数、state、import の仕様、参照元の Form が載っています。
[mapping inventory](/forms/) はリソース一覧と各 `definitionVersion` を、
[identity ledger](/release/provider-form-identities.json) はリリース時の正確な
identity を記録します。

## History と migration

現在の互換性と過去のリリースは [バージョンと互換性](/ja/docs/versions.html)
にまとめています。古い Provider state は
[v2-to-v3 migration](/release/migrations/v2-to-v3.html) を確認してください。
Provider 3 からの更新前には [v3-to-v4 publisher-set
boundary](/release/migrations/v3-to-v4.html) を確認してください。

## 他providerとのnative連携

AWS、Cloudflare、Kubernetes、PostgreSQLなどの業界標準providerは
`required_providers` の対等なsourceとして宣言します。OpenTofuが複数providerの
install、alias、plan/state、dependency graphを管理します。Takoformはそれらを
Formでwrapせず、provider catalogも持ちません。
[native provider composition example](/examples/native-provider-composition/main.tf)
を参照してください。

実行可能な互換性検証は [Conformance](/conformance/) にまとめています。

## Apply 前に確認

Apply 前に、plan に含まれる各 FormRef を Host がサポートしていることを
確認してください。
