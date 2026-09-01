terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative Provider metadata. The current
      # exact FormRef and digest do not contain this name.
      # Provider 3's broader aggregate remains retained history. The next
      # major registers only the tako0614 Edge Form set.
      version = "~> 4.0"
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
