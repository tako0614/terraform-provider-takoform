terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.1"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_relational_database" "example" {
  name              = "relational-database"
  engine            = "postgres"
  engine_version    = "16"
  storage_gib       = 20
  size_class        = "db.small"
  database_name     = "app"
  high_availability = false
}

output "relational_database_outputs" {
  value = takoform_relational_database.example.outputs
}
