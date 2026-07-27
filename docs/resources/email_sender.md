---
page_title: "takoform_email_sender Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable outbound mail identity for one verified domain.
---

# takoform_email_sender

Portable outbound mail identity for one verified domain.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_email_sender/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `domain` (String, required, forces replacement) — Domain the host verifies before it accepts outbound mail.
- `default_sender` (String, optional) — Optional default sender mailbox inside the verified domain.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `email.send@1` — Portable outbound mail operations. Operations: `send`, `status`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_email_sender.example NAME
terraform import takoform_email_sender.example SPACE/NAME
```
