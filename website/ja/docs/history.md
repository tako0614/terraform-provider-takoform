---
title: 履歴
description: 保持された Specification receipt、撤回された epoch、不変の Provider release。
---

# 履歴

このページは履歴資料であり、別の version model ではありません。現行の利用者は
まず [4 つのバージョンモデル](/ja/docs/versions.html)を確認してください。

## Specification receipt — 履歴資料のみ

番号付きの **Specification 1.1** receipt は、normative evidence の不変の snapshot
を一度記録したものです。過去の判断を監査したり、当時の build を再現したりする
ために参照できますが、現行の release train ではありません。Form を昇格させず、
Host API `/v1.1` lane も作りません。

公開前の **Specification 1.0** identity は撤回されました。どちらの番号も現行の
利用者向け version stream ではありません。保持された[仕様資料](/ja/spec/)は
navigation でも分離し、旧 URL ごとに履歴であることを案内しています。

## Provider 配布の履歴

公開済み identity を書き換えず既存 state を recovery / migration できるよう、
Provider package は exact pin で利用できます。

| release | 履歴上の役割                                                  |
| ------- | ------------------------------------------------------------- |
| `3.0.0` | Form 31 個を mapping する、現在の Registry 公開済み release。 |
| `2.1.1` | 過去の Host API v1beta1 用 compatibility client。             |
| `2.0.0` | 撤回された v1alpha2 epoch 用の client。                       |
| `1.0.3` | 撤回された v1alpha1 epoch 用の Legacy client。                |

旧 package や resource identity を新しい Form に再割当てしません。既存 state を
所有する release を pin した上で、[v2 から v3 の移行境界](/release/migrations/v2-to-v3.html)
を参照してください。自動 migration はありません。

## 保持する URL

旧 URL は意図的に引き続き開けます。

- `/spec/` と配下の contract page は履歴 source tree を保持します。
- `/release/` は公開と migration の証拠を保持します。
- `/docs/reference.html` は生成された旧 reference projection のままです。

それぞれの URL には現行 version model へ戻る履歴 banner があります。現行 Provider
surface の入口は[リファレンスの入口](/ja/docs/reference.html)です。

<StatusNote />
