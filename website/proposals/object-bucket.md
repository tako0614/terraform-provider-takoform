# Form Proposal: ObjectBucket

Status: active Proposal; intended first v1alpha2 release `0.1.0`. A local
candidate package exists under `forms/candidates/v1alpha2/`, but no released
FormRef or public release identity exists yet.

## Need and boundary

Takosumi Cloud provides durable object storage. The candidate Form describes a
bucket identity, version-retention semantics, lifecycle, and
`object.storage@1` Interface. Region, storage class, replication, physical
store, capacity, credentials, endpoint/protocol compatibility, policy, and
pricing remain host-owned.

## Substrate-neutrality review

Version retention and object operations can have the same meaning over S3,
R2, GCS, a filesystem-backed host, or another object store. `s3_api`, storage
classes, endpoint shapes, region, and replication are excluded because they
belong to an adapter, support profile, or Service Offering.

## Lifecycle and security risks

Location or storage-policy changes can require migration. Deleting a non-empty
bucket risks irreversible loss and must follow explicit host retention policy.
Import requires exact ownership evidence. Keys, signed URLs, object contents,
and credentials never enter portable Resource state.

## Prior art and gap

OCCI Storage/Link, CIMI Volume lifecycle, TOSCA object-storage nodes,
Kubernetes ObjectBucketClaim, Crossplane buckets, and Terraform S3/R2/GCS/
Azure resources are applicable. The gap is a backend-neutral bucket lifecycle
paired with an explicit object Interface and no credential transport.
