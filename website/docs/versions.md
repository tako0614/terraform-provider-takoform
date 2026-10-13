# Versions and compatibility

A Provider version selects a client schema and its exact Form mappings. It
does not publish Forms or establish Host support. Form definition versions,
package digests, and Host API versions have their own owners and histories.

## Choose an installation or source target

| Provider | Status in this repository | Host API | Mappings | Reference |
| --- | --- | --- | --- | --- |
| **4.1.0** | Registry-published | `forms.takoform.com/v1` | 17 Edge Forms | [Getting started](/docs/getting-started.html) |
| **5.0.0** | Source target | `forms.takoform.com/v2` | 13 exact Edge Form URLs | [Provider 5](/docs/provider-host-api-v2-source.html) |

The [Provider release identity ledger](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/provider-release-identities.json)
records the signed tag, immutable GitHub Release, and Registry readback for
4.1.0. The [5.0.0 descriptor](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/candidates/provider-v5.0.0.json)
selects the source target; it is not evidence of Registry installation.
Check the [live Registry listing](https://registry.terraform.io/providers/tako0614/takoform/latest)
and exact-version readback before changing a consumer pin.

**Provider 4 state is not automatically translated to Provider 5.** Existing
v4 workspaces require a separately planned and verified migration. The default
source-built plugin speaks v2; it does not fall back to v1 on a v2 failure.

## Provider 4 identity model

Provider 4.1.0 selects the 17-Form Edge publication recorded at
[`takoform-forms` commit `3231633605b737ce5279d7fc020b4780568e7091`](https://github.com/tako0614/takoform-forms/commit/3231633605b737ce5279d7fc020b4780568e7091).
Its resource state records the exact four-field FormRef: group, kind,
`definitionVersion`, and `schemaDigest`. The separate package digest identifies
package bytes. The [Embedded Edge candidate snapshot](/forms/candidates/edge.forms.takoform.com/candidate-set.json)
retains `publicationStatus: unpublished` and is not publication evidence.
The linked Core v1.0.1 source is the compatibility checkpoint for this v1
reference, not a description of the v5 plugin.

Provider 5 instead records one exact publisher Form URL in `form_url` and uses
its own schema and private intent-store contract. Its thirteen mappings and
exact definition versions are listed in the Provider 5 reference.

## Published compatibility history

| Provider | Host API lane | Compatibility |
| --- | --- | --- |
| **4.1.0** | `forms.takoform.com/v1` | Published minor adding rotating token-file authentication and pending-operation custody; same Form identities as 4.0.0. |
| **4.0.0** | `forms.takoform.com/v1` | Publisher-selected 17-Form Edge release with static-token configuration. |
| **3.0.0** | `forms.takoform.com/v1` | Immutable 31-Form aggregate release across eight families. |
| **2.1.1** | `forms.takoform.com/v1beta1` | Fifteen immutable Edge Form identities in the [identity ledger](/release/provider-form-identities.json). |
| **2.0.0** | Withdrawn `v1alpha2` epoch | Exact-pin recovery and migration history. |
| **1.0.3** | Withdrawn `v1alpha1` epoch | Exact-pin recovery and migration history. |

Read the [v2-to-v3 migration boundary](/release/migrations/v2-to-v3.html)
for state from a withdrawn epoch and the
[v3-to-v4 publisher-set boundary](/release/migrations/v3-to-v4.html)
before upgrading Provider 3. These boundaries do not perform an automatic migration.

The immutable v4.1.0 tag contains pre-publication reference wording that
calls 4.1.0 a candidate and points to v4.0.0. The current release ledger records
the later Registry readback. Those tagged and Registry-rendered bytes remain
unchanged; corrected Registry documentation must ship in a new version.

See [Release evidence](/release/) and [Conformance](/conformance/) for the
scope of the retained receipts and executable checks.
