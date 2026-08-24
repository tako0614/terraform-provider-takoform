terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative official-provider metadata. The
      # current versionless Form and its digest do not contain this name.
      # Provider 2.1.1 carries only retained versioned history; use a provider
      # release whose exact-Form registry includes this current identity.
      version = ">= 3.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_sqlite_migration_set" "example" {
  revision_owner  = "module-worker"
  manifest_digest = "sha256:344f4d30e8843b60598889c142ad26a7a9958ead6d26f50d787cd0fc32b02338"
}

output "sqlite_migration_set_outputs" {
  value = takoform_sqlite_migration_set.example.outputs_json
}
