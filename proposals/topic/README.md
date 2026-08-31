# Fanout Topic Family proposals

::: warning Historical / deferred candidate family

`topic.forms.takoform.com` is retained source from the earlier Provider
projection. It is not part of the current official corpus (the single Edge
family with 16 Forms), and this family is outside Current navigation. These
pages are English-only historical/deferred proposals.

:::

The Fanout Topic Family, `topic.forms.takoform.com`, is minted under
[decision 0043](../../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md):
pub/sub fanout is popularly offered as a managed service by every major
provider and has no de-facto standard API, so a host-neutral contract is the
only portability there is. Its members fix, completely, the
application-visible semantics of the proven fanout-topic shape — publish
once, every subscription gets its own copy — without naming its vendor
([decision 0008](../../spec/decisions/0008-forms-preserve-service-shape.md)).

## Authoring policy: shape-preserving contracts

Every member Form preserves one service shape end to end: client API, data
model, delivery guarantees, update and delete units, error semantics, and
the capabilities exposed through typed Bindings. No free semantic token is
admitted; a difference in semantics is a different Form, never a selector
value. Outward capability use is a digest-bound Binding held by a revision
resource; inward activation is an attachment resource
([decision 0010](../../spec/decisions/0010-exact-interface-and-binding-contracts.md)).
Desired schemas carry no `name` or envelope plumbing: the resource envelope
owns identity and status
([decision 0011](../../spec/decisions/0011-resource-identity-generation-and-revision.md)).

This prose accompanies the two generated Experimental candidates under
`forms/candidates/topic.forms.takoform.com` and the exact `topic.publish`
Interface. The generated candidate bytes are retained historical/deferred
source, not the current publisher identities; these documents reserve nothing.

## Retained candidate members (historical/deferred)

| Form | Role | One-line semantics | Separate-Form boundary |
| --- | --- | --- | --- |
| [Topic](topic.md) | identity | Fanout topic: an accepted publish is delivered, per-subscription at-least-once, to every matching subscription; nothing is retained or replayed. | A retained, replayable log or an ordered stream is a different Form. |
| [TopicSubscription](topic-subscription.md) | attachment | Subscribes one pull queue to one topic, with an optional closed attribute-equality filter policy, retry, and a dead-letter queue. | HTTPS push delivery to an author-named endpoint is a separate Form once the endpoint-authentication shape is fixed. |

## Cross-family composition

The MVP delivery target is a `PullQueue` from
[`queue.forms.takoform.com`](../queue/README.md). Relations and binding
instances carry an explicit `apiVersion` in their closed
`{apiVersion, kind, name}` reference shape, so a cross-family target is
representable without new machinery
([binding contract](../../spec/binding-contract/README.md)). Designs that
differ in these semantics — a retained stream, an event bus with routing
rules — belong to future families per
[spec/form-families.md](../../spec/form-families.md) and decision 0043's
candidate list.
