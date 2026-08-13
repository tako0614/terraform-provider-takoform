# 0004 — Recorded Form identity changes only through an explicit proved transition

- Status: accepted
- Date: 2026-08-13
- Owners: Takoform maintainers

## Context

Provider v1 state records an exact installed Form identity: API version, kind,
definition version, schema digest, and package digest. Provider `v1.0.2` wrote
`RelationalDatabase@2.0.0` and `EdgeWorker@3.0.0`; provider `v1.0.3` creates
their newer `RelationalDatabase@3.0.0` and `EdgeWorker@4.0.0` identities.

Selecting a codec from the current kind or a SemVer relationship would make an
ordinary refresh or artifact update reinterpret already-managed state. A
Terraform state-only upgrader would be worse: it could claim that a durable
database transition completed without any same-resource host commit.

The provider can safely support one product-declared database transition only
when the host owns an atomic operation that binds the exact old and new Form
references, desired specification, resource revision, native identity, and a
readable commit proof. An HTTP retry alone is not sufficient because a lost
acknowledgement leaves the mutation outcome unknown.

## Decision

The maintained provider-v1 line retains byte-exact registry entries and closed
codecs for `RelationalDatabase@2.0.0` and `EdgeWorker@3.0.0`, alongside the
current `RelationalDatabase@3.0.0` and `EdgeWorker@4.0.0` entries. Create and
import select the declared current exact Form. Read, Update, and Delete select
only the complete Form identity recorded in resource state. Missing, unknown,
or only approximately matching identities fail before a Resource host call;
kind and SemVer never infer a codec.

Only `takoform_relational_database` accepts the closed optional declaration:

```hcl
form_transition = "relational-database-v2-to-v3"
```

That marker requests only exact DB2-to-DB3 same-resource transition. It is a
no-op for a fresh DB3 create or state already proved as DB3. No marker means an
ordinary update under the state-recorded DB2 identity. EdgeWorker has no
transition marker: updating an artifact in recorded Edge3 state remains an
Edge3 update and cannot select Edge4 fields or identity.

The provider and host share RFC 8785/SHA-256 evidence for the exact Form pair,
desired spec, logical Resource identity, operation, and request. Every provider
invocation first performs a read-only GET for the deterministic operation ID.
The exact authenticated `form_transition_operation_not_found` response permits
one POST. So does an exact same-operation/same-request `prepared` response only
when it explicitly proves `dispatchAttempted: false`; host compare-and-swap
then allows exactly one caller to resume dispatch. Prepared state with an
attempted or missing flag, indeterminate state, a digest mismatch, or unknown
GET outcome permits no POST. A lost POST acknowledgement permits one GET and
never another POST in that invocation.

State remains bound to DB2 unless a committed readback returns the same
operation and request digest, exact from/to references and evidence digest,
exact desired-spec digest, unchanged logical/native identity, and one
resource-version-consistent DB3 Resource. A definitive failed operation is
terminal for that exact request and revision. The provider sends no runtime
credential audience or scopes; caller authority remains host-owned.

The implementation is maintained as provider `v1.0.4`, rooted at immutable
provider `v1.0.3`. Current provider-v2 development has separate types and
lifecycle contracts. This v1 work is not automatically backmerged or mixed
into that line; any later implementation must be reviewed against its owning
types.

## Consequences

- Serialized provider `v1.0.2` DB2 and Edge3 state remains readable,
  updateable, and deletable through its exact historical codec.
- The provider embeds and digest-checks the byte-exact historical DB2 and Edge3
  Form Definitions; Edge3 is not reconstructed from Edge4 by field subtraction.
- Adding schema-bundle fields to DB2 without the marker fails locally because
  the closed DB2 codec does not contain them.
- A DB2-to-DB3 transition applies the desired DB3 spec and identity atomically;
  there is no state-only upgrade path.
- Transport uncertainty is an explicit reconcile condition, not permission to
  issue an unproved replacement mutation.
- Hosts that do not advertise `resource_form_transition` cannot perform the
  transition, while ordinary exact-identity lifecycle remains available.

## Rejected alternatives

- **Use the newest Form for every Update.** Rejected because it silently
  changes persisted identity and accepts fields the recorded Form never had.
- **Infer compatibility from SemVer.** Rejected because version order is not
  proof of schema bytes, package bytes, or a durable same-resource operation.
- **Upgrade only Terraform state.** Rejected because state cannot prove a host
  or backend transition.
- **Retry POST after a timeout.** Rejected because an ambiguous outcome must be
  reconciled by readback, even when the host also has an idempotency defense.
- **Generalize the marker to other Form pairs.** Rejected because only the
  exact DB2-to-DB3 product transition and proof contract are compiled here.
