---
page_title: "takoform_identity_client Resource - takoform"
subcategory: "Service Forms"
description: |-
  Portable OIDC relying-party registration. Issued client material stays with the host.
---

# takoform_identity_client

Portable OIDC relying-party registration. Issued client material stays with the host.

The configured host selects and operates the concrete backend. This resource
carries desired state only: it never names a target, a credential, a price, or
an implementation. See the [complete example](../../examples/resources/takoform_identity_client/resource.tf).

## Arguments

- `name` (String, required, forces replacement) — Resource name.
- `redirect_uris` (Set of String, required) — Absolute https redirect URIs the client may return to.
- `auth_method` (String, optional) — How this authorization-code client authenticates at the token endpoint. The host issues and holds any material this implies. One of `none`, `client_secret_basic`, `private_key_jwt`. Defaults to `none`.
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

- `identity.oidc@1` — Portable OIDC relying-party metadata operations. Operations: `metadata`.

A declaration says what exists. It carries no credential and grants no
consumer access; the host creates the record and authorizes its use.

## Import

```console
terraform import takoform_identity_client.example NAME
terraform import takoform_identity_client.example SPACE/NAME
```
