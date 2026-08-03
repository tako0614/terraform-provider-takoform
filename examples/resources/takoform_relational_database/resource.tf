terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.3"
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
  schema_url        = "https://artifacts.portable-conformance.invalid/relational-schema.json"
  schema_sha256     = "1d2181e213a086ae9e025d235ff5e267c43ec60cf4fc2f966977a21f2a95ef7b"
  schema_format     = "takosumi.resource-migrations"
}

output "relational_database_outputs" {
  value = takoform_relational_database.example.outputs
}
