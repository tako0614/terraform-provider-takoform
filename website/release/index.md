# Release

## Published Provider history and Provider 4 candidate

Provider **v3.0.0** is the current Registry-published Terraform/OpenTofu
Provider. Its immutable release registers 31 typed resources across eight versionless families;
each resource keeps an exact FormRef and provider mapping in its release
identity projection. Provider release, Form Definition publication, package
publication, and Host deployment are separate owner actions.

Provider **v4.0.0** is the candidate at the same `tako0614/takoform` address and selects only the
16 exact Edge Forms selected from the `tako0614/takoform-forms` publication. It does not
claim that Provider 4 has been tagged or published. The 15 removed aggregate
types and explicit state paths are documented in [v3 to v4 publisher-set
migration](migrations/v3-to-v4.md). Ordinary AWS, Cloudflare, Kubernetes, and
other resources remain native resources of their own providers.

The unpublished candidate is described by
`release/candidates/provider-v4.0.0.json`, with its derived 17-Form mapping in
`release/candidates/provider-v4.0.0-form-identities.json`.
The retired Provider 3 writer input and identity ledger remain byte-stable
history; neither is reused to publish Provider 4.

The Provider 4 production binary embeds only that publisher closure: 16 Form
Packages, their seven exact Interface definitions, and six exact Binding
definitions. The complete Provider 3 artifact projection remains tracked as
source-side immutable history for goldens and migration verification; it is no
longer bundled into the current provider binary.

The signed release-tag commit must be an ancestor of the reviewed
protected-main/readback commit used for the release.

The release identity and Registry readback are recorded in the
[Provider release identity ledger](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/provider-release-identities.json).
The [Provider reference](../docs/) contains the install pin and generated
resource contracts.

## Compatibility and migration

### Retained v1beta1 history (Provider 2.1.1)

Provider 2.1.1 remains the compatibility predecessor. Its identity ledger
retains 15 exact v1beta1 Form identities; 14 remain readable by Provider 3
state/import dispatch. ObjectBucket has no Provider 3 resource or codec, so
ObjectBucket state needs explicit pre-upgrade handling. Existing Provider 2
state needs an explicit cutover; follow [v2 to v3 migration](migrations/v2-to-v3.md)
before changing the required provider version. Provider 3 does not rewrite
Provider 1 state; the [v1 to v2 migration](migrations/v1-to-v2.md) is the
earlier boundary.

The Provider 4 publisher mapping group is `edge.forms.takoform.com` and uses
`packages.forms.takoform.com/v1alpha5`. Retained Provider 2.1.1 history uses
Host API `forms.takoform.com/v1beta1`, family
`edge.forms.takoform.com/v1beta1`, and package envelope
`packages.forms.takoform.com/v1alpha4`.

The [versions page](https://github.com/tako0614/terraform-provider-takoform/blob/main/website/docs/versions.md)
lists the current API/Core checkpoint, Provider releases, package publication,
and retained history in one place.

## Core and Specification history

The current Core/API checkpoint is [Core v1.0.1](https://github.com/tako0614/takoform/tree/v1.0.1/spec)
on `forms.takoform.com/v1`. Specification 1.1 is retained as an immutable
historical source receipt.

Release tooling and ledgers are maintained in the
[Provider repository](https://github.com/tako0614/terraform-provider-takoform/tree/main/release).
They provide Provider provenance and migration evidence.
