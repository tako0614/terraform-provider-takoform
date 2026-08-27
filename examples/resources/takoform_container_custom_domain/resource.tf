terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative Provider metadata. The current
      # exact FormRef and digest do not contain this name.
      # Provider 2.1.1's 15 versioned identities remain retained history.
      version = "= 3.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_container_custom_domain" "example" {
  name     = "container-custom-domain"
  service  = "container-service"
  hostname = "app.example.invalid"
}

output "container_custom_domain_outputs" {
  value = takoform_container_custom_domain.example.outputs_json
}
