---
title: バージョンモデル
description: Takoform の互換性境界を定める 4 つの独立したバージョンの流れ。
---

# バージョンモデル

Takoform の現行モデルには、4 つのバージョンの流れだけがあります。前半の
2 つは domain の互換性、後半の 2 つは独立した software release です。

| 流れ            | 現在の形                       | 所有範囲と意味                                                                                                  |
| --------------- | ------------------------------ | --------------------------------------------------------------------------------------------------------------- |
| Host API        | `forms.takoform.com/v1`        | discovery と operation の literal lane。互換性のある変更はこの lane に収まり、`/v1.1` route はありません。      |
| Form            | 各 Form の `definitionVersion` | 正確な service shape と lifecycle contract。family は versionless で、各 Form が自身の定義 version を持ちます。 |
| Core ライブラリ | `v1.1.0`                       | 独立して release される Core module / library。SemVer は Host API や Provider の version を決めません。         |
| Provider        | `3.0.0`                        | この repository の Registry 公開済み typed mapping。SemVer は Form や host capability を公開しません。          |

## バージョンの流れではないもの

Specification 1.1 receipt は normative evidence の一回限りの履歴 snapshot です。
現行の release train ではなく、Host API lane も作りません。撤回された
Specification 1.0 identity は履歴資料としてだけ残ります。

Form Package の envelope、schema の `$id`、content digest、record format、family
label、Provider descriptor は artifact または公開状態の識別子です。5 つ目の
流れを追加するものではありません。

## Form identity の読み方

host が公開する Form は package や Provider の label ではなく、完全な FormRef
として読みます。`formId` が service shape を選び、`definitionVersion` がその
Form の contract revision を選びます。受け取った bytes は content digest で
固定し、family label は関連 Form の discovery 用に使います（family 自体は
versionless です）。

Provider は対応する FormRef の typed surface を compile します。未知の Form 用
generic carrier はなく、Provider release が Form definition を暗黙に更新する
こともありません。

## Provider release の履歴

次の値は不変の配布記録であり、現行の追加 stream ではありません。

| Provider release | 履歴上の役割                                                              |
| ---------------- | ------------------------------------------------------------------------- |
| `3.0.0`          | 現在の Experimental Form 31 個を mapping する Registry 公開済み release。 |
| `2.1.1`          | 過去の Host API v1beta1 lane 用に保持する compatibility client。          |
| `2.0.0`          | 撤回された v1alpha2 epoch 用に保持する client。                           |
| `1.0.3`          | 撤回された v1alpha1 epoch 用に保持する Legacy client。                    |

撤回された epoch に依存する state は、対応する Provider release を exact pin の
まま維持します。移行する場合は [v2 から v3 の移行境界](/release/migrations/v2-to-v3.html)
を参照してください。自動 migration はありません。

## 実際の確認順

1. host の literal Host Support Profile と `forms.takoform.com/v1` endpoint を確認します。
2. field を書く前に Form の `formId` と `definitionVersion` を確認します。
3. host integration が使う Core ライブラリの SemVer を固定します。
4. Terraform / OpenTofu の Provider SemVer を固定し、plan を確認します。

[リファレンスの入口](/ja/docs/reference.html)から現行 resource を、[履歴](/ja/docs/history.html)
から Specification evidence と旧 URL の扱いを確認できます。

<StatusNote />
