terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 0.0.0-dev+35a77bdb37483fccbb365998923824040dad67cc"
    }
  }
}

provider "takoform" {}

resource "takoform_container_service" "api" {
  name              = "api"
  image             = "ghcr.io/takos/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  port              = 8080
  health_path       = "/healthz"
  workload_revision = "release-2026-09-28"
  environment = [
    { name = "APP_MODE", value = "production" }
  ]
  required_sensitive_vars = ["API_TOKEN"]
  outbound_internet       = false
}

resource "takoform_vector_index" "search" {
  name        = "search"
  dimension   = 1536
  metric      = "cosine"
  filter_keys = ["spaceId"]
}

resource "takoform_worker_version" "api" {
  name           = "api-v1"
  revision_owner = "module-worker"
  worker         = "module-worker"
  bundle         = "worker-bundle"
  handlers       = ["fetch"]
  vars_json      = jsonencode({ LOG_LEVEL = "info" })
  actor_bindings = []
  vector_bindings = [
    { name = "SEARCH", target_name = takoform_vector_index.search.name }
  ]
  container_http_bindings = [
    { name = "API", target_name = takoform_container_service.api.name }
  ]
}

resource "takoform_worker_deployment" "api" {
  name   = "api"
  worker = "module-worker"
  versions = [
    { worker_version = takoform_worker_version.api.name, weight = 10000 }
  ]
}
