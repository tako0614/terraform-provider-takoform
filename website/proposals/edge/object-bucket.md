# Retained Provider 2.1.1 history — ObjectBucket

This page records the design that shipped in the retained
`edge.forms.takoform.com/v1beta1` package set and Provider 2.1.1 as Terraform
resource type `takoform_edge_object_bucket`. It is not a current proposal or
current Form. The versionless Edge candidate has no `ObjectBucket`, no
`edge.objects` Interface, no `module-worker.object-bucket` Binding, and no
Provider 3 authoring identity for one.

The exact retained Definition and package bytes, not this prose, are the
historical authority. They remain under
`forms/candidates/edge/v1beta1/object-bucket` and in the append-only Provider
identity ledger. Nothing in Specification 1.1 re-identifies or republishes
them.

## Historical workload and consumer

The retained Form modeled a flat-namespace object store for uploads, media, and
exports. A retained Worker Version consumed it through the exact
`module-worker.object-bucket` Binding.

## Historical role and semantics

Its role was `identity`. The bucket had no desired fields; its semantics were
fixed by the retained `edge.objects@1.0.0` Interface. That contract described
head/get/put/delete/list plus four multipart operations, strong
read-after-write consistency, last-writer-wins per key, strong etags with
conditional reads and writes, and cursor pagination with delimiter roll-up.

Bodies streamed beside operation documents. `get` and `put` carried
`bodyStream` and `contentLength`; ranged and conditional reads were typed
inputs of `get`. Objects above `maxSinglePutBytes`, or of unknown size, used
`createMultipartUpload`, `uploadPart`, `completeMultipartUpload`, and
`abortMultipartUpload` (decision 0020).

The original separate-Form boundary treated CORS, lifecycle expiry, retention
lock, eventual consistency, and object versioning as different contracts. A
live retained binding also made deletion fail with `dependency_in_use`.

These statements describe the retained v1beta1 identity only. They do not
define a current object-resource lifecycle or certify an external object
service.

## Current object-service boundary

Current worker code may request an externally managed S3-compatible service
through a sealed `externalServices` slot. The slot carries
`standards.takoform.com/v1` and an opaque reverse-DNS protocol identifier such
as `com.amazonaws.s3`. Takoform does not enumerate protocol identifiers,
provision a bucket Resource, expose endpoints or credentials in portable
state, or certify that a service conforms to the named protocol. The Host owns
support lookup and sealed runtime projection.

That call-only standard-service boundary is intentionally not a replacement
`ObjectBucket` Form. A future portable object Resource would require its own
explicit Form decision and identity; it cannot inherit this retained one.
