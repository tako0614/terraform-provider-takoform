# バージョンと互換性

Takoform の domain version axes は API/Core release の SemVer と各 Form の
`definitionVersion` の二つです。Provider SemVer、package/schema の ID・digest、
withdrawn lane は artifact / history identity であり、domain axis を増やしません。

## Current identity

| surface | identity | 用途 |
| --- | --- | --- |
| API/Core | **`v1.0.1`** | `forms.takoform.com/v1` 上の current compatibility checkpoint。互換性のある 1.x は同じ lane に留まります。 |
| external Edge publication | [`takoform-forms` commit `026f862975b9adb0e2bfd9c6214a5e6691dfb596`](https://github.com/tako0614/takoform-forms/commit/026f862975b9adb0e2bfd9c6214a5e6691dfb596) | 16 content-addressed package の公開正本。 |
| embedded Edge candidate snapshot | [`forms/candidates/edge.forms.takoform.com/candidate-set.json`](/forms/candidates/edge.forms.takoform.com/candidate-set.json) | Provider repository 内の `publicationStatus: unpublished` snapshot。公開 evidence ではありません。 |
| Provider | **`3.0.0`** | 8 versionless family / 31 mapping を持つ Registry 公開済み typed implementation。 |
| Form Package envelope | `packages.forms.takoform.com/v1alpha5` | package identity。公開は Provider release と独立します。 |

Exact Form は family、kind、`definitionVersion`、`schemaDigest` で識別します。
generated resource page は対応する Form Definition に link し、この identity を
state に保持します。Provider resource 名は adapter metadata です。

## Provider compatibility history

| Provider | Host/API lane | 互換性 |
| --- | --- | --- |
| **3.0.0** | current Host API | 8 family / 31 typed mapping。current Registry distribution。 |
| **2.1.1** | `forms.takoform.com/v1beta1` | [identity ledger](/release/provider-form-identities.json) にある 15 immutable Edge Form identity 用の retained client。 |
| **2.0.0** | withdrawn `v1alpha2` epoch | 不変の Registry history。exact-pin recovery と migration のみ。 |
| **1.0.3** | withdrawn `v1alpha1` epoch | 不変の Registry history。exact-pin recovery と migration のみ。 |

Provider release は Form definition / package publication と独立しています。
withdrawn Provider epoch で作成した state は [v2-to-v3 migration boundary](/release/migrations/v2-to-v3.html)
を確認してください。自動 migration はありません。

## Related reference

- [Provider resource reference](/ja/docs/)
- [Provider mapping inventory](/forms/)
- [Specification and release evidence](/release/)
- [Conformance evidence](/conformance/)
