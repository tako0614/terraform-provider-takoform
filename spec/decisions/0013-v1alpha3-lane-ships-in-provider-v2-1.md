# 0013 — The v1alpha3 lane ships in provider v2.1

- Status: accepted
- Date: 2026-08-06
- Owners: Takoform maintainers

## Context

Provider v2.0.0 is published and Registry-verified against Host API
`forms.takoform.com/v1alpha2` and the nine candidate Forms. Decisions
[0008](0008-forms-preserve-service-shape.md) through
[0012](0012-artifacts-use-content-addressed-upload.md) change the Form model,
the wire envelope, connections, and artifact transport incompatibly **at the
API-lane level**. Editing the published v1alpha2 lane in place would break
provider-v2 reproducibility and the immutability promises of
`forms/candidates/v1alpha2` and the published schemas.

At the provider level, however, the new lane is purely additive: every
existing v2.0.0 resource type, configuration, and persisted state remains
valid and untouched, and the new family resources are new types. Under
[`versioning.md`](../versioning.md), compatible new resource types and
host-client capabilities stay within the current provider major.

## Decision

The redesign ships as a new Host API lane carried by a provider **minor**;
the published API lanes freeze.

- Host API: `forms.takoform.com/v1alpha3`, discovered at
  `/.well-known/takoform/v1alpha3` with API base
  `/apis/forms.takoform.com/v1alpha3`. v1alpha1 and v1alpha2 discovery and
  wire behavior are retained unchanged. Host API group identity and provider
  SemVer remain independent axes.
- Long-running operations: create, update, delete, import, and artifact
  commit may return an Operation resource polled at `/operations/{id}` with
  `Retry-After`, exponential backoff with full jitter, an overall deadline,
  and resumable operation IDs. Fixed-interval busy polling is removed.
- A closed portable error taxonomy is extended (`rate_limited`,
  `deadline_exceeded`, `operation_cancelled`, `operation_not_found`,
  `dependency_in_use`, `deletion_protected`, `artifact_missing`,
  `artifact_invalid`, `unsupported_capability`, `migration_required`,
  `uid_mismatch`, `revision_conflict`, `generation_conflict`).
- Host Support Profiles are readable API surfaces (`/support/forms`,
  `/support/interfaces/...`, `/support/bindings/...`) declaring supported
  exact refs, closed capability subsets, and limits. Price, SKU, region, and
  quota never appear there.
- Provider **v2.1.0** is the next release line: it keeps the nine retained
  v1alpha2 resource types byte-compatible, and adds the v1alpha3 client,
  typed per-Form family resources over a shared lifecycle core, a
  multi-FormRef registry dispatching on the exact FormRef recorded in state,
  family state identity `space/apiVersion/kind/uid`, a JSON-serialized
  `status.outputs` document (`outputs_json`),
  per-resource timeouts, artifact upload, and no global mutation mutex. A
  generic `takoform_resource` carries third-party family Forms. Existing
  v2.0.0 state is never reinterpreted; family resources use new state.
- `release/version.json` continues to describe the latest assigned release
  (v2.0.0) until the owner's release flow assigns v2.1.0 with signed
  Registry-readback evidence; a repository worktree never claims an
  unpublished version as released.
- Removing the retained v1alpha2 resource types remains a future provider
  major and a separate decision.
- The v1alpha2 candidates, registry, and generator are retained read-only
  for reproducibility; they are labelled retained provider-v2 preview source
  and are not part of the new design.

## Consequences

- New public schema identities: form-ref/form-definition/host-discovery/
  host-api-wire v1alpha3, package-index v1alpha4, operation v1alpha1, and
  host-support-profile v1alpha1, appended without touching published `$id`
  bytes.
- The per-package Interface distribution ordered by this redesign is
  deferred: Interface candidates ship as digest-bound definition documents
  under `interfaces/candidates/v1alpha1`, and no Interface Package envelope
  identity is published or specified yet
  ([`../interface-contract/`](../interface-contract/README.md)).
- The provider is one client among several: the CLI, SDKs, and controllers
  consume the same Host API.
- Conformance splits per API lane; v1alpha2 checks keep running against
  retained sources.
- One provider binary serves both lanes; discovery negotiation is per lane,
  and each resource type requires its own lane.

## Rejected alternatives

- **Evolve the v1alpha2 wire in place with optional fields.** Rejected
  because the envelope, connection, and artifact changes are semantic, not
  additive; optionality would fork behavior inside one identity.
- **A new provider major ("v3.0.0") for the new lane.** Rejected because the
  addition breaks no existing configuration or state; spending the major now
  would misuse SemVer and force an unnecessary migration signal on users.
- **Skip the new lane and wait for Stable Forms.** Rejected because the
  category-shaped candidates cannot earn stability; the shape-preserving
  redesign is the prerequisite for any maturity progress.
