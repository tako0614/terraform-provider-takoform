# Provider v2 to v3 migration boundary

The next provider release published from this repository MUST be a major,
`3.0.0`. Provider `v2.1.1` was published carrying 24 resource types: the 15
v1beta1 Edge Platform Family resources and the nine v1alpha2 resources
(`takoform_edge_worker`, `takoform_relational_database`,
`takoform_object_bucket`, `takoform_key_value_store`, `takoform_queue`,
`takoform_schedule`, `takoform_container_service`, `takoform_stateful_entity`,
`takoform_vector_index`). The v1alpha2 epoch was withdrawn
([decision 0042](../../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md)),
so the provider built from this repository exposes only the 15 Family
resources. Removing a resource type is a breaking schema change; SemVer makes
that a major, and this document is the migration contract that release must
ship with.

## The nine have no successors

No v1beta1 resource replaces a v1alpha2 resource. The Edge `ObjectBucket`
deliberately registers as `takoform_edge_object_bucket` so the withdrawn
`takoform_object_bucket` resource type is never reoccupied with a different
contract ([decision 0030](../../spec/decisions/0030-a-form-line-moves-a-terraform-resource-type-may-not.md)).
There is no automatic state migration, import mapping, or renamed successor:
the two epochs' Forms are different contracts, and pretending otherwise would
claim a portable migration that does not exist.

## What existing state does

An operator holding state for any of the nine chooses one of three explicit
paths:

1. **Stay pinned.** `version = "= 2.1.1"` (or `= 2.0.0`) keeps working
   against a host that still serves the v1alpha2 lane. Published Registry
   versions are immutable; nothing removes them.
2. **Forget.** `terraform state rm` / `tofu state rm` removes the resource
   from state without touching the remote object. The host object survives
   and stops being managed.
3. **Destroy.** `terraform destroy -target` / `tofu destroy -target` while
   still pinned to v2, before upgrading. This is the only path that removes
   the remote object.

Upgrading to `3.0.0` with any of the nine still in state fails closed: the
resource type no longer exists in the provider, and both CLIs refuse the plan
before any lifecycle request is made. Nothing is silently dropped.

## Obligations the 3.0.0 release carries

- This document ships with the release and its notes name it.
- The signed Registry readback lane retired with the withdrawn epochs'
  tooling must be rebuilt for the 15-resource surface before the release's
  publication can be called Registry-verified
  ([decision 0042](../../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md));
  the package-publication step of [decision 0041](../../spec/decisions/0041-form-packages-publish-with-the-provider-release.md)
  is rebuilt the same way when the publication blockers clear.
- `release/version.json` stays at the published `2.1.1` descriptor until the
  owner release flow assigns `3.0.0`; a repository test holds the descriptor
  to be either the published version or a `3.x` value, so a `2.x` successor
  cannot be cut by habit.
