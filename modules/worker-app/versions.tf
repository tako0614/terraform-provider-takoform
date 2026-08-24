terraform {
  required_version = ">= 1.8.0"

  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # The current module targets the stable Host API v1 Provider 3 line.
      version = ">= 3.0.0"
    }
  }
}
