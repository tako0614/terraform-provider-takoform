---
page_title: "takoform_dns_zone Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable authoritative DNS zone for one domain.
---

# takoform_dns_zone

Portable authoritative DNS zone for one domain.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_dns_zone/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `domain` (String, required, forces replacement) — Domain this zone is authoritative for.
- `default_ttl_seconds` (Number, optional) — Optional default record time to live for this zone, in seconds. At least 1.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Declared runtime interfaces

- `dns.zone@1` — Portable authoritative DNS zone operations. Operations: `list`, `resolve`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_dns_zone.example NAME
terraform import takoform_dns_zone.example SPACE/NAME
```
