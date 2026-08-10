# Form Proposal: VectorIndex

Status: active Proposal; intended first v1alpha2 release `0.1.0`. A local
candidate package exists under `forms/candidates/v1alpha2/`, but no released
FormRef or public release identity exists yet.

## Need and boundary

The candidate Form describes vector indexing with fixed dimension and metric
compatibility, lifecycle identity, and a vector-index Interface. Engine,
placement, capacity, replication, credentials, data plane,
and price remain host-owned.

## Substrate-neutrality review

Dimensions and similarity metric determine whether vectors and queries are
semantically compatible across managed vector products, search engines, and
self-hosted indexes. index size, shard count, region, replication, quantization,
endpoint protocol, and pricing tier remain host/profile concerns.

## Lifecycle and security risks

Dimension or incompatible metric changes require rebuild or replacement.
Delete/rebuild can discard indexed vectors unless export/recovery exists.
Import requires exact dimension, metric, namespace, and ownership evidence.
Vectors, metadata, endpoints, and credentials never enter portable state.

## Prior art and gap

OCCI generic Resource/Link, TOSCA data-service capabilities, managed vector
database operators/Crossplane resources, and Terraform Pinecone/search/
Vectorize resources are applicable; CIMI has no vector-search contract. The
gap is a minimal engine-neutral compatibility contract without owning scaling,
credential, or commercial behavior.
