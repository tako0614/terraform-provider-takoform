# Scheduler Family proposals

The Scheduler Family, `schedule.forms.takoform.com/v1beta1`, is minted under
[decision 0043](../../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md):
the standalone scheduler is popularly offered as a managed service by every
major provider and has no de-facto standard API, so a host-neutral contract
is the only portability there is. Its member fixes, completely, the
application-visible semantics of the proven standalone-cron shape — a cron
expression firing one declared target under a declared retry policy —
without naming its vendor
([decision 0008](../../spec/decisions/0008-forms-preserve-service-shape.md)).

## Authoring policy: shape-preserving contracts

Every member Form preserves one service shape end to end: firing semantics,
grammar, delivery guarantees, update and delete units, and error semantics.
No free semantic token is admitted; a difference in semantics is a
different Form, never a selector value. Outward capability use is a
digest-bound Binding held by a revision resource; inward activation is an
attachment resource
([decision 0010](../../spec/decisions/0010-exact-interface-and-binding-contracts.md)).
Desired schemas carry no `name` or envelope plumbing: the resource envelope
owns identity and status
([decision 0011](../../spec/decisions/0011-resource-identity-generation-and-revision.md)).

No catalog source, generated candidate set, Interface candidate, or Binding
candidate exists for this family yet. A Form exists only when its proposal,
catalog declaration, and candidate package exist
([proposals/README.md](../index.md)); these documents reserve nothing.

## MVP members

| Form | Role | One-line semantics | Separate-Form boundary |
| --- | --- | --- | --- |
| [Schedule](schedule.md) | identity | Five-field UTC cron firing one declared message at one pinned cross-family target, at-least-once, with declared retries and no catch-up replay. | Timezone-aware scheduling, HTTPS invocation, or backfill replay is a different Form. |

## Contrast with the edge cron trigger

The Edge Platform Family's
[WorkerCronTrigger](../edge/worker-cron-trigger.md) is an attachment: it
activates a worker the family already has, and the worker's code is the
payload. A Schedule is an identity that owns its own existence and its own
declared payload — it needs no worker, and its target is an exact
cross-family reference (in MVP, a `PullQueue` send or a `Topic` publish).
The two deliberately share one cron grammar so an expression valid in one
is valid in the other. Designs that differ — `TimezoneSchedule`, an HTTPS
invocation target — are separate Form work per
[spec/form-families.md](../../spec/form-families.md).
