---
page_title: "takoform_stateful_entity Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable namespace of addressable persistent entities implemented by digest-bound application bytes.
---

# takoform_stateful_entity

Portable namespace of addressable persistent entities implemented by digest-bound application bytes.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_stateful_entity/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `artifact_url` (String, required) — Absolute credential-free HTTPS location any conforming host can fetch; userinfo, query, and fragment are forbidden because this value persists in nonsensitive state.
- `artifact_sha256` (String, required) — Digest binding the URL to exact immutable bytes.
- `artifact_media_type` (String, required) — Lowercase type/subtype describing how the bytes are interpreted.
- `entrypoint` (String, required) — Artifact entrypoint implementing entity behavior.
- `runtime` (String, required) — Open runtime capability token required by the artifact.
- `runtime_version` (String, optional) — Optional runtime compatibility version required by the artifact.
- `persistence` (String, required) — Open persistence capability token required by the workload; transactional means invocations for one entity observe serializable state transitions committed atomically with that invocation.
- `configuration` (Map of String, optional) — Non-secret application configuration. Credentials remain host-owned.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
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

- `entity.invoke@1` — Portable stateful entity invocation operations. Operations: `invoke`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_stateful_entity.example NAME
terraform import takoform_stateful_entity.example SPACE/NAME
```
