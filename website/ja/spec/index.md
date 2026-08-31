---
title: 仕様資料（履歴）
description: Specification 1.1 receipt と旧 contract を読むための履歴入口。
---

# 仕様資料（履歴）

この URL は、番号付き Specification の receipt と旧 contract を参照するために
保持しています。現行の version stream ではありません。現行の互換性は[バージョンモデル](/ja/docs/versions.html)、
現行の概念は[概念](/ja/docs/concepts.html)を参照してください。

## 保持される証拠

**Specification 1.1** は normative source の一回限りの immutable receipt です。
公開前に撤回された **Specification 1.0** identity とともに履歴として残り、
Host API、Form、Core、Provider の現行 version を決めません。`/v1.1` lane を
作るものでもありません。

## 旧 contract の入口（英語）

- [Form Families](/spec/form-families.html) — namespace 化された family group
- [Host API v1](/spec/host-api/v1.html) — UID / generation / revision と fencing
- [Interface contracts](/spec/interface-contract/) — capability の contract
- [Binding contracts](/spec/binding-contract/) — revision が保持する typed binding
- [Artifact transport](/spec/artifact-transport/) — content-addressed artifact

これらの projection は、当時の決定と receipt を調べるためのものです。新しい
Form や host capability をこのページから導入することはできません。

## schema identity

旧 epoch の schema identity は retired として保持され、再利用されません。

- [form-ref v1](/schemas/v1/form-ref.schema.json)
- [form-definition v1](/schemas/v1/form-definition.schema.json)
- [host-api-wire v1](/schemas/v1/host-api-wire.schema.json)
- [package-index v1alpha5](/schemas/v1alpha5/package-index.schema.json)

## Form の lifecycle（履歴上の説明）

Form は Proposal → Experimental → Stable → Legacy と進みますが、これは
Specification receipt から独立しています。Stable へ進む場合も Form ごとの
明示的な判断が必要です。

<StatusNote />
