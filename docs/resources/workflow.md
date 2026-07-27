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
- `artifact_path` / `artifact_url` / `artifact_ref` (String, optional) — Exactly one immutable artifact source. `artifact_url` and `artifact_ref` require `artifact_sha256`.
- `artifact_sha256` (String, optional) — Expected artifact digest.
- `entrypoint` (String, required) — Workflow runtime entrypoint.
- `max_attempts` (Number, optional) — Optional maximum attempts per workflow run. At least 1.
- `initial_backoff_seconds` (Number, optional) — Optional initial retry backoff in seconds. At least 0.
- `configuration` (Map of String, optional) — Non-secret configuration passed to the running service. Secret material is never portable state: a host injects it through its own credential path.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `workflow.invoke@1` — Portable durable workflow invocation operations. Operations: `cancel`, `invoke`, `status`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_workflow.example NAME
terraform import takoform_workflow.example SPACE/NAME
```
