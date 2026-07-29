# 0001 — Provider v1 keeps Form versions independent

- Status: accepted
- Date: 2026-07-29
- Owners: Takoform maintainers
- Subsequent decision: [0002](0002-artifact-urls-are-credential-free-state.md)
  advances the current artifact-backed Form identities without changing this
  version-independence decision.

## Context

The active provider successor removes `takoform_http_service` and introduces
`takoform_edge_worker`. This is a breaking provider resource transition, so it
is the appropriate boundary for the first stable provider major.

The exact transition audit also found that all 33 Form kinds common to provider
v0.2.1 and the v1 candidate changed their exact FormRef/package identity. A
direct refresh through v1 could otherwise query a new exact identity, receive
`404` for an old Resource, and remove its state.

The published `v0.2.x` provider also exposed a writable
`takoform_interface` resource. That write path duplicates host authority:
portable projects may declare Interface descriptors inside Form Definitions
and read the resulting projection, while Interface records, bindings,
authorization, write fencing, and lifecycle belong to the host.

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
remain independent. The provider-neutral Form introduced by this decision was
`EdgeWorker@2.0.0`; provider `v1.0.0` did not reset it to `1.0.0`.
Decision 0002 later advances the current artifact-backed identity to
`EdgeWorker@3.0.0` without changing this rule. `ga-core-v2` also remains an
independent admission-generation identifier.

The portable API group remains `forms.takoform.com/v1alpha1`. Provider v1
therefore means a stable Terraform/OpenTofu provider surface, not a stable
portable specification or completed admission.

## Consequences

- Existing `takoform_http_service` state remains pinned to provider `v0.2.1`
  until an operator performs an explicit create, cutover, and destroy
  migration.
- All other v0.2.1 Form resource state remains pinned too. Provider v1 starts
  resource schema version `1`, records the exact FormRef/package identity in
  computed state, and rejects version `0` through a diagnostic-only handler
  that returns no transformed state and makes no Resource lifecycle request.
- Provider `v1.0.0` exposes `takoform_edge_worker`. This decision introduced
  it for `EdgeWorker@2.0.0`; decision 0002 advances the current exact Form
  identity to `EdgeWorker@3.0.0`.
- Provider `v1.0.0` removes the writable `takoform_interface` resource and
  retains only the read-only data source. Existing resource state must be
  reconciled under `v0.2.1` before upgrade; it is not silently reinterpreted.
- Published Form versions and bytes are never overwritten to make version
  numbers look aligned.
- Future breaking provider resource or state changes require a new provider
  major even if the affected Form has a different major.
- An old OpenTofu provider address is normalized only through an explicit
  `state replace-provider`; address replacement never migrates a Form identity.

## Rejected alternatives

- **Republish `EdgeWorker@1.0.0`.** Rejected because it would assign different
  bytes to an immutable public identity.
- **Make every Form and admission generation `1.0.0`.** Rejected because those
  identities change for different reasons and coordinated numbering would
  create false compatibility claims.
- **Keep the breaking provider transition on `v0.3.0`.** Rejected because the
  transition is the clean boundary for declaring and enforcing the provider
  v1 compatibility contract.
