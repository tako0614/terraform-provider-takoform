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

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `tls.certificate@1` — Portable managed certificate status operations. Operations: `status`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_tls_certificate.example NAME
terraform import takoform_tls_certificate.example SPACE/NAME
```
