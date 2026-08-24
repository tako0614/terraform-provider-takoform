# Schedule — `takoform_schedule`

## Workload and consumer

A team runs periodic work — nightly exports, digest kicks, cleanup —
without owning a worker for the clock: the schedule itself delivers a
declared message on a cron cadence, and consumers pull the work from the
queue or subscription it lands in. Lifecycle is managed through the
provider from birth.

## Role

`identity`. Like SQLiteDatabase, the semantics are the identity — plus a
small desired surface: `cron`, `target`, `retryPolicy`, `paused`. There is
no revision or attachment split: the schedule is the whole running thing,
so there is nothing a split would hold together.

## Observable semantics

`cron` is the portable five-field cron grammar, interpreted in UTC only —
the same grammar as the edge WorkerCronTrigger, restated exactly. There is
no timezone field, so two conforming hosts can never fire the same
schedule at different instants and no schedule skips or repeats an hour
for a daylight-saving transition. The five fields are minute `0`-`59`,
hour `0`-`23`, day-of-month `1`-`31`, month `1`-`12`, and day-of-week
`0`-`6` with `0` Sunday. Each field is a comma-separated list of `*`, a
literal, a range `low-high`, `*/step`, or `low-high/step`. Month and day
names, and a step on a bare literal such as `5/10`, are not accepted. When
day-of-month and day-of-week are both restricted the schedule fires on a
day either selects; when only one is restricted, only that one constrains
the day. A host PARSES the expression and refuses a value outside its
field's range, an inverted range, or a step outside `1`..span before any
mutation; the provider runs the same parser at plan time.

`target` is a closed union with exactly one member: `queueMessage`
(`{queue, body, attributes}` — send the declared message to a resource
providing `queue.pull@1.0.0`; in MVP a `PullQueue`) or `topicPublish`
(`{topic, body, attributes}` — publish it to a resource providing
`topic.publish@1.0.0`; in MVP a `Topic`). Body and attributes use the
queue family's message shape and bound. Each reference is the closed exact
`{apiVersion, kind, name}` shape carrying its group explicitly, so the
cross-family target is representable as-is
([binding contract](../../spec/binding-contract/index.md)), and is
UID-pinned: reference-failure semantics follow the Host API v1 relation rules
(`ExternalChange` / `DependencyMissing`, no automatic re-bind, re-apply
re-pins).

Firing: each matched window fires the target once, at-least-once — one
matched window may fire twice, so downstream consumers must be idempotent.
A failed attempt (the target refused or was unavailable) is retried under
`retryPolicy`: `maxAttempts` (1..10, counting the first attempt) tries
separated by `retryDelaySeconds` (0..3600, fixed delay). Attempts for one
window are abandoned when the next matched window fires: the schedule
never works two windows concurrently and never accumulates a backlog.

Misfire: if the scheduler was unavailable across a window, that window is
SKIPPED, never replayed late. The stated bound: no first attempt for a
window begins more than five minutes after its matched minute; a window
whose first attempt has not begun by then never fires. There is no
catch-up replay of missed windows.

`paused`: while true, windows continue to match and are permanently
skipped; unpausing fires nothing retroactively. Every desired field
updates in place — the Form retains no data, so identity is the only thing
replacement would lose.

## Why this is one Form

Grammar, target, payload, and retry are one observable fact with one
lifecycle. The UTC-only rule is the completeness boundary of decision
0008; splitting the target into an attachment would manufacture a parent
with nothing of its own to hold.

## What would require a separate Form

Timezone-aware or calendar-aware scheduling (`TimezoneSchedule`); an HTTPS
invocation target, deferred until the endpoint-authentication shape is
fixed (the same caveat as TopicSubscription push delivery); catch-up or
backfill replay of missed windows, which is a different firing contract.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

Deleting the referenced queue or topic while a Schedule targets it must
fail with `dependency_in_use`. A target deleted and re-created under the
same name is a different incarnation: the schedule stays pinned, reports
the condition per the Host API v1 relation rules, and only a re-apply re-pins
it; while the condition stands, matched windows fire, fail, and are bound
by the misfire rule — they are never queued for the repaired target.

## Prior art

The standalone cron scheduler offered by every major provider — the
"Scheduler" survey row decision 0043 minted this family from. The
withdrawn v1alpha2 `Schedule` kind, which this Form replaces with a
complete firing contract. The Edge family's WorkerCronTrigger for
contrast: an attachment invoking a worker's `scheduled` handler, sharing
this Form's exact cron grammar, with the worker as the payload instead of
a declared message.
