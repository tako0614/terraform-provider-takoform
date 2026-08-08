---
page_title: "takoform_worker_endpoint Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Worker Endpoint (edge.forms.takoform.com/v1alpha1, role attachment).
---

# takoform_worker_endpoint

Makes one Module Worker reachable over HTTPS at an address the HOST assigns, with no customer-owned domain and no DNS of the author's. The desired state is the worker and nothing else: the author asks for reachability, and where that reachability lives is the host's decision, exactly as an account, a region, and a vendor subdomain are. Requests arriving at the address invoke the fetch handler of the worker's ACTIVE DEPLOYMENT, so promotion and rollback move what answers without the endpoint being re-applied and without its address changing. The scheme is https and the path root is `/`; TLS is not an option a host may decline, because an address that is only reachable in plaintext is a different promise from the one this Form makes. A worker has AT MOST ONE endpoint: two would be two addresses for one service with nothing saying which is canonical, and the second is refused. The assigned address is published as outputs — a portable author may rely on a value being returned, on its scheme being https, and on it routing to the active deployment, and on nothing about its SHAPE: which subdomain, which apex, and how long the label is are host detail no portable configuration may parse or reconstruct.

This is an `attachment` resource: it connects a parent to inward activation (routes, domains, schedules, queue consumption). Deleting the attachment never deletes the parent.

This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or
later (source candidate; not yet published). The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](../../examples/resources/takoform_worker_endpoint/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `worker` (String, required, forces replacement) — Module Worker whose active deployment answers requests at the assigned address. Changing it replaces the endpoint. Set the name of the target `ModuleWorker` resource.
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `create_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `30m`). There is no `update_timeout`: this Form declares no update capability. Changing only these provider-side timeouts is applied in place without any host call.

## Read-only attributes

- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.
- `generation` — desired-state generation; increments only when the portable desired spec changes; this Form declares no update capability — every desired attribute forces replacement instead.
- `revision` — representation revision; increments whenever the representation changes — a spec-changing update, new status, or new outputs. Deletes fence on it via `If-Match`.
- `conditions` — the complete status condition list the host reports, in its order. Each entry carries
  `type` (the closed `Ready` / `Reconciling` / `Degraded` / `Drifted` / `Blocked` / `Deleting` vocabulary),
  `status` (`True` / `False` / `Unknown`), the closed portable `reason`, an optional `message`, an optional
  non-portable `host_reason` naming exactly what is wrong, the `observed_generation` the status reflects,
  and `last_transition_time`. Conditions are host-rendered state: they change when this resource changes
  AND when a resource it depends on changes, with no desired spec changing anywhere, so they are read-only
  and a configuration must not assert them.
- `ready` — derived convenience: true when `conditions` carries the closed `Ready` condition with status
  `True`. Read `conditions` for the reason it is not.
- `outputs_json` — the WHOLE `status.outputs` document, JSON-serialized. It is not narrowed by the typed output attributes below: every value that has a typed
  attribute is still in this document under its wire name, so a configuration that reads
  `jsondecode(...)["…"]` keeps working unchanged. What it is now FOR is the other case — reaching an
  output the Form's `outputSchema` does not describe.
- `form_api_version`, `form_kind`, `form_definition_version`, `form_schema_digest` — the exact immutable FormRef this state is bound to; reads dispatch on it.
- `form_package_digest` — audit-only package provenance; never part of resource identity, queries, or fences.
- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes replacing this resource, because this Form declares no in-place update and a host refuses every apply to the existing one. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.
- `pending_operation_id` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.

## Outputs

This Form declares an `outputSchema`, so a conforming host returns exactly these
values in `status.outputs` and the provider surfaces each one as a typed computed
attribute. Read `takoform_worker_endpoint.example.<name>` rather than decoding
`outputs_json`; the JSON document still carries every value under its wire name and
stays the way to reach an output no schema describes.

- `hostname` (String, computed) — Dotted DNS hostname the host assigned to this endpoint, in canonical form: lowercase where DNS is case-insensitive and no trailing root dot. An author's hostname admits those spellings because a host canonicalizes what it is given; an assigned name has no earlier spelling to preserve. Its VALUE is portable to read and pass on; its SHAPE is host detail, so a portable configuration never parses it, never asserts a suffix, and never reconstructs it from the resource name.
- `url` (String, computed) — Absolute HTTPS URL of the endpoint's path root: exactly `https://` + the assigned hostname + `/`. The scheme is fixed by the Form and the path root is `/`; there is no plaintext address and no port, so a consumer composes deeper paths onto this value rather than deriving an origin.

Outputs are never arguments: a configuration that sets one is rejected at validate
time. They are host-computed state, so they can change without any desired attribute
changing — a plan that touches this resource shows them as known-after-apply.

## State continuity

- **Reads dispatch on the recorded FormRef.** `WorkerEndpoint` state is addressed under the
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
terraform import takoform_worker_endpoint.example NAME
terraform import takoform_worker_endpoint.example SPACE/NAME
```

Both short forms bind state to this provider build's default create
FormRef. To adopt a resource created under an EARLIER definition version of
this Form, name the exact identity instead. The import ID is then one JSON
object — not a delimiter-joined string, because a SpaceID is opaque UTF-8
whose only forbidden character is `/`, so no separator can escape it safely:

```console
terraform import takoform_worker_endpoint.example \
  '{"space":"prod","apiVersion":"edge.forms.takoform.com/v1alpha1","kind":"WorkerEndpoint","definitionVersion":"0.1.0","schemaDigest":"sha256:…","name":"…"}'
```

`space` is optional and falls back to the provider default; the four FormRef
members are all-or-nothing. An identity this provider build carries no codec
for is refused, naming the identities it does carry — it is never silently
rebound to the default.
