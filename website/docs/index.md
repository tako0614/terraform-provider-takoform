# Use Takoform Provider

Manage exact Takoform Forms from Terraform or OpenTofu. A compatible Host
provides the endpoint, Space, authentication, and service implementation.

## Choose your version

| Use | Provider | Host API | Resource mappings |
| --- | --- | --- | --- |
| Registry installation | **4.1.0** | `forms.takoform.com/v1` | 17 Edge Forms |
| Source evaluation | **5.0.0** | `forms.takoform.com/v2` | 13 exact Edge Form URLs |

The [release identity ledger](https://github.com/tako0614/terraform-provider-takoform/blob/main/release/provider-release-identities.json)
records the verified 4.1.0 Registry publication. Check the exact live Registry
version before selecting 5.0.0; a source target or tag is not installation
evidence. **Provider 4 state is not automatically translated to Provider 5.**

## Start with published Provider 4.1.0

Follow [Getting started](/docs/getting-started.html) to initialize, validate,
and plan a [Random + Takoform module](/examples/getting-started/main.tf).
Random needs no cloud credentials; the Takoform resource is managed by the
selected Host. The guide explains the required inputs, token handling, lock
file, and exact EdgeKVNamespace 0.1.0 Form identity.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 4.1.0"
    }
  }
}
```

The guide supplies the Host configuration and resource declaration. For an
AWS resource alongside Takoform, see [native provider composition](/examples/native-provider-composition/main.tf).
Each Provider keeps its own source, credentials, and state.

## Evaluate Provider 5

Use the [Provider 5 consumer reference](/docs/provider-host-api-v2-source.html)
for the thirteen mappings, typed HCL, local or remote private-store custody,
import, and lifecycle boundaries. It requires a Host that supports
`forms.takoform.com/v2` and the exact Form URLs, plus one private intent-store
mode. Host support and Runner private-file access are supplied by the
execution environment.

## Resource reference {#resource-reference}

The [Provider reference](/docs/reference.html#resource-reference) and its
seventeen resource pages describe **Provider 4.1.0 / Host API v1**. Use the
Provider 5 reference above for source-v5 schemas and Form URLs.

For migration and release evidence, see [Versions and compatibility](/docs/versions.html),
the [v3-to-v4 migration guide](/release/migrations/v3-to-v4.html), and
[Conformance](/conformance/). For provider-neutral API contracts, see the
[Host API documentation](https://takoform.com/en/host-api/).
