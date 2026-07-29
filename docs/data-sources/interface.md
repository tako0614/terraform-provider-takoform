---
page_title: "takoform_interface Data Source - takoform"
subcategory: "Service Forms"
description: |-
  Reads one exact runtime interface declaration.
---

# takoform_interface

Reads one runtime interface declaration from a conforming host. See the
[complete example](../../examples/data-sources/takoform_interface/data-source.tf).

## Arguments

- `name` (String, required) — declared interface name.
- `version` (String, optional) — exact author-defined version. Omit it only when
  the visible name has one version; multiple versions fail closed.
- `resource_kind` and `resource_name` (String, optional as a pair) — portable
  Resource instance exposing the descriptor. Omit both only when one visible
  Resource matches; multiple instances fail closed.
- `space` (String, optional) — Space to read from. An explicit value wins;
  otherwise the provider default is required. The read fails before contacting
  the host when neither supplies a non-empty Space.

## Read-only attributes

- `version` — exact resolved version (also usable as the optional selector).
- `resource_kind` / `resource_name` — exact resolved portable Resource reference
  (also usable as paired optional selectors).
- `document_json` — exact non-secret declaration document as JSON.
- `values_json` — resolved public values as JSON.
- `form_kind` — declaring Form kind when the host reports it.

This data source grants nothing. It never reads or creates bindings,
permissions, tokens, credentials, or lifecycle state. The descriptor comes
from a Form Definition; there is no portable `takoform_interface` resource or
standalone Interface write path. The host owns the resulting Interface record,
bindings, write fencing, authorization, and lifecycle. Host responses that
violate the portable data-only field policy are rejected before
`document_json` or `values_json` enters non-sensitive Terraform state. No host
Interface id is exposed. The host must advertise
`features.interface_declarations`.

The provider sends the effective Space on every Interface list or exact read
and stores that exact value in data source state. A host scopes visibility and
ambiguity checks to the requested Space; it cannot silently select another
one.
