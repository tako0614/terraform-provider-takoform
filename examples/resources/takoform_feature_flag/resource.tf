terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_feature_flag" "example" {
  name               = "feature-flag"
  flag_key           = "new_checkout"
  enabled_percentage = 25
}

output "feature_flag_outputs" {
  value = takoform_feature_flag.example.outputs
}
