# Topic — `takoform_topic`

> Historical/deferred candidate (English-only). This non-Edge Form is not in
> the current official Edge16 corpus or Current navigation.

## Workload and consumer

A producer publishes one event — an order placed, a file arrived — and
every interested consumer receives its own copy through its own
subscription without producers knowing who listens. Lifecycle is managed
through the provider from birth. Edge workers get a producer binding later
(Accepted Bindings below); publishing from non-worker compute families
awaits the cross-family projection realization and is not invented here.

## Role

`identity`. The topic has no desired fields: its semantics are entirely
fixed by the `topic.publish` Interface and the
[TopicSubscription](topic-subscription.md) attachment.

## Observable semantics

Exactly the `topic.publish@1.0.0` contract: publish one message and fan it
out. A message body is a UTF-8 string or the common canonical
`{"encoding":"base64","data":"..."}` bytes object, plus at most 10 string
attributes; body and attributes together are bounded at 262144 bytes —
the same message shape the queue family carries, so a delivery fits its
target.

An accepted publish is evaluated against the subscription set as of
acceptance: every subscription that exists at that moment and whose filter
policy matches the message's attributes receives its own at-least-once
delivery. Deliveries are independent per subscription — one subscription's
failure, retry backlog, or dead-lettering never delays or affects
another. A subscription created after acceptance never receives the
message. The topic retains nothing and replays nothing: a publish with no
subscriptions, or none matching, succeeds and the message is gone. There
is no ordering guarantee across or within subscriptions, and at-least-once
means a subscription may receive one publish more than once; consumers
must be idempotent.

## Why this is one Form

Fanout-without-retention is the contract publishers and subscribers
program against. A retention or replay selector would change what a late
subscriber can rely on invisibly, and an ordering selector would change
consumer correctness requirements — exactly the free semantic tokens
decision 0008 prohibits.

## What would require a separate Form

A retained, replayable log or stream; an ordered topic; an event bus that
routes by rules rather than fanning out to subscriptions. Each changes the
delivery model and is a different Form (decision 0043 records `eventbus`
as a deferred family candidate).

## Provided Interfaces

`topic.publish@1.0.0` (Interface candidate to be authored with the
family's candidate set).

## Accepted Bindings

None. It is the intended target of a future `module-worker.topic`
producer binding projecting publish into a Worker Version; a binding
instance's `resource` reference carries an explicit `apiVersion`, so a
cross-family target is representable as-is
([binding contract](../../spec/binding-contract/README.md)).

## Lifecycle risks

Deleting a topic that any TopicSubscription references, or that a Schedule
names as its target, must fail with `dependency_in_use`. Because nothing
is retained, delete destroys no messages — only the fanout point.

## Prior art

The fanout topic offered by every major provider — the "Pub/sub fanout"
survey row decision 0043 minted this family from. The withdrawn v1alpha2
epoch had no topic kind (decision 0043's lineup table records no
predecessor); this family is new ground. The Edge family's push
AtLeastOnceQueue is the adjacent contrast: one consumer per queue there,
one delivery per subscription here.
