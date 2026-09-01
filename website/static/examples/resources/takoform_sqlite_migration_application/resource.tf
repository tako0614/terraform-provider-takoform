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

resource "takoform_sqlite_migration_application" "example" {
  name          = "sqlite-migration-application"
  database      = "sqlite-database"
  migration_set = "sqlite-migration-set"
}

output "sqlite_migration_application_outputs" {
  value = takoform_sqlite_migration_application.example.outputs_json
}
