terraform {
  required_version = ">= 1.8.0"

  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # The apply_idempotency_key passthrough requires Provider 3.1.0+.
      version = ">= 3.1.0"
    }
  }
}
