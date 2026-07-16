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

`id`, `selected_implementation`, `target`, `locked`, `portability`, and `outputs` report sanitized host results.
