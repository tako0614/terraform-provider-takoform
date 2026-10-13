---
layout: home

hero:
  name: Takoform Provider
  text: Manage exact Forms from HCL.
  tagline: Typed Terraform and OpenTofu resources for a compatible Takoform Host.
  actions:
    - theme: brand
      text: Get started with 4.1.0
      link: /docs/getting-started.html
    - theme: alt
      text: Evaluate Provider 5
      link: /docs/provider-host-api-v2-source.html
---

Takoform Provider connects ordinary Terraform/OpenTofu configuration to exact
Form contracts served by your Host. Use it beside other Providers and connect
resources through the HCL dependency graph.

| Use | Provider | Host API | Resources |
| --- | --- | --- | --- |
| Registry installation | **4.1.0** | `forms.takoform.com/v1` | 17 exact Edge Forms |
| Source evaluation | **5.0.0** | `forms.takoform.com/v2` | 13 exact Edge Form URLs |

The Host supplies the endpoint, Space, authentication, and implementation.
Start with the [Random + Takoform guide](/docs/getting-started.html); Random
needs no cloud credentials. See [Versions and compatibility](/docs/versions.html)
before changing a Provider pin. Provider 4 state is not automatically
translated to Provider 5.

The [Provider reference](/docs/reference.html) documents the published v4
resources. The [Provider 5 consumer reference](/docs/provider-host-api-v2-source.html)
documents the source-v5 schema, private intent store, and lifecycle. Form
publication belongs to its publisher; Host support must be verified separately.
For the provider-neutral contract, read the [Host API documentation](https://takoform.com/en/host-api/).
