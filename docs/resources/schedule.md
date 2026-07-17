---
page_title: "takoform_schedule Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares a portable cron-triggered invocation.
---

# takoform_schedule

Declares a portable cron schedule. See the [complete example](../../examples/resources/takoform_schedule/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `cron` (String, required) — Portable five-field cron expression.
- `timezone` (String, optional) — Timezone capability token; defaults to `UTC`.
- `connections` (List of Object, required) — Must contain exactly one invocation connection with `name`, `resource`, `permissions`, and `projection`.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `portability`, and `outputs` report the canonical
resource fence and sanitized public host results. Backend placement is not
provider state.
