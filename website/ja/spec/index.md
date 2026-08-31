# 仕様の履歴資料

このページは Specification 1.0 / 1.1 receipt と撤回された互換性 evidence の
archive です。番号付き Specification は現在の version stream ではありません。
現行の normative contract は stable な Host API `forms.takoform.com/v1` と、
Form/Core contract です。

## Current identity（詳細は英語のみ）

| Surface | Identity |
| --- | --- |
| Current Form corpus | `edge.forms.takoform.com` 一つ、exact な Experimental `0.x` Form 16個（Interface 7個、Binding 6個） |
| Source roster | [`takoform-forms` commit `3a395e4`（英語のみ）](https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e) |
| Current Host API | `forms.takoform.com/v1`（discovery は `/.well-known/takoform/v1`、stable wire contract） |
| Form Package envelope | `packages.forms.takoform.com/v1alpha5`（wire/envelope format、product version 軸ではない） |
| Provider distribution | Provider 3.0.0 は Registry 公開済みだが 8 family / 31 resource projection は履歴。Edge16 official-only mapping は次 major candidate（未公開） |
| Historical receipt | Specification 1.0 / 1.1。archive evidence であり、API v1.1 / Form 1.0.0 / Provider release を mint しない |

## 現在の normative lane（英語のみ）

- [Host API v1（英語のみ）](/spec/host-api/v1.html)
- [Form Definition（英語のみ）](/spec/form-definition/)
- [Form Package（英語のみ）](/spec/form-package/)
- [Interface contracts（英語のみ）](/spec/interface-contract/)
- [Binding contracts（英語のみ）](/spec/binding-contract/)
- [Artifact transport（英語のみ）](/spec/artifact-transport/)
- [Core contracts（英語のみ）](/spec/core/)

Form は Proposal → Experimental → Stable → Legacy の順に、Specification receipt
とは独立して進みます。current Edge16 の詳細 inventory は
[バージョンと互換性](/ja/docs/versions.html) と [Form inventory（英語のみ）](/forms/)
を参照してください。

撤回された pre-Beta epoch の schema と release は、公開台帳および git history に
retired として保持されます。履歴を current として再利用しません。

<StatusNote />
