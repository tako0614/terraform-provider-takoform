# ドキュメント

Terraform / OpenTofu で Takoform を使うための手順と、3 つの利用経路 (lane) の説明です。

| Tier | 対象 | 入手方法 |
| --- | --- | --- |
| **公開済み (Current published)** | provider `v2.0.0` と、保持される 9 つの `forms.takoform.com/v1alpha2` リソース | `terraform init` が Registry からインストール |
| **Edge preview** | provider `v2.1` の source candidate と、未公開の `edge.forms.takoform.com/v1alpha1` family | リポジトリの source から provider をビルド。Registry インストールなし |

下のクイックスタートは公開済み tier です。
[Edge preview](#edge-preview-v1alpha3) はこのページの後半にあり、preview で
あることを常に明示しています。

## クイックスタート

provider `v2.0.0` が公開済みの client で、インストールできる経路です。保持される
provider-v2 の 9 リソースを提供します。provider と 9 種類のリソースをまとめて
動かすには、リポジトリの conformance matrix を使います:

```sh
bun run check:current-form-candidates
go run ./cmd/provider-lifecycle-conformance matrix \
  --opentofu tofu --terraform terraform
```

この matrix は、実ホストに触れずに、exact な v1alpha2 契約に対して
preview/apply/observe/refresh/delete を検証します。実ホスト相手に試す場合は、
そのホストが versioned discovery 経路 (`/.well-known/takoform/v1alpha2`) で
exact な v1alpha2 FormRef を公開していることを先に確認してください。

### 公開済み provider のピン留め

保持される provider-v2 レーンを使うには provider `v2.0.0` をピン留めします。
`init` で Registry からインストールされます。

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

### 実ホストに対して使う

matrix は in-process の参照ホストに対して provider を検証します。実ホストを
動かすには、ホストから 3 つの値をもらいます: API の `endpoint`、対象の
`space`、bearer `token` です。provider 設定に書くか、環境変数
`TAKOFORM_ENDPOINT`・`TAKOFORM_SPACE`・`TAKOFORM_TOKEN` で渡せます。

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}

resource "takoform_edge_worker" "example" {
  name                = "edge-worker"
  artifact_media_type = "application/vnd.takoform.edge-worker+tar"
  artifact_sha256     = "sha256:0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37"
  artifact_url        = "https://artifacts.portable-conformance.invalid/edge-worker.tar"
  entrypoint          = "worker.mjs"
  runtime             = "javascript"
  runtime_version     = "2026.1"
  configuration       = { "LOG_LEVEL" = "info" }
}
```

```console
terraform init
terraform plan
terraform apply
```

provider が mutation を発行する前に、ホストは `/.well-known/takoform/v1alpha2`
で exact な v1alpha2 FormRef を公開していなければなりません。exact な identity
を返せないホストは fail closed します。Takosumi Cloud は最初のホストで、9 種類の
kind すべてを実装しています — endpoint・Space・token はアカウントコンソールで
確認できます。artifact の digest と URL は実際に取得できるものを使ってください。
上の値は形だけの例です。

## 3 つの利用経路

provider の address は `registry.terraform.io/tako0614/takoform` の 1 つだけで、
利用経路は 3 つです。

| 経路 | 用途 | インストール |
| --- | --- | --- |
| **v1.0.3** (公開済み) | 既存の Legacy state の保守・delete・recovery | Registry から |
| **v2.0.0** (公開済み・現在の client) | 保持される provider-v2 の 9 契約 | Registry から |
| **v2.1.0** (Edge preview、source candidate、未公開) | [v1alpha3 レーンの Edge Platform Family](#edge-preview-v1alpha3) | source からビルド。Registry インストールなし |

### 公開済み Legacy の保守

既存の v1 state には、公開済みの provider `v1.0.3` をピン留めしてください。
この provider が state を v2 の意味論に自動変換することはありません。

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.3"
    }
  }
}
```

### v1 からの移行

移行は明示的な create/import で行います。state が自動的に書き換わることは
なく、provider v2 は provider-v1 の state を拒否します。

1. provider v1 をピン留めし、Legacy resource を refresh する。
2. secret を除く desired configuration と、必要な public output を記録する。
3. exact な v1alpha2 FormRef で新規作成するか、ホストの conformance が証明された
   resource だけを import する。
4. consumer を切り替えて observe し、rollback が不要になった時点で、v1 を使って
   Legacy を delete する。

## Edge preview: v1alpha3 レーン {#edge-preview-v1alpha3}

::: warning Edge preview — source のみ
`edge.forms.takoform.com/v1alpha1` family は provider `v2.1.0` に乗ります。
これは未公開の source candidate です。Registry インストールも公開ホストも
ありません。リポジトリの source から provider をビルドしてください。family の
Form・Interface・Binding はどれも公開されておらず lifecycle 記録も持ちません。
[publication blocker](/spec/publication-freeze.html) が open の間、この lane は
凍結されたままです。インストールできる経路は、上の公開済み `v2.0.0`
クイックスタートのままです。
:::

family のリソースは `forms.takoform.com/v1alpha3` Host API を話し、discovery は
`/.well-known/takoform/v1alpha3` です。UID/generation/revision による識別、
long-running operation、content-addressed な artifact upload を備えます。

worker が到達可能になるまでは 1 リソースではなく連鎖です。identity、モジュール
バイト列の不変 bundle、その bytes が export する handler を宣言する不変 version、
トラフィックを送る deployment、そしてアドレスを与える attachment。active な
deployment を持たない worker の endpoint は Ready になりません。したがって
連鎖全体で 1 つの構成です。

```hcl
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

各リソースの個別ページには `v2.1.0` の source-candidate ピンを含む単体の例と、
同じ source のみの警告があります。version への能力付与は typed binding で行い、
外からの起動 — custom domain・cron trigger・queue consumer — は常に別の
attachment リソースです。

## リソースリファレンス {#resource-reference}

※ 各リソースの詳細ページ（引数・interface・import の説明）は英語のみです。

各リソースの引数・read-only 属性・宣言 interface・import の挙動は、次のページに
まとまっています。

保持される provider-v2 リソース (公開済み `v2.0.0`):

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

Edge Platform Family リソース (Edge preview、`v2.1.0` source candidate、未公開):

- [module_worker](/docs/resources/module_worker.html)
- [worker_bundle](/docs/resources/worker_bundle.html)
- [worker_version](/docs/resources/worker_version.html)
- [worker_deployment](/docs/resources/worker_deployment.html)
- [worker_custom_domain](/docs/resources/worker_custom_domain.html)
- [worker_endpoint](/docs/resources/worker_endpoint.html)
- [worker_cron_trigger](/docs/resources/worker_cron_trigger.html)
- [edge_kv_namespace](/docs/resources/edge_kv_namespace.html)
- [edge_object_bucket](/docs/resources/edge_object_bucket.html)
- [sqlite_database](/docs/resources/sqlite_database.html)
- [at_least_once_queue](/docs/resources/at_least_once_queue.html)
- [queue_consumer](/docs/resources/queue_consumer.html)

v1alpha3 lane の surface はこれで全部です。provider が組み込んでいない Form を
運ぶ汎用リソースはありません。組み込んでいない FormRef を client が検証する
手段がこの lane に無いためで、第三者 Form への対応は設定値ではなく provider
build の話になります
([decision 0021](/spec/decisions/0021-third-party-forms-and-contract-distribution.html))。

## ホストとの境界

Takoform が所有するのは、workload semantics、schema、exact identity、package、
conformance だけです。capability support、配置、ルーティング、スケーリング、
資格情報、復旧はホストが、マネージドの容量・課金・クォータ・SLA は
Takosumi Cloud が所有します。

<StatusNote />
