# Release

## Published Provider and retained Provider history

Provider **v4.0.0** is the current Registry-published Terraform/OpenTofu
Provider at the `tako0614/takoform` address. It selects only the
17 exact Edge Forms selected from the `tako0614/takoform-forms` set tag
`forms/sets/e7f8a39311dd011b8467e97e7f300cabb9a6b06c` — the 16 Edge Forms
carried over from Provider 3 plus `ObjectBucket`. Its signed tag, immutable
GitHub Release, and Registry readback are the `4.0.0` entry of the identity
ledger below. The 15 removed aggregate
types and explicit state paths are documented in [v3 to v4 publisher-set
migration](migrations/v3-to-v4.md). Ordinary AWS, Cloudflare, Kubernetes, and
other resources remain native resources of their own providers. Why the
publisher set is the released roster is recorded in
[control ADR 0020](https://github.com/tako0614/takos-control/blob/main/docs/platform/decisions/0020-provider-4-is-the-publisher-set-release.md),
which also states why this repository's own decision series cannot take that
entry while the Specification 1.1 receipt seals `spec/`.

Provider **v3.0.0** is retained Registry history at the same address. Its
immutable release registers 31 typed resources across eight versionless families;
each resource keeps an exact FormRef and provider mapping in its release
identity projection. Provider release, Form Definition publication, package
publication, and Host deployment are separate owner actions.

`release/version.json` is the Provider `4.0.0` release descriptor every release
entrypoint reads; it keeps `publicationStatus: candidate-only` by the standing
descriptor convention and is not live publication state. The Registry readback
recorded in the identity ledger is the availability authority.
`release/candidates/provider-v4.0.0.json` is the retained candidate record and
stays byte-identical to it, with its derived 17-Form mapping in
`release/candidates/provider-v4.0.0-form-identities.json`.
`release/history/provider-v3.0.0.json` is the retained Provider 3 writer input.
It and the immutable Provider 3 identity ledger entry remain byte-stable
history; neither is reused to publish Provider 4.

The Provider 4 production binary embeds only that publisher closure: 17 Form
Packages, their eight exact Interface definitions, and seven exact Binding
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

### Provider 3 to Provider 4

Provider 3.0.0 is the compatibility predecessor of Provider 4 and remains
installable. The exact removal and state boundary is [v3 to v4 publisher-set
migration](migrations/v3-to-v4.md); a consumer may instead stay pinned to
`= 3.0.0` as retained history.

### Retained v1beta1 history (Provider 2.1.1)

Provider 2.1.1 remains the retained v1beta1 compatibility predecessor of
Provider 3. Its identity ledger retains 15 exact v1beta1 Form identities; 14
remain readable by Provider 3 state/import dispatch. ObjectBucket has no
Provider 3 resource or codec, so ObjectBucket state carried across the Provider
2 to Provider 3 boundary needs explicit pre-upgrade handling; the Provider 4
publisher set does select `ObjectBucket`. Existing Provider 2 state needs an
explicit cutover; follow [v2 to v3 migration](migrations/v2-to-v3.md) before
changing the required provider version. Provider 3 does not rewrite Provider 1
state; the [v1 to v2 migration](migrations/v1-to-v2.md) is the earlier
boundary.

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
