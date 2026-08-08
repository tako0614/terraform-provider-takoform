# ObjectBucket — `takoform_edge_object_bucket`

The shipped resource type is the transitional name
`takoform_edge_object_bucket`: the retained provider-v2 lane still owns
`takoform_object_bucket` while both lanes are co-registered in one provider
binary. A future provider major that drops the retained v2 lane reclaims
`takoform_object_bucket` for this Form.

## Workload and consumer

A worker stores and serves larger immutable-ish payloads — uploads, media,
exports — in a flat-namespace object store. Workers consume it through
`module-worker.object-bucket` bindings.

## Role

`identity`. The bucket has no desired fields; its semantics are entirely
fixed by the `edge.objects` Interface. There is no versioning field: object
versioning changes observable read semantics and would be a different Form.

## Observable semantics

Exactly the `edge.objects@1.0.0` contract: head/get/put/delete/list plus the
four multipart operations, strong read-after-write consistency (a get, head,
or list after a resolved put or delete observes it), last-writer-wins per
key, strong etags with conditional reads and writes (`precondition_failed` on
mismatch), and cursor pagination with delimiter roll-up.

Bodies STREAM. `get` and `put` declare `bodyStream` and `contentLength`, and
the bytes travel beside the operation document rather than inside it, which
is what makes the 5 GiB object ceiling meaningful at all. Ranged and
conditional reads are typed inputs of `get` — not prose — and objects above
`maxSinglePutBytes`, or of unknown size, are written through
`createMultipartUpload` / `uploadPart` / `completeMultipartUpload` /
`abortMultipartUpload` (decision 0020).

## Why this is one Form

Consumers rely on read-after-write visibility and etag fencing as one
inseparable model; the storage identity and that model must not drift apart.

## What would require a separate Form

Operating rules — CORS, lifecycle expiry, retention lock — are separate
policy resources in the family plan. An eventually consistent object store
or a versioned bucket is a different Form.

## Provided Interfaces

`edge.objects@1.0.0`.

## Accepted Bindings

None; it is a binding target (`module-worker.object-bucket`).

## Lifecycle risks

Deleting a bucket bound by any Worker Version must fail with
`dependency_in_use`. Delete destroys all objects. S3-compatible external
access is adapter material, never a desired field
([spec/portability-boundary.md](../../spec/portability-boundary.md)).

## Prior art

The strongly consistent object storage of a proven edge platform. The
retained v1alpha2 `ObjectBucket` candidate is prior art; its `versioning`
field is deliberately not carried forward.
