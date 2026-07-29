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
- `default_local_part` (String, optional) — Optional local part combined with domain to form the default sender mailbox.
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

- `email.send@1` — Portable outbound mail operations. Operations: `send`, `status`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_email_sender.example NAME
terraform import takoform_email_sender.example SPACE/NAME
```
