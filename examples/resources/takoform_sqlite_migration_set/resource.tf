terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # Provider v2.1.1 is Registry-published; release/version.json remains
      # candidate-only descriptor metadata after owner publication. v2.1.1
      # serves this resource type under the retained v1beta1 identities; the
      # v1beta2 identity this page documents ships with the next release
      # (decision 0046).
      version = "= 2.1.1"
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
