# Provider v2 to v3 migration boundary

Provider `3.0.0` is a breaking provider release. It exposes the 31 current
Experimental Forms from the eight versionless families on Host API
`forms.takoform.com/v1`; it does not promote or publish those Forms.

Provider `2.1.1` remains immutable Registry history. It carried 15
`edge.forms.takoform.com/v1beta1` resource types plus the nine withdrawn
v1alpha2 resource types:

- `takoform_edge_worker`
- `takoform_relational_database`
- `takoform_object_bucket`
- `takoform_key_value_store`
- `takoform_queue`
- `takoform_schedule`
- `takoform_container_service`
- `takoform_stateful_entity`
- `takoform_vector_index`

The v1alpha2 epoch was withdrawn by
[decision 0042](../../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md).
Provider 3 does not register any of those nine names.

## Withdrawn names are never reoccupied

The current versionless families contain Forms whose English names resemble
three withdrawn v1alpha2 Forms, but their contracts and lifecycle identities
are different. Decision
[0030](../../spec/decisions/0030-a-form-line-moves-a-terraform-resource-type-may-not.md)
therefore gives them new Terraform resource types:

- `schedule.forms.takoform.com/Schedule` uses `takoform_message_schedule`;
- `container.forms.takoform.com/ContainerService` uses
  `takoform_serverless_container_service`; and
- `vector.forms.takoform.com/VectorIndex` uses
  `takoform_dense_vector_index`.

The withdrawn `takoform_schedule` is not an alias for either the current
message-delivery Schedule or the Edge `takoform_worker_cron_trigger`. There is
no automatic state migration, import mapping, or renamed successor for any of
the nine. Reusing a withdrawn name would make Terraform decode old state
through a different schema before a host lifecycle request, which Provider 3
must refuse rather than guess.

## Retained v2.1.1 Edge state

Provider 3 keeps exact state dispatch for the 14 retained v2.1.1 Edge resource
types that have a compatible current Terraform surface. Their state remains
bound to the exact historical FormRef recorded in it; it is not silently
rewritten to a current versionless FormRef.

Historical `takoform_edge_object_bucket` is the deliberate exception. Current
Takoform has no ObjectBucket Form or `edge.objects` Interface, and Provider 3
does not register an ObjectBucket resource or retained ObjectBucket codec.
S3-compatible storage is supplied out of band as an opaque standard service.
Existing ObjectBucket state must use one of the explicit paths below before a
Provider 3 upgrade.

## What removed-resource state does

An operator holding state for one of the nine v1alpha2 types or historical
`takoform_edge_object_bucket` chooses one of three explicit paths:

1. **Stay pinned.** `version = "= 2.1.1"` keeps the published provider bytes
   and their historical Host lane available.
2. **Forget.** `terraform state rm` / `tofu state rm` removes the entry from
   state without touching the remote object. The remote object survives and
   stops being managed by that state.
3. **Destroy.** Run `terraform destroy -target` / `tofu destroy -target` while
   still pinned to Provider 2.1.1 and while the historical Host lane is still
   available. This is the only generic path that asks that historical Host to
   remove the remote object.

Upgrading with any removed type still in state fails closed because Provider 3
does not register that Terraform resource type. The CLI refuses the plan
before Takoform lifecycle mutation; nothing is silently dropped or rebound.

## Provider 3 release obligations

- This document ships in the `v3.0.0` release notes.
- `release/provider-form-identities.json` binds the exact eight-family,
  31-Form projection and all provider-owned Terraform resource names.
- The immutable Provider 2.1.1 identity entry and Registry readback remain
  unchanged.
- The Provider 3 publication flow must produce its own signed candidate and
  direct Terraform/OpenTofu Registry readback for the 31-resource surface.
  A local dev override, worker-authoring matrix, or committed report is not
  Registry publication evidence.
- Publishing Provider 3 does not publish a Form Package, promote Form maturity,
  establish Host support, or make any Form commercially available.
