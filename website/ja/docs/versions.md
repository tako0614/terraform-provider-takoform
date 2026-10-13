# バージョンと互換性

Provider の版は client schema と exact Form の対応表を選びます。
Form の公開や Host の対応を保証するものではありません。Form definition の版、
package digest、Host API の版はそれぞれ独立した owner と履歴を持ちます。

## インストールする版と評価するソース

| Provider | この repository での状態 | Host API | 対応表 | 参照先 |
| --- | --- | --- | --- | --- |
| **4.1.0** | Registry 公開済み | `forms.takoform.com/v1` | Edge Form 17 種 | [入門ガイド](/ja/docs/getting-started.html) |
| **5.0.0** | ソースの target | `forms.takoform.com/v2` | exact Edge Form URL 13 種 | [Provider 5（英語）](/docs/provider-host-api-v2-source.html) |

4.1.0 の署名済み tag、GitHub Release、Registry readback は
[Provider release identity ledger](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/provider-release-identities.json)
にあります。[5.0.0 descriptor](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/candidates/provider-v5.0.0.json)
はソースの target を指定するもので、Registry からのインストールを証明しません。
pin を変える前に[Registry の公開一覧](https://registry.terraform.io/providers/tako0614/takoform/latest)と
exact version の readback を確認してください。

**Provider 4 の state は Provider 5 へ自動変換されません。** 既存 v4 workspace の
移行は別に計画し、検証する必要があります。ソースから通常 build した plugin は v2 を使い、
v2 での失敗時に v1 へ切り替えません。

## Provider 4 の identity

4.1.0 の Edge Form 17 種の公開正本は
[`takoform-forms` commit `3231633605b737ce5279d7fc020b4780568e7091`](https://github.com/tako0614/takoform-forms/commit/3231633605b737ce5279d7fc020b4780568e7091)
です。resource state は group、kind、`definitionVersion`、`schemaDigest` の
4 フィールドからなる exact FormRef を記録し、package digest は別に package bytes を識別します。
[embedded candidate snapshot](/forms/candidates/edge.forms.takoform.com/candidate-set.json)の
`publicationStatus: unpublished` は外部での公開 evidence ではありません。
Core v1.0.1 への参照はこの v1 reference の互換性 checkpoint です。

Provider 5 は `form_url` に exact publisher Form URL を記録し、独自の schema と
private intent-store 契約を使います。13 種の対応表と exact definition version は
Provider 5 reference にあります。

## 公開済み版の互換性履歴

| Provider | Host API lane | 互換性 |
| --- | --- | --- |
| **4.1.0** | `forms.takoform.com/v1` | token file と pending-operation custody に対応。Form identity は 4.0.0 と同一。 |
| **4.0.0** | `forms.takoform.com/v1` | publisher set の Edge Form 17 種。静的 token に対応。 |
| **3.0.0** | `forms.takoform.com/v1` | 8 family / 31 Form の不変の aggregate release。 |
| **2.1.1** | `forms.takoform.com/v1beta1` | [identity ledger](/release/provider-form-identities.json)にある 15 種の不変の Edge Form。 |
| **2.0.0** | withdrawn `v1alpha2` epoch | exact-pin recovery と移行の履歴。 |
| **1.0.3** | withdrawn `v1alpha1` epoch | exact-pin recovery と移行の履歴。 |

withdrawn epoch の state は[v2-to-v3 migration（英語）](/release/migrations/v2-to-v3.html)、
Provider 3 からの更新は[v3-to-v4 publisher-set boundary（英語）](/release/migrations/v3-to-v4.html)
を確認してください。自動 migration はありません。

不変の v4.1.0 tag には公開前の文言が残り、4.1.0 を candidate と呼んで v4.0.0 を案内しています。
現行 ledger はその後の Registry readback を記録します。tag と Registry に表示される tag 由来の
文書を上書きせず、修正された Registry 文書は新しい版で提供します。

公開 receipt と実行可能な検証の範囲は[Release evidence（英語）](/release/)と
[Conformance（英語）](/conformance/)を参照してください。
