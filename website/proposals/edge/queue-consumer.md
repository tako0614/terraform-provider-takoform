# QueueConsumer — `takoform_queue_consumer`

## Workload and consumer

A team wires one Module Worker as the batch consumer of one
At-Least-Once Queue: the platform invokes the worker's `queue` handler with
message batches and redelivers failed batches until retries are exhausted.

## Role

`attachment`. Queue consumption is inward activation and therefore an
attachment, never a binding (decision 0010). Deleting the consumer detaches
it and never deletes the queue or the worker.

## Observable semantics

`queue` and `worker` are immutable; changing either replaces the attachment.
One queue has at most one consumer, so a second attachment against the same
queue is refused. Batching is bounded by `maxBatchSize` (1..100) and
`maxBatchTimeoutSeconds` (0..60). A handler that returns without settling
anything acknowledges the whole batch; one that throws retries every message
it had not already acknowledged, after `retryDelaySeconds`. `maxRetries`
counts REDELIVERIES only — the first delivery does not count toward it — so a
message is delivered at most `1 + maxRetries` times; exhausted messages go to
`deadLetterQueue` when declared and are dropped otherwise, and the
dead-letter copy is a new message there with its own identity, its own
acceptance timestamp, and an attempt count starting again at 1.
`maxConcurrency` (1..250) bounds concurrent batch invocations. All of these
change observable delivery behavior and are therefore desired fields, not
host tuning (decision 0020).

## Why this is one Form

Retry, dead-lettering, and batch shape decide what an application observes
under failure; they must travel with the consumer edge itself.

## What would require a separate Form

Pull-based consumption through a client API, or push consumption with
per-message acknowledgement semantics, are different shapes.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

Attaching to a worker whose versions do not declare the `queue` handler must
fail closed. A dead-letter queue must be a distinct queue; hosts must reject
self-dead-lettering. Deleting the referenced queue or worker while attached
must fail with `dependency_in_use`.

## Prior art

The queue-consumer wiring of a proven edge platform, with its retry and
dead-letter semantics made explicit desired state.
