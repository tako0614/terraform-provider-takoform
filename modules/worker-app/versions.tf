terraform {
  required_version = ">= 1.8.0"

  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # The apply_idempotency_key passthrough is part of the Provider 4 mapping.
      version = ">= 4.0.0"
    }
  }
}
