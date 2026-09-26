---
layout: home

hero:
  name: Takoform
  text: One typed provider. Any compatible host.
  tagline: Portable, host-neutral resource contracts for Terraform and OpenTofu.
  actions:
    - theme: brand
      text: Use the provider
      link: /docs/
    - theme: alt
      text: Read Core v1.0.1
      link: https://github.com/tako0614/takoform/tree/v1.0.1/spec
---

Takoform Provider maps typed Terraform/OpenTofu resources to exact Form
contracts exposed by a compatible Host. Registry Provider **`4.0.0`** keeps the
`tako0614/takoform` address and contains only 17 Forms selected from
`tako0614/takoform-forms`. Provider `3.0.0` remains immutable 31-resource
aggregate history. API/Core
**`v1.0.1`** stays on `forms.takoform.com/v1`.

## Install and use

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 4.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://forms.example.com"
  space    = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}
```

## Resource reference

| Publisher family | Provider 4 mappings |
| --- | ---: |
| Edge | 17 |

Read the [Provider reference](/docs/), [Provider mapping inventory](/forms/), and
[conformance evidence](/conformance/) for generated contracts and executable
checks. See [Versions and compatibility](/docs/versions.html) for retained
releases and migration. AWS, Cloudflare, Kubernetes, and other providers are
declared natively beside Takoform in the same OpenTofu module.
