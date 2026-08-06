# Host API v1alpha3

`forms.takoform.com/v1alpha3` is the current Host API lane
([decision 0013](../decisions/0013-v1alpha3-lane-ships-in-provider-v2-1.md)).
It carries namespaced FormRef groups, UID/generation/revision resource
identity, long-running Operations, content-addressed artifact upload, and
Host Support Profiles. The retained v1alpha2 contract in
[`README.md`](README.md) continues to govern the frozen provider-v2 lane;
rules stated there and not changed here (space identity, I-JSON raw-first
decoding, idempotency-key namespacing, replay fingerprinting, same-origin
endpoint negotiation) apply to this lane unchanged.

The wire schema is
[`../schemas/host-api-wire-v1alpha3.schema.json`](../schemas/host-api-wire-v1alpha3.schema.json);
the machine operation table is [`operations-v1alpha3.json`](operations-v1alpha3.json).

## Discovery

`GET /.well-known/takoform/v1alpha3` returns a document validating against
[`../schemas/host-discovery-v1alpha3.schema.json`](../schemas/host-discovery-v1alpha3.schema.json):
`api_versions` is exactly `["forms.takoform.com/v1alpha3"]`; the features
`service_forms`, `exact_form_ref`, `optimistic_concurrency`,
`idempotent_lifecycle`, `operations`, `artifact_upload`, and
`support_profiles` are all required and true; `endpoints.api` is same-origin
with path `/apis/forms.takoform.com/v1alpha3`. Each lane has its own
discovery path; a v1alpha2 client can never select this lane accidentally.

## Resource identity

- `metadata.name` is the only name; Form desired schemas do not carry a
  `name` field.
- `metadata.uid` is host-issued and immutable. Delete followed by re-create
  of the same name yields a new UID. A client MAY fence any mutation with
  `expectedUid`; mismatch fails `uid_mismatch` (409).
- `metadata.generation` increments only when the portable desired spec
  changes. Desired-state mutations fence on the expected generation
  (`Takoform-Expected-Generation` header or `expectedGeneration` body field);
  a stale fence fails `generation_conflict` (412).
- `metadata.revision` increments whenever the representation — including
  status, conditions, and outputs — changes. The strong ETag is the quoted
  revision; `If-Match` fences on it and a stale value fails
  `revision_conflict` (412).
- `status.observedGeneration` names the desired generation the status
  reflects; `status.conditions` uses the closed types `Ready`, `Reconciling`,
  `Degraded`, `Drifted`, `Blocked`, `Deleting` with closed portable reasons
  and an optional non-portable `hostReason`.
- The Form semantic identity is the exact FormRef. The package digest used
  at installation is audit evidence (`form.packageDigest`, optional in
  responses); it never enters queries, fences, or state identity, and a host
  that installed the same FormRef from a different legitimate package MUST
  read and delete the same resources.

## Lifecycle

Endpoints under `/apis/forms.takoform.com/v1alpha3`, keyed by group and
kind so one kind name can exist in many groups:

```
GET    {api}/forms?space&group&kind&definitionVersion&schemaDigest
GET    {api}/form-definitions/{group}/{kind}?…
POST   {api}/resources/validate         diagnostics only, no mutation, no digest
POST   {api}/resources/prepare          short-lived prepare digest for one exact spec
PUT    {api}/resources/{group}/{kind}/{name}    apply; carries review.prepareDigest
GET    {api}/resources/{group}/{kind}/{name}
POST   {api}/resources/{group}/{kind}/{name}/import
POST   {api}/resources/{group}/{kind}/{name}/observe
POST   {api}/resources/{group}/{kind}/{name}/refresh
DELETE {api}/resources/{group}/{kind}/{name}
```

`validate` reports diagnostics without mutating and without minting a
digest; a client MUST NOT describe provider apply-time preparation as
"reviewed in plan". `prepare` binds the exact spec, identity, and fences to
a `prepareDigest` the way v1alpha2 `preview` binds a plan digest;
substitution after prepare fails `invalid_argument` before mutation.

Role rules are wire-enforced: an update to a `revision`-role resource fails
`invalid_argument`; deleting a bound target fails `dependency_in_use` (409);
a resource protected by policy fails `deletion_protected` (409).

## Long-running operations

Create, update, delete, import, and artifact commit MAY
return `202 Accepted` with an Operation envelope
([`../schemas/operation-v1alpha1.schema.json`](../schemas/operation-v1alpha1.schema.json))
instead of the terminal representation:

```
GET  {api}/operations/{id}
POST {api}/operations/{id}/cancel
```

Clients poll with `Retry-After` when present, otherwise exponential backoff
with full jitter, under an overall deadline. The contract guarantees only
that an operation ID is stable, that the host keeps it addressable at
`GET {api}/operations/{id}` while the operation record exists (afterwards the
closed outcome is `operation_not_found`), and that a terminal operation
replays its terminal state instead of re-executing. A
client can resume across its own restart only if it persists the ID first; no
shipped Takoform client persists one today, so a restart during a long
operation currently means re-reading the resource, not resuming the
operation. A terminal Operation carries exactly one of `result` or `error`. Cancel is honored only for safely stoppable
operations; an already-terminal operation replays its terminal state.
`operation_not_found` (404) and `operation_cancelled` (409) are the closed
outcomes; `deadline_exceeded` (504) reports a host-side deadline.

## Artifacts

The content-addressed upload API is
[`../artifact-transport/`](../artifact-transport/README.md). The artifact
endpoints share the lane's auth, idempotency, and error taxonomy.

## Support profiles

```
GET {api}/support/forms
GET {api}/support/forms/{group}/{kind}/{definitionVersion}
GET {api}/support/interfaces/{name}/{version}
GET {api}/support/bindings/{name}/{version}
```

Responses validate against
[`../schemas/host-support-profile-v1alpha1.schema.json`](../schemas/host-support-profile-v1alpha1.schema.json).
A profile declares supported exact refs, closed capability subsets
(`supportedEnums`), inclusive ranges (`supportedRanges`), supported binding
contracts, and numeric limits. Price, SKU, region, quota, and commercial
policy MUST NOT appear; those remain Service Offering data outside this API.

## Errors

The closed taxonomy extends v1alpha2 with `rate_limited` (429),
`deadline_exceeded` (504), `operation_cancelled` (409),
`operation_not_found` (404), `dependency_in_use` (409),
`deletion_protected` (409), `artifact_missing` (404), `artifact_invalid`
(400), `unsupported_capability` (422), `migration_required` (409),
`uid_mismatch` (409), `revision_conflict` (412), and `generation_conflict`
(412). Only `resource_busy`, `backend_unavailable`, `rate_limited`, and
`deadline_exceeded` may be `retryable: true`; `Retry-After` is honored when
present. The v1alpha2 codes `resource_version_conflict`,
`interface_identity_ambiguous`, and `interface_instance_ambiguous` do not
exist in this lane: version conflicts split into the revision/generation
pair, and the v1alpha2 interface projection is replaced by exact Interface
contracts ([`../interface-contract/`](../interface-contract/README.md)).
