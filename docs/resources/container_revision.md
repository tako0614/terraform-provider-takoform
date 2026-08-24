---
page_title: "takoform_container_revision Resource - takoform"
subcategory: "Current Form Families"
description: |-
  Container Revision (container.forms.takoform.com, role revision).
---

# takoform_container_revision

Immutable serving snapshot of one Container Service: a digest-pinned OCI image, process arguments, environment declarations, sealed slots, and resource and scaling bounds.

This is a `revision` resource: an immutable snapshot. It is create-only — every desired attribute forces replacement, and rollback means pointing a deployment at an earlier revision, never editing this one.

This page documents a non-normative official Terraform Provider mapping for the
current Experimental Form `container.forms.takoform.com/ContainerRevision`.
The mapping name is provider metadata: it is absent from the Form Definition and cannot change
the Form's canonical bytes or digest. Provider publication and support are versioned separately.
The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](https://takoform.com/examples/resources/takoform_container_revision/resource.tf).

## Arguments

- `name` (String, optional, computed, forces replacement) — Portable resource name (`metadata.name`). Omit it and set `revision_owner` instead: this Form is an immutable revision, so the provider derives `container-revision-<content digest prefix>-<owner digest prefix>` from this revision's own content and its declared owner. Changed content is then a NEW revision created beside the old one, which is the only way a code change applies at all — a host refuses every update to a revision, and replacing one under a name it still holds completes in neither apply order. Setting it pins the name, which an imported revision needs; the provider then refuses at plan time any change that would replace this revision under it.
- `revision_owner` (String, optional, forces replacement) — Stable name of the logical resource that owns this revision. When the Form carries an owner relation, use that target resource's name. Required whenever `name` is omitted. Two independent resources built from identical content derive identical content digests, so without an owner they would derive one name and two Terraform resources would manage one host address — where a destroy of either breaks the other. It is provider-side authoring input: no wire member carries it, the host never sees it, and it enters only the derived name.
- `service` (String, required, forces replacement) — Container Service identity this immutable revision belongs to. Set the name of the target `ContainerService` resource.
- `image` (String, required, forces replacement) — OCI image reference pinned by a sha256 digest. A mutable tag is not portable state; the host resolves and retains the digest-pinned image before serving.
- `command` (String, optional, forces replacement) — Optional process entrypoint override. Without it the image's declared entrypoint applies; argument order is preserved.
- `args` (String, optional, forces replacement) — Optional process argument override. Without it the image's default arguments apply; argument order is preserved.
- `vars_json` (String, optional, forces replacement) — Non-secret configuration values projected into the process environment. Omitting it projects no variable; sensitive material never enters portable state. Authored as one JSON object string (for example `jsonencode({...})`); the provider sends the parsed object. Defaults to the empty object `{}`.
- `required_sensitive_vars` (Set of String, optional, forces replacement) — Names of sensitive values the host must supply through its sealed path. Only names are portable state; omitting it requires no sensitive value. Defaults to the empty list `[]`.
- `external_services` (List of Object, optional, forces replacement) — Sealed slots naming only a standard protocol and a projected NAME. The host resolves endpoint and credentials out of band; neither is portable state. Omitting it declares no external service. Each entry declares `name` (SCREAMING_SNAKE, the sealed binding slot), an opaque normalized reverse-DNS `protocol` identifier such as `com.amazonaws.s3`, and optional `required` (default true). Takoform carries no central protocol enum or protocol-specific members. The Host must fail closed unless its support profile exactly supports the identifier, then projects one sealed runtime-native binding under the slot name. Defaults to the empty list `[]`.
- `memory_mib` (Number, required, forces replacement) — Usable memory bound for one serving instance, in MiB. Between 128 and 32768.
- `cpu` (Number, required, forces replacement) — Compute allocation for one serving instance, in millicpu units. Between 1 and 16000.
- `concurrency_target` (Number, required, forces replacement) — Maximum concurrent HTTP requests delivered to one ready instance. Between 1 and 1000.
- `min_instances` (Number, required, forces replacement) — Lower bound on serving instances. Zero permits scale-to-zero. Between 0 and 1000.
- `max_instances` (Number, required, forces replacement) — Upper bound on serving instances. Between 1 and 1000.
- `timeout_seconds` (Number, required, forces replacement) — Maximum wall-clock time for one request before the host fails it, in seconds. Between 1 and 3600.
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `create_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `30m`). There is no `update_timeout`: this Form declares no update capability. Changing only these provider-side timeouts is applied in place without any host call.

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

- **Reads dispatch on the recorded FormRef.** `ContainerRevision` state is addressed under the
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

## Import

```console
terraform import takoform_container_revision.example NAME
terraform import takoform_container_revision.example SPACE/NAME
```

Both short forms bind state to this provider build's default create
FormRef. To adopt a resource created under an EARLIER definition version of
this Form, name the exact identity instead. The import ID is then one JSON
object — not a delimiter-joined string, because a SpaceID is opaque UTF-8
whose only forbidden character is `/`, so no separator can escape it safely:

```console
terraform import takoform_container_revision.example \
  '{"space":"prod","apiVersion":"container.forms.takoform.com","kind":"ContainerRevision","definitionVersion":"0.1.0","schemaDigest":"sha256:…","name":"…"}'
```

`space` is optional and falls back to the provider default; the four FormRef
members are all-or-nothing. An identity this provider build carries no codec
for is refused, naming the identities it does carry — it is never silently
rebound to the default.
