---
page_title: "takoform_queue_consumer Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  Queue Consumer (edge.forms.takoform.com, role attachment).
---

# takoform_queue_consumer

Attaches one Module Worker as the batch consumer of one At-Least-Once Queue, invoking its queue handler with message batches and redelivering messages that were not acknowledged. Consumption is inward activation and therefore an attachment, never a binding. One queue has at most one consumer, so a second attachment against the same queue is refused. A handler that returns normally without settling anything acknowledges the whole batch; one that throws retries every message it had not already acknowledged. maxRetries counts REDELIVERIES only — the first delivery does not count toward it — so a message is delivered at most 1 + maxRetries times, and a message that exhausts them moves to dead_letter_queue when one is declared and is dropped otherwise. The dead-letter copy is a new message there: new identity, new acceptance timestamp, and an attempt count starting again at 1 (decision 0020). Because that transfer resets the attempt count, dead_letter_queue MUST NOT lead back: a destination resolving to the queue this consumer drains, or closing a cycle of any length through the dead-letter graph, is refused before any mutation, because an exhausted message would circulate forever instead of coming to rest (decision 0026).

This is an `attachment` resource: it connects a parent to inward activation (routes, domains, schedules, queue consumption). Deleting the attachment never deletes the parent.

This Experimental Form speaks the Host API v1beta1 lane. Its edge.forms.takoform.com identity is not yet carried by any
Registry-published provider release: it ships with the next provider release
(decision 0046). Registry-published provider v2.1.1 serves this resource type
under the retained edge.forms.takoform.com/v1beta1 identities; release/version.json retains
candidate-only descriptor metadata after owner publication. The configured host selects and
operates the concrete backend; no attribute names a vendor, target, credential,
price, or implementation. See the [complete example](https://takoform.com/examples/resources/takoform_queue_consumer/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).
- `queue` (String, required, forces replacement) — Queue this consumer drains. Changing it replaces the attachment. Set the name of the target `AtLeastOnceQueue` resource.
- `worker` (String, required, forces replacement) — Module Worker whose queue handler receives the batches. Changing it replaces the attachment. Set the name of the target `ModuleWorker` resource.
- `max_batch_size` (Number, required) — Largest number of messages delivered in one batch. Between 1 and 100.
- `max_batch_timeout_seconds` (Number, required) — Longest time the host waits to fill a batch before delivering it, in seconds. Between 0 and 60.
- `max_retries` (Number, required) — How many times a failed batch is redelivered before its messages go to the dead-letter queue or are dropped. Between 0 and 100.
- `retry_delay_seconds` (Number, required) — Delay before a failed batch becomes deliverable again, in seconds. Between 0 and 43200.
- `dead_letter_queue` (String, optional) — Queue receiving messages that exhausted their retries. Without it, exhausted messages are dropped. It must not resolve to the queue this consumer drains, or close a cycle through other consumers' dead-letter destinations; a host refuses either before any mutation. Set the name of the target `AtLeastOnceQueue` resource.
- `max_concurrency` (Number, required) — Largest number of concurrent batch invocations. Between 1 and 250.
- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.
- `create_timeout` / `update_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `20m` / `30m`).

## Read-only attributes

- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.
- `generation` — desired-state generation; increments only when the portable desired spec changes. Updates fence on it. It is also the DELETE fence, because a delete withdraws desired state like any other desired-state mutation.
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
- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes an in-place re-apply of the same desired state, which is all a host needs to re-resolve and re-pin every reference. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.
- `pending_operation_id` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.

## State continuity

- **Reads dispatch on the recorded FormRef.** `QueueConsumer` state is addressed under the
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
terraform import takoform_queue_consumer.example NAME
terraform import takoform_queue_consumer.example SPACE/NAME
```

Both short forms bind state to this provider build's default create
FormRef. To adopt a resource created under an EARLIER definition version of
this Form, name the exact identity instead. The import ID is then one JSON
object — not a delimiter-joined string, because a SpaceID is opaque UTF-8
whose only forbidden character is `/`, so no separator can escape it safely:

```console
terraform import takoform_queue_consumer.example \
  '{"space":"prod","apiVersion":"edge.forms.takoform.com","kind":"QueueConsumer","definitionVersion":"0.1.0","schemaDigest":"sha256:…","name":"…"}'
```

`space` is optional and falls back to the provider default; the four FormRef
members are all-or-nothing. An identity this provider build carries no codec
for is refused, naming the identities it does carry — it is never silently
rebound to the default.
