# WorkerCronTrigger — `takoform_worker_cron_trigger`

## Workload and consumer

A team runs periodic work — cache refresh, cleanup, digests — by having the
platform invoke its worker's `scheduled` handler on a cron schedule.

## Role

`attachment`. The trigger activates the worker from the outside; deleting
the trigger never deletes the worker.

## Observable semantics

`cron` is the portable five-field cron subset, interpreted in UTC only.
There is no timezone field, so two conforming hosts can never fire the same
trigger at different instants. Each match invokes the scheduled handler of
the worker's active deployment; a missed window is skipped, not queued.

The grammar is closed and deliberately small: exactly five single-value
fields separated by single spaces, where minute is a literal `0`-`59`, hour a
literal `0`-`23`, day-of-month `*` or `1`-`31`, month `*` or `1`-`12`, and
day-of-week `*` or `0`-`6`. Ranges, lists, steps, and names are rejected, and
`*` is not accepted in the minute or hour field: `* * * * *`, `0 * * * *`,
and `*/5 * * * *` are all invalid. The most frequent representable schedule
is therefore once per day at one fixed UTC time, and hourly or sub-hourly
work cannot be declared with this Form. Widening the grammar changes the
observable firing set, so it is a new definition version of this Form rather
than a host-side relaxation; every conforming host must reject what this
grammar rejects until then.

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
five-field UTC subset already used by the retained catalog.
