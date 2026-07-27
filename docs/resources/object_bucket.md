---
page_title: "takoform_object_bucket Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable object storage with a portable default storage class.
---

# takoform_object_bucket

Portable object storage with a portable default storage class.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_object_bucket/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `storage_class` (String, optional) — Portable default storage class for newly written objects. One of `standard`, `infrequent_access`, `archive`. Defaults to `standard`.
- `versioning` (Bool, optional) — Whether the bucket should retain non-current object versions.
- `access_protocols` (Set of String, optional) — Optional access-protocol capability tokens requested from the host.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `object.storage@1` — Portable object storage operations. Operations: `delete`, `get`, `list`, `put`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_object_bucket.example NAME
terraform import takoform_object_bucket.example SPACE/NAME
```
