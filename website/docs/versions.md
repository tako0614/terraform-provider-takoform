# Versions and compatibility

Takoform has two domain version axes: API/Core release SemVer and each Form's
`definitionVersion`. Provider SemVer, package/schema IDs and digests, and
withdrawn lanes identify artifacts or history; they do not add a domain axis.

## Current identities

| Surface                          | Identity                                                                                                                                                            | Use                                                                                                        |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| API/Core                         | **`v1.0.1`**                                                                                                                                                        | Current compatibility checkpoint on `forms.takoform.com/v1`; compatible 1.x checkpoints stay on that lane. |
| External Edge publication        | [`takoform-forms` at commit `3231633605b737ce5279d7fc020b4780568e7091`](https://github.com/tako0614/takoform-forms/commit/3231633605b737ce5279d7fc020b4780568e7091) | Publication authority for 17 content-addressed packages.                                                   |
| Embedded Edge candidate snapshot | [`forms/candidates/edge.forms.takoform.com/candidate-set.json`](/forms/candidates/edge.forms.takoform.com/candidate-set.json)                                       | Provider-repository snapshot with `publicationStatus: unpublished`; not publication evidence.              |
| Provider                         | **`4.0.0`**                                                                                                                                                         | Registry-published at the `tako0614/takoform` source address, with only the 17 tako0614 Edge mappings.     |
| Form Package envelope            | `packages.forms.takoform.com/v1alpha5`                                                                                                                              | Package identity; publication is independent of Provider release.                                          |

An exact Form is identified by its family, kind, `definitionVersion`, and
`schemaDigest`. Generated resource pages link the corresponding Form Definition
and retain this identity in state; a Provider resource name is only adapter
metadata.

## Provider compatibility history

| Provider   | Host/API lane                | Compatibility                                                                                                           |
| ---------- | ---------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| **4.0.0**  | Current Host API             | Current Registry-published publisher-set 17-Form Edge mapping.                                                          |
| **3.0.0**  | Current Host API             | Immutable retained 31-Form aggregate Registry distribution.                                                             |
| **2.1.1**  | `forms.takoform.com/v1beta1` | Retained client for 15 immutable Edge Form identities in the [identity ledger](/release/provider-form-identities.json). |
| **2.0.0**  | Withdrawn `v1alpha2` epoch   | Immutable Registry history; exact-pin recovery and migration only.                                                      |
| **1.0.3**  | Withdrawn `v1alpha1` epoch   | Immutable Registry history; exact-pin recovery and migration only.                                                      |

Provider releases are independent of Form definition and package publication.
For state created by a withdrawn Provider epoch, follow the [v2-to-v3 migration
boundary](/release/migrations/v2-to-v3.html). For the publisher-set cut, follow
the [v3-to-v4 boundary](/release/migrations/v3-to-v4.html). No automatic
migration occurs.

## Related references

- [Provider resource reference](/docs/)
- [Provider mapping inventory](/forms/)
- [Specification and release evidence](/release/)
- [Conformance evidence](/conformance/)
