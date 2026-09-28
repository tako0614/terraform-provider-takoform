terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 0.0.0-dev+35a77bdb37483fccbb365998923824040dad67cc"
    }
  }
}

provider "takoform" {}

resource "takoform_container_service" "overlap" {
  name              = "api"
  image             = "ghcr.io/takos/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  port              = 8080
  health_path       = "/healthz"
  workload_revision = "release-2026-09-28"
  environment = [
    { name = "APP_MODE", value = "production" }
  ]
  required_sensitive_vars = ["APP_MODE"]
}
