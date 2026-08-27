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
contracts exposed by a compatible Host. The current release is Provider
**`3.0.0`** with API/Core **`v1.0.1`** on `forms.takoform.com/v1`.

## Install and use

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 3.0.0"
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

| Family | Current Provider 3 mappings |
| --- | ---: |
| Edge | 16 |
| Function | 4 |
| Container | 5 |
| Queue | 1 |
| Schedule | 1 |
| Table | 1 |
| Topic | 2 |
| Vector | 1 |

Read the [Provider reference](/docs/), [Provider mapping inventory](/forms/), and
[conformance evidence](/conformance/) for generated contracts and executable
checks. See [Versions and compatibility](/docs/versions.html) for retained
releases and migration.
