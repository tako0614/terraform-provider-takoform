---
page_title: "takoform_stateful_actor_namespace Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares a portable stateful actor namespace.
---

# takoform_stateful_actor_namespace

Declares a namespace whose runtime class owns stateful actor behavior. Individual actor instances are runtime identities, not Terraform resources. See the [complete example](../../examples/resources/takoform_stateful_actor_namespace/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Namespace name.
- `class_name` (String, required) — Runtime class identifier.
- `storage_profile` (String, optional) — Storage capability token; defaults to `durable_sqlite`.
- `migration_tag` (String, optional) — Namespace migration tag.
- `connections` (List of Object, optional) — Non-secret resource connections with `name`, `resource`, `permissions`, and `projection`.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `portability`, and `outputs` report the canonical
resource fence and sanitized public host results. Backend placement is not
provider state.
