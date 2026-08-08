# 0018 — The Host API is deployable behind ordinary infrastructure

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

Decision [0013](0013-v1alpha3-lane-ships-in-provider-v2-1.md) opened the
`forms.takoform.com/v1alpha3` lane, and decisions
[0011](0011-resource-identity-generation-and-revision.md),
[0015](0015-cross-resource-references-are-uid-pinned-relations.md),
[0016](0016-the-worker-aggregate-has-one-active-deployment.md), and
[0017](0017-provider-state-survives-form-evolution-and-interruption.md) settled
what the lane MEANS. None of them asked whether the lane can be put in front of
a real network, in front of more than one caller, or in front of an operator who
has to work out why something is not ready. Five defects made the answer no.

**The lane demanded a percent-encoded slash inside a path segment.** Decision
[0009](0009-form-families-and-namespaced-api-versions.md) made Form groups
namespaced, so an apiVersion contains a slash. The client packed the whole
apiVersion into ONE segment, producing
`/resources/edge.forms.takoform.com%2Fv1alpha1/ModuleWorker/api`. Proxies,
gateways, and web frameworks do not agree on what `%2F` inside a path segment
is: some pass it through, some decode it before routing, some normalize it, and
several reject the request outright. The lane's routing therefore depended on
which intermediaries a deployment happened to have, and a host that worked
directly could stop working the day it was placed behind a load balancer.

**Operation ids and upload ids were bearer capabilities.** Anyone who presented
an id could read an operation, cancel it, push bytes into an upload session,
commit it, or abandon it. Nothing bound either handle to the caller that created
it. Ids travel: they are persisted in Terraform state, logged, and passed
between tools.

**A content address was treated as proof of entitlement.**
`GET /artifacts/{manifestDigest}` and `HEAD /artifacts/blobs/{sha256}` answered
anyone who named an existing digest. A manifest describes someone's deployable
code, and a digest is a short string that appears in state files, CI logs, and
error messages.

**Lane negotiation was serial and shared the resource-operation timeout.** The
provider discovered v1alpha2 and then v1alpha3 through one HTTP client whose
timeout is twelve minutes, because that is what a resource operation may
legitimately need. One unresponsive lane therefore blocked configuration
entirely — and with it the OTHER lane's resources, which had nothing to do with
the failure.

**Conditions were flattened to a boolean.** The wire carries a closed condition
list with reasons, `observedGeneration`, and transition times; the provider
projected `ready` and dropped the rest. `Ready = false` with no reason is the
state an operator most needs explained, and the only way to get the explanation
was to leave Terraform.

These are MUST-level host, wire, and provider semantics, so this repository's
`AGENTS.md` requires a decision record.

## Decision

The lane is defined so that an ordinary deployment can serve it, an id is a
handle rather than a key, and the state a client keeps is the state the host
reports. The rules below are normative; the operative text lives in
[`../host-api/v1alpha3.md`](../host-api/v1alpha3.md),
[`../host-api/operations-v1alpha3.json`](../host-api/operations-v1alpha3.json),
and the generated provider resource documents.

1. **A namespaced group travels as two ordinary path segments.** Every URL
   template that names a group carries `{formGroup}/{formVersion}`:

   ```
   {api}/resources/{formGroup}/{formVersion}/{kind}/{name}
   {api}/form-definitions/{formGroup}/{formVersion}/{kind}
   {api}/support/forms/{formGroup}/{formVersion}/{kind}/{definitionVersion}
   ```

   No path segment ever percent-encodes a slash, and a host rejoins the two
   segments into the exact apiVersion. The FormRef `apiVersion` string is
   unchanged everywhere else — request bodies, responses, and the `group` query
   key still carry `edge.forms.takoform.com/v1alpha1` verbatim, so this changes
   the lane's URLs and nothing about its identities. A client also rejects a
   discovery document whose advertised endpoint paths carry any percent-encoding,
   comparing escaped paths so that `%2Fv1alpha3` cannot pass as `/v1alpha3`. The
   `%2F` convention is removed with no compatibility shim: the lane is
   unpublished, so there is nothing to be compatible with.

2. **Operations and upload sessions are owned.** A host records the
   authenticated tenant and principal an operation or upload session was created
   by. Reading or cancelling an operation, and continuing, committing, or
   abandoning an upload, from a different tenant or a different principal fails
   with the surface's ordinary not-found outcome — `operation_not_found` (404)
   and `artifact_missing` (404) — never `permission_denied`. Abandoning an
   upload id the caller does not hold fails the same way whether the id never
   existed or belongs to someone else.

3. **A content address is not a capability.** `GET /artifacts/{manifestDigest}`
   and `HEAD /artifacts/blobs/{sha256}` answer only a caller whose tenant
   already holds that address, acquired by uploading the bytes or by committing
   a manifest that names them; any other tenant is answered as if the address
   did not exist. `missingBlobs` is therefore answered per tenant rather than
   per byte store. Physical deduplication is explicitly still permitted: one
   stored copy per content address, one immutable identity for identical bytes,
   and no tenant's abandon or garbage collection may take bytes away from
   another.

4. **Lanes negotiate independently, under a short deadline of their own.** A
   client discovers each lane concurrently, bounded by a discovery deadline
   separate from the resource-operation deadline. A lane that fails or hangs
   never prevents another lane's resources from working, and each resource type
   reports its own lane's negotiation error.

5. **Conditions are client-visible state.** The provider projects the full
   condition list — type, status, reason, message, host reason, observed
   generation, and last transition time — as a typed read-only `conditions`
   attribute, and keeps `ready` as a convenience derived from the same list.
   Conditions are host-rendered state that changes when ANOTHER resource is
   mutated (decisions
   [0015](0015-cross-resource-references-are-uid-pinned-relations.md) and
   [0016](0016-the-worker-aggregate-has-one-active-deployment.md)), so the
   attribute is computed-only and carries no plan modifier that would hold a
   previous value the host has already contradicted.

Rules 1 through 3 are proved by required conformance checks
(`namespaced-group-travels-as-two-path-segments`,
`operation-bound-to-its-creating-principal`,
`upload-session-bound-to-its-creating-principal`,
`artifact-digest-is-not-a-capability`); rules 4 and 5 are provider behavior
proved by provider tests.

## Consequences

- Every v1alpha3 URL that names a group changes shape. The client, the reference
  host, the conformance runner, the machine operation table, and the lane
  specification move together; no published schema or identity changes, because
  the apiVersion string itself is untouched.
- A host must carry two more facts it previously did not: who created each
  operation and upload session, and which tenants hold each content address.
  Both are small per-record additions, and neither constrains how the bytes are
  stored.
- `missingBlobs` may now report a blob whose bytes the host already has. The
  cost is one re-upload the first time a tenant wants an address someone else
  introduced; the alternative is a dedup oracle that hands over content to
  whoever can name it.
- Two tenants uploading identical bytes converge on one identity and one stored
  copy, so the dedup that motivated content addressing is preserved exactly.
- Configuration gets faster and more predictable: both lanes are negotiated at
  once, and the worst case is one short discovery deadline rather than one long
  resource-operation timeout.
- `conditions` appears in state for every v1alpha3 resource. It is computed, so
  no configuration changes; operators gain the reason, the host detail, and the
  observed generation without leaving Terraform.

## Rejected alternatives

- **Keep `%2F` and document a proxy requirement.** Rejected because it makes the
  lane's correctness a property of someone else's deployment. "Configure every
  intermediary to pass encoded slashes through" is not a requirement a portable
  specification can impose: the operator often does not control the
  intermediaries, several reject `%2F` with no setting to change, and the
  failure appears as a routing error far from the contract that caused it. Two
  ordinary segments cost nothing and are unambiguous everywhere.
- **Split only the resource route and leave the others.** Rejected because one
  shape is what makes the rule checkable. A lane where two of the four
  group-bearing routes are ordinary and two are not is harder to describe, and
  the two exceptions would be exactly the routes a new implementation gets
  wrong.
- **Return `permission_denied` (403) for another principal's operation.**
  Rejected because it answers the question the stranger asked. A 403 says "this
  id names a real operation, and it is not yours" — an existence oracle over
  ids that appear in state files and logs. The not-found outcome is also the
  honest one: the caller has no operation by that id.
- **Treat a digest as proof of entitlement — "you can only read it if you
  already know it".** Rejected because knowing a digest is not knowing the
  content. Digests are short, copied into state files and CI logs, and derivable
  by anyone who can guess the bytes; a manifest is a description of deployable
  code, and a blob is the code. Content addressing is a naming scheme, and
  making it double as authorization means the only protection is that the name
  is hard to say.
- **Scope artifact access per principal rather than per tenant.** Rejected
  because an artifact is a tenant asset: the CI principal that uploads a bundle
  and the apply principal that references it are routinely different, and
  per-principal scoping would break the ordinary pipeline while adding no
  protection the tenant boundary does not already give.
- **Refuse to store bytes a second tenant uploads, to keep one copy.**
  Rejected as solving the wrong problem: deduplication is a storage decision and
  authorization is an access decision. Physical dedup stays; what changes is
  that holding an address is recorded rather than assumed.
- **Bound discovery with the existing resource-operation timeout.** Rejected
  because the two operations are nothing alike. Discovery is one small read of a
  static document; a resource operation may take minutes by design. Sharing the
  long bound made the provider's startup as slow as its slowest legitimate
  mutation.
- **Project conditions as an opaque `conditions_json` string.** Rejected because
  a typed list is representable in this framework version and a JSON blob would
  push parsing into every configuration that wants a reason, with no schema, no
  documentation, and no type checking.
- **Make `conditions` optional so a configuration can assert an expected
  state.** Rejected because conditions are host-owned and change without any
  desired spec changing; a configured expectation would diff forever.
