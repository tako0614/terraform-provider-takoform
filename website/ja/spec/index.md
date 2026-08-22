# 仕様

## Identity

exact な **FormRef** は、API group・kind・definition version・schema digest の
組み合わせです。互換性は kind 名だけから推測しません — epoch が違えば別の契約です。
Form group は [Form Families](/spec/form-families.html) で namespace 化され、
Host API group はどの Form group からも独立した protocol 互換性 identity です。

| Surface | Identity |
| --- | --- |
| 現在の Form Family | `edge.forms.takoform.com/v1beta1` (Beta family; Experimental `0.1.0` 15種) |
| 現在の Host API wire | `forms.takoform.com/v1beta1` (discovery は `/.well-known/takoform/v1beta1`) |
| 現在の package envelope | `packages.forms.takoform.com/v1alpha4` |
| Provider distribution | `v2.1.1` Registry 公開済みの current stable distribution（descriptor は owner 公開後も `candidate-only` metadata）。`v2.0.0` と `v1.0.3` は撤回された epoch の不変の Registry 履歴 |

pre-Beta の epoch（`forms.takoform.com/v1alpha1` と `/v1alpha2`）は撤回されました
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html))。
identity は公開台帳に retired として記録され、バイト列はリポジトリ履歴に残ります。
provider の SemVer はどの API identity からも独立しています。

## 現在レーンの契約 (英語のみ)

- [Form Families](/spec/form-families.html) — namespace 化された Form group と
  Edge Platform Family
- [Host API v1beta1](/spec/host-api/v1beta1.html) — uid/generation/revision
  識別・long-running operation・fencing
- [Interface contracts](/spec/interface-contract/) — Form のサービスが公開する
  exact な capability 契約
- [Binding contracts](/spec/binding-contract/) — revision が保持する typed な
  capability 利用
- [Artifact transport](/spec/artifact-transport/) — content-addressed な
  artifact upload と manifest

## Normative schemas

`forms.takoform.com/schemas/...` で公開しています。現在の Beta identity:

- [form-ref v1beta1](/schemas/v1beta1/form-ref.schema.json)
- [form-definition v1beta1](/schemas/v1beta1/form-definition.schema.json)
- [host-api-wire v1beta1](/schemas/v1beta1/host-api-wire.schema.json)
- [package-index v1alpha4](/schemas/v1alpha4/package-index.schema.json)

撤回された epoch の schema identity は
[`release/public-schema-identities.json`](/release/public-schema-identities.json)
に retired として記録され、再利用されません。

## ライフサイクル

Form は Proposal → Experimental → Stable → Legacy の順に進みます。成熟度は、
独立した実装と証拠から獲得されるもので、公開された事実や人気から来るものでは
ありません。新しい Form は必ず先行事例 (OCCI、CIMI、TOSCA、Kubernetes/Crossplane、
provider ネイティブのリソース) から検討を始めます。

<StatusNote />
