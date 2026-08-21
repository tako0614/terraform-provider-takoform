---
page_title: "takoform_key_value_store Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable key/value state with declared consistency and expiry semantics.
---

# takoform_key_value_store

Portable key/value state with declared consistency and expiry semantics.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](https://takoform.com/examples/resources/takoform_key_value_store/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `consistency` (String, optional) — Optional read consistency requirement: eventual permits stale reads; per_key_linearizable requires each key's completed writes and subsequent reads to appear in one real-time order. Omission states no portable consistency guarantee. One of `eventual`, `per_key_linearizable`.
- `default_ttl_seconds` (Number, optional) — Optional positive default entry lifetime in seconds; omission requests no default expiry requirement. At least 1.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`form_api_version`, `form_kind`, `form_definition_version`, `form_schema_digest`, and
`form_package_digest` bind state to the exact immutable Form identity.
`id` is the provider-synthesized `Kind/name` identity and `resource_version` is
the host generation fence. `drift_status`, `portability`, and `outputs` are written
only after the host's observed and output documents satisfy this exact Form's
closed schemas, identities, and generation. Undeclared host keys are rejected;
backend placement is never provider state.

## Declared runtime interfaces

- `keyvalue.store@1` — Portable key/value operations. Operations: `delete`, `get`, `list`, `put`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_key_value_store.example NAME
terraform import takoform_key_value_store.example SPACE/NAME
```
