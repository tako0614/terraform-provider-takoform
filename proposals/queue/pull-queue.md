# PullQueue — `takoform_pull_queue`

## Workload and consumer

A team decouples producers from consumers with a queue the consumers DRAIN:
anything that can call the `queue.pull` Interface receives messages, does
the work, and deletes what it finished. Lifecycle is managed through the
provider from birth. Edge workers get a producer binding later (Accepted
Bindings below); runtime consumption from non-worker compute families
awaits the cross-family projection realization and is not invented here.

## Role

`identity`. Retention, visibility, dead-lettering, and the long-poll bound
are the queue's small desired surface; everything else is fixed by the
`queue.pull@1.0.0` contract.

## Observable semantics

Exactly the `queue.pull@1.0.0` contract: send, receive, delete, and
changeVisibility against one unordered at-least-once queue. Delivery is
at-least-once with no ordering guarantee; consumers must be idempotent.

A message body is a UTF-8 string or the common canonical
`{"encoding":"base64","data":"..."}` bytes object, plus at most 10 string
attributes (the vocabulary the topic family's filter policy matches over);
body and attributes together are bounded at 262144 bytes. `send` accepts
one message and returns a message identity that is stable across
redeliveries; the acceptance timestamp never moves.

`receive` returns up to 10 messages, each with its identity, body,
attributes, acceptance timestamp, per-message receive count (1 on first
delivery, incremented on each), and a fresh receipt handle. A received
message becomes invisible for the visibility timeout — the call's own
`visibilityTimeoutSeconds` when stated, otherwise the queue's
`defaultVisibilityTimeoutSeconds` — and is redelivered after it unless
deleted. `waitSeconds` (0 up to `receiveWaitBoundSeconds`; absent means 0)
long-polls: the call returns as soon as messages are deliverable or the
wait elapses, and an empty result is a normal response, not an error.

Receipt handles: each receive of a message invalidates every earlier
handle for it; the newest handle stays valid until the message is next
received, deleted, or expires. `delete` and `changeVisibility` with an
invalidated or unknown handle fail with the Interface's stated stale-handle
error and have no effect — a stated error, not a silent no-op, because
"settled" and "will be redelivered" must be distinguishable. A delete with
the newest handle removes the message permanently, even after its
visibility timeout has lapsed, so long as it was not received again.
`changeVisibility` sets the remaining invisibility to 0..43200 seconds from
now; 0 makes the message immediately deliverable.

Desired fields: `messageRetentionSeconds` (60..1209600) bounds how long an
undeleted message survives. `defaultVisibilityTimeoutSeconds` (0..43200)
and `receiveWaitBoundSeconds` (0..20) are the defaults and bounds above.
`deadLetter` is an optional closed pair `{queue, maxReceiveCount}`
(1..1000): a receive that would raise a message's receive count past
`maxReceiveCount` instead moves it to the named queue as a new message
with its own identity, its own acceptance timestamp, and a receive count
starting again — mirroring the edge QueueConsumer's dead-letter rule.
Without the pair, only deletion or retention expiry removes a message.
`queue` is the closed exact `{apiVersion, kind, name}` reference to
another PullQueue, uid-pinned under the v1beta1 relation rules; a host
rejects self-dead-lettering and any cycle through the dead-letter graph,
because the move resets the receive count and nothing else bounds the loop
([decision 0026](../../spec/decisions/0026-attachment-claims-are-canonical-and-acyclic.md)).

## Why this is one Form

Receive count, handle invalidation, and redelivery-on-timeout are what
consumer code is written against. There is deliberately no ordering or
delivery-mode field: either would change consumer correctness requirements
invisibly (decision 0008).

## What would require a separate Form

A FIFO queue, an exactly-once-effect queue, a task queue with per-message
delivery targets, and the edge family's push-delivery AtLeastOnceQueue are
each a different delivery shape and a different Form.

## Provided Interfaces

`queue.pull@1.0.0` (Interface candidate to be authored with the family's
candidate set).

## Accepted Bindings

None. It is the intended target of a future
`module-worker.queue-pull-producer` binding projecting the send half into a
Worker Version; a binding instance's `resource` reference carries an
explicit `apiVersion`, so a cross-family target is representable as-is
([binding contract](../../spec/binding-contract/README.md)). Non-worker
consumption awaits the cross-family projection realization.

## Lifecycle risks

Deleting a queue referenced as another queue's dead-letter target, as a
TopicSubscription target or dead-letter, or as a Schedule target must fail
with `dependency_in_use`. Retained messages are destroyed with the queue.

## Prior art

The visibility-timeout pull queue offered by every major provider — the
"Pull queue" survey row decision 0043 minted this family from. The
withdrawn v1alpha2 `Queue` kind, whose open `ordering` enum this Form
replaces by fixing one shape. The Edge family's push AtLeastOnceQueue for
contrast: there the host invokes a worker's `queue` handler with batches;
here consumers pull with visibility semantics — two proven shapes, two
Forms (decision 0043).
