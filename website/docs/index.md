# Use Takoform Provider

Manage exact Takoform Forms from Terraform or OpenTofu. A compatible Host
provides the endpoint, Space, authentication, and service implementation.

## Choose your version

| Use | Provider | Host API | Resource mappings |
| --- | --- | --- | --- |
| Current source target / candidate | **5.0.0** | `forms.takoform.com/v2` | 13 exact Edge Form URLs |
| Deprecated Registry release | **4.1.0** | `forms.takoform.com/v1` | 17 Edge Forms |

The default source-built Provider uses Host API v2; the v1 client is deprecated
and unsupported by that entrypoint. The [release identity
ledger](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/provider-release-identities.json)
records the verified 4.1.0 Registry publication. Provider 5.0.0 is a source
target, not proven Registry-published: check the live listing and exact-version
readback before installation. **Provider 4 state is not automatically
translated to Provider 5.**

## Current path: evaluate Provider 5 source

Use the [Provider 5 consumer reference](/docs/provider-host-api-v2-source.html)
for the current v2 schema, thirteen mappings, local or remote private-store
custody, import, and lifecycle boundaries. The Host must independently support
`forms.takoform.com/v2` and each exact Form URL.

## Deprecated published Provider 4.1.0

The [legacy getting-started guide](/docs/getting-started.html) covers
Registry-published Provider 4.1.0 / Host API v1 for retained workspaces. This
release is unsupported by the default Provider 5 entrypoint.

## Resource reference {#resource-reference}

The [Provider reference](/docs/reference.html#resource-reference) and its
seventeen resource pages describe deprecated **Provider 4.1.0 / Host API v1**.
Use the Provider 5 reference above for the current source-v2 schema and Form URLs.

For migration and release evidence, see [Versions and compatibility](/docs/versions.html),
the [v3-to-v4 migration guide](/release/migrations/v3-to-v4.html), and
[Conformance](/conformance/). For provider-neutral API contracts, see the
[Host API documentation](https://takoform.com/en/host-api/).

For an AWS resource alongside Takoform, see [native provider composition](/examples/native-provider-composition/main.tf).
Each Provider keeps its own source, credentials, and state.
