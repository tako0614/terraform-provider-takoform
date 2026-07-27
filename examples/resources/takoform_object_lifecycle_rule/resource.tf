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

resource "takoform_object_lifecycle_rule" "example" {
  name              = "object-lifecycle-rule"
  prefix            = "logs/"
  expire_after_days = 90

  connections = [
    {
      name        = "store"
      resource    = "ObjectBucket/object-bucket"
      permissions = ["administer"]
      projection  = "object.lifecycle.v1"
    },
  ]
}

output "object_lifecycle_rule_outputs" {
  value = takoform_object_lifecycle_rule.example.outputs
}
