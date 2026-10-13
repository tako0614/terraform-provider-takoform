# Takoform Provider を使う

Terraform / OpenTofu の HCL から、互換 Host が公開する exact Form を管理します。
Host の接続先、Space、認証情報、サービスの実装は選択した Host が提供します。

## 利用する版を選ぶ

| 用途 | Provider | Host API | 対応する resource |
| --- | --- | --- | --- |
| Registry からインストール | **4.1.0** | `forms.takoform.com/v1` | Edge Form 17 種 |
| ソースから評価 | **5.0.0** | `forms.takoform.com/v2` | exact Edge Form URL 13 種 |

4.1.0 の Registry 公開は
[release identity ledger](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/provider-release-identities.json)
に記録されています。5.0.0 を Registry から利用する前に、公開一覧と exact version を
確認してください。ソースの target や tag だけではインストール可能とは判断できません。
**Provider 4 の state は Provider 5 へ自動変換されません。**

## 公開済み Provider 4.1.0 から始める

[入門ガイド](/ja/docs/getting-started.html)に沿って
[Random + Takoform の module](/examples/getting-started/main.tf) を初期化、検証し、plan を確認します。
Random はクラウド認証情報を必要とせず、Takoform resource は選択した Host が管理します。
ガイドには Host の入力値、token の扱い、lock file、EdgeKVNamespace 0.1.0 の Form identity をまとめています。

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 4.1.0"
    }
  }
}
```

Host 設定と resource 宣言はガイドにあります。AWS resource と組み合わせる場合は
[native provider composition](/examples/native-provider-composition/main.tf) を参照してください。
各 Provider の source、認証情報、state は独立しています。

## Provider 5 を評価する

[Provider 5 consumer reference（英語）](/docs/provider-host-api-v2-source.html)に、
13 種の対応表、型付き HCL、local / remote private-store の設定、import、lifecycle の境界をまとめています。
`forms.takoform.com/v2` と exact Form URL に対応する Host、および private intent-store の
設定が必要です。Host の対応と Runner の private-file access は実行環境が提供します。

## Resource reference {#resource-reference}

[Provider reference（英語）](/docs/reference.html#resource-reference)と 17 種の個別ページは
**Provider 4.1.0 / Host API v1** の説明です。ソースの v5 schema と Form URL は上記の
Provider 5 reference を参照してください。

更新前には[バージョンと互換性](/ja/docs/versions.html)と
[v3-to-v4 migration（英語）](/release/migrations/v3-to-v4.html)を確認してください。
実行可能な検証は[Conformance（英語）](/conformance/)、provider-neutral な契約は
[Host API ドキュメント](https://takoform.com/host-api/)にあります。
