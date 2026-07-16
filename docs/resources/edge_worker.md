---
page_title: "takoform_edge_worker Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares a portable prebuilt edge Worker artifact.
---

# takoform_edge_worker

Declares a prebuilt edge Worker artifact without selecting its concrete runtime.

See the [complete example](../../examples/resources/takoform_edge_worker/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- Exactly one of `artifact_path`, `artifact_url`, or `artifact_ref` (String) — Runner-local path, immutable HTTPS URL, or host-allocated immutable reference.
- `artifact_sha256` (String) — SHA-256 digest; required with `artifact_url` or `artifact_ref`.
- `compatibility_date` (String, optional) — Runtime compatibility date.
- `compatibility_flags` (Set of String, optional) — Host-supported compatibility tokens.
- `profiles` (Set of String, optional) — Host-supported Worker profile tokens.
- `connections` (List of Object, optional) — Non-secret resource connections with `name`, `resource`, `permissions`, and `projection`.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `selected_implementation`, `target`, `locked`, `portability`, and `outputs` report sanitized host results.
