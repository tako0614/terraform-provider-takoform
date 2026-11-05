---
layout: home

hero:
  name: Takoform Provider
  text: Manage exact Forms from HCL.
  tagline: Typed Terraform and OpenTofu resources for a compatible Takoform Host.
  actions:
    - theme: brand
      text: Evaluate current v2 source
      link: /docs/provider-host-api-v2-source.html
    - theme: alt
      text: Legacy 4.1.0 guide
      link: /docs/getting-started.html
---

Takoform Provider connects ordinary Terraform/OpenTofu configuration to exact
Form contracts served by your Host. Use it beside other Providers and connect
resources through the HCL dependency graph.

| Use | Provider | Host API | Resources |
| --- | --- | --- | --- |
| Current source target / candidate | **5.0.0** | `forms.takoform.com/v2` | 13 exact Edge Form URLs |
| Deprecated Registry release | **4.1.0** | `forms.takoform.com/v1` | 17 exact Edge Forms |

The default source-built Provider uses v2 and does not support v1. Provider
5.0.0 is not proven Registry-published; check [Versions and
compatibility](/docs/versions.html) and exact-version readback before changing
a pin. Provider 4 state is not automatically translated to Provider 5. The
Host supplies its endpoint, Space, authentication, and implementation.

The [Provider 5 consumer reference](/docs/provider-host-api-v2-source.html)
documents the current source-v2 schema, private intent store, and lifecycle.
The [Provider reference](/docs/reference.html) documents deprecated v4
resources. Form publication belongs to its publisher; Host support must be verified separately.
For the provider-neutral contract, read the [Host API documentation](https://takoform.com/en/host-api/).
