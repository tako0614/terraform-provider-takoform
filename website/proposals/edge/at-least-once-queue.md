# AtLeastOnceQueue — `takoform_at_least_once_queue`

## Workload and consumer

A worker defers work — webhooks, fan-out, retries — by sending messages that
a consumer worker processes asynchronously. Producers use
`module-worker.queue-producer` bindings; consumption is the
[QueueConsumer](queue-consumer.md) attachment.

## Role

`identity`.

## Observable semantics

Exactly the `edge.queue@1.0.0` contract: send/sendBatch with at-least-once
delivery and no ordering guarantee. An accepted message is delivered one or
more times, possibly out of send order; consumers must be idempotent.
`messageRetentionSeconds` (60..1209600) bounds how long an undelivered
message survives; `deliveryDelaySeconds` (0..43200) defers deliverability.
Both are portable retention/delay semantics, not host capacity.

## Why this is one Form

The delivery guarantee is the contract consumers program against. There is
deliberately no ordering field: an ordering selector would change consumer
correctness requirements invisibly.

## What would require a separate Form

A FIFO queue, an exactly-once-effect queue, or a multi-consumer published
stream each change delivery or ordering semantics and are different Forms.

## Provided Interfaces

`edge.queue@1.0.0`.

## Accepted Bindings

None; it is a binding target (`module-worker.queue-producer`) and an
attachment target (QueueConsumer).

## Lifecycle risks

Deleting a queue with producer bindings or consumers must fail with
`dependency_in_use`. Retained messages are destroyed with the queue.

## Prior art

The at-least-once queue of a proven edge platform. The retained v1alpha2
`Queue` candidate, whose open `ordering` enum this Form replaces by fixing
one delivery shape.
