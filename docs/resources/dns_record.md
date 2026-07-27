---
page_title: "takoform_dns_record Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable DNS record published into one connected zone.
---

# takoform_dns_record

Portable DNS record published into one connected zone.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_dns_record/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `record_name` (String, required) — Record name relative to the connected zone.
- `record_type` (String, required, forces replacement) — Record type. Changing it replaces the record. One of `A`, `AAAA`, `CNAME`, `TXT`, `MX`, `SRV`, `CAA`, `NS`.
- `values` (Set of String, required) — Record data published for this name.
- `ttl_seconds` (Number, optional) — Optional record time to live in seconds. At least 1.
- `connections` (List of Object, required) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
- `space` (String, optional, forces replacement) — Overrides the provider default.

## Read-only attributes

`id`, `resource_version`, `drift_status`, `portability`, and `outputs` report
the canonical resource identity, its generation fence, the native observation
result, and sanitized public host results. Backend placement is never provider
state.

## Import

```console
terraform import takoform_dns_record.example NAME
terraform import takoform_dns_record.example SPACE/NAME
```
