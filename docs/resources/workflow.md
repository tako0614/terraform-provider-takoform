---
page_title: "takoform_workflow Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable durable workflow definition and instance-state lifecycle.
---

# takoform_workflow

Portable durable workflow definition and instance-state lifecycle.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_workflow/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `artifact_url` (String, required) — Absolute credential-free HTTPS location any conforming host can fetch; userinfo, query, and fragment are forbidden because this value persists in nonsensitive state.
- `artifact_sha256` (String, required) — Digest binding the URL to exact immutable bytes.
- `artifact_media_type` (String, required) — Lowercase type/subtype describing how the bytes are interpreted.
- `entrypoint` (String, required) — Workflow runtime entrypoint.
- `max_attempts` (Number, optional) — Optional maximum attempts per workflow run. At least 1.
- `initial_backoff_seconds` (Number, optional) — Optional initial retry backoff in seconds. At least 0.
- `configuration` (Map of String, optional) — Non-secret configuration passed to the running service. Secret material is never portable state: a host injects it through its own credential path.
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

- `workflow.invoke@1` — Portable durable workflow invocation operations. Operations: `cancel`, `invoke`, `status`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_workflow.example NAME
terraform import takoform_workflow.example SPACE/NAME
```
