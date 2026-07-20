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
  name = "main"

  tables = [{
    name = "records"
    columns = [
      { name = "id", type = "string" },
      { name = "tenant_id", type = "string" },
      { name = "created_at", type = "integer" },
      { name = "score", type = "number", nullable = true },
    ]
    primary_key = ["id"]
    indexes = [{
      name    = "by_tenant_created"
      columns = ["tenant_id", "created_at"]
      unique  = true
    }]
  }]
}

output "database_resource_version" {
  value = takoform_sql_database.main.resource_version
}

output "database_outputs" {
  value = takoform_sql_database.main.outputs
}
