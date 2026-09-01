---
layout: home

hero:
  name: Takoform
  text: Terraform / OpenTofu から Takoform Host を使う
  tagline: 型付きリソースをポータブルな Form contract に対応させる Provider
  actions:
    - theme: brand
      text: Provider を使う
      link: /ja/docs/
    - theme: alt
      text: Core v1.0.1 を読む
      link: https://github.com/tako0614/takoform/tree/v1.0.1/spec
---

Takoform Provider は、Terraform / OpenTofu の型付きリソースを、互換 Host が
公開する Form contract に対応させます。Provider `4.0.0` candidateは既存の
`tako0614/takoform` addressで、`tako0614/takoform-forms` から選んだ
exact Edge Form 17種だけを登録します。Registryの
Provider **`3.0.0`** は31 resource aggregateのimmutable historyです。
API/Core **`v1.0.1`** は `forms.takoform.com/v1` のままです。

## Install と configure

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      # Publisher-specific Provider 4公開後に使用します。
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

`endpoint`、`space`、bearer `token` は `TAKOFORM_ENDPOINT`、`TAKOFORM_SPACE`、
`TAKOFORM_TOKEN` からも設定できます。

## Resource reference

全 mapping の生成済み reference:

| Publisher family | Provider 4 mapping |
| --- | ---: |
| Edge | 17 |

各リソースの FormRef、引数、state、import は [Provider reference](/ja/docs/)、
一覧は [mapping inventory](/forms/)、実行可能な検証は [conformance](/conformance/)
を参照してください。過去のリリースと移行方法は
[バージョンと互換性](/ja/docs/versions.html) にまとめています。
AWS、Cloudflare、Kubernetesなどは同じOpenTofu moduleでTakoformと並ぶ
native providerとして宣言します。

## 必要なもの

Provider の利用には、設定する FormRef を実装した互換 Host が必要です。
