# 0001 — Provider v1 keeps Form versions independent

- Status: accepted
- Date: 2026-07-29
- Owners: Takoform maintainers

## Context

The active provider successor removes `takoform_http_service` and introduces
`takoform_edge_worker`. This is a breaking provider resource transition, so it
is the appropriate boundary for the first stable provider major.

Takoform also has multiple public version streams. Provider versions identify
provider binaries. A Form definition version is part of an exact `FormRef`;
its canonical bytes are bound by `schemaDigest`. Form Package versions and
admission generations have separate identities and lifecycles.

Public Form Package releases already occupy `EdgeWorker@1.0.0` and
`EdgeWorker@1.0.1`. The current provider-neutral EdgeWorker definition has
different bytes and schema.

## Decision

The successor provider candidate is `v1.0.0`. Provider compatibility follows
SemVer: breaking changes to an existing `v1.x` resource type or persisted
state require provider `v2`.

Form definition versions, Form Package versions, and admission generations
remain independent. The active provider-neutral Form stays
`EdgeWorker@2.0.0`; provider `v1.0.0` does not reset it to `1.0.0`.
`ga-core-v2` also remains an independent admission-generation identifier.

The portable API group remains `forms.takoform.com/v1alpha1`. Provider v1
therefore means a stable Terraform/OpenTofu provider surface, not a stable
portable specification or completed admission.

## Consequences

- Existing `takoform_http_service` state remains pinned to provider `v0.2.1`
  until an operator performs an explicit create, cutover, and destroy
  migration.
- Provider `v1.0.0` exposes `takoform_edge_worker` for
  `EdgeWorker@2.0.0`.
- Published Form versions and bytes are never overwritten to make version
  numbers look aligned.
- Future breaking provider resource or state changes require a new provider
  major even if the affected Form has a different major.

## Rejected alternatives

- **Republish `EdgeWorker@1.0.0`.** Rejected because it would assign different
  bytes to an immutable public identity.
- **Make every Form and admission generation `1.0.0`.** Rejected because those
  identities change for different reasons and coordinated numbering would
  create false compatibility claims.
- **Keep the breaking provider transition on `v0.3.0`.** Rejected because the
  transition is the clean boundary for declaring and enforcing the provider
  v1 compatibility contract.
