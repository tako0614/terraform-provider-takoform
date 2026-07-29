---
page_title: "takoform_container_service Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable OCI container service pinned to an immutable image digest.
---

# takoform_container_service

Portable OCI container service pinned to an immutable image digest.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_container_service/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `image` (String, required) — Immutable OCI image reference pinned by sha256 digest.
- `ports` (Set of Number, optional) — Container ports requested by the service. Between 1 and 65535.
- `public_http` (Bool, optional) — Whether this container asks for public HTTP exposure.
- `cpu_millicores` (Number, optional) — Optional CPU request in millicores. At least 1.
- `memory_mib` (Number, optional) — Optional memory request in mebibytes. At least 1.
- `replicas` (Number, optional) — Requested number of identical running instances. At least 1.
- `health_check_path` (String, optional) — Optional HTTP path a host polls to decide whether an instance is serving.
- `configuration` (Map of String, optional) — Non-secret configuration passed to the running service. Secret material is never portable state: a host injects it through its own credential path.
- `connections` (List of Object, optional) — Declared references to other Resources, each with `name`, `resource`, `permissions`, and `projection`. A connection is a request the host validates; it grants nothing by itself.
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

- `http.request@1` — Portable HTTP request surface exposed by a container service. Operations: `request`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_container_service.example NAME
terraform import takoform_container_service.example SPACE/NAME
```
