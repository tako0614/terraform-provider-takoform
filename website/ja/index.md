---
layout: home

hero:
  name: Takoform Provider
  text: HCL から exact Form を管理する
  tagline: 互換 Takoform Host に対応する Terraform / OpenTofu の型付き resource
  actions:
    - theme: brand
      text: 現行 v2 source を評価する
      link: /docs/provider-host-api-v2-source.html
    - theme: alt
      text: 旧版 4.1.0 のガイド
      link: /ja/docs/getting-started.html
---

Terraform / OpenTofu の HCL を、選択した Host が公開する exact Form contract に対応させます。
他の Provider と同じ module に宣言し、HCL の参照で resource をつなげられます。

| 用途 | Provider | Host API | 対応する resource |
| --- | --- | --- | --- |
| 現行の source target / candidate | **5.0.0** | `forms.takoform.com/v2` | exact Edge Form URL 13 種 |
| 非推奨の Registry 公開版 | **4.1.0** | `forms.takoform.com/v1` | Edge Form 17 種 |

通常の source build で使う Provider は v2 で、v1 はサポートしません。Provider 5.0.0 は
Registry 公開済みとは確認されていません。pin を変える前に[バージョンと互換性](/ja/docs/versions.html)
と exact version の readback を確認してください。Provider 4 の state は Provider 5 へ自動変換されません。
接続先、Space、認証情報、サービス実装は Host が提供します。

[Provider 5 consumer reference（英語）](/docs/provider-host-api-v2-source.html)は
現行の source-v2 schema、private intent-store、lifecycle を説明します。
[Provider reference（英語）](/docs/reference.html)は非推奨 v4 resource を説明します。
Form の公開は publisher が行い、Host の対応は別に確認する必要があります。
provider-neutral な契約は[Host API ドキュメント](https://takoform.com/host-api/)を参照してください。
