terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_sql_database" "main" {
  name            = "main"
  engine          = "sqlite"
  migrations_path = "migrations"
}

output "database_resource_version" {
  value = takoform_sql_database.main.resource_version
}

output "database_outputs" {
  value = takoform_sql_database.main.outputs
}
