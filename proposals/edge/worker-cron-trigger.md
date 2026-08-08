# WorkerCronTrigger — `takoform_worker_cron_trigger`

## Workload and consumer

A team runs periodic work — cache refresh, cleanup, digests — by having the
platform invoke its worker's `scheduled` handler on a cron schedule.

## Role

`attachment`. The trigger activates the worker from the outside; deleting
the trigger never deletes the worker.

## Observable semantics

`cron` is the portable five-field cron grammar, interpreted in UTC only.
There is no timezone field, so two conforming hosts can never fire the same
trigger at different instants and no schedule skips or repeats an hour for a
daylight-saving transition. Each match invokes the scheduled handler of the
worker's active deployment.

The five fields are minute `0`-`59`, hour `0`-`23`, day-of-month `1`-`31`,
month `1`-`12`, and day-of-week `0`-`6` with `0` Sunday. Each field is a
comma-separated list of `*`, a literal, a range `low-high`, `*/step`, or
`low-high/step`, so `* * * * *`, `0 * * * *`, and `*/5 * * * *` are all
valid — the grammar exists for exactly those. Month and day names, and a step
on a bare literal such as `5/10`, are not accepted. When day-of-month and
day-of-week are both restricted the trigger fires on a day either selects;
when only one is restricted, only that one constrains the day.

The pattern in the Form Definition is the structural half of the grammar. A
host also PARSES the expression, and refuses a value outside its field's
range, an inverted range, or a step outside `1`..span, before any mutation;
the provider runs the same parser at plan time, so a configuration that plans
is one that applies (decision 0020).

Delivery of a match is at-least-once, so a handler may run more than once for
one matched minute and must be idempotent. A missed run is skipped rather
than queued or fired late, so a schedule never produces a backlog. An
uncaught exception in the handler is a failed invocation reported to host
diagnostics; it is not retried within the matched minute.

## Why this is one Form

Schedule plus target is one observable fact. The UTC-only rule is the
completeness boundary of decision 0008: leaving the zone open would let two
hosts fire at observably different times.

## What would require a separate Form

Timezone-aware or calendar-aware scheduling (a `TimezoneSchedule`) carries
different firing semantics and is separate family work.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

Attaching a trigger to a worker whose versions do not declare the
`scheduled` handler must fail closed. Updating `cron` is an in-place update;
changing `worker` replaces the attachment.

## Prior art

The cron trigger of a proven edge platform, restricted to the interoperable
five-field UTC grammar: `*`, literals, lists, ranges, and steps, with no
names and no seconds field.
