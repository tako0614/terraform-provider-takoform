---
page_title: "Provider: Takoform"
description: |-
  The Takoform provider manages ten portable, statically typed Service Forms through any conforming host.
---

# Takoform Provider

The Takoform provider translates typed Terraform/OpenTofu resources into the `forms.takoform.com/v1alpha1` Service Form API. The canonical provider address is `registry.terraform.io/tako0614/takoform`.

```hcl
terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
    }
  }
}

provider "takoform" {
  endpoint = "https://forms.example.com"
  space    = "prod"
}
```

## Provider arguments

- `endpoint` (String, optional) — Origin of a conforming Service Form host. Falls back to `TAKOFORM_ENDPOINT`; one of the two is required.
- `space` (String, optional) — Default space for resources. Falls back to `TAKOFORM_SPACE`.
- `token` (String, optional, sensitive) — Bearer token for the host. Falls back to `TAKOFORM_TOKEN`.
- `compatibility_fallback` (Boolean, optional) — Explicitly permits the
  historical unversioned `/v1` API only when discovery omits `endpoints.api`.
  Defaults to false and falls back to `TAKOFORM_COMPATIBILITY_FALLBACK`.

The endpoint must advertise `features.service_forms = true`, API version
`forms.takoform.com/v1alpha1`, the versioned endpoint features, and exact
availability for each exact build-pinned candidate FormRef used by configuration. This
provider does not expose target-pool, backend, credential, pricing, billing,
quota, account, or operator-policy resources.

## Import

Every resource supports `terraform import ADDRESS NAME` and `terraform import ADDRESS SPACE/NAME`. The latter form records the resource space explicitly.
