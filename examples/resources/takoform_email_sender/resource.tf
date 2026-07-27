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

resource "takoform_email_sender" "example" {
  name           = "email-sender"
  domain         = "portable-conformance.invalid"
  default_sender = "notifications@portable-conformance.invalid"
}

output "email_sender_outputs" {
  value = takoform_email_sender.example.outputs
}
