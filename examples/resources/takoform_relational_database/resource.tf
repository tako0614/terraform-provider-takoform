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

resource "takoform_relational_database" "example" {
  name           = "relational-database"
  engine         = "postgres"
  engine_version = "16"
}

output "relational_database_outputs" {
  value = takoform_relational_database.example.outputs
}
