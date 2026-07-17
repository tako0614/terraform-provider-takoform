---
page_title: "takoform_queue Resource - takoform"
subcategory: "Service Forms"
description: |-
  Declares a portable message queue.
---

# takoform_queue

Declares a portable message queue. See the [complete example](../../examples/resources/takoform_queue/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `max_retries` (Number, optional) — Delivery retry preference.
- `max_batch_size` (Number, optional) — Consumer batch-size preference.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `portability`, and `outputs` report the canonical
resource fence and sanitized public host results. Backend placement is not
provider state.
