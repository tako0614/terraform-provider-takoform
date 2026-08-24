# TopicSubscription — `takoform_topic_subscription`

## Workload and consumer

A team wires its queue to a topic: every message published to the topic
that matches the subscription's filter lands in the team's PullQueue, and
the team's consumers drain it at their own pace.

## Role

`attachment`. Subscription is activation of the fanout edge; deleting the
subscription detaches it and never deletes the topic or the target queue.

## Observable semantics

`topic` and `target` are immutable; changing either replaces the
attachment. `topic` references a [Topic](topic.md). `target` is the closed
exact `{apiVersion, kind, name}` reference to a resource providing
`queue.pull@1.0.0` — in the current candidate a `PullQueue` of
`queue.forms.takoform.com`. The reference carries its group
explicitly, so the cross-family edge is representable as-is
([binding contract](../../spec/binding-contract/index.md)); the relation
requires the Interface the target provides and is UID-pinned under the
Host API v1 relation rules. At most one subscription may bind one
(topic, target) pair; a second is refused — a union of two filter
policies therefore needs two targets.

Delivery: for each accepted publish that matches, the subscription sends
into the target queue. The delivered message is a new queue message with
the queue's own identity and receive count, carrying the published body
and attributes unchanged. Delivery is at-least-once per subscription, so
one publish may land in the queue twice; consumers must be idempotent.
A failed send is retried after `retryDelaySeconds` (0..3600); `maxRetries`
(0..100) counts retries only, so a message is attempted at most
`1 + maxRetries` times — mirroring the edge QueueConsumer's counting rule.
An exhausted message goes to `deadLetter` (a PullQueue reference of the
same exact shape, distinct from `target`) when declared and is dropped
otherwise; the dead-letter copy is a new message there with its own
identity, acceptance timestamp, and receive count.

`filterPolicy` is an optional closed map from attribute name to a
non-empty list of at most 16 candidate strings, over at most 10 named
attributes. A message matches when, for EVERY named attribute, the message
carries that attribute and its value equals one of the listed candidates —
exact, case-sensitive string equality/inclusion; attributes the policy
does not name are unconstrained; an absent policy matches every message.
There is no numeric range, prefix, negation, or content/JSON-path
filtering. A non-matching message is filtered: not delivered, not
dead-lettered, and final. `filterPolicy`, the retry fields, and
`deadLetter` update in place; a filter change applies to publishes
accepted after the change.

## Why this is one Form

Filter, retry, and dead-lettering decide what a subscriber observes under
load and failure; they must travel with the subscription edge itself, as
the edge QueueConsumer established for its consumer edge.

## What would require a separate Form

HTTPS push delivery to an author-named endpoint is recorded here and
deliberately deferred: it changes the security surface and needs a fixed
endpoint-authentication shape before it can be one complete contract. A
richer filter grammar (ranges, prefixes, negation, payload paths) changes
what "matches" means and is a different subscription Form or a new exact
Interface generation, never a widening of this closed grammar.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

Deleting the referenced topic, target queue, or dead-letter queue while
this subscription exists must fail with `dependency_in_use`. `deadLetter`
equal to `target` is refused: a delivery that failed into the target
cannot dead-letter into the same place. A target deleted and re-created
under the same name is a different incarnation: the subscription stays
pinned, reports `ExternalChange` or `DependencyMissing` per the Host API v1
relation rules, and only a re-apply re-pins it.

## Prior art

The per-subscription filtered fanout of every major provider's pub/sub
topic. The withdrawn v1alpha2 epoch had no subscription kind. The Edge
family's QueueConsumer for contrast: that attachment has the host invoke a
worker handler; this one feeds a pull queue and lets consumers come to the
messages.
