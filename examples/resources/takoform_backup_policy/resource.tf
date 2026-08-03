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

resource "takoform_backup_policy" "example" {
  name           = "backup-policy"
  cron           = "0 3 * * *"
  retention_days = 14
  timezone       = "UTC"

  connections = [
    {
      name        = "origin"
      resource    = "RelationalDatabase/relational-database"
      permissions = ["administer"]
      projection  = "sql.admin.v1"
    },
  ]
}

output "backup_policy_outputs" {
  value = takoform_backup_policy.example.outputs
}
