---
title: 概念
description: Takoform の portability boundary、Form identity、lifecycle。
---

# 概念

Takoform は workload semantics を Form に、運用上の責任を host に寄せます。
Provider の surface を読むときは、次の 3 つの考え方を先に確認してください。

## portability には境界がある

Form は一つの service primitive の application-visible shape を所有します。
実行 ABI、整合性、配信保証、更新単位、lifecycle がその範囲です。実装、配置、
容量、資格情報、routing、recovery、catalog、billing、quota、SLA は host が
所有します。

これは、同じ shape を conforming host の間で移動できるという意味です。複数
service の最小公倍数に薄めるという意味ではありません。意味論が異なるなら、
別の Form です。

## Form は exact identity である

現行の 4 つの流れを明示します。

- Host API lane は literal な `forms.takoform.com/v1`。
- Form は `formId` と `definitionVersion` を持つ。
- Core ライブラリは独立した SemVer（現在 `v1.1.0`）を持つ。
- Provider は独立した Registry SemVer（現在 `3.0.0`）を持つ。

family は discovery のための grouping であり、versionless です。package
envelope、schema `$id`、digest、Provider descriptor は artifact evidence であり、
隠れた 5 つ目の流れではありません。

## resource は lifecycle graph を作る

多くの resource は次の順で進みます。

1. **identity** — service shape と stable UID に名前を付ける。
2. **revision** — immutable bytes と revision field を宣言または upload する。
3. **deployment** — traffic を受ける revision を選ぶ。
4. **attachment** — domain、endpoint、trigger、consumer などの inward edge を付ける。

Provider は UID、generation、revision の fence 付きで validate、prepare、apply、
observe、delete を送ります。deployment がない、generation が古い、といった状態は
contract の不一致であり、version を読み替える理由にはなりません。

## capability は host が所有する

apply 前に Provider は Host Support Profile を読み、exact FormRef が対応して
いるかを確認します。revision には typed binding で capability を追加します。
配置や資格情報が Form definition に入り込むことはありません。

## 次に読むページ

[バージョンモデル](/ja/docs/versions.html)が現行の互換性入口です。旧[仕様資料](/ja/spec/)
は履歴 source として保持され、旧 decision や receipt を調べるための banner が
表示されます。

<StatusNote />
