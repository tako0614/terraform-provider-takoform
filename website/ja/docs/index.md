# ドキュメント

Terraform / OpenTofu で Takoform を使うための手順と、2 つの利用経路 (lane) の説明です。

## クイックスタート

現行ラインは provider `v2.0.0` で、**未公開の source candidate** です。いちばん
手軽な試し方は、リポジトリの conformance matrix を使う方法です。provider を
ビルドして、9 種類すべてのリソースを、Terraform / OpenTofu の隔離された開発用
override で一通り動かします:

```sh
bun run check:current-form-candidates
go run ./cmd/provider-lifecycle-conformance matrix \
  --opentofu tofu --terraform terraform
```

この matrix は、実ホストに触れずに、exact な v1alpha2 契約に対して
preview/apply/observe/refresh/delete を検証します。実ホスト相手に試す場合は、
そのホストが versioned discovery 経路 (`/.well-known/takoform/v1alpha2`) で
exact な v1alpha2 FormRef を公開していることを先に確認してください。

### 現行 source candidate のピン留め

次のピンは、v2 source candidate の identity を示すためのものです。v2 の
リリースが Registry に存在するまでは、通常の `init` ではインストールできません。

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

## 2 つの利用経路

provider の address は `registry.terraform.io/tako0614/takoform` の 1 つだけで、
利用経路は 2 つです。

| 経路 | 用途 | インストール |
| --- | --- | --- |
| **v1.0.3** (公開済み) | 既存の Legacy state の保守・delete・recovery | Registry から |
| **v2.0.0** (source candidate) | 現行の 9 契約 | ソースからビルド + dev override |

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

## リソースリファレンス {#resource-reference}

各リソースの引数・read-only 属性・宣言 interface・import の挙動は、次のページに
まとまっています:

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

## ホストとの境界

Takoform が所有するのは、workload semantics、schema、exact identity、package、
conformance だけです。capability support、配置、ルーティング、スケーリング、
資格情報、復旧はホストが、マネージドの容量・課金・クォータ・SLA は
Takosumi Cloud が所有します。

<div class="status-note">

Takoform は **Experimental specification project** です。現行の FormRef は
`forms.takoform.com/v1alpha2`、現行の package envelope は
`packages.forms.takoform.com/v1alpha3` です。provider `v1.0.3` は公開済みの
Legacy client、provider `v2.0.0` は未公開の source candidate です。
`forms.takoform.com/v1alpha1` の公開済み Form Package identity 34件は、不変の
Legacy 証跡です。現在、中央による承認や admission はありません。

</div>
