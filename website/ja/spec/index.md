# 仕様

## Identity

exact な **FormRef** は、API group・kind・definition version・schema digest の
組み合わせです。互換性は kind 名だけから推測しません — epoch が違えば別の契約です。

| Surface | Identity |
| --- | --- |
| 現行 FormRef | `forms.takoform.com/v1alpha2` |
| Legacy FormRef | `forms.takoform.com/v1alpha1` |
| 現行 package envelope | `packages.forms.takoform.com/v1alpha3` |
| Provider | `v1.0.3` 公開済み · `v2.0.0` source candidate |

## Normative schemas

`forms.takoform.com/schemas/...` で公開しています:

- [form-ref v1alpha2](/schemas/v1alpha2/form-ref.schema.json)
- [form-definition v1alpha2](/schemas/v1alpha2/form-definition.schema.json)
- [package-index v1alpha3](/schemas/v1alpha3/package-index.schema.json)
- [host-api-wire v1alpha2](/schemas/v1alpha2/host-api-wire.schema.json)

## ライフサイクル

Form は Proposal → Experimental → Stable → Legacy の順に進みます。成熟度は、
独立した実装と証拠から獲得されるもので、公開された事実や人気から来るものでは
ありません。新しい Form は必ず先行事例 (OCCI、CIMI、TOSCA、Kubernetes/Crossplane、
provider ネイティブのリソース) から検討を始めます。

<div class="status-note">

Takoform は **Experimental specification project** です。現行の FormRef は
`forms.takoform.com/v1alpha2`、現行の package envelope は
`packages.forms.takoform.com/v1alpha3` です。provider `v1.0.3` は公開済みの
Legacy client、provider `v2.0.0` は未公開の source candidate です。
`forms.takoform.com/v1alpha1` の公開済み Form Package identity 34件は、不変の
Legacy 証跡です。現在、中央による承認や admission はありません。

</div>
