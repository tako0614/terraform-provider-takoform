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
公開する Form contract に対応させます。現在のリリースは Provider **`3.0.0`**、
API/Core **`v1.0.1`** で、`forms.takoform.com/v1` を使います。

## Install と configure

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 3.0.0"
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

| Family | Provider 3 mapping |
| --- | ---: |
| Edge | 16 |
| Function | 4 |
| Container | 5 |
| Queue | 1 |
| Schedule | 1 |
| Table | 1 |
| Topic | 2 |
| Vector | 1 |

各リソースの FormRef、引数、state、import は [Provider reference](/ja/docs/)、
一覧は [mapping inventory](/forms/)、実行可能な検証は [conformance](/conformance/)
を参照してください。過去のリリースと移行方法は
[バージョンと互換性](/ja/docs/versions.html) にまとめています。

## 必要なもの

Provider の利用には、設定する FormRef を実装した互換 Host が必要です。
