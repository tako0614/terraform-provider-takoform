---
page_title: "takoform_tls_certificate Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable managed TLS certificate for a fixed set of domains. Key material stays with the host.
---

# takoform_tls_certificate

Portable managed TLS certificate for a fixed set of domains. Key material stays with the host.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_tls_certificate/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `domains` (Set of String, required, forces replacement) — Domains the certificate covers. Changing them replaces the certificate.
- `key_algorithm` (String, optional) — Requested certificate algorithm. The host generates and holds the key material. One of `ecdsa_p256`, `ecdsa_p384`, `rsa_2048`, `rsa_4096`. Defaults to `ecdsa_p256`.
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

- `tls.certificate@1` — Portable managed certificate status operations. Operations: `status`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_tls_certificate.example NAME
terraform import takoform_tls_certificate.example SPACE/NAME
```
