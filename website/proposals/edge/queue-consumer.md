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
Batching is bounded by `maxBatchSize` (1..100) and `maxBatchTimeoutSeconds`
(0..60). A failed batch is redelivered after `retryDelaySeconds` up to
`maxRetries` times; exhausted messages go to `deadLetterQueue` when declared
and are dropped otherwise. `maxConcurrency` (1..250) bounds concurrent batch
invocations. All of these change observable delivery behavior and are
therefore desired fields, not host tuning.

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
