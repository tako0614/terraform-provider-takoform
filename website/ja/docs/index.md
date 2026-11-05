# Takoform Provider を使う

Terraform / OpenTofu の HCL から、互換 Host が公開する exact Form を管理します。
Host の接続先、Space、認証情報、サービスの実装は選択した Host が提供します。

## 利用する版を選ぶ

| 用途 | Provider | Host API | 対応する resource |
| --- | --- | --- | --- |
| 現行の source target / candidate | **5.0.0** | `forms.takoform.com/v2` | exact Edge Form URL 13 種 |
| 非推奨の Registry 公開版 | **4.1.0** | `forms.takoform.com/v1` | Edge Form 17 種 |

通常の source build で使う Provider は Host API v2 です。v1 client は非推奨で、default
entrypoint ではサポートされません。4.1.0 の Registry 公開は
[release identity ledger](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/provider-release-identities.json)
に記録されています。Provider 5.0.0 は source target で、Registry 公開済みとは確認されていません。
インストール前に公開一覧と exact version の readback を確認してください。
**Provider 4 の state は Provider 5 へ自動変換されません。**

## 現行の source path: Provider 5 を評価する

[Provider 5 consumer reference（英語）](/docs/provider-host-api-v2-source.html)に、現行の v2 schema、
13 種の対応表、local / remote private-store の設定、import、lifecycle の境界をまとめています。
Host は `forms.takoform.com/v2` と exact Form URL に独立して対応している必要があります。

## 非推奨の Registry 公開版 Provider 4.1.0

[旧版の入門ガイド](/ja/docs/getting-started.html)は、既存 workspace 向けの Registry 公開済み
Provider 4.1.0 / Host API v1 を説明します。この release は default Provider 5 entrypoint では
サポートされません。

## Resource reference {#resource-reference}

[Provider reference（英語）](/docs/reference.html#resource-reference)と 17 種の個別ページは
非推奨の **Provider 4.1.0 / Host API v1** の説明です。現行の source-v2 schema と Form URL は
上記の Provider 5 reference を参照してください。

更新前には[バージョンと互換性](/ja/docs/versions.html)と
[v3-to-v4 migration（英語）](/release/migrations/v3-to-v4.html)を確認してください。
実行可能な検証は[Conformance（英語）](/conformance/)、provider-neutral な契約は
[Host API ドキュメント](https://takoform.com/host-api/)にあります。

Host 設定と resource 宣言はガイドにあります。AWS resource と組み合わせる場合は
[native provider composition](/examples/native-provider-composition/main.tf) を参照してください。
各 Provider の source、認証情報、state は独立しています。
