terraform {
  required_version = ">= 1.8.0"

  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # Provider v2.1.1 is Registry-published; release/version.json remains
      # candidate-only descriptor metadata after owner publication.
      version = ">= 2.1.1"
    }
  }
}
