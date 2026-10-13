---
layout: home

hero:
  name: Takoform Provider
  text: HCL から exact Form を管理する
  tagline: 互換 Takoform Host に対応する Terraform / OpenTofu の型付き resource
  actions:
    - theme: brand
      text: 4.1.0 の入門ガイド
      link: /ja/docs/getting-started.html
    - theme: alt
      text: Provider 5 を評価する（英語）
      link: /docs/provider-host-api-v2-source.html
---

Terraform / OpenTofu の HCL を、選択した Host が公開する exact Form contract に対応させます。
他の Provider と同じ module に宣言し、HCL の参照で resource をつなげられます。

| 用途 | Provider | Host API | 対応する resource |
| --- | --- | --- | --- |
| Registry からインストール | **4.1.0** | `forms.takoform.com/v1` | Edge Form 17 種 |
| ソースから評価 | **5.0.0** | `forms.takoform.com/v2` | exact Edge Form URL 13 種 |

接続先、Space、認証情報、サービス実装は Host が提供します。
[Random + Takoform の入門ガイド](/ja/docs/getting-started.html)から始めてください。
Random はクラウド認証情報を必要としません。Provider の pin を変える前には
[バージョンと互換性](/ja/docs/versions.html)を確認してください。
Provider 4 の state は Provider 5 へ自動変換されません。

[Provider reference（英語）](/docs/reference.html)は公開済み v4 resource、
[Provider 5 consumer reference（英語）](/docs/provider-host-api-v2-source.html)は
ソースの v5 schema、private intent-store、lifecycle を説明します。
Form の公開は publisher が行い、Host の対応は別に確認する必要があります。
provider-neutral な契約は[Host API ドキュメント](https://takoform.com/host-api/)を参照してください。
