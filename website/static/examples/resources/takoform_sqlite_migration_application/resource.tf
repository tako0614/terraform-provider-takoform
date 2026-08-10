terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # stable v2.1.0 release target; descriptor remains candidate-only until owner publication.
      version = "= 2.1.0"
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
