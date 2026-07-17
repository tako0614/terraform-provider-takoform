---
page_title: "takoform_durable_workflow Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares a portable durable workflow artifact.
---

# takoform_durable_workflow

Declares a portable durable workflow artifact. See the [complete example](../../examples/resources/takoform_durable_workflow/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- Exactly one of `artifact_path`, `artifact_url`, or `artifact_ref` (String) — Runner-local path, immutable HTTPS URL, or host-allocated immutable reference.
- `artifact_sha256` (String) — SHA-256 digest; required with `artifact_url` or `artifact_ref`.
- `entrypoint` (String, required) — Workflow runtime entrypoint.
- `max_attempts` (Number, optional) — Positive maximum attempt count.
- `initial_backoff_seconds` (Number, optional) — Non-negative initial retry backoff.
- `connections` (List of Object, optional) — Non-secret resource connections with `name`, `resource`, `permissions`, and `projection`.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `portability`, and `outputs` report the canonical
resource fence and sanitized public host results. Backend placement is not
provider state.
