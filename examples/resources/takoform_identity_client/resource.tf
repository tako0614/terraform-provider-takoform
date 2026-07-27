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

resource "takoform_identity_client" "example" {
  name          = "identity-client"
  redirect_uris = ["https://app.portable-conformance.invalid/callback"]
  grant_types   = ["authorization_code"]
  auth_method   = "none"
}

output "identity_client_outputs" {
  value = takoform_identity_client.example.outputs
}
