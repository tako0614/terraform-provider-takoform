# Release

## Current Provider release

Provider **v3.0.0** is the current Registry-published Terraform/OpenTofu
Provider. It registers 31 typed resources across eight versionless families;
each resource keeps an exact FormRef and provider mapping in its release
identity projection. Provider release, Form Definition publication, package
publication, and Host deployment are separate owner actions.

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

The current mapping groups are rooted at `edge.forms.takoform.com` and use
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
