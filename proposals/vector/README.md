# Vector Family proposals

The Vector Family, `vector.forms.takoform.com/v1beta1`, fixes the
fixed-dimension dense vector index shape — namespaced records queried by
top-k similarity — selected by
[decision 0043](../../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md).
Its member preserves, completely, the application-visible semantics of that
proven shape without naming its vendor
([decision 0008](../../spec/decisions/0008-forms-preserve-service-shape.md)).

## Why a family, not an integration

A category with a de-facto standard API is never respecified as a Form;
Takoform reaches it through a sealed standard-service slot instead
([spec/standard-services](../../spec/standard-services/README.md)). The
vector index has no such standard: every major cloud and several
independents offer one, each speaking only its own API, which is exactly the
category decision 0043 assigns to a Form Family — the place where a
host-neutral contract is the only portability there is. Search engines with
the Elasticsearch-compatible API sit on the integrate side of that
decision's survey and are not this family. This family replaces the
withdrawn v1alpha2 `VectorIndex`.

## Authoring policy: shape-preserving contracts

Every member Form preserves one service shape end to end: client API, data
model, consistency, update and delete units, error semantics, and the
capabilities exposed through typed Bindings. No free semantic token is
admitted; a difference in semantics is a different Form, never a selector
value. Outward capability use is a digest-bound Binding held by a revision
resource; inward activation is an attachment resource
([decision 0010](../../spec/decisions/0010-exact-interface-and-binding-contracts.md)).
Desired schemas carry no `name` or envelope plumbing: the resource envelope
owns identity and status
([decision 0011](../../spec/decisions/0011-resource-identity-generation-and-revision.md)).

A Form exists only when its proposal, catalog declaration, and candidate
package exist ([spec/form-families.md](../../spec/form-families.md)); this
directory carries the proposals, and the `vector.index` Interface candidate
joins `interfaces/candidates/v1alpha1` with the implementation. Members
enter as `0.1.0` Experimental; the family channel grants no maturity.

## MVP members

| Form | Role | One-line semantics | Separate-Form boundary |
| --- | --- | --- | --- |
| [VectorIndex](vector-index.md) | identity | Fixed-dimension dense vector index: metric fixed at creation, namespaced upsert/fetch/query/delete by id, closed metadata filter, approximate top-k similarity. | A sparse or hybrid keyword index, an exact-kNN guarantee, or a free filter language is a different Form. |

The family deliberately holds ONE Form. `dimension` and `metric` are the
identity's only desired fields because changing either means re-embedding
the corpus — they are the identity, not configuration — and namespaces are
runtime data reached through the Interface, never Resources. Hybrid search
or a metadata-schema surface would be separate proposals when that work
starts, per [spec/form-families.md](../../spec/form-families.md).
