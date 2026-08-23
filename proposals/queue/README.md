# Pull Queue Family proposals

The Pull Queue Family, `queue.forms.takoform.com/v1beta1`, is minted under
[decision 0043](../../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md):
the pull queue is popularly offered as a managed service by every major
provider and has no de-facto standard API, so a host-neutral contract is the
only portability there is. Its members fix, completely, the
application-visible semantics of the proven visibility-timeout pull queue
shape without naming its vendor
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

No catalog source, generated candidate set, Interface candidate, or Binding
candidate exists for this family yet. A Form exists only when its proposal,
catalog declaration, and candidate package exist
([proposals/README.md](../README.md)); these documents reserve nothing.

## MVP members

| Form | Role | One-line semantics | Separate-Form boundary |
| --- | --- | --- | --- |
| [PullQueue](pull-queue.md) | identity | Unordered at-least-once queue drained by consumers calling receive, with visibility timeout, receive counting, dead-lettering, and long polling. | A FIFO or exactly-once queue, or push delivery into a worker handler, is a different Form. |

## Two queue Forms, deliberately

The Edge Platform Family's
[AtLeastOnceQueue](../edge/at-least-once-queue.md) is not this Form. There
the host pushes batches into a worker's `queue` handler and settlement is
per-batch; here consumers pull messages through an API and settle each one
by deleting it under a visibility timeout. Decision 0043 records why both
exist: two proven shapes, two contracts, per decision 0008. A `FifoQueue`
or a per-message-HTTP-target task queue would likewise be separate Forms
with their own proposals, per
[spec/form-families.md](../../spec/form-families.md).
