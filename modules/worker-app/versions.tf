terraform {
  required_version = ">= 1.8.0"

  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # provider v2.1.1 is an unpublished source candidate; build the provider from source.
      version = ">= 2.1.1"
    }
  }
}
