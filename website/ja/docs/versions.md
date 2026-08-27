# バージョンと互換性

Takoform の domain version axis は二つだけです。Host API major と、各 Form
自身の `definitionVersion` です。一方の domain axis の version は、もう一方の
preview や成熟度を意味しません。Provider SemVer、package/schema の ID や
digest、Interface/Binding ref、Specification release、保持された lane は追加の
domain axis ではなく、artifact/history identity です。

凍結された predecessor source は
[`spec/versioning.md`](https://github.com/tako0614/terraform-provider-takoform/blob/896fb0e6c94557d97ba7445924fda18a8430ba8f/spec/versioning.md)
に残っています。以下の current Provider page はその historical file とは分離して
管理します。

## Current design target

| domain axis | 現在の identity | 意味と利用可能性 |
| ---------- | --------------- | ---------------- |
| Host API major | **`forms.takoform.com/v1`**（unpublished candidate） | protocol envelope、discovery、exact Form availability、operations、fencing、errors。 |
| Form `definitionVersion` | **8 versionless families / 31 exact Forms** | 各 Form が独自の Experimental `0.x` SemVer を持つ。Form の変更で family generation は作らない。 |

## Artifact / history identity

| identity | 現在または保持された意味 |
| -------- | ------------------------ |
| Specification **1.1** | exact な normative source snapshot 一つの release identity。Host API v1、Form、package、Provider 3 を公開・昇格しない。1.0 は公開前に撤回され再利用しない。 |
| Form Family group | 各 exact FormRef の versionless namespace。古い versioned group は retained / withdrawn history として残る。 |
| Form Package envelope **`packages.forms.takoform.com/v1alpha5`** | manifest format の identity。package ID と content digest は artifact bytes を識別し、package 公開は Provider 公開と独立する。 |
| Schema ID / digest | exact な Definition または wire schema の bytes。別の Form version stream ではない。 |
| Interface/Binding ref / digest | Form が参照する exact な operation-surface / typed-capability contract。 |
| Provider **3.0.0、Registry 公開済み** | current Forms 向け non-normative Terraform/OpenTofu client distribution。 |
| Provider **2.1.1、Registry 公開済み** | Host API / Form Family `v1beta1` の 15 個の immutable FormRef を持つ retained Beta client。identity は [provider Form identity ledger](/release/provider-form-identities.json) に記録される。 |
| Provider **2.0.0 / 1.0.3** | withdrawn pre-Beta client identity。不変の Registry 履歴として exact-pin recovery / migration のみ。 |

Provider の配布状態は domain axis ではなく artifact identity です。**Provider 3.0.0** が current Registry
公開済み implementation です。**Provider 2.1.1** は retained Beta `v1beta1`
history で、
Host API `forms.takoform.com/v1beta1` と
`edge.forms.takoform.com/v1beta1` family を対象にし、[provider Form identity
ledger](/release/provider-form-identities.json) に記録された exact FormRef の
ために利用できます。一方、**Provider 2.0.0** と **Provider 1.0.3** は
withdrawn pre-Beta epoch であり、それぞれの Registry identity は
exact-pin recovery / migration のための不変の履歴として残ります
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html))。
current major release は **3.0.0** です。新しい Form の
`definitionVersion` の公開は Provider release を待ちません。
([v2 から v3 への移行境界](/release/migrations/v2-to-v3.html))。

## Published compatibility mapping

| Client / distribution       | Host API          | Form と definition                                                    | 状態 / 用途                                                           |
| --------------------------- | ----------------- | --------------------------------------------------------------------- | --------------------------------------------------------------------- |
| Provider 3.0.0 distribution | Host API v1 candidate       | 8 versionless families / current Experimental Form 31 個              | current Registry 公開済み non-normative reference implementation。    |
| Provider 2.1.1 distribution | Host API v1beta1  | Edge Form Family v1beta1 の immutable historical Form 15 個 | Registry 公開済みの retained client。descriptor は設計上 `candidate-only` metadata。 |
| Provider 2.0.0 distribution | Host API v1alpha2（撤回済み epoch） | 撤回された 9 個の v1alpha2 Form | 不変の Registry 履歴。exact pin のみ、後継なし。 |
| Provider 1.0.3 Legacy       | Host API v1alpha1（撤回済み epoch） | 撤回された v1 Form Package identity | 不変の Registry 履歴。recovery / migration のみ。 |

Host API v1beta1 は wire protocol、Edge Form Family v1beta1 はその上で動く
Form の group です。`definitionVersion` は Form ごとの唯一の domain version で
あり、`schemaDigest` と group は identity material です。definition 0.1.0 は
Form の identity であり、Provider 2.1.1 の SemVer を変更しません。新しい Form
の公開は Provider release を必要としません。

## Form / Package publication 境界の publisher parity

Form と Form Package の publication 境界では、official publisher と
external/third-party publisher は equal です。異なるのは publisher の
identity/domain だけで、contract、verification、trust/admission、lifecycle、
authority の規則は同じです。official namespace だからといって強い semantics
は与えられません。operator が trusted source、issuer、signature/revocation
policy、Host Support policy を明示的に選び、publisher provenance は FormRef
の equality に含めません。

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

Registry 公開済み Provider 3 の resource type 名は次の version floor を使います。

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
された resource の既存利用者は exact pin（`= 2.0.0`、v1 state は
`= 1.0.3`）を維持するか、
[v2 から v3 への移行境界](/release/migrations/v2-to-v3.html) に従ってください。
自動 migration はなく、9 resource のいずれかを state に残したまま撤回を越えて
upgrade すると、lifecycle request の前に fail closed します。

<StatusNote />
