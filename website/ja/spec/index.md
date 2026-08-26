# 仕様

## Identity

exact な **FormRef** は、API group・kind・definition version・schema digest の
組み合わせです。互換性は kind 名だけから推測しません — epoch が違えば別の契約です。
Form group は [Form Families](/spec/form-families.html) で namespace 化され、
Host API group はどの Form group からも独立した protocol 互換性 identity です。

| Surface | Identity |
| --- | --- |
| Specification | Takoform 1.1 open candidate。first numbered release。authority は normative `spec/` tree の exact committed snapshot 1つだけ（1.0 は公開前に撤回され再利用しない） |
| 現在の Form corpus | 8 versionless families / 31 exact Experimental `0.x` Forms |
| Host API candidate | `forms.takoform.com/v1` (discovery は `/.well-known/takoform/v1`、Specification とは separate and unpublished) |
| 現在の package envelope | `packages.forms.takoform.com/v1alpha5` (unpublished) |
| Provider distribution | independent。Provider 3.0.0 は current Registry-published non-normative reference implementation、`v2.1.1` は retained history |

Specification 1.1 は separate Host API v1 candidate を publish / promote せず、
current Form を `1.0.0` に昇格させず、Package や Provider を publish せず、
`/v1.1` / v2 lane も mint しません。

pre-Beta の epoch（`forms.takoform.com/v1alpha1` と `/v1alpha2`）は撤回されました
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html))。
identity は公開台帳に retired として記録され、バイト列はリポジトリ履歴に残ります。
provider の SemVer はどの API identity からも独立しています。

## 現在レーンの契約 (英語のみ)

- [Form Families](/spec/form-families.html) — namespace 化された Form group と
  Edge Platform Family
- [Host API v1](/spec/host-api/v1.html) — uid/generation/revision
  識別・long-running operation・fencing
- [Interface contracts](/spec/interface-contract/) — Form のサービスが公開する
  exact な capability 契約
- [Binding contracts](/spec/binding-contract/) — revision が保持する typed な
  capability 利用
- [Artifact transport](/spec/artifact-transport/) — content-addressed な
  artifact upload と manifest

## Normative schemas

stable Host/API schema identity は append-only local contract lock に記録します:

- [form-ref v1](/schemas/v1/form-ref.schema.json)
- [form-definition v1](/schemas/v1/form-definition.schema.json)
- [host-api-wire v1](/schemas/v1/host-api-wire.schema.json)
- [package-index v1alpha5](/schemas/v1alpha5/package-index.schema.json)

撤回された epoch の schema identity は
[`release/public-schema-identities.json`](/release/public-schema-identities.json)
に retired として記録され、再利用されません。

## ライフサイクル

Form は Proposal → Experimental → Stable → Legacy の順に、Specification とは
独立して進みます。future stable Form は explicit per-Form decision により
`1.0.0` から始まります。

<StatusNote />
