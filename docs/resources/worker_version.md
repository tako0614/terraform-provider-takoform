---
page_title: "takoform_worker_version Resource - takoform"
subcategory: "tako0614 Forms"
description: |-
  Worker Version (edge.forms.takoform.com, role revision).
---

# takoform_worker_version

Immutable executable snapshot of one Module Worker: a bundle, the handlers its module exports, non-secret vars, and the typed capability bindings the code may use. A change is a new Worker Version; traffic moves only through Worker Deployments. The runtime this code runs on is not a field of this Form: it is fixed by the worker.runtime@1.1.0 contract the Module Worker identity provides, so a version carries no compatibility date and no compatibility flag (decision 0019).

This is a `revision` resource: an immutable snapshot. It is create-only — every desired attribute forces replacement, and rollback means pointing a deployment at an earlier revision, never editing this one.

This page documents the publisher-set Provider 4 mapping; its exact FormRef `edge.forms.takoform.com/WorkerVersion` is recorded below.
The resource type is Provider metadata. The published Edge Form Definition is maintained in the [takoform-forms source](https://github.com/tako0614/takoform-forms/blob/3231633605b737ce5279d7fc020b4780568e7091/forms/candidates/edge.forms.takoform.com/worker-version/definition.json).
See the complete exact identity and the [complete example](https://takoform.com/examples/resources/takoform_worker_version/resource.tf).

## Exact FormRef

This Provider mapping carries the following exact four-field FormRef:

```json
{
  "apiVersion": "edge.forms.takoform.com",
  "kind": "WorkerVersion",
  "definitionVersion": "0.3.0",
  "schemaDigest": "sha256:65870343bfab512fe5e7ae6faea8b3dbc48f9c9de0d4d9349dcbfd819f06d365"
}
```

`packageDigest` — Form Package digest (separate from FormRef; embedded Provider provenance): `sha256:21adc2e4e677cd31e905483d38eff60c9eb61112f6c234a01d6a487154980891`.

## Arguments

- `name` (String, optional, computed, forces replacement) — Portable resource name (`metadata.name`). Omit it and set `revision_owner` instead: this Form is an immutable revision, so the provider derives `version-<content digest prefix>-<owner digest prefix>` from this revision's own content and its declared owner. Changed content is then a NEW revision created beside the old one, which is the only way a code change applies at all — a host refuses every update to a revision, and replacing one under a name it still holds completes in neither apply order. Setting it pins the name, which an imported revision needs; the provider then refuses at plan time any change that would replace this revision under it.
- `revision_owner` (String, optional, forces replacement) — Stable name of the logical resource that owns this revision. When the Form carries an owner relation, use that target resource's name. Required whenever `name` is omitted. Two independent resources built from identical content derive identical content digests, so without an owner they would derive one name and two Terraform resources would manage one host address — where a destroy of either breaks the other. It is provider-side authoring input: no wire member carries it, the host never sees it, and it enters only the derived name. The repository [`worker-app` module](https://github.com/tako0614/terraform-provider-takoform/tree/main/modules/worker-app) sets it for you.
- `worker` (String, required, forces replacement) — Module Worker identity this version belongs to. Set the name of the target `ModuleWorker` resource.
- `bundle` (String, required, forces replacement) — Worker Bundle carrying the exact module bytes this version executes. Set the name of the target `WorkerBundle` resource.
- `assets` (Object, optional, forces replacement) — Optional static-asset attachment for this immutable version. Without it the host performs no asset lookup. When present, every member is required and the request order is closed: with runWorkerFirst=false the host tries the asset lookup before invoking fetch; with true it invokes fetch first and tries the asset lookup only when that invocation returns 404. An asset result wins; if both stages miss, the worker's 404 is preserved. The attachment never grants a hidden runtime binding and never mutates the referenced bundle. The object declares `bundle`, `run_worker_first`, `not_found_handling`; when the object is present, every member is required.
- `handlers` (Set of String, required, forces replacement) — Module event handlers this version exports, from the closed vocabulary the worker.runtime@1.1.0 contract defines. A host rejects a handler that contract does not define, and rejects an attachment whose event kind is not declared here. One of `fetch`, `scheduled`, `queue`.
- `vars_json` (String, optional, forces replacement) — Non-secret configuration values projected into the module environment. Sensitive material never enters portable state. Omitting it projects no variable. Authored as one JSON object string (for example `jsonencode({...})`); the provider sends the parsed object. Defaults to the empty object `{}`.
- `kv_bindings` (List of Object, optional, forces replacement) — Typed module-worker.edge-kv bindings projecting the edge.kv API under JavaScript identifier names. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `EdgeKVNamespace` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `bucket_bindings` (List of Object, optional, forces replacement) — Typed module-worker.object-bucket bindings projecting the edge.objects API. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `ObjectBucket` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `sqlite_bindings` (List of Object, optional, forces replacement) — Typed module-worker.sqlite bindings projecting the edge.sql API. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `SQLiteDatabase` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `queue_producer_bindings` (List of Object, optional, forces replacement) — Typed module-worker.queue-producer bindings projecting only send and sendBatch. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `AtLeastOnceQueue` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `service_bindings` (List of Object, optional, forces replacement) — Typed module-worker.service bindings projecting worker.service fetch toward another Module Worker. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `ModuleWorker` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `workflow_bindings` (List of Object, optional, forces replacement) — Typed module-worker.workflow bindings projecting the instance surface — create, get, status, sendEvent, terminate. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `DurableWorkflow` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `actor_bindings` (List of Object, optional, forces replacement) — Typed module-worker.actor bindings projecting addressing and invocation — idFromName, newUniqueId, get. Omitting it declares no such binding. Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `ActorNamespace` resource name); the wire carries the typed `resource` reference. Defaults to the empty list `[]`.
- `required_sensitive_vars` (Set of String, optional, forces replacement) — Names of sensitive values this version requires the host to supply out-of-band. Only the names are portable state; values travel through each host's own sealed path. Omitting it requires no sensitive value. Defaults to the empty list `[]`.
- `external_services` (List of Object, optional, forces replacement) — External standard services this version speaks, each a sealed slot naming only a runtime binding NAME and an opaque namespaced protocol. The host resolves the integration out-of-band and supplies one sealed runtime-native binding under NAME; neither its entries nor the credential is portable state. A required slot the host cannot satisfy keeps the version from becoming Ready. Omitting it declares no external service. Each entry declares `name` (SCREAMING_SNAKE, the sealed binding slot), an opaque normalized reverse-DNS `protocol` identifier such as `com.amazonaws.s3`, and optional `required` (default true). Takoform carries no central protocol enum or protocol-specific members. The Host must fail closed unless its support profile exactly supports the identifier, then projects one sealed runtime-native binding under the slot name. Defaults to the empty list `[]`.
- `apply_idempotency_key` (Provider 4.0.0, String, optional, computed, forces replacement) — Provider-only Host operation identity for this immutable version's apply. Retained Provider 3.0.0 does not expose this attribute. On an ordinary run with no `runtime_input_nonce`, omitting it keeps the Provider's established deterministic operation key; an explicitly configured 1..255-byte visible-ASCII value retains the caller-selected behavior. When `runtime_input_nonce` is configured on this exact Provider instance, this argument must be omitted: the Provider computes it from that nonce and the exact value-free logical WorkerVersion apply identity and records only the opaque result in plan and state. Rotating a sensitive value under the same nonce does not change the key; rotating the nonce does, producing a new immutable WorkerVersion identity. The key is never included in the portable desired spec or read back from the Host.
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `create_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `30m`). There is no `update_timeout`: this Form declares no update capability. Changing only these provider-side timeouts is applied in place without any host call.

## Run-scoped sensitive inputs

`required_sensitive_vars` declares names only. When it is non-empty, the exact Provider instance must receive a 22..128 character unpadded-base64url `runtime_input_nonce` during Plan. Plan never reads a value from the sensitive `runtime_inputs` map: OpenTofu re-plans every resource inside the apply walk with the apply-time provider configuration, so the map may already be present there and is ignored until Apply. Apply must reuse that nonce and supply a map whose names exactly equal the declaration. Missing or extra names, a changed nonce, or a value supplied to a different Provider instance is refused before any Host mutation.

The runner supplies the Apply-only map through an ephemeral root variable and the selected Provider block. It writes the transient variable file to the OpenTofu process's standard input; it does not put values in command arguments, process environment, ordinary credential files, plan, or state. This Provider ignores `TAKOFORM_RUNTIME_INPUTS_FILE` and ambient environment variables named by the declaration. One module may use other Takoform or industry-standard Provider instances; none receives this map unless the root explicitly targets it.

The map is limited to 1..64 bindings, each value to 1..32768 bytes of UTF-8 text without NUL, and the runner dispatch to 1 MiB total. These limits are separate from the value-free public apply envelope: `publicApply.path` is limited to 8,192 UTF-8 bytes and `publicApply.body` is limited to 1,048,576 UTF-8 bytes. An overlong path, body, binding, or dispatch is refused before a commitment, private preparation, or public Host mutation.

Every run first reads the value-free private preparation by the deterministic operation key. Only an absent record permits one same-origin private PUT of the plaintext bindings over TLS. A prepared record continues with one ordinary public PUT; an accepted, dispatched, or consumed record polls its exact ordinary Host operation without resending bindings or replaying the public PUT. After any PUT acknowledgement failure, recovery uses a fresh bounded context and value-free private readback.

Values never enter the public apply body, Terraform plan or state, Provider logs or diagnostics, or the computed operation key. The Provider keeps accepted values in mutable buffers where practical, wipes those buffers and transport response buffers promptly on a best-effort basis, and drops references after the private PUT. This is not a guarantee of Go process-memory erasure: the runtime, compiler, HTTP/TLS stack, operating system, or a crash dump may retain copies outside those buffers. Runner operators must protect or disable crash dumps and process inspection as appropriate. The durable guarantee is absence from plan, state, logs, diagnostics, and public Host requests.

## Read-only attributes

- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.
- `generation` — desired-state generation; increments only when the portable desired spec changes; this Form declares no update capability — every desired attribute forces replacement instead. It is also the DELETE fence, because a delete withdraws desired state like any other desired-state mutation.
- `revision` — representation revision; increments whenever the representation changes — a spec-changing update, new status, new outputs, or a change to another resource this one is rendered from. It is the strong ETag, and it is deliberately NOT the delete fence: a teardown removes dependents first and would otherwise be refused by a revision it moved itself.
- `conditions` — the complete status condition list the host reports, in its order. Each entry carries
  `type` (the closed `Ready` / `Reconciling` / `Degraded` / `Drifted` / `Blocked` / `Deleting` vocabulary),
  `status` (`True` / `False` / `Unknown`), the closed portable `reason`, an optional `message`, an optional
  non-portable `host_reason` naming exactly what is wrong, the `observed_generation` the status reflects,
  and `last_transition_time`. Conditions are host-rendered state: they change when this resource changes
  AND when a resource it depends on changes, with no desired spec changing anywhere, so they are read-only
  and a configuration must not assert them.
- `ready` — derived convenience: true when `conditions` carries the closed `Ready` condition with status
  `True`. Read `conditions` for the reason it is not.
- `outputs_json` — the WHOLE `status.outputs` document, JSON-serialized. This Form declares no `outputSchema`, so a conforming host omits `status.outputs` entirely and this
  attribute is `"{}"`. It stays declared because a host may publish a value no contract describes, and
  an undescribed value must still be reachable rather than silently dropped.
- `form_api_version`, `form_kind`, `form_definition_version`, `form_schema_digest` — the exact immutable FormRef this state is bound to; reads dispatch on it.
- `form_package_digest` — audit-only package provenance; never part of resource identity, queries, or fences.
- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes replacing this resource, because this Form declares no in-place update and a host refuses every apply to the existing one. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.
- `pending_operation_id` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.

## State continuity

- **Reads dispatch on the recorded FormRef.** `WorkerVersion` state is addressed under the
  exact `form_*` identity it records, not under this build's default create ref, so a
  resource created before the Form line advanced stays addressable as itself. An identity
  this provider build carries no codec for is a hard error naming that identity and the
  ones the build does carry; the provider never substitutes another exact FormRef, because
  a substituted query's "not found" is indistinguishable from deletion.
- **A changed `uid` is an error, and state is kept.** When the host serves a different
  `uid` under the recorded name, the resource this state was applied against is gone and
  something re-used its name. The provider reports a hard error naming both uids and keeps
  the resource in state. It does not re-bind — that would adopt a resource you never
  applied — and it does not remove state, which would make the next apply fail against the
  resource that does exist, with no plan left to repair it. Resolve it by importing the new
  incarnation explicitly, restoring the prior one, or deleting the host-side replacement.
- **An unfinished mutation is resumed, not re-created.** When `pending_operation_id` is
  set, a refresh asks the host about that operation before it reads the resource. While the
  operation is still running the resource may legitimately not exist yet, so its absence is
  not treated as deletion and the marker survives; a terminal success is verified against
  the exact identity and settles state; a terminal failure or an expired operation record
  defers to an exact read of the resource, which decides. Refresh again once the host
  settles.

## Accepted bindings

- `module-worker.edge-kv@1.0.0`
- `module-worker.object-bucket@1.1.0`
- `module-worker.sqlite@1.0.0`
- `module-worker.queue-producer@1.0.0`
- `module-worker.service@1.0.0`
- `module-worker.workflow@1.0.0`
- `module-worker.actor@1.0.0`

Outward capability use is a typed binding held by this revision; inward
activation (routes, domains, cron, queue consumption) is a separate
attachment resource. The two are never merged.

## Import

```console
terraform import takoform_worker_version.example NAME
terraform import takoform_worker_version.example SPACE/NAME
```

Both short forms bind state to this provider build's default create
FormRef. To adopt a resource created under an EARLIER definition version of
this Form, name the exact identity instead. The import ID is then one JSON
object — not a delimiter-joined string, because a SpaceID is opaque UTF-8
whose only forbidden character is `/`, so no separator can escape it safely:

```console
terraform import takoform_worker_version.example \
  '{"space":"prod","apiVersion":"edge.forms.takoform.com","kind":"WorkerVersion","definitionVersion":"0.3.0","schemaDigest":"sha256:…","name":"…"}'
```

`space` is optional and falls back to the provider default; the four FormRef
members are all-or-nothing. An identity this provider build carries no codec
for is refused, naming the identities it does carry — it is never silently
rebound to the default.
